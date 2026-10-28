# NetherNet Single-Window Administrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current prototype administration window with one clear native Windows screen that shows the account pool and console-user assignments together while leaving the server process independent.

**Architecture:** Preserve the existing account manager, per-session leasing, encrypted store, named-pipe protocol, discovery, and proxy code. Split the admin presentation into a testable view-model/controller and a native Walk window containing two side-by-side tables plus a read-only server-status strip. The admin executable remains optional and never starts or stops `NetherNetMultiServer.exe`.

**Tech Stack:** Go 1.26, `github.com/lxn/walk`, `github.com/Microsoft/go-winio`, Windows DPAPI, existing length-bounded JSON named-pipe API.

**Spec:** `docs/superpowers/specs/2026-09-20-nethernet-account-pool-admin-design.md`

## Global Constraints

- Do not change UDP 7551 discovery, advertisements, Network IDs, server identities, signaling, backend routing, visual profile, or packet relay behavior.
- `NetherNetMultiServer.exe` remains an independent process; the admin application has no Start, Stop, or Restart command.
- Closing or disconnecting the admin application must not affect discovery or active sessions.
- Email addresses are operator labels only. Never request or store Microsoft passwords.
- Never display or log OAuth tokens, device codes after enrollment, SDP, ICE credentials, or Minecraft login payloads.
- First-time authenticated console XUIDs continue receiving the first healthy free account automatically.
- Reassignment remains unavailable while the player or destination helper account is active.

## Review Focus

- Service absent at startup: the complete window still opens, both lists remain usable for inspection, and the status strip clearly says disconnected without repeated log spam.
- Refresh changes row ordering: the selected account/player IDs remain correct and an action can never target a different row merely because the display was sorted.
- Empty account or player list: selection-dependent buttons are disabled and no index panic occurs.
- Long person labels, emails, and gamertags: values remain readable without breaking the two-panel layout.
- Panel closes during an in-flight refresh or enrollment: background callbacks do not access disposed Walk controls and the server is unaffected.

---

### Task 1: Testable Administrator View Model

**Files:**
- Create: `cmd/nethernet-multi-server-admin/viewmodel.go`
- Create: `cmd/nethernet-multi-server-admin/viewmodel_test.go`
- Modify: `cmd/nethernet-multi-server-admin/client_windows.go`

**Interfaces:**
- Consumes: `service.Status`, `service.PublicAccount`, `accountstore.PlayerMapping`, `accounts.ActiveLease`
- Produces: `type AccountRow struct { ID, PersonLabel, EmailLabel, Gamertag, XUID, State, CurrentPlayer, Backend string }`
- Produces: `type PlayerRow struct { XUID, Gamertag, AccountID, AccountDisplay, State, LastSeen string }`
- Produces: `type ViewState struct { Connected bool; StatusText string; Accounts []AccountRow; Players []PlayerRow; ActiveCount int; CreativeActive, SurvivalActive int }`
- Produces: `BuildViewState(service.Status) ViewState`
- Produces: selection helpers that use stable IDs rather than displayed indexes.

- [ ] **Step 1: Write failing transformation and selection tests**

```go
func TestBuildViewStateShowsBothSidesOfAssignment(t *testing.T) {
    status := service.Status{
        Accounts: []service.PublicAccount{{ID:"a", PersonLabel:"Kevin", EmailLabel:"kevin@example.com", Gamertag:"HelperOne", State:"In use"}},
        Players: []accountstore.PlayerMapping{{ConsoleXUID:"p", ConsoleGamertag:"KevPS5", AccountID:"a"}},
        Active: []accounts.ActiveLease{{ConsoleXUID:"p", AccountID:"a", Server:"Private Creative Server"}},
    }
    state := BuildViewState(status)
    if state.Accounts[0].CurrentPlayer != "KevPS5" || state.Players[0].AccountDisplay != "Kevin / kevin@example.com / HelperOne" { t.Fatalf("%+v", state) }
}

func TestSortedRowsRetainStableIDs(t *testing.T) { /* input b,a; display sorted by order; assert actions return IDs b/a attached to displayed rows */ }
func TestEmptyViewDisablesSelectionActions(t *testing.T) { /* no rows -> CanEdit/CanReassign false */ }
func TestDisconnectedViewHasSingleUsefulMessage(t *testing.T) { /* Connected false and no synthetic account/player rows */ }
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./cmd/nethernet-multi-server-admin -run 'TestBuildViewState|TestSorted|TestEmpty|TestDisconnected' -v`

Expected: FAIL because `ViewState` and `BuildViewState` do not exist.

- [ ] **Step 3: Implement immutable rows keyed by IDs**

Build account and player rows from one `service.Status` snapshot. Resolve mappings through account ID maps, resolve active states through console XUID and account ID maps, and sort copies rather than mutating the source snapshot. Truncate only the displayed event/status text, never the stored labels.

- [ ] **Step 4: Add disposal-safe refresh coordination**

Add a controller generation/cancellation token so only the newest refresh updates the view. Return quietly when the window closes or the service is unavailable. Coalesce identical disconnected errors instead of appending one every two seconds.

- [ ] **Step 5: Run package tests**

Run: `go test ./cmd/nethernet-multi-server-admin -v`

