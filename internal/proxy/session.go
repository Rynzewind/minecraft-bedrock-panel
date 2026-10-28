package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/backend"
	"nethernet-multi-server/internal/sessionhistory"
)

type AccountLeaser interface {
	Acquire(context.Context, accounts.Player, string) (*accounts.Lease, error)
}

type SessionRecorder interface {
	Connected(sessionhistory.Event) string
	Disconnected(string, string)
}

func backendDialContext(parent context.Context) (context.Context, context.CancelFunc) {
	// Heavily customised worlds can finish login after the former 30-second limit.
	return context.WithTimeout(parent, 90*time.Second)
}

func ServeTargets(ctx context.Context, targets []Target, dialer backend.Dialer, leaser AccountLeaser, recorder SessionRecorder, log *slog.Logger) {
	for _, target := range targets {
		target := target
		go acceptTarget(ctx, target, dialer, leaser, recorder, log)
	}
}

func acceptTarget(ctx context.Context, target Target, dialer backend.Dialer, leaser AccountLeaser, recorder SessionRecorder, log *slog.Logger) {
	for {
		raw, err := target.Listener.Accept()
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				log.Error("Minecraft accept failed", "server", target.Name, "error", err)
			}
			return
		}
		inbound, ok := raw.(*minecraft.Conn)
		if !ok {
			_ = raw.Close()
			log.Error("unexpected accepted connection type", "server", target.Name, "type", fmt.Sprintf("%T", raw))
			continue
		}
		go serveSession(ctx, target, inbound, dialer, leaser, recorder, log)
	}
}

func serveSession(parent context.Context, target Target, inbound *minecraft.Conn, dialer backend.Dialer, leaser AccountLeaser, recorder SessionRecorder, log *slog.Logger) {
	started := time.Now()
	sessionLog := log.With("server", target.Name, "network_id", target.NetworkID, "backend", target.BackendURL, "remote", inbound.RemoteAddr())
	defer func() {
		if recovered := recover(); recovered != nil {
			sessionLog.Error("session panic recovered", "panic", recovered)
		}
		sessionLog.Info("session ended", "duration", time.Since(started).Round(time.Millisecond))
	}()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer inbound.Close()
	identity := inbound.IdentityData()
	lease, err := leaser.Acquire(ctx, accounts.Player{XUID: identity.XUID, Gamertag: identity.DisplayName}, target.Name)
	if err != nil {
		sessionLog.Warn("proxy account unavailable", "player", identity.DisplayName, "xuid", identity.XUID, "error", err)
		message := "Unable to assign a proxy player account"
		if errors.Is(err, accounts.ErrPoolExhausted) || errors.Is(err, accounts.ErrMappedAccountBusy) {
			message = "No proxy player account is available"
		}
		_ = target.Listener.Disconnect(inbound, message)
		return
	}
	defer lease.Release()
	sessionLog = sessionLog.With("console_player", identity.DisplayName, "helper_label", lease.Account.PersonLabel, "helper_gamertag", lease.Account.Gamertag)
	historyID := ""
	disconnectReason := "connection closed"
	if recorder != nil {
		historyID = recorder.Connected(sessionhistory.Event{ConsoleGamertag: identity.DisplayName, ConsoleXUID: identity.XUID, AccountLabel: lease.Account.PersonLabel, AccountGamertag: lease.Account.Gamertag, Server: target.Name})
		defer func() { recorder.Disconnected(historyID, disconnectReason) }()
	}
	dialCtx, dialCancel := backendDialContext(ctx)
	upstream, err := dialer.Dial(dialCtx, target.BackendURL, inbound, lease.TokenSource)
	dialCancel()
	if err != nil {
		disconnectReason = "backend connection failed: " + err.Error()
		sessionLog.Error("backend connection failed", "error", err)
		_ = target.Listener.Disconnect(inbound, "Backend unavailable")
		return
	}
	defer upstream.Close()
	sessionLog.Info("backend connected")
	gameData := upstream.GameData()
	sessionLog.Info("backend world eligibility",
		"achievements_disabled", gameData.AchievementsDisabled,
		"commands_enabled", gameData.CommandsEnabled,
		"education_features_enabled", gameData.EducationFeaturesEnabled)

	ready := make(chan error, 2)
	go func() { ready <- inbound.StartGame(gameData) }()
	go func() { ready <- upstream.DoSpawn() }()
	for range 2 {
		if err := <-ready; err != nil {
			disconnectReason = "session startup failed: " + err.Error()
			sessionLog.Error("Minecraft session startup failed", "error", err)
			_ = target.Listener.Disconnect(inbound, "Connection setup failed")
			return
		}
	}
	sessionLog.Info("Minecraft relay started")

	result := make(chan error, 2)
	go func() { result <- relayPackets(inbound, upstream, sessionLog, "console_to_server") }()
	go func() { result <- relayPackets(upstream, inbound, sessionLog, "server_to_console") }()
	err = <-result
	disconnectReason = disconnectMessage(err)
	sessionLog.Info("Minecraft relay stopped", "reason", err)
	cancel()
	_ = target.Listener.Disconnect(inbound, disconnectMessage(err))
}

type packetConnection interface {
	ReadPacket() (packet.Packet, error)
	WritePacket(packet.Packet) error
}

func relayPackets(source, destination packetConnection, log *slog.Logger, direction string) error {
	for {
		pk, err := source.ReadPacket()
		if err != nil {
			return err
		}
		if fov, ok := cameraFieldOfView(pk); ok {
			log.Info("camera FOV instruction relayed", "direction", direction, "field_of_view", fov)
		}
		if err := destination.WritePacket(pk); err != nil {
			return err
		}
	}
}

func cameraFieldOfView(pk packet.Packet) (float32, bool) {
	instruction, ok := pk.(*packet.CameraInstruction)
	if !ok {
		return 0, false
	}
	fov, ok := instruction.FieldOfView.Value()
	if !ok {
		return 0, false
	}
	return fov.FieldOfView, true
}

func disconnectMessage(err error) string {
	var disconnect minecraft.DisconnectError
	if errors.As(err, &disconnect) {
		return disconnect.Error()
	}
	return "Connection lost"
}
