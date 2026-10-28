# NetherNet Static IP Setup and Removal Implementation Plan

> For agentic workers: use superpowers:executing-plans. Implement inline as requested by the user; do not delegate.

**Goal:** Ask for multiple server IPv4 addresses during setup, persist missing addresses on the selected Ethernet adapter, synchronise the servers/helper, and remove installation-owned aliases during uninstall.

**Architecture:** Shared Go validation and configuration planning, with the existing native UAC and readable checksum-verified PowerShell performing Windows mutations. Persist ownership and original adapter state before changes. Extend existing setup rather than replacing working server, snapshot or Xbox components.

**Tech stack:** Go, Walk Windows UI, Windows PowerShell, NetTCPIP/netsh, Task Scheduler, existing migration archive format.

**Spec:** [Network and decommission design](NetherNet-Network-and-Decommission-Design.md), refined by the user's approval of static secondary IP creation/removal. The primary PC address is retained; DHCP must not be silently disabled.

## Global constraints

- No live network changes, server restarts, deletion or deployment during development; tests use isolated fixtures.
- Preserve Creative TCP 19134 and UDP 49152–49200, Survival TCP 19136 and UDP 49201–49249 when those are the existing values. Other installations retain their actual ports.
- No hardcoded adapter name, subnet or destination addresses.
- Preserve worlds, packs, identities, account mappings, unrelated JSON/properties and the existing Xbox fixes.
- Pre-existing aliases are reused without automatic ownership; ask explicitly before adopting them for uninstall removal.
- Keep native UAC and readable scripts. Never weaken antivirus protection.
- All commands below run in work/migration-source using the existing work/Run-Go.ps1 wrapper and complete Go toolchain.

## Review focus

1. An elevated operation must revalidate adapter GUID and primary IP; a stale index must not affect another adapter.
2. A DHCP adapter must retain its lease, gateway and DNS after aliases are added; unsupported coexistence must fail safely.
3. Partial configuration writes or Windows mutation failures must preserve backups, activation gating and recoverable ownership state.
4. Existing project-like IPs, unrelated Minecraft processes and legacy startup entries must never be removed solely by name.
5. Reboot, console joins and real DHCP acceptance cannot be established by fixture tests and must remain explicitly pending until observed.

## Task 1 — Shared network plan and configuration transaction

**Files:** Create internal/setup/network.go, network_test.go, configuration.go, configuration_test.go. Modify internal/migration/restore.go and its restore tests.

**Interfaces:** NetworkPlan contains adapter identity, primary/prefix/broadcast and per-server ID/address/TCP/UDP settings. ValidateNetworkPlan returns an error for unusable hosts, duplicate IPs, incorrect broadcast or overlapping ranges. PrepareNetworkConfiguration(root, plan) returns the exact original and replacement file bytes; CommitNetworkConfiguration applies the complete set with backups/journal and activation gating.

- [ ] Write failing tests for a /20 LAN, stale source IPs, duplicate/gateway/broadcast addresses, reordered server IDs, UDP overlap, unknown JSON fields and property comments/duplicate keys.
- [ ] Run `../Run-Go.ps1 test ./internal/setup ./internal/migration` and observe the new failures.
- [ ] Implement shared validation and structured edits. Preserve unrelated fields and ports; reject ambiguous input. Make restore require/reuse the validated per-server plan rather than silently assigning all backends to the primary address.
- [ ] Add a failing injected partial-write test; implement journal recovery restoring all original files before startup can proceed.
- [ ] Run targeted tests and verify world/packs/identity fixtures remain byte-identical. Commit the task.

## Task 2 — Adapter discovery, persistent aliases and ownership

**Files:** Create scripts/setup-assets/Network-Configuration.ps1 and scripts/tests/network-harness.ps1; add internal/setup/network_windows.go and network_windows_test.go. Extend plan.go and Configure-Windows.ps1; update script checksum validation/tests.

**Interfaces:** Read-only discovery returns adapter GUID/index/name, active IPv4 addresses/prefixes, gateway, DNS, DHCP/coexistence state. An approved plan carries expected baseline state and an explicit list of alias adoptions. Elevated apply returns added/reused/adopted aliases and a structured check report.

- [ ] Write failing harness cases for missing aliases, reused aliases, primary protection, stale adapter identity, occupied addresses, assignment failure, DHCP coexistence and changed gateway/DNS.
- [ ] Run targeted Go tests that invoke the PowerShell harness; observe failures before implementing.
- [ ] Save original network state and a protected, versioned installation resource manifest before mutation. Revalidate baseline and conflict checks after elevation.
- [ ] Create persistent static secondary addresses with SkipAsSource and no gateway. For DHCP use supported coexistence; verify DHCP remains enabled. Stop with a clear explanation when unsupported; do not automatically convert the primary to static.
- [ ] Wait for Preferred state and verify primary/gateway/DNS baseline. Roll back only additions/current-operation settings on failure; retain durable recovery state after interruption.
- [ ] Test retry idempotence and that reused addresses are not marked removable without adoption. Run the harness and commit.

## Task 3 — Setup IP inputs and administration repair

