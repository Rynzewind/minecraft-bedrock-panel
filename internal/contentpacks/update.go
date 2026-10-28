package contentpacks

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/addonupdates"
	"os"
	"path/filepath"
	"time"
)

type UpdateChange struct {
	UUID, Name string
	Version    [3]int
	Type       PackType
}
type UpdatePreview struct {
	ID, SHA256 string
	Servers    []string
	Changes    []UpdateChange
	Expires    time.Time
}
type updateReview struct {
	Public            UpdatePreview
	Path, Fingerprint string
	Selected          string
	Engines           map[string][3]int
}

func (m *Manager) fingerprint() string {
	b, _ := json.Marshal(m.state)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (m *Manager) validateUpdate(in Import, selected string, targets []string, engines map[string][3]int) error {
	if len(targets) == 0 {
		return fmt.Errorf("select at least one server")
	}
	existing := map[string]StoredPack{}
	incoming := map[string]Pack{}
	for _, p := range m.state.Packs {
		existing[p.UUID] = p
	}
	old, ok := existing[selected]
	if !ok {
		return fmt.Errorf("select an installed pack")
	}
	for _, p := range in.Packs {
		incoming[p.UUID] = p
		if x, ok := existing[p.UUID]; ok && x.Type != p.Type {
			return fmt.Errorf("pack type changed for %s", p.Name)
		}
		if x, ok := existing[p.UUID]; ok && versionGreater(x.Version, p.Version) {
			return fmt.Errorf("archive downgrades %s", p.Name)
		}
	}
	p, ok := incoming[selected]
	if !ok || !versionGreater(p.Version, old.Version) {
		return fmt.Errorf("archive must contain a newer version of the selected UUID")
	}
	// Only packs connected by explicit UUID dependencies belong to this review.
	connected := map[string]bool{selected: true}
	for changed := true; changed; {
		changed = false
		for id, p := range incoming {
			for _, d := range p.Dependencies {
				if _, ok := incoming[d.UUID]; ok && (connected[id] || connected[d.UUID]) && (!connected[id] || !connected[d.UUID]) {
					connected[id] = true
					connected[d.UUID] = true
					changed = true
				}
			}
		}
	}
	for id := range incoming {
		if !connected[id] {
			return fmt.Errorf("archive includes unrelated pack %s; import it separately", id)
		}
	}
	seen := map[string]bool{}
	for _, id := range targets {
		if seen[id] {
			return fmt.Errorf("duplicate target server")
		}
		seen[id] = true
		if _, ok := m.servers[id]; !ok {
			return fmt.Errorf("unknown server")
		}
		for _, o := range m.state.Pending {
			if o.ServerID == id {
				return fmt.Errorf("cancel or apply pending pack changes for %s before reviewing an update", id)
			}
		}
		if _, ok := m.state.PendingResourcePolicy[id]; ok {
			return fmt.Errorf("pending pack policy for %s", id)
		}
		engine, ok := engines[id]
		if !ok {
			return fmt.Errorf("server engine version is unknown")
		}
		for _, p := range incoming {
			if versionGreater(p.MinEngineVersion, engine) {
				return fmt.Errorf("%s requires engine %v", p.Name, p.MinEngineVersion)
			}
			for _, d := range p.Dependencies {
				if d.UUID == "" {
					continue
				}
				v, ok := incoming[d.UUID]
				if ok {
					if versionGreater(d.Version, v.Version) {
						return fmt.Errorf("dependency version missing")
					}
					continue
				}
				pin, enabled := m.state.EnabledVersions[id][d.UUID]
				if !m.state.Enabled[id][d.UUID] || !enabled || versionGreater(d.Version, pin) {
					return fmt.Errorf("required dependency %s is absent from this review or deployed server", d.UUID)
				}
			}
		}
	}
	return nil
}
func (m *Manager) PreviewUpdate(path, selected string, targets []string, engines map[string][3]int) (UpdatePreview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.ensureVersions(); e != nil {
		return UpdatePreview{}, e
	}
	st, e := os.Stat(path)
	if e != nil || st.Size() > DefaultLimits.MaxCompressedBytes {
		return UpdatePreview{}, fmt.Errorf("archive is unavailable or exceeds size limit")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return UpdatePreview{}, e
	}
	token := make([]byte, 16)
	if _, e = rand.Read(token); e != nil {
		return UpdatePreview{}, e
	}
	id := hex.EncodeToString(token)
	dir := filepath.Join(m.root, "update-reviews")
	if e = addonupdates.ProtectStaging(dir); e != nil {
		return UpdatePreview{}, e
	}
	copyPath := filepath.Join(dir, id+".zip")
	if e = os.WriteFile(copyPath, b, 0600); e != nil {
		return UpdatePreview{}, e
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(copyPath)
		}
	}()
	in, e := Inspect(copyPath, DefaultLimits)
	if e != nil {
		return UpdatePreview{}, e
	}
	if e = m.validateUpdate(in, selected, targets, engines); e != nil {
		return UpdatePreview{}, e
	}
	hash := sha256.Sum256(b)
	p := UpdatePreview{ID: id, SHA256: hex.EncodeToString(hash[:]), Servers: append([]string(nil), targets...), Expires: time.Now().Add(30 * time.Minute)}
	for _, v := range in.Packs {
		p.Changes = append(p.Changes, UpdateChange{v.UUID, v.Name, v.Version, v.Type})
	}
	if m.previews == nil {
		m.previews = map[string]updateReview{}
	}
	// Bound retained reviews, removing expired files. Reviews never survive helper restart.
	for key, r := range m.previews {
		if time.Now().After(r.Public.Expires) || len(m.previews) >= 16 {
			os.Remove(r.Path)
			delete(m.previews, key)
		}
	}
	engineCopy := map[string][3]int{}
	for k, v := range engines {
		engineCopy[k] = v
	}
	m.previews[id] = updateReview{p, copyPath, m.fingerprint(), selected, engineCopy}
	keep = true
	return p, nil
}
func (m *Manager) StageUpdate(id string) error { return m.StageUpdateForEngines(id, nil) }
func (m *Manager) StageUpdateForEngines(id string, engines map[string][3]int) (retErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.previews[id]
	if !ok {
		return fmt.Errorf("review is missing; preview the archive again")
	}
	if engines != nil {
		for _, server := range r.Public.Servers {
			v, ok := engines[server]
			if !ok || v != r.Engines[server] {
				return fmt.Errorf("server engine changed; preview again")
			}
		}
	}
	delete(m.previews, id)
	defer os.Remove(r.Path)
	if time.Now().After(r.Public.Expires) || r.Fingerprint != m.fingerprint() {
		return fmt.Errorf("review expired or pack state changed; preview again")
	}
	b, e := os.ReadFile(r.Path)
	if e != nil {
		return e
	}
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != r.Public.SHA256 {
		return fmt.Errorf("review archive changed")
	}
	in, e := Inspect(r.Path, DefaultLimits)
	if e != nil {
		return e
	}
	if e = m.validateUpdate(in, r.Selected, r.Public.Servers, r.Engines); e != nil {
		return e
	}
	var newDirs []string
	for _, p := range in.Packs {
		dir := filepath.Join(m.root, "packs", p.UUID, versionString(p.Version))
		if _, e := os.Stat(dir); os.IsNotExist(e) {
			newDirs = append(newDirs, dir)
		}
	}
	defer func() {
		if retErr != nil {
			for _, dir := range newDirs {
				os.RemoveAll(dir)
				os.RemoveAll(dir + ".tmp")
			}
		}
	}()
	old := cloneState(m.state)
	defer func() {
		if retErr != nil {
			m.state = old
		}
	}()
	for i := range in.Packs {
		in.Packs[i].SourceArchive = r.Path
	}
	stored, e := m.importInspectedLocked(in.Packs, false)
	if e != nil {
		return e
	}
	for _, server := range r.Public.Servers {
		for _, p := range stored {
			m.state.Pending = append(m.state.Pending, Operation{Version: p.Version, ServerID: server, PackUUID: p.UUID, Enable: true, Pinned: true})
		}
	}
	return m.save()
}
