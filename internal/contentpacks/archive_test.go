package contentpacks

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func zipFile(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pack.mcaddon")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

const behaviorManifest = `{"format_version":2,"header":{"name":"Useful Pack","description":"test","uuid":"11111111-1111-1111-1111-111111111111","version":[1,2,3],"min_engine_version":[1,26,0]},"modules":[{"type":"data","uuid":"22222222-2222-2222-2222-222222222222","version":[1,2,3]}]}`

func TestInspectFindsBehaviorPack(t *testing.T) {
	got, err := Inspect(zipFile(t, map[string]string{"Useful/manifest.json": behaviorManifest}), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Packs) != 1 || got.Packs[0].Type != Behavior || got.Packs[0].Name != "Useful Pack" {
		t.Fatalf("import=%+v", got)
	}
}

func TestInspectAcceptsScriptOnlyBehaviorPack(t *testing.T) {
	manifest := strings.Replace(behaviorManifest, `"type":"data"`, `"type":"script"`, 1)
	got, err := Inspect(zipFile(t, map[string]string{"manifest.json": manifest}), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Packs) != 1 || got.Packs[0].Type != Behavior {
		t.Fatalf("import=%+v", got)
	}
}

func TestInspectFindsPackNestedInsideMCAddon(t *testing.T) {
	nested := zipBytes(t, map[string]string{"Pack/manifest.json": behaviorManifest, "Pack/items/example.json": "{}"})
	outer := filepath.Join(t.TempDir(), "nested.mcaddon")
	f, err := os.Create(outer)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("behavior.mcpack")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(nested); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	packs, err := m.ImportArchive(outer)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 {
		t.Fatalf("packs=%+v", packs)
	}
	if _, err = os.Stat(filepath.Join(packs[0].InstalledPath, "items", "example.json")); err != nil {
		t.Fatal(err)
	}
}
func TestInspectRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../server.properties", `packs/..\../server.properties`, `C:\server.properties`, `/server.properties`} {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			_, err := Inspect(zipFile(t, map[string]string{name: "x", "manifest.json": behaviorManifest}), DefaultLimits)
			if err == nil {
				t.Fatalf("accepted %q", name)
			}
		})
	}
}
func TestInspectRejectsLimitsAndMissingManifest(t *testing.T) {
	p := zipFile(t, map[string]string{"file.txt": "12345"})
	if _, err := Inspect(p, Limits{MaxCompressedBytes: 1 << 20, MaxExpandedBytes: 4, MaxFiles: 10, MaxDepth: 2}); err == nil {
		t.Fatal("expanded limit accepted")
	}
	p = zipFile(t, map[string]string{"file.txt": "x"})
	if _, err := Inspect(p, DefaultLimits); err == nil {
		t.Fatal("missing manifest accepted")
	}
}

func TestManagerImportsArchiveIntoPrivateStaging(t *testing.T) {
	root := t.TempDir()
	archive := zipFile(t, map[string]string{"Useful/manifest.json": behaviorManifest, "Useful/items/example.json": "{}"})
	m, err := NewManager(filepath.Join(root, "staging"), nil)
	if err != nil {
		t.Fatal(err)
	}
	packs, err := m.ImportArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 {
		t.Fatalf("packs=%+v", packs)
	}
	if _, err = os.Stat(filepath.Join(packs[0].InstalledPath, "items", "example.json")); err != nil {
		t.Fatal(err)
	}
}
func TestImportHigherVersionUpdatesAndUninstallRemovesInventory(t *testing.T) {
	root := t.TempDir()
	m, err := NewManager(filepath.Join(root, "staging"), nil)
	if err != nil {
		t.Fatal(err)
	}
	first := zipFile(t, map[string]string{"manifest.json": behaviorManifest})
	if _, err = m.ImportArchive(first); err != nil {
		t.Fatal(err)
	}
	higher := strings.Replace(behaviorManifest, `[1,2,3]`, `[2,0,0]`, 1)
	second := zipFile(t, map[string]string{"manifest.json": higher})
	if _, err = m.ImportArchive(second); err != nil {
		t.Fatal(err)
	}
	if got := m.Inventory(); len(got) != 1 || got[0].Version != [3]int{2, 0, 0} {
		t.Fatalf("inventory=%+v", got)
	}
	if err = m.Uninstall("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}
	if len(m.Inventory()) != 0 {
		t.Fatal("pack remained installed")
	}
}

func TestImportSameVersionIsIdempotent(t *testing.T) {
	m, err := NewManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	archive := zipFile(t, map[string]string{"manifest.json": behaviorManifest})
	first, err := m.ImportArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.ImportArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].InstalledPath != second[0].InstalledPath {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if got := m.Inventory(); len(got) != 1 {
		t.Fatalf("inventory=%+v", got)
	}
}

func TestImportRejectsMissingPackDependency(t *testing.T) {
	m, err := NewManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest := strings.TrimSuffix(behaviorManifest, "}") + `,"dependencies":[{"uuid":"99999999-9999-9999-9999-999999999999","version":[1,0,0]}]}`
	if _, err = m.ImportArchive(zipFile(t, map[string]string{"manifest.json": manifest})); err == nil {
		t.Fatal("missing linked pack dependency accepted")
	}
	if len(m.Inventory()) != 0 {
		t.Fatal("failed import changed inventory")
	}
}

func TestInspectAcceptsStringDependencyVersion(t *testing.T) {
	manifest := strings.TrimSuffix(behaviorManifest, "}") + `,"dependencies":[{"module_name":"@minecraft/server","version":"1.17.0"}]}`
	got, err := Inspect(zipFile(t, map[string]string{"manifest.json": manifest}), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Packs) != 1 || len(got.Packs[0].Dependencies) != 1 {
		t.Fatalf("import=%+v", got)
	}
	dependency := got.Packs[0].Dependencies[0]
	if dependency.ModuleName != "@minecraft/server" || dependency.Version != [3]int{1, 17, 0} {
		t.Fatalf("dependency=%+v", dependency)
	}
}

func TestInspectAcceptsStringVersionForPackUUIDDependency(t *testing.T) {
	manifest := strings.TrimSuffix(behaviorManifest, "}") + `,"dependencies":[{"uuid":"99999999-9999-9999-9999-999999999999","version":"1.2.3"}]}`
	got, err := Inspect(zipFile(t, map[string]string{"manifest.json": manifest}), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got.Packs[0].Dependencies[0].Version != [3]int{1, 2, 3} {
		t.Fatalf("dependency=%+v", got.Packs[0].Dependencies[0])
	}
}

func TestInspectAcceptsNamedDependencyChannel(t *testing.T) {
	manifest := strings.TrimSuffix(behaviorManifest, "}") + `,"dependencies":[{"module_name":"@minecraft/server","version":"beta"}]}`
	got, err := Inspect(zipFile(t, map[string]string{"manifest.json": manifest}), DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got.Packs[0].Dependencies[0].Version != [3]int{} {
		t.Fatalf("dependency=%+v", got.Packs[0].Dependencies[0])
	}
}
