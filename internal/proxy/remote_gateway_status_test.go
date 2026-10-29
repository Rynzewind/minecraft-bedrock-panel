package proxy

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sandertv/go-raknet"
	"github.com/sandertv/gophertunnel/minecraft"
	"golang.org/x/oauth2"
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/config"
)

func TestGatewayPongIdentifiesEachTargetWorldAndMode(t *testing.T) {
	if os.Getenv("NETHERNET_GATEWAY_NETWORK_TEST") != "1" {
		t.Skip("authenticated listener startup requires Minecraft service discovery; use NETHERNET_GATEWAY_NETWORK_TEST=1")
	}
	for _, tc := range []struct{ name, mode, wantMode string }{{"Private Creative Server", "creative", "Creative"}, {"Private Survival Server", "survival", "Survival"}, {"Adventure World", "adventure", "Adventure"}} {
		t.Run(tc.mode, func(t *testing.T) {
			reservation, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := reservation.LocalAddr().String()
			reservation.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			opts := RemoteGatewayOptions{
				Server:    config.Server{Name: tc.name, LevelName: tc.name, GameType: tc.mode, MaxPlayerCount: 10, Backend: "127.0.0.1:19134"},
				Authorize: func(string) bool { return false },
				Leaser: gatewayTestLeaser(func(context.Context, accounts.Player, string) (*accounts.Lease, error) {
					t.Error("ping leased a helper")
					return nil, errors.New("not allowed")
				}),
				Dialer: gatewayTestDialer(func(context.Context, string, *minecraft.Conn, oauth2.TokenSource) (*minecraft.Conn, error) {
					t.Error("ping dialed a backend")
					return nil, errors.New("not allowed")
				}),
			}
			gatewayTestIdentity(t, &opts)
			g, err := StartRemoteGateway(ctx, address, opts)
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			pong, err := raknet.PingContext(ctx, address)
			// Listener startup publishes its pong on a background goroutine.
			for err == nil && len(pong) == 0 && ctx.Err() == nil {
				time.Sleep(10 * time.Millisecond)
				pong, err = raknet.PingContext(ctx, address)
			}
			if err != nil {
				t.Fatal(err)
			}
			fields := strings.Split(string(pong), ";")
			if len(fields) < 10 || fields[1] != tc.name || fields[7] != tc.name || fields[8] != tc.wantMode || fields[5] != "10" {
				t.Fatalf("wrong gateway status: %s", pong)
			}
		})
	}
}

func TestGatewayStatusMetadataAndCapacity(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want int
	}{{"creative", 1}, {"survival", 0}, {"adventure", 2}} {
		p := remoteGatewayStatus{server: config.Server{Name: "Visible World", LevelName: "Stored World", GameType: tc.mode, MaxPlayerCount: 10}}
		got := p.ServerStatus(2, 0)
		if got.ServerName != "Visible World" || got.ServerSubName != "Stored World" || got.GameType != tc.want || got.PlayerCount != 2 || got.MaxPlayers != 10 {
			t.Fatalf("incorrect %s status: %+v", tc.mode, got)
		}
	}
}
