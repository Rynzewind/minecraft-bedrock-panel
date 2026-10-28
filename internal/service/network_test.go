package service

import (
	"context"
	"encoding/json"
	"testing"
)

func TestShutdownBlocksNewSnapshotUntilExit(t *testing.T) {
	c := NewController(nil, nil, nil)
	c.supervisor = &fakeSupervisor{}
	c.SetShutdown(func() {})
	c.SetMigration(&fakeMigration{})
	if _, e := c.Handle(context.Background(), "system.shutdown", json.RawMessage(`{}`)); e != nil {
		t.Fatal(e)
	}
	if _, e := c.Handle(context.Background(), "migration.start", json.RawMessage(`{}`)); e == nil {
		t.Fatal("snapshot started during shutdown")
	}
}
