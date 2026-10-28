package migration

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func fixtureArchive(t *testing.T, relative ...bool) string {
	t.Helper()
	root := fixture(t)
	packPath := filepath.Join(root, "MultiServer", "content-staging", "packs", "p1")
	if len(relative) > 0 && relative[0] {
		packPath = filepath.Join("content-staging", "packs", "p1")
	}
	put(t, root, "MultiServer/content-staging/state.json", `{"packs":[{"installed_path":`+quote(packPath)+`,"UUID":"p1"}],"pending":[]}`)
	files, e := BuildInventory(root, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	b, e := ExportAccounts(accountstore.Snapshot{Accounts: []accountstore.Account{{ID: "a1", XUID: "42", EncryptedToken: "SECRET"}}, Players: []accountstore.PlayerMapping{{ConsoleXUID: "24", AccountID: "a1"}}})
	if e != nil {
		t.Fatal(e)
	}
	put(t, root, "accounts-migration.json", string(b))
	f, e := fileEntry(root, filepath.Join(root, "accounts-migration.json"))
	if e != nil {
		t.Fatal(e)
	}
	files = append(files, f)
	out := filepath.Join(t.TempDir(), "snapshot.zip")
	if e = WriteArchive(context.Background(), root, out, Manifest{FormatVersion: 1, SourceRoot: root, Files: files}); e != nil {
		t.Fatal(e)
	}
	return out
}

func TestRebasesLegacyRelativeContentLibraryPaths(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "new")
	_, e := Restore(context.Background(), RestoreOptions{ArchivePath: fixtureArchive(t, true), Destination: dst, LANAddress: "10.42.8.9", BroadcastAddress: "10.42.15.255"})
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dst, "MultiServer", "content-staging", "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	var state struct {
		Packs []struct {
			InstalledPath string `json:"installed_path"`
		} `json:"packs"`
	}
	if e = json.Unmarshal(b, &state); e != nil {
		t.Fatal(e)
	}
	if state.Packs[0].InstalledPath != filepath.Join(dst, "MultiServer", "content-staging", "packs", "p1") {
		t.Fatal("relative library path was not resolved against old panel directory")
	}
}
func TestRebasesPathsAndNetworkAndRetainsNetworkIDs(t *testing.T) {
	archive := fixtureArchive(t)
	dst := filepath.Join(t.TempDir(), "different folder")
	r, e := Restore(context.Background(), RestoreOptions{ArchivePath: archive, Destination: dst, LANAddress: "10.42.8.9", BroadcastAddress: "10.42.8.255"})
	if e != nil {
		t.Fatal(e)
	}
	cfg, e := config.Load(filepath.Join(dst, "MultiServer", "config.json"))
	if e != nil {
		t.Fatal(e)
	}
	s := cfg.Servers[0]
	if cfg.InterfaceIP != "10.42.8.9" || cfg.Listen != "10.42.8.255:7551" || s.Backend != "10.42.8.9:19134" || s.NetworkID != "13468123134000001934" || s.Process.Executable != filepath.Join(dst, "Creative", "bedrock_server.exe") {
		t.Fatalf("bad config: %+v %+v", cfg, s)
	}
	if r.AccountsRequiringSignIn != 1 {
		t.Fatal(r)
	}
	wal, e := os.ReadFile(filepath.Join(dst, "Creative", "worlds", "World", "db", "000123.log"))
	if e != nil || string(wal) != "required-world-transactions" {
		t.Fatal("world transaction log was not restored intact", e)
	}
	if _, e = os.Stat(filepath.Join(dst, "migration-activation-required")); e != nil {
		t.Fatal("activation gate missing")
	}
	b, _ := os.ReadFile(filepath.Join(dst, "MultiServer", "content-staging", "state.json"))
	if !strings.Contains(string(b), strings.ReplaceAll(dst, `\`, `\\`)) {
		t.Fatal("pack path was not rebased")
	}
}
func TestFreshUserCanOpenAccountStore(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "new")
	_, e := Restore(context.Background(), RestoreOptions{ArchivePath: fixtureArchive(t), Destination: dst, LANAddress: "10.42.8.9", BroadcastAddress: "10.42.8.255"})
	if e != nil {
		t.Fatal(e)
	}
	store := accountstore.New(filepath.Join(dst, "MultiServer", "account-store", "accounts.json"), accountstore.DPAPIProtector{Entropy: []byte("NetherNetMultiServer/account-store/v1")})
	m, e := accounts.NewManager(store)
	if e != nil {
		t.Fatal(e)
	}
	s := m.Snapshot()
	if len(s.Accounts) != 1 || s.Accounts[0].Healthy || s.Accounts[0].EncryptedToken != "{}" || s.Players[0].AccountID != "a1" {
		t.Fatalf("bad restored accounts")
	}
}
func TestRejectsExistingDestinationAndBadNetwork(t *testing.T) {
	a := fixtureArchive(t)
	dst := t.TempDir()
	if _, e := Restore(context.Background(), RestoreOptions{ArchivePath: a, Destination: dst, LANAddress: "10.42.8.9", BroadcastAddress: "10.42.8.255"}); e == nil {
		t.Fatal("accepted existing destination")
	}
	dst = filepath.Join(t.TempDir(), "new")
	if _, e := Restore(context.Background(), RestoreOptions{ArchivePath: a, Destination: dst, LANAddress: "0.0.0.0", BroadcastAddress: "10.42.8.255"}); e == nil {
		t.Fatal("accepted wildcard")
	}
	if _, e := os.Stat(dst); !os.IsNotExist(e) {
		t.Fatal("created destination on invalid input")
	}
}
func TestCancelledRestoreLeavesNoInstallation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dst := filepath.Join(t.TempDir(), "new")
	if _, e := Restore(ctx, RestoreOptions{ArchivePath: fixtureArchive(t), Destination: dst, LANAddress: "10.42.8.9", BroadcastAddress: "10.42.8.255"}); e == nil {
		t.Fatal("ignored cancel")
	}
	if _, e := os.Stat(dst); !os.IsNotExist(e) {
		t.Fatal("published cancelled restore")
	}
}
