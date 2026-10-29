package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

const goodBootstrap = `{"type":"bootstrap","version":1,"server_id":"server-1","account_id":"account-1","gateway_host":"8.8.8.8","gateway_port":19140,"world_name":"World","expected_protocol":2225,"allow_unknown_friends_route":true}`

func TestBootstrapValidation(t *testing.T) {
	for _, s := range []string{`{}`, strings.Replace(goodBootstrap, `2225`, `2193`, 1), strings.Replace(goodBootstrap, `true`, `false`, 1), strings.Replace(goodBootstrap, `8.8.8.8`, `localhost`, 1), strings.Replace(goodBootstrap, `8.8.8.8`, `203.0.113.8`, 1), strings.Replace(goodBootstrap, `19140`, `0`, 1), strings.Replace(goodBootstrap, `"version":1`, `"version":1,"refresh_token":"SECRET"`, 1)} {
		if _, err := parseBootstrap([]byte(s)); err == nil {
			t.Fatal("unsafe bootstrap accepted")
		}
	}
	if _, err := parseBootstrap([]byte(goodBootstrap)); err != nil {
		t.Fatal(err)
	}
}

type fakeRuntime struct {
	start  func(context.Context) error
	closed chan struct{}
	once   sync.Once
}

func (f *fakeRuntime) Start(c context.Context) error { return f.start(c) }
func (f *fakeRuntime) Wait() error                   { <-f.closed; return nil }
func (f *fakeRuntime) Close() error                  { f.once.Do(func() { close(f.closed) }); return nil }

