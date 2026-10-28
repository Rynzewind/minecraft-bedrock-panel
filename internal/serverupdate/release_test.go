package serverupdate

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCompatibility(t *testing.T) {
	for _, v := range []string{"1.26.60.6", "1.26.60.7"} {
		r, e := parseRelease("https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-" + v + ".zip")
		if e != nil || !r.Compatible {
			t.Fatalf("%s: %+v %v", v, r, e)
		}
	}
	for _, u := range []string{"https://evil.test/bedrock-server-1.26.60.6.zip", "https://www.minecraft.net/bedrockdedicatedserver/bin-win-preview/bedrock-server-1.26.60.6.zip"} {
		if _, e := parseRelease(u); e == nil {
			t.Fatal(u)
		}
	}
	r, e := parseRelease("https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-1.26.70.1.zip")
	if e != nil || r.Compatible {
		t.Fatal("future version must be blocked")
	}
}
func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for n, s := range files {
		f, e := w.Create(n)
		if e != nil {
			t.Fatal(e)
		}
		f.Write([]byte(s))
	}
	w.Close()
	return b.Bytes()
}
func TestArchiveProtectsUserData(t *testing.T) {
	root := t.TempDir()
	data := archive(t, map[string]string{"bedrock_server.exe": "MZsoftware", "server.properties": "defaults", "permissions.json": "[]", "worlds/test/level.dat": "bad", "behavior_packs/vanilla/manifest.json": "builtin", "behavior_packs/custom/manifest.json": "stock"})
	dst := filepath.Join(root, "stage")
	if e := extractArchive(data, dst); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(dst, "server.properties")); !os.IsNotExist(e) {
		t.Fatal("mutable defaults staged")
	}
	if _, e := os.Stat(filepath.Join(dst, "worlds")); !os.IsNotExist(e) {
		t.Fatal("worlds staged")
	}
	if _, e := os.Stat(filepath.Join(dst, "bedrock_server.exe")); e != nil {
		t.Fatal(e)
	}
}
func TestArchiveRejectsTraversalAndDuplicate(t *testing.T) {
	for _, n := range []string{"../escape", "C:/escape", "dir/../../escape", "dir\\escape", "CON", "bedrock_server.exe:stream"} {
		e := extractArchive(archive(t, map[string]string{"bedrock_server.exe": "MZ", n: "bad"}), filepath.Join(t.TempDir(), "stage"))
		if e == nil {
			t.Fatal(n)
		}
	}
	e := extractArchive(archive(t, map[string]string{"bedrock_server.exe": "MZ", "BEDROCK_SERVER.EXE": "MZ"}), filepath.Join(t.TempDir(), "stage"))
	if e == nil || !strings.Contains(e.Error(), "duplicate") {
		t.Fatal(e)
	}
}
