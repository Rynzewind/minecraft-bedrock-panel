package addonupdates

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestProviderUsesBedrockGameAndFiltersStable(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "secret-key" {
			t.Error("missing API authentication")
		}
		switch r.URL.Path {
		case "/v1/games":
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": 99, "slug": "minecraft-bedrock"}}, "pagination": map[string]int{"totalCount": 1}})
		case "/v1/mods/42":
			w.Write([]byte(`{"data":{"id":42,"gameId":99,"slug":"pack","allowModDistribution":true}}`))
		case "/v1/mods/42/files":
			w.Write([]byte(`{"data":[{"id":2,"modId":42,"gameId":99,"isAvailable":true,"fileStatus":4,"releaseType":2},{"id":1,"modId":42,"gameId":99,"isAvailable":true,"fileStatus":4,"releaseType":1}],"pagination":{"totalCount":2}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	p := newProvider("secret-key", s.URL, s.Client(), s.Client())
	project, files, e := p.Check(context.Background(), Source{Provider: "curseforge", ProjectID: 42, Channel: "stable"})
	if e != nil {
		t.Fatal(e)
	}
	if project.ID != 42 || len(files) != 1 || files[0].ID != 1 {
		t.Fatalf("wrong result: %+v %+v", project, files)
	}
}
func TestProviderErrorsDoNotLeakResponseOrKey(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); w.Write([]byte("secret-key")) }))
	defer s.Close()
	p := newProvider("secret-key", s.URL, s.Client(), s.Client())
	e := p.Test(context.Background())
	if e == nil || e.Error() != "CurseForge rejected the API key (401)" {
		t.Fatalf("unsafe error: %v", e)
	}
}

func TestDownloadVerifiesBytesAndKeepsKeyOffCDN(t *testing.T) {
	data := []byte("archive bytes")
	hash := sha1.Sum(data)
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Error("API key leaked to CDN")
		}
		w.Write(data)
	}))
	defer cdn.Close()
	p := newProvider("secret-key", cdn.URL, cdn.Client(), cdn.Client())
	f := ReleaseFile{ID: 1, ModID: 42, IsAvailable: true, FileLength: int64(len(data)), DownloadURL: cdn.URL, Hashes: []FileHash{{Value: hex.EncodeToString(hash[:]), Algo: 1}}}
	project := Project{ID: 42}
	path, e := p.Download(context.Background(), project, f, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(path)
	f.Hashes[0].Value = "bad"
	if _, e = p.Download(context.Background(), project, f, t.TempDir()); e == nil {
		t.Fatal("accepted checksum mismatch")
	}
	f.FileLength--
	if _, e = p.Download(context.Background(), project, f, t.TempDir()); e == nil {
		t.Fatal("accepted wrong size")
	}
	allow := false
	project.AllowModDistribution = &allow
	if _, e = p.Download(context.Background(), project, f, t.TempDir()); e == nil {
		t.Fatal("ignored author distribution policy")
	}
}
func TestProductionClientsKeepCredentialRedirectsClosedAndConstrainCDNRedirects(t *testing.T) {
	p := NewProvider("secret-key")
	if e := p.api.CheckRedirect(&http.Request{}, nil); e == nil {
		t.Fatal("API redirect allowed")
	}
	approved, _ := http.NewRequest("GET", "https://edge.forgecdn.net/files/example.mcaddon", nil)
	if e := p.cdn.CheckRedirect(approved, []*http.Request{approved}); e != nil {
		t.Fatalf("approved CDN redirect refused: %v", e)
	}
	unapproved, _ := http.NewRequest("GET", "https://example.com/file.zip", nil)
	if e := p.cdn.CheckRedirect(unapproved, []*http.Request{approved}); e == nil {
		t.Fatal("unapproved CDN redirect allowed")
	}
	if _, e := p.Download(context.Background(), Project{ID: 42}, ReleaseFile{ID: 1, ModID: 42, IsAvailable: true, FileLength: 1, DownloadURL: "https://example.com/file.zip"}, t.TempDir()); e == nil {
		t.Fatal("unapproved CDN allowed")
	}
}

func TestProviderAcceptsLegacyFileGameIDAfterProjectValidation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/games":
			w.Write([]byte(`{"data":[{"id":99,"slug":"minecraft-bedrock"}],"pagination":{"totalCount":1}}`))
		case "/v1/mods/42":
			w.Write([]byte(`{"data":{"id":42,"gameId":99,"allowModDistribution":true}}`))
		case "/v1/mods/42/files":
			w.Write([]byte(`{"data":[{"id":1,"modId":42,"gameId":0,"isAvailable":true,"fileStatus":4,"releaseType":1}],"pagination":{"totalCount":1}}`))
		}
	}))
	defer s.Close()
	p := newProvider("secret-key", s.URL, s.Client(), s.Client())
	_, files, e := p.Check(context.Background(), Source{Provider: "curseforge", ProjectID: 42, Channel: "stable"})
	if e != nil || len(files) != 1 {
		t.Fatalf("legacy file game ID was rejected: %v %+v", e, files)
	}
}
func TestSourceResultDoesNotOverwriteEditedSource(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "sources.json"))
	old := Source{URL: "https://example.com/old", Provider: "manual", Channel: "stable"}
	s.SaveRecord("pack", old)
	s.Set("pack", "https://example.com/new")
	if e := s.MarkResult("pack", old, "checked"); e == nil {
		t.Fatal("stale check overwrote source")
	}
	rows, _ := s.ListRecords()
	if rows["pack"].URL != "https://example.com/new" {
		t.Fatal("lost source edit")
	}
}

func TestProviderPaginationAndWrongGame(t *testing.T) {
	pages := 0
	wrong := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/games":
			w.Write([]byte(`{"data":[{"id":99,"slug":"minecraft-bedrock"}],"pagination":{"totalCount":1}}`))
		case "/v1/mods/42":
			game := 99
			if wrong {
				game = 1
			}
			fmt.Fprintf(w, `{"data":{"id":42,"gameId":%d}}`, game)
		case "/v1/mods/42/files":
			pages++
			index, _ := strconv.Atoi(r.URL.Query().Get("index"))
			files := []ReleaseFile{}
			for i := 0; i < 50; i++ {
				files = append(files, ReleaseFile{ID: int64(index + i + 1), ModID: 42, GameID: 99, IsAvailable: true, FileStatus: 4, ReleaseType: 1})
			}
			json.NewEncoder(w).Encode(map[string]any{"data": files, "pagination": map[string]int{"totalCount": 100}})
		}
	}))
	defer s.Close()
	p := newProvider("secret-key", s.URL, s.Client(), s.Client())
	_, files, e := p.Check(context.Background(), Source{Provider: "curseforge", ProjectID: 42, Channel: "stable"})
	if e != nil || len(files) != 100 || pages != 2 {
		t.Fatalf("pagination %d %d %v", pages, len(files), e)
	}
	wrong = true
	if _, _, e = p.Check(context.Background(), Source{Provider: "curseforge", ProjectID: 42}); e == nil {
		t.Fatal("Java project accepted")
	}
}
