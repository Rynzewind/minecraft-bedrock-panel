package accounts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"nethernet-multi-server/internal/accountstore"
)

type fakeAuth struct {
	completeErr error
	identity    XboxIdentity
}

func (f fakeAuth) Begin(context.Context) (DeviceCode, error) {
	return DeviceCode{DeviceCode: "private", UserCode: "ABCD-1234", VerificationURI: "https://example.test", Expiry: time.Now().Add(time.Minute)}, nil
}
func (f fakeAuth) Complete(context.Context, DeviceCode) (*oauth2.Token, XboxIdentity, error) {
	if f.completeErr != nil {
		return nil, XboxIdentity{}, f.completeErr
	}
	return &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}, f.identity, nil
}
func (f fakeAuth) Validate(context.Context, *oauth2.Token) (XboxIdentity, error) {
	if f.completeErr != nil {
		return XboxIdentity{}, f.completeErr
	}
	return f.identity, nil
}
func newEnrollmentStore(t *testing.T) *accountstore.Store {
	t.Helper()
	return accountstore.New(filepath.Join(t.TempDir(), "accounts.json"), protector{})
}
func TestEnrollmentPersistsOnlyAfterValidation(t *testing.T) {
	s := newEnrollmentStore(t)
	code, e, err := BeginEnrollment(context.Background(), s, fakeAuth{identity: XboxIdentity{XUID: "123", Gamertag: "Helper"}}, Labels{Person: "Kevin", Email: "kevin@example.com"})
	if err != nil || code.UserCode == "" {
		t.Fatal(err)
	}
	a, err := e.Complete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Gamertag != "Helper" || a.EmailLabel != "kevin@example.com" {
		t.Fatalf("%+v", a)
	}
}
func TestFailedEnrollmentDoesNotModifyStore(t *testing.T) {
	s := newEnrollmentStore(t)
	_, e, _ := BeginEnrollment(context.Background(), s, fakeAuth{completeErr: errors.New("revoked")}, Labels{Person: "Kevin"})
	if _, err := e.Complete(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	snap, err := s.Load()
	if err != nil || len(snap.Accounts) != 0 {
		t.Fatalf("snapshot=%+v err=%v", snap, err)
	}
}
func TestMigrationKeepsBackupAfterVerifiedImport(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "auth-token.json")
	if err := os.WriteFile(legacy, []byte(`{"access_token":"a","refresh_token":"r"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s := accountstore.New(filepath.Join(dir, "account-store", "accounts.json"), protector{})
	if err := MigrateLegacyToken(context.Background(), dir, s, fakeAuth{identity: XboxIdentity{XUID: "123", Gamertag: "Helper"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "auth-token.migrated-backup.json")); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Load()
	if len(snap.Accounts) != 1 {
		t.Fatalf("%+v", snap)
	}
}
