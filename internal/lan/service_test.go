package lan

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	netherdiscovery "github.com/df-mc/go-nethernet/discovery"
)

const (
	creativeID uint64 = 13468123134000001934
	survivalID uint64 = 13468123136000001936
)

func testAdvertisements() []Advertisement {
	return []Advertisement{
		{
			Name:      "Private Creative Server",
			Backend:   "192.168.1.246:19134",
			NetworkID: creativeID,
			ServerData: netherdiscovery.ServerData{
				ServerName: "Private Creative Server", Protocol: 2193, Version: "1.26.51",
				LevelName: "Private Creative Server", GameType: netherdiscovery.GameTypeCreative,
				PlayerCount: 0, MaxPlayerCount: 10, AcceptsOnlineAuth: true,
				AcceptsSelfSignedAuth: true, ConnectionType: 4,
			},
		},
		{
			Name:      "Private Survival Server",
			Backend:   "192.168.1.247:19136",
			NetworkID: survivalID,
			ServerData: netherdiscovery.ServerData{
				ServerName: "Private Survival Server", Protocol: 2193, Version: "1.26.51",
				LevelName: "Private Survival Server", GameType: netherdiscovery.GameTypeSurvival,
				PlayerCount: 0, MaxPlayerCount: 10, AcceptsOnlineAuth: true,
				AcceptsSelfSignedAuth: true, ConnectionType: 4,
			},
		},
	}
}

func TestServiceRespondsWithEveryAdvertisement(t *testing.T) {
	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(serverConn, testAdvertisements(), slog.New(slog.NewTextHandler(io.Discard, nil))).Serve(ctx)
	}()

	clientID := uint64(99887766)
	request := netherdiscovery.Marshal(&netherdiscovery.RequestPacket{}, clientID)
	if _, err := clientConn.WriteToUDP(request, serverConn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	if err := clientConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}

	type received struct {
		id   uint64
		data netherdiscovery.ServerData
	}
	got := make(map[uint64]received)
	buf := make([]byte, 65535)
	for range 2 {
		n, _, err := clientConn.ReadFromUDP(buf)
		if err != nil {
			t.Fatal(err)
		}
		packet, senderID, err := netherdiscovery.Unmarshal(buf[:n])
		if err != nil {
			t.Fatal(err)
		}
		response, ok := packet.(*netherdiscovery.ResponsePacket)
		if !ok {
			t.Fatalf("packet type = %T; want *discovery.ResponsePacket", packet)
		}
		var data netherdiscovery.ServerData
		if err := data.UnmarshalBinary(response.ApplicationData); err != nil {
			t.Fatal(err)
		}
		got[senderID] = received{id: senderID, data: data}
	}

	for _, want := range testAdvertisements() {
		received, ok := got[want.NetworkID]
		if !ok {
			t.Fatalf("missing response for network ID %d; got IDs %v", want.NetworkID, got)
		}
		if received.id == clientID {
			t.Fatalf("response reused client network ID %d", clientID)
		}
		data := received.data
		if data.ServerName != want.ServerData.ServerName || data.LevelName != want.ServerData.LevelName || data.Protocol != 2193 || data.Version != "1.26.51" || data.GameType != want.ServerData.GameType || data.PlayerCount != 0 || data.MaxPlayerCount != 10 || !data.AcceptsOnlineAuth || !data.AcceptsSelfSignedAuth || data.ConnectionType != 4 {
			t.Fatalf("network %d data = %+v; want %+v", want.NetworkID, data, want.ServerData)
		}
	}

	if err := clientConn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := clientConn.ReadFromUDP(buf); err == nil {
		t.Fatal("received unexpected third advertisement")
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("third read error = %v; want timeout", err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() after cancellation = %v; want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not stop within one second")
	}
}

func TestServiceContinuesAfterMalformedDatagram(t *testing.T) {
	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(serverConn, testAdvertisements(), slog.New(slog.NewTextHandler(&logs, nil))).Serve(ctx)
	}()

	serverAddr := serverConn.LocalAddr().(*net.UDPAddr)
	malformed := bytes.Repeat([]byte{0xab}, 300)
	if _, err := clientConn.WriteToUDP(malformed, serverAddr); err != nil {
		t.Fatal(err)
	}
	request := netherdiscovery.Marshal(&netherdiscovery.RequestPacket{}, 11223344)
	if _, err := clientConn.WriteToUDP(request, serverAddr); err != nil {
		t.Fatal(err)
	}
	if err := clientConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 65535)
	for range 2 {
		if _, _, err := clientConn.ReadFromUDP(buf); err != nil {
			t.Fatalf("valid request was not answered after malformed datagram: %v", err)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() = %v; want nil after cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not stop")
	}
	logText := logs.String()
	expectedHex := hex.EncodeToString(malformed[:256])
	for _, want := range []string{
		"time=",
		"from=" + clientConn.LocalAddr().String(),
		"rejected discovery datagram",
		"decode discovery datagram",
		"error=",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("logs = %q; want %q", logText, want)
		}
	}
	if strings.Contains(logText, "hex=") || strings.Contains(logText, expectedHex) {
		t.Fatalf("normal logs exposed raw datagram: %q", logText)
	}
}

