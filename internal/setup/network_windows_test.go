package setup

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNetworkScriptTransactions(t *testing.T) {
	for _, s := range []string{"create", "reuse", "reuse-adopt-later", "adopt", "dhcp", "dhcp-mixed", "dhcp-mixed-reverse", "dhcp-failure", "dhcp-mixed-failure", "dhcp-unsupported", "conflict", "failure", "write-failure", "uninstall", "primary-protection"} {
		t.Run(s, func(t *testing.T) {
			c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "network-harness.ps1"), "-FixtureRoot", t.TempDir(), "-Scenario", s)
			if b, e := c.CombinedOutput(); e != nil {
				t.Fatalf("%v: %s", e, b)
			}
		})
	}
}

func TestCoexistenceStoreVerificationAndRecovery(t *testing.T) {
	for _, scenario := range []string{"silent-noop", "restore-failure", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "coexistence-harness.ps1"), "-Scenario", scenario)
			if b, err := c.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, b)
			}
		})
	}
}
