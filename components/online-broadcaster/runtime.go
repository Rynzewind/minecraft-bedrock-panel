package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	broadcaster "github.com/HashimTheArab/go-mcxboxbroadcast"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"golang.org/x/oauth2"
)

const maxFrame = 64 * 1024

var errBroker = errors.New("parent authorization unavailable")

type bootstrap struct {
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

func strictJSON(b []byte, v any) error {
	if len(b) > maxFrame {
		return errBroker
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return errBroker
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errBroker
	}
	return nil
}
func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func validGateway(s string) bool {
	if s == "" || len(s) > 253 || s != strings.TrimSpace(s) || strings.ContainsAny(s, "/\\@[] \t\r\n") {
		return false
	}
	if ip := net.ParseIP(s); ip != nil {
		for _, reserved := range []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"} {
			_, block, _ := net.ParseCIDR(reserved)
			if block.Contains(ip) {
				return false
			}
		}
		return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback()
	}
	if strings.Contains(s, ":") {
		return false
	}
	lower := strings.ToLower(s)
	if lower == "localhost" || lower == "example.com" || lower == "example.net" || lower == "example.org" || strings.HasSuffix(lower, ".example.com") || strings.HasSuffix(lower, ".example.net") || strings.HasSuffix(lower, ".example.org") || strings.HasSuffix(lower, ".example") || strings.HasSuffix(lower, ".test") || strings.HasSuffix(lower, ".invalid") || strings.HasSuffix(lower, ".local") || !strings.Contains(s, ".") {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
func parseBootstrap(b []byte) (bootstrap, error) {
	var c bootstrap
	if strictJSON(b, &c) != nil || c.Type != "bootstrap" || c.Version != 1 || !validID(c.ServerID) || !validID(c.AccountID) || !validGateway(c.GatewayHost) || c.GatewayPort == 0 || c.ExpectedProtocol != protocol.CurrentProtocol || protocol.CurrentProtocol != 2225 || !c.AllowUnknownFriendsRoute || len(c.WorldName) == 0 || len(c.WorldName) > 128 {
		return c, errBroker
	}
	if c.MaxPlayers < 0 {
		return c, errBroker
	}
	switch c.GameType {
	case "", "survival", "creative", "adventure":
	default:
		return c, errBroker
	}
	for _, r := range c.WorldName {
		if unicode.IsControl(r) {
			return c, errBroker
		}
	}
	return c, nil
}

type accessToken struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	Expiry      time.Time `json:"expiry"`
}
type response struct {
	Type      string       `json:"type"`
	ID        uint64       `json:"id"`
	Token     *accessToken `json:"token,omitempty"`
	ErrorCode string       `json:"error_code,omitempty"`
	Allowed   bool         `json:"allowed,omitempty"`
}
type pendingRequest struct {
	kind string
	ch   chan response
}
type writePacket struct {
	data []byte
	done chan error
}
type peer struct {
	ctx        context.Context
	cancel     context.CancelFunc
	binding    bootstrap
	out        io.Writer
	writes     chan writePacket
	writerDone chan struct{}
	closeOnce  sync.Once
	mu         sync.Mutex
	next       uint64
	pending    map[uint64]pendingRequest
	timeout    time.Duration
	stopped    bool
}

func newPeer(ctx context.Context, out io.Writer, b bootstrap) *peer {
	c, cancel := context.WithCancel(ctx)
	p := &peer{ctx: c, cancel: cancel, binding: b, out: out, pending: make(map[uint64]pendingRequest), timeout: 20 * time.Second, writes: make(chan writePacket, 64), writerDone: make(chan struct{})}
	go p.writeLoop()
	return p
}
func (p *peer) close() { p.cancel(); p.closeOnce.Do(func() { close(p.writerDone) }) }
func (p *peer) writeLoop() {
	for {
		select {
		case <-p.writerDone:
			return
		case packet := <-p.writes:
			n, err := p.out.Write(packet.data)
			if n != len(packet.data) && err == nil {
				err = io.ErrShortWrite
			}
			packet.done <- err
			if err != nil {
				p.cancel()
				return
			}
		}
	}
}
func (p *peer) write(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil || len(b) > maxFrame {
		return errBroker
	}
	packet := writePacket{append(b, '\n'), make(chan error, 1)}
	select {
	case <-ctx.Done():
		return errBroker
	case <-p.writerDone:
		return errBroker
	case p.writes <- packet:
	}
	select {
	case <-ctx.Done():
		return errBroker
	case <-p.writerDone:
		return errBroker
	case err := <-packet.done:
		if err != nil {
			return errBroker
		}
		return nil
	}
}
func (p *peer) event(code string) {
	// Events may be emitted during cancellation, but contain only fixed codes.
	ctx, cancel := context.WithTimeout(context.Background(), min(p.timeout, time.Second))
	defer cancel()
	_ = p.write(ctx, struct {
		Type string `json:"type"`
		Code string `json:"code"`
	}{"event", code})
}
func (p *peer) request(kind, xuid string) (response, error) {
	if p.ctx.Err() != nil {
		return response{}, errBroker
	}
	ctx, cancel := context.WithTimeout(p.ctx, p.timeout)
	defer cancel()
	p.mu.Lock()
	if len(p.pending) >= 64 {
		p.mu.Unlock()
		return response{}, errBroker
	}
	p.next++
	id := p.next
	ch := make(chan response, 1)
	p.pending[id] = pendingRequest{kind, ch}
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	req := struct {
		Type      string `json:"type"`
		ID        uint64 `json:"id"`
		ServerID  string `json:"server_id"`
		AccountID string `json:"account_id"`
		XUID      string `json:"xuid,omitempty"`
	}{kind + "_request", id, p.binding.ServerID, p.binding.AccountID, xuid}
	if p.write(ctx, req) != nil {
		return response{}, errBroker
	}
	select {
	case <-ctx.Done():
		return response{}, errBroker
	case r := <-ch:
		if p.ctx.Err() != nil {
			return response{}, errBroker
		}
		return r, nil
	}
}
func (p *peer) Token() (*oauth2.Token, error) {
	r, err := p.request("token", "")
	if err != nil || r.ErrorCode != "" || r.Token == nil || r.Token.AccessToken == "" || len(r.Token.AccessToken) > 32768 || !strings.EqualFold(r.Token.TokenType, "Bearer") || !r.Token.Expiry.After(time.Now().Add(30*time.Second)) {
		p.event("authentication_required")
		return nil, errBroker
	}
	return &oauth2.Token{AccessToken: r.Token.AccessToken, TokenType: "Bearer", Expiry: r.Token.Expiry}, nil
}
func (p *peer) authorize(xuid string) bool {
	n, err := strconv.ParseUint(xuid, 10, 64)
	if err != nil || n == 0 {
		return false
	}
	r, err := p.request("authorize", xuid)
	return err == nil && r.Allowed
}
func (p *peer) read(s *bufio.Scanner) {
	defer p.cancel()
	for s.Scan() {
		b := s.Bytes()
		var h struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(b, &h) != nil {
			return
		}
		if h.Type == "stop" {
			var v struct {
				Type string `json:"type"`
			}
			if strictJSON(b, &v) != nil {
				return
			}
			p.mu.Lock()
			p.stopped = true
			p.mu.Unlock()
			return
		}
		var r response
		switch h.Type {
		case "token_response":
			var v struct {
				Type      string       `json:"type"`
				ID        uint64       `json:"id"`
				Token     *accessToken `json:"token"`
				ErrorCode string       `json:"error_code"`
			}
			if strictJSON(b, &v) != nil {
				return
			}
			r = response{Type: v.Type, ID: v.ID, Token: v.Token, ErrorCode: v.ErrorCode}
		case "authorize_response":
			var v struct {
				Type    string `json:"type"`
				ID      uint64 `json:"id"`
				Allowed bool   `json:"allowed"`
			}
			if strictJSON(b, &v) != nil {
				return
			}
			r = response{Type: v.Type, ID: v.ID, Allowed: v.Allowed}
		default:
			return
		}
		p.mu.Lock()
		q, ok := p.pending[r.ID]
		p.mu.Unlock()
		if !ok {
			continue
		}
		if r.Type != q.kind+"_response" {
			return
		}
		select {
		case q.ch <- r:
		default:
			return
		}
	}
}

type runtimeService interface {
	Start(context.Context) error
	Wait() error
	Close() error
}
type runtimeFactory func(context.Context, bootstrap, *peer) (runtimeService, error)

func runtimeConfig(b bootstrap, p *peer) broadcaster.Config {
	client := &http.Client{Timeout: 30 * time.Second}
	worldType := broadcaster.WorldTypeSurvival
	switch b.GameType {
	case "creative":
		worldType = broadcaster.WorldTypeCreative
	case "adventure":
		worldType = broadcaster.WorldTypeAdventure
	}
	return broadcaster.Config{
		XBLTokenSource: broadcaster.NewXBLTokenSource(p.ctx, p),
		// Current Bedrock clients normally use Player Messaging (connection
		// type 7). The upstream zero value selects the legacy type-3 path.
		SignalingMode: broadcaster.SignalingModeJSONRPC,
		Server:        broadcaster.ServerInfo{Host: b.GatewayHost, Port: b.GatewayPort},
		Status:        broadcaster.Status{WorldName: b.WorldName, HostName: b.WorldName, WorldType: worldType, MaxPlayers: b.MaxPlayers, Broadcast: int32(broadcaster.BroadcastSettingFriendsOnly)},
		Relay:         nil, FriendSync: nil,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), HTTPClient: client,
		ListenConfig: minecraft.ListenConfig{AuthenticationDisabled: false, HTTPClient: client, Allow: func(_ net.Addr, id login.IdentityData, _ login.ClientData) (string, bool) {
			if !p.authorize(id.XUID) {
				return "Online access denied.", false
			}
			return "", true
		}},
	}
}
func actualFactory(_ context.Context, b bootstrap, p *peer) (runtimeService, error) {
	return broadcaster.New(runtimeConfig(b, p))
}

