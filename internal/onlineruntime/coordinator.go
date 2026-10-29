// Package onlineruntime coordinates optional Online resources without owning
// Minecraft server lifecycle, authentication, firewall policy, or LAN listeners.
package onlineruntime

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

type Desired struct {
	ServerID, AccountID, Host, World                                             string
	Port                                                                         int
	Enabled, AllowUnknownFriendsRoute, Ready, AccountHealthy, ProtectionVerified bool
	// Lifecycle advances synchronously when Servers begins a stop. It prevents
	// an already-built Ready snapshot from clearing that stop barrier.
	Lifecycle uint64
	Revision  string
}
type Event struct{ Code string }
type Worker interface {
	Close() error
	Done() <-chan struct{}
	Err() error
}
type Gateway interface {
	Close() error
	Reconcile()
}
type Factories struct {
	StartGateway func(context.Context, Desired) (Gateway, error)
	StartWorker  func(context.Context, Desired, func(Event)) (Worker, error)
}
type Options struct{ Now func() time.Time }
type State struct {
	ServerID, Status, Code string
	RemoteVerified         bool
	Attempts               int
	NextRetry              time.Time
}
type entry struct {
	stopping   bool
	desired    Desired
	state      State
	worker     Worker
	gateway    Gateway
	cancel     context.CancelFunc
	generation uint64
}
type queuedEvent struct {
	serverID   string
	generation uint64
	event      Event
}
type authHold struct{ sawUnhealthy bool }
type Coordinator struct {
	mu         sync.Mutex
	eventMu    sync.Mutex
	events     []queuedEvent
	factories  Factories
	now        func() time.Time
	entries    map[string]*entry
	holds      map[string]authHold
	barriers   map[string]barrier
	generation uint64
	closed     bool
}
type barrier struct {
	waitingReady bool
	lifecycle    uint64
}

func New(f Factories, opts Options) *Coordinator {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Coordinator{factories: f, now: now, entries: map[string]*entry{}, holds: map[string]authHold{}, barriers: map[string]barrier{}}
}

// Reconcile serializes all starts/stops. It never enables a feature implicitly.
// Caller should invoke it on readiness/configuration/ACL changes and retry ticks.
func (c *Coordinator) Reconcile(ctx context.Context, list []Desired) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.drainEvents()
	seen := map[string]bool{}
	for _, d := range list {
		if d.ServerID == "" || seen[d.ServerID] {
			continue
		}
		seen[d.ServerID] = true
		e := c.entries[d.ServerID]
		if e == nil {
			e = &entry{desired: d, state: State{ServerID: d.ServerID}}
			c.entries[d.ServerID] = e
		}
		if e.stopping {
			select {
			case <-e.worker.Done():
				e.worker = nil
				e.stopping = false
			default:
				continue
			}
		}
		// Readiness/health are observations, not configuration revisions.
		oldConfig, newConfig := e.desired, d
		oldConfig.Ready, oldConfig.AccountHealthy = false, false
		newConfig.Ready, newConfig.AccountHealthy = false, false
		if oldConfig != newConfig {
			if !c.stop(e) {
				continue
			}
			e.state.Attempts = 0
			e.state.NextRetry = time.Time{}
		}
		e.desired = d
		if hold, ok := c.holds[d.AccountID]; ok {
			if !d.AccountHealthy {
				hold.sawUnhealthy = true
				c.holds[d.AccountID] = hold
			} else if hold.sawUnhealthy {
				delete(c.holds, d.AccountID)
				for _, other := range c.entries {
					if other.desired.AccountID == d.AccountID {
						other.state.Attempts = 0
						other.state.NextRetry = time.Time{}
					}
				}
			}
		}
		if edge, ok := c.barriers[d.ServerID]; ok {
			if !d.Ready {
				edge.waitingReady = true
				c.barriers[d.ServerID] = edge
			} else if edge.waitingReady && d.Lifecycle >= edge.lifecycle {
				delete(c.barriers, d.ServerID)
			}
		}
		status, code := c.blocked(d)
		if status != "" {
			if !c.stop(e) {
				continue
			}
			e.state.Status, e.state.Code = status, code
			continue
		}
		// An already finished child is handled synchronously as well as by its monitor.
		if e.worker != nil {
			select {
			case <-e.worker.Done():
				c.failure(e, "worker_stopped")
			default:
			}
		}
		if e.gateway != nil {
			e.gateway.Reconcile()
		}
		if e.worker != nil {
			continue
		}
		if e.state.Attempts > 6 {
			e.state.Status, e.state.Code = "failed", "retry_limit"
			continue
		}
		if !e.state.NextRetry.IsZero() && c.now().Before(e.state.NextRetry) {
			e.state.Status = "retrying"
			continue
		}
		if ctx.Err() != nil {
			e.state.Status, e.state.Code = "blocked", "operation_cancelled"
			continue
		}
		c.start(ctx, e)
		c.drainEvents()
	}
	for id, e := range c.entries {
		if !seen[id] {
			if c.stop(e) {
				delete(c.entries, id)
			}
		}
	}
}

