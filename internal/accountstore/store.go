package accountstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const CurrentVersion = 1

// Non-helper roles require a disk schema that older helpers reject, so a
// rollback cannot silently allocate a broadcaster identity to a LAN player.
const roleAwareDiskVersion = 2

const (
	RoleHelper      = "helper"
	RoleBroadcaster = "broadcaster"
	RoleUnassigned  = "unassigned"
)

// EffectiveRole keeps pre-role account stores compatible with helper allocation.
func EffectiveRole(role string) string {
	if role == "" {
		return RoleHelper
	}
	return role
}

func ValidateRole(role string) error {
	switch EffectiveRole(role) {
	case RoleHelper, RoleBroadcaster, RoleUnassigned:
		return nil
	}
	return fmt.Errorf("invalid account role %q", role)
}

type Account struct {
	Role           string `json:"role"`
	ID             string `json:"id"`
	PersonLabel    string `json:"person_label"`
	EmailLabel     string `json:"email_label,omitempty"`
	Gamertag       string `json:"gamertag"`
	XUID           string `json:"xuid"`
	EncryptedToken string `json:"encrypted_token"`
	Order          int    `json:"order"`
	Healthy        bool   `json:"healthy"`
}
type PlayerMapping struct {
	ConsoleXUID     string    `json:"console_xuid"`
	ConsoleGamertag string    `json:"console_gamertag"`
	AccountID       string    `json:"account_id"`
	LastSeen        time.Time `json:"last_seen"`
}
type Snapshot struct {
	Version  int             `json:"version"`
	Accounts []Account       `json:"accounts"`
	Players  []PlayerMapping `json:"players"`
}
type Protector interface {
	Protect([]byte) ([]byte, error)
	Unprotect([]byte) ([]byte, error)
}

type diskAccount struct {
	Role                                               string `json:"Role,omitempty"`
	ID, PersonLabel, EmailLabel, Gamertag, XUID, Token string
	Order                                              int
	Healthy                                            bool
}
type diskSnapshot struct {
	Version  int
	Accounts []diskAccount
	Players  []PlayerMapping
}

type Store struct {
	path      string
	protector Protector
	mu        sync.Mutex
	rename    func(string, string) error
}

func New(path string, p Protector) *Store {
	return &Store{path: path, protector: p, rename: replaceFile}
}
func (s *Store) Path() string { return s.path }
func (s *Store) Load() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}
func (s *Store) load() (Snapshot, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{Version: CurrentVersion}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("read account store: %w", err)
	}
	var d diskSnapshot
	if err = json.Unmarshal(b, &d); err != nil {
		return Snapshot{}, fmt.Errorf("decode account store: %w", err)
	}
	if d.Version != CurrentVersion && d.Version != roleAwareDiskVersion {
		return Snapshot{}, fmt.Errorf("unsupported account store version %d", d.Version)
	}
	out := Snapshot{Version: CurrentVersion, Players: d.Players, Accounts: make([]Account, 0, len(d.Accounts))}
	for _, a := range d.Accounts {
		if err := ValidateRole(a.Role); err != nil {
			return Snapshot{}, err
		}
		if d.Version == CurrentVersion && EffectiveRole(a.Role) != RoleHelper {
			return Snapshot{}, errors.New("non-helper account role requires role-aware account store version")
		}
		cipher, err := base64.StdEncoding.DecodeString(a.Token)
		if err != nil {
			return Snapshot{}, fmt.Errorf("decode token for %s: %w", a.ID, err)
		}
		plain, err := s.protector.Unprotect(cipher)
		if err != nil {
			return Snapshot{}, fmt.Errorf("decrypt token for %s: %w", a.ID, err)
		}
		out.Accounts = append(out.Accounts, Account{Role: EffectiveRole(a.Role), ID: a.ID, PersonLabel: a.PersonLabel, EmailLabel: a.EmailLabel, Gamertag: a.Gamertag, XUID: a.XUID, EncryptedToken: string(plain), Order: a.Order, Healthy: a.Healthy})
	}
	return out, nil
}
func (s *Store) Save(in Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.Version == 0 {
		in.Version = CurrentVersion
	}
	if in.Version != CurrentVersion {
		return fmt.Errorf("unsupported account store version %d", in.Version)
	}
	d := diskSnapshot{Version: in.Version, Players: in.Players, Accounts: make([]diskAccount, 0, len(in.Accounts))}
	for _, a := range in.Accounts {
		if err := ValidateRole(a.Role); err != nil {
			return err
		}
		if EffectiveRole(a.Role) != RoleHelper {
			d.Version = roleAwareDiskVersion
		}
		cipher, err := s.protector.Protect([]byte(a.EncryptedToken))
		if err != nil {
			return fmt.Errorf("encrypt token for %s: %w", a.ID, err)
		}
		d.Accounts = append(d.Accounts, diskAccount{Role: EffectiveRole(a.Role), ID: a.ID, PersonLabel: a.PersonLabel, EmailLabel: a.EmailLabel, Gamertag: a.Gamertag, XUID: a.XUID, Token: base64.StdEncoding.EncodeToString(cipher), Order: a.Order, Healthy: a.Healthy})
	}
	sort.SliceStable(d.Accounts, func(i, j int) bool { return d.Accounts[i].Order < d.Accounts[j].Order })
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = s.rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install account store: %w", err)
	}
	return nil
}
