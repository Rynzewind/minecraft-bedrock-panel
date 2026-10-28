# Add-on Updates Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans task by task. Record results in work/addon-updates-progress.md.

**Goal:** Ship secure CurseForge credentials, on-demand provider checks, and one safe manual/API update preview and staging flow.
**Architecture:** Extend addonupdates with DPAPI credential storage, structured source metadata and a provider client. Contentpacks owns immutable previews and transactional version-pinned staging. The controller enforces maintenance interlocks; the Walk Add-ons subtab exposes review actions.
**Tech Stack:** Go 1.26, Walk, Windows DPAPI, official CurseForge REST API, existing archive inspection and pack backup code.
**Spec:** ../specs/2026-10-28-addon-updates-design.md (approved).

## Global constraints
Preserve worlds/accounts/identities/enabled selections. No unattended activation or server restart. Never serialize API key in UI responses, source metadata, logs, snapshots or releases. Use official API only; downloads are keyless. Checks are on-demand. Stable releases default; actual manifests determine versions. Installer includes the UI, never a key.

## Review focus
- Legacy state and version provenance must not silently upgrade unselected servers.
- Preview/hash/state changes must invalidate approval.
- Corrupt source state and API errors cannot look up to date.
- Paired packs/dependencies and unchanged companions need conservative validation.
- Key exclusion, redirect handling and sanitized errors must survive packaging/migration.

### Task 1: Structured sources and protected credentials
Files: internal/addonupdates/sources.go, credentials.go, *_test.go; internal/migration/inventory tests; .gitignore.
Interfaces: Source {URL, Provider, ProjectID, Channel, LastChecked, Notes, LastFileID}; Sources.ListRecords() (map[string]Source,error), SaveRecord(uuid,Source) error. Credentials.Set/Remove/Load and Status() configured-only, injected accountstore.Protector.
- [ ] Write/run failing tests: old map migration, corrupt state error, URL/key validation, non-plaintext persistence, unreadable credentials, no returned key.
- [ ] Implement atomic protected writes, DPAPI host wiring and metadata snapshot inclusion/key exclusion.
- [ ] Run addonupdates/migration tests and commit.

### Task 2: Version-aware pack activation
Files: internal/contentpacks/store.go, activation.go, versions.go and tests.
Interfaces: Operation.Version pins target; persistentState.EnabledVersions tracks deployed versions; InventoryItem.DeployedVersions exposes public per-server versions.
- [ ] Red test: import v2 then apply an unrelated operation on the other server; its v1 remains v1. Test pinned staging survives later library v3 import and legacy world activation migration.
- [ ] Retain catalog metadata for historical versions, pin StageMany operations, resolve exact enabled/pending packs during Apply, roll back metadata on save failure.
- [ ] Run contentpacks tests and commit.

### Task 3: Immutable update previews and transactional staging
Files: internal/contentpacks/update.go and tests.
Interfaces: PreviewUpdate(path,selectedUUID,serverIDs,engineVersions) (UpdatePreview,error), StageUpdate(previewID) error.
- [ ] Red tests: wrong UUID, downgrade/unchanged selected, unrelated additions, paired dependency bundle, unchanged companion, absent dependency/engine mismatch, stale archive/state, no running-folder mutation and rollback on failed stage.
- [ ] Protected local archive copy and SHA-256; validate dependency graph and existing activation/pending state; return exact target plan. Commit to cloned state with exact versions; retain previous directories.
- [ ] Run contentpacks tests and commit.

### Task 4: CurseForge provider
Files: internal/addonupdates/provider.go, curseforge.go, downloads.go and tests.
Interfaces: Provider.Test(ctx), Resolve(ctx,Source), Files(ctx,project,channel), Download(ctx,file) -> staged path/provenance. HTTP clients/dependencies injectable for tests; production host fixed.
- [ ] Red tests: game discovery/project mismatch, pagination, channel filtering, 401/403/404/429, redacted response errors, no key across redirects/CDN, size/hash validation and distribution opt-out.
- [ ] Implement bounded official requests and keyless trusted CDN download; no live-key claims.
- [ ] Run provider tests and commit.

### Task 5: Service and Add-ons UI
Files: internal/service/addon_sources.go, addon_updates.go, controller.go; cmd/helper/main.go; admin/addon_updates_windows.go and updates_windows.go.
- [ ] Test credential methods never return key, invalid UUID/server selection rejected, mutation/maintenance interlocks and preview requirement.
- [ ] Wire configured-only key status, masked key dialog Save/Test/Remove, provider/source/channel settings, check selected/all, manual checked notes, local archive/selected provider download preview and explicit staged targets. Display library/deployed versions and truthful check states. Preserve selection on refresh. Responsive scroll content; compact actions.
- [ ] Test native controls and preview rendering with fake data; run full suite and commit.

### Task 6: Review, release and install
- [ ] Fresh whole-change review, fix important findings with regression tests.
- [ ] Update README/site/docs and release notes; audit secret exclusions and modified source.
- [ ] Build all binaries, package/test/Defender-scan single EXE, publish checksum verified Drive/Desktop and GitHub installer/source/site.
- [ ] Prepare backed-up protected app update; close only panel and gracefully restart helper/servers if required when no players. Preserve key/source/account/config/world state. Require user Windows UAC for protected install.
- [ ] Verify installed hashes, Ready states, source migration and manual UI. Real API connection remains pending user's key.
