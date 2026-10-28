package accounts

import (
	"context"
	"errors"
	"nethernet-multi-server/internal/accountstore"
	"testing"
)

func TestRolesIsolateBroadcasterFromHelperPool(t *testing.T) {
	m := testManager(t, 1)
	if err := m.SetRole("a", accountstore.RoleBroadcaster); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Acquire(context.Background(), Player{XUID: "local"}, "Creative"); !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("broadcaster leased: %v", err)
	}
	if _, err := m.PreviewReassign("local", "a"); err == nil {
		t.Fatal("broadcaster assignable")
	}
	src, err := m.BroadcasterTokenSource(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = src.Token(); err != nil {
		t.Fatal(err)
	}
	if err = m.SetRole("a", accountstore.RoleUnassigned); err != nil {
		t.Fatal(err)
	}
	if _, err = m.BroadcasterTokenSource(context.Background(), "a"); err == nil {
		t.Fatal("unassigned broadcaster source")
	}
}

func TestRoleChangesProtectActiveAndMappedPlayers(t *testing.T) {
	m := testManager(t, 1)
	if m.Snapshot().Accounts[0].Role != accountstore.RoleHelper {
		t.Fatal("legacy account not helper")
	}
	lease, err := m.Acquire(context.Background(), Player{XUID: "local"}, "Creative")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.SetRole("a", accountstore.RoleBroadcaster); !errors.Is(err, ErrPlayerActive) {
		t.Fatalf("active role change: %v", err)
	}
	lease.Release()
	if err = m.SetRole("a", accountstore.RoleBroadcaster); !errors.Is(err, ErrAccountAssigned) {
		t.Fatalf("mapped role change: %v", err)
	}
	// Even inconsistent restored mappings must never cause broadcaster leasing.
	m.snapshot.Accounts[0].Role = accountstore.RoleBroadcaster
	if _, err = m.Acquire(context.Background(), Player{XUID: "local"}, "Creative"); err == nil {
		t.Fatal("mapped broadcaster leased")
	}
}

func TestBroadcasterEnrollmentAndReauthenticationPreserveRole(t *testing.T) {
	store := newEnrollmentStore(t)
	auth := fakeAuth{identity: XboxIdentity{XUID: "123", Gamertag: "Broadcast"}}
	_, enrollment, err := BeginEnrollment(context.Background(), store, auth, Labels{Person: "Broadcaster", Role: accountstore.RoleBroadcaster})
	if err != nil {
		t.Fatal(err)
	}
	account, err := enrollment.Complete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.Role != accountstore.RoleBroadcaster {
		t.Fatal("enrollment lost role")
	}
	_, replacement, err := BeginEnrollment(context.Background(), store, auth, Labels{Person: "Broadcaster"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := replacement.CompleteReplacement(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != accountstore.RoleBroadcaster {
		t.Fatal("reauth changed role")
	}
	if _, _, err = BeginEnrollment(context.Background(), store, auth, Labels{Person: "Invalid", Role: "typo"}); err == nil {
		t.Fatal("invalid enrollment role")
	}
}

func TestBroadcasterTokenSourceRejectsHelperAndUnhealthyAccounts(t *testing.T) {
	m := testManager(t, 1)
	if _, err := m.BroadcasterTokenSource(context.Background(), "a"); err == nil {
		t.Fatal("helper accepted")
	}
	m.snapshot.Accounts[0].Role = accountstore.RoleBroadcaster
	m.snapshot.Accounts[0].Healthy = false
	if _, err := m.BroadcasterTokenSource(context.Background(), "a"); err == nil {
		t.Fatal("unhealthy account accepted")
	}
}
