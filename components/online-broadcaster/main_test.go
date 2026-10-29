package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
)

func TestProbeUsesCompiledCapabilities(t *testing.T) {
	var out bytes.Buffer
	if run([]string{"--probe"}, strings.NewReader(""), &out) != 0 {
		t.Fatal(out.String())
	}
	var p capability
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Protocol != 2225 || p.GameVersion != "1.26.60" || !p.DirectTransfer || !p.AuthenticatedAdmission || !p.BrokeredTokens || p.PerClientRoute || p.RemoteIngressVerified || p.LANFirstVerified {
		t.Fatal(out.String())
	}
	c := upstreamConfig()
	if c.ListenConfig.AuthenticationDisabled || c.Relay != nil || c.ListenConfig.Allow == nil {
		t.Fatal("unsafe upstream defaults")
	}
	if _, allowed := c.ListenConfig.Allow(nil, login.IdentityData{XUID: "123"}, login.ClientData{}); allowed {
		t.Fatal("unverified connection was admitted")
	}
}

func TestValidationAlwaysFailsClosed(t *testing.T) {
	cases := []string{
		`{`, strings.Repeat(" ", maxEnvelope+1),
		`{"protocol":2225,"game_version":"1.26.50"}`,
		`{"protocol":2193,"game_version":"1.26.50","upstream_commit":"` + upstreamCommit + `","online_mode":true,"xuid":"123","candidates":[{"type":"host","address":"192.168.1.2"}]}`,
		`{"protocol":2193,"game_version":"1.26.50","upstream_commit":"` + upstreamCommit + `","online_mode":true,"xuid":"","candidates":[]}`,
		`{"protocol":2193,"game_version":"1.26.50","upstream_commit":"` + upstreamCommit + `","online_mode":true,"xuid":"123","candidates":[{"type":"unknown","address":"192.168.1.2"}]}`,
		`{"refresh_token":"SENSITIVE_TEST_TOKEN"}`,
		`{} {}`,
		`{"protocol":2193,"game_version":"1.26.50","upstream_commit":"` + upstreamCommit + `","online_mode":true,"xuid":"123","remote_ingress_verified":true,"lan_first_verified":true,"candidates":[{"type":"host","address":"192.168.1.2"}]}`,
	}
	for _, input := range cases {
		t.Run(input[:min(len(input), 30)], func(t *testing.T) {
			var out bytes.Buffer
			if run([]string{"--validate"}, strings.NewReader(input), &out) == 0 {
				t.Fatal("unsafe acceptance")
			}
			if strings.Contains(out.String(), "SENSITIVE_TEST_TOKEN") || len(out.Bytes()) > 2048 {
				t.Fatal("secret or unbounded output")
			}
			if !json.Valid(out.Bytes()) {
				t.Fatal(out.String())
			}
		})
	}
}

func TestRunCannotActivate(t *testing.T) {
	for _, args := range [][]string{{"--run"}, {"--run", "--force"}, {"--probe", "--run"}} {
		var out bytes.Buffer
		if run(args, strings.NewReader(""), &out) == 0 {
			t.Fatal("activated")
		}
	}
}
