package addonupdates

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Project struct {
	ID                   int64 `json:"id"`
	GameID               int64 `json:"gameId"`
	Name, Slug           string
	AllowModDistribution *bool `json:"allowModDistribution"`
}
type FileHash struct {
	Value string
	Algo  int
}
type ReleaseFile struct {
	ID          int64      `json:"id"`
	ModID       int64      `json:"modId"`
	GameID      int64      `json:"gameId"`
	DisplayName string     `json:"displayName"`
	FileName    string     `json:"fileName"`
	FileDate    time.Time  `json:"fileDate"`
	ReleaseType int        `json:"releaseType"`
	FileStatus  int        `json:"fileStatus"`
	IsAvailable bool       `json:"isAvailable"`
	FileLength  int64      `json:"fileLength"`
	Hashes      []FileHash `json:"hashes"`
	DownloadURL string     `json:"downloadUrl"`
}
type Provider struct {
	key, base string
	api, cdn  *http.Client
	test      bool
}

func NewProvider(key string) *Provider {
	redirect := func(r *http.Request, via []*http.Request) error { return fmt.Errorf("redirect refused") }
	return newProvider(key, "https://api.curseforge.com", &http.Client{Timeout: 30 * time.Second, CheckRedirect: redirect}, &http.Client{Timeout: 5 * time.Minute, CheckRedirect: redirect})
}
func newProvider(key, base string, api, cdn *http.Client) *Provider {
	return &Provider{key: key, base: base, api: api, cdn: cdn, test: base != "https://api.curseforge.com"}
}
func (p *Provider) get(ctx context.Context, path string, out any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", p.base+path, nil)
	if e != nil {
		return fmt.Errorf("invalid provider request")
	}
	req.Header.Set("x-api-key", p.key)
	req.Header.Set("Accept", "application/json")
	resp, e := p.api.Do(req)
	if e != nil {
		return fmt.Errorf("CurseForge request failed; check connectivity")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		switch resp.StatusCode {
		case 401:
			return fmt.Errorf("CurseForge rejected the API key (401)")
		case 403:
			return fmt.Errorf("CurseForge access restricted (403); use the source page")
		case 404:
			return fmt.Errorf("CurseForge project or file not found (404)")
		case 429:
			return fmt.Errorf("CurseForge rate limit reached; try later (429)")
		default:
			return fmt.Errorf("CurseForge request failed (HTTP %d)", resp.StatusCode)
		}
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if e != nil || len(b) > 8<<20 {
		return fmt.Errorf("CurseForge response exceeds limit or could not be read")
	}
	if json.Unmarshal(b, out) != nil {
		return fmt.Errorf("CurseForge returned invalid metadata")
	}
	return nil
}

type page[T any] struct {
	Data       []T
	Pagination struct {
		TotalCount int `json:"totalCount"`
	}
}

func (p *Provider) game(ctx context.Context) (int64, error) {
	for index := 0; index < 10000; index += 50 {
		var out page[struct {
			ID         int64
			Slug, Name string
		}]
		if e := p.get(ctx, fmt.Sprintf("/v1/games?index=%d&pageSize=50", index), &out); e != nil {
			return 0, e
		}
		for _, g := range out.Data {
			if g.Slug == "minecraft-bedrock" {
				return g.ID, nil
			}
		}
		if len(out.Data) == 0 || index+len(out.Data) >= out.Pagination.TotalCount {
			break
		}
	}
	return 0, fmt.Errorf("CurseForge Bedrock game was not found; use manual source downloads")
}
func (p *Provider) Test(ctx context.Context) error { _, e := p.game(ctx); return e }
func (p *Provider) Check(ctx context.Context, s Source) (Project, []ReleaseFile, error) {
	if s.Provider != "curseforge" {
		return Project{}, nil, fmt.Errorf("select a CurseForge Bedrock source")
	}
	game, e := p.game(ctx)
	if e != nil {
		return Project{}, nil, e
	}
	var project Project
	if s.ProjectID > 0 {
		var out struct{ Data Project }
		e = p.get(ctx, fmt.Sprintf("/v1/mods/%d", s.ProjectID), &out)
		project = out.Data
	} else {
		u, err := url.Parse(s.URL)
		if err != nil {
			return project, nil, fmt.Errorf("invalid source")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 3 || parts[0] != "minecraft-bedrock" {
			return project, nil, fmt.Errorf("enter a Bedrock project URL or numeric project ID")
		}
		slug := parts[2]
		var out page[Project]
		e = p.get(ctx, fmt.Sprintf("/v1/mods/search?gameId=%d&slug=%s", game, url.QueryEscape(slug)), &out)
		matches := 0
		for _, v := range out.Data {
			if v.Slug == slug && v.GameID == game {
				project = v
				matches++
			}
		}
		if e == nil && matches != 1 {
			e = fmt.Errorf("project source did not resolve uniquely; enter the numeric project ID")
		}
	}
	if e != nil {
		return project, nil, e
	}
	if project.ID <= 0 || project.GameID != game {
		return project, nil, fmt.Errorf("project is not a Minecraft Bedrock add-on")
	}
	maxChannel := 1
	switch s.Channel {
	case "", "stable":
	case "beta":
		maxChannel = 2
	case "alpha":
		maxChannel = 3
	default:
		return project, nil, fmt.Errorf("unknown release channel")
	}
	var files []ReleaseFile
	for index := 0; index < 10000; index += 50 {
		var out page[ReleaseFile]
		if e = p.get(ctx, fmt.Sprintf("/v1/mods/%d/files?index=%d&pageSize=50", project.ID, index), &out); e != nil {
			return project, nil, e
		}
		for _, f := range out.Data {
			if f.GameID == game && f.ModID == project.ID && f.IsAvailable && (f.FileStatus == 4 || f.FileStatus == 10) && f.ReleaseType >= 1 && f.ReleaseType <= maxChannel {
				files = append(files, f)
			}
		}
		if len(out.Data) == 0 || index+len(out.Data) >= out.Pagination.TotalCount {
			break
		}
		if index == 9950 {
			return project, nil, fmt.Errorf("file listing exceeds API pagination limit; use source page")
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].FileDate.After(files[j].FileDate) })
	if len(files) > 100 {
		files = files[:100]
	}
	return project, files, nil
}
func (p *Provider) Download(ctx context.Context, project Project, f ReleaseFile, dir string) (string, error) {
	if project.AllowModDistribution != nil && !*project.AllowModDistribution {
		return "", fmt.Errorf("author disabled third-party downloads; use Open source")
	}
	if f.ModID != project.ID || !f.IsAvailable || f.FileLength <= 0 || f.FileLength > 256<<20 {
		return "", fmt.Errorf("release is unavailable or exceeds archive limit")
	}
	raw := f.DownloadURL
	if raw == "" {
		var out struct{ Data string }
		if e := p.get(ctx, fmt.Sprintf("/v1/mods/%d/files/%d/download-url", project.ID, f.ID), &out); e != nil {
			return "", e
		}
		raw = out.Data
	}
	if raw == "" {
		return "", fmt.Errorf("author download is restricted; use Open source")
	}
	u, e := url.Parse(raw)
	if e != nil || u.User != nil {
		return "", fmt.Errorf("invalid download URL")
	}
	host := strings.ToLower(u.Hostname())
	if !p.test && (u.Scheme != "https" || u.Port() != "" || !(host == "forgecdn.net" || strings.HasSuffix(host, ".forgecdn.net"))) {
		return "", fmt.Errorf("download host is not an approved CurseForge CDN; use source page")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return "", fmt.Errorf("invalid download request")
	}
	// Deliberately separate client and request: API credentials never reach a download host.
	resp, e := p.cdn.Do(req)
	if e != nil {
		return "", fmt.Errorf("add-on download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("add-on download failed (HTTP %d)", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, f.FileLength+1))
	if e != nil || int64(len(b)) != f.FileLength {
		return "", fmt.Errorf("download length does not match release metadata")
	}
	verified := false
	for _, h := range f.Hashes {
		var actual string
		switch h.Algo {
		case 1:
			v := sha1.Sum(b)
			actual = hex.EncodeToString(v[:])
		case 2:
			v := md5.Sum(b)
			actual = hex.EncodeToString(v[:])
		default:
			continue
		}
		if !strings.EqualFold(actual, h.Value) {
			return "", fmt.Errorf("download checksum mismatch")
		}
		verified = true
	}
	if !verified {
		return "", fmt.Errorf("release has no supported checksum; download manually and review")
	}
	if e = privateDirectory(dir); e != nil {
		return "", fmt.Errorf("cannot protect download staging")
	}
	out, e := os.CreateTemp(dir, "curseforge-*.zip")
	if e != nil {
		return "", e
	}
	path := out.Name()
	if _, e = out.Write(b); e == nil {
		e = out.Sync()
	}
	closeErr := out.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		os.Remove(path)
		return "", fmt.Errorf("cannot save downloaded archive")
	}
	return filepath.Clean(path), nil
}
