package setup

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestorePreviewIncludesDisabledLibraryAndAssignments(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "snapshot.zip")
	file, e := os.Create(archive)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(file)
	payload := map[string]string{"MultiServer/content-staging/state.json": `{"packs":[{"UUID":"a"},{"UUID":"b"}],"enabled":{"creative":{"a":true,"b":false},"survival":{"a":false,"b":true}},"pending":[{"ServerID":"survival","PackUUID":"a","Enable":true}]}`, "Creative/worlds/World/db/CURRENT": "world", "Addons/disabled.mcaddon": "archive"}
	var entries []map[string]any
	for name, data := range payload {
		entry, _ := z.Create(name)
		entry.Write([]byte(data))
		sum := sha256.Sum256([]byte(data))
		entries = append(entries, map[string]any{"path": name, "size": len(data), "sha256": hex.EncodeToString(sum[:])})
	}
	manifest, _ := json.Marshal(map[string]any{"format_version": 1, "source_root": `C:\old`, "files": entries})
	entry, _ := z.Create("manifest.json")
	entry.Write(manifest)
	z.Close()
	file.Close()
	script := filepath.Join("..", "..", "scripts", "setup-assets", "Setup-Wizard.ps1")
	command := `. '` + script + `'; Get-NNRestoreSummary '` + archive + `'`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, want := range []string{"2 library pack(s)", "2 enabled", "2 disabled", "1 pending", "World"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("preview lacks %s: %s", want, out)
		}
	}
}
