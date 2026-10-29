package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/backend"
	"nethernet-multi-server/internal/config"
	identitystore "nethernet-multi-server/internal/identity"
	"nethernet-multi-server/internal/online"
	"nethernet-multi-server/internal/onlineguard"
	"nethernet-multi-server/internal/onlineruntime"
	"nethernet-multi-server/internal/onlineworker"
	proxyservice "nethernet-multi-server/internal/proxy"
	"nethernet-multi-server/internal/supervisor"
)

type onlineRuntimeHost struct {
	ctx               context.Context
	cancel            context.CancelFunc
	cfg               config.Config
	accounts          *accounts.Manager
	online            *online.Manager
	supervisor        *supervisor.Manager
	coordinator       *onlineruntime.Coordinator
	wake, inspectWake chan struct{}
	mu                sync.RWMutex
	evidence          onlineguard.Evidence
	inspectionFailed  bool
	plan              onlineguard.Plan
	report            onlineguard.Report
	blockers          map[string]string
	protected         map[string]bool
	lifecycle         atomic.Uint64
}

// startOnlineRuntime is called only after normal LAN startup and installation
// activation checks. Both background loops remain idle for disabled Online.
func startOnlineRuntime(parent context.Context, cfg config.Config, am *accounts.Manager, om *online.Manager, sm *supervisor.Manager, recorder proxyservice.SessionRecorder, log *slog.Logger, workerPath, identityDirectory string) *onlineRuntimeHost {
	ctx, cancel := context.WithCancel(parent)
	h := &onlineRuntimeHost{ctx: ctx, cancel: cancel, cfg: cfg, accounts: am, online: om, supervisor: sm, wake: make(chan struct{}, 1), inspectWake: make(chan struct{}, 1), blockers: map[string]string{}, protected: map[string]bool{}}
	h.coordinator = onlineruntime.New(onlineruntime.Factories{
		StartGateway: func(ctx context.Context, d onlineruntime.Desired) (onlineruntime.Gateway, error) {
			if !onlineworker.Compatible(ctx, workerPath) {
				return nil, errors.New("compatible pinned Online worker required")
			}
			server, ok := h.server(d.ServerID)
			if !ok {
				return nil, errors.New("Online server unavailable")
			}
			server.Name = d.World
			networkID, err := server.ParsedNetworkID()
			if err != nil {
				return nil, err
			}
			key, _, err := (identitystore.Store{Directory: identityDirectory}).LoadOrCreate(networkID)
			if err != nil {
				return nil, err
			}
			publicIP, err := onlineGatewayIPv4(ctx, d.Host)
			if err != nil {
				return nil, err
			}
			return proxyservice.StartRemoteGateway(ctx, net.JoinHostPort("0.0.0.0", strconv.Itoa(d.Port)), proxyservice.RemoteGatewayOptions{OperatorKey: key, PublicIPv4: publicIP, InterfaceIP: net.ParseIP(cfg.InterfaceIP), Server: server, Leaser: am, Dialer: backend.NetherNetDialer{Log: log}, Recorder: recorder, Authorize: func(xuid string) bool { return h.authorize(d.ServerID, xuid) }, Log: log})
		},
		StartWorker: func(ctx context.Context, d onlineruntime.Desired, event func(onlineruntime.Event)) (onlineruntime.Worker, error) {
			server, ok := h.server(d.ServerID)
			if !ok {
				return nil, errors.New("Online server unavailable")
			}
			bootstrap := onlineworker.Bootstrap{Type: "bootstrap", Version: 1, ServerID: d.ServerID, AccountID: d.AccountID, GatewayHost: d.Host, GatewayPort: uint16(d.Port), WorldName: d.World, ExpectedProtocol: online.WorkerProtocol, AllowUnknownFriendsRoute: d.AllowUnknownFriendsRoute}
			bootstrap.GameType = strings.ToLower(server.GameType)
			bootstrap.MaxPlayers = int(server.MaxPlayerCount)
			return onlineworker.Start(ctx, workerPath, bootstrap, onlineworker.Callbacks{
				Token: func(tokenCtx context.Context) (*oauth2.Token, error) {
					source, err := am.BroadcasterTokenSource(tokenCtx, d.AccountID)
					var token *oauth2.Token
					if err == nil {
						token, err = source.Token()
					}
					if err != nil || token == nil || token.AccessToken == "" || !token.Expiry.After(time.Now().Add(30*time.Second)) {
						_ = am.RequireBroadcasterAuthentication(d.AccountID)
						event(onlineruntime.Event{Code: "auth_required"})
						om.RecordRuntime(d.ServerID, "authentication_required")
						h.changed()
						return nil, errors.New("broadcaster authentication required")
					}
					return token, nil
				},
				Authorize: func(xuid string) bool { return h.authorize(d.ServerID, xuid) },
				Event: func(code string) {
					om.RecordRuntime(d.ServerID, code)
					if code == "authentication_required" {
						_ = am.RequireBroadcasterAuthentication(d.AccountID)
						event(onlineruntime.Event{Code: "auth_required"})
						h.changed()
						return
					}
					event(onlineruntime.Event{Code: code})
				},
			})
		},
	}, onlineruntime.Options{})
	go h.loop()
	go h.inspectLoop()
	return h
}

