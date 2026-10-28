package contentpacks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nethernet-multi-server/internal/config"
)

type StoredPack struct {
	Pack
	InstalledPath string `json:"installed_path"`
}
type AssignmentState struct {
	Enabled bool  `json:"enabled"`
	Pending *bool `json:"pending,omitempty"`
}
type InventoryItem struct {
	StoredPack
	Servers map[string]AssignmentState `json:"servers"`
}
type Operation struct {
	ServerID, PackUUID string
	Enable             bool
}
type persistentState struct {
	Packs                 []StoredPack               `json:"packs"`
	Pending               []Operation                `json:"pending"`
	Enabled               map[string]map[string]bool `json:"enabled,omitempty"`
	PendingResourcePolicy map[string]bool            `json:"pending_resource_policy,omitempty"`
}
type Manager struct {
	mu              sync.Mutex
	root, statePath string
	servers         map[string]config.Server
	state           persistentState
}

func NewManager(root string, servers []config.Server) (*Manager, error) {
	m := &Manager{root: root, statePath: filepath.Join(root, "state.json"), servers: map[string]config.Server{}}
	for _, s := range servers {
		m.servers[s.ID] = s
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	m.state.Enabled = map[string]map[string]bool{}
	b, err := os.ReadFile(m.statePath)
	if err == nil {
		err = json.Unmarshal(b, &m.state)
	} else if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if m.state.Enabled == nil {
		m.state.Enabled = map[string]map[string]bool{}
	}
	if m.state.PendingResourcePolicy == nil {
		m.state.PendingResourcePolicy = map[string]bool{}
	}
	return m, err
}
func (m *Manager) StageResourcePolicy(serverID string, required bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.servers[serverID]; !ok {
		return errors.New("server not found")
	}
	m.state.PendingResourcePolicy[serverID] = required
	return m.save()
}
func (m *Manager) save() error {
	b, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.statePath + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.statePath)
}
func (m *Manager) Stage(serverID, uuid string, enable bool) error {
	return m.StageMany(serverID, []string{uuid}, enable)
}

func (m *Manager) StageMany(serverID string, uuids []string, enable bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.servers[serverID]; !ok {
		return errors.New("server not found")
	}
	available := map[string]bool{}
	for _, p := range m.state.Packs {
		available[p.UUID] = true
	}
	if len(uuids) == 0 {
		return errors.New("at least one pack is required")
	}
	linked := map[string]bool{}
	for _, uuid := range uuids {
		if !available[uuid] {
			return fmt.Errorf("pack %s not found", uuid)
		}
		linked[uuid] = true
	}
	for changed := true; changed; {
		changed = false
		for _, pack := range m.state.Packs {
			for _, dependency := range pack.Dependencies {
				if dependency.UUID == "" {
					continue
				}
				if linked[pack.UUID] && !linked[dependency.UUID] {
					linked[dependency.UUID] = true
					changed = true
				}
				if linked[dependency.UUID] && !linked[pack.UUID] {
					linked[pack.UUID] = true
					changed = true
				}
			}
		}
	}
	for packUUID := range linked {
		updated := false
		for i, operation := range m.state.Pending {
			if operation.ServerID == serverID && operation.PackUUID == packUUID {
				m.state.Pending[i].Enable = enable
				updated = true
				break
			}
		}
		if !updated {
			m.state.Pending = append(m.state.Pending, Operation{serverID, packUUID, enable})
		}
	}
	return m.save()
}
func (m *Manager) Pending(serverID string) []Operation {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Operation
	for _, o := range m.state.Pending {
		if o.ServerID == serverID {
			out = append(out, o)
		}
	}
	if required, ok := m.state.PendingResourcePolicy[serverID]; ok {
		out = append(out, Operation{ServerID: serverID, PackUUID: "__resource_policy__", Enable: required})
	}
	return out
}

func (m *Manager) CancelPending(serverID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.servers[serverID]; !ok {
		return errors.New("server not found")
	}
	kept := m.state.Pending[:0]
	for _, operation := range m.state.Pending {
		if operation.ServerID != serverID {
			kept = append(kept, operation)
		}
	}
	m.state.Pending = kept
	delete(m.state.PendingResourcePolicy, serverID)
	return m.save()
}

