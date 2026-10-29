package onlineguard

import (
	"strings"
	"testing"
	"time"
)

func TestWindowsEmptyOptionalValuesAndSingletonAddresses(t *testing.T) {
	p := planFixture(t)
	now := time.Now()
	e := evidenceFixture(p, now)
	e.Rules[0].Spec.RemoteAddresses = append([]string(nil), p.Rules[0].RemoteAddresses...)
	for i := range e.Profiles {
		e.Profiles[i].DisabledInterfaceAliases = []string{""}
	}
	e.Rules[0].Spec.RemoteDynamicKeywordAddresses = []string{""}
	e.Rules[0].Spec.Platforms = []string{""}
	for i, v := range e.Rules[0].Spec.RemoteAddresses {
		lo, hi, ok := strings.Cut(v, "-")
		if ok && lo == hi {
			e.Rules[0].Spec.RemoteAddresses[i] = lo
		}
	}
	if r := EvaluateAt(p, e, now); !r.SnapshotMatches {
		t.Fatalf("harmless Windows serialization rejected: %v", r.Blockers)
	}
	e.Profiles[0].DisabledInterfaceAliases = []string{"", "Ethernet"}
	if EvaluateAt(p, e, now).SnapshotMatches {
		t.Fatal("real interface exclusion accepted")
	}
}

func TestIPv4BackendPolicyDoesNotDependOnIPv6PrivacyAddresses(t *testing.T) {
	backends := []Backend{{ServerID: "creative", Program: `C:\NetherNet\Creative\bedrock_server.exe`, Target: "192.168.1.246:19134"}}
	a, err := BuildPlan(backends, []string{"192.168.1.246", "2001:db8::123"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildPlan(backends, []string{"192.168.1.246", "2001:db8::456"})
	if err != nil {
		t.Fatal(err)
	}
	if !equalRule(a.Rules[0], b.Rules[0]) {
		t.Fatal("IPv6 privacy rotation changed protection for an IPv4-only backend")
	}
	for _, ip := range []string{"2001:db8::123", "2001:db8::456", "2001:db8::789"} {
		if !RemoteBlocked(a.Rules[0], ip) {
			t.Fatal("unneeded IPv6 exception", ip)
		}
	}
	if RemoteBlocked(a.Rules[0], "192.168.1.246") || RemoteBlocked(a.Rules[0], "::1") {
		t.Fatal("local helper or loopback blocked")
	}
}

func TestSharedProgramRetainsBothBackendAddressFamilies(t *testing.T) {
	program := `C:\NetherNet\Creative\bedrock_server.exe`
	p, err := BuildPlan([]Backend{{ServerID: "v4", Program: program, Target: "192.168.1.246:19134"}, {ServerID: "v6", Program: program, Target: "[2001:db8::123]:19136"}}, []string{"192.168.1.246", "2001:db8::123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rules) != 1 || RemoteBlocked(p.Rules[0], "2001:db8::123") || RemoteBlocked(p.Rules[0], "192.168.1.246") {
		t.Fatal("shared program lost a required local address family")
	}
}

func TestLocalUserScopeIsConservative(t *testing.T) {
	const sid = "S-1-5-21-1-2-3-1009"
	for _, scope := range []string{"D:(A;;CC;;;" + sid + ")", "O:LSD:(A;;CC;;;" + sid + ")"} {
		if got := localUserSID([]string{scope}); got != sid {
			t.Fatalf("scope %q: %q", scope, got)
		}
	}
	for _, values := range [][]string{nil, {""}, {"Any"}, {"D:"}, {"D:(A;;CC;;;WD)"}, {"D:(D;;CC;;;" + sid + ")"}, {"D:(A;;CC;;;" + sid + ")(A;;CC;;;WD)"}, {"D:(A;;CC;;;" + sid + ")", "Any"}} {
		if got := localUserSID(values); got != "" {
			t.Fatalf("ambiguous scope accepted: %v", values)
		}
	}
}

func TestAdditionalBlockScopeNeverExcusesRequiredRuleMismatch(t *testing.T) {
	p := planFixture(t)
	now := time.Now()
	e := evidenceFixture(p, now)
	e.Rules = append(e.Rules, ObservedRule{Spec: RuleSpec{Name: "another-user-only", Enabled: true, Direction: "Inbound", Action: "Block", Program: "Any"}})
	if EvaluateAt(p, e, now).SnapshotMatches {
		t.Fatal("unproven block scope accepted")
	}
	disjoint := func(RuleSpec, []Backend) bool { return true }
	if r := evaluateAt(p, e, now, disjoint); !r.SnapshotMatches {
		t.Fatal(r.Blockers)
	}
	e.Rules[0].Spec.RemoteAddresses = []string{"192.0.2.1"}
	if evaluateAt(p, e, now, disjoint).SnapshotMatches {
		t.Fatal("required protection mismatch ignored")
	}
}
