package main

import (
	"strings"
	"testing"
)

func TestMinecraftRestartChecklistFollowsActualStage(t *testing.T) {
	steps := minecraftProgressSteps("server.restart", "Applying addon/settings changes and creating backups")
	if len(steps) != 4 || steps[0].State != 2 || steps[1].State != 1 || steps[2].State != 0 {
		t.Fatalf("steps=%+v", steps)
	}
	steps = minecraftProgressSteps("server.start", "Checking Minecraft connection readiness")
	if len(steps) != 3 || steps[2].State != 1 {
		t.Fatalf("steps=%+v", steps)
	}
	steps = minecraftProgressSteps("server.stop", "Saving world and waiting for Minecraft to stop")
	if len(steps) != 1 || !strings.Contains(steps[0].Text, "stop") {
		t.Fatalf("steps=%+v", steps)
	}
}
