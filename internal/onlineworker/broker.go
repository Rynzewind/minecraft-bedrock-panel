// Package onlineworker contains the private pipe protocol to the isolated broadcaster.
// Refresh credentials and arbitrary child diagnostics never cross this boundary.
package onlineworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/oauth2"
	"io"
	"strconv"
	"time"
)

var ErrProtocol = errors.New("Online worker protocol unavailable")

type Bootstrap struct {
	Type                     string `json:"type"`
	Version                  int    `json:"version"`
	ServerID                 string `json:"server_id"`
	AccountID                string `json:"account_id"`
	GatewayHost              string `json:"gateway_host"`
	GatewayPort              uint16 `json:"gateway_port"`
	WorldName                string `json:"world_name"`
	GameType                 string `json:"game_type,omitempty"`
	MaxPlayers               int    `json:"max_players,omitempty"`
	ExpectedProtocol         int    `json:"expected_protocol"`
	AllowUnknownFriendsRoute bool   `json:"allow_unknown_friends_route"`
}
type Callbacks struct {
	Token     func(context.Context) (*oauth2.Token, error)
	Authorize func(string) bool
	Event     func(string)
}
type frame struct {
	Type      string `json:"type"`
	ID        uint64 `json:"id,omitempty"`
	ServerID  string `json:"server_id,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	XUID      string `json:"xuid,omitempty"`
	Code      string `json:"code,omitempty"`
}
type accessToken struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	Expiry      time.Time `json:"expiry"`
}

// Serve must run only on inherited anonymous pipes, never a network socket.
// The owner cancels context and closes pipes when stopping the worker.
func Serve(ctx context.Context, in io.Reader, out io.Writer, cfg Bootstrap, cb Callbacks) error {
	enc := json.NewEncoder(out)
	if err := enc.Encode(cfg); err != nil {
		return ErrProtocol
	}
	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 4096), 65536)
	seen := make(map[uint64]bool)
	for scan.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var f frame
		dec := json.NewDecoder(bytes.NewReader(scan.Bytes()))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return ErrProtocol
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			return ErrProtocol
		}
		if f.Type == "event" {
			if f.ID != 0 || f.ServerID != "" || f.AccountID != "" || f.XUID != "" {
				return ErrProtocol
			}
			switch f.Code {
			case "starting", "advertising", "stopped", "authentication_required", "error":
			default:
				return ErrProtocol
			}
			if cb.Event != nil {
				cb.Event(f.Code)
			}
			continue
		}
		if f.ID == 0 || seen[f.ID] || f.ServerID != cfg.ServerID || f.AccountID != cfg.AccountID || f.Code != "" {
			return ErrProtocol
		}
		// IDs are monotonic in the child. Bound remembered IDs and reject replays.
		if len(seen) >= 65536 {
			return ErrProtocol
		}
		seen[f.ID] = true
		switch f.Type {
		case "token_request":
			if f.XUID != "" || cb.Token == nil {
				return ErrProtocol
			}
			tokenCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			tok, err := cb.Token(tokenCtx)
			cancel()
			r := struct {
				Type      string       `json:"type"`
				ID        uint64       `json:"id"`
				Token     *accessToken `json:"token,omitempty"`
				ErrorCode string       `json:"error_code,omitempty"`
			}{Type: "token_response", ID: f.ID}
			if err != nil || tok == nil || tok.AccessToken == "" || len(tok.AccessToken) > 32768 || !tok.Expiry.After(time.Now().Add(30*time.Second)) {
				r.ErrorCode = "authentication_required"
			} else {
				r.Token = &accessToken{tok.AccessToken, "Bearer", tok.Expiry}
			}
			if enc.Encode(r) != nil {
				return ErrProtocol
			}
		case "authorize_request":
			n, e := strconv.ParseUint(f.XUID, 10, 64)
			allowed := e == nil && n != 0 && cb.Authorize != nil && cb.Authorize(f.XUID)
			if enc.Encode(struct {
				Type    string `json:"type"`
				ID      uint64 `json:"id"`
				Allowed bool   `json:"allowed"`
			}{"authorize_response", f.ID, allowed}) != nil {
				return ErrProtocol
			}
		default:
			return ErrProtocol
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrProtocol
}
