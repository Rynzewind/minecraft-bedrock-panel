# Server Updates Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans. Execute tasks with tests and retain progress here.

**Goal:** Safely update both servers to 26.60 and provide a reusable Updates tab.
**Architecture:** A service-owned asynchronous updater downloads official stable server archives, gates protocol compatibility, backs up before replacing version-owned software and journals recovery. The panel previews and starts jobs over the existing named pipe.
**Tech Stack:** Go, Walk, Windows ACLs, official Minecraft downloads, gophertunnel v1.63.0.
**Spec:** ../specs/2026-10-28-server-updates-design.md

## Global Constraints
Preserve worlds, custom packs, settings, accounts, identities and backups. No preview or unattended installation. Updates has its own relevant icon. Open visible panel maximized; preserve startup-in-tray.

## Review Focus
- Unsupported next protocol refuses before stopping servers.
- Archive traversal, links and bundled mutable defaults cannot overwrite user data.
- Interrupted jobs block startup until recovery is reviewed.
- New-engine world opening prohibits automatic engine downgrade.
- Concurrent snapshot/player activity refuses maintenance.

### Task 1: Protocol and maximized startup
- [ ] Compare local gophertunnel v1.62.0 patches, port to v1.63.0; add failing current-protocol assertion then run proxy tests.
- [ ] Add native startup policy test, implement maximization preserving hidden startup and tray reopen.

### Task 2: Update engine
Files: internal/serverupdate/{release,archive,manager,recovery}.go and tests.
Interfaces: New(root, configPath string, supervisor Supervisor) *Manager; Check(ctx) (Release,error), Start(ctx) (Status,error), Status() Status, Busy() bool.
- [ ] Failing tests for stable official metadata, compatibility gate, unsafe/default archive entries, custom pack preservation, copy verification, player refusal, failure stages and interrupted journal.
- [ ] Implement download/staging, verified backups, software replacement, durable journal, startup/network health checks and explicit recovery.
- [ ] Run go test ./internal/serverupdate.

### Task 3: Service and UI
Files: internal/service/updates.go, controller.go; cmd/nethernet-multi-server/main.go; admin/updates_windows.go, theme_windows.go, window_windows.go.
- [ ] Test mutation interlock and Updates page controls/geometry.
- [ ] Connect update methods; shared maintenance interlock; startup recovery guard.
- [ ] Add compact Updates tab, download-arrow icon, preview and polling; recovery requires explicit acknowledgement of backup restore.

### Task 4: Release and installation
- [ ] Full tests and review. Build checksum-verified release and installer; Defender scan and publish per AGENTS.md.
- [ ] Install matching helper/panel then perform initial backed-up 26.60 update when zero players; require Windows UAC for protected paths.
- [ ] Verify actual server versions, configuration/account preservation, Ready state; request console join verification.

## Execution ledger
Ruling: User said approved just make it; execute inline without another plan approval. Existing isolated extracted source has no Git metadata; preserve backups instead of inventing commits.
Pre-flight: Task 1 protocol constants feed Task 2 gate; Task 2 API feeds Task 3; Task 4 depends on all checks.
