// Package online stores Online intent and remote access policy. It deliberately
// cannot activate an ingress until protocol and physical-locality guarantees exist.
package online

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const Version = 1
const WorkerProtocol = 2193

type Settings struct {
	PreferredInterface string                    `json:"preferred_interface,omitempty"`
	Version            int                       `json:"version"`
	PublicHost         string                    `json:"public_host"`
	TrustedSubnets     []string                  `json:"trusted_subnets"`
	Servers            map[string]ServerSettings `json:"servers"`
}
type ServerSettings struct {
	Enabled      bool   `json:"enabled"`
	AccountID    string `json:"account_id"`
	WorldName    string `json:"world_name"`
	ExternalPort int    `json:"external_port"`
}
type Player struct {
	XUID        string          `json:"xuid"`
	Gamertag    string          `json:"gamertag"`
	Permissions map[string]bool `json:"permissions"`
}
type AccountStatus struct {
	ID, Gamertag, Role string
	Healthy            bool
}
type Status struct {
	ServerID, Name, ServerState, State, AccountID, Gamertag, WorldName, LocalTarget, PublicTarget, Routing, Message string
	Broadcasting, RemoteVerified                                                                                    bool
	Players                                                                                                         int
}
type Event struct {
	Time                    time.Time
	ServerID, Type, Message string
}
type diskState struct {
	Version  int      `json:"version"`
	Settings Settings `json:"settings"`
	Players  []Player `json:"players"`
}
type Manager struct {
	mu       sync.RWMutex
	path     string
	servers  []config.Server
	settings Settings
	players  []Player
	events   []Event
	rename   func(string, string) error
}

