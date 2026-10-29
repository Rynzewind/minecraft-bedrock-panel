// Package onlineguard prepares and inspects host-only BDS ingress policy.
// It never applies firewall rules, changes networking, or authorizes activation.
package onlineguard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const RuleGroup = "NetherNet-OnlineGuard-v1"
const MaxEvidenceAge = 2 * time.Minute

// Backend describes the existing BDS executable and helper control endpoint.
type Backend struct {
	ServerID string `json:"server_id"`
	Program  string `json:"program"`
	Target   string `json:"target"`
}
type Plan struct {
	ID             string     `json:"id"`
	Backends       []Backend  `json:"backends"`
	LocalAddresses []string   `json:"local_addresses"`
	Rules          []RuleSpec `json:"rules"`
}

// RuleSpec contains every condition permitted on the planned block rule. Any
// extra condition could narrow the block and is therefore a mismatch.
type RuleSpec struct {
	DynamicTarget, Owner, PolicyAppID                                                                                                                        string
	LooseSourceMapping, LocalOnlyMapping                                                                                                                     bool
	ICMPTypes, RemoteDynamicKeywordAddresses                                                                                                                 []string
	Name, Group, Program, Direction, Action, Protocol, Service, Package, Authentication, Encryption, EdgeTraversalPolicy                                     string
	Enabled, OverrideBlockRules                                                                                                                              bool
	Profiles, LocalAddresses, RemoteAddresses, LocalPorts, RemotePorts, InterfaceAliases, InterfaceTypes, LocalUsers, RemoteUsers, RemoteMachines, Platforms []string
}
type Profile struct {
	Name                     string
	Enabled                  bool
	DefaultInboundAction     string
	AllowInboundRules        bool
	DisabledInterfaceAliases []string
}
type ObservedRule struct {
	Spec                             RuleSpec
	PrimaryStatus, EnforcementStatus string
}
type Evidence struct {
	Platform                                           string
	Complete                                           bool
	ObservedAt                                         time.Time
	FirewallServiceRunning, BaseFilteringEngineRunning bool
	LocalAddresses                                     []string
	Profiles                                           []Profile
	Rules                                              []ObservedRule
}

// SnapshotMatches establishes only a conservative comparison against an
// ActiveStore snapshot. It does not prove packet enforcement or host identity.
type Report struct {
	PlanID                string
	SnapshotMatches       bool
	Blockers, Limitations []string
}

