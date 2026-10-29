package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/df-mc/go-nethernet"
	"github.com/df-mc/go-nethernet/endpoint"
	"net"
	"net/http"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/sandertv/gophertunnel/minecraft"
	"golang.org/x/oauth2"
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/config"
)

func TestGatewayOffersCurrentClientHTTPDiscovery(t *testing.T) {
	if os.Getenv("NETHERNET_GATEWAY_NETWORK_TEST") != "1" {
		t.Skip("authenticated listener startup uses Minecraft service discovery")
	}
	reservation, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.LocalAddr().String()
	reservation.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	opts := RemoteGatewayOptions{Server: config.Server{Name: "Creative", LevelName: "Creative World", GameType: "creative", MaxPlayerCount: 10, Backend: "127.0.0.1:19134", NetworkID: "42"}, Authorize: func(string) bool { return false },
		Leaser: gatewayTestLeaser(func(context.Context, accounts.Player, string) (*accounts.Lease, error) {
			t.Error("discovery leased helper")
			return nil, errors.New("deny")
		}),
		Dialer: gatewayTestDialer(func(context.Context, string, *minecraft.Conn, oauth2.TokenSource) (*minecraft.Conn, error) {
			t.Error("discovery dialed backend")
			return nil, errors.New("deny")
		})}
	gatewayTestIdentity(t, &opts)
	g, err := StartRemoteGateway(ctx, address, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + address + "/v1/join")
	if err != nil {
		t.Fatalf("gateway lacks NetherNet discovery: %v", err)
	}
	defer response.Body.Close()
	var status struct {
		Name, Level, Version           string
		Protocol, MaxPlayers, GameType int
	}
	if response.StatusCode != 200 {
		t.Fatalf("discovery HTTP %d", response.StatusCode)
	}
	if err = json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Name != "Creative" || status.Level != "Creative World" || status.GameType != 1 || status.Protocol != 2225 || status.MaxPlayers != 10 {
		t.Fatalf("incorrect HTTP status: %+v", status)
	}
	// A discoverable gateway must still reject an unsigned/anonymous SDP offer
	// before a helper lease, backend dial or private resource-pack delivery.
	dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
	defer dialCancel()
	if conn, err := (nethernet.Dialer{}).DialContext(dialCtx, "http://"+address, endpoint.NewClient()); err == nil {
		conn.Close()
		t.Fatal("anonymous WebRTC peer admitted")
	}
	if err = g.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Get("http://" + address + "/v1/join"); err == nil {
		t.Fatal("HTTP signaling survived gateway close")
	}
	u, err := net.ListenPacket("udp4", address)
	if err != nil {
		t.Fatal("UDP mux socket leaked", err)
	}
	u.Close()
}

func gatewayTestIdentity(t *testing.T, opts *RemoteGatewayOptions) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	opts.OperatorKey = key
	opts.PublicIPv4 = netip.MustParseAddr("127.0.0.1")
	opts.InterfaceIP = net.ParseIP("127.0.0.1")
	if opts.Server.NetworkID == "" {
		opts.Server.NetworkID = "42"
	}
}
