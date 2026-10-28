# BDS Content Pack Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe local Bedrock add-on management and background Start/Stop/Restart controls for the Creative and Survival BDS instances to the existing administrator application.

**Architecture:** `NetherNetMultiServer.exe` remains the long-running owner of privileged process and filesystem operations. New focused `internal/supervisor` and `internal/contentpacks` packages expose testable lifecycle and transaction services; the existing local admin API connects those services to new Servers and Content Packs GUI tabs. Pack changes are staged while a server runs and applied transactionally only while it is stopped.

**Tech Stack:** Go 1.26, Windows `os/exec`, Walk native GUI, JSON, ZIP archives, existing length-prefixed named-pipe admin API, existing PowerShell release tooling.

**Spec:** `docs/superpowers/specs/2026-09-20-bds-content-pack-management-design.md`

## Global Constraints

- Preserve LAN discovery, NetherNet proxying, advertisements, Network IDs, Microsoft account pooling, and player assignment behavior.
- Support only local `.mcaddon`, `.mcpack`, and `.zip` imports; never download add-ons.
- Never edit or delete world databases, `level.dat`, player data, identities, or authentication data.
- Never force-terminate BDS automatically; managed shutdown writes `stop` and reports a timeout.
- Closing the GUI must not stop either BDS process.
- Existing configuration without process sections must remain valid, with process/content controls reported as unconfigured.
- In-game LAN thumbnails are excluded because NetherNet discovery carries no image field.
- Every archive is untrusted input; reject path traversal, absolute/drive-qualified paths, symlinks, excessive expansion, excessive file count, and excessive nesting.

## Review Focus

- A ZIP entry using mixed slash/backslash traversal such as `packs/..\\..\\server.properties` must be rejected before extraction (Task 3).
- A helper restart while independently launched BDS instances are already listening must report `Externally running`, not start duplicates or kill them (Task 2).
- A linked behavior/resource pair assigned to both servers must generate independent pending operations and valid activation JSON for each world (Task 4).
- A failed file replacement halfway through apply must restore activation/configuration files and leave BDS stopped with an actionable error (Task 4).
- Stop or Restart requested while players are connected must require the GUI confirmation and preserve server-specific player counts (Task 6).

---

### Task 1: Process Configuration and Stable Server IDs

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `outputs/config.json`
- Modify: `config.example.json`

**Interfaces:**
- Produces: `config.Process`, `config.Server.ID`, `config.Server.Process`, and `config.Server.ProcessConfigured() bool`.
- Consumes: existing `config.Load`, `Config.Validate`, and Creative/Survival entries.

- [ ] **Step 1: Write failing configuration tests**

Add table tests proving legacy entries remain valid, configured entries require absolute executable/working/world/log paths, world/log paths remain beneath the intended server root, and server IDs are non-empty and unique:

```go
func TestLegacyServerWithoutProcessRemainsValid(t *testing.T) { /* existing valid fixture, no process */ }
func TestProcessConfigurationRequiresSafeAbsolutePaths(t *testing.T) { /* relative and escaped world paths fail */ }
func TestServerIDsMustBeUnique(t *testing.T) { /* creative duplicated twice fails */ }
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `work\toolchains\go\bin\go.exe test ./internal/config`

Expected: FAIL because `Process`, `ID`, and validation behavior do not exist.

- [ ] **Step 3: Implement configuration types and validation**

Add:

```go
type Process struct {
    Executable       string `json:"executable"`
    WorkingDirectory string `json:"working_directory"`
    WorldDirectory   string `json:"world_directory"`
    LogDirectory     string `json:"log_directory"`
}

type Server struct {
    ID      string   `json:"id,omitempty"`
    Process *Process `json:"process,omitempty"`
    // existing fields unchanged
}

