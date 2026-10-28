package fleet

import (
	"context"
	"errors"
	"nethernet-multi-server/internal/supervisor"
	"testing"
)

type fakeHost struct {
	on     bool
	events *[]string
}

func (h *fakeHost) Ensure(context.Context) error {
	*h.events = append(*h.events, "ensure")
	h.on = true
	return nil
}
func (h *fakeHost) Running() (bool, error) { return h.on, nil }
func (h *fakeHost) WaitStopped(context.Context) error {
	*h.events = append(*h.events, "wait-stopped")
	h.on = false
	return nil
}

type fakeClient struct {
	host   *fakeHost
	states []supervisor.Status
	events *[]string
}

func (c *fakeClient) Call(_ context.Context, m string, p, t any) error {
	*c.events = append(*c.events, m)
	if !c.host.on {
		return errors.New("offline")
	}
	if m == "servers.list" {
		*t.(*[]supervisor.Status) = append([]supervisor.Status(nil), c.states...)
	}
	if m == "server.start" {
		id := p.(map[string]string)["server_id"]
		for i := range c.states {
			if c.states[i].ID == id {
				c.states[i].State = supervisor.Running
			}
		}
	}
	return nil
}
func fixture(states ...supervisor.Status) (*Engine, *fakeHost, *[]string) {
	ev := []string{}
	h := &fakeHost{events: &ev}
	c := &fakeClient{host: h, states: states, events: &ev}
	return &Engine{Client: c, Host: h, Ready: func(context.Context, supervisor.Status) error { ev = append(ev, "ready"); return nil }}, h, &ev
}
func TestStartBootstrapsHelperAndFailedServer(t *testing.T) {
	e, h, ev := fixture(supervisor.Status{ID: "a", State: supervisor.Failed})
	if err := e.Run(context.Background(), "start-all", nil); err != nil {
		t.Fatal(err)
	}
	if !h.on || len(*ev) < 4 {
		t.Fatal(*ev)
	}
	if (*ev)[0] != "ensure" {
		t.Fatal(*ev)
	}
}
func TestStopOfflineIsIdempotent(t *testing.T) {
	e, _, ev := fixture()
	if err := e.Run(context.Background(), "stop-all", nil); err != nil {
		t.Fatal(err)
	}
	if len(*ev) != 0 {
		t.Fatal(*ev)
	}
}
func TestStopWaitsForHelperExit(t *testing.T) {
	e, h, ev := fixture(supervisor.Status{ID: "a", State: supervisor.Running})
	h.on = true
	if err := e.Run(context.Background(), "stop-all", nil); err != nil {
		t.Fatal(err)
	}
	got := *ev
	if got[len(got)-2] != "system.shutdown" || got[len(got)-1] != "wait-stopped" {
		t.Fatal(got)
	}
}
func TestRestartRestartsWholeStack(t *testing.T) {
	e, h, ev := fixture(supervisor.Status{ID: "a", State: supervisor.Running})
	h.on = true
	if err := e.Run(context.Background(), "restart-all", nil); err != nil {
		t.Fatal(err)
	}
	found := false
	for i, m := range *ev {
		if m == "ensure" && i > 0 && (*ev)[i-1] == "wait-stopped" {
			found = true
		}
	}
	if !found {
		t.Fatal(*ev)
	}
}
func TestExternalServerNotStopped(t *testing.T) {
	e, h, ev := fixture(supervisor.Status{ID: "a", State: supervisor.ExternallyRunning})
	h.on = true
	if e.Run(context.Background(), "stop-all", nil) == nil {
		t.Fatal("external server accepted")
	}
	for _, m := range *ev {
		if m == "system.shutdown" || m == "server.stop" {
			t.Fatal(*ev)
		}
	}
}
func TestCancellationPreventsStart(t *testing.T) {
	e, _, ev := fixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e.Run(ctx, "start-all", nil) == nil {
		t.Fatal("cancel ignored")
	}
	if len(*ev) != 0 {
		t.Fatal(*ev)
	}
}

func TestRecoveryLeavesIntentionallyStoppedServersOff(t *testing.T) {
	e, h, _ := fixture(supervisor.Status{ID: "a", State: supervisor.Stopped}, supervisor.Status{ID: "b", State: supervisor.Failed})
	h.on = true
	if err := e.Run(context.Background(), "recover", nil); err != nil {
		t.Fatal(err)
	}
	c := e.Client.(*fakeClient)
	if c.states[0].State != supervisor.Stopped || c.states[1].State != supervisor.Running {
		t.Fatal(c.states)
	}
}

type orphanHost struct{ fakeHost }

func (h *orphanHost) VerifyStopped() error { return errors.New("orphan server still running") }
func TestStopDoesNotReportSuccessForOrphanServers(t *testing.T) {
	events := []string{}
	host := &orphanHost{fakeHost{events: &events}}
	engine := &Engine{Host: host}
	if err := engine.Run(context.Background(), "stop-all", nil); err == nil {
		t.Fatal("reported stopped with orphan server")
	}
}
