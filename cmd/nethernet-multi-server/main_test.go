package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nethernet-multi-server/internal/config"
	identitystore "nethernet-multi-server/internal/identity"
	"nethernet-multi-server/internal/lan"
	proxyservice "nethernet-multi-server/internal/proxy"
)

func TestMain(m *testing.M) {
	listenAdmin = func(string) (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") }
	startListeners = func(context.Context, *lan.Service, []config.Server, identitystore.Store, *slog.Logger) ([]proxyservice.Target, error) {
		return nil, nil
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

type testPacketConn struct {
	closed chan struct{}
	once   sync.Once
}

func newTestPacketConn() *testPacketConn { return &testPacketConn{closed: make(chan struct{})} }
func (c *testPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	<-c.closed
	return 0, nil, net.ErrClosed
}
func (*testPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) { return len(p), nil }
func (c *testPacketConn) Close() error                               { c.once.Do(func() { close(c.closed) }); return nil }
func (*testPacketConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 7551}
}
func (*testPacketConn) SetDeadline(time.Time) error      { return nil }
func (*testPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (*testPacketConn) SetWriteDeadline(time.Time) error { return nil }

func writeTestConfig(t *testing.T) string {
	t.Helper()
	content := `{
  "listen":"192.168.1.255:7551",
	"interface_ip":"192.168.1.246",
  "servers":[
    {"name":"Private Creative Server","level_name":"Private Creative Server","backend":"192.168.1.246:19134","network_id":"13468123134000001934","protocol":2193,"version":"1.26.51","game_type":"creative","player_count":0,"max_player_count":10,"accepts_online_auth":true,"accepts_self_signed_auth":true,"connection_type":4},
    {"name":"Private Survival Server","level_name":"Private Survival Server","backend":"192.168.1.247:19136","network_id":"13468123136000001936","protocol":2193,"version":"1.26.51","game_type":"survival","player_count":0,"max_player_count":10,"accepts_online_auth":true,"accepts_self_signed_auth":true,"connection_type":4}
  ]
}`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunPrintsStageOneBannerAndServers(t *testing.T) {
	original := listenPacket
	var gotNetwork, gotInterfaceIP, gotAddress string
	listenPacket = func(network, interfaceIP, address string) (net.PacketConn, error) {
		gotNetwork = network
		gotInterfaceIP = interfaceIP
		gotAddress = address
		return newTestPacketConn(), nil
	}
	t.Cleanup(func() { listenPacket = original })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	code := run(ctx, []string{writeTestConfig(t)}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q; want 0", code, stderr.String())
	}
	if gotNetwork != "udp4" {
		t.Fatalf("listen network = %q; want udp4", gotNetwork)
	}
	if gotInterfaceIP != "192.168.1.246" || gotAddress != "192.168.1.255:7551" {
		t.Fatalf("listen interface/address = %q/%q; want 192.168.1.246/192.168.1.255:7551", gotInterfaceIP, gotAddress)
	}
	text := stdout.String()
	for _, want := range []string{
		"NetherNet Multi-Server", "Stage 2 connection proxy",
		"Listening: UDP 0.0.0.0:7551", "Interface: 192.168.1.246", "udp4", "IPv4", "SO_BROADCAST enabled", "IP_RECEIVE_BROADCAST enabled",
		"Private Creative Server", "192.168.1.246:19134", "13468123134000001934",
		"Private Survival Server", "192.168.1.247:19136", "13468123136000001936",
		"Waiting for LAN clients...",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("stdout = %q; want %q", text, want)
		}
	}
}

func TestRunReportsMissingConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{filepath.Join(t.TempDir(), "missing.json")}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "load configuration") {
		t.Fatalf("run() code=%d stderr=%q; want non-zero and load configuration", code, stderr.String())
	}
}

func TestRunRejectsExtraArgumentsWithoutBinding(t *testing.T) {
	original := listenPacket
	called := false
	listenPacket = func(network, interfaceIP, address string) (net.PacketConn, error) {
		called = true
		return nil, errors.New("unexpected")
	}
	t.Cleanup(func() { listenPacket = original })
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"one.json", "two.json"}, &stdout, &stderr)
	if code != 2 || called || !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("run() code=%d called=%v stderr=%q; want 2, false, usage", code, called, stderr.String())
	}
}

func TestRunExplainsPortConflict(t *testing.T) {
	original := listenPacket
	listenPacket = func(network, interfaceIP, address string) (net.PacketConn, error) {
		return nil, errors.New("bind: address already in use")
	}
	t.Cleanup(func() { listenPacket = original })
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{writeTestConfig(t)}, &stdout, &stderr)
	text := stderr.String()
	for _, want := range []string{"UDP 7551", "enable-lan-visibility=false", "another process"} {
		if !strings.Contains(text, want) {
			t.Fatalf("stderr = %q; want %q", text, want)
		}
	}
	if code != 1 {
		t.Fatalf("run() code = %d; want 1", code)
	}
}
