package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"nethernet-multi-server/internal/accountstore"
)

var ErrPoolExhausted = errors.New("no proxy player account is available")
var ErrMappedAccountBusy = errors.New("player's assigned proxy account is currently in use")
var ErrPlayerActive = errors.New("player is currently active")
var ErrAccountAssigned = errors.New("proxy account is assigned to another player")

type Player struct {
	XUID     string `json:"xuid"`
	Gamertag string `json:"gamertag"`
}
type Lease struct {
	Account     accountstore.Account
	TokenSource oauth2.TokenSource
	Release     func()
}
type ActiveLease struct {
	ConsoleXUID, ConsoleGamertag, AccountID, Server string
	Started                                         time.Time
}
type StatusSnapshot struct {
	Accounts []accountstore.Account       `json:"accounts"`
	Players  []accountstore.PlayerMapping `json:"players"`
	Active   []ActiveLease                `json:"active"`
}
type ReassignmentImpact struct{ ConsoleXUID, OldAccountID, NewAccountID, OldGamertag, NewGamertag string }

type tokenFactory func(context.Context, accountstore.Account) (oauth2.TokenSource, error)
type Manager struct {
	mu              sync.Mutex
	store           *accountstore.Store
	snapshot        accountstore.Snapshot
	activeByPlayer  map[string]ActiveLease
	activeByAccount map[string]string
	tokens          tokenFactory
}

type persistingTokenSource struct {
	base      oauth2.TokenSource
	manager   *Manager
	accountID string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	t, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	p.manager.mu.Lock()
	defer p.manager.mu.Unlock()
	for i := range p.manager.snapshot.Accounts {
		if p.manager.snapshot.Accounts[i].ID == p.accountID && p.manager.snapshot.Accounts[i].EncryptedToken != string(b) {
			p.manager.snapshot.Accounts[i].EncryptedToken = string(b)
			p.manager.snapshot.Accounts[i].Healthy = true
			if err := p.manager.store.Save(p.manager.snapshot); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

func NewManager(store *accountstore.Store) (*Manager, error) {
	snap, err := store.Load()
	if err != nil {
		return nil, err
	}
	return &Manager{store: store, snapshot: snap, activeByPlayer: map[string]ActiveLease{}, activeByAccount: map[string]string{}, tokens: func(_ context.Context, a accountstore.Account) (oauth2.TokenSource, error) { return TokenSource(a) }}, nil
}
func (m *Manager) Acquire(ctx context.Context, p Player, server string) (*Lease, error) {
	if p.XUID == "" {
		return nil, errors.New("authenticated console XUID is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.activeByPlayer[p.XUID]; ok {
		return nil, ErrMappedAccountBusy
	}
	var mapped *accountstore.PlayerMapping
	for i := range m.snapshot.Players {
		if m.snapshot.Players[i].ConsoleXUID == p.XUID {
			mapped = &m.snapshot.Players[i]
			break
		}
	}
	accounts := append([]accountstore.Account(nil), m.snapshot.Accounts...)
	sort.SliceStable(accounts, func(i, j int) bool {
		if accounts[i].Order == accounts[j].Order {
			return accounts[i].ID < accounts[j].ID
		}
		return accounts[i].Order < accounts[j].Order
	})
	assigned := map[string]bool{}
	for _, pm := range m.snapshot.Players {
		if pm.ConsoleXUID != p.XUID {
			assigned[pm.AccountID] = true
		}
	}
	var chosen accountstore.Account
	var chosenSource oauth2.TokenSource
	found := false
	for _, a := range accounts {
		if mapped != nil && a.ID != mapped.AccountID {
			continue
		}
		if !a.Healthy || assigned[a.ID] {
			continue
		}
		if _, busy := m.activeByAccount[a.ID]; busy {
			if mapped != nil {
				return nil, ErrMappedAccountBusy
			}
			continue
		}
		source, err := m.tokens(ctx, a)
		if err != nil {
			m.setHealthyLocked(a.ID, false)
			continue
		}
		fresh, tokenErr := source.Token()
		if tokenErr != nil {
			m.setHealthyLocked(a.ID, false)
			continue
		}
		if encoded, marshalErr := json.Marshal(fresh); marshalErr == nil {
			for i := range m.snapshot.Accounts {
				if m.snapshot.Accounts[i].ID == a.ID {
					m.snapshot.Accounts[i].EncryptedToken = string(encoded)
					m.snapshot.Accounts[i].Healthy = true
					a = m.snapshot.Accounts[i]
				}
			}
		}
		chosen = a
		chosenSource = source
		found = true
		break
	}
	if !found {
		_ = m.store.Save(m.snapshot)
		if mapped != nil {
			return nil, ErrMappedAccountBusy
		}
		return nil, ErrPoolExhausted
	}
	now := time.Now().UTC()
	if mapped == nil {
		m.snapshot.Players = append(m.snapshot.Players, accountstore.PlayerMapping{ConsoleXUID: p.XUID, ConsoleGamertag: p.Gamertag, AccountID: chosen.ID, LastSeen: now})
	} else {
		mapped.ConsoleGamertag = p.Gamertag
		mapped.LastSeen = now
	}
	if err := m.store.Save(m.snapshot); err != nil {
		return nil, err
	}
	active := ActiveLease{ConsoleXUID: p.XUID, ConsoleGamertag: p.Gamertag, AccountID: chosen.ID, Server: server, Started: now}
	m.activeByPlayer[p.XUID] = active
	m.activeByAccount[chosen.ID] = p.XUID
	var once sync.Once
	return &Lease{Account: chosen, TokenSource: &persistingTokenSource{base: chosenSource, manager: m, accountID: chosen.ID}, Release: func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.activeByPlayer, p.XUID)
			delete(m.activeByAccount, chosen.ID)
			m.mu.Unlock()
		})
	}}, nil
}

