package accounts

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"nethernet-multi-server/internal/accountstore"
)

// RequireBroadcasterAuthentication holds only a dedicated Online identity.
// Reauthentication through Accounts restores its health; LAN helpers are excluded.
func (m *Manager) RequireBroadcasterAuthentication(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, a := range m.snapshot.Accounts {
		if a.ID != id {
			continue
		}
		if accountstore.EffectiveRole(a.Role) != accountstore.RoleBroadcaster {
			return errors.New("account is not a broadcaster")
		}
		m.snapshot.Accounts[i].Healthy = false
		if err := m.store.Save(m.snapshot); err != nil {
			m.snapshot.Accounts[i].Healthy = a.Healthy
			return err
		}
		return nil
	}
	return errors.New("broadcaster account not found")
}

// SetRole cannot change an identity already used for a player's world data.
func (m *Manager) SetRole(id, role string) error {
	if err := accountstore.ValidateRole(role); err != nil {
		return err
	}
	role = accountstore.EffectiveRole(role)
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
		if a.ID != id {
			continue
		}
		m.snapshot.Accounts[i].Role = role
		if err := m.store.Save(m.snapshot); err != nil {
			m.snapshot.Accounts[i].Role = a.Role
			return err
		}
		return nil
	}
	return fmt.Errorf("account %q not found", id)
}

// BroadcasterTokenSource provides broker-side token renewal without making a
// broadcaster identity available to the helper lease pool.
func (m *Manager) BroadcasterTokenSource(ctx context.Context, id string) (oauth2.TokenSource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.snapshot.Accounts {
		if a.ID != id {
			continue
		}
		if accountstore.EffectiveRole(a.Role) != accountstore.RoleBroadcaster {
			return nil, errors.New("account is not a broadcaster")
		}
		if !a.Healthy {
			return nil, errors.New("broadcaster account requires reauthentication")
		}
		source, err := m.tokens(ctx, a)
		if err != nil {
			return nil, err
		}
		return &persistingTokenSource{base: source, manager: m, accountID: id}, nil
	}
	return nil, fmt.Errorf("account %q not found", id)
}
