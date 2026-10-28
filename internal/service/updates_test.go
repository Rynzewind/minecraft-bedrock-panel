package service

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/serverupdate"
	"testing"
)

type fakeUpdates struct{ busy, recovery bool }

func (f fakeUpdates) Busy() bool                  { return f.busy }
func (f fakeUpdates) Status() serverupdate.Status { return serverupdate.Status{Recovery: f.recovery} }
func (f fakeUpdates) Check(context.Context) (serverupdate.Check, error) {
	return serverupdate.Check{}, nil
}
func (f fakeUpdates) Start(context.Context, string) (serverupdate.Status, error) {
	return serverupdate.Status{}, nil
}
func (f fakeUpdates) Recover(context.Context, bool) (serverupdate.Status, error) {
	return serverupdate.Status{}, nil
}
func TestUpdatesBlockMutationsButPermitReadAndRecoveryStop(t *testing.T) {
	for _, busy := range []bool{false, true} {
		c := &Controller{supervisor: &fakeSupervisor{}, updates: fakeUpdates{busy: busy, recovery: true}}
		if _, e := c.Handle(context.Background(), "server.start", json.RawMessage(`{"server_id":"creative"}`)); e == nil {
			t.Fatal("start allowed")
		}
		if _, e := c.Handle(context.Background(), "updates.status", json.RawMessage(`{}`)); e != nil {
			t.Fatal(e)
		}
		_, e := c.Handle(context.Background(), "server.stop", json.RawMessage(`{"server_id":"creative"}`))
		if busy && e == nil {
			t.Fatal("stop raced active update")
		}
		if !busy && e != nil {
			t.Fatal(e)
		}
	}
}
