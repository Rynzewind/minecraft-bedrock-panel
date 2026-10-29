package proxy

import (
	"context"
	"errors"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"golang.org/x/oauth2"
	"io"
	"nethernet-multi-server/internal/accounts"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type gatewayTestLeaser func(context.Context, accounts.Player, string) (*accounts.Lease, error)

func (f gatewayTestLeaser) Acquire(ctx context.Context, p accounts.Player, s string) (*accounts.Lease, error) {
	return f(ctx, p, s)
}

type gatewayTestDialer func(context.Context, string, *minecraft.Conn, oauth2.TokenSource) (*minecraft.Conn, error)

func (f gatewayTestDialer) Dial(ctx context.Context, url string, c *minecraft.Conn, t oauth2.TokenSource) (*minecraft.Conn, error) {
	return f(ctx, url, c, t)
}

func TestRemoteGatewayGuardsHelperAndBackendAtUse(t *testing.T) {
	var leases, backends, releases int
	allowed := false
	leaser := remoteGatewayLeaser{allowed: func() bool { return allowed }, base: gatewayTestLeaser(func(context.Context, accounts.Player, string) (*accounts.Lease, error) {
		leases++
		allowed = false
		return &accounts.Lease{Release: func() { releases++ }}, nil
	})}
	dialer := remoteGatewayDialer{allowed: func() bool { return allowed }, base: gatewayTestDialer(func(context.Context, string, *minecraft.Conn, oauth2.TokenSource) (*minecraft.Conn, error) {
		backends++
		return nil, errors.New("test backend")
	})}
	player := accounts.Player{XUID: "allowed"}
	if _, err := leaser.Acquire(context.Background(), player, "server"); err == nil || leases != 0 {
		t.Fatal("denied access acquired helper")
	}
	if _, err := dialer.Dial(context.Background(), "target", nil, nil); err == nil || backends != 0 {
		t.Fatal("denied access reached backend")
	}
	allowed = true
	if _, err := leaser.Acquire(context.Background(), player, "server"); err == nil || leases != 1 || releases != 1 {
		t.Fatal("revocation during acquisition did not release helper")
	}
	allowed = true
	if _, err := leaser.Acquire(context.Background(), accounts.Player{}, "server"); err == nil || leases != 1 {
		t.Fatal("empty identity acquired helper")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dialer.Dial(ctx, "target", nil, nil); err == nil || backends != 0 {
		t.Fatal("cancelled session reached backend")
	}
}

func TestRemoteGatewayConfigurationRequiresAuthenticatedExplicitBind(t *testing.T) {
	if remoteGatewayListenConfig(nil, nil).AuthenticationDisabled {
		t.Fatal("remote login authentication disabled")
	}
	for _, bind := range []string{"", ":19132", "localhost:19132", "127.0.0.1:0", "127.0.0.1:65536"} {
		if _, err := StartRemoteGateway(context.Background(), bind, RemoteGatewayOptions{}); err == nil {
			t.Fatalf("unsafe/invalid bind %q accepted", bind)
		}
	}
}

type gatewayTestConn struct {
	xuid   string
	closed chan struct{}
	once   sync.Once
}

func (c *gatewayTestConn) IdentityData() login.IdentityData { return login.IdentityData{XUID: c.xuid} }
func (c *gatewayTestConn) Close() error                     { c.once.Do(func() { close(c.closed) }); return nil }

type gatewayTestListener struct {
	accepted chan remoteGatewayConnection
	closed   chan struct{}
	once     sync.Once
}

func (l *gatewayTestListener) Accept() (remoteGatewayConnection, error) {
	select {
	case c := <-l.accepted:
		return c, nil
	case <-l.closed:
		return nil, io.EOF
	}
}
func (l *gatewayTestListener) Close() error { l.once.Do(func() { close(l.closed) }); return nil }
func newGatewayTestListener() *gatewayTestListener {
	return &gatewayTestListener{accepted: make(chan remoteGatewayConnection, 10), closed: make(chan struct{})}
}
func gatewayWait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("gateway action timed out")
	}
}

func TestRemoteGatewayDeniesBeforeSessionAndRejectsEmptyIdentity(t *testing.T) {
	l := newGatewayTestListener()
	var sessions atomic.Int32
	g := newRemoteGateway(context.Background(), l, func(x string) bool { return x == "allowed" }, func(context.Context, remoteGatewayConnection) { sessions.Add(1) }, nil)
	defer g.Close()
	for _, x := range []string{"denied", ""} {
		c := &gatewayTestConn{xuid: x, closed: make(chan struct{})}
		l.accepted <- c
		gatewayWait(t, c.closed)
	}
	if sessions.Load() != 0 {
		t.Fatal("denied client reached helper/session")
	}
}

func TestRemoteGatewayRevokesAndClosesConcurrentSessions(t *testing.T) {
	l := newGatewayTestListener()
	var allowed atomic.Bool
	allowed.Store(true)
	entered := make(chan struct{}, 2)
	g := newRemoteGateway(context.Background(), l, func(string) bool { return allowed.Load() }, func(ctx context.Context, c remoteGatewayConnection) { entered <- struct{}{}; <-ctx.Done() }, nil)
	first := &gatewayTestConn{xuid: "first", closed: make(chan struct{})}
	second := &gatewayTestConn{xuid: "second", closed: make(chan struct{})}
	l.accepted <- first
	l.accepted <- second
	gatewayWait(t, entered)
	gatewayWait(t, entered)
	allowed.Store(false)
	g.Reconcile()
	gatewayWait(t, first.closed)
	gatewayWait(t, second.closed)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	gatewayWait(t, l.closed)
}

