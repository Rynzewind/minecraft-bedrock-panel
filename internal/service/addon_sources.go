package service

import (
	"encoding/json"
	"fmt"
	"nethernet-multi-server/internal/addonupdates"
)

func (c *Controller) SetAddonSources(s *addonupdates.Store) { c.addonSources = s }
func (c *Controller) handleAddonSources(method string, raw json.RawMessage) (any, error) {
	if c.addonSources == nil {
		return nil, fmt.Errorf("add-on sources unavailable")
	}
	if method == "addons.sources.list" {
		return c.addonSources.Links()
	}
	var p struct {
		UUID string `json:"uuid"`
		URL  string `json:"url"`
	}
	if e := json.Unmarshal(raw, &p); e != nil {
		return nil, e
	}
	valid := false
	if c.packs != nil {
		for _, pck := range c.packs.InventoryState() {
			if pck.UUID == p.UUID {
				valid = true
				break
			}
		}
	}
	if !valid {
		return nil, fmt.Errorf("select an installed pack")
	}
	return nil, c.addonSources.Set(p.UUID, p.URL)
}
