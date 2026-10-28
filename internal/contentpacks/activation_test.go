package contentpacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nethernet-multi-server/internal/config"
)

func TestActivationWritesCorrectWorldFiles(t *testing.T) {
	root := t.TempDir()
	world := filepath.Join(root, "worlds", "World")
	if err := os.MkdirAll(world, 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(filepath.Join(root, "staging"), []config.Server{{ID: "creative", Name: "Creative", Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}}})
	if err != nil {
		t.Fatal(err)
	}
	m.state.Packs = []StoredPack{{Pack: Pack{Name: "B", UUID: "11111111-1111-1111-1111-111111111111", Version: [3]int{1, 2, 3}, Type: Behavior}, InstalledPath: filepath.Join(root, "staging", "packs", "b")}}
	if err := os.MkdirAll(m.state.Packs[0].InstalledPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.state.Packs[0].InstalledPath, "manifest.json"), []byte(behaviorManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Stage("creative", m.state.Packs[0].UUID, true); err != nil {
		t.Fatal(err)
	}
	if err := m.Apply("creative"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(world, "world_behavior_packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got []activation
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PackID != m.state.Packs[0].UUID || got[0].Version != [3]int{1, 2, 3} {
		t.Fatalf("activation=%+v", got)
	}
	if _, err = os.Stat(filepath.Join(root, "behavior_packs", m.state.Packs[0].UUID, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestPendingStateSurvivesReload(t *testing.T) {
	root := t.TempDir()
	cfg := []config.Server{{ID: "creative", Name: "Creative", Process: &config.Process{WorkingDirectory: root, WorldDirectory: filepath.Join(root, "world")}}}
	m, err := NewManager(filepath.Join(root, "staging"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.state.Packs = []StoredPack{{Pack: Pack{UUID: "u", Type: Resource}}}
	if err = m.Stage("creative", "u", true); err != nil {
		t.Fatal(err)
	}
	m2, err := NewManager(filepath.Join(root, "staging"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(m2.Pending("creative")) != 1 {
		t.Fatalf("pending=%+v", m2.Pending("creative"))
	}
}
func TestApplyWithNoPendingChangesDoesNotTouchWorld(t *testing.T) {
	root := t.TempDir()
	world := filepath.Join(root, "world")
	m, err := NewManager(filepath.Join(root, "staging"), []config.Server{{ID: "creative", Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Apply("creative"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(world); !os.IsNotExist(err) {
		t.Fatalf("world touched: %v", err)
	}
}
func TestApplyPreservesUnknownActivationEntries(t *testing.T) {
	root := t.TempDir()
	world := filepath.Join(root, "world")
	if err := os.MkdirAll(world, 0o755); err != nil {
		t.Fatal(err)
	}
	unknown := `[{"pack_id":"external-pack","version":[9,9,9]}]`
	if err := os.WriteFile(filepath.Join(world, "world_behavior_packs.json"), []byte(unknown), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(filepath.Join(root, "staging"), []config.Server{{ID: "creative", Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}}})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "staged")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(behaviorManifest), 0o600)
	m.state.Packs = []StoredPack{{Pack: Pack{UUID: "managed", Version: [3]int{1, 0, 0}, Type: Behavior}, InstalledPath: dir}}
	if err = m.Stage("creative", "managed", true); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply("creative"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(world, "world_behavior_packs.json"))
	if !strings.Contains(string(b), "external-pack") || !strings.Contains(string(b), "managed") {
		t.Fatalf("activation=%s", b)
	}
}
func TestResourcePolicyUpdatesOnlyTexturepackRequired(t *testing.T) {
	root := t.TempDir()
	world := filepath.Join(root, "world")
	_ = os.MkdirAll(world, 0o755)
	properties := "# keep\nserver-name=Mine\ntexturepack-required=false\n"
	if err := os.WriteFile(filepath.Join(root, "server.properties"), []byte(properties), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(filepath.Join(root, "staging"), []config.Server{{ID: "creative", Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.StageResourcePolicy("creative", true); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply("creative"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "server.properties"))
	got := string(b)
	if !strings.Contains(got, "# keep") || !strings.Contains(got, "server-name=Mine") || !strings.Contains(got, "texturepack-required=true") {
		t.Fatalf("properties=%q", got)
	}
}

func TestApplyFailureRestoresActivationAndKeepsPending(t *testing.T) {
	root := t.TempDir()
	world := filepath.Join(root, "world")
	_ = os.MkdirAll(world, 0o755)
	original := `[{"pack_id":"external","version":[1,0,0]}]`
	activationPath := filepath.Join(world, "world_behavior_packs.json")
	_ = os.WriteFile(activationPath, []byte(original), 0o600)
	_ = os.Mkdir(filepath.Join(root, "server.properties"), 0o755)
	m, err := NewManager(filepath.Join(root, "staging"), []config.Server{{ID: "creative", Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}}})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "staged")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(behaviorManifest), 0o600)
	m.state.Packs = []StoredPack{{Pack: Pack{UUID: "managed", Version: [3]int{1, 0, 0}, Type: Behavior}, InstalledPath: dir}}
	if err = m.Stage("creative", "managed", true); err != nil {
		t.Fatal(err)
	}
	if err = m.StageResourcePolicy("creative", true); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply("creative"); err == nil {
		t.Fatal("Apply succeeded; want property failure")
	}
	b, _ := os.ReadFile(activationPath)
	if string(b) != original {
		t.Fatalf("activation not restored: %s", b)
	}
	if len(m.Pending("creative")) < 2 {
		t.Fatalf("pending was lost: %+v", m.Pending("creative"))
	}
}

func TestCancelPendingLeavesAppliedStateUnchanged(t *testing.T) {
	m, err := NewManager(t.TempDir(), []config.Server{{ID: "creative"}})
	if err != nil {
		t.Fatal(err)
	}
	m.state.Packs = []StoredPack{{Pack: Pack{UUID: "pack"}}}
	if err = m.Stage("creative", "pack", true); err != nil {
		t.Fatal(err)
	}
	if err = m.StageResourcePolicy("creative", true); err != nil {
		t.Fatal(err)
	}
	if err = m.CancelPending("creative"); err != nil {
		t.Fatal(err)
	}
	if got := m.Pending("creative"); len(got) != 0 {
		t.Fatalf("pending=%+v", got)
	}
	if m.state.Enabled["creative"]["pack"] {
		t.Fatal("cancel changed applied state")
	}
}

func TestInventoryStateShowsAppliedAndPendingAssignments(t *testing.T) {
	m, err := NewManager(t.TempDir(), []config.Server{{ID: "creative"}, {ID: "survival"}})
	if err != nil {
		t.Fatal(err)
	}
	m.state.Packs = []StoredPack{{Pack: Pack{UUID: "pack"}}}
	m.state.Enabled["creative"] = map[string]bool{"pack": true}
	if err = m.Stage("survival", "pack", true); err != nil {
		t.Fatal(err)
	}
	items := m.InventoryState()
	if len(items) != 1 || !items[0].Servers["creative"].Enabled || items[0].Servers["survival"].Pending == nil || !*items[0].Servers["survival"].Pending {
		t.Fatalf("items=%+v", items)
	}
}

func TestStageChangesLinkedBehaviorAndResourcePacksTogether(t *testing.T) {
	m, err := NewManager(t.TempDir(), []config.Server{{ID: "creative"}})
	if err != nil {
		t.Fatal(err)
	}
	m.state.Packs = []StoredPack{
		{Pack: Pack{UUID: "behavior", Type: Behavior, Dependencies: []Dependency{{UUID: "resource"}}}},
		{Pack: Pack{UUID: "resource", Type: Resource}},
	}
	if err = m.Stage("creative", "behavior", true); err != nil {
		t.Fatal(err)
	}
	got := m.Pending("creative")
	if len(got) != 2 {
		t.Fatalf("pending=%+v", got)
	}
}

func TestApplyDisableRemovesManagedPackDirectory(t *testing.T) {
	root := t.TempDir()
	world := filepath.Join(root, "world")
	packDir := filepath.Join(root, "behavior_packs", "managed")
	_ = os.MkdirAll(packDir, 0o755)
	_ = os.WriteFile(filepath.Join(packDir, "old.txt"), []byte("old"), 0o600)
	_ = os.MkdirAll(world, 0o755)
	m, err := NewManager(filepath.Join(root, "staging"), []config.Server{{ID: "creative", Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}}})
	if err != nil {
		t.Fatal(err)
	}
	m.state.Packs = []StoredPack{{Pack: Pack{UUID: "managed", Type: Behavior}}}
	m.state.Enabled["creative"] = map[string]bool{"managed": true}
	if err = m.Stage("creative", "managed", false); err != nil {
		t.Fatal(err)
	}
	if err = m.Apply("creative"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(packDir); !os.IsNotExist(err) {
		t.Fatalf("disabled pack directory still exists: %v", err)
	}
}

func TestStageManyIsAtomicWhenAnyPackIsUnknown(t *testing.T) {
	m, err := NewManager(t.TempDir(), []config.Server{{ID: "creative"}})
	if err != nil {
		t.Fatal(err)
	}
	m.state.Packs = []StoredPack{{Pack: Pack{UUID: "known"}}}
	if err = m.StageMany("creative", []string{"known", "missing"}, true); err == nil {
		t.Fatal("invalid batch accepted")
	}
	if got := m.Pending("creative"); len(got) != 0 {
		t.Fatalf("partial batch was staged: %+v", got)
	}
}

func TestStageManyStagesEverySelectedPack(t *testing.T) {
	m, err := NewManager(t.TempDir(), []config.Server{{ID: "creative"}})
	if err != nil {
		t.Fatal(err)
	}
	m.state.Packs = []StoredPack{{Pack: Pack{UUID: "one"}}, {Pack: Pack{UUID: "two"}}}
	if err = m.StageMany("creative", []string{"one", "two"}, true); err != nil {
		t.Fatal(err)
	}
	if got := m.Pending("creative"); len(got) != 2 {
		t.Fatalf("pending=%+v", got)
	}
}

func TestUninstallManyIncludesLinkedBehaviorAndResourcePacks(t *testing.T) {
	root := t.TempDir()
	m, err := NewManager(root, []config.Server{{ID: "creative"}})
	if err != nil {
		t.Fatal(err)
	}
	bpPath := filepath.Join(root, "packs", "behavior", "1.0.0")
	rpPath := filepath.Join(root, "packs", "resource", "1.0.0")
	for _, path := range []string{bpPath, rpPath} {
		if err = os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	m.state.Packs = []StoredPack{
		{Pack: Pack{Name: "Behavior", UUID: "behavior", Dependencies: []Dependency{{UUID: "resource"}}}, InstalledPath: bpPath},
		{Pack: Pack{Name: "Resources", UUID: "resource"}, InstalledPath: rpPath},
	}
	plan, err := m.UninstallPlan([]string{"behavior"})
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if err = m.UninstallMany([]string{"behavior"}); err != nil {
		t.Fatal(err)
	}
	if len(m.Inventory()) != 0 {
		t.Fatalf("inventory=%+v", m.Inventory())
	}
	if _, err = os.Stat(filepath.Join(root, "uninstall-backups")); err != nil {
		t.Fatalf("rollback backup missing: %v", err)
	}
}

func TestUninstallManyIsAtomicWhenLinkedPackIsEnabled(t *testing.T) {
	root := t.TempDir()
	m, err := NewManager(root, []config.Server{{ID: "creative"}})
	if err != nil {
		t.Fatal(err)
	}
	bpPath := filepath.Join(root, "packs", "behavior", "1.0.0")
	rpPath := filepath.Join(root, "packs", "resource", "1.0.0")
	for _, path := range []string{bpPath, rpPath} {
		if err = os.MkdirAll(path, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	m.state.Packs = []StoredPack{
		{Pack: Pack{Name: "Behavior", UUID: "behavior", Dependencies: []Dependency{{UUID: "resource"}}}, InstalledPath: bpPath},
		{Pack: Pack{Name: "Resources", UUID: "resource"}, InstalledPath: rpPath},
	}
	m.state.Enabled["creative"] = map[string]bool{"resource": true}
	if err = m.UninstallMany([]string{"behavior"}); err == nil {
		t.Fatal("enabled linked pack was uninstalled")
	}
	if len(m.Inventory()) != 2 {
		t.Fatalf("partial inventory mutation: %+v", m.Inventory())
	}
	for _, path := range []string{bpPath, rpPath} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("pack moved despite rejected batch: %s: %v", path, statErr)
		}
	}
}