Expected: PASS for mapping visibility, stable IDs, empty lists, disconnected state, and stale-refresh suppression.

### Task 2: Two-Panel Native Windows Window

**Files:**
- Replace: `cmd/nethernet-multi-server-admin/main_windows.go`
- Create: `cmd/nethernet-multi-server-admin/window_windows.go`
- Create: `cmd/nethernet-multi-server-admin/window_windows_test.go`

**Interfaces:**
- Consumes: `ViewState`, stable account/player IDs, existing `pipeClient.Call`
- Produces: one main window with `Account Pool` and `Console Users` tables visible simultaneously
- Produces: Add Account, Edit, Reauthenticate, Remove, Assign/Reassign, Unpair, and Refresh actions

- [ ] **Step 1: Write failing declarative layout/action tests**

```go
func TestMainWindowHasBothPanelsAndNoServerControl(t *testing.T) {
    spec := mainWindowSpec()
    if !spec.HasAccountPool || !spec.HasConsoleUsers { t.Fatal("both panels must be visible") }
    if spec.HasStartStopControl { t.Fatal("admin must not control server lifecycle") }
}

func TestReassignUsesSelectedStableIDs(t *testing.T) { /* select displayed rows after sort; assert API params contain row IDs */ }
func TestActiveOrIncompleteSelectionDisablesReassign(t *testing.T) { /* active player, busy account, or missing selection -> disabled */ }
func TestClosingWindowCancelsCallbacks(t *testing.T) { /* cancel controller, deliver late refresh, assert no UI update */ }
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./cmd/nethernet-multi-server-admin -run 'TestMainWindow|TestReassign|TestActive|TestClosing' -v`

Expected: FAIL because the new window specification and action-state functions do not exist.

- [ ] **Step 3: Build the single-window layout**

Use a top status strip, a horizontal splitter with native table views for Account Pool and Console Users, action buttons directly beneath their related table, and one compact redacted activity area at the bottom. Columns show the exact labels required by the spec. Use horizontal resizing and sensible minimum column widths for long labels.

- [ ] **Step 4: Bind actions to stable row IDs**

Add account opens the existing person/email plus Microsoft device-code flow. Edit and Reauthenticate operate on the selected account ID. Assign/Reassign requires both selected IDs and the existing impact confirmation. Unpair operates on the selected console XUID. Disable invalid actions from `ViewState` rather than showing avoidable error dialogs.

- [ ] **Step 5: Keep the server independent**

The window only calls the existing named-pipe API. Do not add process launching, service management, UDP calls, or server shutdown messages. The disconnected status reads `Server administrator connection unavailable — the server may be stopped or running an older build.`

- [ ] **Step 6: Run UI/controller tests and build**

Run: `go test ./cmd/nethernet-multi-server-admin -v`

Run: `go build -buildvcs=false -trimpath -ldflags="-H windowsgui -s -w" -o outputs/NetherNetMultiServerAdmin.exe ./cmd/nethernet-multi-server-admin`

Expected: tests PASS and the native PE executable builds without a console window.

### Task 3: Integration, Packaging, and Operator Acceptance

**Files:**
- Modify: `outputs/README.txt`
- Modify: `outputs/ACCOUNT-POOL-SETUP.txt`
- Modify: `scripts/build-release.ps1`
- Modify: `.superpowers/sdd/2026-09-20-nethernet-admin-single-window/progress.md`

**Interfaces:**
- Consumes: unchanged account-pool service candidate and redesigned admin executable
- Produces: final test package and simple operator workflow

- [ ] **Step 1: Add an admin/server independence integration test**

Start a test named-pipe controller, connect and close an admin client, then assert the controller context and a simulated active session remain live. Reconnect a second client and assert the same assignments are returned.

- [ ] **Step 2: Run complete regression verification**

Run: `go vet ./...`

Run: `go test ./...`

Expected: PASS across admin, encrypted storage, leasing, named pipe, discovery, socket, listener, backend routing, visual profile, and packet relay packages.

- [ ] **Step 3: Build and hash both executables**

Run: `powershell -ExecutionPolicy Bypass -File .\scripts\build-release.ps1`

Expected: non-empty `NetherNetMultiServer-AccountPool.exe` and `NetherNetMultiServerAdmin.exe`, with SHA-256 hashes printed; rollback hash remains `6CDB3ABCCAE4FBEDC45182A139FA33C20BC1980774C387F08A6F2CB6879C1FA2`.

- [ ] **Step 4: Smoke-test the native window**

Open the admin executable with no service and confirm the full two-panel window appears in disconnected mode. Start the candidate server separately, confirm the same window connects automatically, and close the window while the server remains running.

- [ ] **Step 5: Update the operator instructions**

Document only the ordinary workflow: start the server through the user's eventual external launcher, open the separate administrator panel when account changes are needed, view both lists, add accounts, and reassign offline users. Keep the detailed rollback and security notes in a separate troubleshooting section.

- [ ] **Step 6: Real-device acceptance before deployment**

Enroll two helper accounts, let PS5 and Switch 2 receive automatic assignments, join the same Creative server simultaneously, repeat on Survival, reconnect to verify stable assignments, and perform one offline reassignment from the two-panel window. Deploy to `C:\MinecraftServers\MultiServer` only after these checks succeed.
