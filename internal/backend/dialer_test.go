package backend

import (
	"context"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"golang.org/x/oauth2"
)

func TestNetherNetDialerRejectsNilInbound(t *testing.T) {
	if _, err := (NetherNetDialer{}).Dial(context.Background(), "http://192.168.1.246:19134", nil, nil); err == nil {
		t.Fatal("nil inbound connection accepted")
	}
}

func TestMinecraftDialerPreservesConsoleVisualProfile(t *testing.T) {
	want := login.ClientData{
		DeviceOS: protocol.DeviceOrbis,
		GUIScale: -1, UIProfile: 0, MaxViewDistance: 32,
		MemoryTier: 4, GraphicsMode: 2,
		CurrentInputMode: 2, DefaultInputMode: 2,
	}
	got := (NetherNetDialer{}).minecraftDialer(want, nil).ClientData
	if got.DeviceOS != want.DeviceOS || got.GUIScale != want.GUIScale || got.UIProfile != want.UIProfile ||
		got.MaxViewDistance != want.MaxViewDistance || got.MemoryTier != want.MemoryTier ||
		got.GraphicsMode != want.GraphicsMode || got.CurrentInputMode != want.CurrentInputMode ||
		got.DefaultInputMode != want.DefaultInputMode {
		t.Fatalf("visual profile changed: got %+v, want %+v", got, want)
	}
}

func TestMinecraftDialerUsesConfiguredTokenSource(t *testing.T) {
	source := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test"})
	d := NetherNetDialer{}
	got := d.minecraftDialer(login.ClientData{}, source)
	if got.TokenSource != source {
		t.Fatal("Minecraft dialer did not receive configured token source")
	}
}
