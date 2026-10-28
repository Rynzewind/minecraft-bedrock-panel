package addonupdates

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	mu   sync.Mutex
	path string
}

func New(path string) *Store { return &Store{path: path} }
func (s *Store) load() (map[string]string, error) {
	out := map[string]string{}
	b, e := os.ReadFile(s.path)
	if os.IsNotExist(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(b, &out)
	return out, e
}
func (s *Store) List() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, _ := s.load()
	return v
}
func (s *Store) Set(id, raw string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		return fmt.Errorf("select an installed pack")
	}
	if raw != "" {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return fmt.Errorf("use an HTTPS project page without embedded credentials")
		}
	}
	v, e := s.load()
	if e != nil {
		return e
	}
	if raw == "" {
		delete(v, id)
	} else {
		v[id] = raw
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(s.path), 0700); e != nil {
		return e
	}
	if e = os.WriteFile(s.path+".tmp", b, 0600); e != nil {
		return e
	}
	return os.Rename(s.path+".tmp", s.path)
}