func (m *Manager) ImportArchive(filename string) ([]StoredPack, error) {
	return m.ImportArchives([]string{filename})
}

func (m *Manager) ImportArchives(filenames []string) ([]StoredPack, error) {
	if len(filenames) == 0 {
		return nil, errors.New("at least one archive is required")
	}
	var incomingPacks []Pack
	for _, filename := range filenames {
		inspected, err := Inspect(filename, DefaultLimits)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(filename), err)
		}
		for _, p := range inspected.Packs {
			p.SourceArchive = filename
			incomingPacks = append(incomingPacks, p)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	available := map[string][3]int{}
	for _, existing := range m.state.Packs {
		available[existing.UUID] = existing.Version
	}
	for _, incoming := range incomingPacks {
		available[incoming.UUID] = incoming.Version
	}
	for _, incoming := range incomingPacks {
		for _, dependency := range incoming.Dependencies {
			if dependency.UUID == "" {
				continue
			}
			version, ok := available[dependency.UUID]
			if !ok {
				return nil, fmt.Errorf("pack %s requires missing pack %s", incoming.Name, dependency.UUID)
			}
			if versionGreater(dependency.Version, version) {
				return nil, fmt.Errorf("pack %s requires %s version %v", incoming.Name, dependency.UUID, dependency.Version)
			}
		}
	}
	var stored []StoredPack
	toInstall := make([]Pack, 0, len(incomingPacks))
	for _, incoming := range incomingPacks {
		alreadyInstalled := false
		for _, existing := range m.state.Packs {
			if existing.UUID != incoming.UUID {
				continue
			}
			if incoming.Version == existing.Version {
				// Re-importing the same file is a successful no-op. This is
				// especially useful when a multi-file selection is retried.
				stored = append(stored, existing)
				alreadyInstalled = true
				break
			}
			if !versionGreater(incoming.Version, existing.Version) {
				return nil, fmt.Errorf("pack UUID %s is older than the installed version", incoming.UUID)
			}
		}
		if !alreadyInstalled {
			toInstall = append(toInstall, incoming)
		}
	}
	var err error
	for _, p := range toInstall {
		dest := filepath.Join(m.root, "packs", p.UUID, fmt.Sprintf("%d.%d.%d", p.Version[0], p.Version[1], p.Version[2]))
		tmp := dest + ".tmp"
		if err = os.RemoveAll(tmp); err != nil {
			return nil, err
		}
		if len(p.ArchiveData) > 0 {
			err = extractPackBytes(p.ArchiveData, p.Root, tmp, DefaultLimits)
		} else {
			err = extractPack(p.SourceArchive, p.Root, tmp, DefaultLimits)
		}
		if err != nil {
			_ = os.RemoveAll(tmp)
			return nil, err
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return nil, err
		}
		if err = os.Rename(tmp, dest); err != nil {
			return nil, err
		}
		stored = append(stored, StoredPack{Pack: p, InstalledPath: dest})
	}
	for _, incoming := range stored {
		replaced := false
		for i, existing := range m.state.Packs {
			if existing.UUID == incoming.UUID {
				m.state.Packs[i] = incoming
				replaced = true
				break
			}
		}
		if !replaced {
			m.state.Packs = append(m.state.Packs, incoming)
		}
	}
	if err = m.save(); err != nil {
		return nil, err
	}
	return stored, nil
}

