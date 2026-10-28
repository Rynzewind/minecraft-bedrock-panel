package migration

import (
	"bytes"
	"encoding/json"
	"nethernet-multi-server/internal/accountstore"
	"testing"
)

func TestPortableAccountRolesExcludeTokens(t *testing.T) {
	b, err := ExportAccounts(accountstore.Snapshot{Accounts: []accountstore.Account{{ID: "a", Role: accountstore.RoleBroadcaster, EncryptedToken: "private-secret", Healthy: true}, {ID: "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("private-secret")) {
		t.Fatal("token exported")
	}
	s, err := ImportAccounts(b)
	if err != nil {
		t.Fatal(err)
	}
	if s.Accounts[0].Role != accountstore.RoleBroadcaster || s.Accounts[1].Role != accountstore.RoleHelper || s.Accounts[0].Healthy {
		t.Fatalf("%+v", s)
	}
	if _, err = ImportAccounts([]byte(`{"version":1,"accounts":[{"id":"a","role":"invalid"}]}`)); err == nil {
		t.Fatal("invalid role imported")
	}
}

func TestRoleAwarePortableMetadataCannotLookLegacy(t *testing.T) {
	b, e := ExportAccounts(accountstore.Snapshot{Version: accountstore.CurrentVersion, Accounts: []accountstore.Account{{ID: "b", Role: accountstore.RoleBroadcaster}}})
	if e != nil {
		t.Fatal(e)
	}
	var p PortableAccounts
	json.Unmarshal(b, &p)
	if p.Version != 2 {
		t.Fatalf("role-aware metadata version %d", p.Version)
	}
	if _, e = ImportAccounts(b); e != nil {
		t.Fatal(e)
	}
	if _, e = ImportAccounts([]byte(`{"version":1,"accounts":[{"id":"b","role":"broadcaster"}]}`)); e == nil {
		t.Fatal("legacy-readable broadcaster metadata accepted")
	}
}
