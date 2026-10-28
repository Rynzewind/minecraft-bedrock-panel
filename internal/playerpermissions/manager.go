package playerpermissions

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

type Entry struct {
	Permission string `json:"permission"`
	XUID       string `json:"xuid"`
}

type fileState struct {
	path    string
	existed bool
	data    []byte
	entries []Entry
}

type Manager struct {
	mu      sync.Mutex
	root    string
	servers []config.Server
}

func NewManager(root string, servers []config.Server) (*Manager, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	return &Manager{root: root, servers: append([]config.Server(nil), servers...)}, nil
}

func (m *Manager) SetAll(xuid, role string) error {
	xuid, role = strings.TrimSpace(xuid), strings.ToLower(strings.TrimSpace(role))
	if xuid == "" {
		return errors.New("player XUID is required")
	}
	if role != "default" && role != "visitor" && role != "member" && role != "operator" {
		return errors.New("role must be default, visitor, member, or operator")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	states, err := m.readAll()
	if err != nil {
		return err
	}
	backup := filepath.Join(m.root, "backups", time.Now().Format("20060102-150405.000000000"))
	if err = os.MkdirAll(backup, 0o750); err != nil {
		return err
	}
	for i := range states {
		if states[i].existed {
			if err = os.WriteFile(filepath.Join(backup, fmt.Sprintf("%02d-permissions.json", i)), states[i].data, 0o600); err != nil {
				return err
			}
		}
		updated := make([]Entry, 0, len(states[i].entries)+1)
		for _, entry := range states[i].entries {
			if entry.XUID != xuid {
				updated = append(updated, entry)
			}
		}
		if role != "default" {
			updated = append(updated, Entry{Permission: role, XUID: xuid})
		}
		states[i].entries = updated
	}
	for i := range states {
		if err = writeEntries(states[i].path, states[i].entries); err != nil {
			for j := 0; j < i; j++ {
				_ = restore(states[j])
			}
			return err
		}
	}
	return nil
}

func (m *Manager) Roles() (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	states, err := m.readAll()
	if err != nil {
		return nil, err
	}
	all := map[string]bool{}
	for _, state := range states {
		for _, entry := range state.entries {
			all[entry.XUID] = true
		}
	}
	out := map[string]string{}
	for xuid := range all {
		role, mixed := "", false
		for _, state := range states {
			current := "default"
			for _, entry := range state.entries {
				if entry.XUID == xuid {
					current = entry.Permission
					break
				}
			}
			if role == "" {
				role = current
			} else if role != current {
				mixed = true
			}
		}
		if mixed {
			out[xuid] = "Mixed"
		} else {
			out[xuid] = title(role)
		}
	}
	return out, nil
}

func (m *Manager) readAll() ([]fileState, error) {
	states := make([]fileState, 0, len(m.servers))
	for _, server := range m.servers {
		if server.Process == nil {
			continue
		}
		state := fileState{path: filepath.Join(server.Process.WorkingDirectory, "permissions.json")}
		data, err := os.ReadFile(state.path)
		if errors.Is(err, os.ErrNotExist) {
			data, err = []byte("[]"), nil
		} else {
			state.existed = err == nil
		}
		if err != nil {
			return nil, err
		}
		state.data = data
		if err = json.Unmarshal(data, &state.entries); err != nil {
			return nil, fmt.Errorf("decode %s: %w", state.path, err)
		}
		states = append(states, state)
	}
	if len(states) == 0 {
		return nil, errors.New("no configured servers")
	}
	return states, nil
}

func writeEntries(path string, entries []Entry) error {
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func restore(state fileState) error {
	if !state.existed {
		return os.Remove(state.path)
	}
	return os.WriteFile(state.path, state.data, 0o600)
}

func title(role string) string {
	if role == "" || role == "default" {
		return "Default"
	}
	return strings.ToUpper(role[:1]) + role[1:]
}