func TestRemoteGatewayCloseAndParentCancelStopActiveClients(t *testing.T) {
	for _, parentCancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "close", true: "cancel"}[parentCancel], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			l := newGatewayTestListener()
			entered := make(chan struct{})
			g := newRemoteGateway(ctx, l, func(string) bool { return true }, func(ctx context.Context, c remoteGatewayConnection) { close(entered); <-ctx.Done() }, nil)
			c := &gatewayTestConn{xuid: "allowed", closed: make(chan struct{})}
			l.accepted <- c
			gatewayWait(t, entered)
			if parentCancel {
				cancel()
			} else {
				_ = g.Close()
			}
			gatewayWait(t, c.closed)
			gatewayWait(t, l.closed)
		})
	}
}

func TestRemoteGatewayRechecksAuthorizationBeforeSession(t *testing.T) {
	l := newGatewayTestListener()
	var calls atomic.Int32
	var sessions atomic.Int32
	g := newRemoteGateway(context.Background(), l, func(string) bool { return calls.Add(1) == 1 }, func(context.Context, remoteGatewayConnection) { sessions.Add(1) }, nil)
	defer g.Close()
	c := &gatewayTestConn{xuid: "changed", closed: make(chan struct{})}
	l.accepted <- c
	gatewayWait(t, c.closed)
	if sessions.Load() != 0 {
		t.Fatal("revoked admission reached session")
	}
}
func TestRemoteGatewayVerifiedLoginAdmissionPrecedesPackTransfer(t *testing.T) {
	allowed := false
	calls := 0
	cfg := remoteGatewayListenConfig(nil, func(xuid string) bool { calls++; return allowed && xuid == "123" })
	if cfg.Allow == nil {
		t.Fatal("gateway must reject unauthorized verified identities in the login hook before resource packs")
	}
	if cfg.AuthenticationDisabled {
		t.Fatal("admission requires authenticated Xbox identity")
	}
	for _, xuid := range []string{"", "456", "123"} {
		if _, ok := cfg.Allow(nil, login.IdentityData{XUID: xuid}, login.ClientData{}); ok {
			t.Fatal("unauthorized identity passed pre-pack admission", xuid)
		}
	}
	allowed = true
	if _, ok := cfg.Allow(nil, login.IdentityData{XUID: "123"}, login.ClientData{}); !ok {
		t.Fatal("authorized verified identity denied")
	}
	allowed = false
	if _, ok := cfg.Allow(nil, login.IdentityData{XUID: "123"}, login.ClientData{}); ok {
		t.Fatal("revocation not observed at login")
	}
	if calls == 0 {
		t.Fatal("current ACL was never queried")
	}
	deny := remoteGatewayListenConfig(nil, nil)
	if _, ok := deny.Allow(nil, login.IdentityData{XUID: "123"}, login.ClientData{}); ok {
		t.Fatal("absent ACL callback must default deny")
	}
}
func TestGatewayWatchdogRevokesWithoutMainReconciliation(t *testing.T) {
	for _, cause := range []string{"ACL revoked", "protection evidence expired"} {
		t.Run(cause, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conn := &gatewayTestConn{xuid: "123", closed: make(chan struct{})}
			sessionCtx, sessionCancel := context.WithCancel(ctx)
			defer sessionCancel()
			l := newGatewayTestListener()
			var allowed atomic.Bool
			allowed.Store(true)
			var calls atomic.Int32
			g := &RemoteGateway{ctx: ctx, cancel: cancel, listener: l, authorize: func(string) bool { calls.Add(1); return allowed.Load() }, active: map[*remoteGatewaySession]struct{}{}, watchdogDone: make(chan struct{})}
			g.active[&remoteGatewaySession{conn: conn, cancel: sessionCancel, xuid: "123"}] = struct{}{}
			ticks := make(chan time.Time)
			go g.authorizationWatchdog(ticks)
			allowed.Store(false)
			ticks <- time.Now()
			gatewayWait(t, conn.closed)
			gatewayWait(t, sessionCtx.Done())
			if calls.Load() != 1 {
				t.Fatal("watchdog did not independently query current authorization")
			}
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			gatewayWait(t, g.watchdogDone)
			select {
			case ticks <- time.Now():
				t.Fatal("closed watchdog still consuming ticks")
			default:
			}
		})
	}
}
func TestConstructedGatewayOwnsAndStopsAuthorizationWatchdog(t *testing.T) {
	l := newGatewayTestListener()
	g := newRemoteGateway(context.Background(), l, func(string) bool { return true }, func(context.Context, remoteGatewayConnection) {}, nil)
	if g.watchdogDone == nil {
		t.Fatal("gateway did not start independent authorization watchdog")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	gatewayWait(t, g.watchdogDone)
}
