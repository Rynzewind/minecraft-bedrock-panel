package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	netherdiscovery "github.com/df-mc/go-nethernet/discovery"
)

func IdentityDirectory(configPath string) string {
	absolute, err := filepath.Abs(configPath)
	if err != nil {
		return filepath.Join(filepath.Dir(configPath), "identities")
	}
	return filepath.Join(filepath.Dir(absolute), "identities")
}

func AuthTokenPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "auth-token.json")
}

func AccountStorePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "account-store", "accounts.json")
}

const defaultListenAddress = "192.168.1.255:7551"

type Config struct {
	InterfaceIP string   `json:"interface_ip"`
	Listen      string   `json:"listen"`
	Servers     []Server `json:"servers"`
}

type Server struct {
	ID                    string   `json:"id,omitempty"`
	Process               *Process `json:"process,omitempty"`
	Name                  string   `json:"name"`
	Subtitle              string   `json:"subtitle,omitempty"`
	LevelName             string   `json:"level_name"`
	Backend               string   `json:"backend"`
	NetworkID             string   `json:"network_id"`
	Protocol              int32    `json:"protocol"`
	Version               string   `json:"version"`
	GameType              string   `json:"game_type"`
	PlayerCount           int32    `json:"player_count"`
	MaxPlayerCount        int32    `json:"max_player_count"`
	AcceptsOnlineAuth     bool     `json:"accepts_online_auth"`
	AcceptsSelfSignedAuth bool     `json:"accepts_self_signed_auth"`
	Nonce                 string   `json:"nonce,omitempty"`
	ConnectionType        int32    `json:"connection_type"`
}

type Process struct {
	Executable       string `json:"executable"`
	WorkingDirectory string `json:"working_directory"`
	WorldDirectory   string `json:"world_directory"`
	LogDirectory     string `json:"log_directory"`
}

func (s Server) ProcessConfigured() bool { return s.Process != nil }

func (s Server) BackendURL() string { return "http://" + s.Backend }

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	var cfg Config
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, errors.New("configuration must contain a single JSON object")
		}
		return Config{}, fmt.Errorf("configuration must contain a single JSON object: %w", err)
	}
	if strings.TrimSpace(cfg.Listen) == "" {
		cfg.Listen = defaultListenAddress
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	interfaceIP := net.ParseIP(strings.TrimSpace(c.InterfaceIP))
	if interfaceIP == nil || interfaceIP.To4() == nil {
		return fmt.Errorf("interface_ip %q must be an IPv4 address", c.InterfaceIP)
	}
	listenAddress, err := net.ResolveUDPAddr("udp4", c.Listen)
	if err != nil {
		return fmt.Errorf("listen address %q: %w", c.Listen, err)
	}
	if listenAddress.IP == nil || listenAddress.IP.IsUnspecified() {
		return fmt.Errorf("listen address %q must use the Ethernet subnet broadcast address, not a wildcard", c.Listen)
	}
	if len(c.Servers) == 0 {
		return errors.New("at least one server is required")
	}
	seen := make(map[uint64]int, len(c.Servers))
	seenIDs := make(map[string]int, len(c.Servers))
	for i, server := range c.Servers {
		prefix := fmt.Sprintf("servers[%d]", i)
		if strings.TrimSpace(server.Name) == "" {
			return fmt.Errorf("%s.name must not be empty", prefix)
		}
		serverID := strings.ToLower(strings.TrimSpace(server.ID))
		if serverID == "" {
			serverID = strings.ToLower(strings.TrimSpace(server.Name))
		}
		if serverID == "" {
			return fmt.Errorf("%s.id must not be empty", prefix)
		}
		if previous, ok := seenIDs[serverID]; ok {
			return fmt.Errorf("%s.id: duplicate server ID also used by servers[%d]", prefix, previous)
		}
		seenIDs[serverID] = i
		id, err := server.ParsedNetworkID()
		if err != nil {
			return fmt.Errorf("%s.network_id: %w", prefix, err)
		}
		if previous, ok := seen[id]; ok {
			return fmt.Errorf("%s.network_id: duplicate network ID also used by servers[%d]", prefix, previous)
		}
		seen[id] = i
		if strings.TrimSpace(server.LevelName) == "" {
			return fmt.Errorf("%s.level_name must not be empty", prefix)
		}
		host, portText, err := net.SplitHostPort(server.Backend)
		if err != nil {
			return fmt.Errorf("%s.backend %q: %w", prefix, server.Backend, err)
		}
		if net.ParseIP(host) == nil {
			return fmt.Errorf("%s.backend %q: host must be an IP address", prefix, server.Backend)
		}
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("%s.backend %q: port must be an integer from 1 through 65535", prefix, server.Backend)
		}
		if server.Protocol < 1 {
			return fmt.Errorf("%s.protocol must be positive", prefix)
		}
		if strings.TrimSpace(server.Version) == "" {
			return fmt.Errorf("%s.version must not be empty", prefix)
		}
		if _, err := gameTypeValue(server.GameType); err != nil {
			return fmt.Errorf("%s.game_type: %w", prefix, err)
		}
		if server.PlayerCount < 0 {
			return fmt.Errorf("%s.player_count must not be negative", prefix)
		}
		if server.MaxPlayerCount < 1 || server.MaxPlayerCount < server.PlayerCount {
			return fmt.Errorf("%s.max_player_count must be positive and at least player_count", prefix)
		}
		if server.ConnectionType < 1 {
			return fmt.Errorf("%s.connection_type must be positive", prefix)
		}
		if server.Process != nil {
			if err := validateProcess(*server.Process); err != nil {
				return fmt.Errorf("%s.process: %w", prefix, err)
			}
		}
	}
	return nil
}

