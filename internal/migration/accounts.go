package migration

import (
	"encoding/json"
	"errors"
	"nethernet-multi-server/internal/accountstore"
)

type PortableAccount struct {
	Role        string `json:"role"`
	ID          string `json:"id"`
	PersonLabel string `json:"person_label"`
	EmailLabel  string `json:"email_label,omitempty"`
	Gamertag    string `json:"gamertag"`
	XUID        string `json:"xuid"`
	Order       int    `json:"order"`
}
type PortableAccounts struct {
	Version  int                          `json:"version"`
	Accounts []PortableAccount            `json:"accounts"`
	Players  []accountstore.PlayerMapping `json:"players"`
}

func ExportAccounts(s accountstore.Snapshot) ([]byte, error) {
	p := PortableAccounts{Version: 1, Players: s.Players}
	for _, a := range s.Accounts {
		if err := accountstore.ValidateRole(a.Role); err != nil {
			return nil, err
		}
		if accountstore.EffectiveRole(a.Role) != accountstore.RoleHelper {
			p.Version = 2
		}
		p.Accounts = append(p.Accounts, PortableAccount{Role: accountstore.EffectiveRole(a.Role), ID: a.ID, PersonLabel: a.PersonLabel, EmailLabel: a.EmailLabel, Gamertag: a.Gamertag, XUID: a.XUID, Order: a.Order})
	}
	return json.MarshalIndent(p, "", "  ")
}
func ImportAccounts(b []byte) (accountstore.Snapshot, error) {
	var p PortableAccounts
	if err := json.Unmarshal(b, &p); err != nil {
		return accountstore.Snapshot{}, err
	}
	if p.Version != 1 && p.Version != 2 {
		return accountstore.Snapshot{}, errors.New("unsupported account export")
	}
	s := accountstore.Snapshot{Version: accountstore.CurrentVersion, Players: p.Players}
	seen := map[string]bool{}
	for _, a := range p.Accounts {
		if err := accountstore.ValidateRole(a.Role); err != nil {
			return s, err
		}
		if a.ID == "" || seen[a.ID] {
			return s, errors.New("invalid or duplicate account ID")
		}
		if p.Version == 1 && accountstore.EffectiveRole(a.Role) != accountstore.RoleHelper {
			return s, errors.New("non-helper roles require account metadata version 2")
		}
		seen[a.ID] = true
		s.Accounts = append(s.Accounts, accountstore.Account{Role: accountstore.EffectiveRole(a.Role), ID: a.ID, PersonLabel: a.PersonLabel, EmailLabel: a.EmailLabel, Gamertag: a.Gamertag, XUID: a.XUID, Order: a.Order, Healthy: false, EncryptedToken: "{}"})
	}
	for _, p := range s.Players {
		if !seen[p.AccountID] {
			return s, errors.New("player mapping references a missing account")
		}
	}
	return s, nil
}
