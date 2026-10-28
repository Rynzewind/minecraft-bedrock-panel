package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreatePersistsIdentity(t *testing.T) {
	store := Store{Directory: t.TempDir()}
	first, firstFingerprint, err := store.LoadOrCreate(13468123134000001934)
	if err != nil {
		t.Fatal(err)
	}
	second, secondFingerprint, err := store.LoadOrCreate(13468123134000001934)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint != secondFingerprint || first.D.Cmp(second.D) != 0 {
		t.Fatal("identity changed after reload")
	}
}

func TestLoadOrCreateSeparatesNetworkIDs(t *testing.T) {
	store := Store{Directory: t.TempDir()}
	_, first, err := store.LoadOrCreate(1)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := store.LoadOrCreate(2)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("different network IDs reused an identity")
	}
}

func TestLoadOrCreateRejectsCorruptKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "42.pem"), []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (Store{Directory: dir}).LoadOrCreate(42); err == nil {
		t.Fatal("corrupt key accepted")
	}
}

func TestPathUsesDecimalNetworkID(t *testing.T) {
	if got, want := Path(`C:\ids`, 42), filepath.Join(`C:\ids`, "42.pem"); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}