func (m *Manager) setHealthyLocked(accountID string, healthy bool) {
	for i := range m.snapshot.Accounts {
		if m.snapshot.Accounts[i].ID == accountID {
			m.snapshot.Accounts[i].Healthy = healthy
		}
	}
}
func (m *Manager) Snapshot() StatusSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := StatusSnapshot{Accounts: append([]accountstore.Account(nil), m.snapshot.Accounts...), Players: append([]accountstore.PlayerMapping(nil), m.snapshot.Players...)}
	for _, a := range m.activeByPlayer {
		out.Active = append(out.Active, a)
	}
	return out
}
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap, err := m.store.Load()
	if err != nil {
		return err
	}
	m.snapshot = snap
	return nil
}
func (m *Manager) UpdateLabels(id, person, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if person == "" {
		return errors.New("person label is required")
	}
	for i := range m.snapshot.Accounts {
		if m.snapshot.Accounts[i].ID == id {
			m.snapshot.Accounts[i].PersonLabel = person
			m.snapshot.Accounts[i].EmailLabel = email
			return m.store.Save(m.snapshot)
		}
	}
	return fmt.Errorf("account %q not found", id)
}
func (m *Manager) RemoveAccount(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.activeByAccount[id]; ok {
		return ErrPlayerActive
	}
	for _, p := range m.snapshot.Players {
		if p.AccountID == id {
			return ErrAccountAssigned
		}
	}
	for i, a := range m.snapshot.Accounts {
		if a.ID == id {
			m.snapshot.Accounts = append(m.snapshot.Accounts[:i], m.snapshot.Accounts[i+1:]...)
			return m.store.Save(m.snapshot)
		}
	}
	return fmt.Errorf("account %q not found", id)
}
func (m *Manager) Reorder(ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(ids) != len(m.snapshot.Accounts) {
		return errors.New("reorder must contain every account")
	}
	order := map[string]int{}
	for i, id := range ids {
		if _, exists := order[id]; exists {
			return errors.New("duplicate account in reorder")
		}
		order[id] = i
	}
	for i := range m.snapshot.Accounts {
		n, ok := order[m.snapshot.Accounts[i].ID]
		if !ok {
			return fmt.Errorf("account %q missing from reorder", m.snapshot.Accounts[i].ID)
		}
		m.snapshot.Accounts[i].Order = n
	}
	return m.store.Save(m.snapshot)
}
func (m *Manager) PreviewReassign(xuid, accountID string) (ReassignmentImpact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.previewLocked(xuid, accountID)
}
func (m *Manager) previewLocked(xuid, accountID string) (ReassignmentImpact, error) {
	if _, ok := m.activeByPlayer[xuid]; ok {
		return ReassignmentImpact{}, ErrPlayerActive
	}
	if _, ok := m.activeByAccount[accountID]; ok {
		return ReassignmentImpact{}, ErrPlayerActive
	}
	var old string
	for _, p := range m.snapshot.Players {
		if p.ConsoleXUID == xuid {
			old = p.AccountID
		} else if p.AccountID == accountID {
			return ReassignmentImpact{}, ErrAccountAssigned
		}
	}
	var impact = ReassignmentImpact{ConsoleXUID: xuid, OldAccountID: old, NewAccountID: accountID}
	for _, a := range m.snapshot.Accounts {
		if a.ID == old {
			impact.OldGamertag = a.Gamertag
		}
		if a.ID == accountID {
			impact.NewGamertag = a.Gamertag
		}
	}
	if impact.NewGamertag == "" {
		return ReassignmentImpact{}, fmt.Errorf("account %q not found", accountID)
	}
	return impact, nil
}
func (m *Manager) Reassign(xuid, accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.previewLocked(xuid, accountID); err != nil {
		return err
	}
	for i := range m.snapshot.Players {
		if m.snapshot.Players[i].ConsoleXUID == xuid {
			m.snapshot.Players[i].AccountID = accountID
			m.snapshot.Players[i].LastSeen = time.Now().UTC()
			return m.store.Save(m.snapshot)
		}
	}
	return fmt.Errorf("player %q not found", xuid)
}
func (m *Manager) Unpair(xuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.activeByPlayer[xuid]; ok {
		return ErrPlayerActive
	}
	for i, p := range m.snapshot.Players {
		if p.ConsoleXUID == xuid {
			m.snapshot.Players = append(m.snapshot.Players[:i], m.snapshot.Players[i+1:]...)
			return m.store.Save(m.snapshot)
		}
	}
	return nil
}
