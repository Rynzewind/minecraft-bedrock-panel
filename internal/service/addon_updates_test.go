package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/contentpacks"
	"os"
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

func TestAddonCheckOnlyVerifiesMatchingArchive(t *testing.T) {
	const id = "11111111-1111-1111-1111-111111111111"
	for _, match := range []bool{true, false} {
		t.Run(fmt.Sprint(match), func(t *testing.T) {
			root := t.TempDir()
			sources := addonupdates.New(filepath.Join(root, "sources.json"))
			src := addonupdates.Source{Provider: "curseforge", ProjectID: 42, Channel: "stable"}
			sources.SaveRecord(id, src)
			credentials := addonupdates.NewCredentials(filepath.Join(root, "credentials", "key.json"), plainProtector{})
			credentials.Set("fixture-api-key")
			c := &Controller{packs: &overviewPacks{}}
			c.SetAddonSources(sources)
			c.SetAddonUpdates(credentials, "", root)
			c.addonProviderFactory = func(string) addonProvider { return &overviewProvider{root: root, match: match} }
			if _, e := c.Handle(context.Background(), "addons.check", json.RawMessage(`{"UUID":"`+id+`"}`)); e != nil {
				t.Fatal(e)
			}
			rows, _ := sources.ListRecords()
			r := rows[id]
			if match {
				if !r.LatestCheckOK || r.VerifiedVersion == nil || *r.VerifiedVersion != [3]int{2, 0, 0} {
					t.Fatal("matching archive not verified")
				}
			} else {
				if r.LatestCheckOK || r.VerifiedVersion != nil {
					t.Fatal("wrong UUID marked verified")
				}
			}
		})
	}
}

type overviewPacks struct{ fakePacks }

func (*overviewPacks) InventoryState() []contentpacks.InventoryItem {
	return []contentpacks.InventoryItem{{StoredPack: contentpacks.StoredPack{Pack: contentpacks.Pack{UUID: "11111111-1111-1111-1111-111111111111", Type: contentpacks.Behavior, Version: [3]int{1, 0, 0}}}}}
}

type overviewProvider struct {
	root  string
	match bool
}

func (*overviewProvider) Test(context.Context) error { return nil }
func (*overviewProvider) Check(context.Context, addonupdates.Source) (addonupdates.Project, []addonupdates.ReleaseFile, error) {
	return addonupdates.Project{ID: 42}, []addonupdates.ReleaseFile{{ID: 123}}, nil
}
func (p *overviewProvider) Download(context.Context, addonupdates.Project, addonupdates.ReleaseFile, string) (string, error) {
	id := "11111111-1111-1111-1111-111111111111"
	if !p.match {
		id = "22222222-2222-2222-2222-222222222222"
	}
	path := filepath.Join(p.root, "archive.zip")
	f, e := os.Create(path)
	if e != nil {
		return "", e
	}
	z := zip.NewWriter(f)
	w, _ := z.Create("manifest.json")
	fmt.Fprintf(w, `{"header":{"name":"fixture","uuid":"%s","version":[2,0,0]},"modules":[{"type":"data"}]}`, id)
	z.Close()
	f.Close()
	return path, nil
}
