package accountstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRoleAwareDiskVersionPreventsLegacyHelperRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	s := New(path, testProtector{1})
	for _, tc := range []struct {
		role    string
		version int
	}{{"", 1}, {RoleHelper, 1}, {RoleBroadcaster, 2}, {RoleUnassigned, 2}} {
		if err := s.Save(Snapshot{Version: CurrentVersion, Accounts: []Account{{ID: "a", Role: tc.role}}}); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var disk diskSnapshot
		if err = json.Unmarshal(raw, &disk); err != nil {
			t.Fatal(err)
		}
		if disk.Version != tc.version {
			t.Fatalf("role %q disk version=%d want=%d", tc.role, disk.Version, tc.version)
		}
		snap, err := s.Load()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Version != CurrentVersion || snap.Accounts[0].Role != EffectiveRole(tc.role) {
			t.Fatalf("%+v", snap)
		}
	}
	for _, role := range []string{RoleBroadcaster, RoleUnassigned} {
		forged := diskSnapshot{Version: 1, Accounts: []diskAccount{{ID: "a", Role: role}}}
		raw, _ := json.Marshal(forged)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Load(); err == nil {
			t.Fatalf("legacy disk accepted %s", role)
		}
	}
}

func TestLegacyStoreDefaultsRoleAndRejectsUnknownRole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	s := New(path, testProtector{1})
	if err := os.WriteFile(path, []byte(`{"Version":1,"Accounts":[{"ID":"legacy","Token":""}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Accounts[0].Role != RoleHelper {
		t.Fatal("legacy helper role lost")
	}
	if err = os.WriteFile(path, []byte(`{"Version":1,"Accounts":[{"ID":"legacy","Token":"","Role":"typo"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(); err == nil {
		t.Fatal("unknown persisted role accepted")
	}
}

func TestRolePersistenceAndValidation(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "accounts.json"), testProtector{1})
	for _, role := range []string{"", RoleBroadcaster, RoleUnassigned} {
		if err := s.Save(Snapshot{Accounts: []Account{{ID: "a", Role: role}}}); err != nil {
			t.Fatal(err)
		}
		got, err := s.Load()
		if err != nil {
			t.Fatal(err)
		}
		if got.Accounts[0].Role != EffectiveRole(role) {
			t.Fatalf("role=%q", got.Accounts[0].Role)
		}
	}
	if err := s.Save(Snapshot{Accounts: []Account{{ID: "a", Role: "unknown"}}}); err == nil {
		t.Fatal("invalid role accepted")
	}
}
