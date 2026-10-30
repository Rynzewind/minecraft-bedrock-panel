package addonupdates

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIdentifyImportRequiresExactVerifiedBedrockDownload(t *testing.T) {
	for _, scenario := range []string{"origin", "fingerprint", "changed", "wrong-game", "partial-only", "hash-missing", "ambiguous", "wrong-link"} {
		t.Run(scenario, func(t *testing.T) {
			data := []byte("known archive bytes")
			sum := sha1.Sum(data)
			hashes := []FileHash{{Algo: 1, Value: hex.EncodeToString(sum[:])}}
			if scenario == "hash-missing" {
				hashes = nil
			}
			file := ReleaseFile{ID: 7, ModID: 42, FileLength: int64(len(data)), FileStatus: 4, IsAvailable: true, Hashes: hashes}
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-api-key") != "fixture-key" {
					t.Error("missing API credential")
				}
				switch r.URL.Path {
				case "/v1/games":
					io.WriteString(w, `{"data":[{"id":99,"slug":"minecraft-bedrock"}],"pagination":{"totalCount":1}}`)
				case "/v1/mods/files":
					if r.Method != "POST" {
						t.Error("wrong file lookup method")
					}
					json.NewEncoder(w).Encode(map[string]any{"data": []ReleaseFile{file}})
				case "/v1/fingerprints/99":
					body, _ := io.ReadAll(r.Body)
					if strings.Contains(string(body), "known archive") {
						t.Error("archive contents uploaded")
					}
					if r.Method != "POST" {
						t.Error("wrong fingerprint method")
					}
					matches := []any{map[string]any{"id": 42, "file": file}}
					if scenario == "partial-only" {
						matches = nil
					}
					if scenario == "ambiguous" {
						other := file
						other.ID = 8
						other.ModID = 43
						matches = append(matches, map[string]any{"id": 43, "file": other})
					}
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"exactMatches": matches, "partialMatches": []any{map[string]any{"id": 42, "file": file}}}})
				case "/v1/mods/42":
					game := 99
					if scenario == "wrong-game" {
						game = 432
					}
					link := "https://www.curseforge.com/minecraft-bedrock/addons/example"
					if scenario == "wrong-link" {
						link = "https://attacker.example/example"
					}
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 42, "gameId": game, "slug": "example", "name": "Example", "links": map[string]any{"websiteUrl": link}}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer api.Close()
			p := newProvider("fixture-key", api.URL, api.Client(), api.Client())
			hint := int64(0)
			if scenario == "origin" {
				hint = 7
			}
			if scenario == "changed" {
				data[0] = 'X'
			}
			project, file, err := p.IdentifyDownload(context.Background(), data, hint)
			success := scenario == "origin" || scenario == "fingerprint"
			if success && (err != nil || project.ID != 42 || file.ID != 7 || project.Website() == "") {
				t.Fatalf("match failed: %+v %v", project, err)
			}
			if !success && err == nil {
				t.Fatal("unsafe or ambiguous source accepted")
			}
		})
	}
}
func TestDownloadOriginIsOnlyAConstrainedHint(t *testing.T) {
	cases := map[string]int64{"https://mediafilez.forgecdn.net/files/8601/53/DynamicLight.mcaddon": 8601053, "https://edge.forgecdn.net/files/8601/053/a.mcaddon?token=not-retained": 8601053, "http://mediafilez.forgecdn.net/files/8601/53/a": 0, "https://forgecdn.net.attacker.example/files/8601/53/a": 0, "https://user@edge.forgecdn.net/files/8601/53/a": 0, "https://edge.forgecdn.net/files/8601/1000/a": 0, "https://edge.forgecdn.net/files/-1/53/a": 0}
	for raw, want := range cases {
		if got := downloadFileID(raw); got != want {
			t.Fatalf("origin %q: %d != %d", raw, got, want)
		}
	}
}
func TestFingerprintNormalizesCurseForgeWhitespace(t *testing.T) {
	if fileFingerprint([]byte("a b\nc\rd\te")) != fileFingerprint([]byte("abcde")) {
		t.Fatal("normalization differs")
	}
}

func TestNameSearchLinksOnlyChecksumIdenticalArchives(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			data := []byte("exact archive")
			sum := sha1.Sum(data)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/games":
					io.WriteString(w, `{"data":[{"id":99,"slug":"minecraft-bedrock"}],"pagination":{"totalCount":1}}`)
				case "/v1/mods/search":
					if r.URL.Query().Get("searchFilter") != "Example" {
						t.Error("display name not normalized")
					}
					file := ReleaseFile{ID: 7, ModID: 42, FileLength: int64(len(data)), FileStatus: 4, IsAvailable: true, Hashes: []FileHash{{Algo: 1, Value: hex.EncodeToString(sum[:])}}}
					project := Project{ID: 42, GameID: 99, Slug: "example", LatestFiles: []ReleaseFile{file}}
					project.Links.WebsiteURL = "https://www.curseforge.com/minecraft-bedrock/addons/example"
					json.NewEncoder(w).Encode(map[string]any{"data": []Project{project}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer api.Close()
			p := newProvider("fixture-key", api.URL, api.Client(), api.Client())
			if changed {
				data[0] = 'X'
			}
			project, _, err := p.IdentifyByNames(context.Background(), data, []string{"Example v1.2 [BP]"})
			if !changed && (err != nil || project.ID != 42) {
				t.Fatalf("match failed: %v", err)
			}
			if changed && err == nil {
				t.Fatal("name-only match accepted modified bytes")
			}
		})
	}
}
