package serverupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"testing"
)

type fakeSupervisor struct {
	states    []supervisor.Status
	stops     int
	starts    int
	failStart bool
}

func (s *fakeSupervisor) Statuses() []supervisor.Status {
	return append([]supervisor.Status(nil), s.states...)
}
func (s *fakeSupervisor) Stop(_ context.Context, id string) error {
	s.stops++
	for i := range s.states {
		if s.states[i].ID == id {
			s.states[i].State = supervisor.Stopped
		}
	}
	return nil
}
func (s *fakeSupervisor) Start(_ context.Context, id string) error {
	s.starts++
	if s.failStart {
		return fmt.Errorf("simulated startup failure")
	}
	for i := range s.states {
		if s.states[i].ID == id {
			s.states[i].State = supervisor.Running
			s.states[i].Ready = true
		}
	}
	return nil
}
func (s *fakeSupervisor) RecentLog(string) string { return "Version: 1.26.60.6" }
func fixture(t *testing.T) (*Manager, *fakeSupervisor, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Creative")
	os.MkdirAll(filepath.Join(dir, "worlds", "world"), 0700)
	os.MkdirAll(filepath.Join(dir, "behavior_packs", "custom"), 0700)
	os.WriteFile(filepath.Join(dir, "worlds", "world", "level.dat"), []byte("world"), 0600)
	os.WriteFile(filepath.Join(dir, "server.properties"), []byte("custom settings"), 0600)
	os.WriteFile(filepath.Join(dir, "bedrock_server.exe"), []byte("MZold"), 0600)
	os.WriteFile(filepath.Join(dir, "behavior_packs", "custom", "manifest.json"), []byte("custom pack"), 0600)
	cfg := config.Config{Servers: []config.Server{{ID: "creative", Version: "1.26.51", Protocol: 2193, Process: &config.Process{WorkingDirectory: dir, Executable: filepath.Join(dir, "bedrock_server.exe")}}}}
	b, _ := json.Marshal(cfg)
	path := filepath.Join(root, "MultiServer", "config.json")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, b, 0600)
	s := &fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running, Ready: true}}}
	m := New(root, path, s)
	return m, s, dir
}
func TestUpdatePreservesWorldsSettingsAndCustomPacks(t *testing.T) {
	m, s, dir := fixture(t)
	r, _ := parseRelease("https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-1.26.60.6.zip")
	data := archive(t, map[string]string{"bedrock_server.exe": "MZnew", "server.properties": "bad", "behavior_packs/custom/manifest.json": "bad", "behavior_packs/vanilla/manifest.json": "stock"})
	if e := m.apply(context.Background(), r, data); e != nil {
		t.Fatal(e)
	}
	for n, want := range map[string]string{"worlds/world/level.dat": "world", "server.properties": "custom settings", "behavior_packs/custom/manifest.json": "custom pack", "bedrock_server.exe": "MZnew"} {
		b, e := os.ReadFile(filepath.Join(dir, filepath.FromSlash(n)))
		if e != nil || string(b) != want {
			t.Fatalf("%s: %s %v", n, b, e)
		}
	}
	if s.stops != 1 || s.starts != 1 {
		t.Fatal(s)
	}
	if m.Status().Backup == "" {
		t.Fatal("missing backup")
	}
}
func TestPlayersAndFutureProtocolRefusedBeforeStop(t *testing.T) {
	for _, players := range []bool{true, false} {
		m, s, _ := fixture(t)
		r, _ := parseRelease("https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-1.26.70.1.zip")
		if players {
			r, _ = parseRelease("https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-1.26.60.6.zip")
			s.states[0].Players = 1
		}
		if e := m.apply(context.Background(), r, nil); e == nil {
			t.Fatal("expected refusal")
		}
		if s.stops != 0 {
			t.Fatal("stopped before validation")
		}
	}
}
func TestInterruptedDownloadDoesNotRequireWorldRestore(t *testing.T) {
	m, s, _ := fixture(t)
	m.status = Status{Phase: "downloading", Done: false}
	if e := m.save(); e != nil {
		t.Fatal(e)
	}
	n := New(m.root, m.path, s)
	if n.Status().Recovery {
		t.Fatal("no server files changed during download")
	}
}
func TestRecoveryRejectsUnverifiedBackup(t *testing.T) {
	m, _, _ := fixture(t)
	dir := filepath.Join(m.root, "MultiServer", "backups", "server-updates", "partial")
	os.MkdirAll(dir, 0700)
	m.status = Status{Recovery: true, Backup: dir}
	if _, e := m.Recover(context.Background(), true); e == nil {
		t.Fatal("incomplete backup restored")
	}
}
func TestNewNonVanillaPackCopiesAllFiles(t *testing.T) {
	m, _, dir := fixture(t)
	stage := filepath.Join(m.root, "stage")
	os.MkdirAll(filepath.Join(stage, "behavior_packs", "newpack"), 0700)
	for _, n := range []string{"a", "b"} {
		os.WriteFile(filepath.Join(stage, "behavior_packs", "newpack", n), []byte(n), 0600)
	}
	if e := installSoftware(stage, dir); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(dir, "behavior_packs", "newpack", "b")); e != nil {
		t.Fatal(e)
	}
}
func TestRefuseDowngradeAndExactVersionParsing(t *testing.T) {
	if compareVersion("1.26.60.60", "1.26.60.6") <= 0 {
		t.Fatal("numeric version comparison")
	}
	if logVersion("[INFO] Version: 1.26.60.60\r\n") != "1.26.60.60" {
		t.Fatal("exact version parse")
	}
	m, s, dir := fixture(t)
	os.WriteFile(filepath.Join(dir, "release-notes.txt"), []byte(""), 0600)
	b, _ := os.ReadFile(m.path)
	var cfg config.Config
	json.Unmarshal(b, &cfg)
	cfg.Servers[0].Version = "1.26.70"
	b, _ = json.Marshal(cfg)
	os.WriteFile(m.path, b, 0600)
	r, _ := parseRelease("https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-1.26.60.6.zip")
	if e := m.apply(context.Background(), r, nil); e == nil {
		t.Fatal("downgrade accepted")
	}
	if s.stops != 0 {
		t.Fatal("downgrade stopped server")
	}
}
func TestBackupTamperBlocksRecovery(t *testing.T) {
	m, s, _ := fixture(t)
	r, _ := parseRelease("https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-1.26.60.6.zip")
	if e := m.apply(context.Background(), r, archive(t, map[string]string{"bedrock_server.exe": "MZnew"})); e != nil {
		t.Fatal(e)
	}
	backup := m.Status().Backup
	os.WriteFile(filepath.Join(backup, "servers", "creative", "worlds", "world", "level.dat"), []byte("tamper"), 0600)
	s.states[0].State = supervisor.Stopped
	m.status.Recovery = true
	if _, e := m.Recover(context.Background(), true); e == nil {
		t.Fatal("tampered backup accepted")
	}
}
func TestFailedNewEngineNeedsExplicitRecovery(t *testing.T) {
	m, s, dir := fixture(t)
	s.failStart = true
	r, _ := parseRelease("https://www.minecraft.net/bedrockdedicatedserver/bin-win/bedrock-server-1.26.60.6.zip")
	if e := m.apply(context.Background(), r, archive(t, map[string]string{"bedrock_server.exe": "MZnew"})); e == nil {
		t.Fatal("startup failure ignored")
	}
	if !m.Status().Recovery || !m.Status().Opened {
		t.Fatal("unsafe automatic downgrade")
	}
	if _, e := m.Recover(context.Background(), false); e == nil {
		t.Fatal("unconfirmed restore")
	}
	if _, e := m.Recover(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "bedrock_server.exe"))
	if string(b) != "MZold" {
		t.Fatal("old engine not restored")
	}
}
