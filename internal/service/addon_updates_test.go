package service

import (
	"context"
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/contentpacks"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestProviderProvenanceRequiresSuccessfulStage(t *testing.T) {
	root := t.TempDir()
	sources := addonupdates.New(filepath.Join(root, "sources.json"))
	src := addonupdates.Source{URL: "https://www.curseforge.com/minecraft-bedrock/addons/test", Provider: "curseforge", ProjectID: 42, Channel: "stable"}
	sources.SaveRecord("id", src)
	packs := &provenancePacks{fail: true}
	c := &Controller{packs: packs}
	c.SetAddonSources(sources)
	c.SetAddonUpdates(addonupdates.NewCredentials(filepath.Join(root, "key.json"), plainProtector{}), filepath.Join("..", "..", "config.example.json"), root)
	c.addonOrigins["review"] = addonOrigin{UUID: "id", Source: src, ProjectID: 42, FileID: 123, Expires: time.Now().Add(time.Minute)}
	if _, e := c.Handle(context.Background(), "addons.stage", json.RawMessage(`{"PreviewID":"review"}`)); e == nil {
		t.Fatal("expected stage failure")
	}
	rows, _ := sources.ListRecords()
	if rows["id"].LastFileID != 0 {
		t.Fatal("failed stage acknowledged release")
	}
	packs.fail = false
	if _, e := c.Handle(context.Background(), "addons.stage", json.RawMessage(`{"PreviewID":"review"}`)); e != nil {
		t.Fatal(e)
	}
	rows, _ = sources.ListRecords()
	if rows["id"].LastFileID != 123 {
		t.Fatal("successful stage omitted provenance")
	}
}

type provenancePacks struct {
	fakePacks
	fail bool
}

func (p *provenancePacks) StageUpdateForEngines(string, map[string][3]int) error {
	if p.fail {
		return fmt.Errorf("failed")
	}
	return nil
}
func (p *provenancePacks) PreviewUpdate(string, string, []string, map[string][3]int) (contentpacks.UpdatePreview, error) {
	return contentpacks.UpdatePreview{}, nil
}
