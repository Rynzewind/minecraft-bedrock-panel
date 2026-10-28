package accountstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testProtector struct{ key byte }

func (p testProtector) Protect(b []byte) ([]byte, error) {
	out := append([]byte(nil), b...)
	for i := range out {
		out[i] ^= p.key
	}
	return out, nil
}
func (p testProtector) Unprotect(b []byte) ([]byte, error) { return p.Protect(b) }

func TestStoreEncryptsTokensAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	s := New(path, testProtector{0xaa})
	want := Snapshot{Version: CurrentVersion, Accounts: []Account{{ID: "a", PersonLabel: "Kevin", Gamertag: "Helper", XUID: "123", EncryptedToken: "secret-refresh"}}}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if bytes.Contains(raw, []byte("secret-refresh")) {
		t.Fatal("plaintext token persisted")
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Accounts[0].EncryptedToken != want.Accounts[0].EncryptedToken {
		t.Fatalf("got %+v", got)
	}
}

func TestStoreFailedInstallPreservesPreviousSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	s := New(path, testProtector{1})
	if err := s.Save(Snapshot{Version: CurrentVersion, Accounts: []Account{{ID: "old"}}}); err != nil {
		t.Fatal(err)
	}
	s.rename = func(_, _ string) error { return errors.New("injected rename failure") }
	if err := s.Save(Snapshot{Version: CurrentVersion, Accounts: []Account{{ID: "new"}}}); err == nil {
		t.Fatal("expected failure")
	}
	s.rename = replaceFile
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Accounts[0].ID != "old" {
		t.Fatalf("previous snapshot lost: %+v", got)
	}
}

func TestDPAPIRoundTrip(t *testing.T) {
	p := DPAPIProtector{Entropy: []byte("NetherNetMultiServer/v1")}
	enc, err := p.Protect([]byte("refresh-token-value"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "refresh-token-value") {
		t.Fatal("DPAPI output contains plaintext")
	}
	got, err := p.Unprotect(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "refresh-token-value" {
		t.Fatalf("got %q", got)
	}
}
