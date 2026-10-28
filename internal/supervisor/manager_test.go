package supervisor

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"nethernet-multi-server/internal/config"
)

type fakeProcess struct {
	mu     sync.Mutex
	writes string
	done   chan error
}

func (p *fakeProcess) PID() int { return 42 }
func (p *fakeProcess) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes += string(b)
	return len(b), nil
}
func (p *fakeProcess) Wait() error { return <-p.done }

type fakeFactory struct{ p *fakeProcess }

func (f fakeFactory) Start(context.Context, config.Process, io.Writer) (Process, error) {
	return f.p, nil
}

func configuredServer() config.Server {
	return config.Server{ID: "creative", Name: "Creative", Process: &config.Process{Executable: `C:\MinecraftServers\Creative\bedrock_server.exe`, WorkingDirectory: `C:\MinecraftServers\Creative`, WorldDirectory: `C:\MinecraftServers\Creative\worlds\Private Creative Server`, LogDirectory: `C:\MinecraftServers\Creative\logs`}}
}

func TestStartAndGracefulStop(t *testing.T) {
	p := &fakeProcess{done: make(chan error, 1)}
	m := New([]config.Server{configuredServer()}, fakeFactory{p: p}, Options{StopTimeout: time.Second})
	if err := m.Start(context.Background(), "creative"); err != nil {
		t.Fatal(err)
	}
	if got := m.Status("creative"); got.State != Running || got.PID != 42 {
		t.Fatalf("status=%+v", got)
	}
	done := make(chan error, 1)
	go func() { done <- m.Stop(context.Background(), "creative") }()
	time.Sleep(10 * time.Millisecond)
	p.mu.Lock()
	writes := p.writes
	p.mu.Unlock()
	if writes != "stop\r\n" {
		t.Fatalf("writes=%q", writes)
	}
	p.done <- nil
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := m.Status("creative"); got.State != Stopped {
		t.Fatalf("state=%s", got.State)
	}
}

func TestDuplicateStartRejected(t *testing.T) {
	p := &fakeProcess{done: make(chan error, 1)}
	m := New([]config.Server{configuredServer()}, fakeFactory{p: p}, Options{})
	if err := m.Start(context.Background(), "creative"); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background(), "creative"); err == nil {
		t.Fatal("duplicate start succeeded")
	}
	p.done <- nil
}

func TestStopTimeoutDoesNotKill(t *testing.T) {
	p := &fakeProcess{done: make(chan error)}
	m := New([]config.Server{configuredServer()}, fakeFactory{p: p}, Options{StopTimeout: 20 * time.Millisecond})
	if err := m.Start(context.Background(), "creative"); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(context.Background(), "creative"); err == nil {
		t.Fatal("Stop succeeded; want timeout")
	}
}

func TestRecentLogIsBoundedAndExternalStateCanBeRecorded(t *testing.T) {
	m := New([]config.Server{configuredServer()}, fakeFactory{}, Options{MaxLogBytes: 8})
	m.log("creative").Write([]byte("1234567890"))
	if got := m.RecentLog("creative"); got != "34567890" {
		t.Fatalf("RecentLog() = %q", got)
	}
	if err := m.MarkExternallyRunning("creative", 99); err != nil {
		t.Fatal(err)
	}
	if got := m.Status("creative"); got.State != ExternallyRunning || got.PID != 99 {
		t.Fatalf("status = %+v", got)
	}
}
func TestParseNetstatUDPFindsBackendOwner(t *testing.T) {
	text := `  UDP    0.0.0.0:7551           *:*                                    100
  UDP    192.168.1.246:19134   *:*                                    4242`
	if got := parseNetstatUDPPID(text, "192.168.1.246:19134"); got != 4242 {
		t.Fatalf("pid=%d", got)
	}
}
func TestExternalServerBecomesStoppedWhenListenerDisappears(t *testing.T) {
	m := New([]config.Server{configuredServer()}, fakeFactory{}, Options{})
	if err := m.observeExternal("creative", 99); err != nil {
		t.Fatal(err)
	}
	if err := m.observeExternal("creative", 0); err != nil {
		t.Fatal(err)
	}
	if got := m.Status("creative").State; got != Stopped {
		t.Fatalf("state=%s", got)
	}
}
func TestConfiguredProcessPathSelectsCorrectServer(t *testing.T) {
	processes := map[string]int{
		`c:\minecraftservers\creative\bedrock_server.exe`: 42,
		`c:\minecraftservers\survival\bedrock_server.exe`: 84,
	}
	if got := configuredProcessPID(processes, `C:\MinecraftServers\Creative\bedrock_server.exe`); got != 42 {
		t.Fatalf("creative pid=%d", got)
	}
	if got := configuredProcessPID(processes, `C:\MinecraftServers\Survival\bedrock_server.exe`); got != 84 {
		t.Fatalf("survival pid=%d", got)
	}
}

func TestFailedProcessCanBeStartedAgainAfterExit(t *testing.T) {
	p := &fakeProcess{done: make(chan error, 1)}
	m := New([]config.Server{configuredServer()}, fakeFactory{p: p}, Options{})
	if err := m.Start(context.Background(), "creative"); err != nil {
		t.Fatal(err)
	}
	p.done <- fmt.Errorf("crash")
	deadline := time.Now().Add(time.Second)
	for m.Status("creative").State != Failed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := m.Start(context.Background(), "creative"); err != nil {
		t.Fatalf("cannot recover failed server: %v", err)
	}
}

func TestStartupReadinessSurvivesLogTruncationAndResetsOnRestart(t *testing.T) {
	p := &fakeProcess{done: make(chan error, 1)}
	m := New([]config.Server{configuredServer()}, fakeFactory{p: p}, Options{MaxLogBytes: 40})
	if e := m.Start(context.Background(), "creative"); e != nil {
		t.Fatal(e)
	}
	m.log("creative").Write([]byte("Server started.\n"))
	m.log("creative").Write([]byte(strings.Repeat("later log data", 20)))
	if !m.Status("creative").Ready {
		t.Fatal("ready lost after log truncation")
	}
	p.done <- fmt.Errorf("crash")
	for i := 0; i < 100 && m.Status("creative").State != Failed; i++ {
		time.Sleep(time.Millisecond)
	}
	if e := m.Start(context.Background(), "creative"); e != nil {
		t.Fatal(e)
	}
	if m.Status("creative").Ready {
		t.Fatal("old readiness leaked into new process")
	}
}
