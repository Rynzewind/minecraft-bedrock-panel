package sessionhistory

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsConnectedAndDisconnectedEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	now := time.Date(2026, 9, 22, 17, 30, 0, 125000000, time.Local)
	store, err := Open(path)
	if err != nil { t.Fatal(err) }
	store.now = func() time.Time { return now }
	id := store.Connected(Event{ConsoleGamertag: "Kev", AccountGamertag: "ProxyKev", Server: "Private Creative Server"})
	store.now = func() time.Time { return now.Add(90 * time.Second) }
	store.Disconnected(id, "connection lost")

	reopened, err := Open(path)
	if err != nil { t.Fatal(err) }
	events := reopened.List(10)
	if len(events) != 2 { t.Fatalf("events=%d, want 2", len(events)) }
	if events[0].Type != Disconnected || events[0].DurationMS != 90000 || events[0].Reason != "connection lost" {
		t.Fatalf("disconnect event=%+v", events[0])
	}
	if events[1].Type != Connected || events[1].ConsoleGamertag != "Kev" { t.Fatalf("connect event=%+v", events[1]) }
}

func TestListLimitsNewestEvents(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "sessions.jsonl"))
	if err != nil { t.Fatal(err) }
	store.Connected(Event{ConsoleGamertag: "first"})
	store.Connected(Event{ConsoleGamertag: "second"})
	got := store.List(1)
	if len(got) != 1 || got[0].ConsoleGamertag != "second" { t.Fatalf("got=%+v", got) }
}
