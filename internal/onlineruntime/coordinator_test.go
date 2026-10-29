package onlineruntime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type testGateway struct {
	closed, reconciled int
	stopped            chan struct{}
}

func (g *testGateway) Close() error {
	g.closed++
	if g.stopped != nil && g.closed == 1 {
		close(g.stopped)
	}
	return nil
}
func (g *testGateway) Reconcile() { g.reconciled++ }

type testWorker struct {
	done      chan struct{}
	once      sync.Once
	closed    int
	failClose bool
}

func (w *testWorker) Close() error {
	w.closed++
	if w.failClose {
		return errors.New("private child stop failure")
	}
	w.once.Do(func() { close(w.done) })
	return nil
}
func (w *testWorker) Done() <-chan struct{} { return w.done }
func (w *testWorker) Err() error            { return errors.New("secret token should never enter state") }
func desired() Desired {
	return Desired{ServerID: "creative", AccountID: "broadcaster", Host: "192.0.2.1", World: "Creative", Port: 19132, Enabled: true, AllowUnknownFriendsRoute: true, Ready: true, AccountHealthy: true, ProtectionVerified: true, Revision: "1"}
}

func TestFailedWorkerStopRetainsOwnershipUntilConfirmedExit(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	d := desired()
	ctx := context.Background()
	f.c.Reconcile(ctx, []Desired{d})
	f.workers[0].failClose = true
	d.Revision = "replacement"
	f.c.Reconcile(ctx, []Desired{d})
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 1 || f.gateways[0].closed != 1 || f.c.Snapshot()[0].Code != "worker_stop_unconfirmed" {
		t.Fatal("replacement started while old child survived")
	}
	f.workers[0].once.Do(func() { close(f.workers[0].done) })
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 2 {
		t.Fatal("confirmed child exit did not release ownership")
	}
}

func TestObservedBackendStoppingAllowsFastRestartWithoutIntermediatePoll(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	d := desired()
	ctx := context.Background()
	f.c.Reconcile(ctx, []Desired{d})
	// The supervisor already transitioned to Stopping before this callback.
	// A complete restart may occur between the coordinator's polling ticks.
	f.c.BackendStopping(d.ServerID, 1)
	d.Lifecycle = 1
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 2 || f.workers[0].closed != 1 {
		t.Fatal("observed stopping edge lost between polls")
	}
}

func TestObservedBackendStoppingRejectsStaleReadySnapshot(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	d := desired()
	ctx := context.Background()
	f.c.Reconcile(ctx, []Desired{d})
	f.c.BackendStopping(d.ServerID, 2)
	// This snapshot was built before the supervisor synchronously entered Stopping.
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 1 {
		t.Fatal("stale ready snapshot restarted Online")
	}
	d.Lifecycle = 2
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 2 {
		t.Fatal("new lifecycle readiness did not restart Online")
	}
}

type fixture struct {
	c        *Coordinator
	gateways []*testGateway
	workers  []*testWorker
	events   []func(Event)
	now      time.Time
	fail     bool
	syncAuth bool
}

