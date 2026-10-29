package service

import (
	"context"
	"encoding/json"
	"nethernet-multi-server/internal/accounts"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/online"
	"path/filepath"
	"testing"
)

func TestOnlineRuntimeRefreshesAfterRevokeAndSuppliesActualStatus(t *testing.T) {
	m, e := online.New(filepath.Join(t.TempDir(), "online.json"), []config.Server{{ID: "creative", Backend: "192.0.2.1:19132"}})
	if e != nil {
		t.Fatal(e)
	}
	c := &Controller{}
	c.SetOnline(m, nil)
	changed := 0
	c.SetOnlineRuntime(func(s []online.Status) []online.Status { s[0].Message = "Runtime attached"; return s }, func() { changed++ }, func() any { return "effective evidence" })
	if _, e = c.Handle(context.Background(), "online.players.revoke", json.RawMessage(`{"xuid":"123","server":"creative"}`)); e != nil {
		t.Fatal(e)
	}
	if changed != 1 {
		t.Fatal("revocation not reconciled immediately")
	}
	v, e := c.Handle(context.Background(), "online.status", nil)
	if e != nil || v.([]online.Status)[0].Message != "Runtime attached" {
		t.Fatal("runtime status omitted", e)
	}
	v, e = c.Handle(context.Background(), "online.diagnostics", nil)
	if e != nil || v != "effective evidence" {
		t.Fatal("runtime diagnostics omitted", e)
	}
}

func TestSuccessfulBroadcasterReauthenticationExplicitlyClearsRuntimeHold(t *testing.T) {
	c := &Controller{}
	resetID := ""
	changed := 0
	c.SetOnlineRuntime(nil, func() { changed++ }, nil, func(id string) { resetID = id })
	c.onlineReauthenticated(accountstore.Account{ID: "helper", Role: accountstore.RoleHelper})
	if changed != 0 {
		t.Fatal("LAN helper reset Online hold")
	}
	c.onlineReauthenticated(accountstore.Account{ID: "dedicated", Role: accountstore.RoleBroadcaster})
	if resetID != "dedicated" || changed != 1 {
		t.Fatal("reauthentication hold not cleared")
	}
}

func TestOnlineNeverAuthorizesArbitraryClientXUID(t *testing.T) {
	m, e := online.New(filepath.Join(t.TempDir(), "online.json"), []config.Server{{ID: "creative", Name: "Creative", Protocol: 2225, Backend: "192.0.2.1:19132"}})
	if e != nil {
		t.Fatal(e)
	}
	c := &Controller{}
	c.SetOnline(m, nil)
	if _, e = c.Handle(context.Background(), "online.players.grant", json.RawMessage(`{"xuid":"123","servers":["creative"]}`)); e == nil {
		t.Fatal("client supplied identity granted")
	}
	if m.Authorize("creative", "123", true) {
		t.Fatal("unapproved identity authorized")
	}
	if _, e = c.Handle(context.Background(), "online.players.update", json.RawMessage(`{"xuid":"123","servers":["creative"]}`)); e == nil {
		t.Fatal("unresolved identity update granted")
	}
	c.SetMigration(&fakeMigration{busy: true})
	if _, e = c.Handle(context.Background(), "online.settings.save", json.RawMessage(`{}`)); e == nil {
		t.Fatal("mutation during snapshot")
	}
}
func TestOnlineProfileRequiresStableMatchingIdentity(t *testing.T) {
	b := []byte(`{"profileUsers":[{"id":"253327000001","settings":[{"id":"Gamertag","value":"FixtureFriend"}]}]}`)
	p, e := parseOnlineProfile(b, "fixturefriend")
	if e != nil || p.XUID != "253327000001" {
		t.Fatal(p, e)
	}
	if _, e = parseOnlineProfile(b, "SomebodyElse"); e == nil {
		t.Fatal("mismatched identity")
	}
	if _, e = parseOnlineProfile([]byte(`{"profileUsers":[{"id":"not-xuid","settings":[{"id":"Gamertag","value":"FixtureFriend"}]}]}`), "FixtureFriend"); e == nil {
		t.Fatal("invalid XUID")
	}
}

func TestOnlineIdentityReviewIsExplicitAndServerScoped(t *testing.T) {
	store := accountstore.New(filepath.Join(t.TempDir(), "accounts.json"), plainProtector{})
	if e := store.Save(accountstore.Snapshot{Version: 1, Accounts: []accountstore.Account{{ID: "b", Role: accountstore.RoleBroadcaster, XUID: "111", Gamertag: "FixtureBroadcaster", Healthy: true, EncryptedToken: "{}"}}}); e != nil {
		t.Fatal(e)
	}
	manager, e := accounts.NewManager(store)
	if e != nil {
		t.Fatal(e)
	}
	policy, e := online.New(filepath.Join(t.TempDir(), "online.json"), []config.Server{{ID: "creative", Protocol: 2225, Backend: "192.0.2.1:19132"}, {ID: "survival", Protocol: 2225, Backend: "192.0.2.2:19132"}})
	if e != nil {
		t.Fatal(e)
	}
	c := &Controller{manager: manager, store: store}
	c.SetOnline(policy, nil)
	c.onlineResolver = func(context.Context, string, string) (online.Player, error) {
		return online.Player{XUID: "253327000001", Gamertag: "FixtureFriend"}, nil
	}
	result, e := c.Handle(context.Background(), "online.players.resolve", json.RawMessage(`{"account_id":"b","gamertag":"FixtureFriend"}`))
	if e != nil {
		t.Fatal(e)
	}
	if policy.Authorize("creative", "253327000001", true) {
		t.Fatal("lookup auto-authorized")
	}
	id := result.(map[string]any)["resolution_id"].(string)
	body, _ := json.Marshal(map[string]any{"resolution_id": id, "servers": []string{"creative"}})
	if _, e = c.Handle(context.Background(), "online.players.grant", body); e != nil {
		t.Fatal(e)
	}
	if !policy.Authorize("creative", "253327000001", true) || policy.Authorize("survival", "253327000001", true) || policy.Authorize("creative", "253327000001", false) {
		t.Fatal("scope or authentication policy wrong")
	}
	if _, e = c.Handle(context.Background(), "online.players.grant", body); e == nil {
		t.Fatal("review replay accepted")
	}
	if _, e = c.Handle(context.Background(), "online.players.update", json.RawMessage(`{"xuid":"253327000001","servers":[]}`)); e != nil {
		t.Fatal(e)
	}
	if policy.Authorize("creative", "253327000001", true) {
		t.Fatal("revocation failed")
	}
}