func (s Server) ProcessConfigured() bool { return s.Process != nil }
```

Normalize paths only for comparison; retain configured spelling. For compatibility, derive a stable runtime ID from the lower-cased trimmed `name` only when `id` is absent. Require explicit unique IDs for the deployed configuration.

- [ ] **Step 4: Update deployment/example configuration**

Add `id: creative` and `id: survival` plus their exact BDS paths from the specification. Do not alter existing network or discovery fields.

- [ ] **Step 5: Run focused and full tests**

Run: `work\toolchains\go\bin\go.exe test ./internal/config ./...`

Expected: PASS.

### Task 2: Background BDS Supervisor

**Files:**
- Create: `internal/supervisor/types.go`
- Create: `internal/supervisor/manager.go`
- Create: `internal/supervisor/process_windows.go`
- Create: `internal/supervisor/manager_test.go`
- Modify: `cmd/nethernet-multi-server/main.go`
- Modify: `cmd/nethernet-multi-server/main_test.go`

**Interfaces:**
- Consumes: `config.Server.ID`, `config.Server.Process`, current active leases/player counts.
- Produces: `supervisor.Manager`, `Status`, `Start(ctx,id)`, `Stop(ctx,id)`, `Restart(ctx,id)`, `RecentLog(id)`, and `ApplyPending func(context.Context,string) error` hook.

- [ ] **Step 1: Write failing state-machine tests**

Use a fake process factory and controllable fake process. Test stopped-to-starting-to-running, graceful `stop\n`, exit status, duplicate request rejection, timeout without forced kill, separate Creative/Survival state, and externally-running detection:

```go
type ProcessFactory interface { Start(context.Context, config.Process, io.Writer) (Process, error) }
type Process interface { PID() int; Write([]byte) (int,error); Wait() error }
```

- [ ] **Step 2: Run supervisor tests and verify RED**

Run: `work\toolchains\go\bin\go.exe test ./internal/supervisor`

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the synchronized manager**

Represent public state using constants `unconfigured`, `stopped`, `starting`, `running`, `stopping`, `externally_running`, and `failed`. Serialize operations per server. Store PID, start time, last exit text, pending flag, player count, and a bounded line ring.

- [ ] **Step 4: Implement the Windows process adapter**

Use `exec.CommandContext` only to start, not to bind process lifetime to helper cancellation. Set `SysProcAttr.HideWindow = true` and `CreationFlags = CREATE_NO_WINDOW`; retain stdin; combine stdout/stderr into persistent timestamped logs and the bounded ring. Graceful stop writes exactly `stop\r\n` and waits without calling `Kill`.

- [ ] **Step 5: Integrate construction without changing startup ownership**

Construct the supervisor in `main.go`, but do not automatically start stopped servers. Detect configured backend UDP listeners and mark an unowned running instance `Externally running`. Inject factories/detection in tests so no real BDS or network endpoint is opened.

- [ ] **Step 6: Run focused and full tests**

Run: `work\toolchains\go\bin\go.exe test ./internal/supervisor ./cmd/nethernet-multi-server ./...`

Expected: PASS.

### Task 3: Secure Pack Archive Inspection

**Files:**
- Create: `internal/contentpacks/manifest.go`
- Create: `internal/contentpacks/archive.go`
- Create: `internal/contentpacks/archive_test.go`
- Create: `internal/contentpacks/testdata/README.txt`

**Interfaces:**
- Produces: `Inspect(path string, limits Limits) (Import, error)`, `Manifest`, `Pack`, `Dependency`, and `Limits`.
- Consumes: local archive path selected by the GUI.

- [ ] **Step 1: Write failing archive security and manifest tests**

Generate archives in-memory during tests. Cover valid root/nested manifests, behavior/resource modules, dependency links, duplicate UUIDs, malformed JSON, absent manifests, absolute paths, drive paths, `../`, mixed separators, symlink attributes, maximum files, maximum nesting, compressed size, and expanded size.

Use explicit defaults:

```go
var DefaultLimits = Limits{
    MaxCompressedBytes: 256 << 20,
    MaxExpandedBytes:   1 << 30,
    MaxFiles:           20000,
    MaxDepth:           32,
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `work\toolchains\go\bin\go.exe test ./internal/contentpacks -run 'TestInspect|TestReject'`

Expected: FAIL because inspection is absent.

- [ ] **Step 3: Implement manifest parsing and archive inspection**

Accept `.mcaddon`, `.mcpack`, and `.zip` case-insensitively. Normalize each entry with slash semantics after replacing backslashes; reject unsafe entries before reading content. Parse manifest v2 vectors and preserve enough version information to compare lexicographically. Classify module type `data` as behavior and `resources` as resource. Return all validation findings without extracting.

- [ ] **Step 4: Implement extraction into a caller-created staging directory**

Add `Extract(imported Import, destination string) error`. Revalidate every path during extraction, create files with non-executable permissions, and remove partial extraction on failure only when the destination is confirmed beneath the helper staging root supplied by the caller.

- [ ] **Step 5: Run focused and full tests**

Run: `work\toolchains\go\bin\go.exe test ./internal/contentpacks ./...`

Expected: PASS.

### Task 4: Persistent Staging, Activation, Backup, and Rollback

**Files:**
- Create: `internal/contentpacks/store.go`
- Create: `internal/contentpacks/activation.go`
- Create: `internal/contentpacks/transaction.go`
- Create: `internal/contentpacks/store_test.go`
- Create: `internal/contentpacks/transaction_test.go`

**Interfaces:**
- Consumes: `contentpacks.Import`, configured server/world roots, supervisor stopped/running state.
- Produces: `Manager.Import`, `Inventory`, `Stage`, `Pending`, `CancelPending`, and `Apply(ctx,serverID)`.

- [ ] **Step 1: Write failing persistent-state and activation tests**

Test restart-safe inventory/pending JSON, linked pack grouping, independent assignments for both servers, version conflict rules, generated `world_behavior_packs.json` and `world_resource_packs.json`, and preservation of unknown valid activation entries.

- [ ] **Step 2: Verify RED**

Run: `work\toolchains\go\bin\go.exe test ./internal/contentpacks -run 'TestStore|TestActivation'`

Expected: FAIL because persistence and activation do not exist.

- [ ] **Step 3: Implement the store and pending-operation model**

Persist helper-owned metadata beneath `content-staging/state.json` using write-temp, sync, rename. Define operations `enable`, `disable`, `assign`, `unassign`, `update`, `uninstall`, and `require_resources`, keyed by stable server ID and pack header UUID.

- [ ] **Step 4: Write failing transaction/rollback tests**

Inject a filesystem adapter that fails after a selected rename. Verify timestamped backup content, operation manifest, atomic activation writes, rollback after mid-apply failure, untouched world DB/`level.dat`, and refusal to apply while running.

- [ ] **Step 5: Implement transactional application**

Plan all copies/removals/writes before mutation. Back up affected files and pack directories. Apply to temporary sibling paths, rename into place, and restore activation/configuration state on error. Update `texturepack-required` without reordering or rewriting unrelated `server.properties` lines.

- [ ] **Step 6: Connect supervisor pending application hook**

Before Start and between Stop/Start in Restart, call `Apply(ctx, serverID)`. Leave BDS stopped on failure and expose the error through supervisor status.

- [ ] **Step 7: Run focused and full tests**

Run: `work\toolchains\go\bin\go.exe test ./internal/contentpacks ./internal/supervisor ./...`

Expected: PASS.

### Task 5: Administration API for Servers and Packs

**Files:**
- Modify: `internal/adminapi/messages.go`
- Modify: `internal/adminapi/handler.go`
- Modify: `internal/adminapi/handler_test.go`
- Modify: `internal/service/controller.go`
- Modify: `internal/service/controller_test.go`

**Interfaces:**
- Consumes: supervisor and content-pack manager interfaces.
- Produces: named-pipe methods `servers.list`, `server.start`, `server.stop`, `server.restart`, `server.logs`, `packs.list`, `pack.import`, `pack.stage`, `packs.pending`, `packs.cancel`, and `packs.apply`.

- [ ] **Step 1: Write failing controller tests**

Use fakes to assert stable server IDs are passed unchanged, lifecycle conflicts receive coded errors, import returns complete validation details, apply refuses running servers, and status responses never expose filesystem-internal temporary paths or token material.

- [ ] **Step 2: Verify RED**

Run: `work\toolchains\go\bin\go.exe test ./internal/service ./internal/adminapi`

Expected: FAIL for unknown methods/types.

- [ ] **Step 3: Add request/response DTOs and controller dependencies**

Define narrow interfaces in `service` so controller tests do not launch processes or touch real BDS folders. Keep the current account constructor as a compatibility wrapper and add a full constructor accepting supervisor/content managers.

- [ ] **Step 4: Implement methods and coded errors**

Map state conflicts to `invalid_server_state`, external ownership to `externally_running`, unsafe/invalid archives to `invalid_pack`, conflicts to `pack_conflict`, and transaction failures to `pack_apply_failed`. Continue filtering authentication material through `safeMessage`.

- [ ] **Step 5: Run focused and full tests**

Run: `work\toolchains\go\bin\go.exe test ./internal/service ./internal/adminapi ./...`

Expected: PASS.

### Task 6: Servers Tab and Lifecycle Controls

**Files:**
- Create: `cmd/nethernet-multi-server-admin/servers_viewmodel.go`
- Create: `cmd/nethernet-multi-server-admin/servers_viewmodel_test.go`
- Modify: `cmd/nethernet-multi-server-admin/window_windows.go`
- Modify: `cmd/nethernet-multi-server-admin/window_windows_test.go`

**Interfaces:**
- Consumes: `servers.list`, lifecycle methods, recent logs, and active-player counts.
- Produces: Servers tab cards/table, state-aware buttons, confirmations, and log display.

- [ ] **Step 1: Write failing view-model tests**

Test state labels, green/red/amber semantic roles, button enablement for every state, independent player counts, pending markers, and confirmation requirements only when Stop/Restart would disconnect players.

- [ ] **Step 2: Verify RED**

Run: `work\toolchains\go\bin\go.exe test ./cmd/nethernet-multi-server-admin -run 'TestServer'`

Expected: FAIL because the server view model is absent.

- [ ] **Step 3: Implement the pure view model**

Keep process state mapping and action rules outside Walk widgets. Treat color as supplementary; button labels and state text remain sufficient.

- [ ] **Step 4: Add a Walk tab container and Servers tab**

Refactor the existing Account Pool/Console Users layout into an `Accounts` tab without changing its behavior. Add `Servers` with one selected-server table/card, Start/Stop/Restart controls, status metadata, and read-only recent logs. All calls remain asynchronous and marshal results back through `a.sync`.

- [ ] **Step 5: Add confirmation and error behavior**

Stop/Restart with active players shows server name and count. Externally-running status explains that one manual stop is required. Lifecycle errors append to Activity and remain visible in the server card.

- [ ] **Step 6: Run focused and full tests**

Run: `work\toolchains\go\bin\go.exe test ./cmd/nethernet-multi-server-admin ./...`

Expected: PASS.

### Task 7: Content Packs Tab

**Files:**
- Create: `cmd/nethernet-multi-server-admin/packs_viewmodel.go`
- Create: `cmd/nethernet-multi-server-admin/packs_viewmodel_test.go`
- Modify: `cmd/nethernet-multi-server-admin/window_windows.go`
- Modify: `cmd/nethernet-multi-server-admin/window_windows_test.go`

**Interfaces:**
- Consumes: pack inventory/import/stage/pending/apply API methods and server state.
- Produces: local file import, per-server assignment/enablement controls, pending summary, and apply action.

- [ ] **Step 1: Write failing view-model tests**

Test linked-pack display, validation-error rendering, Creative/Survival assignment status, enable/disable/update/uninstall action rules, resource-policy state, pending summary, and Apply Now enabled only for a stopped configured server.

- [ ] **Step 2: Verify RED**

Run: `work\toolchains\go\bin\go.exe test ./cmd/nethernet-multi-server-admin -run 'TestPack'`

Expected: FAIL because pack presentation does not exist.

- [ ] **Step 3: Implement the pure pack view model**

Represent selection by pack UUID and server ID rather than row indices. Group linked packs into one visible add-on while retaining per-pack type/version details.

- [ ] **Step 4: Add the Content Packs tab**

Use Walk `FileDialog` filtered to `*.mcaddon;*.mcpack;*.zip`. Add pack inventory, details, Creative/Survival target selection, Enable/Disable/Update/Uninstall, resource acceptance checkbox, pending operations, Cancel Pending, and Apply Now.

- [ ] **Step 5: Add safe confirmations and progress feedback**

Uninstall names affected servers and confirms that world data is untouched. Import validation errors appear before any staging action. Running servers show `Applies at next restart`; stopped servers offer Apply Now. Long calls disable only the affected controls and append results to Activity.

- [ ] **Step 6: Run focused and full tests**

Run: `work\toolchains\go\bin\go.exe test ./cmd/nethernet-multi-server-admin ./...`

Expected: PASS.

### Task 8: Release, Operator Documentation, and Deployment Verification

**Files:**
- Modify: `outputs/README.txt`
- Modify: `outputs/ACCOUNT-POOL-SETUP.txt`
- Create: `outputs/CONTENT-PACKS-AND-SERVER-CONTROLS.txt`
- Modify: `scripts/build-release.ps1`
- Modify: `README.md`

**Interfaces:**
- Consumes: completed supervisor, pack manager, API, GUI, and deployed configuration.
- Produces: verified Windows executables, operator guide, rollback artifacts, and staged deployment.

- [ ] **Step 1: Write documentation acceptance checks**

Extend the release script’s preflight to require documentation to contain exact headings `Importing an add-on`, `Pending changes`, `Start, Stop, and Restart`, `Externally running`, `Backups`, and `Rollback after an error`.

- [ ] **Step 2: Run release preflight and verify RED**

Run: `powershell -ExecutionPolicy Bypass -File scripts\build-release.ps1`

Expected: FAIL because the new guide/headings are absent.

- [ ] **Step 3: Write operator documentation**

Document file import, server targeting, staged changes, required restart behavior, lifecycle controls, external-process takeover, backup locations, failure recovery, and the explicit absence of in-game thumbnail customization.

- [ ] **Step 4: Extend release packaging**

Package the new guide and configured process sections. Preserve the existing working executables as timestamped rollback copies before deployment. Do not copy over any live executable during the build.

- [ ] **Step 5: Run complete verification**

Run:

```powershell
work\toolchains\go\bin\go.exe test ./...
work\toolchains\go\bin\go.exe vet ./...
powershell -ExecutionPolicy Bypass -File scripts\build-release.ps1
```

Expected: all commands exit 0, both executables are non-empty, manifest resources are present, and source/output hashes reported by the release script are stable.

- [ ] **Step 6: Perform non-destructive Windows smoke tests**

Run the admin against an isolated temporary configuration and fake BDS executable to verify hidden start, log capture, graceful stop input, restart, import validation, staging, apply, and rollback. Do not touch live worlds during automated smoke testing.

- [ ] **Step 7: Stage deployment and provide manual acceptance sequence**

Copy new files beside—not over—the running deployment. After operator-approved downtime, replace executables/config, start the helper, identify existing BDS as externally running, stop each through its old mechanism once, and then start each from the GUI. Complete the nine manual verification steps from the specification before declaring the deployment final.
