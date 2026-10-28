package setup

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestUpdaterInstallsNativeWizardMetadataWithoutChangingWorlds(t *testing.T) {
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "update-panel-harness.ps1"), "-FixtureRoot", t.TempDir())
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
}
