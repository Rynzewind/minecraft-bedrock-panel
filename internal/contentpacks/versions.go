package contentpacks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cloneState(s persistentState) persistentState {
	b, _ := json.Marshal(s)
	var out persistentState
	_ = json.Unmarshal(b, &out)
	return out
}
func versionString(v [3]int) string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }
func (m *Manager) rememberVersion(p StoredPack) {
	for i, v := range m.state.Versions {
		if v.UUID == p.UUID && v.Version == p.Version {
			m.state.Versions[i] = p
			return
		}
	}
	m.state.Versions = append(m.state.Versions, p)
}
func (m *Manager) ensureVersions() error {
	if m.state.EnabledVersions == nil {
		m.state.EnabledVersions = map[string]map[string][3]int{}
	}
	for _, p := range m.state.Packs {
		m.rememberVersion(p)
	}
	for id, srv := range m.servers {
		if m.state.EnabledVersions[id] != nil {
			continue
		}
		m.state.EnabledVersions[id] = map[string][3]int{}
		actual := map[string][3]int{}
		if srv.Process != nil {
			for _, name := range []string{"world_behavior_packs.json", "world_resource_packs.json"} {
				b, e := os.ReadFile(filepath.Join(srv.Process.WorldDirectory, name))
				if os.IsNotExist(e) {
					continue
				}
				if e != nil {
					return fmt.Errorf("cannot read deployed pack versions for %s", id)
				}
				var entries []activation
				if json.Unmarshal(b, &entries) != nil {
					return fmt.Errorf("invalid deployed pack selections for %s", id)
				}
				for _, a := range entries {
					actual[a.PackID] = a.Version
				}
			}
		}
		for _, p := range m.state.Packs {
			if m.state.Enabled[id][p.UUID] {
				v, ok := actual[p.UUID]
				if !ok {
					continue
				}
				m.state.EnabledVersions[id][p.UUID] = v
			}
		}
	}
	for i, o := range m.state.Pending {
		if o.Enable && !o.Pinned {
			for _, p := range m.state.Packs {
				if p.UUID == o.PackUUID {
					m.state.Pending[i].Version = p.Version
					m.state.Pending[i].Pinned = true
				}
			}
		}
	}
	return nil
}
func (m *Manager) packAtVersion(id string, version [3]int) (StoredPack, error) {
	for _, p := range m.state.Packs {
		if p.UUID == id && p.Version == version {
			return p, nil
		}
	}
	for _, p := range m.state.Versions {
		if p.UUID == id && p.Version == version {
			return p, nil
		}
	}
	dir := filepath.Join(m.root, "packs", id, versionString(version))
	b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		return StoredPack{}, fmt.Errorf("retained pack version %s %s is unavailable", id, versionString(version))
	}
	p, e := parseManifest(b, "")
	if e != nil || p.UUID != id || p.Version != version {
		return StoredPack{}, fmt.Errorf("retained pack version does not match its manifest")
	}
	return StoredPack{Pack: p, InstalledPath: dir}, nil
}
func (m *Manager) resolvedPacks(serverID string, enabled map[string]bool) ([]StoredPack, map[string][3]int, error) {
	if e := m.ensureVersions(); e != nil {
		return nil, nil, e
	}
	versions := map[string][3]int{}
	for id, v := range m.state.EnabledVersions[serverID] {
		versions[id] = v
	}
	for _, o := range m.state.Pending {
		if o.ServerID == serverID {
			if o.Enable {
				versions[o.PackUUID] = o.Version
			} else {
				delete(versions, o.PackUUID)
			}
		}
	}
	var out []StoredPack
	for _, latest := range m.state.Packs {
		p := latest
		if enabled[p.UUID] {
			v, ok := versions[p.UUID]
			if !ok {
				return nil, nil, fmt.Errorf("deployed version for %s is unknown; explicitly stage a version", p.Name)
			}
			var e error
			p, e = m.packAtVersion(p.UUID, v)
			if e != nil {
				return nil, nil, e
			}
		}
		out = append(out, p)
	}
	for _, p := range out {
		if !enabled[p.UUID] {
			continue
		}
		for _, dep := range p.Dependencies {
			if dep.UUID != "" && (!enabled[dep.UUID] || versionGreater(dep.Version, versions[dep.UUID])) {
				return nil, nil, fmt.Errorf("%s requires enabled dependency %s version %v", p.Name, dep.UUID, dep.Version)
			}
		}
	}
	return out, versions, nil
}

func (m *Manager) managedPackPath(id string, version [3]int) (string, error) {
	if !validPackUUID(id) {
		return "", fmt.Errorf("invalid stored pack UUID")
	}
	for _, v := range version {
		if v < 0 {
			return "", fmt.Errorf("invalid stored pack version")
		}
	}
	base, e := filepath.Abs(filepath.Join(m.root, "packs"))
	if e != nil {
		return "", e
	}
	dest := filepath.Join(base, id, versionString(version))
	rel, e := filepath.Rel(base, dest)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("pack path escapes library")
	}
	return dest, nil
}
