package service

import (
	"context"
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/serverupdate"
)

type updateManager interface {
	Busy() bool
	Status() serverupdate.Status
	Check(context.Context) (serverupdate.Check, error)
	Start(context.Context, string) (serverupdate.Status, error)
	Recover(context.Context, bool) (serverupdate.Status, error)
}

func (c *Controller) SetUpdates(m updateManager) { c.updates = m }
func (c *Controller) handleUpdates(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if c.updates == nil {
		return nil, fmt.Errorf("Update support requires the newer NetherNet helper")
	}
	switch method {
	case "updates.check":
		return c.updates.Check(ctx)
	case "updates.status":
		return c.updates.Status(), nil
	case "updates.start":
		if c.supervisor != nil {
			for _, srv := range c.supervisor.Statuses() {
				if c.packs != nil && len(c.packs.Pending(srv.ID)) > 0 || c.settings != nil && c.settings.Pending(srv.ID) {
					return nil, fmt.Errorf("Apply or cancel pending server changes before updating.")
				}
			}
		}
		var p struct {
			Version string `json:"version"`
		}
		if e := json.Unmarshal(raw, &p); e != nil {
			return nil, e
		}
		return c.updates.Start(ctx, p.Version)
	case "updates.recover":
		var p struct {
			Confirm bool `json:"confirm"`
		}
		if e := json.Unmarshal(raw, &p); e != nil {
			return nil, e
		}
		return c.updates.Recover(ctx, p.Confirm)
	}
	return nil, fmt.Errorf("unknown update operation")
}
