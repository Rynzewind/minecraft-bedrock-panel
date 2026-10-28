package migration

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func WriteArchive(ctx context.Context, root, out string, m Manifest) (err error) {
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(out)
		}
	}()
	if err = privateDirectory(out); err != nil {
		return err
	}
	z := zip.NewWriter(f)
	defer z.Close()
	if err = noLinks(root); err != nil {
		return err
	}
	checkedParents := map[string]bool{}
	for _, entry := range m.Files {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = checkHash(entry); err != nil {
			return err
		}
		p := filepath.Join(root, filepath.FromSlash(entry.Path))
		parent := filepath.Dir(p)
		if !checkedParents[parent] {
			if err = noLinks(parent); err != nil {
				return err
			}
			checkedParents[parent] = true
		}
		info, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("archive payload is not a regular file")
		}
		src, e := os.Open(p)
		if e != nil {
			return e
		}
		w, e := z.Create(entry.Path)
		if e != nil {
			src.Close()
			return e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(w, h), contextReader{ctx, src})
		src.Close()
		if e != nil {
			return e
		}
		if n != entry.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), entry.SHA256) {
			return errors.New("payload changed during archive creation")
		}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > maxManifest {
		return errors.New("manifest too large")
	}
	w, err := z.Create("manifest.json")
	if err != nil {
		return err
	}
	if _, err = w.Write(b); err != nil {
		return err
	}
	if err = z.Close(); err != nil {
		return err
	}
	return f.Sync()
}

func VerifyArchive(ctx context.Context, p string) (Manifest, error) {
	z, err := zip.OpenReader(p)
	if err != nil {
		return Manifest{}, err
	}
	defer z.Close()
	return verifyZip(ctx, &z.Reader)
}
func verifyZip(ctx context.Context, z *zip.Reader) (Manifest, error) {
	var m Manifest
	if len(z.File) > maxFiles+1 {
		return m, errors.New("too many archive entries")
	}
	entries := map[string]*zip.File{}
	var total uint64
	var manifestFile *zip.File
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return m, err
		}
		if err := safeName(f.Name); err != nil {
			return m, err
		}
		if !f.Mode().IsRegular() {
			return m, errors.New("archive must contain regular files only")
		}
		k := strings.ToLower(f.Name)
		if entries[k] != nil {
			return m, errors.New("duplicate archive entry")
		}
		entries[k] = f
		if excluded(f.Name) {
			return m, errors.New("archive contains excluded credentials or backup data")
		}
		if f.UncompressedSize64 > maxExpanded-total {
			return m, errors.New("archive expansion limit exceeded")
		}
		total += f.UncompressedSize64
		if f.Name == "manifest.json" {
			manifestFile = f
		}
	}
	if manifestFile == nil || manifestFile.UncompressedSize64 > maxManifest {
		return m, errors.New("missing or oversized manifest")
	}
	r, err := manifestFile.Open()
	if err != nil {
		return m, err
	}
	b, err := io.ReadAll(io.LimitReader(r, maxManifest+1))
	r.Close()
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if m.FormatVersion != FormatVersion || !filepath.IsAbs(m.SourceRoot) {
		return m, errors.New("unsupported or invalid migration manifest")
	}
	if len(m.Files)+1 != len(entries) {
		return m, errors.New("manifest does not describe every payload file")
	}
	seen := map[string]bool{}
	for _, entry := range m.Files {
		if err = checkHash(entry); err != nil {
			return m, err
		}
		k := strings.ToLower(entry.Path)
		f := entries[k]
		if f == nil || f.Name != entry.Path || seen[k] || entry.Path == "manifest.json" || f.UncompressedSize64 != uint64(entry.Size) {
			return m, errors.New("invalid manifest entry")
		}
		seen[k] = true
		r, err := f.Open()
		if err != nil {
			return m, err
		}
		h := sha256.New()
		n, err := io.Copy(h, contextReader{ctx, io.LimitReader(r, entry.Size+1)})
		r.Close()
		if err != nil {
			return m, err
		}
		if n != entry.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), entry.SHA256) {
			return m, fmt.Errorf("checksum mismatch: %s", entry.Path)
		}
	}
	return m, nil
}