func New(path string, servers []config.Server) (*Manager, error) {
	m := &Manager{path: path, servers: append([]config.Server(nil), servers...), rename: os.Rename, settings: Settings{Version: Version, Servers: map[string]ServerSettings{}}, players: []Player{}}
	for _, s := range servers {
		if s.ID == "" {
			return nil, errors.New("online server ID required")
		}
		if _, ok := m.settings.Servers[s.ID]; ok {
			return nil, errors.New("duplicate online server ID")
		}
		m.settings.Servers[s.ID] = ServerSettings{WorldName: s.LevelName}
	}
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return m, nil
	}
	if e != nil {
		return nil, e
	}
	if e = rejectDuplicateKeys(b); e != nil {
		return nil, e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	var state diskState
	if e = d.Decode(&state); e != nil {
		return nil, fmt.Errorf("decode online state: %w", e)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("trailing online state")
	}
	if state.Version != Version {
		return nil, errors.New("unsupported online state version")
	}
	if e = m.validate(state.Settings, state.Players); e != nil {
		return nil, e
	}
	m.settings = cloneSettings(state.Settings)
	m.players = clonePlayers(state.Players)
	return m, nil
}
func rejectDuplicateKeys(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	var value func() error
	value = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		if delim, ok := t.(json.Delim); ok {
			if delim == '{' {
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					if e != nil {
						return e
					}
					key, ok := k.(string)
					if !ok || seen[key] {
						return errors.New("duplicate online JSON key")
					}
					seen[key] = true
					if e = value(); e != nil {
						return e
					}
				}
				_, e = d.Token()
				return e
			}
			if delim == '[' {
				for d.More() {
					if e = value(); e != nil {
						return e
					}
				}
				_, e = d.Token()
				return e
			}
		}
		return nil
	}
	return value()
}
func safeText(s string, max int) bool {
	if len(s) > max || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validHost(s string) bool {
	if s == "" {
		return true
	}
	if net.ParseIP(s) != nil {
		return true
	}
	if len(s) > 253 || !safeText(s, 253) {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if len(part) == 0 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
func validXUID(s string) bool {
	n, e := strconv.ParseUint(s, 10, 64)
	return e == nil && n > 0 && strconv.FormatUint(n, 10) == s
}
func (m *Manager) validate(s Settings, players []Player) error {
	if s.Version != Version {
		return errors.New("unsupported online settings version")
	}
	if !safeText(s.PreferredInterface, 128) {
		return errors.New("invalid preferred interface metadata")
	}
	if !validHost(s.PublicHost) {
		return errors.New("public host must be a host without scheme, path or port")
	}
	seenSubnets := map[string]bool{}
	for _, v := range s.TrustedSubnets {
		_, n, e := net.ParseCIDR(v)
		if e != nil || seenSubnets[n.String()] {
			return errors.New("invalid or duplicate trusted subnet")
		}
		seenSubnets[n.String()] = true
	}
	if len(s.Servers) != len(m.servers) {
		return errors.New("online settings require every configured server")
	}
	known := map[string]bool{}
	for _, v := range m.servers {
		known[v.ID] = true
	}
	for id, v := range s.Servers {
		if !known[id] {
			return errors.New("unknown online server")
		}
		if v.ExternalPort < 0 || v.ExternalPort > 65535 {
			return errors.New("external port must be 0 (unset) or 1 through 65535")
		}
		if !safeText(v.AccountID, 128) || !safeText(v.WorldName, 256) {
			return errors.New("invalid online account or world name")
		}
	}
	seen := map[string]bool{}
	for _, p := range players {
		if !validXUID(p.XUID) || p.Gamertag == "" || !safeText(p.Gamertag, 64) || seen[p.XUID] {
			return errors.New("invalid or duplicate resolved player identity")
		}
		seen[p.XUID] = true
		for id := range p.Permissions {
			if !known[id] {
				return errors.New("unknown player permission server")
			}
		}
	}
	return nil
}
func cloneSettings(s Settings) Settings {
	s.TrustedSubnets = append([]string(nil), s.TrustedSubnets...)
	v := map[string]ServerSettings{}
	for k, x := range s.Servers {
		v[k] = x
	}
	s.Servers = v
	return s
}
func clonePlayers(p []Player) []Player {
	out := make([]Player, len(p))
	for i, v := range p {
		out[i] = v
		out[i].Permissions = map[string]bool{}
		for k, b := range v.Permissions {
			out[i].Permissions[k] = b
		}
	}
	return out
}
func (m *Manager) Settings() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneSettings(m.settings)
}
func (m *Manager) Players() []Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return clonePlayers(m.players)
}
func (m *Manager) persist(s Settings, p []Player) error {
	if e := m.validate(s, p); e != nil {
		return e
	}
	b, e := json.MarshalIndent(diskState{Version: Version, Settings: s, Players: p}, "", "  ")
	if e != nil {
		return e
	}
	if e = privateDirectory(filepath.Dir(m.path)); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(m.path), ".online-*.tmp")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	if e = m.rename(tmp, m.path); e != nil {
		return e
	}
	m.settings = cloneSettings(s)
	m.players = clonePlayers(p)
	return nil
}
func (m *Manager) record(server, kind, message string) {
	m.events = append(m.events, Event{Time: time.Now().UTC(), ServerID: server, Type: kind, Message: message})
	if len(m.events) > 200 {
		m.events = append([]Event(nil), m.events[len(m.events)-200:]...)
	}
}
func (m *Manager) Save(s Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.persist(cloneSettings(s), m.players); e != nil {
		return e
	}
	m.record("", "settings", "Online settings saved; production activation remains blocked.")
	return nil
}
func (m *Manager) GrantResolved(p Player) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := clonePlayers(m.players)
	found := false
	for i, v := range next {
		if v.XUID == p.XUID {
			next[i] = p
			found = true
		}
	}
	if !found {
		next = append(next, p)
	}
	sort.Slice(next, func(i, j int) bool { return next[i].XUID < next[j].XUID })
	if e := m.persist(m.settings, next); e != nil {
		return e
	}
	m.record("", "authorization", "Resolved player permissions saved.")
	return nil
}
func (m *Manager) Revoke(xuid, server string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !validXUID(xuid) {
		return errors.New("invalid XUID")
	}
	if _, ok := m.settings.Servers[server]; !ok {
		return errors.New("unknown server")
	}
	next := clonePlayers(m.players)
	for i := range next {
		if next[i].XUID == xuid {
			delete(next[i].Permissions, server)
		}
	}
	if e := m.persist(m.settings, next); e != nil {
		return e
	}
	m.record(server, "authorization", "Server permission revoked.")
	return nil
}
func (m *Manager) Remove(xuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !validXUID(xuid) {
		return errors.New("invalid XUID")
	}
	next := []Player{}
	for _, p := range m.players {
		if p.XUID != xuid {
			next = append(next, p)
		}
	}
	if e := m.persist(m.settings, next); e != nil {
		return e
	}
	m.record("", "authorization", "Player permissions removed.")
	return nil
}

