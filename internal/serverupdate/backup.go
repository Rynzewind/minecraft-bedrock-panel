package serverupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type backupFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func backupInventory(root string) ([]backupFile, error) {
	var out []backupFile
	for _, name := range []string{"servers", "config.json"} {
		src := filepath.Join(root, name)
		e := filepath.WalkDir(src, func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				return fmt.Errorf("linked backup path")
			}
			if d.IsDir() {
				return nil
			}
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			h := sha256.Sum256(b)
			r, _ := filepath.Rel(root, p)
			out = append(out, backupFile{filepath.ToSlash(r), hex.EncodeToString(h[:])})
			return nil
		})
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func sealBackup(root string) error {
	files, e := backupInventory(root)
	if e != nil {
		return e
	}
	b, e := json.Marshal(files)
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(root, "backup-complete.json"), b)
}
func verifyBackup(root string) error {
	b, e := os.ReadFile(filepath.Join(root, "backup-complete.json"))
	if e != nil {
		return fmt.Errorf("backup is incomplete: %w", e)
	}
	var want []backupFile
	if e = json.Unmarshal(b, &want); e != nil || len(want) == 0 {
		return fmt.Errorf("invalid backup manifest")
	}
	got, e := backupInventory(root)
	if e != nil {
		return e
	}
	if len(want) != len(got) {
		return fmt.Errorf("backup inventory changed")
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Errorf("backup checksum mismatch: %s", want[i].Path)
		}
	}
	return nil
}
