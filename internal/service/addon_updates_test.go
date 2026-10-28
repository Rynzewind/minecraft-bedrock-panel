package service

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/addonupdates"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddonCredentialsNeverReturnKeyAndMutationHonorsMaintenance(t *testing.T) {
	root := t.TempDir()
	c := &Controller{}
	c.SetAddonSources(addonupdates.New(filepath.Join(root, "sources.json")))
	c.SetAddonUpdates(addonupdates.NewCredentials(filepath.Join(root, "credentials", "key.json"), plainProtector{}), "", "")
	ctx := context.Background()
	if _, e := c.Handle(ctx, "addons.credentials.save", json.RawMessage(`{"Key":"test-secret-key"}`)); e != nil {
		t.Fatal(e)
	}
	result, e := c.Handle(ctx, "addons.credentials.status", json.RawMessage(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(result)
	if strings.Contains(string(b), "test-secret-key") {
		t.Fatal("key returned")
	}
	c.SetMigration(&fakeMigration{busy: true})
	if _, e = c.Handle(ctx, "addons.credentials.remove", json.RawMessage(`{}`)); e == nil {
		t.Fatal("mutation during snapshot")
	}
	if _, e = c.Handle(ctx, "addons.stage", json.RawMessage(`{"PreviewID":"invented"}`)); e == nil {
		t.Fatal("stage during snapshot")
	}
	c.SetMigration(nil)
	if _, e = c.Handle(ctx, "addons.preview", json.RawMessage(`{"UUID":"wrong"}`)); e == nil {
		t.Fatal("invalid UUID accepted")
	}
	if _, e = c.Handle(ctx, "addons.sources.records", json.RawMessage(`{}`)); e != nil {
		t.Fatal(e)
	}
}
