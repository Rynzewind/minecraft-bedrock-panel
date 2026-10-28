# NetherNet Account Pool and Windows Admin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a native Windows administrator and secure account pool so PS5 and Switch 2 players can join the same BDS concurrently using stable, distinct Microsoft/Xbox identities.

**Architecture:** The existing helper remains the discovery/WebRTC/Bedrock service and gains an isolated account manager plus a current-user-only Windows named-pipe API. A separate native Windows executable controls enrollment, labels, mappings, and status; tokens remain encrypted and usable only by the service.

**Tech Stack:** Go 1.26, gophertunnel, OAuth2, Windows DPAPI through `golang.org/x/sys/windows`, `github.com/Microsoft/go-winio` named pipes, and `github.com/lxn/walk` native Windows controls.

**Spec:** `docs/superpowers/specs/2026-09-20-nethernet-account-pool-admin-design.md`

## Global Constraints

- Preserve UDP 7551 binding, discovery packet handling, advertisements, Network IDs, persistent server identities, signalling routing, WebRTC implementation, backend mappings, and packet relay behavior.
- `NetherNetMultiServerAdmin.exe` may be opened or closed without interrupting the service or connected players.
- Never request or store Microsoft passwords; email is an operator-entered label only.
- Store refresh tokens only as Windows current-user DPAPI ciphertext; never log tokens, device codes, SDP, ICE credentials, or Minecraft login payloads.
- A helper account and an incoming console XUID may each have at most one active lease.
- Persistent mappings are keyed by the incoming authenticated Xbox XUID and survive restart.
- Reassignment is permitted only while both the player and destination helper account are offline.
- Preserve `C:\MinecraftServers\MultiServer\NetherNetMultiServer.exe` as a rollback artifact until same-server two-console acceptance succeeds.
- No Git commit is required; the user previously waived blocked commits. Record every completed task and ruling in the plan ledger.

## Review Focus

- Two joins racing for the last free account must yield one lease and one clear pool-exhausted disconnect, never duplicate use.
- A crash or forced shutdown during an atomic metadata write must leave either the old or new valid store, never truncated JSON.
- A named-pipe client running as another Windows user must be denied while the same-user admin connects normally.
- An expired or revoked token must mark only that account unhealthy and continue allocation using another healthy account.
- A returning player whose mapped account is temporarily busy must not be silently remapped and lose BDS player continuity.

---

### Task 1: DPAPI Protection and Atomic Account Store

**Files:**
- Create: `internal/accountstore/dpapi_windows.go`
- Create: `internal/accountstore/dpapi_windows_test.go`
- Create: `internal/accountstore/store.go`
- Create: `internal/accountstore/store_test.go`

**Interfaces:**
- Produces: `type Protector interface { Protect([]byte) ([]byte, error); Unprotect([]byte) ([]byte, error) }`
- Produces: `type Account`, `type PlayerMapping`, `type Snapshot`, and `type Store`
- Produces: `Open(directory string, protector Protector) (*Store, error)`, `(*Store).Snapshot() Snapshot`, and `(*Store).Update(func(*Snapshot) error) error`

- [ ] **Step 1: Write failing DPAPI and store tests**

```go
func TestDPAPIRoundTripCurrentUser(t *testing.T) {
    p := DPAPIProtector{Description: "NetherNet Multi-Server account"}
    encrypted, err := p.Protect([]byte("refresh-secret"))
    if err != nil { t.Fatal(err) }
    if bytes.Contains(encrypted, []byte("refresh-secret")) { t.Fatal("plaintext leaked") }
    got, err := p.Unprotect(encrypted)
    if err != nil || string(got) != "refresh-secret" { t.Fatalf("got %q, %v", got, err) }
}

func TestStoreUpdateIsAtomicAndContainsNoPlaintextToken(t *testing.T) {
    store, _ := Open(t.TempDir(), fakeProtector{})
    err := store.Update(func(s *Snapshot) error {
        s.Accounts = append(s.Accounts, Account{ID: "helper-xuid", EncryptedToken: []byte("cipher")})
        return nil
    })
    if err != nil { t.Fatal(err) }
    raw, _ := os.ReadFile(filepath.Join(store.Directory(), "accounts.json"))
    if bytes.Contains(raw, []byte("refresh-secret")) { t.Fatal("plaintext token persisted") }
}
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./internal/accountstore -run 'TestDPAPI|TestStore' -v`

