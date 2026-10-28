package playerpermissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"nethernet-multi-server/internal/config"
)

func TestSetAllPreservesOtherPlayersAndUpdatesEveryServer(t *testing.T) {
	root := t.TempDir()
	servers := []config.Server{
		{ID: "creative", Process: &config.Process{WorkingDirectory: filepath.Join(root, "creative")}},
		{ID: "survival", Process: &config.Process{WorkingDirectory: filepath.Join(root, "survival")}},
	}
	for _, server := range servers {
		if err := os.MkdirAll(server.Process.WorkingDirectory, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(server.Process.WorkingDirectory, "permissions.json"), []byte(`[{"permission":"member","xuid":"other"}]`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := NewManager(filepath.Join(root, "state"), servers)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SetAll("player", "operator"); err != nil {
		t.Fatal(err)
	}
	for _, server := range servers {
		var entries []Entry
		b, err := os.ReadFile(filepath.Join(server.Process.WorkingDirectory, "permissions.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(b, &entries); err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 || entries[0].XUID != "other" || entries[1] != (Entry{Permission: "operator", XUID: "player"}) {
			t.Fatalf("entries=%+v", entries)
		}
	}
	roles, err := m.Roles()
	if err != nil || roles["player"] != "Operator" {
		t.Fatalf("roles=%+v err=%v", roles, err)
	}
}

func TestDefaultRemovesExplicitRoleFromEveryServer(t *testing.T) {
	root := t.TempDir()
	server := config.Server{ID: "creative", Process: &config.Process{WorkingDirectory: root}}
	if err := os.WriteFile(filepath.Join(root, "permissions.json"), []byte(`[{"permission":"operator","xuid":"player"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(filepath.Join(root, "state"), []config.Server{server})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SetAll("player", "default"); err != nil {
		t.Fatal(err)
	}
	roles, err := m.Roles()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := roles["player"]; ok {
		t.Fatalf("roles=%+v", roles)
	}
}