func runWorker(ctx context.Context, in io.Reader, out io.Writer, factory runtimeFactory) int {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), maxFrame+1)
	if !scanner.Scan() {
		emit(out, struct {
			Type string `json:"type"`
			Code string `json:"code"`
		}{"event", "error"})
		return 1
	}
	b, err := parseBootstrap(scanner.Bytes())
	if err != nil {
		emit(out, struct {
			Type string `json:"type"`
			Code string `json:"code"`
		}{"event", "error"})
		return 1
	}
	p := newPeer(ctx, out, b)
	defer p.close()
	go p.read(scanner)
	p.event("starting")
	// Require a valid access token from this parent before constructing upstream
	// resources. There is no device-code or refresh-token fallback in the child.
	if _, err := p.Token(); err != nil {
		p.event("error")
		return 1
	}
	service, err := factory(p.ctx, b, p)
	if err != nil {
		p.event("error")
		return 1
	}
	if err = service.Start(p.ctx); err != nil {
		_ = service.Close()
		p.event("error")
		return 1
	}
	if p.ctx.Err() != nil {
		_ = service.Close()
		p.event("stopped")
		return 0
	}
	p.event("advertising")
	done := make(chan error, 1)
	go func() { done <- service.Wait() }()
	select {
	case <-p.ctx.Done():
	case err = <-done:
	}
	p.cancel()
	closeErr := service.Close()
	if err != nil || closeErr != nil {
		p.event("error")
		return 1
	}
	p.event("stopped")
	return 0
}
