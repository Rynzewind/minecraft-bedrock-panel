package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/resource"
	"golang.org/x/oauth2"
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/backend"
	"nethernet-multi-server/internal/config"
)

// RemoteGatewayOptions deliberately contains no server lifecycle or LAN control.
// Authorize must be concurrency-safe and query the current explicit XUID ACL.
type RemoteGatewayOptions struct {
	Server    config.Server
	Leaser    AccountLeaser
	Dialer    backend.Dialer
	Recorder  SessionRecorder
	Authorize func(string) bool
	Log       *slog.Logger
}

type remoteGatewayConnection interface {
	IdentityData() login.IdentityData
	Close() error
}
type remoteGatewayListener interface {
	Accept() (remoteGatewayConnection, error)
	Close() error
}
type authenticatedGatewayListener struct{ *minecraft.Listener }

func (l authenticatedGatewayListener) Accept() (remoteGatewayConnection, error) {
	raw, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	conn, ok := raw.(*minecraft.Conn)
	if !ok {
		_ = raw.Close()
		return nil, errors.New("unexpected gateway connection type")
	}
	return conn, nil
}

type remoteGatewaySession struct {
	conn   remoteGatewayConnection
	cancel context.CancelFunc
	xuid   string
}

// RemoteGateway owns only its authenticated remote listener and accepted clients.
type RemoteGateway struct {
	watchdogDone chan struct{}
	mu           sync.Mutex
	listener     remoteGatewayListener
	authorize    func(string) bool
	run          func(context.Context, remoteGatewayConnection)
	log          *slog.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	active       map[*remoteGatewaySession]struct{}
	closed       bool
}

// StartRemoteGateway opens only the explicitly requested numeric IP and port.
// It does not configure firewall rules, expose BDS, or change LAN discovery.
func StartRemoteGateway(ctx context.Context, bindAddress string, opts RemoteGatewayOptions) (*RemoteGateway, error) {
	if ctx == nil {
		return nil, errors.New("gateway context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host, port, err := net.SplitHostPort(bindAddress)
	if err != nil || net.ParseIP(host) == nil {
		return nil, errors.New("gateway bind address must contain an explicit IP and port")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, errors.New("gateway bind port must be between 1 and 65535")
	}
	if opts.Authorize == nil || opts.Leaser == nil || opts.Dialer == nil {
		return nil, errors.New("gateway authorization, helper leaser and backend dialer are required")
	}
	if strings.TrimSpace(opts.Server.Name) == "" || strings.TrimSpace(opts.Server.Backend) == "" {
		return nil, errors.New("gateway target server is required")
	}
	logger := opts.Log
	if logger == nil {
		logger = slog.Default()
	}
	listenConfig := remoteGatewayListenConfig(logger, opts.Authorize)
	listenConfig.TexturePacksRequired = texturePacksRequired(opts.Server)
	listenConfig.FetchResourcePacks = func(_ login.IdentityData, _ login.ClientData, current []*resource.Pack) []*resource.Pack {
		packs, err := activeResourcePacks(opts.Server)
		if err != nil {
			logger.Error("load active gateway resource packs", "server", opts.Server.Name, "error", err)
			return current
		}
		return packs
	}
	listener, err := listenConfig.Listen("raknet", bindAddress)
	if err != nil {
		return nil, fmt.Errorf("start authenticated remote gateway: %w", err)
	}
	target := Target{Name: opts.Server.Name, BackendURL: opts.Server.BackendURL(), Listener: listener}
	run := func(sessionCtx context.Context, raw remoteGatewayConnection) {
		inbound := raw.(*minecraft.Conn)
		xuid := inbound.IdentityData().XUID
		gate := func() bool { return sessionCtx.Err() == nil && xuid != "" && opts.Authorize(xuid) }
		serveSession(sessionCtx, target, inbound, remoteGatewayDialer{base: opts.Dialer, allowed: gate}, remoteGatewayLeaser{base: opts.Leaser, allowed: gate}, opts.Recorder, logger)
	}
	return newRemoteGateway(ctx, authenticatedGatewayListener{listener}, opts.Authorize, run, logger), nil
}

func remoteGatewayListenConfig(log *slog.Logger, authorize func(string) bool) minecraft.ListenConfig {
	// AuthenticationDisabled stays false. Accepted IdentityData comes from the
	// listener's verified Xbox/Minecraft login, never a forwarding header.
	return minecraft.ListenConfig{ErrorLog: log, LoginTimeout: 15 * time.Second,
		// The authenticated login hook runs before encryption completion and resource
		// pack transfer. Accept-time ACL checks alone would expose private packs.
		Allow: func(_ net.Addr, identity login.IdentityData, _ login.ClientData) (string, bool) {
			if identity.XUID == "" || authorize == nil || !authorize(identity.XUID) {
				return "Online access denied.", false
			}
			return "", true
		},
	}
}

func newRemoteGateway(parent context.Context, l remoteGatewayListener, authorize func(string) bool, run func(context.Context, remoteGatewayConnection), log *slog.Logger) *RemoteGateway {
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(parent)
	g := &RemoteGateway{listener: l, authorize: authorize, run: run, log: log, ctx: ctx, cancel: cancel, active: make(map[*remoteGatewaySession]struct{}), watchdogDone: make(chan struct{})}
	go g.accept()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		g.authorizationWatchdog(ticker.C)
	}()
	go func() { <-ctx.Done(); _ = g.Close() }()
	return g
}

func (g *RemoteGateway) accept() {
	defer g.Close()
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			return
		}
		xuid := conn.IdentityData().XUID
		if xuid == "" || !g.authorize(xuid) {
			_ = conn.Close()
			continue
		}
		ctx, cancel := context.WithCancel(g.ctx)
		s := &remoteGatewaySession{conn: conn, cancel: cancel, xuid: xuid}
		g.mu.Lock()
		if g.closed {
			g.mu.Unlock()
			cancel()
			_ = conn.Close()
			return
		}
		g.active[s] = struct{}{}
		g.mu.Unlock()
		go g.serve(ctx, s)
	}
}

