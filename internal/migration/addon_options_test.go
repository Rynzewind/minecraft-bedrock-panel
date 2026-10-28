package migration

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/accountstore"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptionalAddonLibraryPreservesWorldPacks(t *testing.T) {
	root := fixture(t)
	put(t, root, "Addons/disabled.mcaddon", "download")
	put(t, root, "MultiServer/content-staging/packs/disabled/manifest.json", "disabled payload")
	put(t, root, "MultiServer/content-staging/state.json", `{"packs":[],"enabled":{"creative":{"p":true},"survival":{"p":false}}}`)
	put(t, root, "Creative/behavior_packs/active/manifest.json", "active world pack")
	for _, include := range []bool{true, false} {
		files, e := BuildInventoryWithOptions(root, t.TempDir(), SnapshotOptions{IncludeAddons: include})
		if e != nil {
			t.Fatal(e)
		}
		seen := map[string]bool{}
		for _, f := range files {
			seen[f.Path] = true
		}
		if !seen["Creative/behavior_packs/active/manifest.json"] {
			t.Fatal("removed active world addon")
		}
		if seen["Addons/disabled.mcaddon"] != include || seen["MultiServer/content-staging/state.json"] != include || seen["MultiServer/content-staging/packs/disabled/manifest.json"] != include {
			t.Fatalf("wrong addon-library selection: %v", seen)
		}
	}
}
func TestFullAddonRestorePreservesDisabledPacksAndServerSelections(t *testing.T) {
	root := fixture(t)
	state := `{"packs":[{"UUID":"enabled","installed_path":` + quote(filepath.Join(root, "MultiServer", "content-staging", "packs", "enabled")) + `},{"UUID":"disabled","installed_path":` + quote(filepath.Join(root, "MultiServer", "content-staging", "packs", "disabled")) + `}],"enabled":{"creative":{"enabled":true,"disabled":false},"survival":{"enabled":false,"disabled":false}},"pending":[{"ServerID":"survival","PackUUID":"disabled","Enable":true}]}`
	put(t, root, "MultiServer/content-staging/state.json", state)
	put(t, root, "MultiServer/content-staging/packs/enabled/manifest.json", "enabled-payload")
	put(t, root, "MultiServer/content-staging/packs/disabled/manifest.json", "disabled-payload")
	put(t, root, "Addons/disabled.mcaddon", "original archive")
	accounts, e := ExportAccounts(accountstore.Snapshot{Version: 1})
	if e != nil {
		t.Fatal(e)
	}
	put(t, root, "accounts-migration.json", string(accounts))
	files, e := BuildInventoryWithOptions(root, t.TempDir(), SnapshotOptions{IncludeAddons: true})
	if e != nil {
		t.Fatal(e)
	}
	entry, e := fileEntry(root, filepath.Join(root, "accounts-migration.json"))
	if e != nil {
		t.Fatal(e)
	}
	files = append(files, entry)
	archive := filepath.Join(t.TempDir(), "all-addons.zip")
	if e = WriteArchive(context.Background(), root, archive, Manifest{FormatVersion: 1, SourceRoot: root, Files: files}); e != nil {
		t.Fatal(e)
	}
	dst := filepath.Join(t.TempDir(), "restored")
	if _, e = Restore(context.Background(), RestoreOptions{ArchivePath: archive, Destination: dst, LANAddress: "10.42.8.9", BroadcastAddress: "10.42.8.255"}); e != nil {
		t.Fatal(e)
	}
	restored, e := os.ReadFile(filepath.Join(dst, "MultiServer/content-staging/state.json"))
	if e != nil {
		t.Fatal(e)
	}
	var original, after map[string]json.RawMessage
	json.Unmarshal([]byte(state), &original)
	json.Unmarshal(restored, &after)
	for _, key := range []string{"enabled", "pending"} {
		var want, got any
		json.Unmarshal(original[key], &want)
		json.Unmarshal(after[key], &got)
		a, _ := json.Marshal(want)
		b, _ := json.Marshal(got)
		if string(a) != string(b) {
			t.Fatalf("%s changed", key)
		}
	}
	for _, path := range []string{"MultiServer/content-staging/packs/disabled/manifest.json", "Addons/disabled.mcaddon"} {
		if _, e = os.Stat(filepath.Join(dst, filepath.FromSlash(path))); e != nil {
			t.Fatal(e)
		}
	}
	if strings.Contains(string(restored), strings.ReplaceAll(root, `\`, `\\`)) {
		t.Fatal("old addon paths survived restore")
	}
}
