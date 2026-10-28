package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/adminpipe"
	"nethernet-multi-server/internal/backend"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/contentpacks"
	identitystore "nethernet-multi-server/internal/identity"
	"nethernet-multi-server/internal/lan"
	"nethernet-multi-server/internal/migration"
	"nethernet-multi-server/internal/online"
	"nethernet-multi-server/internal/playerpermissions"
	proxyservice "nethernet-multi-server/internal/proxy"
	"nethernet-multi-server/internal/serversettings"
	"nethernet-multi-server/internal/serverupdate"
	adminservice "nethernet-multi-server/internal/service"
	"nethernet-multi-server/internal/sessionhistory"
	"nethernet-multi-server/internal/supervisor"
)

var listenPacket = listenIPv4Broadcast
var listenAdmin = adminpipe.ListenCurrentUser
var startListeners = proxyservice.StartListeners

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	if len(args) > 1 {
		fmt.Fprintln(stderr, "Usage: NetherNetMultiServer.exe [config.json]")
		return 2
	}
	configPath := "config.json"
	if len(args) == 1 {
		configPath = args[0]
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error: load configuration %q: %v\n", configPath, err)
		return 1
	}
	protector := accountstore.DPAPIProtector{Entropy: []byte("NetherNetMultiServer/account-store/v1")}
	accountStore := accountstore.New(config.AccountStorePath(configPath), protector)
	if err := accounts.MigrateLegacyToken(ctx, filepath.Dir(configPath), accountStore, accounts.LiveAuthenticator{}); err != nil {
		fmt.Fprintf(stderr, "Error: migrate existing Microsoft authentication: %v\n", err)
		return 1
	}
	accountManager, err := accounts.NewManager(accountStore)
	if err != nil {
		fmt.Fprintf(stderr, "Error: open encrypted account store: %v\n", err)
		return 1
	}
	packManager, err := contentpacks.NewManager(filepath.Join(filepath.Dir(configPath), "content-staging"), cfg.Servers)
	if err != nil {
		fmt.Fprintf(stderr, "Error: open content pack store: %v\n", err)
		return 1
	}
	settingsManager, err := serversettings.NewManager(filepath.Join(filepath.Dir(configPath), "server-settings"), cfg.Servers)
	if err != nil {
		fmt.Fprintf(stderr, "Error: open server settings store: %v\n", err)
		return 1
	}
	permissionsManager, err := playerpermissions.NewManager(filepath.Join(filepath.Dir(configPath), "player-permissions"), cfg.Servers)
	if err != nil {
		fmt.Fprintf(stderr, "Error: open player permissions store: %v\n", err)
		return 1
	}
	historyStore, err := sessionhistory.Open(filepath.Join(filepath.Dir(configPath), "session-logs", "sessions.jsonl"))
	if err != nil {
		fmt.Fprintf(stderr, "Error: open player session history: %v\n", err)
		return 1
	}
	serverManager := supervisor.New(cfg.Servers, supervisor.WindowsFactory{}, supervisor.Options{ApplyPending: func(_ context.Context, id string) error {
		if err := packManager.Apply(id); err != nil {
			return err
		}
		if settingsManager.Pending(id) {
			return settingsManager.ClearPending(id)
		}
		return nil
	}})
	serverManager.DetectExternal(cfg.Servers)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				serverManager.DetectExternal(cfg.Servers)
			}
		}
	}()
	controller := adminservice.NewManagedController(accountStore, accountManager, accounts.LiveAuthenticator{}, serverManager, packManager)
	controller.SetServerSettings(settingsManager)
	controller.SetPlayerPermissions(permissionsManager)
	controller.SetSessionHistory(historyStore)
	onlineManager, onlineError := online.New(filepath.Join(filepath.Dir(configPath), "online", "settings.json"), cfg.Servers)
	controller.SetOnline(onlineManager, onlineError)
	if onlineError != nil {
		fmt.Fprintln(stderr, "Online configuration unavailable; existing LAN operation remains independent.")
	}
	installationRoot, _ := filepath.Abs(filepath.Join(filepath.Dir(configPath), ".."))
	controller.SetAddonSources(addonupdates.New(filepath.Join(installationRoot, "MultiServer", "addon-update-sources.json")))
	controller.SetAddonUpdates(addonupdates.NewCredentials(filepath.Join(installationRoot, "MultiServer", "provider-credentials", "curseforge.json"), accountstore.DPAPIProtector{Entropy: []byte("NetherNet.CurseForge.v1")}), configPath, filepath.Join(installationRoot, "MultiServer", "addon-downloads"))
	controller.SetUpdates(serverupdate.New(installationRoot, configPath, serverManager))
	controller.SetMigration(migration.NewManager(installationRoot, serverManager, func() ([]byte, error) {
		s := accountManager.Snapshot()
		return migration.ExportAccounts(accountstore.Snapshot{Version: accountstore.CurrentVersion, Accounts: s.Accounts, Players: s.Players})
	}))
	controller.SetShutdown(func() {
		go func() {
			time.Sleep(250 * time.Millisecond)
			cancelRun()
		}()
	})
	adminListener, err := listenAdmin(adminpipe.DefaultName)
	if err != nil {
		fmt.Fprintf(stderr, "Error: start local administration channel: %v\n", err)
		return 1
	}
	defer adminListener.Close()
	go serveAdmin(ctx, adminListener, controller, slog.New(slog.NewTextHandler(stderr, nil)))
	if _, err := os.Stat(filepath.Join(installationRoot, "migration-activation-required")); err == nil {
		fmt.Fprintln(stdout, "Migration restored: administration is available; LAN advertising is disabled until setup activation.")
		<-ctx.Done()
		return 0
	}

	if _, e := os.Stat(filepath.Join(installationRoot, "installation-retired")); e == nil {
		fmt.Fprintln(stdout, "Installation retired. Advertising is disabled.")
		<-ctx.Done()
		return 0
	}
	if !waitForManagedNetwork(ctx, cfg, stdout) {
		return 0
	}
	advertisements := make([]lan.Advertisement, 0, len(cfg.Servers))
	for i, server := range cfg.Servers {
		networkID, err := server.ParsedNetworkID()
		if err != nil {
			fmt.Fprintf(stderr, "Error: prepare servers[%d] Network ID: %v\n", i, err)
			return 1
		}
		server.Version = protocol.CurrentVersion
		server.Protocol = protocol.CurrentProtocol
		data, err := server.DiscoveryData()
		if err != nil {
			fmt.Fprintf(stderr, "Error: prepare servers[%d] advertisement: %v\n", i, err)
			return 1
		}
		advertisements = append(advertisements, lan.Advertisement{
			Name: server.Name, Backend: server.Backend, NetworkID: networkID, ServerData: *data,
		})
	}

	conn, err := listenPacket("udp4", cfg.InterfaceIP, cfg.Listen)
	if err != nil {
		fmt.Fprintf(stderr, "Error: listen on UDP %s: %v\n", cfg.Listen, err)
		if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
			fmt.Fprintln(stderr, "UDP 7551 is already in use. Set enable-lan-visibility=false on both BDS instances and check whether another process owns the port.")
		}
		return 1
	}

	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	service := lan.New(conn, advertisements, logger)
	targets, err := startListeners(ctx, service, cfg.Servers, identitystore.Store{Directory: config.IdentityDirectory(configPath)}, logger)
	if err != nil {
		_ = conn.Close()
		fmt.Fprintf(stderr, "Error: start NetherNet listeners: %v\n", err)
		return 1
	}
	defer proxyservice.CloseTargets(targets)
	if len(cfg.Servers) > 0 && cfg.Servers[0].Process != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if stopAdvertisingOnDrift(cfg, func() { _ = conn.Close(); proxyservice.CloseTargets(targets) }) {
						logger.Error("Network bindings changed; advertising stopped. Administration remains available. Use Repair Network Configuration, then restart the helper.")
						return
					}
				}
			}
		}()
	}
	proxyservice.ServeTargets(ctx, targets, backend.NetherNetDialer{Log: logger}, accountManager, historyStore, logger)

	fmt.Fprintln(stdout, "NetherNet Multi-Server")
	fmt.Fprintln(stdout, "Stage 2 connection proxy")
	fmt.Fprintf(stdout, "Account pool: %d account(s)\n", len(accountManager.Snapshot().Accounts))
	fmt.Fprintln(stdout, "Admin: local Windows control panel ready")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Listening: UDP %s\n", conn.LocalAddr())
	fmt.Fprintf(stdout, "Interface: %s\n", cfg.InterfaceIP)
	fmt.Fprintln(stdout, "Socket: udp4 / IPv4 (SO_BROADCAST enabled, IP_RECEIVE_BROADCAST enabled)")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Servers:")
	for i, advertisement := range advertisements {
		fmt.Fprintf(stdout, "\n[%d] %s\n    %s\n    Network ID: %d\n", i+1, advertisement.Name, advertisement.Backend, advertisement.NetworkID)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Waiting for LAN clients...")
	if err := service.Serve(ctx); err != nil {
		fmt.Fprintf(stderr, "Error: discovery service stopped: %v\n", err)
		return 1
	}
	return 0
}

func serveAdmin(ctx context.Context, listener net.Listener, controller adminapi.Controller, logger *slog.Logger) {
	go func() { <-ctx.Done(); _ = listener.Close() }()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("admin pipe accept failed", "error", err)
			}
			return
		}
		go func() {
			defer conn.Close()
			if err := adminapi.ServeConnection(ctx, conn, controller); err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
				logger.Warn("admin client disconnected", "error", err)
			}
		}()
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
