package contentpacks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type activation struct {
	PackID  string `json:"pack_id"`
	Version [3]int `json:"version"`
}

type backupEntry struct {
	Path       string `json:"path"`
	BackupPath string `json:"backup_path"`
	Directory  bool   `json:"directory"`
	Existed    bool   `json:"existed"`
}

type backupManifest struct {
	ServerID string        `json:"server_id"`
	Created  time.Time     `json:"created"`
	Entries  []backupEntry `json:"entries"`
}

func (m *Manager) Apply(serverID string) (applyErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[serverID]
	if !ok || s.Process == nil {
		return errors.New("server is unconfigured")
	}
	hasPending := false
	for _, o := range m.state.Pending {
		if o.ServerID == serverID {
			hasPending = true
			break
		}
	}
	_, policyPending := m.state.PendingResourcePolicy[serverID]
	hasPending = hasPending || policyPending
	if !hasPending {
		return nil
	}
	managed := map[string]bool{}
	for _, p := range m.state.Packs {
		managed[p.UUID] = true
	}
	behavior := readActivations(filepath.Join(s.Process.WorldDirectory, "world_behavior_packs.json"), managed)
	resource := readActivations(filepath.Join(s.Process.WorldDirectory, "world_resource_packs.json"), managed)
	enabled := map[string]bool{}
	for uuid, on := range m.state.Enabled[serverID] {
		enabled[uuid] = on
	}
	for _, o := range m.state.Pending {
		if o.ServerID == serverID {
			enabled[o.PackUUID] = o.Enable
		}
	}
	resolved, versions, resolveErr := m.resolvedPacks(serverID, enabled)
	if resolveErr != nil {
		return resolveErr
	}
	backup := filepath.Join(m.root, "backups", serverID, time.Now().Format("20060102-150405.000000000"))
	if err := os.MkdirAll(backup, 0o750); err != nil {
		return err
	}
	propertiesPath := filepath.Join(s.Process.WorkingDirectory, "server.properties")
	manifest := backupManifest{ServerID: serverID, Created: time.Now()}
	for _, path := range []string{
		propertiesPath,
		filepath.Join(s.Process.WorldDirectory, "world_behavior_packs.json"),
		filepath.Join(s.Process.WorldDirectory, "world_resource_packs.json"),
	} {
		entry, err := backupPath(path, filepath.Join(backup, "files", filepath.Base(path)), false)
		if err != nil {
			return err
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	for _, p := range m.state.Packs {
		packDir := "resource_packs"
		if p.Type == Behavior {
			packDir = "behavior_packs"
		}
		destDir := filepath.Join(s.Process.WorkingDirectory, packDir, p.UUID)
		entry, err := backupPath(destDir, filepath.Join(backup, "packs", packDir, p.UUID), true)
		if err != nil {
			return err
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(backup, "manifest.json"), manifestBytes, 0o600); err != nil {
		return err
	}
	mutating := true
	defer func() {
		if applyErr != nil && mutating {
			if rollbackErr := restoreBackup(manifest); rollbackErr != nil {
				applyErr = fmt.Errorf("%w; automatic rollback also failed: %v", applyErr, rollbackErr)
			}
		}
	}()
	for _, p := range resolved {
		packDir := "resource_packs"
		if p.Type == Behavior {
			packDir = "behavior_packs"
		}
		destDir := filepath.Join(s.Process.WorkingDirectory, packDir, p.UUID)
		if !enabled[p.UUID] {
			if err := os.RemoveAll(destDir); err != nil {
				return err
			}
			continue
		}
		tmpDir := destDir + ".tmp"
		if err := os.RemoveAll(tmpDir); err != nil {
			return err
		}
		if err := copyDirectory(p.InstalledPath, tmpDir); err != nil {
			return err
		}
		if err := os.RemoveAll(destDir); err != nil {
			return err
		}
		if err := os.Rename(tmpDir, destDir); err != nil {
			return err
		}
		a := activation{p.UUID, p.Version}
		if p.Type == Behavior {
			behavior = append(behavior, a)
		} else {
			resource = append(resource, a)
		}
	}
	files := map[string][]activation{"world_behavior_packs.json": behavior, "world_resource_packs.json": resource}
	for name, list := range files {
		dest := filepath.Join(s.Process.WorldDirectory, name)
		b, err := json.MarshalIndent(list, "", "  ")
		if err != nil {
			return err
		}
		tmp := dest + ".tmp"
		if err = os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return err
		}
		if err = os.WriteFile(tmp, b, 0o600); err != nil {
			return err
		}
		if err = os.Rename(tmp, dest); err != nil {
			return err
		}
	}
	if policyPending {
		if err := setProperty(propertiesPath, "texturepack-required", fmt.Sprint(m.state.PendingResourcePolicy[serverID])); err != nil {
			return err
		}
	}
	oldState := cloneState(m.state)
	oldState.Pending = append([]Operation(nil), m.state.Pending...)
	oldState.Enabled = cloneEnabled(m.state.Enabled)
	oldState.PendingResourcePolicy = clonePolicies(m.state.PendingResourcePolicy)
	keep := m.state.Pending[:0]
	for _, o := range m.state.Pending {
		if o.ServerID != serverID {
			keep = append(keep, o)
		}
	}
	m.state.Pending = keep
	delete(m.state.PendingResourcePolicy, serverID)
	if m.state.Enabled == nil {
		m.state.Enabled = map[string]map[string]bool{}
	}
	m.state.Enabled[serverID] = enabled
	m.state.EnabledVersions[serverID] = versions
	if err := m.save(); err != nil {
		m.state = oldState
		return err
	}
	mutating = false
	return nil
}

func backupPath(source, destination string, directory bool) (backupEntry, error) {
	entry := backupEntry{Path: source, BackupPath: destination, Directory: directory}
	info, err := os.Stat(source)
	if errors.Is(err, os.ErrNotExist) {
		return entry, nil
	}
	if err != nil {
		return entry, err
	}
	entry.Existed = true
	if info.IsDir() {
		entry.Directory = true
	}
	if entry.Directory {
		if !info.IsDir() {
			return entry, fmt.Errorf("expected directory at %s", source)
		}
		return entry, copyDirectory(source, destination)
	}
	if info.IsDir() {
		return entry, nil
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return entry, err
	}
	b, err := os.ReadFile(source)
	if err != nil {
		return entry, err
	}
	return entry, os.WriteFile(destination, b, 0o600)
}

func restoreBackup(manifest backupManifest) error {
	var failures []string
	for i := len(manifest.Entries) - 1; i >= 0; i-- {
		entry := manifest.Entries[i]
		if err := os.RemoveAll(entry.Path); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if !entry.Existed {
			continue
		}
		var err error
		if entry.Directory {
			err = copyDirectory(entry.BackupPath, entry.Path)
		} else {
			var b []byte
			b, err = os.ReadFile(entry.BackupPath)
			if err == nil {
				err = os.MkdirAll(filepath.Dir(entry.Path), 0o750)
			}
			if err == nil {
				err = os.WriteFile(entry.Path, b, 0o600)
			}
		}
		if err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func cloneEnabled(in map[string]map[string]bool) map[string]map[string]bool {
	out := make(map[string]map[string]bool, len(in))
	for server, values := range in {
		out[server] = make(map[string]bool, len(values))
		for uuid, enabled := range values {
			out[server][uuid] = enabled
		}
	}
	return out
}

func clonePolicies(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for server, required := range in {
		out[server] = required
	}
	return out
}

func setProperty(filename, key, value string) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			lines[i] = key + "=" + value
			found = true
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}
	tmp := filename + ".tmp"
	if err = os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filename)
}

func readActivations(filename string, managed map[string]bool) []activation {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}
	var all []activation
	if json.Unmarshal(b, &all) != nil {
		return nil
	}
	out := all[:0]
	for _, a := range all {
		if !managed[a.PackID] {
			out = append(out, a)
		}
	}
	return out
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destination, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o750)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("staged pack contains a symlink")
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
