package contentpacks

import (
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/config"
	"os"
	"path/filepath"
	"testing"
)

const updatePackID = "11111111-1111-1111-1111-111111111111"

func versionArchive(t *testing.T, version int) string {
	return zipFile(t, map[string]string{"manifest.json": fmt.Sprintf(`{"format_version":2,"header":{"name":"Versioned pack","description":"test","uuid":"%s","version":[%d,0,0],"min_engine_version":[1,26,0]},"modules":[{"type":"data","uuid":"22222222-2222-2222-2222-222222222222","version":[1,0,0]}]}`, updatePackID, version), "version.txt": fmt.Sprint(version)})
}
func versionFixture(t *testing.T) (*Manager, []config.Server) {
	t.Helper()
	return versionFixtureAt(t, 1)
}
func versionFixtureAt(t *testing.T, initial int) (*Manager, []config.Server) {
	root := t.TempDir()
	var servers []config.Server
	for _, id := range []string{"creative", "survival"} {
		dir := filepath.Join(root, id)
		world := filepath.Join(dir, "worlds", "World")
		os.MkdirAll(world, 0700)
		os.WriteFile(filepath.Join(dir, "server.properties"), []byte(""), 0600)
		servers = append(servers, config.Server{ID: id, Process: &config.Process{WorkingDirectory: dir, WorldDirectory: world}})
	}
	m, e := NewManager(filepath.Join(root, "library"), servers)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.ImportArchive(versionArchive(t, initial)); e != nil {
		t.Fatal(e)
	}
	for _, s := range servers {
		if e = m.Stage(s.ID, updatePackID, true); e != nil {
			t.Fatal(e)
		}
		if e = m.Apply(s.ID); e != nil {
			t.Fatal(e)
		}
	}
	return m, servers
}
func deployedVersion(t *testing.T, s config.Server) [3]int {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(s.Process.WorldDirectory, "world_behavior_packs.json"))
	if e != nil {
		t.Fatal(e)
	}
	var v []activation
	if e = json.Unmarshal(b, &v); e != nil || len(v) != 1 {
		t.Fatalf("activation %s %v", b, e)
	}
	return v[0].Version
}
func TestUnrelatedApplyDoesNotUpgradeOtherServer(t *testing.T) {
	m, servers := versionFixture(t)
	if _, e := m.ImportArchive(versionArchive(t, 2)); e != nil {
		t.Fatal(e)
	}
	if e := m.StageResourcePolicy("survival", true); e != nil {
		t.Fatal(e)
	}
	if e := m.Apply("survival"); e != nil {
		t.Fatal(e)
	}
	if v := deployedVersion(t, servers[1]); v != [3]int{1, 0, 0} {
		t.Fatalf("unselected server upgraded to %v", v)
	}
}
func TestPendingPackVersionRemainsPinnedAfterLibraryImport(t *testing.T) {
	m, servers := versionFixture(t)
	m.ImportArchive(versionArchive(t, 2))
	if e := m.Stage("creative", updatePackID, true); e != nil {
		t.Fatal(e)
	}
	m.ImportArchive(versionArchive(t, 3))
	if e := m.Apply("creative"); e != nil {
		t.Fatal(e)
	}
	if v := deployedVersion(t, servers[0]); v != [3]int{2, 0, 0} {
		t.Fatalf("approved v2 replaced by %v", v)
	}
}

func TestZeroVersionPendingRemainsPinned(t *testing.T) {
	m, servers := versionFixtureAt(t, 0)
	if e := m.Stage("creative", updatePackID, true); e != nil {
		t.Fatal(e)
	}
	if _, e := m.ImportArchive(versionArchive(t, 2)); e != nil {
		t.Fatal(e)
	}
	if e := m.Apply("creative"); e != nil {
		t.Fatal(e)
	}
	if v := deployedVersion(t, servers[0]); v != [3]int{} {
		t.Fatalf("zero version replaced: %v", v)
	}
}
func TestLegacyVersionsReadDeployedSelection(t *testing.T) {
	m, servers := versionFixture(t)
	m.ImportArchive(versionArchive(t, 2))
	m.state.EnabledVersions = nil
	m.state.Versions = nil
	if e := m.save(); e != nil {
		t.Fatal(e)
	}
	reopened, e := NewManager(m.root, servers)
	if e != nil {
		t.Fatal(e)
	}
	if e = reopened.StageResourcePolicy("survival", true); e != nil {
		t.Fatal(e)
	}
	if e = reopened.Apply("survival"); e != nil {
		t.Fatal(e)
	}
	if v := deployedVersion(t, servers[1]); v != [3]int{1, 0, 0} {
		t.Fatalf("legacy selection changed: %v", v)
	}
}
