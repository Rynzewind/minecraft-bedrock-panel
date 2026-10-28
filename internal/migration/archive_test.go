package migration

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/config"
)

func put(t *testing.T, root, name, value string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cfg := config.Config{InterfaceIP: "192.168.1.137", Listen: "192.168.1.255:7551", Servers: []config.Server{{ID: "creative", Name: "Creative", LevelName: "World", Backend: "192.168.1.137:19134", NetworkID: "13468123134000001934", Protocol: 2193, Version: "1.26.51", GameType: "creative", MaxPlayerCount: 10, ConnectionType: 4, Process: &config.Process{Executable: filepath.Join(root, "Creative", "bedrock_server.exe"), WorkingDirectory: filepath.Join(root, "Creative"), WorldDirectory: filepath.Join(root, "Creative", "worlds", "World"), LogDirectory: filepath.Join(root, "Creative", "logs")}}}}
	b, _ := json.Marshal(cfg)
	put(t, root, "MultiServer/config.json", string(b))
	for _, p := range []string{"Creative/bedrock_server.exe", "Creative/worlds/World/db/CURRENT", "Creative/resource_packs/a/manifest.json", "Creative/permissions.json", "MultiServer/identities/13468123134000001934.pem", "MultiServer/NetherNetMultiServer.exe", "MultiServer/NetherNetMultiServerAdmin.exe", "MultiServer/NetherNetMultiServerControl.exe"} {
		put(t, root, p, "kept")
	}
	put(t, root, "Creative/server.properties", "server-port=19134\n")
	put(t, root, "Creative/worlds/World/db/000123.log", "required-world-transactions")
	return root
}

func TestSnapshotRejectsMissingPersistentIdentity(t *testing.T) {
	root := fixture(t)
	if e := os.Remove(filepath.Join(root, "MultiServer", "identities", "13468123134000001934.pem")); e != nil {
		t.Fatal(e)
	}
	if _, e := BuildInventory(root, t.TempDir()); e == nil {
		t.Fatal("snapshot would silently change the server identity on restore")
	}
}
func TestInventoryExcludesTokensBackupsAndOutput(t *testing.T) {
	root := fixture(t)
	for _, p := range []string{"MultiServer/account-store/accounts.json", "MultiServer/auth-token.migrated-backup.json", "MultiServer/identities/old.bak", "MultiServer/backups/old.exe", "MultiServer-Candidate/a.exe", "Creative/logs/log.txt", "Creative/packet-statistics.txt"} {
		put(t, root, p, "SECRET-TOKEN")
	}
	files, err := BuildInventory(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(f.Path, "account-store") || strings.Contains(f.Path, "backup") || strings.Contains(f.Path, "logs/") || strings.Contains(f.Path, "Candidate") {
			t.Fatalf("included %s", f.Path)
		}
	}
	if len(files) < 8 {
		t.Fatalf("missing payload: %d", len(files))
	}
}

func TestWorldDatabaseLogsArePreserved(t *testing.T) {
	root := fixture(t)
	put(t, root, "Creative/worlds/World/db/000123.log", "required-world-transactions")
	put(t, root, "Creative/logs/diagnostic.log", "not needed")
	files, e := BuildInventory(root, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, f := range files {
		if f.Path == "Creative/worlds/World/db/000123.log" {
			found = true
		}
		if f.Path == "Creative/logs/diagnostic.log" {
			t.Fatal("included diagnostic log")
		}
	}
	if !found {
		t.Fatal("excluded required world database transaction log")
	}
}
func TestInventoryRejectsOverlapAndReparsePoints(t *testing.T) {
	root := fixture(t)
	if _, err := BuildInventory(root, filepath.Join(root, "snapshots")); err == nil {
		t.Fatal("accepted recursive output")
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "Creative", "linked")); err == nil {
		if _, err = BuildInventory(root, t.TempDir()); err == nil {
			t.Fatal("followed symlink")
		}
	}
}
func TestMetadataPreservesIDsMappingsAndRoles(t *testing.T) {
	src := accountstore.Snapshot{Version: 1, Accounts: []accountstore.Account{{ID: "a1", PersonLabel: "Me", Gamertag: "Game", XUID: "42", EncryptedToken: "SECRET-TOKEN", Healthy: true}}, Players: []accountstore.PlayerMapping{{ConsoleXUID: "24", AccountID: "a1"}}}
	b, err := ExportAccounts(src)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SECRET-TOKEN") || strings.Contains(string(b), "encrypted_token") {
		t.Fatal("exported credential")
	}
	dst, err := ImportAccounts(b)
	if err != nil {
		t.Fatal(err)
	}
	if dst.Accounts[0].ID != "a1" || dst.Accounts[0].Healthy || dst.Accounts[0].EncryptedToken != "{}" || dst.Players[0].AccountID != "a1" {
		t.Fatalf("wrong metadata: %+v", dst)
	}
}
func TestArchiveDetectsTampering(t *testing.T) {
	root := fixture(t)
	files, err := BuildInventory(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "snapshot.zip")
	if err = WriteArchive(context.Background(), root, out, Manifest{FormatVersion: 1, SourceRoot: root, Files: files}); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyArchive(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	offset, err := z.File[0].DataOffset()
	z.Close()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	raw[offset] ^= 0xff
	bad := filepath.Join(t.TempDir(), "corrupt.zip")
	if err = os.WriteFile(bad, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyArchive(context.Background(), bad); err == nil {
		t.Fatal("accepted corrupted payload")
	}
	put(t, root, "Creative/bedrock_server.exe", "tampered")
	if err = WriteArchive(context.Background(), root, filepath.Join(t.TempDir(), "bad.zip"), Manifest{FormatVersion: 1, SourceRoot: root, Files: files}); err == nil {
		t.Fatal("allowed changed file")
	}
}
func TestRejectsTraversalAbsoluteDuplicateAndCaseCollision(t *testing.T) {
	for _, names := range [][]string{{"../escape"}, {"C:/escape"}, {"a", "A"}, {"MultiServer/auth-token.json"}, {"CON.txt"}, {"a:stream"}, {"a. /b"}} {
		t.Run(strings.Join(names, "_"), func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "bad.zip")
			f, _ := os.Create(p)
			z := zip.NewWriter(f)
			m := Manifest{FormatVersion: 1, SourceRoot: t.TempDir()}
			for _, n := range names {
				m.Files = append(m.Files, FileEntry{Path: n, Size: 1, SHA256: "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"})
			}
			b, _ := json.Marshal(m)
			w, _ := z.Create("manifest.json")
			w.Write(b)
			for _, n := range names {
				w, _ = z.Create(n)
				w.Write([]byte("x"))
			}
			z.Close()
			f.Close()
			if _, err := VerifyArchive(context.Background(), p); err == nil {
				t.Fatal("accepted unsafe archive")
			}
		})
	}
}

func TestRejectsArchiveSymlinksAndOversizedExpansion(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "linked", Method: zip.Store}
	h.SetMode(os.ModeSymlink | 0600)
	p, _ := w.CreateHeader(h)
	p.Write([]byte("target"))
	w.Close()
	z, e := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = verifyZip(context.Background(), z); e == nil || !strings.Contains(e.Error(), "regular files") {
		t.Fatal("did not reject symlink", e)
	}
	z = &zip.Reader{File: []*zip.File{{FileHeader: zip.FileHeader{Name: "huge", UncompressedSize64: maxExpanded + 1}}}}
	if _, e = verifyZip(context.Background(), z); e == nil || !strings.Contains(e.Error(), "expansion limit") {
		t.Fatal("did not reject expansion", e)
	}
}
