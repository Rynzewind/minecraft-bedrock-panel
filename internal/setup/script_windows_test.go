package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationScriptRequiresTheShippedChecksum(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "NetherNetMultiServerSetup.exe")
	if _, e := configurationScript(exe); e == nil {
		t.Fatal("accepted missing configuration script")
	}
	b, e := os.ReadFile(filepath.Join("..", "..", "scripts", "setup-assets", "Configure-Windows.ps1"))
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "setup-assets")
	os.Mkdir(dir, 0700)
	p := filepath.Join(dir, "Configure-Windows.ps1")
	os.WriteFile(p, b, 0600)
	for name := range ScriptHashes {
		if name == "Configure-Windows.ps1" {
			continue
		}
		data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "setup-assets", name))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, name), data, 0600)
	}
	if _, e = configurationScript(exe); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(p, append(b, []byte("\n# modified")...), 0600)
	if _, e = configurationScript(exe); e == nil {
		t.Fatal("accepted modified configuration script")
	}
}
