package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"nethernet-multi-server/internal/autoupdate"
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/supervisor"
	"time"
)

func (c *Controller) SetAutomaticUpdates(path string) error {
	s, err := autoupdate.Open(path)
	if err == nil {
		c.automatic = s
	}
	return err
}
func (c *Controller) handleAutomaticSettings(raw json.RawMessage, save bool) (any, error) {
	if c.automatic == nil {
		return nil, errors.New("Automatic update settings are unavailable")
	}
	if !save {
		return c.automatic.Get(), nil
	}
	var p struct{ Servers, Addons *bool }
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return c.automatic.Set(p.Servers, p.Addons)
}
func (c *Controller) automaticIdle() bool {
	if c.supervisor == nil {
		return false
	}
	states := c.supervisor.Statuses()
	if len(states) == 0 {
		return false
	}
	for _, s := range states {
		if s.State != supervisor.Running || !s.Ready || s.Players != 0 {
			return false
		}
	}
	if c.manager != nil && len(c.manager.Snapshot().Active) > 0 {
		return false
	}
	return true
}
func (c *Controller) automaticBlocked() bool {
	return c.shuttingDown || c.migration != nil && c.migration.Busy() || c.updates != nil && (c.updates.Busy() || c.updates.Status().Recovery)
}
func (c *Controller) automaticPending() bool {
	for _, s := range c.supervisor.Statuses() {
		if c.packs != nil && len(c.packs.Pending(s.ID)) > 0 || c.settings != nil && c.settings.Pending(s.ID) {
			return true
		}
	}
	return false
}