func makeFixture() *fixture {
	f := &fixture{now: time.Unix(100, 0)}
	f.c = New(Factories{StartGateway: func(context.Context, Desired) (Gateway, error) {
		g := &testGateway{stopped: make(chan struct{})}
		f.gateways = append(f.gateways, g)
		return g, nil
	}, StartWorker: func(_ context.Context, _ Desired, event func(Event)) (Worker, error) {
		f.events = append(f.events, event)
		if f.syncAuth {
			event(Event{Code: "auth_required"})
		}
		if f.fail {
			return nil, errors.New("private-secret")
		}
		w := &testWorker{done: make(chan struct{})}
		f.workers = append(f.workers, w)
		return w, nil
	}}, Options{Now: func() time.Time { return f.now }})
	return f
}
func TestDefaultDenyAndNoDuplicateStart(t *testing.T) {
	for _, mutate := range []func(*Desired){func(d *Desired) { d.Enabled = false }, func(d *Desired) { d.Ready = false }, func(d *Desired) { d.AccountHealthy = false }, func(d *Desired) { d.ProtectionVerified = false }, func(d *Desired) { d.AllowUnknownFriendsRoute = false }, func(d *Desired) { d.AccountID = "" }, func(d *Desired) { d.Host = "" }, func(d *Desired) { d.Port = 0 }} {
		f := makeFixture()
		d := desired()
		mutate(&d)
		f.c.Reconcile(context.Background(), []Desired{d})
		if len(f.gateways) != 0 {
			t.Fatal("unsafe configuration started")
		}
		_ = f.c.Close()
	}
	f := makeFixture()
	defer f.c.Close()
	d := desired()
	for i := 0; i < 3; i++ {
		f.c.Reconcile(context.Background(), []Desired{d})
	}
	if len(f.workers) != 1 {
		t.Fatal("duplicate worker")
	}
	if f.gateways[0].reconciled < 2 {
		t.Fatal("ACL reconciliation omitted")
	}
	if f.c.Snapshot()[0].RemoteVerified {
		t.Fatal("external verification invented")
	}
}
func TestReadinessConfigurationAndStopBarrier(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	d := desired()
	ctx := context.Background()
	f.c.Reconcile(ctx, []Desired{d})
	f.c.StopServer(d.ServerID)
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 1 || f.workers[0].closed != 1 || f.gateways[0].closed != 1 {
		t.Fatal("stale readiness restarted server")
	}
	d.Ready = false
	f.c.Reconcile(ctx, []Desired{d})
	d.Ready = true
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 2 {
		t.Fatal("new readiness edge did not restart")
	}
	d.Revision = "2"
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 3 || f.workers[1].closed != 1 || f.gateways[1].closed != 1 {
		t.Fatal("configuration change not replaced")
	}
	d.Ready = false
	f.c.Reconcile(ctx, []Desired{d})
	if f.workers[2].closed != 1 {
		t.Fatal("readiness loss did not stop")
	}
}
func TestWorkerFailureBackoffIsBoundedAndClosesGateway(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	f.fail = true
	d := desired()
	ctx := context.Background()
	f.c.Reconcile(ctx, []Desired{d})
	if f.gateways[0].closed != 1 || f.c.Snapshot()[0].Status != "retrying" {
		t.Fatal("failed start leaked gateway")
	}
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.gateways) != 1 {
		t.Fatal("backoff ignored")
	}
	f.now = f.now.Add(5 * time.Second)
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.gateways) != 2 {
		t.Fatal("first retry not scheduled at five seconds")
	}
	for i := 0; i < 10; i++ {
		f.now = f.now.Add(3 * time.Minute)
		f.c.Reconcile(ctx, []Desired{d})
	}
	if len(f.gateways) != 7 || f.c.Snapshot()[0].Status != "failed" {
		t.Fatalf("retries not bounded: starts=%d state=%+v", len(f.gateways), f.c.Snapshot())
	}
}
func TestWorkerExitAndSynchronousAuthenticationHold(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	ctx := context.Background()
	d := desired()
	f.c.Reconcile(ctx, []Desired{d})
	f.workers[0].once.Do(func() { close(f.workers[0].done) })
	f.c.Reconcile(ctx, []Desired{d})
	if f.gateways[0].closed != 1 || f.c.Snapshot()[0].Status != "retrying" {
		t.Fatal("worker exit did not stop gateway")
	}
	f = makeFixture()
	defer f.c.Close()
	f.syncAuth = true
	f.c.Reconcile(ctx, []Desired{d})
	f.c.Reconcile(ctx, []Desired{d})
	if f.c.Snapshot()[0].Status != "authentication required" || len(f.workers) != 1 || f.gateways[0].closed != 1 {
		t.Fatal("synchronous auth event not latched")
	}
	f.syncAuth = false
	f.now = f.now.Add(time.Hour)
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 1 {
		t.Fatal("auth hold retried")
	}
	d.AccountHealthy = false
	f.c.Reconcile(ctx, []Desired{d})
	d.AccountHealthy = true
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 2 {
		t.Fatal("reauthentication edge did not resume")
	}
}
func TestExplicitAuthenticationResetAndShutdown(t *testing.T) {
	f := makeFixture()
	d := desired()
	ctx := context.Background()
	f.c.Reconcile(ctx, []Desired{d})
	f.events[0](Event{Code: "auth_required"})
	f.c.Reconcile(ctx, []Desired{d})
	f.c.ResetAuthentication(d.AccountID)
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 2 {
		t.Fatal("explicit reauth reset failed")
	}
	_ = f.c.Close()
	f.c.Reconcile(ctx, []Desired{d})
	if len(f.workers) != 2 || f.workers[1].closed != 1 || f.gateways[1].closed != 1 {
		t.Fatal("shutdown failed")
	}
}

func TestAuthenticationEventOnFailedStartDoesNotRetry(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	f.fail = true
	f.syncAuth = true
	f.c.Reconcile(context.Background(), []Desired{desired()})
	if state := f.c.Snapshot()[0]; state.Status != "authentication required" {
		t.Fatalf("authentication event lost on failed start: %+v", state)
	}
	f.now = f.now.Add(time.Hour)
	f.c.Reconcile(context.Background(), []Desired{desired()})
	if len(f.gateways) != 1 {
		t.Fatal("failed authenticated start retried")
	}
}

func TestConcurrentReconcileDoesNotDuplicateRuntime(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.c.Reconcile(context.Background(), []Desired{desired()}); _ = f.c.Snapshot() }()
	}
	wg.Wait()
	if len(f.gateways) != 1 || len(f.workers) != 1 {
		t.Fatal("concurrent reconcile duplicated runtime")
	}
}

func TestSynchronousAdvertisingEventIsPreserved(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	start := f.c.factories.StartWorker
	f.c.factories.StartWorker = func(ctx context.Context, d Desired, event func(Event)) (Worker, error) {
		w, err := start(ctx, d, event)
		event(Event{Code: "advertising"})
		return w, err
	}
	f.c.Reconcile(context.Background(), []Desired{desired()})
	if f.c.Snapshot()[0].Status != "advertising" {
		t.Fatal("synchronous advertising event lost")
	}
}

func TestChildExitClosesGatewayWithoutAnotherReconcile(t *testing.T) {
	f := makeFixture()
	defer f.c.Close()
	f.c.Reconcile(context.Background(), []Desired{desired()})
	f.workers[0].once.Do(func() { close(f.workers[0].done) })
	select {
	case <-f.gateways[0].stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("child exit left gateway open")
	}
	if f.c.Snapshot()[0].Status != "retrying" {
		t.Fatal("child exit not recorded")
	}
}
