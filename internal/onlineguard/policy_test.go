package onlineguard

import (
	"strings"
	"testing"
	"time"
)

func planFixture(t *testing.T) Plan {
	t.Helper()
	p, e := BuildPlan([]Backend{{ServerID: "creative", Program: `C:\NetherNet\Creative\bedrock_server.exe`, Target: "192.168.1.246:19134"}}, []string{"192.168.1.137", "192.168.1.246", "::1", "127.0.0.1"})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func evidenceFixture(p Plan, now time.Time) Evidence {
	rules := []ObservedRule{}
	for _, r := range p.Rules {
		rules = append(rules, ObservedRule{Spec: r, EnforcementStatus: "Full", PrimaryStatus: "OK"})
	}
	return Evidence{Platform: "windows", Complete: true, ObservedAt: now, FirewallServiceRunning: true, BaseFilteringEngineRunning: true, LocalAddresses: append([]string(nil), p.LocalAddresses...), Profiles: []Profile{{Name: "Domain", Enabled: true, DefaultInboundAction: "Block", AllowInboundRules: true}, {Name: "Private", Enabled: true, DefaultInboundAction: "Block", AllowInboundRules: true}, {Name: "Public", Enabled: true, DefaultInboundAction: "Block", AllowInboundRules: true}}, Rules: rules}
}
func TestPlanExcludesOnlyHostAddressesAcrossBothFamilies(t *testing.T) {
	p := planFixture(t)
	if len(p.Rules) != 1 {
		t.Fatal(p)
	}
	r := p.Rules[0]
	if r.Action != "Block" || r.Protocol != "Any" || r.Direction != "Inbound" || !r.Enabled || strings.Join(r.Profiles, ",") != "Domain,Private,Public" {
		t.Fatal(r)
	}
	for _, s := range []string{"8.8.8.8", "192.168.1.138", "192.168.1.1", "2606:4700::1"} {
		if !RemoteBlocked(r, s) {
			t.Fatal("external source unblocked", s)
		}
	}
	for _, s := range p.LocalAddresses {
		if RemoteBlocked(r, s) {
			t.Fatal("host source blocked", s)
		}
	}
	q, e := BuildPlan(p.Backends, []string{"127.0.0.1", "192.168.1.246", "192.168.1.137", "::1"})
	if e != nil || p.ID != q.ID {
		t.Fatal("nondeterministic", e)
	}
}
func TestInvalidPlanInputs(t *testing.T) {
	for _, program := range []string{"bedrock_server.exe", `\\server\share\bedrock_server.exe`, `C:\*\bedrock_server.exe`, `%USERPROFILE%\bedrock_server.exe`, `C:\a\..\bedrock_server.exe`} {
		_, e := BuildPlan([]Backend{{ServerID: "a", Program: program, Target: "192.168.1.246:19134"}}, []string{"192.168.1.246"})
		if e == nil {
			t.Fatal("invalid program", program)
		}
	}
	for _, target := range []string{"0.0.0.0:19134", "8.8.8.8:19134", "localhost:19134", "192.168.1.246:0"} {
		_, e := BuildPlan([]Backend{{ServerID: "a", Program: `C:\x\bedrock_server.exe`, Target: target}}, []string{"192.168.1.246"})
		if e == nil {
			t.Fatal("invalid target", target)
		}
	}
	for _, addr := range []string{"LocalSubnet", "192.168.1.0/24", "0.0.0.0", "ff02::1", "fe80::1%12"} {
		_, e := BuildPlan(planFixture(t).Backends, []string{addr})
		if e == nil {
			t.Fatal("invalid localaddress", addr)
		}
	}
}
func TestEvidenceFailsClosed(t *testing.T) {
	p := planFixture(t)
	now := time.Now()
	good := evidenceFixture(p, now)
	if r := EvaluateAt(p, good, now); !r.SnapshotMatches {
		t.Fatal(r)
	}
	cases := map[string]func(*Evidence){"incomplete": func(e *Evidence) { e.Complete = false }, "stale": func(e *Evidence) { e.ObservedAt = now.Add(-3 * time.Minute) }, "disabled profile": func(e *Evidence) { e.Profiles[2].Enabled = false }, "missing profile": func(e *Evidence) { e.Profiles = e.Profiles[:2] }, "service off": func(e *Evidence) { e.FirewallServiceRunning = false }, "missing rule": func(e *Evidence) { e.Rules = nil }, "wrong program": func(e *Evidence) { e.Rules[0].Spec.Program = `C:\other.exe` }, "local subnet": func(e *Evidence) { e.Rules[0].Spec.RemoteAddresses = []string{"LocalSubnet"} }, "only UDP": func(e *Evidence) { e.Rules[0].Spec.Protocol = "UDP" }, "disabled rule": func(e *Evidence) { e.Rules[0].Spec.Enabled = false }, "address drift": func(e *Evidence) { e.LocalAddresses = append(e.LocalAddresses, "192.168.1.200") }, "bypass": func(e *Evidence) {
		e.Rules = append(e.Rules, ObservedRule{Spec: RuleSpec{Name: "bypass", Enabled: true, Direction: "Inbound", Action: "Allow", OverrideBlockRules: true}})
	}}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := evidenceFixture(p, now)
			mutate(&e)
			if EvaluateAt(p, e, now).SnapshotMatches {
				t.Fatal("accepted unsafe evidence")
			}
		})
	}
}
func TestBroadAllowCannotOverrideExplicitBlockButBroadBlockBreaksLAN(t *testing.T) {
	p := planFixture(t)
	now := time.Now()
	e := evidenceFixture(p, now)
	e.Rules = append(e.Rules, ObservedRule{Spec: RuleSpec{Name: "old-any-allow", Program: "Any", Enabled: true, Direction: "Inbound", Action: "Allow"}})
	if !EvaluateAt(p, e, now).SnapshotMatches {
		t.Fatal("ordinary allow overrode explicit block")
	}
	e.Rules = append(e.Rules, ObservedRule{Spec: RuleSpec{Name: "old-block", Program: "Any", Enabled: true, Direction: "Inbound", Action: "Block"}})
	if EvaluateAt(p, e, now).SnapshotMatches {
		t.Fatal("broad block incorrectly preserves helpers")
	}
}
func TestEvidenceAdditionalConditionsAndTampering(t *testing.T) {
	p := planFixture(t)
	now := time.Now()
	cases := map[string]func(*Evidence){"security narrowed": func(e *Evidence) { e.Rules[0].Spec.RemoteUsers = []string{"user"} }, "platform narrowed": func(e *Evidence) { e.Rules[0].Spec.Platforms = []string{"WindowsServer"} }, "service narrowed": func(e *Evidence) { e.Rules[0].Spec.Service = "SomeService" }, "dynamic transport": func(e *Evidence) { e.Rules[0].Spec.DynamicTarget = "ProximityApps" }, "mapping": func(e *Evidence) { e.Rules[0].Spec.LocalOnlyMapping = true }, "duplicate rule": func(e *Evidence) { e.Rules = append(e.Rules, e.Rules[0]) }, "not enforced": func(e *Evidence) { e.Rules[0].EnforcementStatus = "LocalFirewallRulesDisallowed" }, "bfe off": func(e *Evidence) { e.BaseFilteringEngineRunning = false }, "interface excluded": func(e *Evidence) { e.Profiles[0].DisabledInterfaceAliases = []string{"Ethernet"} }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			e := evidenceFixture(p, now)
			change(&e)
			if EvaluateAt(p, e, now).SnapshotMatches {
				t.Fatal("unsafe condition accepted")
			}
		})
	}
	e := evidenceFixture(p, now)
	q := p
	q.Rules = append([]RuleSpec(nil), p.Rules...)
	q.Rules[0].RemoteAddresses = []string{"Any"}
	if EvaluateAt(q, e, now).SnapshotMatches {
		t.Fatal("tampered plan accepted")
	}
}
func TestDefaultProfileActionCannotOverrideExplicitBlocks(t *testing.T) {
	p := planFixture(t)
	now := time.Now()
	for _, action := range []string{"Block", "Allow", "NotConfigured"} {
		e := evidenceFixture(p, now)
		for i := range e.Profiles {
			e.Profiles[i].DefaultInboundAction = action
		}
		if !EvaluateAt(p, e, now).SnapshotMatches {
			t.Fatal("default action incorrectly overrode explicit block", action)
		}
	}
}
