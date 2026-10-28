package service

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/serversettings"
	"nethernet-multi-server/internal/sessionhistory"
	"nethernet-multi-server/internal/supervisor"
	"path/filepath"
	"testing"

	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/accountstore"
)

type fakeSessionHistory struct{ events []sessionhistory.Event }

func (f fakeSessionHistory) List(limit int) []sessionhistory.Event {
	if limit > len(f.events) {
		limit = len(f.events)
	}
	return append([]sessionhistory.Event(nil), f.events[:limit]...)
}

func TestSessionHistoryList(t *testing.T) {
	c := &Controller{}
	c.SetSessionHistory(fakeSessionHistory{events: []sessionhistory.Event{{ConsoleGamertag: "Kev", Server: "Creative"}}})
	result, err := c.Handle(context.Background(), "sessions.list", json.RawMessage(`{"limit":100}`))
	if err != nil {
		t.Fatal(err)
	}
	events := result.([]sessionhistory.Event)
	if len(events) != 1 || events[0].ConsoleGamertag != "Kev" {
		t.Fatalf("events=%+v", events)
	}
}

type fakeSupervisor struct{ started string }

func (f *fakeSupervisor) Statuses() []supervisor.Status {
	return []supervisor.Status{{ID: "creative", Name: "Creative", State: supervisor.Stopped}}
}
func (f *fakeSupervisor) Start(_ context.Context, id string) error { f.started = id; return nil }
func (f *fakeSupervisor) Stop(context.Context, string) error       { return nil }
func (f *fakeSupervisor) Restart(context.Context, string) error    { return nil }
func (f *fakeSupervisor) RecentLog(string) string                  { return "ready" }

type fakePacks struct {
	applied string
	staged  []string
}

func (fakePacks) InventoryState() []contentpacks.InventoryItem            { return nil }
func (fakePacks) ImportArchive(string) ([]contentpacks.StoredPack, error) { return nil, nil }
func (fakePacks) Stage(string, string, bool) error                        { return nil }
func (f *fakePacks) StageMany(_ string, ids []string, _ bool) error {
	f.staged = append([]string(nil), ids...)
	return nil
}
func (fakePacks) Pending(string) []contentpacks.Operation   { return nil }
func (f *fakePacks) Apply(id string) error                  { f.applied = id; return nil }
func (f *fakePacks) StageResourcePolicy(string, bool) error { return nil }
func (f *fakePacks) Uninstall(string) error                 { return nil }
func (f *fakePacks) UninstallPlan(ids []string) ([]contentpacks.StoredPack, error) {
	f.staged = append([]string(nil), ids...)
	return []contentpacks.StoredPack{{Pack: contentpacks.Pack{Name: "Linked", UUID: "linked"}}}, nil
}
func (f *fakePacks) UninstallMany(ids []string) error {
	f.staged = append([]string(nil), ids...)
	return nil
}
func (f *fakePacks) CancelPending(string) error { return nil }

func TestManagementMethodsUseStableServerID(t *testing.T) {
	c := &Controller{supervisor: &fakeSupervisor{}, packs: &fakePacks{}}
	if _, err := c.Handle(context.Background(), "server.start", json.RawMessage(`{"server_id":"creative"}`)); err != nil {
		t.Fatal(err)
	}
	if c.supervisor.(*fakeSupervisor).started != "creative" {
		t.Fatal("stable server ID not forwarded")
	}
	result, err := c.Handle(context.Background(), "servers.list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.([]supervisor.Status)) != 1 {
		t.Fatalf("result=%+v", result)
	}
}

func TestPackStageManyForwardsEveryUUID(t *testing.T) {
	p := &fakePacks{}
	c := &Controller{packs: p}
	if _, err := c.Handle(context.Background(), "pack.stage_many", json.RawMessage(`{"server_id":"creative","pack_uuids":["a","b"],"enable":true}`)); err != nil {
		t.Fatal(err)
	}
	if len(p.staged) != 2 || p.staged[0] != "a" || p.staged[1] != "b" {
		t.Fatalf("staged=%v", p.staged)
	}
}

