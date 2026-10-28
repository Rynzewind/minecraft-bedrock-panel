package accounts

import (
	"context"
	"errors"
	"golang.org/x/oauth2"
	"nethernet-multi-server/internal/accountstore"
	"path/filepath"
	"sync"
	"testing"
)

type protector struct{}

func (protector) Protect(b []byte) ([]byte, error)   { return b, nil }
func (protector) Unprotect(b []byte) ([]byte, error) { return b, nil }
func testManager(t *testing.T, n int) *Manager {
	t.Helper()
	s := accountstore.New(filepath.Join(t.TempDir(), "a.json"), protector{})
	snap := accountstore.Snapshot{Version: 1}
	for i := 0; i < n; i++ {
		snap.Accounts = append(snap.Accounts, accountstore.Account{ID: string(rune('a' + i)), XUID: string(rune('1' + i)), Gamertag: "helper", EncryptedToken: "{}", Healthy: true, Order: i})
	}
	if err := s.Save(snap); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(s)
	if err != nil {
		t.Fatal(err)
	}
	m.tokens = func(context.Context, accountstore.Account) (oauth2.TokenSource, error) {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "ok"}), nil
	}
	return m
}
func TestAcquirePersistsStableAssignment(t *testing.T) {
	m := testManager(t, 2)
	a, err := m.Acquire(context.Background(), Player{XUID: "p1", Gamertag: "Sam"}, "Creative")
	if err != nil {
		t.Fatal(err)
	}
	id := a.Account.ID
	a.Release()
	b, err := m.Acquire(context.Background(), Player{XUID: "p1", Gamertag: "Sam"}, "Survival")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	if b.Account.ID != id {
		t.Fatalf("assignment changed %s to %s", id, b.Account.ID)
	}
}
func TestConcurrentAcquireNeverDoubleLeases(t *testing.T) {
	m := testManager(t, 1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"p1", "p2"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			l, e := m.Acquire(context.Background(), Player{XUID: id}, "Creative")
			if e == nil {
				defer l.Release()
			}
			errs <- e
		}(id)
	}
	close(start)
	wg.Wait()
	close(errs)
	success, exhausted := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrPoolExhausted) {
			exhausted++
		}
	}
	if success != 1 || exhausted != 1 {
		t.Fatalf("success=%d exhausted=%d", success, exhausted)
	}
}
