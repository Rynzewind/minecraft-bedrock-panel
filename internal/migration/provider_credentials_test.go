package migration

import (
	"os"
	"path/filepath"
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
