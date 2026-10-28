package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/online"
	"nethernet-multi-server/internal/supervisor"
	"strings"
	"time"
)

type resolvedOnlinePlayer struct {
	Player    online.Player
	AccountID string
	Expires   time.Time
}

func (c *Controller) SetOnline(m *online.Manager, loadError error) {
	c.online = m
	c.onlineLoadError = loadError
	c.onlineResolutions = map[string]resolvedOnlinePlayer{}
}
func (c *Controller) onlineAccounts() []online.AccountStatus {
	var out []online.AccountStatus
	if c.manager != nil {
		for _, a := range c.manager.Snapshot().Accounts {
			out = append(out, online.AccountStatus{ID: a.ID, Gamertag: a.Gamertag, Role: accountstore.EffectiveRole(a.Role), Healthy: a.Healthy})
		}
	}
	return out
}
func (c *Controller) handleOnline(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if c.online == nil {
		return nil, fmt.Errorf("Online configuration is unavailable; LAN operation is independent. Repair the saved Online settings before continuing")
	}
	var p struct {
		Settings     online.Settings `json:"settings"`
		AccountID    string          `json:"account_id"`
		Gamertag     string          `json:"gamertag"`
		ResolutionID string          `json:"resolution_id"`
		XUID         string          `json:"xuid"`
		Servers      []string        `json:"servers"`
		Server       string          `json:"server"`
		Type         string          `json:"type"`
	}
	if len(raw) > 0 {
		if len(raw) > 65536 || json.Unmarshal(raw, &p) != nil {
			return nil, fmt.Errorf("invalid Online request")
		}
	}
	switch method {
	case "online.settings.get":
		return c.online.Settings(), nil
	case "online.settings.save":
		used := map[string]bool{}
		available := map[string]online.AccountStatus{}
		for _, a := range c.onlineAccounts() {
			available[a.ID] = a
		}
		for _, s := range p.Settings.Servers {
			if s.AccountID == "" {
				continue
			}
			a, ok := available[s.AccountID]
			if !ok || a.Role != accountstore.RoleBroadcaster {
				return nil, fmt.Errorf("select a broadcaster account enrolled through Accounts")
			}
			if used[a.ID] {
				return nil, fmt.Errorf("use a separate broadcaster identity for each world")
			}
			used[a.ID] = true
		}
		return nil, c.online.Save(p.Settings)
	case "online.status":
		var statuses []supervisor.Status
		if c.supervisor != nil {
			statuses = c.supervisor.Statuses()
		}
		return c.online.Statuses(statuses, c.onlineAccounts()), nil
	case "online.players.list":
		return c.online.Players(), nil
	case "online.players.resolve":
		if strings.TrimSpace(p.Gamertag) == "" || len(p.Gamertag) > 64 {
			return nil, fmt.Errorf("enter an Xbox gamertag")
		}
		if !c.broadcasterAccount(p.AccountID) {
			return nil, fmt.Errorf("select a healthy broadcaster account from Accounts")
		}
		resolver := c.onlineResolver
		if resolver == nil {
			resolver = c.resolveOnlineGamertag
		}
		player, e := resolver(ctx, p.AccountID, strings.TrimSpace(p.Gamertag))
		if e != nil {
			return nil, e
		}
		player.Permissions = map[string]bool{}
		idBytes := make([]byte, 16)
		if _, e = rand.Read(idBytes); e != nil {
			return nil, fmt.Errorf("cannot create identity review")
		}
		id := hex.EncodeToString(idBytes)
		c.mu.Lock()
		for key, v := range c.onlineResolutions {
			if time.Now().After(v.Expires) {
				delete(c.onlineResolutions, key)
			}
		}
		if len(c.onlineResolutions) >= 64 {
			c.mu.Unlock()
			return nil, fmt.Errorf("too many pending identity reviews")
		}
		c.onlineResolutions[id] = resolvedOnlinePlayer{player, p.AccountID, time.Now().Add(5 * time.Minute)}
		c.mu.Unlock()
		return map[string]any{"resolution_id": id, "player": player}, nil
	case "online.players.grant":
		c.mu.Lock()
		r, ok := c.onlineResolutions[p.ResolutionID]
		delete(c.onlineResolutions, p.ResolutionID)
		c.mu.Unlock()
		if !ok || time.Now().After(r.Expires) || !c.broadcasterAccount(r.AccountID) {
			return nil, fmt.Errorf("identity review expired; resolve the gamertag again")
		}
		r.Player.Permissions = map[string]bool{}
		for _, id := range p.Servers {
			r.Player.Permissions[id] = true
		}
		return nil, c.online.GrantResolved(r.Player)
	case "online.players.update":
		for _, player := range c.online.Players() {
			if player.XUID == p.XUID {
				player.Permissions = map[string]bool{}
				for _, id := range p.Servers {
					player.Permissions[id] = true
				}
				return nil, c.online.GrantResolved(player)
			}
		}
		return nil, fmt.Errorf("select an existing resolved player")
	case "online.players.revoke":
		return nil, c.online.Revoke(p.XUID, p.Server)
	case "online.players.remove":
		return nil, c.online.Remove(p.XUID)
	case "online.logs":
		return c.online.Events(p.Server, p.Type), nil
	case "online.diagnostics":
		interfaces, e := net.Interfaces()
		if e != nil {
			return nil, fmt.Errorf("cannot inspect local adapters")
		}
		type adapter struct {
			Name      string
			Addresses []string
		}
		var adapters []adapter
		for _, iface := range interfaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addresses, e := iface.Addrs()
			if e != nil {
				continue
			}
			a := adapter{Name: iface.Name}
			for _, addr := range addresses {
				a.Addresses = append(a.Addresses, addr.String())
			}
			adapters = append(adapters, a)
		}
		return map[string]any{"adapters": adapters, "lan": "Existing LAN discovery remains independent", "internet": "Unverified: this build has no remote game listener", "locality": "Unknown: candidate/private IP data is not proof of physical LAN membership", "firewall": "No firewall rules opened or changed", "cgnat": "Unverified: compare router WAN address with public address; local inspection alone cannot establish inbound reachability"}, nil
	}
	return nil, fmt.Errorf("unknown Online operation")
}
func (c *Controller) broadcasterAccount(id string) bool {
	for _, a := range c.onlineAccounts() {
		if a.ID == id && a.Role == accountstore.RoleBroadcaster && a.Healthy {
			return true
		}
	}
	return false
}
