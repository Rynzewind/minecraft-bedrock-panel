package main

import (
	"nethernet-multi-server/internal/migration"
	"strings"
	"testing"
)

func TestSnapshotCompletionShowsRestartFailure(t *testing.T) {
	text := snapshotSummary(migration.JobStatus{Done: true, Phase: "complete", Validated: true, Path: `D:\Snapshots\one.zip`, RestartError: "creative could not restart"})
	if !strings.Contains(text, "creative could not restart") || !strings.Contains(text, `D:\Snapshots\one.zip`) {
		t.Fatal(text)
	}
}
func TestSnapshotSummaryExplainsCountdownAndCleanupWarning(t *testing.T) {
	text := snapshotSummary(migration.JobStatus{Phase: "warning", WarningSeconds: 240, Warning: "cleanup retained everything"})
	for _, part := range []string{"4 minute", "even with players online", "cleanup retained everything"} {
		if !strings.Contains(text, part) {
			t.Fatal(text)
		}
	}
}
func TestSnapshotFailureDoesNotClaimReady(t *testing.T) {
	text := snapshotSummary(migration.JobStatus{Done: true, Phase: "failed", Error: "disk full"})
	if strings.Contains(text, "ready") || !strings.Contains(text, "disk full") {
		t.Fatal(text)
	}
}

func TestSnapshotSummaryUsesSingleFileInstaller(t *testing.T) {
	text := snapshotSummary(migration.JobStatus{Done: true, Phase: "complete", Validated: true, Path: "snapshot.zip"})
	if !strings.Contains(text, "NetherNetSetup.exe") || strings.Contains(text, "setup-assets") || strings.Contains(text, "complete installer folder") {
		t.Fatalf("outdated migration instructions: %s", text)
	}
}
