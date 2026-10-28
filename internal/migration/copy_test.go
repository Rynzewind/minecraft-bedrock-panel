package migration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyHashesTheStagedWorldBytes(t *testing.T) {
	root := t.TempDir()
	put(t, root, "source", "hello")
	dst := filepath.Join(t.TempDir(), "world", "staged")
	f, e := copyFile(context.Background(), filepath.Join(root, "source"), dst)
	if e != nil {
		t.Fatal(e)
	}
	if f.Size != 5 || f.SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("incorrect captured hash: %+v", f)
	}
	b, e := os.ReadFile(dst)
	if e != nil || string(b) != "hello" {
		t.Fatal("staged bytes differ", e)
	}
}
