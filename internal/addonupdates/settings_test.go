package addonupdates

import (
	"encoding/json"
	"nethernet-multi-server/internal/accountstore"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type xorProtector struct{}

func (xorProtector) Protect(b []byte) ([]byte, error) {
	v := append([]byte(nil), b...)
	for i := range v {
		v[i] ^= 0xA5
	}
	return v, nil
}
func (xorProtector) Unprotect(b []byte) ([]byte, error) { return xorProtector{}.Protect(b) }
func TestLegacySourcesMigrateWithoutLosingLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.json")
	os.WriteFile(path, []byte(`{"pack":"https://www.curseforge.com/minecraft-bedrock/addons/example"}`), 0600)
	s := New(path)
	rows, e := s.ListRecords()
	if e != nil || rows["pack"].URL == "" {
		t.Fatalf("%+v %v", rows, e)
	}
	r := rows["pack"]
	r.Notes = "Reviewed manually"
	if e = s.SaveRecord("pack", r); e != nil {
		t.Fatal(e)
	}
	again, e := New(path).ListRecords()
	if e != nil || again["pack"].Notes != r.Notes {
		t.Fatal(again, e)
	}
	os.WriteFile(path, []byte("broken"), 0600)
	if _, e = New(path).ListRecords(); e == nil {
		t.Fatal("corrupt state shown as empty")
	}
}
func TestCredentialsEncryptedAndStatusNeverContainsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "key.json")
	c := NewCredentials(path, xorProtector{})
	key := "test-curseforge-key-$long-value"
	if e := c.Set(key); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), key) {
		t.Fatal("plaintext key persisted")
	}
	loaded, e := c.Load()
	if e != nil || loaded != key {
		t.Fatal("cannot read encrypted key")
	}
	status, e := c.Status()
	if e != nil || !status.Configured || status.Valid {
		t.Fatal(status, e)
	}
	j, _ := json.Marshal(status)
	if strings.Contains(string(j), key) {
		t.Fatal("key returned in public status")
	}
	if e = c.Set("bad\r\nheader"); e == nil {
		t.Fatal("header injection accepted")
	}
	if e = c.Remove(); e != nil {
		t.Fatal(e)
	}
	status, _ = c.Status()
	if status.Configured {
		t.Fatal("key not removed")
	}
}
func TestDPAPIKeyRoundTrip(t *testing.T) {
	c := NewCredentials(filepath.Join(t.TempDir(), "private", "curseforge.json"), accountstore.DPAPIProtector{Entropy: []byte("NetherNet.CurseForge.v1")})
	if e := c.Set("test-key-only-not-a-real-credential"); e != nil {
		t.Fatal(e)
	}
	if v, e := c.Load(); e != nil || v != "test-key-only-not-a-real-credential" {
		t.Fatal("DPAPI roundtrip failed", e)
	}
}
