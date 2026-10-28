package sessionhistory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type EventType string

const (
	Connected    EventType = "connected"
	Disconnected EventType = "disconnected"
)

type Event struct {
	ID              string    `json:"id"`
	Time            time.Time `json:"time"`
	Type            EventType `json:"type"`
	ConsoleGamertag string    `json:"console_gamertag"`
	ConsoleXUID     string    `json:"console_xuid,omitempty"`
	AccountLabel    string    `json:"account_label,omitempty"`
	AccountGamertag string    `json:"account_gamertag,omitempty"`
	Server          string    `json:"server"`
	DurationMS      int64     `json:"duration_ms,omitempty"`
	Reason          string    `json:"reason,omitempty"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	events []Event
	active map[string]Event
	now    func() time.Time
	next   atomic.Uint64
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, active: map[string]Event{}, now: time.Now}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return nil, err
			}
			return s, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode session history: %w", err)
		}
		s.events = append(s.events, event)
		if event.Type == Connected {
			s.active[event.ID] = event
		} else if event.Type == Disconnected {
			delete(s.active, event.ID)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Connected(event Event) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	event.ID = fmt.Sprintf("%d-%d", now.UnixNano(), s.next.Add(1))
	event.Time = now
	event.Type = Connected
	s.active[event.ID] = event
	s.append(event)
	return event.ID
}

func (s *Store) Disconnected(id, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	started, ok := s.active[id]
	if !ok {
		return
	}
	now := s.now()
	event := started
	event.Time = now
	event.Type = Disconnected
	event.DurationMS = now.Sub(started.Time).Milliseconds()
	event.Reason = reason
	delete(s.active, id)
	s.append(event)
}

func (s *Store) append(event Event) {
	s.events = append(s.events, event)
	b, err := json.Marshal(event)
	if err != nil {
		return
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

func (s *Store) List(limit int) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	out := make([]Event, limit)
	for i := 0; i < limit; i++ {
		out[i] = s.events[len(s.events)-1-i]
	}
	return out
}
