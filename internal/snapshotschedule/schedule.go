package snapshotschedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/fleet"
	"nethernet-multi-server/internal/migration"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultFolder = `G:\My Drive\Minecraft Server Setup\Backup Snapshots`

type Settings struct {
	TaskName     string       `json:"task_name,omitempty"`
	OutputRoot   string       `json:"output_root"`
	Enabled      bool         `json:"enabled"`
	IntervalDays int          `json:"interval_days"`
	At           string       `json:"at"`
	Weekday      time.Weekday `json:"weekday"`
	Host         string       `json:"host"`
}

func Load(directory string) (Settings, error) {
	s := Settings{OutputRoot: DefaultFolder, IntervalDays: 7, At: "00:00", Weekday: time.Monday}
	b, err := os.ReadFile(filepath.Join(directory, "installer", "snapshot-settings.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	var legacy struct {
		Weekly  bool  `json:"weekly"`
		Enabled *bool `json:"enabled"`
	}
	if err = json.Unmarshal(b, &legacy); err != nil {
		return s, err
	}
	if legacy.Enabled == nil {
		s.Enabled = legacy.Weekly
	}
	if strings.EqualFold(filepath.Clean(s.OutputRoot), filepath.Clean(`G:\My Drive\Minecraft Server Setup`)) {
		s.OutputRoot = DefaultFolder
	}
	host, _ := os.Hostname()
	if s.Host != host {
		s.Enabled = false
	}
	return s, s.validate()
}
func Validate(root string) error {
	if !filepath.IsAbs(root) || strings.TrimSpace(root) == "" {
		return errors.New("choose an absolute snapshot folder")
	}
	return nil
}
func Save(directory string, s Settings) error {
	s = s.normalized()
	if err := s.validate(); err != nil {
		return err
	}
	s.Host, _ = os.Hostname()
	s.TaskName = TaskName(directory)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(directory, "installer", "snapshot-settings.json")
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return writeJSON(path, b)
}

func (s Settings) normalized() Settings {
	if s.IntervalDays == 0 {
		s.IntervalDays = 7
		s.Weekday = time.Monday
	}
	if s.At == "" {
		s.At = "00:00"
	}
	return s
}
func (s Settings) validate() error {
	if err := Validate(s.OutputRoot); err != nil {
		return err
	}
	if s.IntervalDays < 1 || s.IntervalDays > 7 {
		return errors.New("backup interval must be 1 to 7 days")
	}
	clock, err := time.Parse("15:04", s.At)
	if err != nil || clock.Format("15:04") != s.At {
		return errors.New("choose a valid 24-hour backup time")
	}
	if s.Weekday < time.Sunday || s.Weekday > time.Saturday {
		return errors.New("choose a valid weekly backup day")
	}
	return nil
}
func NextStart(s Settings, now time.Time) (time.Time, error) {
	s = s.normalized()
	if err := s.validate(); err != nil {
		return time.Time{}, err
	}
	clock, _ := time.Parse("15:04", s.At)
	next := time.Date(now.Year(), now.Month(), now.Day(), clock.Hour(), clock.Minute(), 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	if s.IntervalDays == 7 {
		for next.Weekday() != s.Weekday {
			next = next.AddDate(0, 0, 1)
		}
	}
	return next, nil
}
func (s Settings) Description() string {
	s = s.normalized()
	if s.IntervalDays == 7 {
		return fmt.Sprintf("Weekly on %s at %s", s.Weekday, s.At)
	}
	if s.IntervalDays == 1 {
		return "Daily at " + s.At
	}
	return fmt.Sprintf("Every %d days at %s", s.IntervalDays, s.At)
}
func writeJSON(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".snapshot-settings-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func Destination(root string, now time.Time) string {
	return filepath.Join(root, now.Format("2006-01-02_15-04-05"))
}
func ProtectedDestination(root string, now time.Time) string {
	return Destination(filepath.Join(root, "Protected"), now)
}
func ScheduledDestination(root string, now time.Time) string {
	return Destination(filepath.Join(root, "Scheduled"), now)
}
func Capture(ctx context.Context, client fleet.Client, root string, now time.Time, notify func(string)) error {
	if err := Validate(root); err != nil {
		return err
	}
	if notify == nil {
		notify = func(string) {}
	}
	full := true
	var job struct {
		ID string `json:"job_id"`
	}
	if err := client.Call(ctx, "migration.start", adminapi.MigrationStartParams{OutputDirectory: ScheduledDestination(root, now), IncludeAddons: &full, Kind: "scheduled"}, &job); err != nil {
		return err
	}
	for {
		var status migration.JobStatus
		if err := client.Call(ctx, "migration.status", adminapi.MigrationJobParams{JobID: job.ID}, &status); err != nil {
			return err
		}
		notify(fmt.Sprintf("%s: %d%%", status.Phase, status.Progress))
		if status.Done {
			if status.Error != "" {
				return errors.New(status.Error)
			}
			if !status.Validated {
				return errors.New("snapshot did not pass validation")
			}
			if status.RestartError != "" {
				return fmt.Errorf("snapshot saved but server restart failed: %s", status.RestartError)
			}
			notify("Snapshot saved: " + status.Path)
			if status.Warning != "" {
				notify("Backup warning: " + status.Warning)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
