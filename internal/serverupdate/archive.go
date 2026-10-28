package serverupdate

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func safeName(n string) bool {
	if n == "" || strings.ContainsAny(n, "\\:") || strings.HasPrefix(n, "/") {
		return false
	}
	for _, p := range strings.Split(strings.TrimSuffix(n, "/"), "/") {
		if p == "" || p == "." || p == ".." || strings.TrimRight(p, ". ") != p {
			return false
		}
		b := strings.ToUpper(strings.Split(p, ".")[0])
		if b == "CON" || b == "PRN" || b == "AUX" || b == "NUL" || len(b) == 4 && (strings.HasPrefix(b, "COM") || strings.HasPrefix(b, "LPT")) && b[3] >= '0' && b[3] <= '9' {
			return false
		}
	}
	return true
}
func mutable(n string) bool {
	p := strings.Split(strings.ToLower(n), "/")
	switch p[0] {
	case "worlds", "logs", "backups", "config", "server.properties", "permissions.json", "allowlist.json", "whitelist.json", "packetlimitconfig.json":
		return true
	}
	return false
}
func extractArchive(data []byte, dst string) error {
	r, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return e
	}
	if len(r.File) > 100000 {
		return fmt.Errorf("too many archive entries")
	}
	seen := map[string]bool{}
	var total uint64
	hasExe := false
	for _, f := range r.File {
		if !safeName(f.Name) || f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe archive path: %s", f.Name)
		}
		k := strings.ToLower(strings.TrimSuffix(f.Name, "/"))
		if seen[k] {
			return fmt.Errorf("duplicate archive path: %s", f.Name)
		}
		seen[k] = true
		total += f.UncompressedSize64
		if f.UncompressedSize64 > 512<<20 || total > 4<<30 {
			return fmt.Errorf("archive exceeds size limit")
		}
		if f.FileInfo().IsDir() || mutable(f.Name) {
			continue
		}
		out := filepath.Join(dst, filepath.FromSlash(f.Name))
		if e = os.MkdirAll(filepath.Dir(out), 0700); e != nil {
			return e
		}
		in, e := f.Open()
		if e != nil {
			return e
		}
		b, e := io.ReadAll(io.LimitReader(in, int64(f.UncompressedSize64)+1))
		in.Close()
		if e != nil {
			return e
		}
		if len(b) != int(f.UncompressedSize64) {
			return fmt.Errorf("invalid archive size")
		}
		if k == "bedrock_server.exe" {
			hasExe = len(b) >= 2 && string(b[:2]) == "MZ"
		}
		if e = os.WriteFile(out, b, 0600); e != nil {
			return e
		}
	}
	if !hasExe {
		return fmt.Errorf("valid Windows server executable missing")
	}
	return nil
}
