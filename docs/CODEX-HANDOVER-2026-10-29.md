# Current verified status — 29 October 2026

Both Creative and Survival were successfully joined by the user from a Switch on a separate internet connection. Both servers are currently Ready; both public HTTP discovery ports return the correct world and protocol 2225. Backend protection evidence matches with no blockers. All four installed executables match tested release release-20261029-232439. The Drive single-EXE installer matches SHA256 9f3e2f45f97755b49fb85e18905c7ff11e56b20eea02ca34b833957e559b4034. Codex remains installed. Earlier pending-test statements below are historical and superseded.

Durable deliverables belong in G:\My Drive\Minecraft Server Setup; source and documentation are synchronized to GitHub branch codex/online-private-access. Only the panel shortcut remains on the Desktop. Required installed/runtime/build files remain on C:. Credentials, keys, worlds and private configuration must not enter GitHub. No known blocker remains for the reported visibility/joining issue; unauthorized-access and broader LAN regression tests were not repeated in this final check.

---
# NetherNet handover — 29 October 2026, Online repair

## Subsequent public metadata and Switch visibility repair

The two public ports were independently reachable but both returned the default Minecraft Server / Gophertunnel / Survival metadata. Gateway status now reports the correct world, game mode and configured capacity. The worker bootstrap also carries game mode/capacity, with canonical broadcaster world types.

Read-only Xbox diagnostics confirmed mutual social relationships, online Minecraft presence, one open joinable session per broadcaster, and matching protocol 2225 / version 1.26.60. The old sessions used connection type 3. Workers now explicitly select the supported Player Messaging / JSON-RPC path (type 7), instead of the pinned broadcaster's older WebSocket default. No friendship, invite, privacy, account-role or firewall changes were made for this update.

Both local and public pings now identify Creative on UDP 19140 and Survival on UDP 19141. Xbox session metadata now has the correct world types, capacity 10 and connection type 7. Both entries remain Advertising; both BDS servers remain Ready; protection still has no blockers.

The user confirmed that BOTH worlds now appear in Friends on a first-generation Switch running version 26.60 after fully closing and reopening Minecraft. Joining each world is the next physical acceptance test. Do not infer outside-network joining from local console visibility.

This update passed the full project and isolated worker suites, real authenticated gateway-ping tests, independent review, installer/wizard/corrupt-payload checks and Defender scan. All installed binaries match the tested release, and the verified latest installer has been published to Drive with its previous copy backed up. The prior guard-repair backup remains available at backups\migration-panel-20261029-215150-772; the newest binary rollback folder is listed below.

## Verified live status

Creative and Survival are running and Ready. Both Online entries report Advertising with Broadcasting=true. The effective protection report has SnapshotMatches=true and no blockers. Both gateway UDP listeners are bound. Codex remains installed; removing Codex is not required for this configuration.

External player joining remains unverified. RemoteVerified must remain false until an approved player actually joins from a separate internet connection. Advertisement success is not proof of router forwarding, outside reachability or player admission.

## Root causes and repair

The previous handover's Codex-only diagnosis was incomplete:

- Empty optional CIM array entries were mistaken for excluded firewall interfaces and narrowed rule conditions.
- Windows singleton address formatting differed from the generated range format.
- The additional-block check ignored LocalUser scope. The observed rule was scoped to a different user from the actual running backends.
- Old IPv6 privacy addresses remained in protection rules even though both backend targets use IPv4.

The repaired guard normalizes harmless empty optional entries and singleton ranges, while rejecting real interface exclusions, missing/narrowed rules, stale evidence and address drift. It proves an additional simple LocalUser SID disjoint using actual running backend tokens (user and all groups); unknown SDDL, unreadable/restricted tokens and missing processes stay blocking. It never relies on a rule name or product-specific exception.

Host-source exceptions now use the families of each program's configured backend targets, plus explicit loopback in both families. IPv4-only backends block other IPv6 sources, so IPv6 privacy-address rotation no longer changes their rules. Shared-program mixed-family targets retain both families. Full inventory freshness checks remain.

Only the two existing NetherNet-owned backend block-rule address filters were refreshed, and the installed read-only audit collector was updated. No Codex rules or firewall profiles were changed. Worlds, account stores and server identities were preserved by the existing application updater.

## Installation, source and rollback

- Live installation: C:\MinecraftServers; application: C:\MinecraftServers\MultiServer.
- Repository: C:\Users\kquin\Documents\Codex\2026-10-27\do-have-context-saved-in-our\repository\minecraft-bedrock-panel.
- Branch: codex/online-private-access. Base commit: 7fb07db; repair source changes are saved locally and not committed or pushed.
- Tested release package: repository outputs\release-20261029-221001 and matching ZIP.
- Previous binaries: C:\MinecraftServers\MultiServer\backups\migration-panel-20261029-221306-906.
- Previous NetherNet rule address filters and audit collector: C:\MinecraftServers\MultiServer\backups\onlineguard-repair-20261029-215218-571.
- Verified latest installer: G:\My Drive\Minecraft Server Setup\NetherNetSetup.exe. The previous Drive installer was backed up by the release workflow.
- Installer SHA-256: c26aa926eb98f5fe7c91d39dd86caf1e97e39a2ef486b3cbb28fa829ef718951.

Do not roll back worlds to roll back application binaries. Stop Online before any protection rollback. Keep the backend protection and online-mode=true; do not forward backend ports or disable Windows Firewall.

## Verification performed