**Files:** Modify cmd/nethernet-multi-server-setup/window_windows.go; create network_windows.go in that command. Modify admin window/migration UI and service operation dispatch as needed; add focused view-model/service tests.

**Interfaces:** Setup and repair consume the same NetworkPlan and transaction API. Repair is mutually exclusive with snapshots and other mutations, and restarts only components previously running after a confirmed maintenance stop.

- [ ] Write failing tests for selected adapter changes, invalid IP inputs, per-server field mapping, manual overrides and confirmation summaries.
- [ ] Add editable server IP fields and detected primary/subnet/gateway/DHCP display. Show derived broadcast, preserved ports, conflict status, DHCP reservation instructions and explicit adoption checkboxes.
- [ ] Apply aliases and configuration together only after a complete preview and Windows elevation. Keep advertising gated until verification succeeds.
- [ ] Add Repair Network Configuration to the admin UI; preview discrepancies, require maintenance confirmation, stop cleanly and use the same workflow without touching worlds.
- [ ] Test snapshot/repair exclusion and helper restart handling. Run UI creation and targeted tests; commit.

## Task 4 — Endpoint checks, firewall and startup readiness

**Files:** Extend Configure-Windows.ps1, configuration-harness.ps1 and workflow_windows_test.go. Modify generated startup scripts in internal/migration/restore.go and helper startup diagnostics.

**Interfaces:** ValidationReport records passed/failed/blocked/manual checks independently; no success result from process launch alone.

- [ ] Write failing tests for missing local aliases, wrong-PID listeners, unreachable backends, TCP/UDP firewall coverage, wildcard/IPv6 discovery sockets and startup before network readiness.
- [ ] Add scoped TCP signalling, configured UDP gameplay and discovery firewall rules with manifest ownership.
- [ ] Verify owned process/listener endpoints and backend reachability before advertising. Keep diagnostics/repair available when obsolete bindings prevent discovery startup.
- [ ] Make the scheduled task observe a long-lived supervised action so crash recovery can work. Wait for network and server readiness with bounded retries and helpful logs; honour retired/activation markers.
- [ ] Run targeted regression tests and commit. Record real boot/crash recovery tests as pending until performed on a disposable/destination PC.

## Task 5 — Standalone decommission and uninstall-owned aliases

**Files:** Create internal/decommission inventory/plan/verification code and tests; add cmd/nethernet-multi-server-uninstall Windows UI/CLI and readable setup-assets decommission script plus harness. Extend installation manifest and release build packaging.

**Interfaces:** Inventory(root) yields exact resources and ownership evidence. PlanDecommission(inventory, options) produces a dry-run removal list. Apply requires reviewed scope, migration verification attestation and the selected retain-data/complete-removal confirmations.

- [ ] Write failing tests with unrelated processes/tasks/services/aliases, a primary IP appearing in legacy config, tampered manifests, reparse paths and external backup folders.
- [ ] Inventory verified executable paths, services/tasks/actions, watchdogs, Startup shortcuts/Run/RunOnce, firewall IDs, aliases, data/app/cache/temp paths, credentials and uninstall registrations. Mark inaccessible or ambiguous resources unresolved.
- [ ] Disable owned startup/recovery mechanisms before graceful process shutdown. Remove only created or explicitly adopted aliases, revalidating primary/gateway/adapter protection immediately before removal.
- [ ] Implement retain-data mode and independently confirmed complete-removal mode. Preserve final archives outside deletion scope; refuse arbitrary manifest paths and broad cleanup. Never run either mode against this working installation during development.
- [ ] Add legacy discovery using correlated paths/configuration and explicit alias adoption. Independently re-enumerate resources after cleanup, report retained/unresolved items and provide post-reboot verification mode.
- [ ] Run harness/tests, including idempotent partial-failure retry. Commit.

## Task 6 — Regression verification and deliverables

**Files:** scripts/build-migration-release.ps1, scripts/update-panel.ps1, release README/source packaging/checksum manifests; outputs acceptance report.

- [ ] Run `../Run-Go.ps1 test ./...` and `../Run-Go.ps1 vet ./...`; inspect every result.
- [ ] Review the full diff for ownership, privilege-boundary, interruption and primary-address safety; fix material issues with regression tests.
- [ ] Build a separate updated release folder containing setup, uninstall, admin/helper/control and readable verified scripts. Verify source/package checksums and scan the new release with Windows Defender without changing security settings.
- [ ] Restore a real snapshot into an isolated fresh folder; verify coordinated IP settings and preserved world/packs/identity hashes without assigning test IPs to the working adapter.
- [ ] Publish explicit acceptance status: fixture-tested versus actual Windows different-subnet/DHCP/occupied-address/reboot/session-0/console-join/decommission-reboot checks still requiring destination testing.
- [ ] Deliver the installer package and standalone uninstaller with instructions. Deployment to the working installation remains a separate reviewed action.

## Execution handoff

The user has chosen inline execution. Review this plan once; after confirmation, execute all tasks continuously without asking between tasks. The approved IP-entry/create/remove flow is binding. The broader migration and decommission requirements remain included; implementation is not complete until the corresponding code and verification are delivered.
