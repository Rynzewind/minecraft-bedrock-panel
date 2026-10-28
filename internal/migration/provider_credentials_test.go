package migration

import (
	"encoding/json"
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

func TestOnlineRestoreAlwaysDisablesIntent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "online.json")
	os.WriteFile(p, []byte(`{"version":1,"settings":{"version":1,"servers":{"creative":{"enabled":true,"account_id":"b"}}},"players":[{"xuid":"123","gamertag":"fixture","permissions":{"creative":true}}]}`), 0600)
	if e := disableRestoredOnline(p); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	var state struct {
		Settings struct {
			Servers map[string]struct {
				Enabled   bool
				AccountID string `json:"account_id"`
			}
		}
		Players []any
	}
	if e := json.Unmarshal(b, &state); e != nil {
		t.Fatal(e)
	}
	if state.Settings.Servers["creative"].Enabled || state.Settings.Servers["creative"].AccountID != "b" || len(state.Players) != 1 {
		t.Fatalf("restore lost policy or enabled Online: %s", b)
	}
}

func TestOnlineRestoreRejectsNullServer(t *testing.T) {
	p := filepath.Join(t.TempDir(), "online.json")
	os.WriteFile(p, []byte(`{"version":1,"settings":{"version":1,"servers":{"creative":null}},"players":[]}`), 0600)
	if e := disableRestoredOnline(p); e == nil {
		t.Fatal("null server entry accepted")
	}
}
