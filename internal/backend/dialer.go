package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/df-mc/go-nethernet/endpoint"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"golang.org/x/oauth2"
)

type Dialer interface {
	Dial(context.Context, string, *minecraft.Conn, oauth2.TokenSource) (*minecraft.Conn, error)
}

type NetherNetDialer struct {
	Log *slog.Logger
}

func (d NetherNetDialer) minecraftDialer(clientData login.ClientData, tokenSource oauth2.TokenSource) minecraft.Dialer {
	return minecraft.Dialer{ClientData: clientData, TokenSource: tokenSource, ErrorLog: d.Log}
}

func (d NetherNetDialer) Dial(ctx context.Context, backendURL string, inbound *minecraft.Conn, tokenSource oauth2.TokenSource) (*minecraft.Conn, error) {
	if inbound == nil {
		return nil, errors.New("backend dial: nil inbound Minecraft connection")
	}
	client := endpoint.NewClient()
	conn, err := d.minecraftDialer(inbound.ClientData(), tokenSource).DialContextNetwork(ctx, minecraft.NetherNet{Signaling: client, Log: d.Log}, backendURL)
	if err != nil {
		return nil, fmt.Errorf("dial backend %s: %w", backendURL, err)
	}
	return conn, nil
}

var _ Dialer = NetherNetDialer{}
