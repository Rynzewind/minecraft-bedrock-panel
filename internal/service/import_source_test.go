package service

import (
	"context"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/contentpacks"
	"os"
	"path/filepath"
	"testing"
)

type importIdentityProvider struct{ overviewProvider }

func (*importIdentityProvider) IdentifyDownload(context.Context, []byte, int64) (addonupdates.Project, addonupdates.ReleaseFile, error) {
	p := addonupdates.Project{ID: 42, GameID: 99, Slug: "example", Name: "Example"}
	p.Links.WebsiteURL = "https://www.curseforge.com/minecraft-bedrock/addons/example"
	return p, addonupdates.ReleaseFile{ID: 123, ModID: 42}, nil
}
func TestImportedSourceLinksOnlyNewExactBytesAndPreservesCustomSettings(t *testing.T) {
	root := t.TempDir()
	provider := &overviewProvider{root: root, match: true}
	archive, err := provider.Download(context.Background(), addonupdates.Project{}, addonupdates.ReleaseFile{}, root)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := contentpacks.NewManager(filepath.Join(root, "packs"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.ImportArchive(archive); err != nil {
		t.Fatal(err)
	}
	sources := addonupdates.New(filepath.Join(root, "sources.json"))
	credentials := addonupdates.NewCredentials(filepath.Join(root, "key.json"), plainProtector{})
	credentials.Set("fixture-key")
	c := &Controller{packs: manager}
	c.SetAddonSources(sources)
	c.SetAddonUpdates(credentials, "", root)
	c.addonProviderFactory = func(string) addonProvider { return &importIdentityProvider{} }
	result, err := c.identifyImportedSource(context.Background(), archive)
	if err != nil || result.Linked != 1 {
		t.Fatalf("not linked: %+v %v", result, err)
	}
	const id = "11111111-1111-1111-1111-111111111111"
	records, _ := sources.ListRecords()
	source := records[id]
	if source.ProjectID != 42 || source.URL == "" || source.LatestCheckOK {
		t.Fatal("incorrect source metadata")
	}
	source.LocallyModified = true
	source.Notes = "Keep my changes"
	sources.SaveRecord(id, source)
	result, err = c.identifyImportedSource(context.Background(), archive)
	if err != nil || result.Linked != 0 || result.Preserved != 1 {
		t.Fatal("existing custom source changed")
	}
	records, _ = sources.ListRecords()
	if !records[id].LocallyModified || records[id].Notes != "Keep my changes" {
		t.Fatal("custom protection lost")
	}
	// A different outer ZIP with identical pack UUID/version must not identify the original stored bytes.
	sources = addonupdates.New(filepath.Join(root, "other-sources.json"))
	c.SetAddonSources(sources)
	data, _ := os.ReadFile(archive)
	data = append(data, []byte("different trailing bytes")...)
	os.WriteFile(archive, data, 0600)
	result, err = c.identifyImportedSource(context.Background(), archive)
	if err != nil || result.Linked != 0 {
		t.Fatalf("changed archive linked: %+v %v", result, err)
	}
}
