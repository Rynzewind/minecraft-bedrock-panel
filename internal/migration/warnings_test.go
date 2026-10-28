package migration

import (
	"context"
	"errors"
	"nethernet-multi-server/internal/supervisor"
	"strings"
	"testing"
	"time"
)

type announcingSupervisor struct {
	fakeSupervisor
	messages          []string
	announcementError error
}

func (s *announcingSupervisor) Announce(_ context.Context, id, text string) error {
	s.messages = append(s.messages, id+":"+text)
	return s.announcementError
}
func TestScheduledCountdownWarnsRegardlessOfPlayers(t *testing.T) {
	s := &announcingSupervisor{fakeSupervisor: fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running, Players: 2}, {ID: "survival", State: supervisor.Running}}}, announcementError: errors.New("console unavailable")}
	m := NewManager(fixture(t), s, emptyExport)
	waits := 0
	m.warningWait = func(ctx context.Context, d time.Duration) error {
		if d != time.Minute {
			t.Fatal(d)
		}
		waits++
		return nil
	}
	if err := m.warnScheduled(context.Background()); err != nil {
		t.Fatal(err)
	}
	if waits != 5 || len(s.messages) != 12 {
		t.Fatalf("waits=%d messages=%v", waits, s.messages)
	}
	for i := 0; i < 5; i++ {
		if !strings.Contains(s.messages[i*2], string(rune('5'-i))+" minute") {
			t.Fatal(s.messages)
		}
	}
	if s.Statuses()[0].State != supervisor.Running {
		t.Fatal("countdown stopped a server")
	}
	if m.job.Warning == "" {
		t.Fatal("announcement failure not reported")
	}
}
func TestCountdownCancellationDoesNotStopServers(t *testing.T) {
	s := &announcingSupervisor{fakeSupervisor: fakeSupervisor{states: []supervisor.Status{{ID: "creative", State: supervisor.Running}}}}
	m := NewManager(fixture(t), s, emptyExport)
	m.warningWait = func(context.Context, time.Duration) error { return context.Canceled }
	if !errors.Is(m.warnScheduled(context.Background()), context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	if len(s.messages) != 2 || !strings.Contains(s.messages[1], "cancelled") || s.Statuses()[0].State != supervisor.Running {
		t.Fatal(s.messages)
	}
}