func onlineGatewayIPv4(ctx context.Context, host string) (netip.Addr, error) {
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
	if err != nil {
		return netip.Addr{}, errors.New("public gateway IPv4 resolution failed")
	}
	for _, a := range addresses {
		a = a.Unmap()
		if a.Is4() && a.IsGlobalUnicast() && !a.IsPrivate() && !a.IsLoopback() && !a.IsLinkLocalUnicast() {
			return a, nil
		}
	}
	return netip.Addr{}, errors.New("public gateway requires a routable IPv4 address")
}

func (h *onlineRuntimeHost) server(id string) (config.Server, bool) {
	for _, s := range h.cfg.Servers {
		if s.ID == id {
			return s, true
		}
	}
	return config.Server{}, false
}
func onlineRuntimeAuthorized(manager *online.Manager, id, xuid string, live supervisor.Status) bool {
	if manager == nil || live.ID != id || !live.Ready || (live.State != supervisor.Running && live.State != supervisor.ExternallyRunning) {
		return false
	}
	settings := manager.Settings()
	return settings.Servers[id].Enabled && settings.AllowUnknownFriendsRoute && manager.Authorize(id, xuid, true)
}
func (h *onlineRuntimeHost) authorize(id, xuid string) bool {
	if !onlineRuntimeAuthorized(h.online, id, xuid, h.supervisor.Status(id)) {
		return false
	}
	h.mu.RLock()
	evidence, inspectionFailed := h.evidence, h.inspectionFailed
	h.mu.RUnlock()
	// Admission must not depend on the reconciliation loop, which can be
	// waiting on account persistence or child startup. Rebuild from current
	// host addresses and evaluate evidence freshness for every ACL check.
	if inspectionFailed {
		return false
	}
	addresses, err := currentOnlineAddresses()
	if err != nil || !onlineAdmissionProtection(h.cfg, evidence, inspectionFailed, addresses) {
		return false
	}
	server, ok := h.server(id)
	return ok && onlineBackendAuthenticated(server)
}

