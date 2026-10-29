package online

import (
	"errors"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (*Manager, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "online.json")
	m, e := New(p, []config.Server{{ID: "creative", Name: "Creative", Backend: "127.0.0.1:19133", Protocol: 2225}, {ID: "survival", Name: "Survival", Backend: "127.0.0.1:19134", Protocol: 2225}})
	if e != nil {
		t.Fatal(e)
	}
	return m, p
}
func TestPersistenceACLAndCopies(t *testing.T) {
	m, p := fixture(t)
	if m.Settings().Servers["creative"].Enabled {
		t.Fatal("default enabled")
	}
	if m.Authorize("creative", "123", true) {
		t.Fatal("default allow")
	}
	player := Player{XUID: "123", Gamertag: "Friend", Permissions: map[string]bool{"creative": true}}
	if e := m.GrantResolved(player); e != nil {
		t.Fatal(e)
	}
	player.Permissions["creative"] = false
	if !m.Authorize("creative", "123", true) || m.Authorize("survival", "123", true) || m.Authorize("creative", "123", false) || m.Authorize("creative", "", true) {
		t.Fatal("ACL isolation")
	}
	players := m.Players()
	players[0].Permissions["creative"] = false
	if !m.Authorize("creative", "123", true) {
		t.Fatal("copy leaked")
	}
	n, e := New(p, m.servers)
	if e != nil || !n.Authorize("creative", "123", true) {
		t.Fatal("persistence", e)
	}
	if e = m.Revoke("123", "creative"); e != nil {
		t.Fatal(e)
	}
	if m.Authorize("creative", "123", true) {
		t.Fatal("revoke failed")
	}
	if e = m.Remove("123"); e != nil || len(m.Players()) != 0 {
		t.Fatal("remove", e)
	}
}
func TestValidationAndCorruption(t *testing.T) {
	m, p := fixture(t)
	for _, host := range []string{"https://example.org", "a:19132", "x/y", "a b", "-bad.org"} {
		s := m.Settings()
		s.PublicHost = host
		if m.Save(s) == nil {
			t.Fatalf("accepted host %s", host)
		}
	}
	s := m.Settings()
	s.TrustedSubnets = []string{"not-a-subnet"}
	if m.Save(s) == nil {
		t.Fatal("bad subnet")
	}
	for _, x := range []string{"", "0", "+123", "00123", "12x"} {
		if m.GrantResolved(Player{XUID: x, Gamertag: "Friend", Permissions: map[string]bool{"creative": true}}) == nil {
			t.Fatal("bad XUID", x)
		}
	}
	if e := os.WriteFile(p, []byte(`{"version":999}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := New(p, m.servers); e == nil {
		t.Fatal("corruption accepted")
	}
}
func TestAtomicRollback(t *testing.T) {
	m, p := fixture(t)
	s := m.Settings()
	s.PublicHost = "example.org"
	if e := m.Save(s); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(p)
	m.rename = func(string, string) error { return errors.New("injected failure") }
	s.PublicHost = "changed.org"
	if m.Save(s) == nil {
		t.Fatal("expected failure")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) || m.Settings().PublicHost != "example.org" {
		t.Fatal("partial update")
	}
}
func TestHonestLifecycle(t *testing.T) {
	m, _ := fixture(t)
	if m.Statuses(nil, nil)[0].State != "Disabled" {
		t.Fatal("default")
	}
	s := m.Settings()
	v := s.Servers["creative"]
	v.Enabled = true
	v.AccountID = "broadcast"
	s.Servers["creative"] = v
	if e := m.Save(s); e != nil {
		t.Fatal(e)
	}
	if m.Statuses(nil, nil)[0].State != "Server stopped" {
		t.Fatal("stopped")
	}
	live := []supervisor.Status{{ID: "creative", State: supervisor.Running}}
	if m.Statuses(live, nil)[0].State != "Waiting for server" {
		t.Fatal("waiting")
	}
	live[0].Ready = true
	if m.Statuses(live, nil)[0].State != "Account missing" {
		t.Fatal("account")
	}
	a := []AccountStatus{{ID: "broadcast", Role: "broadcaster"}}
	if m.Statuses(live, a)[0].State != "Authentication required" {
		t.Fatal("auth")
	}
	a[0].Healthy = true
	st := m.Statuses(live, a)[0]
	if st.State != "Blocked" || st.Broadcasting || st.RemoteVerified || st.LocalTarget != "127.0.0.1:19133" {
		t.Fatal(st)
	}
	a[0].Role = "helper"
	if m.Statuses(live, a)[0].State != "Blocked" {
		t.Fatal("role bypass")
	}
}
func TestStrictStoredState(t *testing.T) {
	for _, content := range []string{`{"version":1,"version":1}`, `{"version":1,"settings":{"version":1,"servers":{"creative":{},"survival":{}}},"players":[{"xuid":"123","gamertag":"A"},{"xuid":"123","gamertag":"B"}]}`, `{"version":1,"settings":{"version":1,"servers":{"creative":{},"survival":{}}},"unexpected":true}`} {
		m, p := fixture(t)
		if e := os.WriteFile(p, []byte(content), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := New(p, m.servers); e == nil {
			t.Fatal("accepted invalid stored state", content)
		}
	}
}
func TestSettingsIsolationAndFailedGrant(t *testing.T) {
	m, _ := fixture(t)
	s := m.Settings()
	delete(s.Servers, "creative")
	if len(m.Settings().Servers) != 2 {
		t.Fatal("settings map exposed")
	}
	s = m.Settings()
	s.TrustedSubnets = []string{"192.168.0.0/16"}
	if e := m.Save(s); e != nil {
		t.Fatal(e)
	}
	s.TrustedSubnets[0] = "bad"
	if m.Settings().TrustedSubnets[0] != "192.168.0.0/16" {
		t.Fatal("slice exposed")
	}
	m.rename = func(string, string) error { return errors.New("write failure") }
	if m.GrantResolved(Player{XUID: "123", Gamertag: "A", Permissions: map[string]bool{"creative": true}}) == nil {
		t.Fatal("expected failure")
	}
	if m.Authorize("creative", "123", true) {
		t.Fatal("failed grant applied")
	}
	if m.GrantResolved(Player{XUID: "123", Gamertag: "A", Permissions: map[string]bool{"unknown": true}}) == nil {
		t.Fatal("unknown server accepted")
	}
}
func TestInvalidPorts(t *testing.T) {
	m, _ := fixture(t)
	for _, port := range []int{-1, 65536} {
		s := m.Settings()
		v := s.Servers["creative"]
		v.ExternalPort = port
		s.Servers["creative"] = v
		if m.Save(s) == nil {
			t.Fatal("invalid port", port)
		}
	}
}

func TestFriendsRoutingConsentPersistsButDefaultsOff(t *testing.T) {
	m, p := fixture(t)
	s := m.Settings()
	if s.AllowUnknownFriendsRoute {
		t.Fatal("consent defaulted on")
	}
	s.AllowUnknownFriendsRoute = true
	if err := m.Save(s); err != nil {
		t.Fatal(err)
	}
	n, err := New(p, m.servers)
	if err != nil || !n.Settings().AllowUnknownFriendsRoute {
		t.Fatal("explicit choice not saved", err)
	}
	v := s.Servers["creative"]
	v.ExternalPort = 19140
	s.Servers["creative"] = v
	v = s.Servers["survival"]
	v.ExternalPort = 19140
	s.Servers["survival"] = v
	if m.Save(s) == nil {
		t.Fatal("duplicate gateway ports accepted")
	}
}
func TestPreferredInterfaceMetadata(t *testing.T) {
	m, _ := fixture(t)
	s := m.Settings()
	s.PreferredInterface = "Ethernet"
	if e := m.Save(s); e != nil {
		t.Fatal(e)
	}
	if m.Settings().PreferredInterface != "Ethernet" {
		t.Fatal("metadata missing")
	}
	for _, value := range []string{strings.Repeat("a", 129), "Ethernet\nsecret"} {
		s.PreferredInterface = value
		if m.Save(s) == nil {
			t.Fatal("invalid interface accepted")
		}
	}
	if m.Statuses(nil, nil)[0].State != "Disabled" {
		t.Fatal("metadata activated routing")
	}
}
