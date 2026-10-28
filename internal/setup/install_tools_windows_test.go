package setup

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadableToolInstallation(t *testing.T) {
	for _, s := range []string{"success", "failure", "entrypoint", "script-only"} {
		t.Run(s, func(t *testing.T) {
			c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "install-tools-harness.ps1"), "-FixtureRoot", t.TempDir(), "-Scenario", s)
			if b, e := c.CombinedOutput(); e != nil {
				t.Fatalf("%v: %s", e, b)
			}
		})
	}
}

