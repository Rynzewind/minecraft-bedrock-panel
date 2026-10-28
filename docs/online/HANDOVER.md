# Online preparation build — operator handover

## Read this first

This is an implemented **preparation candidate**, not a working remote-access release. It must not be described as successfully broadcasting, enforcing LAN-first Friends joins, or admitting remote players. Production activation is unconditionally blocked. Nothing has been installed into the live deployment, published to GitHub/Drive/Desktop, forwarded on the router or opened in Windows Firewall.

The inspected live servers use `transport=nethernet`, `online-mode=true` and `allow-list=false`. Those settings were read only and preserved. A public RakNet Transfer destination is therefore not a verified drop-in replacement for the existing backend. Never forward the existing backend as a shortcut.

## Implemented

- Seventh top-level Online tab, with Overview, Authorised Players, Connectivity and Logs; original Servers remains the only BDS lifecycle controller.
- Per-server requested Online settings, broadcaster account assignment, world name, public host/ports, preferred interface and trusted subnet metadata. Existing configured backend addresses are derived, not replaced. Status follows existing supervisor readiness and remains Disabled/Waiting/Account missing/Authentication required/Blocked. No advertisement is inferred.
- Accounts-only Add broadcaster and account role controls; existing official device-code flow and DPAPI token persistence. Legacy accounts remain helpers; broadcaster/unassigned roles cannot be leased or reassigned as LAN helpers. Mapped/active and Online-assigned accounts cannot silently change roles or be removed. New panels require explicit role capability from the helper before broadcaster enrolment.
- Stable authenticated Xbox profile resolution with a short-lived, single-use explicit review before per-server policy grant. Arbitrary caller-supplied XUIDs cannot create grants. Policy defaults deny; Creative/Survival scopes and revocation predicates are tested. Friendship does not grant access.
- Private local Online settings/ACL file, atomic persistence and rollback; token-free bounded configuration/policy event log and local-interface diagnostics. These records do not claim live gateway enforcement or observable remote connections.
- Role-aware disk/portable metadata uses schema 2 when non-helper roles exist, so older helpers/installers reject it rather than misusing broadcasters as LAN helpers. Helper-only legacy data remains schema 1 compatible. Restored Online intent is disabled; tokens remain excluded from portable snapshots.
- Separate compiled `NetherNetOnline.exe` offline probe/admission diagnostic, pinned dependencies/licences and optional installer companion integrity handling. Root `go.mod`, `go.sum` and modified Gophertunnel are unchanged.

## Selected dependency and blockers

