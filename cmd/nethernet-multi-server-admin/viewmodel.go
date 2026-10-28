package main

import (
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/service"
	"sort"
	"strings"
)

type AccountRow struct {
	Role                                                                       string
	ID, PersonLabel, EmailLabel, Gamertag, XUID, State, CurrentPlayer, Backend string
	Order                                                                      int
}
type PlayerRow struct{ XUID, Gamertag, AccountID, AccountDisplay, Role, State, LastSeen string }
type ViewState struct {
	Connected                                   bool
	StatusText                                  string
	Accounts                                    []AccountRow
	Players                                     []PlayerRow
	ActiveCount, CreativeActive, SurvivalActive int
}

func BuildViewState(s service.Status) ViewState {
	state := ViewState{Connected: true, ActiveCount: len(s.Active)}
	accounts := append([]service.PublicAccount(nil), s.Accounts...)
	sort.SliceStable(accounts, func(i, j int) bool {
		if accounts[i].Order == accounts[j].Order {
			return accounts[i].PersonLabel < accounts[j].PersonLabel
		}
		return accounts[i].Order < accounts[j].Order
	})
	accountByID := make(map[string]service.PublicAccount, len(accounts))
	activeByPlayer := map[string]string{}
	activeByAccount := map[string]struct{ player, server string }{}
	assignedByAccount := map[string]string{}
	for _, a := range accounts {
		accountByID[a.ID] = a
	}
	for _, player := range s.Players {
		assignedByAccount[player.AccountID] = player.ConsoleGamertag
	}
	for _, active := range s.Active {
		server := shortServer(active.Server)
		activeByPlayer[active.ConsoleXUID] = server
		activeByAccount[active.AccountID] = struct{ player, server string }{active.ConsoleGamertag, server}
		if server == "Creative" {
			state.CreativeActive++
		} else if server == "Survival" {
			state.SurvivalActive++
		}
	}
	for _, a := range accounts {
		row := AccountRow{Role: accountstore.EffectiveRole(a.Role), ID: a.ID, PersonLabel: a.PersonLabel, EmailLabel: a.EmailLabel, Gamertag: a.Gamertag, XUID: a.XUID, State: a.State, Order: a.Order}
		row.CurrentPlayer = assignedByAccount[a.ID]
		if active, ok := activeByAccount[a.ID]; ok {
			row.CurrentPlayer = active.player
			row.Backend = active.server
			row.State = "In use"
		}
		state.Accounts = append(state.Accounts, row)
	}
	players := append([]servicePlayer(nil), makeServicePlayers(s)...)
	sort.SliceStable(players, func(i, j int) bool {
		return strings.ToLower(players[i].Gamertag) < strings.ToLower(players[j].Gamertag)
	})
	for _, p := range players {
		role := s.Roles[p.XUID]
		if role == "" {
			role = "Default"
		}
		row := PlayerRow{XUID: p.XUID, Gamertag: p.Gamertag, AccountID: p.AccountID, Role: role, State: "Offline", LastSeen: p.LastSeen}
		if a, ok := accountByID[p.AccountID]; ok {
			row.AccountDisplay = accountDisplay(a)
		} else {
			row.AccountDisplay = "Unassigned"
		}
		if active, ok := activeByPlayer[p.XUID]; ok {
			row.State = active
		}
		state.Players = append(state.Players, row)
	}
	state.StatusText = "Server connected - " + itoa(len(state.Accounts)) + " accounts - " + itoa(state.ActiveCount) + " active players"
	return state
}

type servicePlayer struct{ XUID, Gamertag, AccountID, LastSeen string }

func makeServicePlayers(s service.Status) []servicePlayer {
	out := make([]servicePlayer, 0, len(s.Players))
	for _, p := range s.Players {
		last := "Never"
		if !p.LastSeen.IsZero() {
			last = p.LastSeen.Local().Format("02 Jan 2006 15:04")
		}
		out = append(out, servicePlayer{p.ConsoleXUID, p.ConsoleGamertag, p.AccountID, last})
	}
	return out
}
func accountDisplay(a service.PublicAccount) string {
	parts := []string{a.PersonLabel}
	if a.EmailLabel != "" {
		parts = append(parts, a.EmailLabel)
	}
	if a.Gamertag != "" {
		parts = append(parts, a.Gamertag)
	}
	return strings.Join(parts, " / ")
}
func shortServer(s string) string {
	v := strings.ToLower(s)
	if strings.Contains(v, "creative") {
		return "Creative"
	}
	if strings.Contains(v, "survival") {
		return "Survival"
	}
	if s == "" {
		return "Connecting"
	}
	return s
}
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
func (v ViewState) CanEditAccount(i int) bool { return v.Connected && i >= 0 && i < len(v.Accounts) }
func (v ViewState) CanUnpair(i int) bool {
	return v.Connected && i >= 0 && i < len(v.Players) && v.Players[i].State == "Offline"
}
func (v ViewState) CanReassign(playerIndex, accountIndex int) bool {
	return v.CanUnpair(playerIndex) && v.CanEditAccount(accountIndex) && accountstore.EffectiveRole(v.Accounts[accountIndex].Role) == accountstore.RoleHelper && v.Accounts[accountIndex].State == "Available" && v.Accounts[accountIndex].CurrentPlayer == ""
}

// Expired authentication does not prevent removal; assignments and active
// sessions must still be cleared by the user before an identity is deleted.
func (v ViewState) CanRemoveAccount(i int) bool {
	if !v.CanEditAccount(i) {
		return false
	}
	row := v.Accounts[i]
	if row.State == "In use" || row.CurrentPlayer != "" {
		return false
	}
	for _, player := range v.Players {
		if player.AccountID == row.ID {
			return false
		}
	}
	return true
}
