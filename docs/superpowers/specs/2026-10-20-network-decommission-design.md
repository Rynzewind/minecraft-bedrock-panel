# NetherNet installation, network repair and decommissioning design

Status: proposed design for review; no application changes or live network changes have been made in this phase.

## Goal and constraints

Move the complete existing installation to a replacement Windows PC. The installer asks for the network adapter and IP inputs, applies an approved network plan, synchronises the helper and both servers, and verifies actual endpoints. After destination validation, a standalone local utility retires the source safely. Preserve worlds, packs, identities, account mappings, settings, backup configuration and the existing Xbox compatibility fixes. Machine-bound Windows authentication tokens still require destination sign-in where they cannot be securely migrated.

## Inspection findings

Source reviewed: work/migration-source, branch migration-installer, commit 136eac4. Relevant code includes internal/migration/restore.go, internal/setup, the setup/admin Windows UI, helper startup, the readable elevated configuration script, and legacy boot-task scripts.

The existing snapshot implementation stops managed servers cleanly, retains their previous running state, includes worlds and LevelDB transaction logs, hashes and verifies payloads, preserves persistent identities, and gates advertising after restore. Keep these mechanisms.

The current restore rewrites helper backends to the primary LAN IP but leaves server.properties unchanged. The setup UI offers an interface but no per-server IP inputs. Windows configuration adds UDP program firewall permissions but lacks explicit NetherNet TCP permissions. It does not assign IP aliases, persist a resource ownership manifest, provide network repair, or provide comprehensive decommissioning.

Read-only inspection on 20 October 2026 confirmed:

| Item | Observed value |
|---|---|
| Adapter | Ethernet 2, interface index 41 |
| Primary IPv4 | 192.168.1.144/24, SkipAsSource false |
| Creative | 192.168.1.246:19134, UDP 49152–49200 |
| Survival | 192.168.1.247:19136, UDP 49201–49249 |
| Alias state | Both present, SkipAsSource true |
| DHCP | Disabled on the selected adapter |
| Gateway | 192.168.1.1 |
| Helper configuration | interface .144; broadcast .255:7551 |
| Processes | Both Bedrock processes, helper and admin running under verified C:\MinecraftServers paths |
| TCP endpoints | Creative .246:19134 and Survival .247:19136 listening, owned by their respective processes |
| Discovery port | Helper IPv4 wildcard UDP 7551; Survival also has an IPv6 UDP 7551 endpoint |
| Legacy startup | A current-user Startup shortcut targets the NetherNet admin application |

No matching scheduled tasks or services were returned by the available inspection. This is a discovery result, not proof that all users and protected locations have been exhaustively inspected. Firewall inventory returned access denied and must remain an incomplete check until run with an actually elevated Windows token. Sandbox approval alone does not grant Windows administrator rights.

Legacy scripts register the task `NetherNet Minecraft Servers`; new setup uses a root-derived `NetherNet-Migration-<hash>` name/group. Names alone are insufficient proof of ownership: actions, executable paths and installation identity must match.

## Approaches considered

1. Extend independent PowerShell scripts only. Lowest initial change, but installer and repair can diverge and rollback/ownership rules become duplicated.
2. Shared Go planning/validation and journalling, with readable elevated PowerShell for Windows system operations. Recommended: fits the current native UAC architecture and permits shared installer/admin behavior and fixture-based Windows operation tests.
3. Replace startup and administration with a new Windows service architecture. Larger compatibility and credential migration impact; defer unless existing mechanisms cannot satisfy readiness and recovery.

## Shared network plan and UI

Introduce a versioned plan containing adapter GUID plus current index/name, selected primary IP and prefix, derived broadcast, gateway/DNS/DHCP observations, per-server ID/IP/TCP port/UDP range, and discovery port. Use server IDs rather than assuming array order. Adapter GUID identifies the same adapter across reindexing; require re-selection if it disappears.

The installer and admin Repair Network Configuration screen show active IPv4 adapters, favor physical Ethernet, and allow explicit selection. Show editable server IP inputs, selected primary address, subnet, gateway, DHCP state and the derived broadcast. Preserve existing ports and nonoverlapping UDP ranges by default. A primary address change is a separate explicit operation, not an incidental consequence of adding aliases.

