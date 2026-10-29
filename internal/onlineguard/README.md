# Backend ingress protection plan (review only)

This package does not install rules or change network settings. `BuildPlan` returns
literal, deterministic rule specifications; `Inspect` reads the effective Windows
Firewall ActiveStore; `Evaluate` compares that evidence to the plan. A true
`Report.SnapshotMatches` is usable as a runtime policy gate. It is not a successful
external connection test and must not set the product's `RemoteVerified` flag.

Threat model: the local Windows machine, its administrator and its local
processes are trusted. Internet clients, other LAN clients and arbitrary existing
ordinary firewall allow rules are not trusted. A Windows administrator can always
replace the binary or disable filtering and is outside this guard's boundary.

## Rules to review before any application

For each distinct configured BDS executable, generate one named inbound **Block**
rule, enabled on Domain, Private and Public, for **Any** protocol, local/remote
ports, local address, interface, service and package. Its remote addresses are
exact IPv4 and IPv6 ranges covering every address **except** the current host's
own addresses in the families used by that program's configured backend targets,
plus explicit `127.0.0.1` / `::1`. An IPv4-only backend has no global or link-local
IPv6 source exceptions, so IPv6 privacy-address rotation cannot change its rule.
Programs shared by IPv4 and IPv6 backends retain both host-source families.
These are host addresses, never
`LocalSubnet`, the LAN prefix, router, NAT address or a claimed client address.
The spec's program path is a literal absolute local `bedrock_server.exe` path.
`Target` must be the existing configured host-owned helper endpoint, including
its real port. No port or LAN IP is hardcoded.

Do not replace this with an all-address block plus a host allow: explicit blocks
win, so the allow would not restore helper access. Ordinary existing broad allow
rules cannot override the generated disjoint blocks. Authenticated IPsec bypass
rules **can** override explicit blocks, so the evaluator rejects their presence.
Additional blocks matching BDS or any program require manual conflict review,
unless their sole simple LocalUser SID is proven absent from every running
configured backend's user and group token SIDs. Missing processes, unreadable or
restricted tokens and complex/unknown user scopes remain blocking. This checks
actual backend tokens, never rule names or the helper's assumed identity; it
does not change or bypass other applications' firewall rules.

Empty optional CIM array entries and Windows singleton-address formatting are
normalized during comparison. Actual interface exclusions, altered address
coverage and narrowed protection conditions still fail closed.

## Concrete approval-ready rollout

1. Keep the gateway stopped. Collect complete host address and effective policy
   evidence as an administrator; export existing policy and relevant rule/filter
   records for rollback. Verify configured BDS executable paths and
   `online-mode=true` separately. Include both server processes and IPv6.
2. Build and display the exact generated plan/ranges. Review DHCP/privacy-address
   churn and any existing block or authenticated-bypass conflicts. Preserve
   existing LAN proxy, discovery and helper rules for their own executables.
3. With explicit approval, install or replace **only** the named generated rules
   in `NetherNet-OnlineGuard-v1`, using every `RuleSpec` condition. This package
   intentionally contains no apply API. Do not delete unrelated allow rules.
4. Re-read ActiveStore, including all rule filters, effective profiles, MpsSvc,
   BFE and host addresses. Require `SnapshotMatches=true`; do not infer success
   from a rule name or from a successful New-NetFirewallRule command.
5. Test a real LAN console through the unchanged root proxy and local helper.
   Test direct remote BDS TCP/UDP/NetherNet failure over IPv4 and IPv6, including
   an intentionally forwarded backend port. Then test authorized gateway success
   and unauthorized gateway refusal separately. Keep external verification false
   until the corresponding actual test succeeds.
6. Gate gateway operation on fresh matching evidence and re-inspect continuously.
   Address, binary-path, firewall service/profile/policy changes invalidate the
   snapshot. Reconcile while access is stopped; if protecting BDS through a
   planned address removal, stop BDS before removing the address, because an old
   excluded source address must not become another machine's accepted source.
7. Rollback only the generated named/grouped rules from the reviewed pre-change
   inventory. A rollback removes this protection; gateway operation must stop.

## Boundaries and fail-closed behavior

The evaluator rejects missing/duplicate/mismatched rules, narrowed program,
protocol, port/address/interface/security/platform conditions, partial or stale
inspection, disabled filtering services/profiles, disabled interface filtering,
unknown enforcement status, authenticated bypass, and changed host addresses.

Windows Firewall is an IP/process firewall, not an Xbox authorization service.
Host-source exclusion preserves local helpers but does not identify which trusted
local process sent a packet. Remote source spoofing cannot complete a normal
TCP or authenticated WebRTC handshake when replies route to the host itself;
a trusted local relay is outside this guard's threat model. This is not proof of
physical client locality and cannot replace gateway XUID ACL enforcement.

An unforeseen DHCP/address removal can leave an old excluded address until
reconciliation; enabling a new firewall rule does not make address churn atomic.
A stable backend/helper address arrangement is preferable. Other WFP providers,
NAT/hairpin behavior, router policy, unsolicited versus established-flow handling
and local loopback behavior require actual packet tests. A matching snapshot
supports a practical runtime policy decision; it does not claim these tests ran.

Microsoft documentation:
- [Rule precedence](https://learn.microsoft.com/windows/security/operating-system-security/network-security/windows-firewall/rules)
- [Authenticated bypass](https://learn.microsoft.com/powershell/module/netsecurity/new-netfirewallrule)
- [Effective enforcement status](https://learn.microsoft.com/windows/win32/fwp/wmi/wfascimprov/msft-netfirewallrule)
