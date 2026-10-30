package serverupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Supervisor interface {
	Statuses() []supervisor.Status
	Start(context.Context, string) error
	Stop(context.Context, string) error
	RecentLog(string) string
}
type Status struct {
	Phase    string   `json:"phase"`
	Message  string   `json:"message"`
	Done     bool     `json:"done"`
	Recovery bool     `json:"recovery"`
	Backup   string   `json:"backup,omitempty"`
	Version  string   `json:"version,omitempty"`
	SHA256   string   `json:"sha256,omitempty"`
	Opened   bool     `json:"opened"`
	Running  []string `json:"running,omitempty"`
}
type Installed struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}
type Check struct {
	Release   Release     `json:"release"`
	Installed []Installed `json:"installed"`
	Needed    bool        `json:"needed"`
}
type Manager struct {
	mu         sync.Mutex
	root, path string
	s          Supervisor
	busy       bool
	status     Status
}

func New(root, path string, s Supervisor) *Manager {
	m := &Manager{root: root, path: path, s: s, status: Status{Phase: "idle", Done: true, Message: "Check for official stable Minecraft server updates."}}
	b, e := os.ReadFile(m.journal())
	if e == nil {
		if json.Unmarshal(b, &m.status) != nil {
			m.status = Status{Phase: "recovery", Done: true, Recovery: true, Message: "Update journal is unreadable. Review backups before starting servers."}
		} else if !m.status.Done {
			m.status.Done = true
			m.status.Recovery = m.status.Phase == "installing" || m.status.Phase == "starting" || m.status.Phase == "recovering"
			m.status.Phase = "interrupted"
			m.status.Message = "Update interrupted before software replacement. Review server states and retry."
			if m.status.Recovery {
				m.status.Message = "Update interrupted. Servers must remain stopped until recovery is reviewed."
			}
			m.save()
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		m.status = Status{Phase: "recovery", Done: true, Recovery: true, Message: e.Error()}
	}
	return m
}
func (m *Manager) journal() string { return filepath.Join(m.root, "MultiServer", "server-update.json") }
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.status
	v.Running = append([]string(nil), v.Running...)
	return v
}
func (m *Manager) Busy() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.busy }
func (m *Manager) save() error {
	b, e := json.MarshalIndent(m.status, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(m.journal(), b)
}
func atomicWrite(path string, b []byte) error {
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
func (m *Manager) phase(p, msg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.Phase = p
	m.status.Message = msg
	return m.save()
}
func (m *Manager) Check(ctx context.Context) (Check, error) {
	r, e := latest(ctx)
	if e != nil {
		return Check{}, e
	}
	cfg, e := m.load()
	if e != nil {
		return Check{}, e
	}
	out := Check{Release: r}
	for _, s := range cfg.Servers {
		v := s.Version
		if live := logVersion(m.s.RecentLog(s.ID)); compareVersion(live, v) > 0 {
			v = live
		}
		out.Installed = append(out.Installed, Installed{s.ID, v})
		if compareVersion(v, r.Version) > 0 {
			out.Release.Compatible = false
			out.Release.Reason = "Installed server is newer than the available release. Downgrades are blocked to protect worlds."
		}
		if compareVersion(v, r.Version) < 0 {
			out.Needed = true
		}
	}
	return out, nil
}
func (m *Manager) Start(ctx context.Context, version string) (Status, error) {
	m.mu.Lock()
	if m.busy || m.status.Recovery {
		m.mu.Unlock()
		return Status{}, fmt.Errorf("another update or recovery is pending")
	}
	m.busy = true
	m.mu.Unlock()
	r, e := latest(ctx)
	if e == nil && r.Version != version {
		e = fmt.Errorf("available release changed; check and preview again")
	}
	if e == nil && !r.Compatible {
		e = errors.New(r.Reason)
	}
	if e != nil {
		m.mu.Lock()
		m.busy = false
		m.mu.Unlock()
		return Status{}, e
	}
	m.mu.Lock()
	m.status = Status{Phase: "downloading", Version: r.Version, Message: "Downloading official stable server software."}
	e = m.save()
	m.mu.Unlock()
	if e != nil {
		m.mu.Lock()
		m.busy = false
		m.mu.Unlock()
		return Status{}, e
	}
	go func() {
		jobCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		data, err := get(jobCtx, officialClient(), r.URL, 512<<20)
		if err == nil {
			err = m.apply(jobCtx, r, data)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		m.busy = false
		m.status.Done = true
		if err != nil {
			m.status.Message = err.Error()
			m.status.Phase = "failed"
		} else {
			m.status.Phase = "complete"
			m.status.Message = "Server update complete. Previously running servers passed startup checks."
		}
		if se := m.save(); se != nil {
			m.status.Recovery = true
			m.status.Message += "; cannot save completion journal: " + se.Error()
		}
	}()
	return m.Status(), nil
}
func (m *Manager) load() (config.Config, error) {
	b, e := os.ReadFile(m.path)
	if e != nil {
		return config.Config{}, e
	}
	var cfg config.Config
	e = json.Unmarshal(b, &cfg)
	if e != nil {
		return cfg, e
	}
	if len(cfg.Servers) == 0 {
		return cfg, fmt.Errorf("no servers configured")
	}
	for _, s := range cfg.Servers {
		if !safeID(s.ID) || s.Process == nil || !within(m.root, s.Process.WorkingDirectory) || filepath.Clean(s.Process.WorkingDirectory) == filepath.Clean(m.root) || !strings.EqualFold(filepath.Base(s.Process.Executable), "bedrock_server.exe") {
			return cfg, fmt.Errorf("unsupported server location: %s", s.ID)
		}
		if e = noLinks(s.Process.WorkingDirectory); e != nil {
			return cfg, e
		}
	}
	return cfg, nil
}
func within(root, path string) bool {
	r, e := filepath.Rel(root, path)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(os.PathSeparator)) && !filepath.IsAbs(r)
}
func noLinks(path string) error {
	p, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	for {
		f, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("linked paths are not allowed: %s", p)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return nil
		}
		p = parent
	}
}
func copyVerified(src, dst string) error {
	if e := noLinks(src); e != nil {
		return e
	}
	b, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(dst), 0700); e != nil {
		return e
	}
	if e = atomicWrite(dst, b); e != nil {
		return e
	}
	out, e := os.ReadFile(dst)
	if e != nil {
		return e
	}
	if sha256.Sum256(b) != sha256.Sum256(out) {
		return fmt.Errorf("backup verification failed: %s", src)
	}
	return nil
}
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("linked path: %s", p)
		}
		r, e := filepath.Rel(src, p)
		if e != nil {
			return e
		}
		out := filepath.Join(dst, r)
		if d.IsDir() {
			return os.MkdirAll(out, 0700)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported file: %s", p)
		}
		return copyVerified(p, out)
	})
}
func (m *Manager) validateStoppedOrRunning() ([]string, error) {
	var ids []string
	for _, s := range m.s.Statuses() {
		if s.Players > 0 {
			return nil, fmt.Errorf("%s has connected players; update when everyone has left", s.ID)
		}
		switch s.State {
		case supervisor.Running:
			ids = append(ids, s.ID)
		case supervisor.Stopped, supervisor.Failed:
		default:
			return nil, fmt.Errorf("%s is not safe for maintenance: %s", s.ID, s.State)
		}
	}
	return ids, nil
}
func (m *Manager) apply(ctx context.Context, r Release, data []byte) (err error) {
	if !r.Compatible {
		return errors.New(r.Reason)
	}
	running, e := m.validateStoppedOrRunning()
	if e != nil {
		return e
	}
	cfg, e := m.load()
	if e != nil {
		return e
	}
	for _, srv := range cfg.Servers {
		if compareVersion(srv.Version, r.Version) > 0 || compareVersion(logVersion(m.s.RecentLog(srv.ID)), r.Version) > 0 {
			return fmt.Errorf("server %s is newer than target; downgrade blocked", srv.ID)
		}
	}
	parent := filepath.Join(m.root, "MultiServer", "backups", "server-updates")
	if e = os.MkdirAll(parent, 0700); e != nil {
		return e
	}
	if e = noLinks(parent); e != nil {
		return e
	}
	backup, e := os.MkdirTemp(parent, time.Now().Format("20060102-150405")+"-")
	if e != nil {
		return e
	}
	if e = protect(backup); e != nil {
		return e
	}
	stage := filepath.Join(backup, "new-software")
	if e = extractArchive(data, stage); e != nil {
		return e
	}
	hash := sha256.Sum256(data)
	m.mu.Lock()
	m.status.Backup = backup
	m.status.Version = r.Version
	m.status.SHA256 = hex.EncodeToString(hash[:])
	m.status.Running = running
	m.mu.Unlock()
	if e = m.phase("stopping", "Stopping servers cleanly before verified backups."); e != nil {
		return e
	}
	running, e = m.validateStoppedOrRunning()
	if e != nil {
		return e
	}
	var stopped []string
	modified := false
	defer func() {
		if err == nil {
			return
		}
		if modified {
			m.mu.Lock()
			m.status.Recovery = true
			m.mu.Unlock()
			for _, s := range m.s.Statuses() {
				if s.State == supervisor.Running {
					stopCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
					_ = m.s.Stop(stopCtx, s.ID)
					cancel()
				}
			}
		} else {
			recovery, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			for _, id := range stopped {
				for {
					state := supervisor.Unconfigured
					for _, status := range m.s.Statuses() {
						if status.ID == id {
							state = status.State
						}
					}
					if state == supervisor.Stopping {
						select {
						case <-recovery.Done():
							err = errors.Join(err, fmt.Errorf("%s did not finish stopping", id))
						case <-time.After(25 * time.Millisecond):
							continue
						}
						break
					}
					if state == supervisor.Stopped || state == supervisor.Failed {
						if startErr := m.s.Start(recovery, id); startErr != nil {
							err = errors.Join(err, startErr)
						}
					}
					break
				}
			}
		}
	}()
	for _, id := range running {
		stopped = append(stopped, id)
		stop := m.s.Stop
		if empty, ok := m.s.(interface {
			StopIfEmpty(context.Context, string) error
		}); ok {
			stop = empty.StopIfEmpty
		}
		if e = stop(ctx, id); e != nil {
			return e
		}
	}
	for _, s := range m.s.Statuses() {
		if s.State != supervisor.Stopped && s.State != supervisor.Failed {
			return fmt.Errorf("%s did not stop", s.ID)
		}
	}
	if e = m.phase("backup", "Copying and verifying worlds, add-ons and configuration."); e != nil {
		return e
	}
	for _, s := range cfg.Servers {
		if e = copyTree(s.Process.WorkingDirectory, filepath.Join(backup, "servers", s.ID)); e != nil {
			return e
		}
	}
	if e = copyVerified(m.path, filepath.Join(backup, "config.json")); e != nil {
		return e
	}
	for _, name := range []string{"account-store", "identities", "content-staging", "server-settings", "player-permissions"} {
		src := filepath.Join(m.root, "MultiServer", name)
		if _, e = os.Stat(src); e == nil {
			if e = copyTree(src, filepath.Join(backup, "panel-data", name)); e != nil {
				return e
			}
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	if e = sealBackup(backup); e != nil {
		return e
	}
	if e = m.phase("installing", "Installing server software; existing worlds and settings are retained."); e != nil {
		return e
	}
	modified = true
	for _, s := range cfg.Servers {
		if e = installSoftware(stage, s.Process.WorkingDirectory); e != nil {
			return e
		}
	}
	b, e := os.ReadFile(m.path)
	if e != nil {
		return e
	}
	var raw map[string]json.RawMessage
	if e = json.Unmarshal(b, &raw); e != nil {
		return e
	}
	var servers []map[string]json.RawMessage
	if e = json.Unmarshal(raw["servers"], &servers); e != nil {
		return e
	}
	for _, s := range servers {
		s["version"], _ = json.Marshal(r.Version)
		s["protocol"], _ = json.Marshal(r.Protocol)
	}
	raw["servers"], _ = json.Marshal(servers)
	b, e = json.MarshalIndent(raw, "", "  ")
	if e != nil {
		return e
	}
	if e = atomicWrite(m.path, b); e != nil {
		return e
	}
	m.mu.Lock()
	m.status.Opened = len(running) > 0
	m.mu.Unlock()
	if e = m.phase("starting", "Starting previously running servers and checking their version."); e != nil {
		return e
	}
	for _, id := range running {
		if e = m.s.Start(ctx, id); e != nil {
			return e
		}
	}
	deadline := time.NewTimer(120 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		ready := true
		for _, id := range running {
			found := false
			for _, s := range m.s.Statuses() {
				if s.ID == id {
					found = true
					if s.State == supervisor.Failed {
						return fmt.Errorf("%s failed startup: %s", id, s.LastError)
					}
					if !s.Ready || logVersion(m.s.RecentLog(id)) != r.Version {
						ready = false
					}
				}
			}
			if !found {
				ready = false
			}
		}
		if ready {
			for _, srv := range cfg.Servers {
				for _, id := range running {
					if srv.ID == id && srv.Backend != "" {
						conn, e := net.DialTimeout("tcp4", srv.Backend, 2*time.Second)
						if e != nil {
							ready = false
						} else {
							conn.Close()
						}
					}
				}
			}
			if ready {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("startup checks timed out; review update recovery")
		case <-ticker.C:
		}
	}
}
func installSoftware(stage, dst string) error {
	preserve := map[string]bool{}
	for _, kind := range []string{"behavior_packs", "resource_packs"} {
		entries, e := os.ReadDir(filepath.Join(stage, kind))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		for _, d := range entries {
			if !d.IsDir() {
				continue
			}
			key := kind + "/" + d.Name()
			existing := filepath.Join(dst, kind, d.Name())
			if _, e := os.Stat(existing); os.IsNotExist(e) {
				continue
			} else if e != nil {
				return e
			}
			stock := strings.HasPrefix(d.Name(), "vanilla") || d.Name() == "chemistry"
			if !stock || packUUID(filepath.Join(stage, kind, d.Name())) == "" || packUUID(filepath.Join(stage, kind, d.Name())) != packUUID(existing) {
				preserve[key] = true
			}
		}
	}
	return filepath.WalkDir(stage, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		r, _ := filepath.Rel(stage, p)
		parts := strings.Split(filepath.ToSlash(r), "/")
		if len(parts) > 1 && preserve[parts[0]+"/"+parts[1]] {
			return nil
		}
		target := filepath.Join(dst, r)
		parent := filepath.Dir(target)
		for {
			if _, e := os.Stat(parent); e == nil {
				if e = noLinks(parent); e != nil {
					return e
				}
				break
			} else if !os.IsNotExist(e) {
				return e
			}
			next := filepath.Dir(parent)
			if next == parent {
				return fmt.Errorf("missing destination parent")
			}
			parent = next
		}
		return copyVerified(p, target)
	})
}
func packUUID(dir string) string {
	b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		return ""
	}
	var v struct {
		Header struct {
			UUID string `json:"uuid"`
		} `json:"header"`
	}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return v.Header.UUID
}
func safeID(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\:")
}