func TestLifecycleBrokeredTokenAndAdmission(t *testing.T) {
	pin, cin := io.Pipe()
	cout, pout := io.Pipe()
	done := make(chan int, 1)
	closed := make(chan struct{})
	factory := func(ctx context.Context, b bootstrap, p *peer) (runtimeService, error) {
		c := runtimeConfig(b, p)
		if c.Relay != nil || c.FriendSync != nil || c.ListenConfig.AuthenticationDisabled {
			t.Error("unsafe configuration")
		}
		return &fakeRuntime{closed: closed, start: func(ctx context.Context) error {
			tok, err := p.Token()
			if err != nil {
				return err
			}
			if tok.AccessToken != "SECRET_ACCESS" {
				t.Error("wrong token")
			}
			if _, ok := c.ListenConfig.Allow(nil, login.IdentityData{}, login.ClientData{}); ok {
				t.Error("missing XUID allowed")
			}
			if _, ok := c.ListenConfig.Allow(nil, login.IdentityData{XUID: "123"}, login.ClientData{}); ok {
				t.Error("unauthorized allowed")
			}
			return nil
		}}, nil
	}
	go func() { done <- runWorker(context.Background(), pin, pout, factory) }()
	enc := json.NewEncoder(cin)
	_, _ = io.WriteString(cin, goodBootstrap+"\n")
	dec := json.NewDecoder(cout)
	for {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(toJSON(m), "SECRET_ACCESS") {
			t.Fatal("token leaked")
		}
		switch m["type"] {
		case "token_request":
			_ = enc.Encode(map[string]any{"type": "token_response", "id": m["id"], "token": map[string]any{"access_token": "SECRET_ACCESS", "token_type": "Bearer", "expiry": time.Now().Add(time.Hour)}})
		case "authorize_request":
			_ = enc.Encode(map[string]any{"type": "authorize_response", "id": m["id"], "allowed": false})
		case "event":
			if !fixedEventCode(m["code"]) {
				t.Fatalf("unexpected event code: %v", m["code"])
			}
			if m["code"] == "advertising" {
				_ = enc.Encode(map[string]any{"type": "stop"})
				goto wait
			}
		}
	}
wait:
	// Drain final event so the pipe writer cannot block.
	go io.Copy(io.Discard, cout)
	select {
	case code := <-done:
		if code != 0 {
			t.Fatal(code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown hung")
	}
	_ = cin.Close()
	_ = cout.Close()
}

func fixedEventCode(v any) bool {
	switch v {
	case "starting", "advertising", "stopped", "authentication_required", "error":
		return true
	}
	return false
}

func TestApprovalDoesNotEmitUnlistedEvent(t *testing.T) {
	pin, cin := io.Pipe()
	cout, pout := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := newPeer(ctx, pout, bootstrap{ServerID: "s", AccountID: "a"})
	defer p.close()
	scanner := bufio.NewScanner(pin)
	go p.read(scanner)
	done := make(chan bool, 1)
	go func() {
		_, ok := runtimeConfig(p.binding, p).ListenConfig.Allow(nil, login.IdentityData{XUID: "123"}, login.ClientData{})
		done <- ok
	}()
	var req map[string]any
	if err := json.NewDecoder(cout).Decode(&req); err != nil {
		t.Fatal(err)
	}
	_ = json.NewEncoder(cin).Encode(map[string]any{"type": "authorize_response", "id": req["id"], "allowed": true})
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("approval denied")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("callback emitted unlisted event or blocked")
	}
	if len(p.writes) != 0 {
		t.Fatal("unexpected event queued")
	}
	_ = cin.Close()
	_ = cout.Close()
}

func TestRejectedBootstrapNeverCreatesRuntime(t *testing.T) {
	for _, s := range []string{"{\n", strings.Repeat("x", maxFrame+1) + "\n"} {
		var out bytes.Buffer
		code := runWorker(context.Background(), strings.NewReader(s), &out, func(context.Context, bootstrap, *peer) (runtimeService, error) {
			t.Fatal("factory called")
			return nil, nil
		})
		if code == 0 || strings.Contains(out.String(), s) {
			t.Fatal("unsafe input behavior")
		}
	}
}

func TestParentLostDenies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := newPeer(ctx, io.Discard, bootstrap{ServerID: "s", AccountID: "a"})
	p.timeout = time.Millisecond
	if _, err := p.Token(); err == nil {
		t.Fatal("token accepted after parent loss")
	}
	if p.authorize("123") {
		t.Fatal("allowed after parent loss")
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestConcurrentRequestsMatchIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parent, child := io.Pipe()
	childOut, parentOut := io.Pipe()
	p := newPeer(ctx, parentOut, bootstrap{ServerID: "s", AccountID: "a"})
	defer p.cancel()
	scanner := bufio.NewScanner(parent)
	scanner.Buffer(make([]byte, 4096), maxFrame+1)
	go p.read(scanner)
	got := make(chan bool, 2)
	go func() { got <- p.authorize("111") }()
	go func() { got <- p.authorize("222") }()
	d := json.NewDecoder(childOut)
	requests := make([]map[string]any, 2)
	for i := range requests {
		if err := d.Decode(&requests[i]); err != nil {
			t.Fatal(err)
		}
	}
	e := json.NewEncoder(child)
	for i := len(requests) - 1; i >= 0; i-- {
		r := requests[i]
		if r["server_id"] != "s" || r["account_id"] != "a" {
			t.Fatal("unbound request")
		}
		_ = e.Encode(map[string]any{"type": "authorize_response", "id": r["id"], "allowed": r["xuid"] == "111"})
	}
	a, b := <-got, <-got
	if a == b {
		t.Fatal("responses not independently matched")
	}
	_ = child.Close()
	_ = childOut.Close()
}

func TestRefreshFieldRejectedAndNoFallback(t *testing.T) {
	pin, cin := io.Pipe()
	cout, pout := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- runWorker(context.Background(), pin, pout, func(context.Context, bootstrap, *peer) (runtimeService, error) {
			t.Error("created runtime without valid broker token")
			return nil, errBroker
		})
	}()
	_, _ = io.WriteString(cin, goodBootstrap+"\n")
	d := json.NewDecoder(cout)
	for {
		var m map[string]any
		if err := d.Decode(&m); err != nil {
			t.Fatal(err)
		}
		if m["type"] == "token_request" {
			_ = json.NewEncoder(cin).Encode(map[string]any{"type": "token_response", "id": m["id"], "token": map[string]any{"access_token": "SECRET_ACCESS", "refresh_token": "SECRET_REFRESH", "token_type": "Bearer", "expiry": time.Now().Add(time.Hour)}})
			break
		}
	}
	var captured bytes.Buffer
	copied := make(chan struct{})
	go func() { _, _ = io.Copy(&captured, cout); close(copied) }()
	select {
	case code := <-done:
		if code == 0 {
			t.Fatal("accepted refresh token")
		}
	case <-time.After(time.Second):
		t.Fatal("no cancellation")
	}
	_ = pout.Close()
	_ = cin.Close()
	// The worker emits fixed events; never echo rejected input or OAuth fields.
	// Read after Copy completes to avoid racing its buffer.
	<-copied
	if strings.Contains(captured.String(), "SECRET_") || strings.Contains(captured.String(), "refresh_token") {
		t.Fatal("secret leaked")
	}
}

func TestBoundedRequestTimeoutAndParentEOF(t *testing.T) {
	p := newPeer(context.Background(), io.Discard, bootstrap{ServerID: "s", AccountID: "a"})
	defer p.cancel()
	p.timeout = 5 * time.Millisecond
	if p.authorize("123") {
		t.Fatal("timed-out parent authorized")
	}
	p.mu.Lock()
	n := len(p.pending)
	p.mu.Unlock()
	if n != 0 {
		t.Fatal("pending request leaked")
	}
	s := bufio.NewScanner(strings.NewReader(""))
	p.read(s)
	if p.authorize("123") {
		t.Fatal("EOF authorized")
	}
}

func TestUnresponsiveParentWriterIsBounded(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	p := newPeer(context.Background(), w, bootstrap{ServerID: "s", AccountID: "a"})
	defer p.close()
	p.timeout = 5 * time.Millisecond
	done := make(chan bool, 1)
	go func() { done <- p.authorize("123") }()
	select {
	case allowed := <-done:
		if allowed {
			t.Fatal("unread parent authorized")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked pipe defeated RPC deadline")
	}
}
