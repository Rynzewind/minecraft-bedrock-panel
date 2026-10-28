package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddonSourcesIncludedButProviderCredentialsExcluded(t *testing.T) {
	root := fixture(t)
	key := filepath.Join(root, "MultiServer", "provider-credentials", "curseforge.json")
	os.MkdirAll(filepath.Dir(key), 0700)
	os.WriteFile(key, []byte("protected-key"), 0600)
	os.WriteFile(filepath.Join(root, "MultiServer", "addon-update-sources.json"), []byte(`{"version":2,"sources":{}}`), 0600)
	files, e := BuildInventory(root, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, f := range files {
		if f.Path == "MultiServer/provider-credentials/curseforge.json" {
			t.Fatal("key included in portable snapshot")
		}
		if f.Path == "MultiServer/addon-update-sources.json" {
			found = true
		}
	}
	if !found {
		t.Fatal("source metadata omitted")
	}
}

func TestRetainedVersionCatalogRebased(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(p, []byte(`{"packs":[{"installed_path":"old/current"}],"versions":[{"installed_path":"old/retained"}],"enabled_versions":{"creative":{"id":[1,0,0]}}}`), 0600)
	if e := rebasePackState(p, func(v string) (string, error) { return "new/" + v, nil }); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "new/old/retained") || !strings.Contains(string(b), "enabled_versions") {
		t.Fatalf("catalog lost: %s", b)
	}
}
