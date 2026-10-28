package supervisor

import (
	"context"
	"io"
	"nethernet-multi-server/internal/config"
	"testing"
)

type progressFactory struct {
	check func()
	p     *fakeProcess
}

func (f progressFactory) Start(context.Context, config.Process, io.Writer) (Process, error) {
	f.check()
	return f.p, nil
}
func TestStartupProgressDistinguishesAddonWorkAndProcessLaunch(t *testing.T) {
	p := &fakeProcess{done: make(chan error, 1)}
	var m *Manager
	m = New([]config.Server{configuredServer()}, progressFactory{p: p, check: func() {
		if got := m.Status("creative").Phase; got != "launching" {
			t.Fatalf("launch phase=%q", got)
		}
	}}, Options{ApplyPending: func(context.Context, string) error {
		if got := m.Status("creative").Phase; got != "applying_changes" {
			t.Fatalf("apply phase=%q", got)
		}
		return nil
	}})
	if err := m.Start(context.Background(), "creative"); err != nil {
		t.Fatal(err)
	}
	if got := m.Status("creative"); got.Phase != "running" {
		t.Fatalf("status=%+v", got)
	}
	p.done <- nil
}
