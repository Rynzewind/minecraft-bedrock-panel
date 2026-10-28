package addonupdates

import (
	"path/filepath"
	"testing"
)

func TestSourcesPersistAndRejectUnsafeURLs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.json")
	s := New(path)
	for _, u := range []string{"javascript:alert(1)", "file:///C:/secret", "https://user:password@example.com/a"} {
		if e := s.Set("pack", u); e == nil {
			t.Fatal(u)
		}
	}
	if e := s.Set("pack", "https://www.curseforge.com/minecraft-bedrock/addons/example"); e != nil {
		t.Fatal(e)
	}
	if New(path).List()["pack"] == "" {
		t.Fatal("source not saved")
	}
	if e := s.Set("pack", ""); e != nil || len(New(path).List()) != 0 {
		t.Fatal("source removal failed")
	}
}
