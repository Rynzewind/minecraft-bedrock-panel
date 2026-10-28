package snapshotschedule

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/migration"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type snapshotClient struct {
	root   string
	full   bool
	kind   string
	result migration.JobStatus
}

func (c *snapshotClient) Call(_ context.Context, method string, p, t any) error {
	if method == "migration.start" {
		options := p.(adminapi.MigrationStartParams)
		c.root = options.OutputDirectory
		c.full = options.IncludeAddons != nil && *options.IncludeAddons
		c.kind = options.Kind
		b := []byte(`{"job_id":"one"}`)
		return json.Unmarshal(b, t)
	}
	*t.(*migration.JobStatus) = c.result
	return nil
}
func TestFullSnapshotUsesDatedFolderAndIncludesAddons(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local)
	c := &snapshotClient{result: migration.JobStatus{Done: true, Validated: true, Path: "saved.zip"}}
	if err := Capture(context.Background(), c, root, now, nil); err != nil {
		t.Fatal(err)
	}
	if c.root != filepath.Join(root, "Scheduled", "2026-10-26_00-00-00") || !c.full || c.kind != "scheduled" {
		t.Fatalf("wrong capture settings: %+v", c)
	}
}
func TestManualDestinationIsProtected(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local)
	if got := ProtectedDestination(root, now); got != filepath.Join(root, "Protected", "2026-10-26_00-00-00") {
		t.Fatal(got)
	}
}
func TestSnapshotRestartFailureIsNotSuccess(t *testing.T) {
	c := &snapshotClient{result: migration.JobStatus{Done: true, Validated: true, RestartError: "server failed"}}
	if Capture(context.Background(), c, t.TempDir(), time.Now(), nil) == nil {
		t.Fatal("restart failure reported success")
	}
}
func TestDefaultAndSavedFolder(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil || s.OutputRoot != `G:\My Drive\Minecraft Server Setup\Backup Snapshots` || s.Enabled {
		t.Fatalf("wrong defaults: %+v %v", s, err)
	}
	s.OutputRoot = filepath.Join(t.TempDir(), "custom")
	s.IntervalDays = 3
	s.At = "03:15"
	s.Weekday = time.Sunday
	s.Enabled = true
	if err = Save(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.OutputRoot != s.OutputRoot || !got.Enabled || got.IntervalDays != 3 || got.At != "03:15" || got.Weekday != time.Sunday {
		t.Fatalf("settings lost: %+v %v", got, err)
	}
}

func TestOldDefaultMovesIntoBackupSnapshots(t *testing.T) {
	dir := t.TempDir()
	s := Settings{OutputRoot: `G:\My Drive\Minecraft Server Setup`, Enabled: true}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.OutputRoot != DefaultFolder || !got.Enabled {
		t.Fatalf("default upgrade lost settings: %+v %v", got, err)
	}
}

func TestScheduleNextStartAndDefaults(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.Local)
	s := Settings{OutputRoot: DefaultFolder, Enabled: true, IntervalDays: 3, At: "03:15", Weekday: time.Monday}
	next, err := NextStart(s, now)
	if err != nil || next.Format("2006-01-02 15:04") != "2026-10-25 03:15" {
		t.Fatalf("wrong first interval run: %v %v", next, err)
	}
	s.IntervalDays = 7
	next, err = NextStart(s, now)
	if err != nil || next.Format("2006-01-02 15:04") != "2026-10-26 03:15" {
		t.Fatalf("wrong weekly run: %v %v", next, err)
	}
	s.IntervalDays = 8
	if _, err = NextStart(s, now); err == nil {
		t.Fatal("invalid interval accepted")
	}
	s.IntervalDays = 1
	s.At = "24:00"
	if _, err = NextStart(s, now); err == nil {
		t.Fatal("invalid time accepted")
	}
}

func TestLegacyWeeklySettingsKeepMondayMidnight(t *testing.T) {
	dir := t.TempDir()
	host, _ := os.Hostname()
	path := filepath.Join(dir, "installer")
	os.MkdirAll(path, 0700)
	b, _ := json.Marshal(map[string]any{"output_root": DefaultFolder, "weekly": true, "host": host})
	os.WriteFile(filepath.Join(path, "snapshot-settings.json"), b, 0600)
	s, err := Load(dir)
	if err != nil || !s.Enabled || s.IntervalDays != 7 || s.At != "00:00" || s.Weekday != time.Monday {
		t.Fatalf("legacy schedule changed: %+v %v", s, err)
	}
}