func onlineAdmissionProtection(cfg config.Config, evidence onlineguard.Evidence, inspectionFailed bool, addresses []string) bool {
	if inspectionFailed {
		return false
	}
	plan, err := onlineProtectionPlan(cfg, addresses)
	return err == nil && onlineguard.Evaluate(plan, evidence).SnapshotMatches
}
func onlineBackendAuthenticated(server config.Server) bool {
	if server.Process == nil {
		return false
	}
	b, err := os.ReadFile(filepath.Join(server.Process.WorkingDirectory, "server.properties"))
	if err != nil {
		return false
	}
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "online-mode" {
			// Reject duplicate properties rather than relying on differing parsers.
			if found || strings.TrimSpace(value) != "true" {
				return false
			}
			found = true
		}
	}
	return found
}
func canonicalOnlineAddresses(raw []string) ([]string, error) {
	seen := map[string]bool{}
	for _, value := range raw {
		host, _, _ := strings.Cut(value, "/")
		host, _, _ = strings.Cut(host, "%")
		addr, err := netip.ParseAddr(host)
		if err != nil {
			return nil, errors.New("host address inventory unavailable")
		}
		addr = addr.Unmap()
		if addr.IsUnspecified() || addr.IsMulticast() {
			return nil, errors.New("host address inventory invalid")
		}
		seen[addr.String()] = true
	}
	out := make([]string, 0, len(seen))
	for address := range seen {
		out = append(out, address)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, errors.New("host address inventory empty")
	}
	return out, nil
}
func currentOnlineAddresses() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	raw := make([]string, 0, len(addrs))
	for _, a := range addrs {
		raw = append(raw, a.String())
	}
	return canonicalOnlineAddresses(raw)
}
func onlineProtectionPlan(cfg config.Config, addresses []string) (onlineguard.Plan, error) {
	var backends []onlineguard.Backend
	for _, s := range cfg.Servers {
		if s.Process != nil {
			backends = append(backends, onlineguard.Backend{ServerID: s.ID, Program: s.Process.Executable, Target: s.Backend})
		}
	}
	return onlineguard.BuildPlan(backends, addresses)
}
func (h *onlineRuntimeHost) changed() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
	select {
	case h.inspectWake <- struct{}{}:
	default:
	}
}
func (h *onlineRuntimeHost) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer h.coordinator.Close()
	for {
		h.reconcile()
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
		case <-h.wake:
		}
	}
}
func (h *onlineRuntimeHost) anyEnabled() bool {
	for _, s := range h.online.Settings().Servers {
		if s.Enabled {
			return true
		}
	}
	return false
}
func (h *onlineRuntimeHost) inspectLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var last time.Time
	for {
		if h.anyEnabled() && (last.IsZero() || time.Since(last) >= 30*time.Second) {
			ctx, cancel := context.WithTimeout(h.ctx, 25*time.Second)
			evidence, err := onlineguard.Inspect(ctx)
			cancel()
			last = time.Now()
			h.mu.Lock()
			h.evidence = evidence
			h.inspectionFailed = err != nil
			h.mu.Unlock()
			select {
			case h.wake <- struct{}{}:
			default:
			}
		}
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
		case <-h.inspectWake:
		}
	}
}
func (h *onlineRuntimeHost) reconcile() {
	settings := h.online.Settings()
	h.mu.RLock()
	evidence, inspectionFailed := h.evidence, h.inspectionFailed
	h.mu.RUnlock()
	var plan onlineguard.Plan
	var report onlineguard.Report
	planOK := false
	if h.anyEnabled() {
		if addresses, err := currentOnlineAddresses(); err == nil {
			if built, err := onlineProtectionPlan(h.cfg, addresses); err == nil {
				plan = built
				report = onlineguard.Evaluate(plan, evidence)
				planOK = report.SnapshotMatches && !inspectionFailed
			}
		}
	}
	accountsByID := map[string]accountstore.Account{}
	for _, a := range h.accounts.Snapshot().Accounts {
		accountsByID[a.ID] = a
	}
	desired := make([]onlineruntime.Desired, 0, len(h.cfg.Servers))
	blockers := map[string]string{}
	protected := map[string]bool{}
	for _, server := range h.cfg.Servers {
		s := settings.Servers[server.ID]
		live := h.supervisor.Status(server.ID)
		account := accountsByID[s.AccountID]
		auth := onlineBackendAuthenticated(server)
		protection := planOK && auth
		protected[server.ID] = protection
		if s.Enabled {
			switch {
			case !auth:
				blockers[server.ID] = "BDS must explicitly retain online-mode=true; Online is blocked."
			case inspectionFailed:
				blockers[server.ID] = "Read-only Windows firewall inspection unavailable or access denied; Online protection is not verified."
			case !planOK:
				blockers[server.ID] = "Effective host-only BDS protection is missing, stale or differs from current host addresses; Online is blocked. No firewall changes were made."
			}
		}
		d := onlineruntime.Desired{ServerID: server.ID, AccountID: s.AccountID, Host: settings.PublicHost, World: s.WorldName, Port: s.ExternalPort, Enabled: s.Enabled, AllowUnknownFriendsRoute: settings.AllowUnknownFriendsRoute, Ready: live.Ready && (live.State == supervisor.Running || live.State == supervisor.ExternallyRunning), AccountHealthy: account.ID != "" && accountstore.EffectiveRole(account.Role) == accountstore.RoleBroadcaster && account.Healthy, ProtectionVerified: protection}
		d.Lifecycle = h.lifecycle.Load()
		revision, _ := json.Marshal([]any{d.AccountID, d.Host, d.World, d.Port, d.AllowUnknownFriendsRoute, plan.ID})
		sum := sha256.Sum256(revision)
		d.Revision = hex.EncodeToString(sum[:])
		desired = append(desired, d)
	}
	h.mu.Lock()
	h.plan, h.report, h.blockers, h.protected = plan, report, blockers, protected
	h.mu.Unlock()
	h.coordinator.Reconcile(h.ctx, desired)
}
func (h *onlineRuntimeHost) backendStopping(id string) {
	h.coordinator.BackendStopping(id, h.lifecycle.Add(1))
}
func (h *onlineRuntimeHost) status(rows []online.Status) []online.Status {
	live := map[string]onlineruntime.State{}
	for _, s := range h.coordinator.Snapshot() {
		live[s.ServerID] = s
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	messages := map[string]string{"starting": "Starting authenticated Online gateway.", "advertising": "Friends advertisement active; external joining remains unverified.", "blocked": "Online prerequisites are not satisfied.", "authentication required": "Reauthenticate the broadcaster in Accounts.", "retrying": "Online worker unavailable; bounded automatic retry is pending.", "failed": "Online retries exhausted. Review diagnostics and change configuration before retrying.", "disabled": "Online is disabled."}
	for i := range rows {
		rows[i].RemoteVerified = false
		rows[i].Broadcasting = false
		rows[i].Routing = "Existing LAN discovery remains local. Friends-route locality is unknown and uses the authenticated gateway; physical LAN-first routing is not verified."
		if s, ok := live[rows[i].ServerID]; ok {
			rows[i].State = s.Status
			rows[i].Broadcasting = s.Status == "advertising"
			rows[i].Message = messages[s.Status]
			if reason := h.blockers[rows[i].ServerID]; reason != "" && s.Status == "blocked" {
				rows[i].Message = reason
			}
		}
	}
	return rows
}
func (h *onlineRuntimeHost) diagnostics() any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return struct {
		Plan                  onlineguard.Plan
		Report                onlineguard.Report
		Evidence              onlineguard.Evidence
		InspectionUnavailable bool
		RemoteVerified        bool
	}{h.plan, h.report, h.evidence, h.inspectionFailed, false}
}
func (h *onlineRuntimeHost) close() error { h.cancel(); return h.coordinator.Close() }
