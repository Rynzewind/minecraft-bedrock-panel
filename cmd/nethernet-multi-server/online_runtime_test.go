package main

import (
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/online"
	"nethernet-multi-server/internal/onlineguard"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOnlineBackendAuthenticationFailsClosed(t *testing.T) {
	dir := t.TempDir()
	server := config.Server{Process: &config.Process{WorkingDirectory: dir}}
	for _, text := range []string{"", "online-mode=false\n", "# online-mode=true\n", "online-mode=true\nonline-mode=false\n"} {
		if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if onlineBackendAuthenticated(server) {
			t.Fatalf("unsafe online-mode accepted: %q", text)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "server.properties"), []byte("online-mode=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !onlineBackendAuthenticated(server) {
		t.Fatal("authenticated BDS rejected")
	}
}
func TestOnlineCurrentAddressCanonicalization(t *testing.T) {
	got, err := canonicalOnlineAddresses([]string{"192.168.1.2/24", "fe80::abcd%12/64", "::ffff:127.0.0.1/128", "192.168.1.2/24"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%v", got)
	}
	for _, s := range got {
		if s == "fe80::abcd" {
			return
		}
	}
	t.Fatal("IPv6 zone not removed")
}
func TestOnlineAdmissionUsesLatestSettingsACLAndReadiness(t *testing.T) {
	servers := []config.Server{{ID: "a", Name: "A", Backend: "127.0.0.1:7559"}}
	manager, err := online.New(filepath.Join(t.TempDir(), "online.json"), servers)
	if err != nil {
		t.Fatal(err)
	}
	live := supervisor.Status{ID: "a", State: supervisor.Running, Ready: true}
	if onlineRuntimeAuthorized(manager, "a", "123", live) {
		t.Fatal("disabled Online admitted player")
	}
	// Manager validates explicit settings on save; authorization helper still
	// requires all independent gates even when the player has a stored grant.
	if err = manager.GrantResolved(online.Player{XUID: "123", Gamertag: "Player", Permissions: map[string]bool{"a": true}}); err != nil {
		t.Fatal(err)
	}
	if onlineRuntimeAuthorized(manager, "a", "123", live) {
		t.Fatal("grant bypassed disabled settings")
	}
	settings := manager.Settings()
	settings.AllowUnknownFriendsRoute = true
	settings.Servers["a"] = online.ServerSettings{Enabled: true, AccountID: "broadcaster", WorldName: "A", ExternalPort: 19140}
	if err = manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	if !onlineRuntimeAuthorized(manager, "a", "123", live) {
		t.Fatal("explicit authorized player rejected")
	}
	live.Ready = false
	if onlineRuntimeAuthorized(manager, "a", "123", live) {
		t.Fatal("unready BDS admitted player")
	}
	live.Ready = true
	settings.AllowUnknownFriendsRoute = false
	if err = manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	if onlineRuntimeAuthorized(manager, "a", "123", live) {
		t.Fatal("withdrawn Friends-route consent ignored")
	}
	settings.AllowUnknownFriendsRoute = true
	if err = manager.Save(settings); err != nil {
		t.Fatal(err)
	}
	if err = manager.Revoke("123", "a"); err != nil {
		t.Fatal(err)
	}
	if onlineRuntimeAuthorized(manager, "a", "123", live) {
		t.Fatal("revoked player admitted")
	}
}

func TestOnlineProtectionGateUsesCurrentAddressInventory(t *testing.T) {
	cfg := config.Config{Servers: []config.Server{{ID: "a", Backend: "127.0.0.1:7559", Process: &config.Process{Executable: `C:\NetherNet\Creative\bedrock_server.exe`}}}}
	plan, err := onlineProtectionPlan(cfg, []string{"192.168.1.2"})
	if err != nil {
		t.Fatal(err)
	}
	evidence := onlineguard.Evidence{Platform: "windows", Complete: true, ObservedAt: time.Now(), FirewallServiceRunning: true, BaseFilteringEngineRunning: true, LocalAddresses: plan.LocalAddresses}
	for _, name := range []string{"Domain", "Private", "Public"} {
		evidence.Profiles = append(evidence.Profiles, onlineguard.Profile{Name: name, Enabled: true, AllowInboundRules: true})
	}
	for _, rule := range plan.Rules {
		evidence.Rules = append(evidence.Rules, onlineguard.ObservedRule{Spec: rule, PrimaryStatus: "OK", EnforcementStatus: "Full"})
	}
	if !onlineguard.Evaluate(plan, evidence).SnapshotMatches {
		t.Fatal("matching effective protection rejected")
	}
	if !onlineAdmissionProtection(cfg, evidence, false, []string{"192.168.1.2"}) {
		t.Fatal("current admission protection rejected")
	}
	if onlineAdmissionProtection(cfg, evidence, true, []string{"192.168.1.2"}) {
		t.Fatal("failed latest inspection reused previous matching evidence")
	}
	if onlineAdmissionProtection(cfg, evidence, false, []string{"192.168.1.3"}) {
		t.Fatal("admission reused cached protected flag after address drift")
	}
	stale := evidence
	stale.ObservedAt = time.Now().Add(-onlineguard.MaxEvidenceAge - time.Second)
	if onlineAdmissionProtection(cfg, stale, false, []string{"192.168.1.2"}) {
		t.Fatal("admission trusted expired evidence while reconcile stalled")
	}
	drift, err := onlineProtectionPlan(cfg, []string{"192.168.1.3"})
	if err != nil {
		t.Fatal(err)
	}
	if onlineguard.Evaluate(drift, evidence).SnapshotMatches {
		t.Fatal("host drift reused old firewall evidence")
	}
	if onlineguard.Evaluate(plan, onlineguard.Evidence{}).SnapshotMatches {
		t.Fatal("missing/read-denied evidence enabled Online")
	}
}