Expected: FAIL because `DPAPIProtector`, `Open`, and store models do not exist.

- [ ] **Step 3: Implement DPAPI and the atomic store**

Use `windows.CryptProtectData`/`windows.CryptUnprotectData` with `CRYPTPROTECT_UI_FORBIDDEN`; copy returned bytes before `windows.LocalFree`. Store only base64 ciphertext in `accounts.json`. Implement updates by cloning the snapshot, validating unique account IDs and mapping targets, writing `accounts.json.tmp` with mode `0600`, flushing, closing, and atomically renaming.

```go
type Account struct {
    ID             string `json:"id"`
    PersonLabel    string `json:"person_label"`
    EmailLabel     string `json:"email_label,omitempty"`
    Gamertag       string `json:"gamertag"`
    XUID           string `json:"xuid"`
    EncryptedToken string `json:"encrypted_token"`
    Order          int    `json:"order"`
}
type PlayerMapping struct {
    ConsoleXUID     string    `json:"console_xuid"`
    ConsoleGamertag string    `json:"console_gamertag"`
    AccountID       string    `json:"account_id"`
    LastSeen        time.Time `json:"last_seen"`
}
type Snapshot struct {
    Version  int             `json:"version"`
    Accounts []Account       `json:"accounts"`
    Players  []PlayerMapping `json:"players"`
}
```

- [ ] **Step 4: Run package tests**

Run: `go test ./internal/accountstore -v`

Expected: PASS, including a test that injects a pre-rename write failure and reloads the previous snapshot.

- [ ] **Step 5: Record checkpoint**

Append `Task 1: complete — DPAPI and atomic account store tests passed` to the plan ledger.

### Task 2: Account Enrollment, Validation, and Single-Token Migration

**Files:**
- Create: `internal/accounts/enrollment.go`
- Create: `internal/accounts/enrollment_test.go`
- Create: `internal/accounts/migrate.go`
- Create: `internal/accounts/migrate_test.go`
- Modify: `internal/authstore/store.go`

**Interfaces:**
- Consumes: `accountstore.Store`, `accountstore.Protector`
- Produces: `type Authenticator interface { Begin(context.Context) (DeviceCode, error); Complete(context.Context, DeviceCode) (*oauth2.Token, XboxIdentity, error); Validate(context.Context, *oauth2.Token) (XboxIdentity, error) }`
- Produces: `Enroll(ctx context.Context, store *accountstore.Store, auth Authenticator, labels Labels) (DeviceCode, func(context.Context) (accountstore.Account, error), error)`
- Produces: `MigrateLegacyToken(ctx context.Context, configDir string, store *accountstore.Store, auth Authenticator) error`

- [ ] **Step 1: Write enrollment and rollback tests**

```go
func TestEnrollmentPersistsOnlyAfterValidation(t *testing.T) {
    auth := &fakeAuthenticator{identity: XboxIdentity{XUID: "28123", Gamertag: "KevsGaming"}}
    code, complete, err := Enroll(context.Background(), store, auth, Labels{Person: "Kevin", Email: "kevin@example.com"})
    if err != nil || code.UserCode == "" { t.Fatal(err) }
    account, err := complete(context.Background())
    if err != nil { t.Fatal(err) }
    if account.Gamertag != "KevsGaming" || account.EmailLabel != "kevin@example.com" { t.Fatalf("%+v", account) }
}

func TestFailedEnrollmentDoesNotModifyStore(t *testing.T) { /* fake Complete returns revoked-token error; assert snapshot unchanged */ }
func TestMigrationRenamesPlaintextOnlyAfterDecryptVerification(t *testing.T) { /* assert auth-token.migrated-backup.json exists */ }
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./internal/accounts -run 'TestEnrollment|TestMigration' -v`

Expected: FAIL because enrollment and migration do not exist.

- [ ] **Step 3: Implement enrollment using gophertunnel authentication**

Split the existing blocking device flow into begin/complete calls. Validate the resulting account by obtaining the Xbox identity and the Minecraft authorization material used by `minecraft.Dialer`. Encrypt the serialized OAuth token through the store protector before persisting.

