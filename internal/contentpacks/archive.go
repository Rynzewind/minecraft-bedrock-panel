package contentpacks

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Limits struct {
	MaxCompressedBytes, MaxExpandedBytes int64
	MaxFiles, MaxDepth                   int
}

var DefaultLimits = Limits{256 << 20, 1 << 30, 20000, 32}

func Inspect(filename string, limits Limits) (Import, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".zip" && ext != ".mcpack" && ext != ".mcaddon" {
		return Import{}, fmt.Errorf("unsupported pack extension %q", ext)
	}
	st, err := os.Stat(filename)
	if err != nil {
		return Import{}, err
	}
	if st.Size() > limits.MaxCompressedBytes {
		return Import{}, fmt.Errorf("archive exceeds compressed size limit")
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		return Import{}, err
	}
	in, err := inspectArchiveBytes(filename, b, limits, 0)
	if err != nil {
		return in, err
	}
	hash := sha256.Sum256(b)
	for i := range in.Packs {
		in.Packs[i].ImportSHA256 = hex.EncodeToString(hash[:])
		if len(in.Packs[i].ArchiveData) == 0 {
			in.Packs[i].ArchiveData = b
		}
	}
	return in, nil
}

func inspectArchiveBytes(source string, data []byte, limits Limits, depth int) (Import, error) {
	if depth >= limits.MaxDepth {
		return Import{}, fmt.Errorf("nested archive depth exceeds limit")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Import{}, err
	}
	if len(z.File) > limits.MaxFiles {
		return Import{}, fmt.Errorf("archive exceeds file count limit")
	}
	out := Import{Source: source}
	seen := map[string]bool{}
	var expanded uint64
	for _, f := range z.File {
		clean, err := safeArchivePath(f.Name, limits.MaxDepth)
		if err != nil {
			return Import{}, err
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return Import{}, fmt.Errorf("symlink entry %q is not allowed", f.Name)
		}
		expanded += f.UncompressedSize64
		if expanded > uint64(limits.MaxExpandedBytes) {
			return Import{}, fmt.Errorf("archive exceeds expanded size limit")
		}
		if strings.EqualFold(path.Base(clean), "manifest.json") {
			r, err := f.Open()
			if err != nil {
				return Import{}, err
			}
			b, err := io.ReadAll(io.LimitReader(r, 2<<20))
			_ = r.Close()
			if err != nil {
				return Import{}, err
			}
			p, err := parseManifest(b, path.Dir(clean))
			if err != nil {
				return Import{}, fmt.Errorf("%s: %w", clean, err)
			}
			if seen[p.UUID] {
				return Import{}, fmt.Errorf("duplicate pack UUID %s", p.UUID)
			}
			localizeArchivePack(&p, z)
			seen[p.UUID] = true
			out.Packs = append(out.Packs, p)
		}
	}
	for _, f := range z.File {
		ext := strings.ToLower(path.Ext(strings.ReplaceAll(f.Name, "\\", "/")))
		if ext != ".mcpack" && ext != ".mcaddon" && ext != ".zip" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return Import{}, err
		}
		child, err := io.ReadAll(io.LimitReader(r, limits.MaxCompressedBytes+1))
		_ = r.Close()
		if err != nil {
			return Import{}, err
		}
		if int64(len(child)) > limits.MaxCompressedBytes {
			return Import{}, fmt.Errorf("nested archive %q exceeds compressed size limit", f.Name)
		}
		nested, err := inspectArchiveBytes(source+"!"+f.Name, child, limits, depth+1)
		if err != nil {
			return Import{}, fmt.Errorf("%s: %w", f.Name, err)
		}
		for _, p := range nested.Packs {
			if seen[p.UUID] {
				return Import{}, fmt.Errorf("duplicate pack UUID %s", p.UUID)
			}
			seen[p.UUID] = true
			if len(p.ArchiveData) == 0 {
				p.ArchiveData = child
			}
			out.Packs = append(out.Packs, p)
		}
	}
	if len(out.Packs) == 0 {
		return Import{}, fmt.Errorf("archive contains no manifest.json")
	}
	return out, nil
}
func safeArchivePath(name string, maxDepth int) (string, error) {
	n := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(n, "/") || strings.Contains(n, ":") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	clean := path.Clean(n)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	if len(strings.Split(clean, "/")) > maxDepth {
		return "", fmt.Errorf("archive path too deep %q", name)
	}
	return clean, nil
}

func extractPack(archive, packRoot, destination string, limits Limits) error {
	b, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	return extractPackBytes(b, packRoot, destination, limits)
}

func extractPackBytes(data []byte, packRoot, destination string, limits Limits) error {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(destination, 0o750); err != nil {
		return err
	}
	prefix := strings.TrimSuffix(packRoot, "/")
	if prefix != "." && prefix != "" {
		prefix += "/"
	} else {
		prefix = ""
	}
	for _, f := range z.File {
		clean, err := safeArchivePath(f.Name, limits.MaxDepth)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(clean, prefix) {
			continue
		}
		rel := strings.TrimPrefix(clean, prefix)
		if rel == "" || strings.HasSuffix(f.Name, "/") {
			continue
		}
		dest := filepath.Join(destination, filepath.FromSlash(rel))
		if err = os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			_ = r.Close()
			return err
		}
		_, copyErr := io.Copy(w, r)
		closeErr := w.Close()
		_ = r.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