func TestPackUninstallPreviewAndCommitForwardEveryUUID(t *testing.T) {
	p := &fakePacks{}
	c := &Controller{packs: p}
	result, err := c.Handle(context.Background(), "pack.uninstall.preview", json.RawMessage(`{"pack_uuids":["a","b"]}`))
	if err != nil || len(result.([]contentpacks.StoredPack)) != 1 || len(p.staged) != 2 {
		t.Fatalf("preview result=%+v staged=%v err=%v", result, p.staged, err)
	}
	if _, err = c.Handle(context.Background(), "pack.uninstall_many", json.RawMessage(`{"pack_uuids":["a","b"]}`)); err != nil {
		t.Fatal(err)
	}
	if len(p.staged) != 2 || p.staged[0] != "a" || p.staged[1] != "b" {
		t.Fatalf("committed=%v", p.staged)
	}
}

type runningSupervisor struct{ *fakeSupervisor }

func (runningSupervisor) Statuses() []supervisor.Status {
	return []supervisor.Status{{ID: "creative", State: supervisor.Running}}
}
func TestPackApplyRejectedWhileServerRunning(t *testing.T) {
	p := &fakePacks{}
	c := &Controller{supervisor: runningSupervisor{&fakeSupervisor{}}, packs: p}
	_, err := c.Handle(context.Background(), "packs.apply", json.RawMessage(`{"server_id":"creative"}`))
	if err == nil {
		t.Fatal("apply while running succeeded")
	}
	if p.applied != "" {
		t.Fatal("pack manager was called")
	}
}

type shutdownSupervisor struct {
	fakeSupervisor
	stopped []string
}

func (s *shutdownSupervisor) Statuses() []supervisor.Status {
	return []supervisor.Status{{ID: "creative", State: supervisor.Running}, {ID: "survival", State: supervisor.Running}}
}
func (s *shutdownSupervisor) Stop(_ context.Context, id string) error {
	s.stopped = append(s.stopped, id)
	return nil
}

func TestSystemShutdownGracefullyStopsManagedServersBeforeCallback(t *testing.T) {
	s := &shutdownSupervisor{}
	called := false
	c := &Controller{supervisor: s, shutdown: func() { called = true }}
	if _, err := c.Handle(context.Background(), "system.shutdown", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if len(s.stopped) != 2 || !called {
		t.Fatalf("stopped=%v callback=%v", s.stopped, called)
	}
}

type fakeSettings struct {
	saved string
	value serversettings.Settings
}

func (f *fakeSettings) Get(string) (serversettings.Settings, error) { return f.value, nil }
func (f *fakeSettings) Save(id string, value serversettings.Settings) error {
	f.saved = id
	f.value = value
	return nil
}
func (f *fakeSettings) Pending(string) bool       { return true }
func (f *fakeSettings) ClearPending(string) error { return nil }

func TestServerSettingsAPIUsesStableServerID(t *testing.T) {
	f := &fakeSettings{value: serversettings.Settings{GameMode: "creative", Difficulty: "normal", MaxPlayers: 10, ViewDistance: 24, TickDistance: 4, DefaultPermission: "member"}}
	c := &Controller{}
	c.SetServerSettings(f)
	if _, err := c.Handle(context.Background(), "server.settings.save", json.RawMessage(`{"server_id":"creative","settings":{"game_mode":"creative","difficulty":"normal","max_players":12,"view_distance":24,"tick_distance":4,"player_idle_timeout":0,"default_permission":"member"}}`)); err != nil {
		t.Fatal(err)
	}
	if f.saved != "creative" || f.value.MaxPlayers != 12 {
		t.Fatalf("saved=%q value=%+v", f.saved, f.value)
	}
}

type plainProtector struct{}

func (plainProtector) Protect(b []byte) ([]byte, error)   { return append([]byte(nil), b...), nil }
func (plainProtector) Unprotect(b []byte) ([]byte, error) { return append([]byte(nil), b...), nil }
func TestStatusNeverReturnsEncryptedToken(t *testing.T) {
	store := accountstore.New(filepath.Join(t.TempDir(), "accounts.json"), plainProtector{})
	if err := store.Save(accountstore.Snapshot{Version: 1, Accounts: []accountstore.Account{{ID: "a", PersonLabel: "Kevin", Gamertag: "Helper", XUID: "1", EncryptedToken: "TOP-SECRET", Healthy: true}}}); err != nil {
		t.Fatal(err)
	}
	manager, err := accounts.NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	controller := NewController(store, manager, nil)
	result, err := controller.Handle(context.Background(), "status.list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result)
	if string(b) == "" || contains(string(b), "TOP-SECRET") {
		t.Fatalf("token leaked: %s", b)
	}
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