- [ ] **Step 4: Implement guarded legacy migration**

Read `auth-token.json`, validate it, import it with labels `Existing helper account` and an empty email label, save and decrypt-check the new account record, then rename the source to `auth-token.migrated-backup.json`. If any step fails, retain the original file and roll back the new record.

- [ ] **Step 5: Run account tests**

Run: `go test ./internal/accounts -v`

Expected: PASS for successful enrollment, revoked-token rollback, duplicate-XUID rejection, and migration rollback.

- [ ] **Step 6: Record checkpoint**

Append `Task 2: complete — enrollment validation and legacy migration tests passed` to the ledger.

### Task 3: Persistent Assignment and Concurrent Lease Manager

**Files:**
- Create: `internal/accounts/manager.go`
- Create: `internal/accounts/manager_test.go`

**Interfaces:**
- Consumes: `accountstore.Store`, encrypted OAuth tokens
- Produces: `type Player struct { XUID, Gamertag string }`
- Produces: `type Lease struct { Account accountstore.Account; TokenSource oauth2.TokenSource; Release func() }`
- Produces: `(*Manager).Acquire(ctx context.Context, player Player, server string) (*Lease, error)`
- Produces: `(*Manager).Reassign(consoleXUID, accountID string) error`, `Unpair(consoleXUID string) error`, and `Snapshot() StatusSnapshot`
- Produces sentinel errors `ErrPoolExhausted`, `ErrMappedAccountBusy`, `ErrPlayerActive`, and `ErrAccountAssigned`

- [ ] **Step 1: Write allocation and concurrency tests**

```go
func TestAcquirePersistsStableFirstAvailableAssignment(t *testing.T) {
    manager := newTestManager(accounts("a", "b"))
    first, _ := manager.Acquire(ctx, Player{XUID: "console-1", Gamertag: "Alex"}, "Creative")
    first.Release()
    again, _ := manager.Acquire(ctx, Player{XUID: "console-1", Gamertag: "Alex"}, "Survival")
    if first.Account.ID != again.Account.ID { t.Fatal("assignment changed") }
}

func TestConcurrentAcquireNeverLeasesOneAccountTwice(t *testing.T) { /* start two goroutines against one account; assert one success and one ErrPoolExhausted */ }
func TestMappedBusyPlayerIsNotSilentlyRemapped(t *testing.T) { /* assert ErrMappedAccountBusy although another account is free */ }
func TestUnhealthyAccountFallsThroughToNextHealthyAccount(t *testing.T) { /* revoked first token; second leased */ }
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./internal/accounts -run 'TestAcquire|TestConcurrent|TestMapped|TestUnhealthy' -race -v`

Expected: FAIL because `Manager.Acquire` and lease state do not exist.

- [ ] **Step 3: Implement transactional assignment and in-memory leases**

Hold one mutex across mapping lookup, token health check, candidate selection, durable mapping update, and lease insertion. Sort candidates by `Account.Order` then ID. Make `Release` idempotent with `sync.Once`. Persist assignments but never persist active leases.

- [ ] **Step 4: Implement reassignment safety**

Reject active players, active destination accounts, and accounts mapped to another player. Return a `ReassignmentImpact` containing old/new gamertags for UI confirmation before the caller submits the final mutation.

- [ ] **Step 5: Run manager tests with the race detector**

Run: `go test ./internal/accounts -race -v`

Expected: PASS with exactly one lease in the last-account race.

- [ ] **Step 6: Record checkpoint**

Append `Task 3: complete — stable mappings and race-safe lease manager passed` to the ledger.

### Task 4: Per-Session Authentication Integration

**Files:**
- Modify: `internal/backend/dialer.go`
- Modify: `internal/backend/dialer_test.go`
- Modify: `internal/proxy/session.go`
- Modify: `internal/proxy/session_test.go`
- Modify: `cmd/nethernet-multi-server/main.go`
- Modify: `cmd/nethernet-multi-server/main_test.go`

**Interfaces:**
- Consumes: `accounts.Manager.Acquire`
- Changes: `backend.Dialer.Dial(context.Context, string, *minecraft.Conn, oauth2.TokenSource) (*minecraft.Conn, error)`
- Changes: `proxy.ServeTargets(ctx, targets, dialer, accountLeaser, log)`

