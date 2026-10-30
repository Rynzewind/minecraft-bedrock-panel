package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/serverupdate"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type autoTestSupervisor struct {
	onStart       func(string) error
	states        []supervisor.Status
	stops, starts int
	refuse        string
}

func (s *autoTestSupervisor) Statuses() []supervisor.Status {
	return append([]supervisor.Status(nil), s.states...)
}
func (s *autoTestSupervisor) Start(_ context.Context, id string) error {
	s.starts++
	if s.onStart != nil {
		if err := s.onStart(id); err != nil {
			return err
		}
	}
	for i := range s.states {
		if s.states[i].ID == id {
			s.states[i].State = supervisor.Running
			s.states[i].Ready = true
		}
	}
	return nil
}
func (s *autoTestSupervisor) Stop(ctx context.Context, id string) error {
	return s.StopIfEmpty(ctx, id)
}
func (s *autoTestSupervisor) StopIfEmpty(_ context.Context, id string) error {
	if s.refuse == id {
		return errors.New("player joined")
	}
	s.stops++
	for i := range s.states {
		if s.states[i].ID == id {
			s.states[i].State = supervisor.Stopped
		}
	}
	return nil
}
func (s *autoTestSupervisor) Restart(context.Context, string) error { return nil }
func (s *autoTestSupervisor) RecentLog(string) string               { return "" }

type autoTestUpdates struct {
	fakeUpdates
	check            serverupdate.Check
	checked, started int
}

func (u *autoTestUpdates) Check(context.Context) (serverupdate.Check, error) {
	u.checked++
	return u.check, nil
}
func (u *autoTestUpdates) Start(context.Context, string) (serverupdate.Status, error) {
	u.started++
	return serverupdate.Status{}, nil
}
func autoController(t *testing.T) (*Controller, *autoTestSupervisor) {
	t.Helper()
	s := &autoTestSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running, Ready: true}, {ID: "survival", State: supervisor.Running, Ready: true}}}
	c := &Controller{supervisor: s}
	if err := c.SetAutomaticUpdates(filepath.Join(t.TempDir(), "auto.json")); err != nil {
		t.Fatal(err)
	}
	yes := true
	c.automatic.Set(&yes, &yes)
	return c, s
}
func TestAutomaticServerUpdateHonorsOffPlayersAndProtocol(t *testing.T) {
	for _, scenario := range []string{"off", "players", "protocol", "compatible"} {
		t.Run(scenario, func(t *testing.T) {
			c, s := autoController(t)
			u := &autoTestUpdates{check: serverupdate.Check{Needed: true, Release: serverupdate.Release{Compatible: scenario == "compatible", Version: "1.26.60"}}}
			c.updates = u
			switch scenario {
			case "off":
				no := false
				c.automatic.Set(&no, &no)
			case "players":
				s.states[0].Players = 1
			}
			c.automaticCycle(context.Background())
			if scenario == "compatible" {
				if u.started != 1 {
					t.Fatal("compatible update did not start")
				}
			} else if u.started != 0 {
				t.Fatal("unsafe/disabled server update started")
			}
			if scenario == "players" && u.checked != 0 {
				t.Fatal("cycle ignored active player")
			}
		})
	}
}
func TestAutoSwitchesCanBeDisabledDuringMaintenance(t *testing.T) {
	c, _ := autoController(t)
	c.updates = fakeUpdates{busy: true, recovery: true}
	if _, err := c.Handle(context.Background(), "updates.auto.save", json.RawMessage(`{"Servers":false,"Addons":false}`)); err != nil {
		t.Fatal(err)
	}
	if c.automatic.Get().Servers || c.automatic.Get().Addons {
		t.Fatal("switch off failed")
	}
}
func TestAutomaticAddonAbortRestartsOnlyAlreadyStoppedWorlds(t *testing.T) {
	c, s := autoController(t)
	s.refuse = "survival"
	c.addonSources = addonupdates.New(filepath.Join(t.TempDir(), "sources.json"))
	err := c.applyAutomaticAddon(context.Background(), contentpacks.UpdatePreview{Servers: []string{"creative", "survival"}})
	if err == nil || s.stops != 1 || s.starts != 1 || s.states[0].State != supervisor.Running {
		t.Fatalf("abort recovery failed: %v %+v", err, s)
	}
}
func TestAutomaticAddonCannotStageProtectedCompanion(t *testing.T) {
	c, s := autoController(t)
	c.addonSources = addonupdates.New(filepath.Join(t.TempDir(), "sources.json"))
	c.addonSources.SaveRecord("custom", addonupdates.Source{LocallyModified: true, Provider: "manual"})
	err := c.applyAutomaticAddon(context.Background(), contentpacks.UpdatePreview{Servers: []string{"creative"}, Changes: []contentpacks.UpdateChange{{UUID: "custom"}}})
	if err == nil || s.starts != 1 {
		t.Fatalf("custom companion was allowed: %v", err)
	}
}

