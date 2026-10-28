package setup

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDecommissionOwnership(t *testing.T) {
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "decommission-harness.ps1"), "-FixtureRoot", t.TempDir())
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
}
func TestDecommissionRetainAndRemoveScope(t *testing.T) {
	for _, s := range []string{"retain", "remove"} {
		t.Run(s, func(t *testing.T) {
			c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "decommission-transaction-harness.ps1"), "-FixtureRoot", t.TempDir(), "-Scenario", s)
			if b, e := c.CombinedOutput(); e != nil {
				t.Fatalf("%v: %s", e, b)
			}
		})
	}
}
func TestWatchdogHonoursCleanShutdown(t *testing.T) {
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "runtime-harness.ps1"))
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
}
func TestLegacyAliasDiscoveryNeedsExplicitOwnership(t *testing.T) {
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "legacy-alias-harness.ps1"))
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
}