// RunAutomaticUpdates belongs to the helper, so the panel may be closed.
func (c *Controller) RunAutomaticUpdates(ctx context.Context) {
	if c.automatic == nil {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var idleSince time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if !c.automaticIdle() {
				idleSince = time.Time{}
				continue
			}
			if idleSince.IsZero() {
				idleSince = now
			}
			settings := c.automatic.Get()
			if !autoupdate.Due(now, idleSince, settings) {
				continue
			}
			c.operationMu.Lock()
			blocked := c.automaticBlocked()
			c.operationMu.Unlock()
			if blocked {
				continue
			}
			cycle, cancel := context.WithTimeout(ctx, 20*time.Minute)
			message, err := c.automaticCycle(cycle)
			cancel()
			if err != nil {
				message = "Automatic updates deferred: " + err.Error()
			}
			c.operationMu.Lock()
			if !c.automaticBlocked() {
				_ = c.automatic.Record(message)
			}
			c.operationMu.Unlock()
		}
	}
}
func (c *Controller) automaticCall(ctx context.Context, method string, p any) (any, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return c.Handle(ctx, method, b)
}
func (c *Controller) automaticCycle(ctx context.Context) (string, error) {
	if !c.automaticIdle() {
		return "", errors.New("waiting for empty ready worlds")
	}
	if c.automaticPending() {
		return "", errors.New("apply or cancel pending manual changes first")
	}
	settings := c.automatic.Get()

	serverNote := ""
	if settings.Servers && c.updates != nil {
		check, checkErr := c.updates.Check(ctx)
		if checkErr != nil {
			serverNote = "Server check needs review: " + checkErr.Error() + ". "
		} else if check.Needed {
			if !check.Release.Compatible {
				serverNote = "Server release deferred until NetherNet protocol support is available. "
			} else {
				if !c.automatic.Get().Servers {
					return "Server automatic updates switched off.", nil
				}
				if !c.automaticIdle() {
					return "", errors.New("players joined; server update postponed")
				}
				_, startErr := c.automaticCall(ctx, "updates.start", map[string]string{"version": check.Release.Version})
				if startErr != nil {
					return "", startErr
				}
				return "Compatible server update started. Add-on checks follow on the next cycle.", nil
			}
		}
	}
	if !settings.Addons || !c.automatic.Get().Addons {
		return serverNote + "Automatic check complete.", nil
	}
	if c.addonSources == nil || c.packs == nil {
		return "", errors.New("add-on updater is unavailable")
	}
	records, err := c.addonSources.ListRecords()
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	updated := 0
	failures := 0
	inventory := c.packs.InventoryState()
	for _, pack := range inventory {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !c.automatic.Get().Addons {
			return "Add-on automatic updates switched off.", nil
		}
		source := records[pack.UUID]
		if source.LocallyModified || source.Provider != "curseforge" {
			continue
		}
		key := fmt.Sprintf("%d:%s", source.ProjectID, source.URL)
		if seen[key] {
			continue
		}
		var targets []string
		for id, state := range pack.Servers {
			if state.Enabled {
				targets = append(targets, id)
			}
		}
		if len(targets) == 0 {
			continue
		}
		result, err := c.automaticCall(ctx, "addons.check", map[string]string{"UUID": pack.UUID})
		if err != nil {
			failures++
			continue
		}
		check, ok := result.(addonCheckResult)
		if !ok || len(check.Files) == 0 {
			continue
		}
		fresh, err := c.addonSources.ListRecords()
		if err != nil {
			return "", err
		}
		src := fresh[pack.UUID]
		if src.LocallyModified || !src.LatestCheckOK || src.VerifiedVersion == nil || src.VerifiedFileID != src.LatestFileID {
			continue
		}
		if !autoVersionNewer(*src.VerifiedVersion, pack.Version) {
			continue
		}
		value, err := c.automaticCall(ctx, "addons.download", map[string]any{"UUID": pack.UUID, "FileID": src.LatestFileID, "Servers": targets})
		if err != nil {
			failures++
			continue
		}
		preview, ok := value.(contentpacks.UpdatePreview)
		if !ok {
			failures++
			continue
		}
		fresh, err = c.addonSources.ListRecords()
		if err != nil {
			return "", err
		}
		protected := false
		for _, change := range preview.Changes {
			if fresh[change.UUID].LocallyModified {
				protected = true
			}
		}
		if protected {
			failures++
			continue
		}
		if err = c.applyAutomaticAddon(ctx, preview); err != nil {
			return "", err
		}
		seen[key] = true
		updated++
	}
	return serverNote + fmt.Sprintf("Automatic check complete: %d add-on project(s) updated, %d check(s) need review. Custom and manual packs excluded.", updated, failures), nil
}
func autoVersionNewer(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
func (c *Controller) applyAutomaticAddon(ctx context.Context, p contentpacks.UpdatePreview) (err error) {
	c.operationMu.Lock()
	defer c.operationMu.Unlock()
	if c.automaticBlocked() || !c.automaticIdle() || c.automaticPending() {
		return errors.New("worlds changed or maintenance is pending; add-on update postponed")
	}
	if !c.automatic.Get().Addons {
		return errors.New("automatic add-on updates are off")
	}
	stop, ok := c.supervisor.(interface {
		StopIfEmpty(context.Context, string) error
	})
	if !ok {
		return errors.New("empty-world stop protection is unavailable")
	}
	var stopped []string
	defer func() {
		recovery, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		for _, id := range stopped {
			for {
				state := supervisor.Unconfigured
				for _, status := range c.supervisor.Statuses() {
					if status.ID == id {
						state = status.State
					}
				}
				if state == supervisor.Stopping {
					select {
					case <-recovery.Done():
						err = errors.Join(err, fmt.Errorf("%s did not finish stopping; review its state", id))
					case <-time.After(25 * time.Millisecond):
						continue
					}
					break
				}
				if state == supervisor.Stopped || state == supervisor.Failed {
					if startErr := c.supervisor.Start(recovery, id); startErr != nil {
						err = errors.Join(err, fmt.Errorf("restart %s: %w", id, startErr))
					}
				}
				break
			}
		}
		if err == nil {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				ready := true
				states := c.supervisor.Statuses()
				for _, id := range stopped {
					found := false
					for _, state := range states {
						if state.ID == id {
							found = true
							if state.State == supervisor.Failed {
								err = fmt.Errorf("%s failed after update", id)
								return
							}
							if state.State != supervisor.Running || !state.Ready {
								ready = false
							}
						}
					}
					if !found {
						ready = false
					}
				}
				if ready {
					return
				}
				select {
				case <-recovery.Done():
					err = errors.Join(err, errors.New("updated worlds did not become ready"))
					return
				case <-ticker.C:
				}
			}
		}
	}()
	for _, id := range p.Servers {
		stopped = append(stopped, id)
		if err = stop.StopIfEmpty(ctx, id); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	records, e := c.addonSources.ListRecords()
	if e != nil {
		return e
	}
	for _, change := range p.Changes {
		if records[change.UUID].LocallyModified {
			return errors.New("custom companion pack is protected")
		}
	}
	_, err = c.handleAddonUpdates(ctx, "addons.stage", json.RawMessage(fmt.Sprintf(`{"PreviewID":%q}`, p.ID)))
	return err
}
