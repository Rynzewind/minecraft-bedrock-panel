package authstore

import (
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestSaveAndLoadToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth-token.json")
	want := &oauth2.Token{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer", Expiry: time.Now().UTC().Truncate(time.Second)}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken || !got.Expiry.Equal(want.Expiry) {
		t.Fatalf("token mismatch: got %#v, want %#v", got, want)
	}
}

func TestLoadMissingTokenReportsNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected missing token error")
	}
}