- Regression failures reproduced before fixes; focused guard, token, service and runtime tests passed.
- Full project suite passed through scripts/build.ps1; isolated Online component tests passed.
- Independent review found no important issue.
- A stale GUI spacing test was corrected to measure the full action bar rather than a button that is no longer in its final row; the original spacing threshold was retained. No UI behavior was changed.
- Single-EXE extraction, payload checks, native setup wizard and corrupt-payload rejection passed.
- Microsoft Defender found no threats; real-time protection remained enabled.
- Published Drive installer hash matched the scanned build.
- All four installed executables matched the tested release manifest.
- After restart and approved rule refresh, both BDS servers were Ready and both Online entries were Advertising with no protection blockers.

## Next test (visibility now confirmed; joining pending)

Use an approved Xbox account on a genuinely separate internet connection, such as mobile data, and try each Friends entry. Record which world appears and whether joining succeeds. Also verify an unauthorized account is refused and existing LAN console access still works. Do not treat advertisement alone as an external-access pass.

Keep credentials, live account data, worlds and identity material out of Git and release artifacts. For subsequent application updates, follow AGENTS.md: build/test/Defender-scan the single EXE and publish a checksum-verified latest installer to the private Drive setup folder, retaining the prior copy.

## Current open issue: external joining

The user confirmed both Friends worlds are visible on a first-generation Switch running 26.60, but neither joins. The Switch is on a different internet connection from the server PC. It reaches "Connecting to external server" then reports "an error occurred" for both. This suggests transfer is attempted; the failing boundary is not yet proven.

Both BDS servers remain Ready, both broadcasters Advertising, and protection has no blockers. The independent status API now sees both public ports online with the correct distinct world names/modes. DNS has one IPv4 record and no IPv6 record. No new backend session-history event was recorded during the original failed attempts.

The unchanged installed helper was cleanly restarted once with private stdout/stderr capture to diagnose a fresh Survival retry. Log paths are recorded in the current chat workspace work\active-join-logs.json. No additional networking, firewall or authentication changes have been made. RemoteVerified remains false. Do not describe external joining as working.

## Gateway transport update — 29 October 2026

Replaced the RakNet-only public gameplay gateway with authenticated NetherNet HTTP/WebRTC transport. The existing Friends advertisements remain JSONRPC/type 7. Lightweight UDP pings remain available for status tools. Server identity keys persist in the configured identity directory. Signed client identity, per-world ACL checks and backend host-only protection remain enforced.

Installed tested release outputs/release-20261029-224816. Rollback binaries: C:\MinecraftServers\MultiServer\backups\migration-panel-20261029-225406-059. Two new helper-scoped inbound TCP rules for 19140/19141 were installed; existing UDP and backend blocks were retained. Inventory: C:\MinecraftServers\MultiServer\backups\onlinegateway-tcp-20261029-230017-365.

Verified both public HTTP /v1/join endpoints through quinnminecraft.ddns.net return the correct distinct world names and protocol 2225. Both worlds Advertising; protection SnapshotMatches true with no blockers. Actual external Switch join remains pending. Helper was restarted independently after deployment shell cleanup; work/active-join-logs.json references the earlier helper and is no longer the current stdout capture.

Full build/tests including actual network listener tests passed, review completed, single-EXE package tests and Defender scan passed. Installer retained locally in current chat outputs/nethernet-gateway-20261029/NetherNetSetup.exe. SHA256 f2fb9c601ec4376b72864939f141908671d765f7db46cc3ad25dc437dd32d3f7. Drive publication was rejected by automatic approval review because this specific external synced destination requires explicit user authorization. Do not retry until authorized.

## External Survival join confirmed

The user confirmed from the first-generation Switch on a different internet connection: the trust prompt appeared, add-ons downloaded, and Survival loaded successfully. This verifies the actual authenticated NetherNet transfer and gameplay path for Survival. Creative external joining still needs its own user test; do not claim it passed yet. Codex remains installed.

The user explicitly authorized Drive publication. The tested installer was successfully published to G:\My Drive\Minecraft Server Setup\NetherNetSetup.exe with checksum verification and the previous installer backed up. The earlier publication approval blocker is resolved.

## Resolution confirmed — both worlds

The user also confirmed Creative loads the correct world from the external Switch Friends entry. Both Survival and Creative external joining are now user-verified, including Survival's trust prompt and add-on download. This resolves the reported Friends visibility and external joining issues while retaining Codex. Historical pending-test statements above are superseded by these confirmed results. The tested installer is published to the approved Drive destination and the previous installer retained as a backup.

## Ongoing workflow requested by user

Maintain the current handover at G:\My Drive\Minecraft Server Setup\CODEX-HANDOVER.md alongside the latest tested installer. Update both repository documentation and Drive handover after developments. Keep GitHub current by committing validated changes and pushing the working branch, codex/online-private-access. Keep installers off the Desktop; preserve the application shortcut. AGENTS.md records these standing requirements.



## Account list update — 29 October 2026

Accounts now has separate Helpers and Broadcasters sections. Helpers are expanded and Broadcasters collapsed whenever the panel opens. Both sections can be toggled; refresh preserves their open/closed state. The shared edit/reauthenticate/remove/role/Online access actions use the selected row's stable account ID in either section. Only helpers can be assigned to LAN players. Non-helper accounts, including explicitly unassigned accounts, are in the second section with their role visible.

The top status now reports non-helper accounts and helpers separately, followed by active players. Regression checks cover role partition/counts, default expansion, selection across sections, and collapsed selection clearing. The complete build and native UI suite passed. Installer extraction, integrity, native wizard and corrupt-payload checks passed, and Defender found no threats with protection still enabled.

Release: release-20261029-232439. Drive installer SHA256: 9f3e2f45f97755b49fb85e18905c7ff11e56b20eea02ca34b833957e559b4034. Updated the installed panel only; the unchanged helper, broadcaster worker and control binaries already match this release, so neither world needed a restart. Previous panel and installer are retained in Drive Archive. User visual acceptance of this UI update remains pending; both external world joins were confirmed before this UI-only update.