- [ ] **Step 1: Write failing proxy lease lifecycle tests**

```go
func TestSessionAcquiresUsingAuthenticatedInboundIdentityAndAlwaysReleases(t *testing.T) {
    leaser := &fakeLeaser{lease: testLease("helper-a")}
    dialer := &fakeDialer{fail: errors.New("backend down")}
    serveTestSession(inboundWithIdentity("console-xuid", "Sam"), dialer, leaser)
    if leaser.gotPlayer.XUID != "console-xuid" || leaser.releaseCalls != 1 { t.Fatalf("%+v", leaser) }
}

func TestPoolExhaustionProducesSpecificDisconnect(t *testing.T) { /* ErrPoolExhausted -> No proxy player account is available */ }
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./internal/backend ./internal/proxy -run 'TestSession|TestPoolExhaustion' -v`

Expected: FAIL because the proxy still uses one process-wide token source.

- [ ] **Step 3: Pass the leased token source into the existing dialer**

Remove `TokenSource` from `NetherNetDialer` state and supply it per `Dial`. Keep endpoint creation, backend URL selection, copied `ClientData`, timeouts, StartGame/DoSpawn, and packet relay unchanged.

- [ ] **Step 4: Acquire and defer release at the session boundary**

Build `accounts.Player` from `inbound.IdentityData().XUID` and `DisplayName`. Reject an empty XUID rather than allocating by mutable name. Acquire before backend dial and `defer lease.Release()` immediately after success.

- [ ] **Step 5: Run proxy and full regression tests**

Run: `go test ./internal/backend ./internal/proxy ./internal/lan ./internal/config -race -v`

Expected: PASS; existing advertisement, routing, listener, and relay tests remain unchanged and green.

- [ ] **Step 6: Record checkpoint**

Append `Task 4: complete — per-session lease integration passed without discovery/relay regression` to the ledger.

### Task 5: Bounded Administration Protocol

**Files:**
- Create: `internal/adminapi/messages.go`
- Create: `internal/adminapi/messages_test.go`
- Create: `internal/adminapi/handler.go`
- Create: `internal/adminapi/handler_test.go`

**Interfaces:**
- Consumes: account manager status, enrollment, labels, reassignment, and unpair operations
- Produces: `type Request struct { ID, Method string; Params json.RawMessage }`
- Produces: `type Response struct { ID string; Result any; Error *APIError }`
- Produces: `ServeConnection(ctx context.Context, rw io.ReadWriter, controller Controller) error`

- [ ] **Step 1: Write framing and validation tests**

```go
func TestServeConnectionRejectsFrameOverOneMiB(t *testing.T) { /* length prefix 1MiB+1 -> ErrFrameTooLarge */ }
func TestMalformedJSONReturnsErrorWithoutStoppingNextRequest(t *testing.T) { /* bad frame then list request */ }
func TestReassignActivePlayerReturnsConflict(t *testing.T) { /* API error code player_active */ }
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./internal/adminapi -v`

Expected: FAIL because the API package does not exist.

- [ ] **Step 3: Implement length-prefixed JSON request/response handling**

Use a four-byte little-endian length prefix with a 1 MiB maximum. Support `status.list`, `account.enroll.begin`, `account.enroll.complete`, `account.reauthenticate`, `account.labels.update`, `account.reorder`, `player.reassign.preview`, `player.reassign.commit`, and `player.unpair`.

- [ ] **Step 4: Add redaction and malformed-request resilience**

Return stable error codes without embedding wrapped OAuth tokens or raw request data. Continue serving after a request-level error; close only for framing errors, EOF, cancellation, or write failure.

- [ ] **Step 5: Run API tests**

Run: `go test ./internal/adminapi -v`

Expected: PASS for bounds, malformed JSON recovery, conflicts, and redaction.

- [ ] **Step 6: Record checkpoint**

Append `Task 5: complete — bounded redacted admin API passed` to the ledger.

### Task 6: Current-User-Only Windows Named Pipe

**Files:**
- Create: `internal/adminpipe/server_windows.go`
- Create: `internal/adminpipe/server_windows_test.go`
- Create: `internal/adminpipe/client_windows.go`
- Create: `internal/adminpipe/client_windows_test.go`

