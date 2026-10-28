package serverupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
)

// Recover restores the verified pre-update state. The displaced state is retained.
func (m *Manager) Recover(ctx context.Context, ack bool) (Status, error) {
	m.mu.Lock()
	if m.busy || !m.status.Recovery || !ack {
		m.mu.Unlock()
		return Status{}, fmt.Errorf("explicit pre-update backup recovery confirmation is required")
	}
	backup := m.status.Backup
	m.busy = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.busy = false; m.mu.Unlock() }()
	parent := filepath.Join(m.root, "MultiServer", "backups", "server-updates")
	if filepath.Dir(backup) != parent {
		return Status{}, fmt.Errorf("invalid recovery backup")
	}
	if e := noLinks(backup); e != nil {
		return Status{}, e
	}
	for _, s := range m.s.Statuses() {
		if s.State != supervisor.Stopped && s.State != supervisor.Failed {
			return Status{}, fmt.Errorf("stop all servers before recovery")
		}
	}
	if e := verifyBackup(backup); e != nil {
		return Status{}, e
	}
	b, e := os.ReadFile(filepath.Join(backup, "config.json"))
	if e != nil {
		return Status{}, e
	}
	var cfg struct {
		Servers []struct {
			ID      string
			Process struct {
				WorkingDirectory string `json:"working_directory"`
			} `json:"process"`
		} `json:"servers"`
	}
	if e = json.Unmarshal(b, &cfg); e != nil {
		return Status{}, e
	}
	for _, s := range cfg.Servers {
		if !safeID(s.ID) || !within(m.root, s.Process.WorkingDirectory) || filepath.Clean(s.Process.WorkingDirectory) == filepath.Clean(m.root) {
			return Status{}, fmt.Errorf("unsafe recovery server location")
		}
		stage := filepath.Join(backup, "recovery-staging", s.ID)
		if _, e = os.Stat(stage); e == nil {
			return Status{}, fmt.Errorf("existing recovery staging requires manual review")
		}
		if e = copyTree(filepath.Join(backup, "servers", s.ID), stage); e != nil {
			return Status{}, e
		}
	}
	if e = m.phase("recovering", "Restoring verified pre-update backup; displaced current data will be kept."); e != nil {
		return Status{}, e
	}
	for _, s := range cfg.Servers {
		dst := s.Process.WorkingDirectory
		if e = noLinks(dst); e != nil {
			return Status{}, e
		}
		saved := filepath.Join(backup, "displaced", s.ID)
		if e = os.MkdirAll(filepath.Dir(saved), 0700); e != nil {
			return Status{}, e
		}
		if e = os.Rename(dst, saved); e != nil {
			return Status{}, e
		}
		if e = os.Rename(filepath.Join(backup, "recovery-staging", s.ID), dst); e != nil {
			_ = os.Rename(saved, dst)
			return Status{}, e
		}
	}
	if e = atomicWrite(m.path, b); e != nil {
		return Status{}, e
	}
	m.mu.Lock()
	m.status.Recovery = false
	m.status.Done = true
	m.status.Phase = "recovered"
	m.status.Message = "Pre-update backup restored. Current data was retained in the displaced folder. Update NetherNet to a matching version before starting these older servers."
	e = m.save()
	m.mu.Unlock()
	return m.Status(), e
}
