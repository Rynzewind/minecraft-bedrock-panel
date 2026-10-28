package service

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/migration"
	"testing"
)

type fakeMigration struct {
	busy      bool
	cancelled string
}
type optionsMigration struct {
	fakeMigration
	options migration.SnapshotOptions
}

func (m *optionsMigration) StartWithOptions(_ context.Context, _ string, o migration.SnapshotOptions) (string, error) {
	m.options = o
	return "one", nil
}
func TestScheduledKindForwardedWithoutAddonPointer(t *testing.T) {
	m := &optionsMigration{}
	c := NewController(nil, nil, nil)
	c.SetMigration(m)
	if _, err := c.Handle(context.Background(), "migration.start", json.RawMessage(`{"output_directory":"x","kind":"scheduled"}`)); err != nil {
		t.Fatal(err)
	}
	if m.options.Kind != "scheduled" || !m.options.IncludeAddons {
		t.Fatal(m.options)
	}
}

func (m *fakeMigration) Busy() bool { return m.busy }
func (m *fakeMigration) Start(context.Context, string) (string, error) {
	m.busy = true
	return "job1", nil
}
func (m *fakeMigration) Status(string) (migration.JobStatus, error) {
	return migration.JobStatus{ID: "job1", Phase: "copying", Progress: 35}, nil
}
func (m *fakeMigration) Cancel(id string) error { m.cancelled = id; return nil }
func TestMigrationBusyGatesMutationsAndAllowsStatus(t *testing.T) {
	m := &fakeMigration{busy: true}
	c := NewController(nil, nil, nil)
	c.SetMigration(m)
	for _, method := range []string{"server.start", "server.stop", "server.restart", "server.settings.save", "pack.import", "pack.stage", "player.role.set", "account.labels", "system.shutdown", "enrollment.complete"} {
		if _, e := c.Handle(context.Background(), method, json.RawMessage(`{}`)); e == nil {
			t.Fatalf("allowed %s", method)
		} else if ce, ok := e.(coded); !ok || ce.code != "migration_busy" {
			t.Fatalf("wrong error for %s: %v", method, e)
		}
	}
	r, e := c.Handle(context.Background(), "migration.status", json.RawMessage(`{"job_id":"job1"}`))
	if e != nil || r.(migration.JobStatus).Progress != 35 {
		t.Fatal(r, e)
	}
	_, e = c.Handle(context.Background(), "migration.cancel", json.RawMessage(`{"job_id":"job1"}`))
	if e != nil || m.cancelled != "job1" {
		t.Fatal(e)
	}
}
