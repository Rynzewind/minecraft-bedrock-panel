package migration

import (
	"errors"
	"io/fs"
	"nethernet-multi-server/internal/config"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func excluded(n string) bool {
	parts := strings.Split(strings.ToLower(filepath.ToSlash(n)), "/")
	if len(parts) < 2 || parts[0] == "addons" {
		return false
	}
	// Exclusions are anchored to application-owned operational paths. Worlds
	// and pack payloads are opaque content: logs/backups may be asset names,
	// and LevelDB *.log files are essential world data after a clean stop.
	second := parts[1]
	if second == "logs" || second == "backups" || second == "rollback" || second == "uninstall-backups" || second == "session-logs" {
		return true
	}
	if parts[0] == "multiserver" {
		if second == "account-store" || strings.HasPrefix(second, "auth-token") {
			return true
		}
		if len(parts) == 3 && (second == "identities" || second == "server-settings" || second == "content-staging") {
			name := parts[2]
			if name == "backups" || name == "uninstall-backups" || strings.HasSuffix(name, ".bak") || strings.Contains(name, ".bak.") || strings.HasSuffix(name, ".tmp") {
				return true
			}
		}
		if len(parts) > 3 && (second == "server-settings" || second == "content-staging") && (parts[2] == "backups" || parts[2] == "uninstall-backups") {
			return true
		}
	}
	if len(parts) == 2 && (strings.HasSuffix(second, ".log") || strings.HasSuffix(second, ".bak") || strings.Contains(second, ".bak.") || strings.HasSuffix(second, ".tmp") || second == "packet-statistics.txt") {
		return true
	}
	return false
}
func BuildInventory(root, output string) ([]FileEntry, error) {
	return buildInventory(root, output, true)
}
func inventoryForSizing(root, output string) ([]FileEntry, error) {
	return buildInventory(root, output, false)
}
func buildInventory(root, output string, hashContents bool) ([]FileEntry, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	if inside(root, output) || inside(output, root) {
		return nil, errors.New("snapshot output must be outside the server folder")
	}
	if err = noLinks(root); err != nil {
		return nil, err
	}
	if err = noLinks(output); err != nil {
		return nil, err
	}
	cfg, err := config.Load(filepath.Join(root, "MultiServer", "config.json"))
	if err != nil {
		return nil, err
	}
	roots := []string{"MultiServer/config.json", "MultiServer/identities", "MultiServer/content-staging", "MultiServer/server-settings", "MultiServer/player-permissions", "MultiServer/installer", "Addons"}
	for _, name := range []string{"NetherNetMultiServer.exe", "NetherNetMultiServerAdmin.exe", "NetherNetMultiServerControl.exe"} {
		roots = append(roots, "MultiServer/"+name)
	}
	for _, name := range []string{"MultiServer/online", "MultiServer/NetherNetOnline.exe", "MultiServer/addon-update-sources.json", "MultiServer/automatic-updates.json", "MultiServer/installation-resources.json", "StartMinecraftServers.ps1", "Watch-NetherNet.ps1", "StartMinecraftServers.bat", "StartMinecraftServers.vbs", "StopMinecraftServers.bat", "MultiServer/Run-NetherNetMultiServer.cmd", "MultiServer/Start-NetherNetMultiServer.cmd", "MultiServer/engineering-handover/NetherNet-Confirmed-Fixes-Source-20261008.zip", "MultiServer/engineering-handover/NetherNet-Migration-Source.zip"} {
		roots = append(roots, name)
	}
	for _, s := range cfg.Servers {
		if s.Process == nil || !inside(root, s.Process.WorkingDirectory) || strings.EqualFold(root, s.Process.WorkingDirectory) {
			return nil, errors.New("all server folders must be managed subfolders of the installation root")
		}
		rel, _ := filepath.Rel(root, s.Process.WorkingDirectory)
		if strings.Contains(rel, string(filepath.Separator)) || strings.EqualFold(rel, "MultiServer") {
			return nil, errors.New("server folders must be separate top-level folders")
		}
		roots = append(roots, rel)
	}
	var files []FileEntry
	seen := map[string]bool{}
	for _, n := range roots {
		p := filepath.Join(root, filepath.FromSlash(n))
		if err = noLinks(p); err != nil {
			return nil, err
		}
		if _, e := os.Lstat(p); os.IsNotExist(e) {
			continue
		}
		err = filepath.WalkDir(p, func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(root, p)
			if e != nil {
				return e
			}
			rel = filepath.ToSlash(rel)
			if excluded(rel) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				return errors.New("symbolic links and reparse points are not supported")
			}
			if d.IsDir() {
				return nil
			}
			i, e := d.Info()
			if e != nil {
				return e
			}
			if !i.Mode().IsRegular() {
				return errors.New("unsupported file type")
			}
			if e = safeName(rel); e != nil {
				return e
			}
			key := strings.ToLower(rel)
			if seen[key] {
				return nil
			}
			seen[key] = true
			f := FileEntry{Path: rel, Size: i.Size()}
			if hashContents {
				f, e = fileEntry(root, p)
				if e != nil {
					return e
				}
			}
			files = append(files, f)
			if len(files) > maxFiles {
				return errors.New("too many files")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	required := []string{"MultiServer/config.json", "MultiServer/NetherNetMultiServer.exe", "MultiServer/NetherNetMultiServerAdmin.exe", "MultiServer/NetherNetMultiServerControl.exe"}
	for _, s := range cfg.Servers {
		r, _ := filepath.Rel(root, s.Process.Executable)
		required = append(required, filepath.ToSlash(r))
		required = append(required, "MultiServer/identities/"+s.NetworkID+".pem")
		r, _ = filepath.Rel(root, s.Process.WorldDirectory)
		hasWorld := false
		for _, f := range files {
			if strings.HasPrefix(strings.ToLower(f.Path), strings.ToLower(filepath.ToSlash(r))+"/") {
				hasWorld = true
				break
			}
		}
		if !hasWorld {
			return nil, errors.New("configured world folder is empty or missing")
		}
	}
	for _, n := range required {
		if !seen[strings.ToLower(n)] {
			return nil, errors.New("missing required deployment file: " + n)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
