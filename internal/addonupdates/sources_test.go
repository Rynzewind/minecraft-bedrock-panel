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

func TestProtectionInvalidatesInFlightVerification(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "sources.json"))
	before := Source{Provider: "curseforge", ProjectID: 1, Channel: "stable"}
	if err := store.SaveRecord("pack", before); err != nil {
		t.Fatal(err)
	}
	after := before
	after.LocallyModified = true
	if err := store.SaveRecord("pack", after); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkResult("pack", before, "late API result", 123); err == nil {
		t.Fatal("stale check ignored protection change")
	}
	if err := store.MarkVerified("pack", before, 123, [3]int{2, 0, 0}); err == nil {
		t.Fatal("protected pack became verified")
	}
}
