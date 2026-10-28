package main

import (
	"nethernet-multi-server/internal/supervisor"
	"testing"
)

func TestAllServerActionsOnlyIncludeEligibleServers(t *testing.T) {
	rows := []ServerRow{BuildServerRow(supervisor.Status{ID: "creative", Name: "Creative", State: supervisor.Running}), BuildServerRow(supervisor.Status{ID: "survival", Name: "Survival", State: supervisor.Stopped}), BuildServerRow(supervisor.Status{ID: "other", Name: "Unmanaged", State: supervisor.ExternallyRunning})}
	for _, c := range []struct{ method, id string }{{"server.start", "survival"}, {"server.stop", "creative"}, {"server.restart", "creative"}} {
		got, skipped := batchServerRows(rows, c.method)
		if len(got) != 1 || got[0].ID != c.id || len(skipped) != 2 {
			t.Fatalf("%s: %+v skips=%v", c.method, got, skipped)
		}
	}
}