**Interfaces:**
- Consumes: `adminapi.ServeConnection`
- Produces: `ListenCurrentUser(pipeName string) (net.Listener, error)`
- Produces: `DialCurrentUser(ctx context.Context, pipeName string) (net.Conn, error)`
- Pipe name: `\\.\pipe\NetherNetMultiServer.Admin.v1`

- [ ] **Step 1: Write same-user connection and ACL tests**

```go
func TestCurrentUserCanRoundTripStatus(t *testing.T) { /* listener + DialCurrentUser + status response */ }
func TestPipeSecurityDescriptorContainsCurrentUserAndSystemOnly(t *testing.T) { /* inspect constructed SDDL; no WD/AU ACE */ }
func TestClientReconnectsAfterServiceRestart(t *testing.T) { /* close listener, recreate, dial again */ }
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./internal/adminpipe -v`

Expected: FAIL because named-pipe server/client do not exist.

- [ ] **Step 3: Implement the pipe with `go-winio`**

Resolve the current process token user SID and construct a protected SDDL granting generic-all only to `SY` and that SID. Use `winio.ListenPipe` and `winio.DialPipeContext`; do not create a TCP fallback.

- [ ] **Step 4: Run pipe tests on Windows**

Run: `go test ./internal/adminpipe -v`

Expected: PASS. Manually inspect the SDDL test output to confirm no Everyone (`WD`) or Authenticated Users (`AU`) access.

- [ ] **Step 5: Record checkpoint**

Append `Task 6: complete — current-user named pipe and reconnect tests passed` to the ledger.

### Task 7: Native Windows Admin Application

**Files:**
- Create: `cmd/nethernet-multi-server-admin/main_windows.go`
- Create: `cmd/nethernet-multi-server-admin/controller.go`
- Create: `cmd/nethernet-multi-server-admin/controller_test.go`
- Create: `cmd/nethernet-multi-server-admin/window_windows.go`
- Create: `cmd/nethernet-multi-server-admin/resources.rc`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `adminpipe.DialCurrentUser`, admin API request/response types
- Produces: `NetherNetMultiServerAdmin.exe`
- Produces controller methods `Refresh`, `BeginEnrollment`, `CompleteEnrollment`, `UpdateLabels`, `PreviewReassignment`, `CommitReassignment`, `Unpair`, and `Reconnect`

- [ ] **Step 1: Write controller state tests before UI code**

```go
func TestDisconnectedControllerShowsStartServiceMessage(t *testing.T) { /* dial returns file-not-found */ }
func TestRefreshPopulatesAccountAndPlayerRows(t *testing.T) { /* fake API status response */ }
func TestReassignRequiresImpactConfirmation(t *testing.T) { /* preview text includes inventory/permissions warning */ }
func TestEnrollmentNeverPlacesTokenOrDeviceCodeInAccountGrid(t *testing.T) { /* inspect view model */ }
```

- [ ] **Step 2: Run controller tests and confirm RED**

Run: `go test ./cmd/nethernet-multi-server-admin -v`

Expected: FAIL because the controller does not exist.

- [ ] **Step 3: Implement the testable controller**

Keep pipe calls and presentation state outside Walk widgets. Expose immutable `ViewState` containing service status, account rows, player rows, active-session count, and sanitized event text.

- [ ] **Step 4: Build the native Walk window**

Create tabs for Accounts and Players, a service-status banner, account Add/Reauthenticate/Edit/Remove buttons, player Reassign/Unpair buttons, confirmation dialogs, and a redacted event area. Disable invalid actions based on `ViewState`. Poll status every two seconds while connected and reconnect with capped backoff while disconnected.

- [ ] **Step 5: Build and smoke-test the Windows executable**

Run: `go test ./cmd/nethernet-multi-server-admin -v`

Run: `go build -buildvcs=false -trimpath -ldflags="-H windowsgui -s -w" -o outputs/NetherNetMultiServerAdmin.exe ./cmd/nethernet-multi-server-admin`

Expected: tests PASS and PE build succeeds without opening a console window.

- [ ] **Step 6: Record checkpoint**

Append `Task 7: complete — native admin controller tests and Windows GUI build passed` to the ledger.

### Task 8: Service Wiring, Migration, and Status Events

