package migration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreSynchronisesDistinctServerIP(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "new")
	_, e := Restore(context.Background(), RestoreOptions{ArchivePath: fixtureArchive(t), Destination: dst, LANAddress: "10.42.8.9", BroadcastAddress: "10.42.15.255", ServerAddresses: map[string]string{"creative": "10.42.9.246"}})
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dst, "Creative", "server.properties"))
	if e != nil || !strings.Contains(string(b), "server-ip=10.42.9.246") {
		t.Fatal("old binding survived", e, string(b))
	}
}
func TestSnapshotRetainsPlayerPermissionState(t *testing.T) {
	root := fixture(t)
	put(t, root, "MultiServer/player-permissions/roles.json", `{"42":"operator"}`)
	entries, e := BuildInventory(root, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range entries {
		if f.Path == "MultiServer/player-permissions/roles.json" {
			return
		}
	}
	t.Fatal("player permission state omitted")
}
