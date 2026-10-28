package config

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	netherdiscovery "github.com/df-mc/go-nethernet/discovery"
)

func TestIdentityDirectoryIsBesideConfig(t *testing.T) {
	path := filepath.Join(`C:\portable\nethernet`, "config.json")
	want := filepath.Join(filepath.Dir(path), "identities")
	if got := IdentityDirectory(path); got != want {
		t.Fatalf("IdentityDirectory() = %q, want %q", got, want)
	}
}

func TestAuthTokenPathIsBesideConfig(t *testing.T) {
	path := filepath.Join(`C:\portable\nethernet`, "config.json")
	want := filepath.Join(filepath.Dir(path), "auth-token.json")
	if got := AuthTokenPath(path); got != want {
		t.Fatalf("AuthTokenPath() = %q, want %q", got, want)
	}
}

func TestServerBackendURL(t *testing.T) {
	s := Server{Backend: "192.168.1.246:19134"}
	if got := s.BackendURL(); got != "http://192.168.1.246:19134" {
		t.Fatalf("BackendURL() = %q", got)
	}
}

func validServer() Server {
	return Server{
		Name:                  "Private Creative Server",
		LevelName:             "Private Creative Server",
		Backend:               "192.168.1.246:19134",
		NetworkID:             "13468123134000001934",
		Protocol:              2193,
		Version:               "1.26.51",
		GameType:              "creative",
		PlayerCount:           0,
		MaxPlayerCount:        10,
		AcceptsOnlineAuth:     true,
		AcceptsSelfSignedAuth: true,
		ConnectionType:        4,
	}
}

func validConfig() Config {
	return Config{InterfaceIP: "192.168.1.246", Listen: "192.168.1.255:7551", Servers: []Server{validServer()}}
}

func TestServerParsedNetworkIDPreservesUint64(t *testing.T) {
	s := Server{NetworkID: "18446744073709551615"}
	got, err := s.ParsedNetworkID()
	if err != nil || got != math.MaxUint64 {
		t.Fatalf("ParsedNetworkID() = %d, %v; want %d, nil", got, err, uint64(math.MaxUint64))
	}
}

func TestValidateRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "empty server list", mutate: func(c *Config) { c.Servers = nil }, want: "at least one server"},
		{name: "empty interface IP", mutate: func(c *Config) { c.InterfaceIP = "" }, want: "interface_ip"},
		{name: "non-IPv4 interface IP", mutate: func(c *Config) { c.InterfaceIP = "::1" }, want: "interface_ip"},
		{name: "wildcard listen address", mutate: func(c *Config) { c.Listen = "0.0.0.0:7551" }, want: "broadcast"},
		{name: "zero network ID", mutate: func(c *Config) { c.Servers[0].NetworkID = "0" }, want: "network_id"},
		{name: "non-decimal network ID", mutate: func(c *Config) { c.Servers[0].NetworkID = "not-a-number" }, want: "network_id"},
		{name: "overflow network ID", mutate: func(c *Config) { c.Servers[0].NetworkID = "18446744073709551616" }, want: "network_id"},
		{name: "empty name", mutate: func(c *Config) { c.Servers[0].Name = "" }, want: "name"},
		{name: "empty level name", mutate: func(c *Config) { c.Servers[0].LevelName = "" }, want: "level_name"},
		{name: "invalid backend", mutate: func(c *Config) { c.Servers[0].Backend = "192.168.1.246" }, want: "backend"},
		{name: "empty backend host", mutate: func(c *Config) { c.Servers[0].Backend = ":19134" }, want: "backend"},
		{name: "backend hostname instead of IP", mutate: func(c *Config) { c.Servers[0].Backend = "localhost:19134" }, want: "backend"},
		{name: "nonnumeric backend port", mutate: func(c *Config) { c.Servers[0].Backend = "192.168.1.246:minecraft" }, want: "backend"},
		{name: "zero backend port", mutate: func(c *Config) { c.Servers[0].Backend = "192.168.1.246:0" }, want: "backend"},
		{name: "oversized backend port", mutate: func(c *Config) { c.Servers[0].Backend = "192.168.1.246:65536" }, want: "backend"},
		{name: "zero protocol", mutate: func(c *Config) { c.Servers[0].Protocol = 0 }, want: "protocol"},
		{name: "empty version", mutate: func(c *Config) { c.Servers[0].Version = "" }, want: "version"},
		{name: "unknown game type", mutate: func(c *Config) { c.Servers[0].GameType = "builder" }, want: "game_type"},
		{name: "negative players", mutate: func(c *Config) { c.Servers[0].PlayerCount = -1 }, want: "player_count"},
		{name: "max below players", mutate: func(c *Config) { c.Servers[0].PlayerCount = 2; c.Servers[0].MaxPlayerCount = 1 }, want: "max_player_count"},
		{name: "zero max players", mutate: func(c *Config) { c.Servers[0].MaxPlayerCount = 0 }, want: "max_player_count"},
		{name: "zero connection type", mutate: func(c *Config) { c.Servers[0].ConnectionType = 0 }, want: "connection_type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v; want error containing %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsDuplicateNetworkIDs(t *testing.T) {
	cfg := validConfig()
	second := validServer()
	second.Name = "Private Survival Server"
	cfg.Servers = append(cfg.Servers, second)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate network ID") {
		t.Fatalf("Validate() error = %v; want duplicate network ID", err)
	}
}