func TestAutomaticAddonUpdatesEnabledPackWithBackupAndRestart(t *testing.T) {
	const id = "11111111-1111-1111-1111-111111111111"
	root := t.TempDir()
	world := filepath.Join(root, "world")
	os.MkdirAll(world, 0700)
	manager, err := contentpacks.NewManager(filepath.Join(root, "library"), []config.Server{{ID: "creative", Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}}})
	if err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(root, "old.zip")
	file, err := os.Create(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(file)
	entry, _ := z.Create("manifest.json")
	fmt.Fprintf(entry, `{"format_version":2,"header":{"name":"Example","uuid":"%s","version":[1,0,0],"min_engine_version":[1,21,0]},"modules":[{"type":"data","uuid":"22222222-2222-2222-2222-222222222222","version":[1,0,0]}]}`, id)
	z.Close()
	file.Close()
	if _, err = manager.ImportArchive(oldPath); err != nil {
		t.Fatal(err)
	}
	if err = manager.Stage("creative", id, true); err != nil {
		t.Fatal(err)
	}
	if err = manager.Apply("creative"); err != nil {
		t.Fatal(err)
	}
	c, s := autoController(t)
	no := false
	c.automatic.Set(&no, nil)
	c.packs = manager
	sources := addonupdates.New(filepath.Join(root, "sources.json"))
	sources.SaveRecord(id, addonupdates.Source{Provider: "curseforge", ProjectID: 42, Channel: "stable"})
	c.SetAddonSources(sources)
	credentials := addonupdates.NewCredentials(filepath.Join(root, "credentials.json"), plainProtector{})
	credentials.Set("fixture-key")
	c.SetAddonUpdates(credentials, filepath.Join("..", "..", "config.example.json"), root)
	c.addonProviderFactory = func(string) addonProvider { return &overviewProvider{root: root, match: true} }
	s.onStart = func(server string) error {
		if server == "creative" {
			return manager.Apply(server)
		}
		return nil
	}
	message, err := c.automaticCycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	packs := manager.InventoryState()
	if packs[0].Version != [3]int{2, 0, 0} || packs[0].Servers["creative"].Version != [3]int{2, 0, 0} {
		t.Fatalf("not deployed: %+v; %s", packs, message)
	}
	if s.stops != 1 || s.starts != 1 {
		t.Fatal("wrong running state restoration")
	}
	backups, _ := filepath.Glob(filepath.Join(root, "library", "backups", "creative", "*", "manifest.json"))
	if len(backups) < 2 {
		t.Fatal("update backup not created")
	}
}

type interruptedAutoSupervisor struct{ autoTestSupervisor }

func (s *interruptedAutoSupervisor) StopIfEmpty(_ context.Context, id string) error {
	s.stops++
	for i := range s.states {
		if s.states[i].ID == id {
			s.states[i].State = supervisor.Stopped
		}
	}
	return context.Canceled
}
func TestAutomaticInterruptedStopStillRecoversTheWorld(t *testing.T) {
	c, original := autoController(t)
	s := &interruptedAutoSupervisor{autoTestSupervisor: *original}
	c.supervisor = s
	c.addonSources = addonupdates.New(filepath.Join(t.TempDir(), "sources.json"))
	if err := c.applyAutomaticAddon(context.Background(), contentpacks.UpdatePreview{Servers: []string{"creative"}}); err == nil {
		t.Fatal("expected interrupted stop")
	}
	if s.starts != 1 || s.states[0].State != supervisor.Running {
		t.Fatal("interrupted stop was not recovered")
	}
}
func TestUnsupportedServerReleaseDoesNotBlockIndependentAddonChecks(t *testing.T) {
	c, _ := autoController(t)
	c.updates = &autoTestUpdates{check: serverupdate.Check{Needed: true, Release: serverupdate.Release{Compatible: false}}}
	c.addonSources = addonupdates.New(filepath.Join(t.TempDir(), "sources.json"))
	c.packs = &fakePacks{}
	message, err := c.automaticCycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "protocol") || !strings.Contains(message, "add-on project") {
		t.Fatalf("checks were not independent: %s", message)
	}
}
