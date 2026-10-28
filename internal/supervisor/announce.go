package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode"
)

// Announce sends plain text through the owned Bedrock console, without changing
// cheat/world settings or exposing an arbitrary console-command API.
func (m *Manager) Announce(ctx context.Context, id, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(text) == 0 || len(text) > 1024 {
		return errors.New("invalid announcement length")
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return errors.New("announcement contains control characters")
		}
	}
	payload, _ := json.Marshal(map[string]any{"rawtext": []map[string]string{{"text": text}}})
	m.mu.Lock()
	s := m.servers[id]
	if s == nil || s.status.State != Running || s.process == nil {
		m.mu.Unlock()
		return errors.New("announcement requires an owned running server")
	}
	if s.announcementPending {
		m.mu.Unlock()
		return errors.New("server console has a pending announcement write")
	}
	p := s.process
	s.announcementPending = true
	m.mu.Unlock()
	command := append(append([]byte("tellraw @a "), payload...), '\r', '\n')
	done := make(chan error, 1)
	go func() {
		n, err := p.Write(command)
		if err == nil && n != len(command) {
			err = io.ErrShortWrite
		}
		m.mu.Lock()
		if s.process == p {
			s.announcementPending = false
		}
		m.mu.Unlock()
		done <- err
	}()
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	select {
	case err := <-done:
		return err
	case <-bounded.Done():
		return bounded.Err()
	}
}
