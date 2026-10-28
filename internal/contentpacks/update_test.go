package contentpacks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdatePreviewRequiresNewerSelectedAndFreshState(t *testing.T) {
	m, _ := versionFixture(t)
	if _, e := m.PreviewUpdate(versionArchive(t, 1), updatePackID, []string{"creative"}, map[string][3]int{"creative": {1, 26, 60}}); e == nil {
		t.Fatal("accepted unchanged selected pack")
	}
	p, e := m.PreviewUpdate(versionArchive(t, 2), updatePackID, []string{"creative"}, map[string][3]int{"creative": {1, 26, 60}})
	if e != nil {
		t.Fatal(e)
	}
	if e = m.StageResourcePolicy("survival", true); e != nil {
		t.Fatal(e)
	}
	if e = m.StageUpdate(p.ID); e == nil {
		t.Fatal("accepted stale review")
	}
}
func TestUpdateStagesOnlyReviewedTarget(t *testing.T) {
	m, servers := versionFixture(t)
	p, e := m.PreviewUpdate(versionArchive(t, 2), updatePackID, []string{"creative"}, map[string][3]int{"creative": {1, 26, 60}})
	if e != nil {
		t.Fatal(e)
	}
	if e = m.StageUpdate(p.ID); e != nil {
		t.Fatal(e)
	}
	if v := deployedVersion(t, servers[0]); v != [3]int{1, 0, 0} {
		t.Fatal("staging changed running folder")
	}
	if e = m.Apply("creative"); e != nil {
		t.Fatal(e)
	}
	if e = m.StageResourcePolicy("survival", true); e != nil {
		t.Fatal(e)
	}
	if e = m.Apply("survival"); e != nil {
		t.Fatal(e)
	}
	if deployedVersion(t, servers[0]) != [3]int{2, 0, 0} || deployedVersion(t, servers[1]) != [3]int{1, 0, 0} {
		t.Fatal("wrong target versions")
	}
	if e = m.StageUpdate(p.ID); e == nil {
		t.Fatal("reused review")
	}
}

func TestUpdateReviewRejectsChangedArchiveAndRollsBackSave(t *testing.T) {
	m, _ := versionFixture(t)
	engines := map[string][3]int{"creative": {1, 26, 60}}
	p, e := m.PreviewUpdate(versionArchive(t, 2), updatePackID, []string{"creative"}, engines)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(m.previews[p.ID].Path, []byte("changed"), 0600)
	if e = m.StageUpdate(p.ID); e == nil {
		t.Fatal("accepted modified reviewed archive")
	}
	p, e = m.PreviewUpdate(versionArchive(t, 2), updatePackID, []string{"creative"}, engines)
	if e != nil {
		t.Fatal(e)
	}
	before := m.fingerprint()
	saved := m.statePath
	m.statePath = filepath.Join(t.TempDir(), "missing", "state.json")
	if e = m.StageUpdate(p.ID); e == nil {
		t.Fatal("expected save error")
	}
	m.statePath = saved
	if before != m.fingerprint() {
		t.Fatal("state changed after failed transaction")
	}
	p, e = m.PreviewUpdate(versionArchive(t, 2), updatePackID, []string{"creative"}, engines)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.StageUpdate(p.ID); e != nil {
		t.Fatalf("retry failed after rollback: %v", e)
	}
}
func TestUpdateReviewRejectsWrongUUIDAndEngine(t *testing.T) {
	m, _ := versionFixture(t)
	if _, e := m.PreviewUpdate(versionArchive(t, 2), "wrong", []string{"creative"}, map[string][3]int{"creative": {1, 26, 60}}); e == nil {
		t.Fatal("wrong UUID accepted")
	}
	if _, e := m.PreviewUpdate(versionArchive(t, 2), updatePackID, []string{"creative"}, map[string][3]int{"creative": {1, 20, 0}}); e == nil {
		t.Fatal("engine mismatch accepted")
	}
}

