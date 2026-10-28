package serversettings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nethernet-multi-server/internal/config"
)

func testManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	properties := filepath.Join(root, "server.properties")
	content := "# keep this comment\nserver-ip=192.168.1.246\nserver-port=19134\nallow-cheats=false\nview-distance=24\ntick-distance=4\n"
	if err := os.WriteFile(properties, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(filepath.Join(root, "state"), []config.Server{{ID: "creative", Process: &config.Process{WorkingDirectory: root}}})
	if err != nil {
		t.Fatal(err)
	}
	return m, properties
}

func TestSaveChangesOnlyAllowlistedPropertiesAndCreatesBackup(t *testing.T) {
	m, properties := testManager(t)
	settings, err := m.Get("creative")
	if err != nil {
		t.Fatal(err)
	}
	settings.AllowCheats = true
	settings.ViewDistance = 32
	if err = m.Save("creative", settings); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(properties)
	text := string(b)
	for _, want := range []string{"# keep this comment", "server-ip=192.168.1.246", "server-port=19134", "allow-cheats=true", "view-distance=32"} {
		if !strings.Contains(text, want) {
			t.Fatalf("properties missing %q:\n%s", want, text)
		}
	}
	backups, _ := filepath.Glob(filepath.Join(m.root, "backups", "creative", "*", "server.properties"))
	if len(backups) != 1 {
		t.Fatalf("backups=%v", backups)
	}
	if !m.Pending("creative") {
		t.Fatal("save did not mark pending restart")
	}
}

func TestSaveRejectsUnsafeValuesWithoutChangingFile(t *testing.T) {
	m, properties := testManager(t)
	original, _ := os.ReadFile(properties)
	settings, _ := m.Get("creative")
	settings.TickDistance = 99
	if err := m.Save("creative", settings); err == nil {
		t.Fatal("invalid tick distance accepted")
	}
	after, _ := os.ReadFile(properties)
	if string(after) != string(original) {
		t.Fatal("invalid save changed properties")
	}
}

func TestPendingSurvivesReloadAndCanBeCleared(t *testing.T) {
	m, _ := testManager(t)
	settings, _ := m.Get("creative")
	if err := m.Save("creative", settings); err != nil {
		t.Fatal(err)
	}
	m2, err := NewManager(m.root, []config.Server{{ID: "creative", Process: m.servers["creative"].Process}})
	if err != nil {
		t.Fatal(err)
	}
	if !m2.Pending("creative") {
		t.Fatal("pending flag did not survive reload")
	}
	if err = m2.ClearPending("creative"); err != nil {
		t.Fatal(err)
	}
	if m2.Pending("creative") {
		t.Fatal("pending flag was not cleared")
	}
}