func validateProcess(p Process) error {
	paths := []struct{ name, value string }{
		{"executable", p.Executable}, {"working_directory", p.WorkingDirectory},
		{"world_directory", p.WorldDirectory}, {"log_directory", p.LogDirectory},
	}
	for _, item := range paths {
		if !filepath.IsAbs(strings.TrimSpace(item.value)) {
			return fmt.Errorf("%s must be an absolute path", item.name)
		}
	}
	root := filepath.Clean(p.WorkingDirectory)
	if !strings.EqualFold(filepath.Dir(filepath.Clean(p.Executable)), root) {
		return errors.New("executable must be directly inside working_directory")
	}
	for _, item := range []struct{ name, value string }{{"world_directory", p.WorldDirectory}, {"log_directory", p.LogDirectory}} {
		rel, err := filepath.Rel(root, filepath.Clean(item.value))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("%s must be inside working_directory", item.name)
		}
	}
	return nil
}

func (s Server) ParsedNetworkID() (uint64, error) {
	id, err := strconv.ParseUint(s.NetworkID, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("must be a decimal uint64: %w", err)
	}
	if id == 0 {
		return 0, errors.New("must be non-zero")
	}
	return id, nil
}

func (s Server) DiscoveryData() (*netherdiscovery.ServerData, error) {
	gameType, err := gameTypeValue(s.GameType)
	if err != nil {
		return nil, fmt.Errorf("game_type: %w", err)
	}
	subtitle := s.Subtitle
	if subtitle == "" {
		subtitle = "\u00a0"
	}
	return &netherdiscovery.ServerData{
		ServerName:            subtitle,
		Protocol:              s.Protocol,
		Version:               s.Version,
		LevelName:             s.LevelName,
		GameType:              gameType,
		PlayerCount:           s.PlayerCount,
		MaxPlayerCount:        s.MaxPlayerCount,
		EditorWorld:           false,
		Hardcore:              false,
		AcceptsOnlineAuth:     s.AcceptsOnlineAuth,
		AcceptsSelfSignedAuth: s.AcceptsSelfSignedAuth,
		Nonce:                 s.Nonce,
		ConnectionType:        s.ConnectionType,
	}, nil
}

func gameTypeValue(value string) (int32, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "survival":
		return netherdiscovery.GameTypeSurvival, nil
	case "creative":
		return netherdiscovery.GameTypeCreative, nil
	case "adventure":
		return netherdiscovery.GameTypeAdventure, nil
	default:
		return 0, fmt.Errorf("unknown value %q (want survival, creative, or adventure)", value)
	}
}