Offer address suggestions from usable hosts in the actual subnet, excluding network/broadcast/gateway/primary, local assignments on every adapter, duplicate proposals and discovered neighbors. Probe the selected interface for conflicts; distinguish occupied, apparently available and inconclusive. No response cannot prove an address is free. Require confirmation that static aliases are excluded/reserved in the router's DHCP allocation. Do not copy the example addresses onto a different subnet.

## Safe Windows networking

Before mutation, save protected, durable original adapter/address/route/DNS/DHCP/coexistence state and a write-ahead operation journal. Revalidate adapter identity and observations immediately before applying a plan. Record addresses added by this installation separately from pre-existing addresses adopted for use.

Use persistent secondary addresses with SkipAsSource and no additional default gateway. Wait for duplicate-address detection and Preferred state. Verify primary address, DHCP mode, gateway routes and DNS configuration have not unexpectedly changed.

Microsoft documents that New-NetIPAddress automatically disables DHCP on a DHCP-enabled interface. Do not use it blindly. Windows provides DHCP/static IP coexistence through netsh; local command help confirms this capability on this machine. On supported systems use the coexistence path, record its prior state, and verify DHCP remains enabled after assignment. If the capability is absent or fails validation, stop and offer a separately reviewed static configuration with explicit gateway/DNS confirmation; never silently fall back.

Limit rollback to mutations made by this operation. Roll back on assignment failure or unexpected baseline changes; retain a local recovery journal and recovery instructions for interrupted operations. Recovery must not depend on internet access or a still-running installer. Test interruption recovery and DHCP behavior on disposable Windows adapters/VMs before claiming that path supported.

## Transactional configuration and activation

One shared plan prepares updates for config.json and each configured server.properties. Parse JSON structurally while retaining unrelated/unknown fields. Update only interface_ip, listen and backend fields selected by server ID. Update named properties without blind substring replacement, preserve comments/unrelated settings and reject ambiguous duplicate keys. Validate transport, address membership, ports and UDP overlap before commit.

Keep stopped-state checks, timestamped original-file backups, hashes and a journal across all files. There is no cross-file filesystem atomic rename: startup stays gated until every file is committed and verified. Interrupted or failed commits recover the entire previous set. Never touch world contents during repair.

Repair previews discrepancies and edits before approval. Acquire the existing operation lock; refuse concurrent snapshot jobs. Warn about players and confirm the maintenance interruption. Gracefully stop the previously running components, apply the coordinated change, and restart only that set. External/unowned processes require manual attention rather than forced termination. Helper restart is required to load changed bindings.

Create narrow, installation-owned firewall rules for TCP signalling, configured UDP gameplay ranges and UDP discovery on the selected trusted LAN. Do not reset the firewall or alter unrelated rules/network profiles. Validate wildcard/dual-stack ownership carefully; port 7551 use by Bedrock IPv6 must not be misreported as a conflicting helper IPv4 socket.

Activation requires assigned IPs and matching properties/helper backends. Readiness checks verify owned process paths/PIDs, actual TCP listeners, bound-source backend connections and expected signalling responses, helper listener/interface binding and firewall policy. Report passed, failed, blocked or awaiting manual verification separately. Successful process creation is insufficient.

## Startup and runtime repair

Keep supported Task Scheduler startup under the Windows identity that owns DPAPI credentials. Boot mode uses Password logon and supports session 0 without interactive login. The long-lived startup/watchdog action must remain active so Task Scheduler recovery can observe failures; a short launcher that exits successfully cannot provide crash recovery by itself.

Wait for the adapter and required addresses, then start Bedrock, wait for TCP readiness, then advertise. Bound retries and log unavailable adapter/IP/ports. Track shutdown/decommission markers so recovery never resurrects a retired installation. If bindings drift, expose admin diagnostics/repair while keeping advertising disabled; do not automatically change the PC's network.

## Ownership manifest and migration evidence

Maintain a separate versioned installation manifest, distinct from the archive checksum manifest: installation UUID, machine identity, canonical roots, executable paths, owned files/directories, services, tasks/actions, startup entries/shortcuts, firewall rule IDs, created/adopted aliases, original network state and associated user/data/credential locations. Protect the manifest and journal with Windows ACLs. An editable manifest is not sufficient authority for elevated deletion: validate every resource against current ownership and canonical scope.