The isolated module pins [Go broadcaster bb06c2d](https://github.com/HashimTheArab/go-mcxboxbroadcast/tree/bb06c2ddc46fb61b2bbbb886f08d6edf11383a3e), with fork Gophertunnel f12973c6db70 and NetherNet 215e46422b58. Its compiled protocol is **2193 / 1.26.50**; this project uses **2225 / 1.26.60**. MIT notices and 49 dependency licence directories are retained in the component. The GPL Java original was investigated, not incorporated.

Direct Transfer has one static destination. NetherNet RemoteAddr/ICE candidates do not reliably establish physical locality, including NAT, TURN, shared public IP and VPN cases. Untrusted private addresses cannot supply a locality proof. A future reviewed per-client routing callback plus trusted fresh connection-bound evidence is required, or the routing requirement must be explicitly changed. This build never silently sends an unknown-origin Friends client to public IP and never marks LAN-first verified.

Native BDS allowlisting alone cannot separate LAN helper XUIDs from approved remote accounts, and existing backends use NetherNet. A protected authenticated ingress with no directly reachable bypass must be designed and verified while preserving `online-mode=true`. The upstream relay mode that requires weakened backend authentication is not enabled. Stopping an advertisement alone cannot close a manually forwarded backend port; this build does not claim otherwise.

## Accounts instructions

Use only matching candidate helper/panel versions. Open Accounts > Add broadcaster, give labels, follow the existing Microsoft device-code page and reauthenticate there when required. Use separate broadcaster identities for Creative and Survival. Do not change mapped family helper accounts into broadcasters; enrol dedicated accounts. No Microsoft password is requested. No new plaintext token cache or worker token broker is present.

## Online configuration and policy instructions

Open Online > Refresh. Select previously enrolled broadcaster accounts, enter world names and optional connectivity metadata, and Save. Enable Online Access stores intent only; it does not advertise or open an entry path. Every readiness/security blocker remains visible.

Under Authorised Players, enter the exact gamertag, select a healthy broadcaster for the authenticated profile lookup, choose Creative/Survival, and resolve. Verify returned gamertag/XUID before accepting. Existing resolved records can have either permission revoked or be removed. These are local managed policies only; remote enforcement/synchronisation remains unverified while ingress is blocked. Existing allowlists, console mappings and world data are not edited.

Connectivity displays derived local targets and real local interface addresses. Preferred interface/trusted subnet settings are metadata, not an origin or authorisation assertion. Router WAN/CGNAT, public endpoint reachability and Windows Firewall effective ingress rules remain unverified. No port-forwarding/firewall changes are performed or needed to inspect this candidate.

Logs show configuration/policy events only. A deferred minor: grant/update/removal events have a global server ID, so per-server log filters may hide them; All servers shows them. Events are bounded and retained in memory for the current helper run; no connection/transfer/session recovery events are invented.

## Incomplete requirements and future acceptance

Not implemented/verified: real Xbox session publication, protocol-2225 broadcaster compatibility, worker token/session broker, independent broadcaster process startup/recovery/backoff, protected public authenticated ingress, BDS/gateway allowlist synchronisation and prompt live disconnect on revocation, per-client direct-transfer route selection, authoritative locality proof, packet-path diagnostics, remote firewall enforcement/disablement, external reachability/CGNAT proof, or actual Friends-tab joins.

Acceptance testing must remain separate:

| Category | Result |
| --- | --- |
| Automated role, policy, identity-review, persistence, migration, service and native UI tests | Run; results in local build/test logs |
| Isolated worker tests/probe/forced-run denial | Run; compiled capabilities prove blockers, not compatibility |
| Candidate local LAN integration / packet capture | Not run |
| Candidate external internet / authorised and denied join tests | Not run |
| Physical Xbox / PS5 / Switch | Not run |
| Simultaneous local and remote play / actual LAN-first Friends route | Not run |

Only after compatibility and protected ingress are implemented should tests attempt approved Creative access, Creative-only denial on Survival, unauthorised direct-IP/Friends denial, revocation and Online disablement. A PS5 and Switch on LAN must be traced on the actual LAN path, including selecting the same Friends entry used externally. Advertisement success never substitutes for these tests.

## Rollback

The live deployment was not changed, so current rollback is simply to leave the candidate uninstalled. Source work is isolated on `codex/online-private-access`; the tested release remains on main/Drive/Desktop/GitHub.

If a future test installation creates non-helper accounts, do not merely replace binaries with an older helper: schema 2 deliberately prevents that unsafe rollback. Use the matching candidate Accounts/Online UI to unassign Online accounts, remove or deliberately re-role unmapped non-helper accounts, and verify helper-only schema 1 before legacy software is used, or restore the protected account-store backup taken before that test. Do not restore older world data or overwrite current worlds as part of an application rollback. Retain software/account backups and original identities.

## Significant files

- UI: `cmd/nethernet-multi-server-admin/online_windows.go`, `online_accounts_windows.go`, `online_windows_test.go`, existing window/viewmodel/theme integration.
- Composition/service: `cmd/nethernet-multi-server/main.go`, `internal/service/online.go`, `online_profile.go`, `online_test.go`, controller/admin API/migration-read method integration.
- Roles: `internal/accounts/roles.go`, enrolment/manager changes, `internal/accountstore/store.go` and role tests.
- Policy: `internal/online/manager.go`, Windows privacy implementation and tests.
- Migration: portable accounts/roles, inventory, restore and regression tests; PowerShell Archive-Operations and harness.
- Packaging: `components/online-broadcaster/**`, `scripts/build.ps1`, optional-companion installer/update scripts and integrity pins; `THIRD_PARTY_NOTICES.md`.

See `ARCHITECTURE.md` and the component README for source-backed constraints and exact probe/validation schema.
