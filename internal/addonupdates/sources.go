package addonupdates

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Source struct {
	URL         string    `json:"url"`
	Provider    string    `json:"provider"`
	ProjectID   int64     `json:"project_id,omitempty"`
	Channel     string    `json:"channel"`
	LastChecked time.Time `json:"last_checked,omitempty"`
	CheckState  string    `json:"check_state,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	LastFileID  int64     `json:"last_file_id,omitempty"`
}
type sourceDisk struct {
	Version int               `json:"version"`
	Sources map[string]Source `json:"sources"`
}
type Store struct {
	mu   sync.Mutex
	path string
}

func New(path string) *Store { return &Store{path: path} }
func (s *Store) load() (map[string]Source, error) {
	out := map[string]Source{}
	b, e := os.ReadFile(s.path)
	if errors.Is(e, os.ErrNotExist) {
		return out, nil
	}
	if e != nil {
		return nil, fmt.Errorf("cannot read add-on sources")
	}
	var header map[string]json.RawMessage
	if json.Unmarshal(b, &header) != nil {
		return nil, fmt.Errorf("add-on source settings are unreadable; restore or repair the saved file")
	}
	if _, ok := header["version"]; ok {
		var disk sourceDisk
		if json.Unmarshal(b, &disk) != nil || disk.Version != 2 || disk.Sources == nil {
			return nil, fmt.Errorf("unsupported or invalid add-on source settings")
		}
		return disk.Sources, nil
	}
	var legacy map[string]string
	if json.Unmarshal(b, &legacy) != nil {
		return nil, fmt.Errorf("invalid legacy add-on sources")
	}
	for id, u := range legacy {
		r := Source{URL: u, Channel: "stable", Provider: "manual"}
		if isCurseForgeURL(u) {
			r.Provider = "curseforge"
		}
		out[id] = r
	}
	return out, nil
}
func (s *Store) ListRecords() (map[string]Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}
func (s *Store) Links() (map[string]string, error) {
	rows, e := s.ListRecords()
	if e != nil {
		return nil, e
	}
	out := map[string]string{}
	for id, r := range rows {
		if r.URL != "" {
			out[id] = r.URL
		}
	}
	return out, nil
}

// List is retained for compatibility with the first source-link UI.
func (s *Store) List() map[string]string { v, _ := s.Links(); return v }
func normalizeSource(r Source) (Source, error) {
	r.URL = strings.TrimSpace(r.URL)
	if len(r.Notes) > 4096 || r.ProjectID < 0 {
		return r, fmt.Errorf("invalid source metadata")
	}
	if r.Channel == "" {
		r.Channel = "stable"
	}
	if r.Channel != "stable" && r.Channel != "beta" && r.Channel != "alpha" {
		return r, fmt.Errorf("select stable, beta or alpha releases")
	}
	if r.URL != "" {
		u, e := url.Parse(r.URL)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return r, fmt.Errorf("use an HTTPS project page without embedded credentials")
		}
		for key := range u.Query() {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "key") || strings.Contains(lower, "password") {
				return r, fmt.Errorf("do not put credentials in a project URL")
			}
		}
	}
	if r.Provider == "" {
		r.Provider = "manual"
		if isCurseForgeURL(r.URL) {
			r.Provider = "curseforge"
		}
	}
	if r.Provider != "manual" && r.Provider != "curseforge" {
		return r, fmt.Errorf("unsupported add-on provider")
	}
	if r.Provider == "curseforge" && r.URL != "" && !isCurseForgeURL(r.URL) {
		return r, fmt.Errorf("link a Minecraft Bedrock CurseForge project page")
	}
	if r.Provider == "manual" && r.ProjectID != 0 {
		return r, fmt.Errorf("project IDs apply only to CurseForge sources")
	}
	return r, nil
}
func isCurseForgeURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && (u.Host == "www.curseforge.com" || u.Host == "curseforge.com") && strings.HasPrefix(u.Path, "/minecraft-bedrock/")
}
func (s *Store) SaveRecord(id string, r Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		return fmt.Errorf("select an installed pack")
	}
	r, e := normalizeSource(r)
	if e != nil {
		return e
	}
	rows, e := s.load()
	if e != nil {
		return e
	}
	rows[id] = r
	b, e := json.MarshalIndent(sourceDisk{Version: 2, Sources: rows}, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(s.path, b)
}
func (s *Store) Set(id, raw string) error {
	rows, e := s.ListRecords()
	if e != nil {
		return e
	}
	r := rows[id]
	if r.URL != raw {
		r.ProjectID = 0
	}
	r.URL = raw
	r.Provider = ""
	return s.SaveRecord(id, r)
}
func (s *Store) MarkChecked(id, state, notes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.load()
	if e != nil {
		return e
	}
	r := rows[id]
	r.LastChecked = time.Now().UTC()
	r.CheckState = state
	r.Notes = notes
	r, e = normalizeSource(r)
	if e != nil {
		return e
	}
	rows[id] = r
	b, e := json.MarshalIndent(sourceDisk{Version: 2, Sources: rows}, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(s.path, b)
}
func atomicWrite(path string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(path+".tmp", path)
	}
	return e
}

// MarkResult records a check only if the reviewed source settings still match.
func (s *Store) MarkResult(id string, expected Source, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.load()
	if e != nil {
		return e
	}
	r := rows[id]
	if r.URL != expected.URL || r.Provider != expected.Provider || r.ProjectID != expected.ProjectID || r.Channel != expected.Channel {
		return fmt.Errorf("source changed while checking; check again")
	}
	r.LastChecked = time.Now().UTC()
	r.CheckState = state
	rows[id] = r
	b, e := json.MarshalIndent(sourceDisk{Version: 2, Sources: rows}, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(s.path, b)
}

// ProtectStaging creates a user-private local directory for untrusted archives.
func ProtectStaging(path string) error { return privateDirectory(path) }

func (s *Store) Acknowledge(id string, expected Source, projectID, fileID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.load()
	if e != nil {
		return e
	}
	r := rows[id]
	if r.URL != expected.URL || r.Provider != expected.Provider || r.ProjectID != expected.ProjectID || r.Channel != expected.Channel {
		return fmt.Errorf("source changed; staged archive provenance was not saved")
	}
	r.ProjectID = projectID
	r.LastFileID = fileID
	rows[id] = r
	b, e := json.MarshalIndent(sourceDisk{Version: 2, Sources: rows}, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(s.path, b)
}
