package contentpacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalizedArchiveNamesStayInsideEachPack(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(behaviorManifest), &raw); err != nil {
		t.Fatal(err)
	}
	raw["header"].(map[string]any)["name"] = "pack.name"
	b, _ := json.Marshal(raw)
	in, err := Inspect(zipFile(t, map[string]string{"pack/manifest.json": string(b), "pack/texts/en_US.lang": "\ufeff## comment\npack.name=Airplanes 1.2\t## note\n", "texts/en_US.lang": "pack.name=Wrong pack"}), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if in.Packs[0].Name != "Airplanes 1.2" {
		t.Fatalf("name=%q", in.Packs[0].Name)
	}
}

func TestStoredLocalizationDoesNotRewriteManifest(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "texts"), 0700)
	manifest := []byte(`{"unchanged":true}`)
	os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0600)
	os.WriteFile(filepath.Join(root, "texts", "en_GB.lang"), []byte("pack.name=Grappling Hook\npack.description=Swing=fun\n"), 0600)
	p := StoredPack{Pack: Pack{Name: "pack.name", Description: "pack.description", UUID: "identity", Version: [3]int{1, 0, 3}}, InstalledPath: root}
	localizeStoredPack(&p)
	if p.Name != "Grappling Hook" || p.Description != "Swing=fun" || p.UUID != "identity" || p.Version != [3]int{1, 0, 3} {
		t.Fatalf("%+v", p)
	}
	got, _ := os.ReadFile(filepath.Join(root, "manifest.json"))
	if string(got) != string(manifest) {
		t.Fatal("manifest was changed")
	}
	p.Name = "Literal name"
	localizeStoredPack(&p)
	if p.Name != "Literal name" {
		t.Fatal("literal name changed")
	}
}

func TestStoredLibraryReloadResolvesLocalizedName(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "example")
	if err := os.MkdirAll(filepath.Join(dir, "texts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "texts", "en_US.lang"), []byte("pack.name=Airplanes 1.2"), 0600); err != nil {
		t.Fatal(err)
	}
	state := persistentState{Packs: []StoredPack{{Pack: Pack{Name: "pack.name", UUID: "example"}, InstalledPath: dir}}}
	b, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(root, "state.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Inventory()[0].Name; got != "Airplanes 1.2" {
		t.Fatalf("stored name=%q", got)
	}
}

func TestImportExtractsTheInspectedBytesAndDoesNotRetainArchiveMemory(t *testing.T) {
	archive := zipFile(t, map[string]string{"manifest.json": behaviorManifest, "original.txt": "original bytes"})
	in, err := Inspect(archive, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	// Alter the source file after inspection; installation must use the inspected snapshot.
	if err = os.WriteFile(archive, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	stored, err := manager.importInspectedLocked(in.Packs, true)
	manager.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(stored[0].InstalledPath, "original.txt"))
	if err != nil || string(got) != "original bytes" {
		t.Fatal("inspected archive was not used")
	}
	if stored[0].ImportSHA256 == "" || len(stored[0].ArchiveData) != 0 {
		t.Fatal("provenance missing or archive memory retained")
	}
}
