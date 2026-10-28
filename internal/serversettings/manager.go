package serversettings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nethernet-multi-server/internal/config"
)

type Settings struct {
	GameMode                  string `json:"game_mode"`
	Difficulty                string `json:"difficulty"`
	AllowCheats               bool   `json:"allow_cheats"`
	ForceGameMode             bool   `json:"force_game_mode"`
	MaxPlayers                int    `json:"max_players"`
	ViewDistance              int    `json:"view_distance"`
	TickDistance              int    `json:"tick_distance"`
	PlayerIdleTimeout         int    `json:"player_idle_timeout"`
	DefaultPermission         string `json:"default_permission"`
	AllowList                 bool   `json:"allow_list"`
	PlayerVsPlayer            bool   `json:"player_vs_player"`
	ClientSideChunkGeneration bool   `json:"client_side_chunk_generation"`
}

type persistentState struct {
	Pending map[string]bool `json:"pending"`
}
type Manager struct {
	mu              sync.Mutex
	root, statePath string
	servers         map[string]config.Server
	state           persistentState
}

func NewManager(root string, servers []config.Server) (*Manager, error) {
	m := &Manager{root: root, statePath: filepath.Join(root, "state.json"), servers: map[string]config.Server{}, state: persistentState{Pending: map[string]bool{}}}
	for _, server := range servers {
		m.servers[server.ID] = server
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	if b, err := os.ReadFile(m.statePath); err == nil {
		if err = json.Unmarshal(b, &m.state); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if m.state.Pending == nil {
		m.state.Pending = map[string]bool{}
	}
	return m, nil
}

func (m *Manager) propertiesPath(serverID string) (string, error) {
	server, ok := m.servers[serverID]
	if !ok || server.Process == nil {
		return "", errors.New("server settings are not configured")
	}
	return filepath.Join(server.Process.WorkingDirectory, "server.properties"), nil
}

func (m *Manager) Get(serverID string) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.propertiesPath(serverID)
	if err != nil {
		return Settings{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, err
	}
	values := parseProperties(string(b))
	return Settings{
		GameMode: textValue(values, "gamemode", "survival"), Difficulty: textValue(values, "difficulty", "normal"),
		AllowCheats: boolValue(values, "allow-cheats", false), ForceGameMode: boolValue(values, "force-gamemode", false),
		MaxPlayers: intValue(values, "max-players", 10), ViewDistance: intValue(values, "view-distance", 24), TickDistance: intValue(values, "tick-distance", 4),
		PlayerIdleTimeout: intValue(values, "player-idle-timeout", 0), DefaultPermission: textValue(values, "default-player-permission-level", "member"),
		AllowList: boolValue(values, "allow-list", false), PlayerVsPlayer: boolValue(values, "player-vs-player", true),
		ClientSideChunkGeneration: boolValue(values, "client-side-chunk-generation-enabled", true),
	}, nil
}

func (m *Manager) Save(serverID string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.propertiesPath(serverID)
	if err != nil {
		return err
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backup := filepath.Join(m.root, "backups", serverID, time.Now().Format("20060102-150405.000000000"))
	if err = os.MkdirAll(backup, 0o750); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(backup, "server.properties"), original, 0o600); err != nil {
		return err
	}
	updates := map[string]string{
		"gamemode": settings.GameMode, "difficulty": settings.Difficulty, "allow-cheats": strconv.FormatBool(settings.AllowCheats),
		"force-gamemode": strconv.FormatBool(settings.ForceGameMode), "max-players": strconv.Itoa(settings.MaxPlayers), "view-distance": strconv.Itoa(settings.ViewDistance),
		"tick-distance": strconv.Itoa(settings.TickDistance), "player-idle-timeout": strconv.Itoa(settings.PlayerIdleTimeout),
		"default-player-permission-level": settings.DefaultPermission, "allow-list": strconv.FormatBool(settings.AllowList),
		"player-vs-player": strconv.FormatBool(settings.PlayerVsPlayer), "client-side-chunk-generation-enabled": strconv.FormatBool(settings.ClientSideChunkGeneration),
	}
	updated := updateProperties(string(original), updates)
	tmp := path + ".settings.tmp"
	if err = os.WriteFile(tmp, []byte(updated), 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	m.state.Pending[serverID] = true
	return m.saveState()
}

func (m *Manager) Pending(serverID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.Pending[serverID]
}
func (m *Manager) ClearPending(serverID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.Pending, serverID)
	return m.saveState()
}
func (m *Manager) saveState() error {
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

func (s Settings) Validate() error {
	if !oneOf(s.GameMode, "survival", "creative", "adventure") {
		return errors.New("game mode must be survival, creative, or adventure")
	}
	if !oneOf(s.Difficulty, "peaceful", "easy", "normal", "hard") {
		return errors.New("difficulty must be peaceful, easy, normal, or hard")
	}
	if !oneOf(s.DefaultPermission, "visitor", "member", "operator") {
		return errors.New("default permission must be visitor, member, or operator")
	}
	if s.MaxPlayers < 1 || s.MaxPlayers > 1000 {
		return errors.New("maximum players must be between 1 and 1000")
	}
	if s.ViewDistance < 5 || s.ViewDistance > 96 {
		return errors.New("view distance must be between 5 and 96")
	}
	if s.TickDistance < 4 || s.TickDistance > 12 {
		return errors.New("simulation distance must be between 4 and 12")
	}
	if s.PlayerIdleTimeout < 0 || s.PlayerIdleTimeout > 10080 {
		return errors.New("idle timeout must be between 0 and 10080 minutes")
	}
	return nil
}
func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
func parseProperties(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if i := strings.Index(trimmed, "="); i > 0 {
			out[strings.TrimSpace(trimmed[:i])] = strings.TrimSpace(trimmed[i+1:])
		}
	}
	return out
}
func textValue(v map[string]string, k, d string) string {
	if x, ok := v[k]; ok {
		return strings.ToLower(x)
	}
	return d
}
func boolValue(v map[string]string, k string, d bool) bool {
	if x, ok := v[k]; ok {
		b, e := strconv.ParseBool(x)
		if e == nil {
			return b
		}
	}
	return d
}
func intValue(v map[string]string, k string, d int) int {
	if x, ok := v[k]; ok {
		n, e := strconv.Atoi(x)
		if e == nil {
			return n
		}
	}
	return d
}
func updateProperties(text string, updates map[string]string) string {
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}
		if n := strings.Index(trim, "="); n > 0 {
			key := strings.TrimSpace(trim[:n])
			if value, ok := updates[key]; ok {
				lines[i] = key + "=" + value
				seen[key] = true
			}
		}
	}
	for key, value := range updates {
		if !seen[key] {
			lines = append(lines, key+"="+value)
		}
	}
	return strings.Join(lines, newline)
}