func (g *RemoteGateway) serve(ctx context.Context, s *remoteGatewaySession) {
	defer func() {
		s.cancel()
		_ = s.conn.Close()
		g.mu.Lock()
		delete(g.active, s)
		g.mu.Unlock()
		if recovered := recover(); recovered != nil {
			g.log.Error("remote gateway session panic recovered", "panic", recovered)
		}
	}()
	if ctx.Err() != nil || !g.authorize(s.xuid) {
		return
	}
	g.run(ctx, s.conn)
}

// authorizationWatchdog owns an independent revocation loop. The main runtime
// reconciliation may wait for account refresh or other service operations; that
// must not extend the lifetime of a session whose current authorization fails.
func (g *RemoteGateway) authorizationWatchdog(ticks <-chan time.Time) {
	defer close(g.watchdogDone)
	for {
		select {
		case <-g.ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok || g.ctx.Err() != nil {
				return
			}
			g.Reconcile()
		}
	}
}

// Reconcile disconnects sessions whose current authenticated XUID is no longer
// authorised. Invoke after every ACL update; future admissions always recheck it.
func (g *RemoteGateway) Reconcile() {
	g.mu.Lock()
	sessions := make([]*remoteGatewaySession, 0, len(g.active))
	for s := range g.active {
		sessions = append(sessions, s)
	}
	g.mu.Unlock()
	for _, s := range sessions {
		if !g.authorize(s.xuid) {
			s.cancel()
			_ = s.conn.Close()
		}
	}
}

// Close is idempotent and cancels all owned clients without stopping BDS or LAN.
func (g *RemoteGateway) Close() error {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return nil
	}
	g.closed = true
	sessions := make([]*remoteGatewaySession, 0, len(g.active))
	for s := range g.active {
		sessions = append(sessions, s)
	}
	g.mu.Unlock()
	g.cancel()
	err := g.listener.Close()
	for _, s := range sessions {
		s.cancel()
		_ = s.conn.Close()
	}
	return err
}

type remoteGatewayLeaser struct {
	base    AccountLeaser
	allowed func() bool
}

func (l remoteGatewayLeaser) Acquire(ctx context.Context, p accounts.Player, server string) (*accounts.Lease, error) {
	if ctx.Err() != nil || p.XUID == "" || !l.allowed() {
		return nil, errors.New("remote access is not authorised")
	}
	lease, err := l.base.Acquire(ctx, p, server)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || !l.allowed() {
		lease.Release()
		return nil, errors.New("remote access was revoked")
	}
	return lease, nil
}

type remoteGatewayDialer struct {
	base    backend.Dialer
	allowed func() bool
}

func (d remoteGatewayDialer) Dial(ctx context.Context, url string, inbound *minecraft.Conn, tokens oauth2.TokenSource) (*minecraft.Conn, error) {
	if ctx.Err() != nil || !d.allowed() {
		return nil, errors.New("remote access is not authorised")
	}
	conn, err := d.base.Dial(ctx, url, inbound, tokens)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || !d.allowed() {
		_ = conn.Close()
		return nil, errors.New("remote access was revoked")
	}
	return conn, nil
}
