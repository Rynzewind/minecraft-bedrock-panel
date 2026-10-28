package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// CompanionHashes is supplied by the release build after its companion binaries are built.
var CompanionHashes string

func InstallTools(root string) error {
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	for name := range ScriptHashes {
		if _, e = VerifiedScript(executable, name); e != nil {
			return e
		}
	}
	if CompanionHashes == "" {
		return fmt.Errorf("installer release is missing companion executable checksums")
	}
	source := filepath.Dir(executable)
	companions := source
	if strings.EqualFold(source, filepath.Join(root, "MultiServer", "installer")) {
		companions = filepath.Join(root, "MultiServer")
	}
	script, e := VerifiedScript(executable, "Install-Tools.ps1")
	if e != nil {
		return e
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Root", root, "-SourceDirectory", source, "-CompanionDirectory", companions, "-SetupPath", executable, "-CompanionHashes", CompanionHashes)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, e := command.CombinedOutput()
	if e != nil {
		return fmt.Errorf("software installation needs attention: %s", output)
	}
	return nil
}
