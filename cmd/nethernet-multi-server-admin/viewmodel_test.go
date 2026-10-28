package main

import (
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/service"
	"testing"
	"time"
)

func TestBuildViewStateShowsBothSidesOfAssignment(t *testing.T) {
	s := service.Status{Accounts: []service.PublicAccount{{ID: "a", PersonLabel: "Kevin", EmailLabel: "kevin@example.com", Gamertag: "HelperOne", State: "In use", Order: 0}}, Players: []accountstore.PlayerMapping{{ConsoleXUID: "p", ConsoleGamertag: "KevPS5", AccountID: "a", LastSeen: time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)}}, Active: []accounts.ActiveLease{{ConsoleXUID: "p", ConsoleGamertag: "KevPS5", AccountID: "a", Server: "Private Creative Server"}}}
	state := BuildViewState(s)
	if state.Accounts[0].CurrentPlayer != "KevPS5" || state.Players[0].AccountDisplay != "Kevin / kevin@example.com / HelperOne" {
		t.Fatalf("%+v", state)
	}
	if state.CreativeActive != 1 || state.ActiveCount != 1 {
		t.Fatalf("%+v", state)
	}
}
func TestBuildViewStateSortsWithoutLosingStableIDs(t *testing.T) {
	s := service.Status{Accounts: []service.PublicAccount{{ID: "b", PersonLabel: "Second", Order: 1}, {ID: "a", PersonLabel: "First", Order: 0}}}
	state := BuildViewState(s)
	if state.Accounts[0].ID != "a" || state.Accounts[1].ID != "b" {
		t.Fatalf("%+v", state.Accounts)
	}
	if s.Accounts[0].ID != "b" {
		t.Fatal("source snapshot mutated")
	}
}
func TestEmptyViewDisablesActions(t *testing.T) {
	if (ViewState{}).CanEditAccount(0) || (ViewState{}).CanReassign(0, 0) {
		t.Fatal("empty state enabled an action")
	}
}
func TestActivePlayerOrBusyAccountCannotReassign(t *testing.T) {
	state := ViewState{Connected: true, Accounts: []AccountRow{{ID: "a", State: "In use"}}, Players: []PlayerRow{{XUID: "p", State: "Creative"}}}
	if state.CanReassign(0, 0) {
		t.Fatal("active reassignment enabled")
	}
}

func TestOfflineAssignmentShownInAccountPool(t *testing.T) {
	s := service.Status{
		Accounts: []service.PublicAccount{{ID: "a", PersonLabel: "Kevin", State: "Available"}},
		Players:  []accountstore.PlayerMapping{{ConsoleXUID: "p", ConsoleGamertag: "KevSwitch", AccountID: "a"}},
	}
	state := BuildViewState(s)
	if state.Accounts[0].CurrentPlayer != "KevSwitch" {
		t.Fatalf("offline assignment missing: %+v", state.Accounts[0])
	}
	state.Players = append(state.Players, PlayerRow{XUID: "other", State: "Offline"})
	if state.CanReassign(1, 0) {
		t.Fatal("account assigned to another offline player was offered as available")
	}
}

func TestRemoveAccountAllowsUnhealthyUnpairedButProtectsAssignments(t *testing.T) {
	for _, tc := range []struct {
		name            string
		row             AccountRow
		connected, want bool
	}{
		{"expired unpaired", AccountRow{ID: "a", State: "Needs sign-in"}, true, true},
		{"healthy unpaired", AccountRow{ID: "a", State: "Available"}, true, true},
		{"expired paired", AccountRow{ID: "a", State: "Needs sign-in", CurrentPlayer: "Console"}, true, false},
		{"healthy paired", AccountRow{ID: "a", State: "Available", CurrentPlayer: "Console"}, true, false},
		{"active", AccountRow{ID: "a", State: "In use"}, true, false},
		{"disconnected", AccountRow{ID: "a", State: "Available"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := ViewState{Connected: tc.connected, Accounts: []AccountRow{tc.row}}
			if got := v.CanRemoveAccount(0); got != tc.want {
				t.Fatalf("CanRemoveAccount=%v want %v", got, tc.want)
			}
		})
	}
	if (ViewState{}).CanRemoveAccount(-1) || (ViewState{}).CanRemoveAccount(0) {
		t.Fatal("invalid selection allows removal")
	}
}
