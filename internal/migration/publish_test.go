package migration

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishArchiveCopiesAndChecksBytes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "destination")
	payload := bytes.Repeat([]byte("archive bytes"), 10000)
	os.WriteFile(src, payload, 0600)
	if err := copyVerifiedArchive(context.Background(), src, dst, nil); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(dst)
	if !bytes.Equal(actual, payload) {
		t.Fatal("published bytes changed")
	}
}
func TestPublishArchivePreservesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "destination")
	os.WriteFile(src, []byte("new"), 0600)
	os.WriteFile(dst, []byte("existing"), 0600)
	if copyVerifiedArchive(context.Background(), src, dst, nil) == nil {
		t.Fatal("overwrote destination")
	}
	actual, _ := os.ReadFile(dst)
	if string(actual) != "existing" {
		t.Fatal("existing destination changed")
	}
}
func TestPublishArchiveRejectsCorruptedCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	dst := filepath.Join(dir, "destination")
	os.WriteFile(src, []byte("original bytes"), 0600)
	err := copyVerifiedArchive(context.Background(), src, dst, func(_, _ int64) {
		if e := os.WriteFile(dst, []byte("corrupted"), 0600); e != nil {
			t.Fatal(e)
		}
	})
	if err == nil {
		t.Fatal("corruption not detected")
	}
	if _, err = os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("corrupted partial remains")
	}
}
