package main

import (
	"reflect"
	"testing"

	"nethernet-multi-server/internal/supervisor"
)

func TestPlanStartAllStartsOnlyStoppedServers(t *testing.T) {
	statuses := []supervisor.Status{{ID: "creative", State: supervisor.Stopped}, {ID: "survival", State: supervisor.Running}}
	got, err := planLifecycle("start-all", statuses)
	if err != nil || !reflect.DeepEqual(got, []operation{{"server.start", "creative"}}) {
		t.Fatalf("operations=%+v", got)
	}
}

func TestPlanStopAllStopsManagedServersThenShutsDown(t *testing.T) {
	statuses := []supervisor.Status{{ID: "creative", State: supervisor.Running}, {ID: "survival", State: supervisor.Stopped}}
	want := []operation{{"server.stop", "creative"}, {"system.shutdown", ""}}
	if got, err := planLifecycle("stop-all", statuses); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("operations=%+v", got)
	}
}

func TestPlanStopAllRejectsExternallyRunningServer(t *testing.T) {
	statuses := []supervisor.Status{{ID: "creative", State: supervisor.ExternallyRunning}}
	if _, err := planLifecycle("stop-all", statuses); err == nil {
		t.Fatal("external server was not rejected")
	}
}
