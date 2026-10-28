package migration

import (
	"encoding/json"
	"errors"
	"nethernet-multi-server/internal/accountstore"
)

type PortableAccount struct {
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
		p.Accounts = append(p.Accounts, PortableAccount{a.ID, a.PersonLabel, a.EmailLabel, a.Gamertag, a.XUID, a.Order})
	}
	return json.MarshalIndent(p, "", "  ")
}
func ImportAccounts(b []byte) (accountstore.Snapshot, error) {
	var p PortableAccounts
	if err := json.Unmarshal(b, &p); err != nil {
		return accountstore.Snapshot{}, err
	}
	if p.Version != 1 {
		return accountstore.Snapshot{}, errors.New("unsupported account export")
	}
	s := accountstore.Snapshot{Version: accountstore.CurrentVersion, Players: p.Players}
	seen := map[string]bool{}
	for _, a := range p.Accounts {
		if a.ID == "" || seen[a.ID] {
			return s, errors.New("invalid or duplicate account ID")
		}
		seen[a.ID] = true
		s.Accounts = append(s.Accounts, accountstore.Account{ID: a.ID, PersonLabel: a.PersonLabel, EmailLabel: a.EmailLabel, Gamertag: a.Gamertag, XUID: a.XUID, Order: a.Order, Healthy: false, EncryptedToken: "{}"})
	}
	for _, p := range s.Players {
		if !seen[p.AccountID] {
			return s, errors.New("player mapping references a missing account")
		}
	}
	return s, nil
}
