package service

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/supervisor"
	"testing"
	"time"
)

func TestProgressReadableWhileMutationIsBusy(t *testing.T) {
	c := &Controller{supervisor: &fakeSupervisor{}}
	c.operationMu.Lock()
	defer c.operationMu.Unlock()
	done := make(chan error, 1)
	go func() {
		result, err := c.Handle(context.Background(), "server.progress", json.RawMessage(`{"server_id":"creative"}`))
		if err == nil {
			if result.(supervisor.Progress).Log != "ready" {
				t.Error("missing log")
			}
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("progress blocked behind mutation")
	}
}