func TestUpdateBundleRequiresConnectedDependencies(t *testing.T) {
	m, _ := versionFixture(t)
	engines := map[string][3]int{"creative": {1, 26, 60}}
	makePack := func(id string, version int, deps string) string {
		return fmt.Sprintf(`{"format_version":2,"header":{"name":"Bundle pack","uuid":"%s","version":[%d,0,0],"min_engine_version":[1,26,0]},"modules":[{"type":"data","uuid":"33333333-3333-3333-3333-333333333333","version":[1,0,0]}],"dependencies":%s}`, id, version, deps)
	}
	companion := "44444444-4444-4444-4444-444444444444"
	archive := zipFile(t, map[string]string{"main/manifest.json": makePack(updatePackID, 2, "[]"), "other/manifest.json": makePack(companion, 1, "[]")})
	if _, e := m.PreviewUpdate(archive, updatePackID, []string{"creative"}, engines); e == nil {
		t.Fatal("unrelated bundle accepted")
	}
	archive = zipFile(t, map[string]string{"main/manifest.json": makePack(updatePackID, 2, fmt.Sprintf(`[{"uuid":"%s","version":[1,0,0]}]`, companion)), "other/manifest.json": makePack(companion, 1, "[]")})
	p, e := m.PreviewUpdate(archive, updatePackID, []string{"creative"}, engines)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Changes) != 2 {
		t.Fatal("dependency omitted")
	}
	if e = m.StageUpdate(p.ID); e != nil {
		t.Fatal(e)
	}
	if e = m.Apply("creative"); e != nil {
		t.Fatal(e)
	}
}

func TestUpdateRejectsEscapingUUID(t *testing.T) {
	bad := strings.Replace(behaviorManifest, updatePackID, "../../outside", 1)
	if _, e := Inspect(zipFile(t, map[string]string{"manifest.json": bad}), DefaultLimits); e == nil {
		t.Fatal("unsafe UUID accepted")
	}
}
func TestUpdateRetainedManifestIsAuthoritative(t *testing.T) {
	m, _ := versionFixture(t)
	companion := "44444444-4444-4444-4444-444444444444"
	retained := strings.ReplaceAll(behaviorManifest, updatePackID, companion)
	retained = strings.Replace(retained, "[1,26,0]", "[1,99,0]", 1)
	if _, e := m.ImportArchive(zipFile(t, map[string]string{"manifest.json": retained})); e != nil {
		t.Fatal(e)
	}
	main := strings.Replace(behaviorManifest, "[1,2,3]", "[2,0,0]", 1)
	main = strings.TrimSuffix(main, "}") + fmt.Sprintf(`,"dependencies":[{"uuid":"%s","version":[1,2,3]}]}`, companion)
	fake := strings.ReplaceAll(behaviorManifest, updatePackID, companion)
	if _, e := m.PreviewUpdate(zipFile(t, map[string]string{"a/manifest.json": main, "b/manifest.json": fake}), updatePackID, []string{"creative"}, map[string][3]int{"creative": {1, 26, 60}}); e == nil {
		t.Fatal("discarded manifest hid retained engine requirement")
	}
}
func TestUpdateCannotEnableDisabledTarget(t *testing.T) {
	m, _ := versionFixture(t)
	m.Stage("survival", updatePackID, false)
	m.Apply("survival")
	if _, e := m.PreviewUpdate(versionArchive(t, 2), updatePackID, []string{"creative", "survival"}, map[string][3]int{"creative": {1, 26, 60}, "survival": {1, 26, 60}}); e == nil {
		t.Fatal("disabled target accepted without enablement review")
	}
}

func TestNewDependentCannotMasqueradeAsRequiredCompanion(t *testing.T) {
	m, _ := versionFixture(t)
	newID := "44444444-4444-4444-4444-444444444444"
	main := strings.Replace(behaviorManifest, "[1,2,3]", "[2,0,0]", 1)
	extra := strings.ReplaceAll(behaviorManifest, updatePackID, newID)
	extra = strings.TrimSuffix(extra, "}") + fmt.Sprintf(`,"dependencies":[{"uuid":"%s","version":[2,0,0]}]}`, updatePackID)
	if _, e := m.PreviewUpdate(zipFile(t, map[string]string{"a/manifest.json": main, "b/manifest.json": extra}), updatePackID, []string{"creative"}, map[string][3]int{"creative": {1, 26, 60}}); e == nil {
		t.Fatal("new unrelated dependent accepted")
	}
}
