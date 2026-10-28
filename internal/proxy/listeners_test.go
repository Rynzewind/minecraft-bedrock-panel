package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/pion/webrtc/v4"

	"nethernet-multi-server/internal/config"
	identitystore "nethernet-multi-server/internal/identity"
)

func TestActiveResourcePacksLoadsWorldActivationInOrder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	world := filepath.Join(root, "worlds", "Test")
	packID := "4b8f1c3e-9d52-4a70-b7e0-6c1f8a3d5b92"
	packDir := filepath.Join(root, "resource_packs", packID)
	if err := os.MkdirAll(packDir, 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"format_version": 2,
		"header":         map[string]any{"name": "Textures", "description": "test", "uuid": packID, "version": []int{1, 0, 0}, "min_engine_version": []int{1, 21, 0}},
		"modules":        []any{map[string]any{"type": "resources", "uuid": "1a7c9e5f-3b24-4d80-90f2-7e5c1b8a4d60", "version": []int{1, 0, 0}}},
	}
	b, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(packDir, "manifest.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(world, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(world, "world_resource_packs.json"), []byte(`[{"pack_id":"`+packID+`","version":[1,0,0]}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	packs, err := activeResourcePacks(config.Server{Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}})
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].UUID().String() != packID {
		t.Fatalf("packs=%v", packs)
	}
}

func TestActiveResourcePacksRejectsManifestUUIDMismatch(t *testing.T) {
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	world := filepath.Join(root, "worlds", "Test")
	if err := os.MkdirAll(filepath.Join(root, "resource_packs", "expected"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(world, 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := `{"format_version":2,"header":{"name":"Wrong","description":"","uuid":"4b8f1c3e-9d52-4a70-b7e0-6c1f8a3d5b92","version":[1,0,0],"min_engine_version":[1,21,0]},"modules":[{"type":"resources","uuid":"1a7c9e5f-3b24-4d80-90f2-7e5c1b8a4d60","version":[1,0,0]}]}`
	_ = os.WriteFile(filepath.Join(root, "resource_packs", "expected", "manifest.json"), []byte(manifest), 0o600)
	_ = os.WriteFile(filepath.Join(world, "world_resource_packs.json"), []byte(`[{"pack_id":"expected","version":[1,0,0]}]`), 0o600)
	if _, err := activeResourcePacks(config.Server{Process: &config.Process{WorkingDirectory: root, WorldDirectory: world}}); err == nil {
		t.Fatal("UUID mismatch accepted")
	}
}

func TestTargetMetadataPreservesServerRouting(t *testing.T) {
	servers := []config.Server{
		{Name: "Creative", Backend: "192.168.1.246:19134", NetworkID: "13468123134000001934"},
		{Name: "Survival", Backend: "192.168.1.247:19136", NetworkID: "13468123136000001936"},
	}
	targets := make([]Target, 0, len(servers))
	for _, server := range servers {
		id, err := server.ParsedNetworkID()
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, targetMetadata(server, id))
	}
	if len(targets) != 2 {
		t.Fatalf("targets=%d", len(targets))
	}
	if targets[0].BackendURL != "http://192.168.1.246:19134" || targets[0].NetworkID != 13468123134000001934 {
		t.Fatalf("creative=%+v", targets[0])
	}
	if targets[1].BackendURL != "http://192.168.1.247:19136" || targets[1].NetworkID != 13468123136000001936 {
		t.Fatalf("survival=%+v", targets[1])
	}
}

func TestLANListenerAllowsAnonymousSDPOffer(t *testing.T) {
	key, _, err := (identitystore.Store{Directory: t.TempDir()}).LoadOrCreate(42)
	if err != nil {
		t.Fatal(err)
	}
	cfg := listenerConfig(context.Background(), key, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !cfg.AllowAnonymous {
		t.Fatal("PS5 LAN offer without SDP identity would be rejected")
	}
}

func TestNetherNetListenersUseIPv4UDPCandidatesOnly(t *testing.T) {
	got := netherNetNetworkTypes()
	if len(got) != 1 || got[0] != webrtc.NetworkTypeUDP4 {
		t.Fatalf("network types = %v; want [UDP4]", got)
	}
}
