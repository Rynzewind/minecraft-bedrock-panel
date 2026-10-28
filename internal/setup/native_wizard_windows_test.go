package setup

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNativeSetupWizardCreatesIPControls(t *testing.T) {
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "setup-assets", "Setup-Wizard.ps1"), "-SmokeTest")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
}
func TestNativeWizardPassesDistinctIPsToArchive(t *testing.T) {
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "native-wizard-harness.ps1"), "-FixtureRoot", t.TempDir())
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
}
func TestNativeWizardDoesNotConnectToUnrelatedHelper(t *testing.T) {
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join("..", "..", "scripts", "tests", "native-ownership-harness.ps1"), "-FixtureRoot", t.TempDir())
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, b)
	}
}
