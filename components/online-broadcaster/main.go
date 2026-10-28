// NetherNetOnline is an offline compatibility and admission proof of concept.
// It deliberately contains no session startup or token persistence path.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"os"
	"strconv"

	broadcaster "github.com/HashimTheArab/go-mcxboxbroadcast"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
)

const upstreamCommit = "bb06c2ddc46fb61b2bbbb886f08d6edf11383a3e"
const maxEnvelope = 16 * 1024

type capability struct {
	Protocol               int    `json:"protocol"`
	GameVersion            string `json:"game_version"`
	UpstreamCommit         string `json:"upstream_commit"`
	DirectTransfer         bool   `json:"direct_transfer"`
	AuthenticatedAdmission bool   `json:"authenticated_admission"`
	PerClientRoute         bool   `json:"per_client_route"`
	RemoteIngressVerified  bool   `json:"remote_ingress_verified"`
	LANFirstVerified       bool   `json:"lan_first_verified"`
}

type envelope struct {
	Protocol              int    `json:"protocol"`
	GameVersion           string `json:"game_version"`
	UpstreamCommit        string `json:"upstream_commit"`
	OnlineMode            bool   `json:"online_mode"`
	Relay                 bool   `json:"relay"`
	RemoteIngressVerified bool   `json:"remote_ingress_verified"`
	LANFirstVerified      bool   `json:"lan_first_verified"`
	XUID                  string `json:"xuid"`
	Candidates            []struct {
		Type    string `json:"type"`
		Address string `json:"address"`
	} `json:"candidates"`
}

func upstreamConfig() broadcaster.Config {
	return broadcaster.Config{Relay: nil, ListenConfig: minecraft.ListenConfig{
		AuthenticationDisabled: false,
		Allow: func(_ net.Addr, _ login.IdentityData, _ login.ClientData) (string, bool) {
			return "Online admission is unavailable: locality and protected ingress are unverified.", false
		},
	}}
}

func emit(out io.Writer, v any) { _ = json.NewEncoder(out).Encode(v) }
func deny(out io.Writer, code string) int {
	emit(out, struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}{false, code})
	return 1
}

func run(args []string, in io.Reader, out io.Writer) int {
	if len(args) != 1 {
		return deny(out, "invalid_arguments")
	}
	switch args[0] {
	case "--probe":
		// Reference actual compiled upstream configuration and protocol constants.
		c := upstreamConfig()
		emit(out, capability{protocol.CurrentProtocol, protocol.CurrentVersion, upstreamCommit, c.Relay == nil, !c.ListenConfig.AuthenticationDisabled && c.ListenConfig.Allow != nil, false, false, false})
		return 0
	case "--validate":
		b, err := io.ReadAll(io.LimitReader(in, maxEnvelope+1))
		if err != nil || len(b) > maxEnvelope {
			return deny(out, "input_limit_or_read_error")
		}
		var e envelope
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		if d.Decode(&e) != nil {
			return deny(out, "invalid_settings")
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return deny(out, "invalid_settings")
		}
		if e.Protocol != protocol.CurrentProtocol || e.GameVersion != protocol.CurrentVersion || e.UpstreamCommit != upstreamCommit {
			return deny(out, "protocol_or_build_mismatch")
		}
		if !e.OnlineMode || e.Relay {
			return deny(out, "unsafe_authentication_or_relay")
		}
		if _, err := strconv.ParseUint(e.XUID, 10, 64); err != nil || e.XUID == "0" {
			return deny(out, "missing_or_invalid_xuid")
		}
		if len(e.Candidates) == 0 {
			return deny(out, "unknown_transport_locality")
		}
		for _, c := range e.Candidates {
			if c.Type != "host" || net.ParseIP(c.Address) == nil {
				return deny(out, "unknown_transport_locality")
			}
		}
		// Caller claims can never upgrade compiled capability or authorize ingress.
		return deny(out, "locality_and_protected_ingress_unverified")
	case "--run":
		return deny(out, "activation_blocked_protocol_locality_and_ingress_unverified")
	default:
		return deny(out, "invalid_arguments")
	}
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout)) }
