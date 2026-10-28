package proxy

import (
	"context"
	"github.com/df-mc/go-nethernet"
	"strings"
	"testing"
)

type capturedReply struct {
	nethernet.Signaling
	got *nethernet.Signal
}

func (c *capturedReply) Signal(_ context.Context, s *nethernet.Signal) error { c.got = s; return nil }

func TestNativeLANAnswerPreservesTransportAndRouting(t *testing.T) {
	original := &nethernet.Signal{Type: nethernet.SignalTypeAnswer, ConnectionID: 42, NetworkID: "99", Data: "v=0\r\na=ice-ufrag:test\r\na=identity:temporary\r\na=fingerprint:sha-256 test\r\na=candidate:test\r\n"}
	capture := &capturedReply{}
	if err := (nativeLANSignaling{capture}).Signal(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(capture.got.Data, "a=identity:") {
		t.Fatal("identity assertion still present")
	}
	if capture.got.ConnectionID != 42 || capture.got.NetworkID != "99" || capture.got.Type != original.Type {
		t.Fatal("routing changed")
	}
	if !strings.Contains(capture.got.Data, "a=fingerprint:sha-256 test\r\na=candidate:test\r\n") || !strings.Contains(capture.got.Data, "a=ice-ufrag:test") {
		t.Fatal("transport fields changed")
	}
	if !strings.Contains(original.Data, "a=identity:") {
		t.Fatal("caller-owned signal mutated")
	}
	offer := &nethernet.Signal{Type: nethernet.SignalTypeOffer, Data: original.Data}
	if err := (nativeLANSignaling{capture}).Signal(context.Background(), offer); err != nil {
		t.Fatal(err)
	}
	if capture.got != offer {
		t.Fatal("non-answer signal changed")
	}
}
