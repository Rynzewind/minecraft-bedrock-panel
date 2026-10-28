package migration

import (
	"context"
	"fmt"
	"nethernet-multi-server/internal/supervisor"
	"strings"
	"time"
)

func (m *Manager) warning(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !strings.Contains(m.job.Warning, text) {
		if m.job.Warning != "" {
			m.job.Warning += "; "
		}
		m.job.Warning += text
		m.saveLocked()
	}
}
func (m *Manager) broadcast(ctx context.Context, text string) {
	a, ok := m.supervisor.(interface {
		Announce(context.Context, string, string) error
	})
	for _, s := range m.supervisor.Statuses() {
		if s.State != supervisor.Running {
			continue
		}
		if !ok {
			m.warning("In-game warnings unavailable")
			continue
		}
		if err := a.Announce(ctx, s.ID, text); err != nil {
			m.warning("In-game warning failed for " + s.ID + ": " + err.Error())
		}
	}
}
func (m *Manager) warnScheduled(ctx context.Context) error {
	running := false
	for _, s := range m.supervisor.Statuses() {
		if s.State == supervisor.Running {
			running = true
		}
	}
	if !running {
		return ctx.Err()
	}
	wait := m.warningWait
	if wait == nil {
		wait = func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}
	for minutes := 5; minutes >= 1; minutes-- {
		if err := ctx.Err(); err != nil {
			m.broadcast(context.Background(), "[NetherNet] Scheduled backup cancelled. Servers will remain online.")
			return err
		}
		m.mu.Lock()
		m.job.Phase = "warning"
		m.job.Progress = 5
		m.job.WarningSeconds = minutes * 60
		m.saveLocked()
		m.mu.Unlock()
		unit := "minutes"
		if minutes == 1 {
			unit = "minute"
		}
		m.broadcast(ctx, fmt.Sprintf("[NetherNet] Scheduled backup: server will restart in %d %s. Your world will be saved. Please reconnect after the restart.", minutes, unit))
		if err := wait(ctx, time.Minute); err != nil {
			m.broadcast(context.Background(), "[NetherNet] Scheduled backup cancelled. Servers will remain online.")
			return err
		}
	}
	m.mu.Lock()
	m.job.WarningSeconds = 0
	m.mu.Unlock()
	m.broadcast(ctx, "[NetherNet] Scheduled backup starting now. Saving the world and restarting; please reconnect shortly.")
	return ctx.Err()
}
