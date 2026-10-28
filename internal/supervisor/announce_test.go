package supervisor

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/config"
	"strings"
	"testing"
	"time"
)

func TestAnnounceUsesLiteralTellrawAndOwnedRunningProcess(t *testing.T) {
	p := &fakeProcess{done: make(chan error, 1)}
	m := New([]config.Server{configuredServer()}, fakeFactory{p: p}, Options{})
	if err := m.Announce(context.Background(), "creative", "hello"); err == nil {
		t.Fatal("announced to stopped server")
	}
	if err := m.Start(context.Background(), "creative"); err != nil {
		t.Fatal(err)
	}
	defer func() { p.done <- nil }()
	text := `Backup "now"; stop`
	if err := m.Announce(context.Background(), "creative", text); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	got := p.writes
	p.mu.Unlock()
	var payload struct {
		Rawtext []struct {
			Text string `json:"text"`
		} `json:"rawtext"`
	}
	if !strings.HasPrefix(got, "tellraw @a ") || json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(got, "tellraw @a "), "\r\n")), &payload) != nil || len(payload.Rawtext) != 1 || payload.Rawtext[0].Text != text {
		t.Fatalf("unsafe command: %q", got)
	}
	if err := m.Announce(context.Background(), "creative", "hi\nstop"); err == nil {
		t.Fatal("newline accepted")
	}
}

type blockedAnnouncementProcess struct{ release chan struct{} }

func (p *blockedAnnouncementProcess) PID() int                    { return 1 }
func (p *blockedAnnouncementProcess) Wait() error                 { return nil }
func (p *blockedAnnouncementProcess) Write(b []byte) (int, error) { <-p.release; return len(b), nil }
func TestBlockedAnnouncementCanCancelWithoutBlockingStatus(t *testing.T) {
	p := &blockedAnnouncementProcess{release: make(chan struct{})}
	defer close(p.release)
	m := New([]config.Server{configuredServer()}, fakeFactory{}, Options{})
	m.servers["creative"].process = p
	m.servers["creative"].status.State = Running
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Announce(ctx, "creative", "backup") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked write succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("announcement ignored cancellation")
	}
	status := make(chan Status, 1)
	go func() { status <- m.Status("creative") }()
	select {
	case <-status:
	case <-time.After(time.Second):
		t.Fatal("status blocked by console write")
	}
	if err := m.Announce(context.Background(), "creative", "again"); err == nil {
		t.Fatal("queued another blocked writer")
	}
	if err := m.Stop(context.Background(), "creative"); err == nil {
		t.Fatal("stop queued behind blocked console")
	}
}
