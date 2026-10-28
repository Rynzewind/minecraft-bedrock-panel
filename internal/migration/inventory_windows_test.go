package migration

import (
	"golang.org/x/sys/windows"
	"path/filepath"
	"testing"
)

func TestPreflightSizesWorldsWithoutOpeningTheirContents(t *testing.T) {
	root := fixture(t)
	p, _ := windows.UTF16PtrFromString(filepath.Join(root, "Creative", "worlds", "World", "db", "CURRENT"))
	handle, e := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer windows.CloseHandle(handle)
	files, e := inventoryForSizing(root, t.TempDir())
	if e != nil {
		t.Fatalf("preflight unnecessarily reads world contents: %v", e)
	}
	found := false
	for _, f := range files {
		if f.Path == "Creative/worlds/World/db/CURRENT" && f.Size == 4 {
			found = true
		}
	}
	if !found {
		t.Fatal("preflight did not size world file")
	}
}