func versionGreater(a, b [3]int) bool {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
func (m *Manager) Uninstall(uuid string) error {
	return m.UninstallMany([]string{uuid})
}

func (m *Manager) UninstallPlan(uuids []string) ([]StoredPack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.uninstallPlanLocked(uuids)
}

func (m *Manager) uninstallPlanLocked(uuids []string) ([]StoredPack, error) {
	if len(uuids) == 0 {
		return nil, errors.New("at least one pack is required")
	}
	available := make(map[string]bool, len(m.state.Packs))
	linked := make(map[string]bool, len(uuids))
	for _, pack := range m.state.Packs {
		available[pack.UUID] = true
	}
	for _, uuid := range uuids {
		if !available[uuid] {
			return nil, fmt.Errorf("pack %s not found", uuid)
		}
		linked[uuid] = true
	}
	for changed := true; changed; {
		changed = false
		for _, pack := range m.state.Packs {
			for _, dependency := range pack.Dependencies {
				if dependency.UUID == "" {
					continue
				}
				if linked[pack.UUID] && !linked[dependency.UUID] {
					linked[dependency.UUID] = true
					changed = true
				}
				if linked[dependency.UUID] && !linked[pack.UUID] {
					linked[pack.UUID] = true
					changed = true
				}
			}
		}
	}
	for server, enabled := range m.state.Enabled {
		for uuid := range linked {
			if enabled[uuid] {
				return nil, fmt.Errorf("disable %s on %s before uninstalling", packName(m.state.Packs, uuid), server)
			}
		}
	}
	for _, op := range m.state.Pending {
		if linked[op.PackUUID] {
			return nil, fmt.Errorf("cancel or apply pending changes for %s before uninstalling", packName(m.state.Packs, op.PackUUID))
		}
	}
	plan := make([]StoredPack, 0, len(linked))
	for _, pack := range m.state.Packs {
		if linked[pack.UUID] {
			plan = append(plan, pack)
		}
	}
	return plan, nil
}

func packName(packs []StoredPack, uuid string) string {
	for _, pack := range packs {
		if pack.UUID == uuid {
			return pack.Name
		}
	}
	return uuid
}

func (m *Manager) UninstallMany(uuids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	plan, err := m.uninstallPlanLocked(uuids)
	if err != nil {
		return err
	}
	backupRoot := filepath.Join(m.root, "uninstall-backups", time.Now().Format("20060102-150405.000000000"))
	if err = os.MkdirAll(backupRoot, 0o750); err != nil {
		return err
	}
	type movedPack struct{ source, backup string }
	var moved []movedPack
	rollback := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			_ = os.MkdirAll(filepath.Dir(moved[i].source), 0o750)
			_ = os.Rename(moved[i].backup, moved[i].source)
		}
	}
	packsRoot, err := filepath.Abs(filepath.Join(m.root, "packs"))
	if err != nil {
		return err
	}
	for _, pack := range plan {
		source, absErr := filepath.Abs(filepath.Dir(pack.InstalledPath))
		if absErr != nil {
			rollback()
			return absErr
		}
		rel, relErr := filepath.Rel(packsRoot, source)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			rollback()
			return fmt.Errorf("refusing to uninstall pack outside managed storage: %s", source)
		}
		backup := filepath.Join(backupRoot, pack.UUID)
		if err = os.Rename(source, backup); err != nil {
			rollback()
			return fmt.Errorf("back up %s before uninstall: %w", pack.Name, err)
		}
		moved = append(moved, movedPack{source, backup})
	}
	removing := make(map[string]bool, len(plan))
	for _, pack := range plan {
		removing[pack.UUID] = true
	}
	oldPacks := m.state.Packs
	kept := make([]StoredPack, 0, len(oldPacks)-len(plan))
	for _, pack := range oldPacks {
		if !removing[pack.UUID] {
			kept = append(kept, pack)
		}
	}
	m.state.Packs = kept
	if err = m.save(); err != nil {
		m.state.Packs = oldPacks
		rollback()
		return err
	}
	return nil
}

func (m *Manager) Inventory() []StoredPack {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]StoredPack(nil), m.state.Packs...)
}

func (m *Manager) InventoryState() []InventoryItem {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]InventoryItem, 0, len(m.state.Packs))
	for _, pack := range m.state.Packs {
		item := InventoryItem{StoredPack: pack, Servers: map[string]AssignmentState{}}
		for serverID := range m.servers {
			state := AssignmentState{Enabled: m.state.Enabled[serverID][pack.UUID]}
			for _, operation := range m.state.Pending {
				if operation.ServerID == serverID && operation.PackUUID == pack.UUID {
					pending := operation.Enable
					state.Pending = &pending
				}
			}
			item.Servers[serverID] = state
		}
		items = append(items, item)
	}
	return items
}
