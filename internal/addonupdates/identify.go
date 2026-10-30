package addonupdates

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type ImportSourceResult struct {
	Found                     bool
	ProjectID                 int64
	URL, ProjectName, Message string
	Linked, Preserved         int
}

// CurseForge fingerprints are MurmurHash2(seed 1) after removing four whitespace bytes.
// They find candidates only; supported cryptographic checksums must also match.
func fileFingerprint(data []byte) uint32 {
	normalized := make([]byte, 0, len(data))
	for _, b := range data {
		if b != 9 && b != 10 && b != 13 && b != 32 {
			normalized = append(normalized, b)
		}
	}
	const m uint32 = 0x5bd1e995
	h := uint32(1) ^ uint32(len(normalized))
	for len(normalized) >= 4 {
		k := binary.LittleEndian.Uint32(normalized)
		k *= m
		k ^= k >> 24
		k *= m
		h *= m
		h ^= k
		normalized = normalized[4:]
	}
	switch len(normalized) {
	case 3:
		h ^= uint32(normalized[2]) << 16
		fallthrough
	case 2:
		h ^= uint32(normalized[1]) << 8
		fallthrough
	case 1:
		h ^= uint32(normalized[0])
		h *= m
	}
	h ^= h >> 13
	h *= m
	h ^= h >> 15
	return h
}
func matchesImport(data []byte, f ReleaseFile) bool {
	if f.FileLength != int64(len(data)) {
		return false
	}
	verified := false
	for _, hash := range f.Hashes {
		actual := ""
		switch hash.Algo {
		case 1:
			sum := sha1.Sum(data)
			actual = hex.EncodeToString(sum[:])
		case 2:
			sum := md5.Sum(data)
			actual = hex.EncodeToString(sum[:])
		default:
			continue
		}
		if !strings.EqualFold(actual, hash.Value) {
			return false
		}
		verified = true
	}
	return verified
}
func projectWebsite(p Project) string {
	u, err := url.Parse(p.Links.WebsiteURL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || (u.Hostname() != "www.curseforge.com" && u.Hostname() != "curseforge.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "minecraft-bedrock" || (parts[1] != "addons" && parts[1] != "texture-packs") || p.Slug == "" || parts[2] != p.Slug {
		return ""
	}
	return u.String()
}
func (p *Provider) Identify(ctx context.Context, data []byte) (Project, ReleaseFile, error) {
	var zero Project
	var empty ReleaseFile
	game, err := p.game(ctx)
	if err != nil {
		return zero, empty, err
	}
	var result struct {
		Data struct {
			ExactMatches []struct {
				ID   int64
				File ReleaseFile
			}
		}
	}
	payload := []byte(fmt.Sprintf(`{"fingerprints":[%d]}`, fileFingerprint(data)))
	if err = p.request(ctx, http.MethodPost, fmt.Sprintf("/v1/fingerprints/%d", game), bytes.NewReader(payload), &result); err != nil {
		return zero, empty, err
	}
	var matched ReleaseFile
	for _, candidate := range result.Data.ExactMatches {
		f := candidate.File
		if candidate.ID != f.ModID || f.ID <= 0 || f.ModID <= 0 || !f.IsAvailable || (f.FileStatus != 4 && f.FileStatus != 10) || !matchesImport(data, f) {
			continue
		}
		if matched.ID != 0 && (matched.ID != f.ID || matched.ModID != f.ModID) {
			return zero, empty, fmt.Errorf("download matches more than one CurseForge file; choose its source manually")
		}
		matched = f
	}
	if matched.ID == 0 {
		return zero, empty, fmt.Errorf("no exact CurseForge download match; keep the source manual")
	}
	var project struct{ Data Project }
	if err = p.get(ctx, fmt.Sprintf("/v1/mods/%d", matched.ModID), &project); err != nil {
		return zero, empty, err
	}
	if project.Data.ID != matched.ModID || project.Data.GameID != game || projectWebsite(project.Data) == "" {
		return zero, empty, fmt.Errorf("matched project is not a verified Minecraft Bedrock source")
	}
	return project.Data, matched, nil
}
func (p Project) Website() string { return projectWebsite(p) }

// IdentifyDownload uses a browser download file ID when available, with exact
// archive verification. Missing provenance falls back to the fingerprint index.
func (p *Provider) IdentifyDownload(ctx context.Context, data []byte, fileID int64) (Project, ReleaseFile, error) {
	if fileID <= 0 {
		return p.Identify(ctx, data)
	}
	var empty Project
	var zero ReleaseFile
	game, err := p.game(ctx)
	if err != nil {
		return empty, zero, err
	}
	var files struct{ Data []ReleaseFile }
	payload := []byte(fmt.Sprintf(`{"fileIds":[%d]}`, fileID))
	if err = p.request(ctx, http.MethodPost, "/v1/mods/files", bytes.NewReader(payload), &files); err != nil {
		return empty, zero, err
	}
	for _, file := range files.Data {
		if file.ID != fileID || file.ModID <= 0 || !file.IsAvailable || (file.FileStatus != 4 && file.FileStatus != 10) || !matchesImport(data, file) {
			continue
		}
		var project struct{ Data Project }
		if err = p.get(ctx, fmt.Sprintf("/v1/mods/%d", file.ModID), &project); err != nil {
			return empty, zero, err
		}
		if project.Data.ID == file.ModID && project.Data.GameID == game && projectWebsite(project.Data) != "" {
			return project.Data, file, nil
		}
	}
	return p.Identify(ctx, data)
}

// Names find a small candidate list only. Byte length and all supported archive
// checksums, rather than the display name, decide whether a source may be linked.
func (p *Provider) IdentifyByNames(ctx context.Context, data []byte, names []string) (Project, ReleaseFile, error) {
	var matched Project
	var archive ReleaseFile
	game, err := p.game(ctx)
	if err != nil {
		return matched, archive, err
	}
	seen := map[string]bool{}
	queries := 0
	for _, raw := range names {
		name := cleanImportName(raw)
		if len(name) < 3 || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		queries++
		if queries > 3 {
			break
		}
		var out struct{ Data []Project }
		if err = p.get(ctx, fmt.Sprintf("/v1/mods/search?gameId=%d&searchFilter=%s&pageSize=10&sortField=2&sortOrder=desc", game, url.QueryEscape(name)), &out); err != nil {
			return Project{}, ReleaseFile{}, err
		}
		for _, candidate := range out.Data {
			if candidate.ID <= 0 || candidate.GameID != game || projectWebsite(candidate) == "" {
				continue
			}
			for _, file := range candidate.LatestFiles {
				if file.ModID != candidate.ID || file.ID <= 0 || !file.IsAvailable || (file.FileStatus != 4 && file.FileStatus != 10) || !matchesImport(data, file) {
					continue
				}
				if matched.ID != 0 && (matched.ID != candidate.ID || archive.ID != file.ID) {
					return Project{}, ReleaseFile{}, fmt.Errorf("archive matches multiple source releases; choose its source manually")
				}
				matched = candidate
				archive = file
			}
		}
	}
	if matched.ID == 0 {
		return matched, archive, fmt.Errorf("no checksum-verified source found; set its project link manually")
	}
	return matched, archive, nil
}
func cleanImportName(name string) string {
	for _, ext := range []string{".mcaddon", ".mcpack", ".zip"} {
		name = strings.TrimSuffix(name, ext)
	}
	name = regexp.MustCompile(`§.`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`\[[^\]]*\]`).ReplaceAllString(name, " ")
	name = regexp.MustCompile(`(?i)\bv?\d+(?:\.\d+){1,3}\b`).ReplaceAllString(name, " ")
	name = regexp.MustCompile(`(?i)\b(bp|rp|we|behavior|resource|resources|pack)\b`).ReplaceAllString(name, " ")
	name = strings.SplitN(name, " + ", 2)[0]
	name = strings.SplitN(name, " | ", 2)[0]
	return strings.Join(strings.Fields(name), " ")
}