// Authorize is a policy predicate only; it cannot enable the blocked ingress.
func (m *Manager) Authorize(server, xuid string, authenticated bool) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !authenticated || !validXUID(xuid) {
		return false
	}
	if _, ok := m.settings.Servers[server]; !ok {
		return false
	}
	for _, p := range m.players {
		if p.XUID == xuid {
			return p.Permissions[server]
		}
	}
	return false
}
func (m *Manager) UsesAccount(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id == "" {
		return false
	}
	for _, s := range m.settings.Servers {
		if s.AccountID == id {
			return true
		}
	}
	return false
}
func (m *Manager) Events(server, typeFilter string) []Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []Event{}
	for _, v := range m.events {
		if (server == "" || v.ServerID == server) && (typeFilter == "" || v.Type == typeFilter) {
			out = append(out, v)
		}
	}
	return out
}
func (m *Manager) Statuses(statuses []supervisor.Status, accounts []AccountStatus) []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.servers))
	for _, server := range m.servers {
		s := m.settings.Servers[server.ID]
		v := Status{ServerID: server.ID, Name: server.Name, State: "Disabled", AccountID: s.AccountID, WorldName: s.WorldName, LocalTarget: server.Backend, Routing: "Blocked: physical LAN locality cannot be proven from NetherNet candidates", Message: "Online is disabled."}
		if m.settings.PublicHost != "" && s.ExternalPort != 0 {
			v.PublicTarget = net.JoinHostPort(m.settings.PublicHost, strconv.Itoa(s.ExternalPort))
		}
		var live supervisor.Status
		for _, x := range statuses {
			if x.ID == server.ID {
				live = x
				break
			}
		}
		v.ServerState = string(live.State)
		v.Players = live.Players
		var account AccountStatus
		for _, a := range accounts {
			if a.ID == s.AccountID {
				account = a
				break
			}
		}
		v.Gamertag = account.Gamertag
		if s.Enabled {
			switch {
			case live.State != supervisor.Running && live.State != supervisor.ExternallyRunning && live.State != supervisor.Starting:
				v.State = "Server stopped"
				v.Message = "Start the server from Servers."
			case !live.Ready || live.State == supervisor.Starting:
				v.State = "Waiting for server"
				v.Message = "Waiting for server readiness."
			case s.AccountID == "" || account.ID == "":
				v.State = "Account missing"
				v.Message = "Select a dedicated broadcaster account from Accounts."
			case account.Role != "broadcaster":
				v.State = "Blocked"
				v.Message = "Selected account must have the broadcaster role."
			case !account.Healthy:
				v.State = "Authentication required"
				v.Message = "Authenticate the broadcaster in Accounts."
			default:
				v.State = "Blocked"
				v.Message = fmt.Sprintf("Production activation blocked: worker protocol %d; server protocol %d. Remote ACL synchronization to ingress is unverified. Authenticated per-server ingress and direct-IP bypass protection are unverified. Physical LAN locality cannot be proven; native allowlist cannot separate LAN helper XUIDs from remote access.", WorkerProtocol, server.Protocol)
			}
		}
		out = append(out, v)
	}
	return out
}
