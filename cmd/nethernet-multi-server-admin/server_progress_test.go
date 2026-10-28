package main

import (
	"nethernet-multi-server/internal/supervisor"
	"strings"
	"testing"
	"time"
)

func TestProgressKeepsStartupBusyUntilEndpointReady(t *testing.T) {
	s := supervisor.Status{State: supervisor.Running, Phase: "running", PID: 42}
	p := serverProgressText("server.restart", s, 65*time.Second, false)
	if !strings.Contains(p, "Checking") || !strings.Contains(p, "1m5s") {
		t.Fatalf("progress=%q", p)
	}
}
func TestProgressShowsRealAddonAndStopStages(t *testing.T) {
	for _, c := range []struct{ phase, want string }{{"applying_changes", "Applying"}, {"launching", "Launching"}, {"stopping", "Saving"}} {
		p := serverProgressText("server.restart", supervisor.Status{Phase: c.phase}, time.Second, false)
		if !strings.Contains(p, c.want) {
			t.Fatalf("phase %s = %q", c.phase, p)
		}
	}
}
func TestLastProgressLogLineHandlesWindowsOutput(t *testing.T) {
	if got := lastProgressLogLine("first\r\nsecond\r\n\r\n"); got != "second" {
		t.Fatal(got)
	}
}
func TestStartIsNotCompleteUntilEndpointReady(t *testing.T) {
	s := supervisor.Status{State: supervisor.Running, PID: 42}
	if serverOperationComplete("server.start", s, false) {
		t.Fatal("process launch reported ready")
	}
	if !serverOperationComplete("server.restart", s, true) {
		t.Fatal("ready server did not complete")
	}
	s.State = supervisor.Stopping
	if serverOperationComplete("server.stop", s, false) {
		t.Fatal("unfinished save reported stopped")
	}
	s.State = supervisor.Stopped
	if !serverOperationComplete("server.stop", s, false) {
		t.Fatal("stopped server did not complete")
	}
}