func TestServiceObservesMessageWithoutReply(t *testing.T) {
	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(serverConn, testAdvertisements(), slog.New(slog.NewTextHandler(&logs, nil))).Serve(ctx)
	}()

	clientID := uint64(55667788)
	message := &netherdiscovery.MessagePacket{RecipientID: creativeID, Data: "CONNECTREQUEST 42 sample-offer"}
	if _, err := clientConn.WriteToUDP(netherdiscovery.Marshal(message, clientID), serverConn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	if err := clientConn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := clientConn.ReadFromUDP(make([]byte, 65535)); err == nil {
		t.Fatal("message packet unexpectedly received a Stage 1 reply")
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("read error = %v; want timeout", err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	text := logs.String()
	for _, want := range []string{"signalling message observed", "55667788", "13468123134000001934", "signal_bytes=30"} {
		if !strings.Contains(text, want) {
			t.Fatalf("logs = %q; want %q", text, want)
		}
	}
	if strings.Contains(text, "sample-offer") {
		t.Fatalf("logs exposed signalling payload: %q", text)
	}
}

func TestServiceRedactsLongSignallingPayload(t *testing.T) {
	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(serverConn, testAdvertisements(), slog.New(slog.NewTextHandler(&logs, nil))).Serve(ctx)
	}()
	longSignal := strings.Repeat("x", 2000)
	message := &netherdiscovery.MessagePacket{RecipientID: creativeID, Data: longSignal}
	if _, err := clientConn.WriteToUDP(netherdiscovery.Marshal(message, 1234), serverConn.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	if err := clientConn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	_, _, _ = clientConn.ReadFromUDP(make([]byte, 65535))
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	text := logs.String()
	if !strings.Contains(text, "signal_bytes=2000") {
		t.Fatalf("logs do not report signal length: %q", text)
	}
	if strings.Contains(text, strings.Repeat("x", 32)) {
		t.Fatalf("logs contain signalling payload: %q", text)
	}
}

type stubAddr string

func (a stubAddr) Network() string { return "udp" }
func (a stubAddr) String() string  { return string(a) }

type writeFailConn struct {
	writes atomic.Int32
}

func (*writeFailConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, net.ErrClosed }
func (c *writeFailConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	attempt := c.writes.Add(1)
	if attempt == 1 {
		return 0, errors.New("injected write failure")
	}
	return len(p), nil
}
func (*writeFailConn) Close() error                     { return nil }
func (*writeFailConn) LocalAddr() net.Addr              { return stubAddr("127.0.0.1:7551") }
func (*writeFailConn) SetDeadline(time.Time) error      { return nil }
func (*writeFailConn) SetReadDeadline(time.Time) error  { return nil }
func (*writeFailConn) SetWriteDeadline(time.Time) error { return nil }

func TestServiceAttemptsAllWritesAfterOneFails(t *testing.T) {
	conn := &writeFailConn{}
	var logs bytes.Buffer
	service := New(conn, testAdvertisements(), slog.New(slog.NewTextHandler(&logs, nil)))
	request := netherdiscovery.Marshal(&netherdiscovery.RequestPacket{}, 42)
	err := service.handleDatagram(request, stubAddr("192.168.1.50:7551"))
	if err != nil {
		t.Fatalf("handleDatagram() = %v; want non-fatal write failure", err)
	}
	if got := conn.writes.Load(); got != 2 {
		t.Fatalf("write attempts = %d; want 2", got)
	}
	if !strings.Contains(logs.String(), "Private Creative Server") || !strings.Contains(logs.String(), "injected write failure") {
		t.Fatalf("logs = %q; want failed advertisement name and error", logs.String())
	}
}

type blockingConn struct {
	closed     chan struct{}
	closeOnce  sync.Once
	closeCount atomic.Int32
}

func newBlockingConn() *blockingConn { return &blockingConn{closed: make(chan struct{})} }
func (c *blockingConn) ReadFrom([]byte) (int, net.Addr, error) {
	<-c.closed
	return 0, nil, net.ErrClosed
}
func (*blockingConn) WriteTo(p []byte, addr net.Addr) (int, error) { return len(p), nil }
func (c *blockingConn) Close() error {
	c.closeOnce.Do(func() { c.closeCount.Add(1); close(c.closed) })
	return nil
}
func (*blockingConn) LocalAddr() net.Addr              { return stubAddr("127.0.0.1:7551") }
func (*blockingConn) SetDeadline(time.Time) error      { return nil }
func (*blockingConn) SetReadDeadline(time.Time) error  { return nil }
func (*blockingConn) SetWriteDeadline(time.Time) error { return nil }

func TestServiceStopsOnCancellation(t *testing.T) {
	conn := newBlockingConn()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- New(conn, nil, nil).Serve(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve() = %v; want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not return within one second")
	}
	if got := conn.closeCount.Load(); got != 1 {
		t.Fatalf("Close calls = %d; want 1", got)
	}
}
