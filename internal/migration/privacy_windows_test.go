package migration

import (
	"context"
	"golang.org/x/sys/windows"
	"path/filepath"
	"testing"
)

func TestSnapshotArchiveHasPrivateProtectedACL(t *testing.T) {
	root := fixture(t)
	files, e := BuildInventory(root, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "snapshot.zip")
	if e = WriteArchive(context.Background(), root, p, Manifest{FormatVersion: 1, SourceRoot: root, Files: files}); e != nil {
		t.Fatal(e)
	}
	sd, e := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	control, _, e := sd.Control()
	if e != nil {
		t.Fatal(e)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("archive inherits permissions from output folder")
	}
}