func TestLegacyServerWithoutProcessRemainsValid(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("legacy configuration rejected: %v", err)
	}
}

func TestProcessConfigurationRequiresSafeAbsolutePaths(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Process)
	}{
		{name: "relative executable", mutate: func(p *Process) { p.Executable = `bedrock_server.exe` }},
		{name: "relative working directory", mutate: func(p *Process) { p.WorkingDirectory = `Creative` }},
		{name: "world outside root", mutate: func(p *Process) { p.WorldDirectory = `C:\MinecraftServers\Survival\worlds\Other` }},
		{name: "log outside root", mutate: func(p *Process) { p.LogDirectory = `C:\Temp\logs` }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Servers[0].ID = "creative"
			cfg.Servers[0].Process = &Process{
				Executable: `C:\MinecraftServers\Creative\bedrock_server.exe`, WorkingDirectory: `C:\MinecraftServers\Creative`,
				WorldDirectory: `C:\MinecraftServers\Creative\worlds\Private Creative Server`, LogDirectory: `C:\MinecraftServers\Creative\logs`,
			}
			test.mutate(cfg.Servers[0].Process)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() succeeded; want unsafe process path error")
			}
		})
	}
}

func TestServerIDsMustBeUnique(t *testing.T) {
	cfg := validConfig()
	cfg.Servers[0].ID = "same"
	second := validServer()
	second.ID = "same"
	second.NetworkID = "13468123136000001936"
	cfg.Servers = append(cfg.Servers, second)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate server ID") {
		t.Fatalf("Validate() error = %v; want duplicate server ID", err)
	}
}

func TestLoadDefaultsListenAndRejectsUnknownFieldsOrTrailingJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "unknown field", content: `{"servers":[],"surprise":true}`, wantErr: "unknown field"},
		{name: "trailing JSON", content: `{"servers":[]} {}`, wantErr: "single JSON object"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Load() error = %v; want error containing %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadDefaultsMissingListenAddress(t *testing.T) {
	content, err := json.Marshal(Config{InterfaceIP: "192.168.1.246", Servers: []Server{validServer()}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "192.168.1.255:7551" {
		t.Fatalf("Load().Listen = %q; want 192.168.1.255:7551", cfg.Listen)
	}
}

func TestServerDiscoveryData(t *testing.T) {
	tests := []struct {
		gameType string
		want     int32
	}{
		{gameType: "creative", want: netherdiscovery.GameTypeCreative},
		{gameType: "Survival", want: netherdiscovery.GameTypeSurvival},
		{gameType: "ADVENTURE", want: netherdiscovery.GameTypeAdventure},
	}
	for _, test := range tests {
		t.Run(test.gameType, func(t *testing.T) {
			s := validServer()
			s.GameType = test.gameType
			s.Nonce = "abc123"
			got, err := s.DiscoveryData()
			if err != nil {
				t.Fatal(err)
			}
			if got.ServerName != "\u00a0" || got.LevelName != s.LevelName || got.Protocol != 2193 || got.Version != "1.26.51" || got.GameType != test.want || got.PlayerCount != 0 || got.MaxPlayerCount != 10 || !got.AcceptsOnlineAuth || !got.AcceptsSelfSignedAuth || got.Nonce != "abc123" || got.ConnectionType != 4 {
				t.Fatalf("DiscoveryData() = %+v; fields do not match source server", got)
			}
		})
	}
}

func TestServerDiscoveryDataUsesInvisibleNonEmptyDefaultSubtitle(t *testing.T) {
	s := validServer()
	s.Subtitle = ""
	data, err := s.DiscoveryData()
	if err != nil {
		t.Fatal(err)
	}
	if data.ServerName != "\u00a0" || data.LevelName != "Private Creative Server" {
		t.Fatalf("menu labels = subtitle %q, title %q", data.ServerName, data.LevelName)
	}
}

func TestServerDiscoveryDataUsesConfiguredSubtitle(t *testing.T) {
	s := validServer()
	s.Subtitle = "Kevs Private Server"
	data, err := s.DiscoveryData()
	if err != nil {
		t.Fatal(err)
	}
	if data.ServerName != "Kevs Private Server" || data.LevelName != "Private Creative Server" {
		t.Fatalf("menu labels = subtitle %q, title %q", data.ServerName, data.LevelName)
	}
}

func TestServerDiscoveryDataRejectsUnknownGameType(t *testing.T) {
	s := validServer()
	s.GameType = "builder"
	if _, err := s.DiscoveryData(); err == nil || !strings.Contains(err.Error(), "game_type") {
		t.Fatalf("DiscoveryData() error = %v; want game_type error", err)
	}
}