func (c *Coordinator) blocked(d Desired) (string, string) {
	if !d.Enabled {
		return "disabled", "disabled"
	}
	if !d.Ready {
		return "blocked", "server_not_ready"
	}
	if _, ok := c.barriers[d.ServerID]; ok {
		return "blocked", "waiting_for_readiness_edge"
	}
	if !d.AccountHealthy {
		return "authentication required", "auth_required"
	}
	if _, ok := c.holds[d.AccountID]; ok {
		return "authentication required", "auth_required"
	}
	if d.AccountID == "" || d.Host == "" || d.World == "" || d.Port < 1 || d.Port > 65535 {
		return "blocked", "configuration_required"
	}
	if !d.AllowUnknownFriendsRoute {
		return "blocked", "friends_route_not_accepted"
	}
	if !d.ProtectionVerified {
		return "blocked", "protection_required"
	}
	if c.factories.StartGateway == nil || c.factories.StartWorker == nil {
		return "blocked", "runtime_unavailable"
	}
	return "", ""
}

func (c *Coordinator) start(ctx context.Context, e *entry) {
	c.generation++
	generation := c.generation
	e.generation = generation
	e.state.Status, e.state.Code = "starting", "starting"
	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	gateway, err := c.factories.StartGateway(runCtx, e.desired)
	if err != nil || gateway == nil {
		if gateway != nil {
			_ = gateway.Close()
		}
		c.failure(e, "gateway_start_failed")
		return
	}
	e.gateway = gateway
	id := e.desired.ServerID
	worker, err := c.factories.StartWorker(runCtx, e.desired, func(event Event) { c.enqueue(queuedEvent{serverID: id, generation: generation, event: event}) })
	e.worker = worker
	// A factory can synchronously report authentication failure and return an
	// error. Consume that event before failure cleanup invalidates its generation.
	c.drainEvents()
	if e.generation != generation {
		return
	}
	if err != nil || worker == nil {
		c.failure(e, "worker_start_failed")
		return
	}
	e.state.NextRetry = time.Time{}
	// Advertising is asserted only by a worker event, not successful process launch.
	go func() {
		select {
		case <-worker.Done():
			c.mu.Lock()
			defer c.mu.Unlock()
			current := c.entries[id]
			if !c.closed && current == e && e.generation == generation && e.worker == worker {
				c.drainEvents()
				if e.worker == worker {
					c.failure(e, "worker_stopped")
				}
			}
		case <-runCtx.Done():
		}
	}()
}

