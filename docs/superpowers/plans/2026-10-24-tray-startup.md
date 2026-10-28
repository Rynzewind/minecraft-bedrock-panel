# Tray Startup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans inline, following the user's existing preference for autonomous work. Track each step below.

**Goal:** Replace separate launcher scripts with reliable bulk lifecycle controls and silent logon startup in the admin tray.
**Architecture:** A shared internal/fleet engine coordinates an injectable client/host/readiness probe. The admin owns tray lifetime, single-instance activation, serialized operations and bounded recovery; Windows setup owns logon registration.
**Tech Stack:** Go 1.26, existing Walk/Win32, PowerShell installer assets.
**Spec:** docs/superpowers/specs/2026-10-24-tray-startup.md

## Global Constraints
- No forced process termination, world edits, credential resets or network changes.
- Sign-in startup is silent; intentional Stop All pauses recovery until Start or next panel session.
- Existing single-server controls, installer boot mode and decommissioning remain supported.
- Retry at most three times per fifteen minutes, no more often than once per minute.

## Review Focus
- Disconnected helper and crash/failed state must remain controllable without duplicate processes.
- Stop All must not be undone by recovery, including an operation already in flight.
- A TCP listener alone is insufficient: await the current process's Server started marker and short stability interval.
- Close-to-tray must not trap Windows shutdown or leave duplicate panel instances.
- Setup/uninstall must own and record exactly one startup registration.

### Task 1: Shared fleet lifecycle and failed-state recovery
Files: internal/fleet/controller.go, controller_test.go; internal/supervisor/manager.go and manager_test.go.
Interfaces: Client.Call(context.Context,string,any,any) error; Host.Ensure(context.Context) error, Running() (bool,error), WaitStopped(context.Context) error; Engine.Run(context.Context,string,func(string)) error.
- [ ] Add failing fake-host/client tests for disconnected start, idempotent stop, full restart ordering, failed server retry, external-server refusal and cancellation.
- [ ] Observe failures; implement orchestration and native helper ownership with bounded readiness.
- [ ] Add supervisor crash/restart regression; allow Start from Failed only after its owned process has exited.
- [ ] Run fleet/supervisor tests and commit.

### Task 2: Tray, singleton, bulk progress and recovery
Files: cmd/nethernet-multi-server-admin/fleet_windows.go, tray_windows.go and tests; window_windows.go, server_progress_windows.go; internal/fleet/recovery.go and tests.
Interfaces: adminWindow.fleetAction(method string, silent bool); setupTray(startup bool) error; Recovery.Allow(time.Time) bool, Reset().
- [ ] Test always-available idle bulk controls and bounded recovery/manual-stop policy; test singleton activation and native tray creation/close behaviour.
- [ ] Implement hidden helper launch, UI progress bridge, grass icon, tray actions, close-to-tray and --startup path without changing test-window creation behaviour.
- [ ] Keep manual player-disconnect confirmations, serialize recovery/manual work, and report failures clearly.
- [ ] Run admin/fleet regressions and commit.

### Task 3: Installer registration, release and live update
Files: scripts/setup-assets/Configure-Windows.ps1, internal/setup/script_hashes.go and installer tests; release documentation and standalone install assets.
- [ ] Add and run installer regression for direct admin --startup interactive task and manifest ownership; preserve explicit boot/none choices.
- [ ] Update pinned setup hashes and run full Go tests, vet and PowerShell checks.
- [ ] Build three existing executables and matching source; verify/scan complete release folder and ZIP.
- [ ] Inventory/backup live binaries and project startup entries, install verified builds and direct logon task, retaining legacy files as rollback copies.
- [ ] Verify tray/menu, helper-off Start/Stop semantics in isolated tests; verify live panel/servers/discovery and startup registration without disrupting connected players.

### Task 4: Explicit addon snapshot and installer restore UX
Files: internal/migration inventory/capture/manifest, migration API params/UI, existing setup wizard/PowerShell installer.
- [ ] Audit current captured content and restoration; regression-test an enabled pack, disabled library pack, and distinct assignments on two servers.
- [ ] Expose include-all-addons default-on snapshot option without discarding active-world pack files or breaking older packages.
- [ ] Add a clearly labelled restore-saved-servers/addons setup choice and inventory preview, preserving nonempty-destination backup confirmation.
- [ ] Verify round-trip pack files, assignments and pending state, old package compatibility, then include in the same tested release/live update.
