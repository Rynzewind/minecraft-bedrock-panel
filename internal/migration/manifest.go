// Package migration owns the portable, credential-free Windows migration format.
package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const FormatVersion = 1
const maxManifest = 16 << 20
const maxFiles = 300000
const maxExpanded uint64 = 1 << 40 // 1 TiB, independent of compressed input size.

type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Component struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type Manifest struct {
	Backup               *BackupMetadata `json:"backup,omitempty"`
	AddonLibraryIncluded *bool           `json:"addon_library_included,omitempty"`
	FormatVersion        int             `json:"format_version"`
	CreatedUTC           time.Time       `json:"created_utc"`
	SourceRoot           string          `json:"source_root"`
	Files                []FileEntry     `json:"files"`
	Components           []Component     `json:"components"`
}

func safeName(n string) error {
	if n == "" || strings.ContainsAny(n, "\\:\x00") || strings.HasPrefix(n, "/") || path.Clean(n) != n {
		return errors.New("unsafe package path")
	}
	for _, p := range strings.Split(n, "/") {
		if p == "." || p == ".." || strings.TrimRight(p, " .") != p || strings.ContainsAny(p, "<>\"|?*") {
			return errors.New("unsafe Windows filename")
		}
		for _, r := range p {
			if r < 32 {
				return errors.New("control character in path")
			}
		}
		base := strings.ToUpper(strings.SplitN(p, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
			return errors.New("reserved Windows filename")
		}
	}
	return nil
}
func inside(root, p string) bool {
	r, e := filepath.Rel(root, p)
	return e == nil && !filepath.IsAbs(r) && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}
func noLinks(p string) error {
	a, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	for {
		i, e := os.Lstat(a)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && i.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return errors.New("symbolic links and reparse points are not supported")
		}
		next := filepath.Dir(a)
		if next == a {
			break
		}
		a = next
	}
	return nil
}
func fileEntry(root, p string) (FileEntry, error) {
	f, err := os.Open(p)
	if err != nil {
		return FileEntry{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return FileEntry{}, err
	}
	r, err := filepath.Rel(root, p)
	if err != nil {
		return FileEntry{}, err
	}
	return FileEntry{filepath.ToSlash(r), n, hex.EncodeToString(h.Sum(nil))}, nil
}
func checkHash(f FileEntry) error {
	b, e := hex.DecodeString(f.SHA256)
	if e != nil || len(b) != 32 || f.Size < 0 {
		return fmt.Errorf("invalid file checksum: %s", f.Path)
	}
	return safeName(f.Path)
}
