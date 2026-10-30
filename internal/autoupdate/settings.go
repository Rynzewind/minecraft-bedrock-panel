package autoupdate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Settings struct {
	Servers   bool      `json:"servers"`
	Addons    bool      `json:"addons"`
	LastCheck time.Time `json:"last_check,omitempty"`
	NextCheck time.Time `json:"next_check,omitempty"`
	Message   string    `json:"message"`
}
type Store struct {
	mu    sync.Mutex
	path  string
	value Settings
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, value: Settings{Message: "Automatic updates are off."}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &s.value); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) Get() Settings { s.mu.Lock(); defer s.mu.Unlock(); return s.value }
func (s *Store) save(v Settings) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(s.path+".tmp", b, 0600); err != nil {
		return err
	}
	if err = os.Rename(s.path+".tmp", s.path); err != nil {
		return err
	}
	s.value = v
	return nil
}
func (s *Store) Set(servers, addons *bool) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.value
	if servers != nil {
		v.Servers = *servers
	}
	if addons != nil {
		v.Addons = *addons
	}
	v.NextCheck = time.Time{}
	v.Message = "Settings saved. Checks run every 15 minutes after both worlds have been empty for five minutes."
	if !v.Servers && !v.Addons {
		v.Message = "Automatic updates are off."
	}
	err := s.save(v)
	return s.value, err
}
func (s *Store) Record(message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.value
	v.LastCheck = time.Now()
	v.NextCheck = v.LastCheck.Add(15 * time.Minute)
	v.Message = message
	return s.save(v)
}

// Due keeps installation quiet until the entire managed fleet has stayed empty.
func Due(now, idleSince time.Time, s Settings) bool {
	return (s.Servers || s.Addons) && !idleSince.IsZero() && now.Sub(idleSince) >= 5*time.Minute && (s.NextCheck.IsZero() || !now.Before(s.NextCheck))
}
