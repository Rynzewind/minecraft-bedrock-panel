package proxy

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/df-mc/go-nethernet"
	"github.com/pion/webrtc/v4"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/resource"

	"nethernet-multi-server/internal/config"
	identitystore "nethernet-multi-server/internal/identity"
	"nethernet-multi-server/internal/lan"
)

type Target struct {
	Name        string
	BackendURL  string
	NetworkID   uint64
	Fingerprint string
	Listener    *minecraft.Listener
}

func targetMetadata(server config.Server, networkID uint64) Target {
	return Target{Name: server.Name, BackendURL: server.BackendURL(), NetworkID: networkID}
}

func ObserveAccepted(ctx context.Context, targets []Target, log *slog.Logger) {
	for _, target := range targets {
		target := target
		go func() {
			for {
				conn, err := target.Listener.Accept()
				if err != nil {
					if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
						log.Error("Minecraft accept failed", "server", target.Name, "error", err)
					}
					return
				}
				log.Info("Minecraft connection accepted", "server", target.Name, "network_id", target.NetworkID, "remote", conn.RemoteAddr())
				_ = conn.Close()
			}
		}()
	}
}

func StartListeners(ctx context.Context, service *lan.Service, servers []config.Server, identities identitystore.Store, log *slog.Logger) ([]Target, error) {
	if log == nil {
		log = slog.Default()
	}
	targets := make([]Target, 0, len(servers))
	for _, server := range servers {
		networkID, err := server.ParsedNetworkID()
		if err != nil {
			CloseTargets(targets)
			return nil, fmt.Errorf("server %q network ID: %w", server.Name, err)
		}
		key, fingerprint, err := identities.LoadOrCreate(networkID)
		if err != nil {
			CloseTargets(targets)
			return nil, fmt.Errorf("server %q identity: %w", server.Name, err)
		}
		targetLog := log.With("server", server.Name, "network_id", networkID)
		network := minecraft.NetherNet{Signaling: nativeLANSignaling{service.Signaling(networkID)}, ListenConfig: listenerConfig(ctx, key, targetLog), Log: targetLog}
		listener, err := (minecraft.ListenConfig{
			ErrorLog:             targetLog,
			TexturePacksRequired: texturePacksRequired(server),
			FetchResourcePacks: func(_ login.IdentityData, _ login.ClientData, current []*resource.Pack) []*resource.Pack {
				packs, loadErr := activeResourcePacks(server)
				if loadErr != nil {
					targetLog.Error("load active resource packs", "error", loadErr)
					return current
				}
				targetLog.Info("resource packs prepared for console", "count", len(packs))
				return packs
			},
		}).ListenNetwork(network, fmt.Sprint(networkID))
		if err != nil {
			CloseTargets(targets)
			return nil, fmt.Errorf("server %q listener: %w", server.Name, err)
		}
		target := targetMetadata(server, networkID)
		target.Fingerprint = fingerprint
		target.Listener = listener
		targets = append(targets, target)
		log.Info("NetherNet listener ready", "server", target.Name, "network_id", target.NetworkID, "backend", target.BackendURL, "identity_fingerprint", target.Fingerprint)
	}
	go func() { <-ctx.Done(); CloseTargets(targets) }()
	return targets, nil
}

type resourcePackActivation struct {
	PackID  string `json:"pack_id"`
	Version [3]int `json:"version"`
}

func activeResourcePacks(server config.Server) ([]*resource.Pack, error) {
	if server.Process == nil {
		return nil, nil
	}
	b, err := os.ReadFile(filepath.Join(server.Process.WorldDirectory, "world_resource_packs.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var activations []resourcePackActivation
	if err = json.Unmarshal(b, &activations); err != nil {
		return nil, fmt.Errorf("decode world resource packs: %w", err)
	}
	packs := make([]*resource.Pack, 0, len(activations))
	for _, activation := range activations {
		path := filepath.Join(server.Process.WorkingDirectory, "resource_packs", activation.PackID)
		pack, readErr := resource.ReadPath(path)
		if readErr != nil {
			return nil, fmt.Errorf("read resource pack %s: %w", activation.PackID, readErr)
		}
		if !strings.EqualFold(pack.UUID().String(), activation.PackID) {
			return nil, fmt.Errorf("resource pack directory %s contains manifest UUID %s", activation.PackID, pack.UUID())
		}
		wantVersion := fmt.Sprintf("%d.%d.%d", activation.Version[0], activation.Version[1], activation.Version[2])
		if pack.Version() != wantVersion {
			return nil, fmt.Errorf("resource pack %s activation version %s does not match manifest version %s", activation.PackID, wantVersion, pack.Version())
		}
		packs = append(packs, pack)
	}
	return packs, nil
}

func texturePacksRequired(server config.Server) bool {
	if server.Process == nil {
		return false
	}
	b, err := os.ReadFile(filepath.Join(server.Process.WorkingDirectory, "server.properties"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "texturepack-required") {
			return strings.EqualFold(strings.TrimSpace(parts[1]), "true")
		}
	}
	return false
}

func listenerConfig(_ context.Context, key *ecdsa.PrivateKey, log *slog.Logger) nethernet.ListenConfig {
	var settings webrtc.SettingEngine
	settings.SetNetworkTypes(netherNetNetworkTypes())
	return nethernet.ListenConfig{
		Log:            log,
		API:            webrtc.NewAPI(webrtc.WithSettingEngine(settings)),
		AllowAnonymous: true,
		NegotiationContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, 15*time.Second)
		},
		ConnContext: func(parent context.Context, _ *nethernet.Conn) (context.Context, context.CancelFunc) {
			return context.WithTimeout(parent, 15*time.Second)
		},
		IssueServerIdentity: func(context.Context) (*nethernet.Identity, error) {
			return nethernet.GenerateServerIdentity(key, "self")
		},
	}
}

func netherNetNetworkTypes() []webrtc.NetworkType {
	return []webrtc.NetworkType{webrtc.NetworkTypeUDP4}
}

func CloseTargets(targets []Target) {
	for _, target := range targets {
		if target.Listener != nil {
			_ = target.Listener.Close()
		}
	}
}