func safe(s string) bool {
	if strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func canonicalProgram(p string) (string, error) {
	if len(p) < 4 || len(p) > 260 || !safe(p) || p[1] != ':' || p[2] != '\\' || !(p[0] >= 'A' && p[0] <= 'Z' || p[0] >= 'a' && p[0] <= 'z') || strings.ContainsAny(p[3:], `*?%:"<>|/`) {
		return "", errors.New("backend program must be a literal absolute local Windows path")
	}
	for _, part := range strings.Split(p[3:], `\`) {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, " .") != part {
			return "", errors.New("backend path contains ambiguous component")
		}
	}
	if !strings.EqualFold(p[strings.LastIndex(p, `\`)+1:], "bedrock_server.exe") {
		return "", errors.New("backend program must identify bedrock_server.exe")
	}
	return strings.ToLower(p), nil
}
func addresses(values []string) ([]string, error) {
	seen := map[netip.Addr]bool{netip.MustParseAddr("127.0.0.1"): true, netip.IPv6Loopback(): true}
	if len(values) == 0 || len(values) > 256 {
		return nil, errors.New("complete bounded host address inventory required")
	}
	for _, v := range values {
		a, e := netip.ParseAddr(v)
		if e != nil || a.Zone() != "" || a.Is4In6() || a.IsUnspecified() || a.IsMulticast() || a.String() != v {
			return nil, errors.New("host inventory requires canonical unicast IP addresses without zones or subnets")
		}
		seen[a] = true
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a.String())
	}
	sort.Slice(out, func(i, j int) bool { return netip.MustParseAddr(out[i]).Less(netip.MustParseAddr(out[j])) })
	return out, nil
}
func complement(local []string) []string {
	out := []string{}
	for _, is4 := range []bool{true, false} {
		cursor := netip.IPv6Unspecified()
		last := netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
		if is4 {
			cursor = netip.IPv4Unspecified()
			last = netip.MustParseAddr("255.255.255.255")
		}
		for _, v := range local {
			a := netip.MustParseAddr(v)
			if a.Is4() != is4 {
				continue
			}
			if cursor.Less(a) {
				out = append(out, cursor.String()+"-"+a.Prev().String())
			}
			cursor = a.Next()
		}
		if cursor.IsValid() && cursor.Compare(last) <= 0 {
			out = append(out, cursor.String()+"-"+last.String())
		}
	}
	return out
}
func BuildPlan(backends []Backend, localAddresses []string) (Plan, error) {
	local, e := addresses(localAddresses)
	if e != nil {
		return Plan{}, e
	}
	if len(backends) == 0 || len(backends) > 64 {
		return Plan{}, errors.New("one through 64 backend descriptors required")
	}
	p := Plan{LocalAddresses: local}
	ids := map[string]bool{}
	programs := map[string]bool{}
	for _, b := range backends {
		if b.ServerID == "" || len(b.ServerID) > 128 || !safe(b.ServerID) || ids[b.ServerID] {
			return Plan{}, errors.New("invalid or duplicate backend ID")
		}
		ids[b.ServerID] = true
		b.Program, e = canonicalProgram(b.Program)
		if e != nil {
			return Plan{}, e
		}
		host, port, e := net.SplitHostPort(b.Target)
		n, pe := strconv.Atoi(port)
		if e != nil || pe != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return Plan{}, errors.New("backend target requires literal local IP and canonical valid port")
		}
		a, ae := netip.ParseAddr(host)
		if ae != nil || a.Zone() != "" || a.Is4In6() || a.String() != host {
			return Plan{}, errors.New("invalid backend target IP")
		}
		found := false
		for _, v := range local {
			if v == host {
				found = true
			}
		}
		if !found {
			return Plan{}, errors.New("backend target is not assigned to this host")
		}
		p.Backends = append(p.Backends, b)
		if programs[b.Program] {
			continue
		}
		programs[b.Program] = true
		h := sha256.Sum256([]byte(b.Program))
		p.Rules = append(p.Rules, RuleSpec{DynamicTarget: "Any", ICMPTypes: []string{"Any"}, Name: "NetherNet-OnlineGuard-" + hex.EncodeToString(h[:8]), Group: RuleGroup, Program: b.Program, Direction: "Inbound", Action: "Block", Protocol: "Any", Service: "Any", Package: "Any", Authentication: "NotRequired", Encryption: "NotRequired", EdgeTraversalPolicy: "Block", Enabled: true, Profiles: []string{"Domain", "Private", "Public"}, LocalAddresses: []string{"Any"}, RemoteAddresses: complement(local), LocalPorts: []string{"Any"}, RemotePorts: []string{"Any"}, InterfaceAliases: []string{"Any"}, InterfaceTypes: []string{"Any"}, LocalUsers: []string{"Any"}, RemoteUsers: []string{"Any"}, RemoteMachines: []string{"Any"}, Platforms: []string{}})
	}
	// Exempt host sources only in address families actually used by this
	// program's backend endpoints. IPv4-only helpers need no global/link-local
	// IPv6 exceptions: privacy-address rotation must not invalidate their rules.
	// Keep explicit loopback in both families, and combine shared-program targets.
	for i := range p.Rules {
		v4, v6 := false, false
		for _, b := range p.Backends {
			if b.Program != p.Rules[i].Program {
				continue
			}
			host, _, _ := net.SplitHostPort(b.Target)
			if netip.MustParseAddr(host).Is4() {
				v4 = true
			} else {
				v6 = true
			}
		}
		var allowed []string
		for _, value := range local {
			a := netip.MustParseAddr(value)
			if a.IsLoopback() || (a.Is4() && v4) || (!a.Is4() && v6) {
				allowed = append(allowed, value)
			}
		}
		p.Rules[i].RemoteAddresses = complement(allowed)
	}
	sort.Slice(p.Backends, func(i, j int) bool { return p.Backends[i].ServerID < p.Backends[j].ServerID })
	sort.Slice(p.Rules, func(i, j int) bool { return p.Rules[i].Name < p.Rules[j].Name })
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	p.ID = hex.EncodeToString(sum[:])
	return p, nil
}

// RemoteBlocked checks the address ranges in a generated rule, for plan review.
func RemoteBlocked(r RuleSpec, remote string) bool {
	a, e := netip.ParseAddr(remote)
	if e != nil {
		return false
	}
	for _, v := range r.RemoteAddresses {
		lo, hi, ok := strings.Cut(v, "-")
		if !ok {
			continue
		}
		l, le := netip.ParseAddr(lo)
		h, he := netip.ParseAddr(hi)
		if le == nil && he == nil && a.Is4() == l.Is4() && a.Compare(l) >= 0 && a.Compare(h) <= 0 {
			return true
		}
	}
	return false
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
func equalRule(a, b RuleSpec) bool {
	a.ICMPTypes = sorted(a.ICMPTypes)
	b.ICMPTypes = sorted(b.ICMPTypes)
	a.RemoteDynamicKeywordAddresses = sorted(nonempty(a.RemoteDynamicKeywordAddresses))
	b.RemoteDynamicKeywordAddresses = sorted(nonempty(b.RemoteDynamicKeywordAddresses))
	a.Profiles = sorted(a.Profiles)
	b.Profiles = sorted(b.Profiles)
	a.LocalAddresses = sorted(a.LocalAddresses)
	b.LocalAddresses = sorted(b.LocalAddresses)
	a.RemoteAddresses = normalizedRanges(a.RemoteAddresses)
	b.RemoteAddresses = normalizedRanges(b.RemoteAddresses)
	a.LocalPorts = sorted(a.LocalPorts)
	b.LocalPorts = sorted(b.LocalPorts)
	a.RemotePorts = sorted(a.RemotePorts)
	b.RemotePorts = sorted(b.RemotePorts)
	a.InterfaceAliases = sorted(a.InterfaceAliases)
	b.InterfaceAliases = sorted(b.InterfaceAliases)
	a.InterfaceTypes = sorted(a.InterfaceTypes)
	b.InterfaceTypes = sorted(b.InterfaceTypes)
	a.LocalUsers = sorted(a.LocalUsers)
	b.LocalUsers = sorted(b.LocalUsers)
	a.RemoteUsers = sorted(a.RemoteUsers)
	b.RemoteUsers = sorted(b.RemoteUsers)
	a.RemoteMachines = sorted(a.RemoteMachines)
	b.RemoteMachines = sorted(b.RemoteMachines)
	a.Platforms = sorted(nonempty(a.Platforms))
	b.Platforms = sorted(nonempty(b.Platforms))
	a.Program = strings.ToLower(a.Program)
	b.Program = strings.ToLower(b.Program)
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func sorted(v []string) []string { o := append([]string{}, v...); sort.Strings(o); return o }

// Windows serializes absent optional CIM fields as either [] or [""].
func nonempty(values []string) []string {
	var out []string
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Windows renders singleton address ranges as individual addresses.
func normalizedRanges(values []string) []string {
	out := append([]string(nil), values...)
	for i, v := range out {
		if a, err := netip.ParseAddr(v); err == nil && a.Zone() == "" {
			out[i] = a.String() + "-" + a.String()
		}
	}
	return sorted(out)
}
func Evaluate(p Plan, e Evidence) Report {
	return evaluateAt(p, e, time.Now().UTC(), unrelatedLocalUserBlock)
}
func EvaluateAt(p Plan, e Evidence, now time.Time) Report {
	return evaluateAt(p, e, now, nil)
}
func evaluateAt(p Plan, e Evidence, now time.Time, disjoint func(RuleSpec, []Backend) bool) Report {
	r := Report{PlanID: p.ID, Limitations: []string{"Snapshot comparison is not a packet-enforcement test or permission to activate Online.", "Host IP addresses are not proof of helper process identity or physical LAN locality; a local relay or source spoofing requires separate controls.", "Local helper connectivity and external IPv4/IPv6 direct-BDS denial require supervised positive and negative tests before activation.", "Address, executable, firewall policy and service changes invalidate this snapshot; continuous monitoring and stop-before-drift handling are required.", "Firewall does not replace authenticated per-server remote gateway ACL enforcement; router/NAT and other WFP providers are not verified."}}
	block := func(s string) { r.Blockers = append(r.Blockers, s) }
	rebuilt, err := BuildPlan(p.Backends, p.LocalAddresses)
	if err != nil || rebuilt.ID != p.ID || len(rebuilt.Rules) != len(p.Rules) {
		block("Protection plan is invalid or was modified.")
		return r
	}
	for i := range p.Rules {
		if !equalRule(p.Rules[i], rebuilt.Rules[i]) {
			block("Protection rule plan was modified.")
			return r
		}
	}
	if e.Platform != "windows" || !e.Complete {
		block("Complete Windows ActiveStore evidence is unavailable.")
	}
	if e.ObservedAt.IsZero() || now.Sub(e.ObservedAt) > MaxEvidenceAge || e.ObservedAt.After(now.Add(5*time.Second)) {
		block("Firewall evidence is stale or has an invalid timestamp.")
	}
	if !e.FirewallServiceRunning || !e.BaseFilteringEngineRunning {
		block("Windows Firewall and Base Filtering Engine must both be running.")
	}
	local, err := addresses(e.LocalAddresses)
	if err != nil || !equalStrings(local, p.LocalAddresses) {
		block("Host addresses changed or address inventory is invalid; rebuild policy before activation.")
	}
	profiles := map[string]bool{}
	for _, profile := range e.Profiles {
		if profiles[profile.Name] {
			block("Duplicate firewall profile evidence.")
		}
		profiles[profile.Name] = true
		if !profile.Enabled || !profile.AllowInboundRules || len(nonempty(profile.DisabledInterfaceAliases)) != 0 {
			block("Firewall profile is disabled, disallows inbound rules or bypasses interfaces: " + profile.Name)
		}
	}
	if len(profiles) != 3 || !profiles["Domain"] || !profiles["Private"] || !profiles["Public"] {
		block("All three effective firewall profiles must be present.")
	}
	observed := map[string][]ObservedRule{}
	for _, rule := range e.Rules {
		observed[rule.Spec.Name] = append(observed[rule.Spec.Name], rule)
		if !rule.Spec.Enabled || rule.Spec.Direction != "Inbound" {
			continue
		}
		if rule.Spec.OverrideBlockRules {
			block("An enabled authenticated bypass rule can override explicit blocks.")
		}
		owned := false
		for _, want := range p.Rules {
			if rule.Spec.Name == want.Name {
				owned = true
			}
		}
		if !owned && rule.Spec.Action == "Block" {
			if disjoint != nil && disjoint(rule.Spec, p.Backends) {
				continue
			}
			for _, b := range p.Backends {
				if rule.Spec.Program == "" || strings.EqualFold(rule.Spec.Program, "Any") || strings.EqualFold(rule.Spec.Program, b.Program) {
					block("An additional inbound block may break local helper access; manual conflict review is required.")
					break
				}
			}
		}
	}
	for _, want := range p.Rules {
		found := observed[want.Name]
		if len(found) != 1 {
			block("A required effective protection rule is missing or duplicated: " + want.Name)
			continue
		}
		got := found[0]
		if !equalRule(got.Spec, want) || got.PrimaryStatus != "OK" || !enforced(got.EnforcementStatus) {
			block("A required protection rule differs or is not fully enforced: " + want.Name)
		}
	}
	r.SnapshotMatches = len(r.Blockers) == 0
	return r
}

// Windows reports an enabled rule as either Full or a comma-separated set
// including Enforced; ProfileInactive merely means another profile is not
// currently selected and is not a loss of enforcement for an Any-profile rule.
func enforced(status string) bool {
	if status == "Full" {
		return true
	}
	for _, value := range strings.Split(status, ",") {
		if strings.TrimSpace(value) == "Enforced" {
			return true
		}
	}
	return false
}
