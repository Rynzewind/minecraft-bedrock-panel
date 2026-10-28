package lan

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/df-mc/go-nethernet"
	netherdiscovery "github.com/df-mc/go-nethernet/discovery"
)

type captureNotifier struct{ ch chan *nethernet.Signal }

func (n *captureNotifier) NotifySignal(s *nethernet.Signal) bool { n.ch <- s; return true }

type capturedWrite struct {
	data []byte
	addr net.Addr
}
type capturePacketConn struct{ writes chan capturedWrite }

func (*capturePacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, net.ErrClosed }
func (c *capturePacketConn) WriteTo(p []byte, a net.Addr) (int, error) {
	c.writes <- capturedWrite{append([]byte(nil), p...), a}
	return len(p), nil
}
func (*capturePacketConn) Close() error                     { return nil }
func (*capturePacketConn) LocalAddr() net.Addr              { return stubAddr("0.0.0.0:7551") }
func (*capturePacketConn) SetDeadline(time.Time) error      { return nil }
func (*capturePacketConn) SetReadDeadline(time.Time) error  { return nil }
func (*capturePacketConn) SetWriteDeadline(time.Time) error { return nil }

func TestMessageRoutesOnlyToRecipientEndpoint(t *testing.T) {
	conn := &capturePacketConn{writes: make(chan capturedWrite, 1)}
	s := New(conn, testAdvertisements(), nil)
	creative := &captureNotifier{ch: make(chan *nethernet.Signal, 1)}
	survival := &captureNotifier{ch: make(chan *nethernet.Signal, 1)}
	s.Signaling(creativeID).Notify(creative)
	s.Signaling(survivalID).Notify(survival)
	p := netherdiscovery.Marshal(&netherdiscovery.MessagePacket{RecipientID: creativeID, Data: "CONNECTREQUEST 99 sample-offer"}, 3829603440984922419)
	if err := s.handleDatagram(p, stubAddr("192.168.1.125:7551")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-creative.ch:
		if got.Type != nethernet.SignalTypeOffer || got.ConnectionID != 99 || got.Data != "sample-offer" || got.NetworkID != "3829603440984922419" {
			t.Fatalf("signal = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("creative did not receive signal")
	}
	select {
	case got := <-survival.ch:
		t.Fatalf("survival received %+v", got)
	default:
	}
}

func TestOutgoingSignalUsesServerAndClientIDs(t *testing.T) {
	conn := &capturePacketConn{writes: make(chan capturedWrite, 1)}
	s := New(conn, testAdvertisements(), nil)
	endpoint := s.Signaling(creativeID)
	clientID := uint64(3829603440984922419)
	incoming := netherdiscovery.Marshal(&netherdiscovery.MessagePacket{RecipientID: creativeID, Data: "CONNECTREQUEST 99 offer"}, clientID)
	if err := s.handleDatagram(incoming, stubAddr("192.168.1.125:7551")); err != nil {
		t.Fatal(err)
	}
	err := endpoint.Signal(context.Background(), &nethernet.Signal{Type: nethernet.SignalTypeAnswer, ConnectionID: 99, NetworkID: "3829603440984922419", Data: "answer"})
	if err != nil {
		t.Fatal(err)
	}
	w := <-conn.writes
	packet, sender, err := netherdiscovery.Unmarshal(w.data)
	if err != nil {
		t.Fatal(err)
	}
	message := packet.(*netherdiscovery.MessagePacket)
	if sender != creativeID || message.RecipientID != clientID || message.Data != "CONNECTRESPONSE 99 answer" || w.addr.String() != "192.168.1.125:7551" {
		t.Fatalf("sender=%d message=%+v addr=%s", sender, message, w.addr)
	}
}

func TestEmptyAndPingMessagesAreIgnored(t *testing.T) {
	s := New(&capturePacketConn{writes: make(chan capturedWrite, 1)}, testAdvertisements(), nil)
	n := &captureNotifier{ch: make(chan *nethernet.Signal, 1)}
	s.Signaling(creativeID).Notify(n)
	for _, data := range []string{"", "Ping"} {
		p := netherdiscovery.Marshal(&netherdiscovery.MessagePacket{RecipientID: creativeID, Data: data}, 123)
		if err := s.handleDatagram(p, stubAddr("192.168.1.125:7551")); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case got := <-n.ch:
		t.Fatalf("received %+v", got)
	default:
	}
}

var _ nethernet.Notifier = (*captureNotifier)(nil)
var _ net.PacketConn = (*capturePacketConn)(nil)