Source resource records travel as historical evidence in the migration package. Generate a new destination installation identity and actual local resource manifest; never replay source tasks, firewall rules or alias ownership onto the destination. Preserve Minecraft network IDs and persistent identity keys.

Export a destination verification report tied to the archive ID/hash, restored data inventory and server identities. World/packs integrity is verified against the restored package, including LevelDB logs. Keep the same-subnet source stopped while the destination uses transferred aliases/identities. Source decommissioning accepts the portable report and asks the user to confirm actual destination joins; a local file cannot independently prove the destination remains online.

## Standalone decommissioning

Ship a separate local utility alongside the installer. First scan only: discover candidate installations from explicit selection, uninstall registrations and manifest/config/executable evidence. Scan all required resource classes: owned processes, services, tasks, startup folders/Run/RunOnce, watchdogs, firewall, aliases, installation/data/cache/log/temp paths, shortcuts, uninstall registrations and owned saved credentials. Inspect other user hives/paths only when accessible; report inaccessible locations instead of claiming complete cleanup.

For legacy installations, correlate full executable/action paths and configuration roots. Known filenames and task names locate candidates but never authorize killing/deleting. Existing manually configured aliases require explicit adoption as project-owned, excluding the primary IP, gateways, DHCP addresses and any address used by unrelated components. Never classify an old helper interface_ip as disposable.

Display a concrete inventory with evidence, exact removal targets, ambiguous resources and permissions needed. A dry-run report is the default. Require successful destination verification evidence/user attestation before retirement; offer a verified final archive stored outside deletion scope.

Option A retains server data and rollback configuration while disabling all owned startup/recovery mechanisms, cleanly stopping owned processes, unregistering owned tasks/services/startup entries, and removing approved aliases/firewall rules. Write a durable retired marker before disabling recovery.

Option B additionally requires explicit permanent deletion confirmation naming the canonical installation scope. Delete only verified selected resources, never broad AppData/ProgramData/temporary directories, unrelated backups or manifest-supplied arbitrary paths. Refuse recursive deletion through reparse points and recheck scope immediately before each mutation. Run the utility from outside the selected tree so cleanup can finish. Do not execute Option B on this working installation as part of development.

Remove credentials only when ownership is proven, with separate disclosure that reauthentication will be required for rollback. Retained data stays protected. Record partial failure and allow idempotent retry. Verification re-enumerates resources independently, checks owned endpoints/processes are gone and reports retained/removed/unresolved items. Reboot verification is explicitly pending until a post-reboot scan succeeds; do not reboot automatically.

## Validation and release gates

Automated fixtures cover non-/24 subnets, multiple adapters/addresses, stale primary and backend bindings, server IDs in different order, unknown JSON fields, property comments/duplicates, unchanged worlds/packs/identities, occupied/unknown aliases, UDP overlap and TCP conflicts, partial configuration writes, interruption recovery and manifest tampering.

Windows operation harnesses cover persistent alias creation, DHCP coexistence without DHCP loss, failure rollback, unchanged DNS/routes, precise firewall rules, startup ownership/readiness/recovery, path-based process selection and decommissioning with unrelated Minecraft processes/tasks/aliases present. Test missing privileges as blocked checks.

Actual Windows acceptance needs disposable fresh/different-subnet and DHCP destinations, occupied aliases, interruption tests, real restart persistence, session-0 boot startup, crash recovery, both console joins/independent discovery and post-decommission reboot. Mocks and this machine's static adapter cannot establish those outcomes. Keep a test matrix showing not-run scenarios until observed.

Release packaging must include the standalone utility and checksum-verified readable scripts, preserve current antivirus-compatible UAC design, refresh installer/source/package hashes and run the existing regression suite. Preserve Xbox login timeout and LAN SDP handling unchanged.

## References

- Microsoft New-NetIPAddress: https://learn.microsoft.com/en-us/powershell/module/nettcpip/new-netipaddress
- Microsoft netsh interface, including DHCP/static coexistence: https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/netsh-interface

## Review decision

Recommended approach: shared Go network/resource planning with readable elevated Windows operations; retain the current application and startup model while fixing readiness/recovery. Implement and test without modifying this live working installation. A subsequent written implementation plan will break this design into reviewed, verifiable changes.
