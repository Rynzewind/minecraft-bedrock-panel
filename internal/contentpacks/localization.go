package contentpacks

import (
	"archive/zip"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Localized names are display metadata only. Never rewrite the pack manifest.
func localizePack(p *Pack, read func(string) ([]byte, error)) {
	for _, locale := range []string{"en_US", "en_GB"} {
		b, err := read("texts/" + locale + ".lang")
		if err != nil || len(b) > 1<<20 {
			continue
		}
		values := map[string]string{}
		for _, line := range strings.Split(strings.TrimPrefix(string(b), "\ufeff"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			if index := strings.Index(value, "\t##"); index >= 0 {
				value = value[:index]
			}
			key, value = strings.TrimSpace(key), strings.TrimSpace(value)
			if key != "" && value != "" {
				values[key] = value
			}
		}
		if value := values[p.Name]; value != "" {
			p.Name = value
		}
		if value := values[p.Description]; value != "" {
			p.Description = value
		}
	}
}

func localizeArchivePack(p *Pack, z *zip.Reader) {
	localizePack(p, func(name string) ([]byte, error) {
		entry, err := z.Open(path.Join(p.Root, name))
		if err != nil {
			return nil, err
		}
		defer entry.Close()
		return io.ReadAll(io.LimitReader(entry, (1<<20)+1))
	})
}

func localizeStoredPack(p *StoredPack) {
	localizePack(&p.Pack, func(name string) ([]byte, error) {
		file, err := os.Open(filepath.Join(p.InstalledPath, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		defer file.Close()
		return io.ReadAll(io.LimitReader(file, (1<<20)+1))
	})
}