func (c *Coordinator) enqueue(event queuedEvent) {
	// Callback may run synchronously inside StartWorker/Close, while mu is held.
	c.eventMu.Lock()
	c.events = append(c.events, event)
	c.eventMu.Unlock()
	go func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.closed {
			c.drainEvents()
		}
	}()
}
func (c *Coordinator) drainEvents() {
	c.eventMu.Lock()
	events := c.events
	c.events = nil
	c.eventMu.Unlock()
	for _, queued := range events {
		e := c.entries[queued.serverID]
		if e == nil || e.generation != queued.generation {
			continue
		}
		switch queued.event.Code {
		case "auth_required":
			c.holds[e.desired.AccountID] = authHold{}
			for _, other := range c.entries {
				if other.desired.AccountID == e.desired.AccountID {
					if !c.stop(other) {
						continue
					}
					other.state.Status, other.state.Code = "authentication required", "auth_required"
				}
			}
		case "advertising":
			if e.worker != nil {
				e.state.Status, e.state.Code = "advertising", "advertising"
			}
		case "starting":
			if e.worker != nil {
				e.state.Status, e.state.Code = "starting", "starting"
			}
		}
	}
}

func (c *Coordinator) failure(e *entry, code string) {
	if !c.stop(e) {
		return
	}
	if _, held := c.holds[e.desired.AccountID]; held {
		e.state.Status, e.state.Code = "authentication required", "auth_required"
		return
	}
	e.state.Attempts++
	if e.state.Attempts > 6 {
		e.state.Status, e.state.Code = "failed", "retry_limit"
		e.state.NextRetry = time.Time{}
		return
	}
	delay := 5 * time.Second << uint(e.state.Attempts-1)
	if delay > 2*time.Minute {
		delay = 2 * time.Minute
	}
	e.state.Status, e.state.Code = "retrying", code
	e.state.NextRetry = c.now().Add(delay)
}
func (c *Coordinator) stop(e *entry) bool {
	// Clear ownership and advance generation before Close can deliver callbacks.
	worker, gateway, cancel := e.worker, e.gateway, e.cancel
	e.worker, e.gateway, e.cancel = nil, nil, nil
	e.generation = 0
	if cancel != nil {
		cancel()
	}
	if gateway != nil {
		_ = gateway.Close()
	}
	if worker != nil {
		_ = worker.Close()
	}
	if worker != nil {
		select {
		case <-worker.Done():
		default:
			e.worker = worker
			e.stopping = true
			e.state.Status, e.state.Code = "failed", "worker_stop_unconfirmed"
			return false
		}
	}
	e.stopping = false
	return true
}

// StopServer must precede stopping its BDS process. Stale Ready=true snapshots
// cannot restart Online until a subsequent false -> true readiness edge.
func (c *Coordinator) StopServer(id string) {
	c.stopServer(id, false)
}

// BackendStopping is for the supervisor callback after it has synchronously
// entered Stopping. That observed not-ready edge must not depend on polling;
// a fast restart may be ready again before the next reconciliation tick.
func (c *Coordinator) BackendStopping(id string, lifecycle uint64) {
	c.stopServer(id, true, lifecycle)
}

func (c *Coordinator) stopServer(id string, observedNotReady bool, lifecycle ...uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	edge := barrier{waitingReady: observedNotReady}
	if len(lifecycle) > 0 {
		edge.lifecycle = lifecycle[0]
	}
	c.barriers[id] = edge
	if e := c.entries[id]; e != nil {
		if !c.stop(e) {
			return
		}
		e.state.Status, e.state.Code = "blocked", "waiting_for_readiness_edge"
	}
}
func (c *Coordinator) ResetAuthentication(accountID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.holds, accountID)
	for _, e := range c.entries {
		if e.desired.AccountID == accountID {
			e.state.Attempts = 0
			e.state.NextRetry = time.Time{}
		}
	}
}
func (c *Coordinator) Snapshot() []State {
	c.mu.Lock()
	defer c.mu.Unlock()
	states := make([]State, 0, len(c.entries))
	for _, e := range c.entries {
		states = append(states, e.state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].ServerID < states[j].ServerID })
	return states
}
func (c *Coordinator) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var result error
	for _, e := range c.entries {
		if c.stop(e) {
			e.state.Status, e.state.Code = "disabled", "shutdown"
		} else {
			result = errors.New("Online worker stop could not be confirmed")
		}
	}
	return result
}
