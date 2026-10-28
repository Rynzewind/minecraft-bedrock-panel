package setup

import (
	"context"
	"errors"
	"nethernet-multi-server/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPlanScopesFirewallAndStartupToRestoredUser(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{Servers: []config.Server{{Process: &config.Process{Executable: filepath.Join(root, "Creative", "bedrock_server.exe")}}}}
	p, e := BuildPlan(Options{Root: root, UserSID: "S-1-5-21-123-456-789-1001", Startup: "logon", OldDeploymentStopped: true}, cfg)
	if e != nil {
		t.Fatal(e)
	}
	if p.UserSID != "S-1-5-21-123-456-789-1001" || len(p.FirewallPrograms) != 2 || p.FirewallPrograms[1] != filepath.Join(root, "Creative", "bedrock_server.exe") || p.Startup != "logon" {
		t.Fatalf("wrong scoped plan: %+v", p)
	}
}
func TestRefusesActivationWithoutOldDeploymentConfirmation(t *testing.T) {
	if _, e := BuildPlan(Options{Root: t.TempDir(), UserSID: "S-1-5-21-1", Startup: "none"}, config.Config{}); e == nil {
		t.Fatal("allowed conflicting advertising")
	}
}

type testPlatform struct{ fail bool }

func (p *testPlatform) DisableStartup(context.Context, Plan) error { return nil }
func (p *testPlatform) EnsureRuntime(context.Context, Plan) error {
	if p.fail {
		return errors.New("missing runtime")
	}
	return nil
}
func (p *testPlatform) ConfigureFirewall(context.Context, Plan) error { return nil }
func (p *testPlatform) RegisterStartup(context.Context, Plan) error   { return nil }
func (p *testPlatform) EnableStartup(context.Context, Plan) error     { return nil }
func TestFailedSetupRetainsGateAndRetryActivates(t *testing.T) {
	root := t.TempDir()
	gate := filepath.Join(root, "migration-activation-required")
	os.WriteFile(gate, []byte("pending"), 0600)
	p := Plan{Root: root, Startup: "logon"}
	platform := &testPlatform{fail: true}
	if e := Apply(context.Background(), p, platform); e == nil {
		t.Fatal("ignored failure")
	}
	if _, e := os.Stat(gate); e != nil {
		t.Fatal("activated failed installation")
	}
	platform.fail = false
	if e := Apply(context.Background(), p, platform); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(gate); !os.IsNotExist(e) {
		t.Fatal("activation gate remains")
	}
	if e := Apply(context.Background(), p, platform); e != nil {
		t.Fatal("retry not idempotent", e)
	}
}
func TestBroadcastUsesActualSubnetMask(t *testing.T) {
	b, e := Broadcast("10.42.8.9/20")
	if e != nil || b != "10.42.15.255" {
		t.Fatal(b, e)
	}
	if _, e = Broadcast("10.42.8.9/32"); e == nil {
		t.Fatal("accepted host-only subnet")
	}
}
