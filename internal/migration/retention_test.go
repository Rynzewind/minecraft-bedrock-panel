package migration

import (
	"context"
	"fmt"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func scheduledFixture(t *testing.T, m *Manager, base string, n int) string {
	t.Helper()
	owner, err := m.snapshotOwner()
	if err != nil {
		t.Fatal(err)
	}
	root := fixture(t)
	files, err := BuildInventory(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("%016x", n)
	out := filepath.Join(base, "Scheduled", time.Date(2026, 10, n, 1, 0, 0, 0, time.UTC).Format("2006-01-02_15-04-05"))
	if err = os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(out, "Minecraft-20261024-010000-"+id+".zip")
	manifest := Manifest{FormatVersion: FormatVersion, CreatedUTC: time.Date(2026, 10, n, 1, 0, 0, 0, time.UTC), SourceRoot: root, Files: files, Backup: &BackupMetadata{Kind: "scheduled", Owner: owner, JobID: id}}
	if err = WriteArchive(context.Background(), root, path, manifest); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestRetentionOnlyRotatesTrackedVerifiedScheduledArchives(t *testing.T) {
	m := NewManager(fixture(t), &fakeSupervisor{}, emptyExport)
	base := t.TempDir()
	protected := filepath.Join(base, "Protected", "keep.zip")
	put(t, base, "Protected/keep.zip", "keep")
	unknown := filepath.Join(base, "Scheduled", "unknown.zip")
	put(t, base, "Scheduled/unknown.zip", "unknown")
	var paths []string
	for i := 1; i <= 9; i++ {
		p := scheduledFixture(t, m, base, i)
		paths = append(paths, p)
		if i == 1 {
			put(t, filepath.Dir(p), "keep-notes.txt", "unrelated")
		}
		if err := m.retainScheduled(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		for j, old := range paths {
			_, err := os.Stat(old)
			if j < i-7 {
				if !os.IsNotExist(err) {
					t.Fatal("old archive retained", old)
				}
			} else if err != nil {
				t.Fatal("recent archive lost", old, err)
			}
		}
	}
	for _, p := range []string{protected, unknown} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("untracked data lost", p)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(paths[0]), "keep-notes.txt")); err != nil {
		t.Fatal("unrelated dated-folder contents removed", err)
	}
}

func TestScheduledCaptureWarnsWithPlayersAndRestartFailureRetainsBackups(t *testing.T) {
	s := &announcingSupervisor{fakeSupervisor: fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running, Players: 1}}, failStart: "creative"}}
	m := NewManager(fixture(t), s, emptyExport)
	waits := 0
	m.warningWait = func(context.Context, time.Duration) error { waits++; return nil }
	output := filepath.Join(t.TempDir(), "Scheduled", "2026-10-24_01-00-00")
	id, err := m.StartWithOptions(context.Background(), output, SnapshotOptions{IncludeAddons: true, Kind: "scheduled"})
	if err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, m, id)
	if !job.Validated || job.RestartError == "" || waits != 5 {
		t.Fatalf("scheduled capture failed: %+v waits=%d", job, waits)
	}
	if _, err = os.Stat(m.retentionPath()); !os.IsNotExist(err) {
		t.Fatal("retention attempted despite restart failure")
	}
}
func TestScheduledPathRejectsOutsideAndTraversal(t *testing.T) {
	base := t.TempDir()
	for _, p := range []string{filepath.Join(base, "Protected", "2026-10-24_01-00-00", "x.zip"), filepath.Join(base, "Scheduled", "nondated", "x.zip"), base + string(filepath.Separator) + "Scheduled" + string(filepath.Separator) + ".." + string(filepath.Separator) + "2026-10-24_01-00-00" + string(filepath.Separator) + "x.zip"} {
		if _, err := scheduledRoot(p); err == nil {
			t.Fatal("unsafe path accepted", p)
		}
	}
}
func TestRetentionCorruptionNeverDeletesOldCopies(t *testing.T) {
	m := NewManager(fixture(t), &fakeSupervisor{}, emptyExport)
	base := t.TempDir()
	var paths []string
	for i := 1; i <= 7; i++ {
		p := scheduledFixture(t, m, base, i)
		paths = append(paths, p)
		if err := m.retainScheduled(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(paths[0], []byte("corrupt"), 0600)
	latest := scheduledFixture(t, m, base, 8)
	if err := m.retainScheduled(context.Background(), latest); err == nil {
		t.Fatal("corruption not reported")
	}
	for _, p := range append(paths, latest) {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("deleted after corruption", p)
		}
	}
}
func TestRetentionRejectsProtectedAndCorruptLedger(t *testing.T) {
	m := NewManager(fixture(t), &fakeSupervisor{}, emptyExport)
	base := t.TempDir()
	p := scheduledFixture(t, m, base, 1)
	protected := filepath.Join(base, "Protected", filepath.Base(filepath.Dir(p)), filepath.Base(p))
	os.MkdirAll(filepath.Dir(protected), 0700)
	os.Rename(p, protected)
	if m.retainScheduled(context.Background(), protected) == nil {
		t.Fatal("protected path accepted")
	}
	p = scheduledFixture(t, m, base, 2)
	os.MkdirAll(filepath.Dir(m.retentionPath()), 0700)
	os.WriteFile(m.retentionPath(), []byte("bad ledger"), 0600)
	if m.retainScheduled(context.Background(), p) == nil {
		t.Fatal("corrupt ledger accepted")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}