**Files:**
- Modify: `cmd/nethernet-multi-server/main.go`
- Modify: `cmd/nethernet-multi-server/main_test.go`
- Create: `internal/service/runtime.go`
- Create: `internal/service/runtime_test.go`
- Modify: `internal/proxy/session.go`

**Interfaces:**
- Consumes: account store, manager, admin API, named-pipe listener, proxy account leaser
- Produces: `service.Start(ctx, configPath, logger) (*Runtime, error)` and `(*Runtime).Close() error`

- [ ] **Step 1: Write startup ordering and isolation tests**

```go
func TestAccountStoreFailureOccursBeforeUDPBind(t *testing.T) { /* corrupt encrypted store; assert listenPacket not called */ }
func TestAdminDisconnectDoesNotStopDiscoveryOrActiveSession(t *testing.T) { /* close pipe client; service context remains live */ }
func TestSessionStatusIncludesSelectedBackendAndLeaseLabels(t *testing.T) { /* connect/relay/disconnect event sequence */ }
```

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./internal/service ./cmd/nethernet-multi-server -v`

Expected: FAIL because runtime orchestration does not exist.

- [ ] **Step 3: Wire initialization before UDP binding**

Open the DPAPI store, run guarded legacy migration, create the manager, start the named-pipe listener, then initialize the unchanged UDP service/listeners and proxy. Preserve the current console banner and server list; add account-pool count and admin-pipe status without displaying identities or tokens.

- [ ] **Step 4: Publish sanitized session state**

Publish connecting/active/offline transitions from session boundaries. Include console XUID/gamertag, helper labels/gamertag, backend name, and timestamps. Never include remote SDP, ICE data, OAuth errors containing responses, or tokens.

- [ ] **Step 5: Run service and regression tests**

Run: `go test ./... -race`

Expected: PASS across the complete project.

- [ ] **Step 6: Record checkpoint**

Append `Task 8: complete — service wiring, startup ordering, and full race suite passed` to the ledger.

### Task 9: Packaging, Rollback, and Real-Device Acceptance

**Files:**
- Modify: `outputs/README.txt`
- Create: `outputs/ACCOUNT-POOL-SETUP.txt`
- Create: `scripts/build-release.ps1`
- Modify: `.superpowers/sdd/2026-09-20-nethernet-account-pool-admin/progress.md`

**Interfaces:**
- Consumes: both executable entry points and the existing deployment layout
- Produces: final service/admin executables and operator instructions

- [ ] **Step 1: Write release-script validation tests**

Add Pester-independent PowerShell checks inside `build-release.ps1` that stop on failure, run `go test ./... -race`, build both PE files with `-buildvcs=false -trimpath`, verify non-zero sizes, and print SHA-256 hashes.

- [ ] **Step 2: Preserve rollback and build candidates**

Copy the currently proven deployed executable to `outputs/rollback/NetherNetMultiServer-single-account.exe` without deleting or overwriting it. Build candidates as `outputs/NetherNetMultiServer-AccountPool.exe` and `outputs/NetherNetMultiServerAdmin.exe`.

- [ ] **Step 3: Write exact operator instructions**

Document service startup, opening the admin app, migrating the existing account, adding a second account, editing person/email labels, interpreting health/lease states, automatic pairing, reassignment consequences, DPAPI backup limitations, firewall scope, and rollback.

- [ ] **Step 4: Run the release verification script**

Run: `powershell -ExecutionPolicy Bypass -File .\scripts\build-release.ps1`

Expected: all Go tests PASS, both executables build, and SHA-256 hashes print.

- [ ] **Step 5: Perform real-device acceptance without replacing the deployed rollback**

Enroll two helper accounts. Join Creative simultaneously from PS5 and Switch 2. Confirm two distinct helper gamertags, stable mappings after reconnect, and no duplicate-player rejection. Repeat on Survival. Restart the service and confirm mappings persist. Reassign one offline test player and confirm the warning plus expected identity change.

- [ ] **Step 6: Deploy only after acceptance**

Stop the old service, copy the verified service/admin candidates and documentation into `C:\MinecraftServers\MultiServer`, retain `NetherNetMultiServer-single-account.exe`, start the new service, and verify both LAN entries before declaring completion.

- [ ] **Step 7: Record completion**

Append the final test counts, hashes, real-device results, deployment path, rollback path, and every ruling to the ledger.
