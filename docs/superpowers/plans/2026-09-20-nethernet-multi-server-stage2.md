# NetherNet Multi-Server Stage 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Accept PS5 and Nintendo Switch 2 NetherNet connections for either advertised identity and relay each session to its configured BDS backend.

**Architecture:** One proven IPv4 UDP 7551 service continues discovery and additionally demultiplexes signalling by recipient network ID into per-server `go-nethernet.Listener` instances. Each accepted WebRTC connection is paired with a gophertunnel NetherNet connection to the selected BDS and relayed at the Bedrock packet layer. Server identity keys persist by network ID.

**Tech Stack:** Go 1.26, `github.com/df-mc/go-nethernet`, Pion WebRTC through go-nethernet, `github.com/sandertv/gophertunnel`, Windows Winsock, JSON configuration.

**Spec:** `docs/superpowers/specs/2026-09-20-nethernet-multi-server-stage2-design.md`

## Global Constraints

- Preserve the proven IPv4 UDP 7551 socket and Stage 1 advertisement encoding.
- Creative maps only to `192.168.1.246:19134`; Survival maps only to `192.168.1.247:19136`.
- Both BDS instances retain `transport=nethernet` and `enable-lan-visibility=false`.
- Use upstream libraries for discovery framing, identity assertions, ICE, DTLS, SCTP, WebRTC, and Bedrock packets.
- Never log private keys, identity tokens, ICE passwords, complete SDP, JWTs, or raw login packets.
- Preserve the current working executable as `outputs/NetherNetMultiServer-Stage1.exe` before replacing the normal output.
- No Git commit is required.

## Review Focus

- Two consoles using the same advertised server concurrently must keep distinct source addresses and connection IDs.
- An empty/Ping discovery message must not enter the signalling parser or create a negotiation.
- A stale client-address mapping must expire rather than route answers to an old console address.
- Backend failure must close only that session and leave discovery and other servers running.
- Identity reload after restart must produce the same public-key fingerprint and never expose key material in logs.

---

### Task 1: Preserve Stage 1 and Establish Stage 2 Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/nethernet-multi-server/main.go`
- Modify: `cmd/nethernet-multi-server/main_test.go`
- Modify: `scripts/build.ps1`

**Interfaces:**
- Produces: `Config.IdentityDirectory(configPath string) string`
- Produces: `Server.BackendURL() string`

- [ ] **Step 1: Copy the proven executable before any Stage 2 build**

Run:

```powershell
Copy-Item -LiteralPath .\outputs\NetherNetMultiServer.exe -Destination .\outputs\NetherNetMultiServer-Stage1.exe -Force
Get-FileHash .\outputs\NetherNetMultiServer-Stage1.exe -Algorithm SHA256
```

Expected SHA-256: `102B8A7F505FF85E3407DAFD04312B100A10669E173A129CC3742050D1B5F1BE`.

- [ ] **Step 2: Add failing configuration tests**

Add tests asserting:

```go
func TestIdentityDirectoryIsBesideConfig(t *testing.T) {
    path := filepath.Join(`C:\portable\nethernet`, "config.json")
    if got := IdentityDirectory(path); got != filepath.Join(filepath.Dir(path), "identities") {
        t.Fatalf("IdentityDirectory() = %q", got)
    }
}

func TestServerBackendURL(t *testing.T) {
    s := Server{Backend: "192.168.1.246:19134"}
    if got := s.BackendURL(); got != "http://192.168.1.246:19134" {
        t.Fatalf("BackendURL() = %q", got)
    }
}
```

Also update the main test to expect a Stage 2 banner without weakening its assertions for `udp4`, `SO_BROADCAST`, both advertisements, backends, and network IDs.

- [ ] **Step 3: Run the focused tests and confirm they fail**

Run:

```powershell
& .\work\toolchains\go\bin\go.exe test .\internal\config .\cmd\nethernet-multi-server
```

Expected: compile failures for `IdentityDirectory` and `BackendURL`, plus the old Stage 1 banner expectation.

- [ ] **Step 4: Implement the minimal configuration helpers**

Implement:

```go
func IdentityDirectory(configPath string) string {
    absolute, err := filepath.Abs(configPath)
    if err != nil {
        return filepath.Join(filepath.Dir(configPath), "identities")
    }
    return filepath.Join(filepath.Dir(absolute), "identities")
}

func (s Server) BackendURL() string {
    return "http://" + s.Backend
}
```

Change startup text to `NetherNet Multi-Server` and `Stage 2 connection proxy`, but do not change socket creation or advertisements. Change `scripts/build.ps1` so it never deletes or overwrites `NetherNetMultiServer-Stage1.exe`.

- [ ] **Step 5: Run focused tests**

Run the command from Step 3. Expected: PASS.

### Task 2: Persistent Per-Server Identity Store

**Files:**
- Create: `internal/identity/store.go`
- Create: `internal/identity/store_test.go`

**Interfaces:**
- Produces: `type Store struct { Directory string }`
- Produces: `func (s Store) LoadOrCreate(networkID uint64) (*ecdsa.PrivateKey, string, error)`
- Produces: `func Path(directory string, networkID uint64) string`

- [ ] **Step 1: Write identity lifecycle tests**

Tests must verify:

```go
func TestLoadOrCreatePersistsIdentity(t *testing.T) {
    store := Store{Directory: t.TempDir()}
    first, firstFingerprint, err := store.LoadOrCreate(13468123134000001934)
    if err != nil { t.Fatal(err) }
    second, secondFingerprint, err := store.LoadOrCreate(13468123134000001934)
    if err != nil { t.Fatal(err) }
    if firstFingerprint != secondFingerprint || first.D.Cmp(second.D) != 0 {
        t.Fatal("identity changed after reload")
    }
}
```

Add cases proving different network IDs get different keys, corrupt PEM fails closed, and the filename is exactly `<decimal-network-id>.pem`.

- [ ] **Step 2: Run the identity tests and confirm failure**

```powershell
& .\work\toolchains\go\bin\go.exe test .\internal\identity
```

Expected: package or symbol does not exist.

- [ ] **Step 3: Implement the identity store**

Use `ecdsa.GenerateKey(elliptic.P384(), rand.Reader)`, `x509.MarshalECPrivateKey`, PEM type `EC PRIVATE KEY`, and `os.WriteFile(path, pemBytes, 0o600)`. Create the directory with `0o700`. Refuse files that are not P-384 EC keys. Derive the returned fingerprint with `ssh.NewPublicKey` and `ssh.FingerprintSHA256`.

Write through a same-directory temporary file opened with `O_CREATE|O_EXCL`, close it, then rename it to the final path so interruption cannot leave a partial key.

- [ ] **Step 4: Run identity tests**

Expected: PASS.

### Task 3: Shared UDP Signalling Router

**Files:**
- Create: `internal/lan/signaling.go`
- Create: `internal/lan/signaling_test.go`
- Modify: `internal/lan/service.go`
- Modify: `internal/lan/service_test.go`

**Interfaces:**
- Produces: `func (s *Service) Signaling(networkID uint64) nethernet.Signaling`
- Internal type: `signalingEndpoint`
- Internal method: `dispatchMessage(senderID uint64, packet *discovery.MessagePacket, addr net.Addr) error`

The adapter must implement:

```go
type Signaling interface {
    Signal(context.Context, *nethernet.Signal) error
    Notify(nethernet.Notifier) func()
    Context() context.Context
    Credentials(context.Context) (*nethernet.Credentials, error)
    NetworkID() string
    PongData([]byte)
}
```

- [ ] **Step 1: Add failing routing tests**

Create a notifier stub that records `*nethernet.Signal`. Test that a marshalled message:

```go
&discovery.MessagePacket{
    RecipientID: creativeID,
    Data: "CONNECTREQUEST 16495077517502677389 sample-offer",
}
```

sent by client ID `3829603440984922419` reaches only Creative's notifier and arrives as:

```go
nethernet.Signal{
    Type: nethernet.SignalTypeOffer,
    ConnectionID: 16495077517502677389,
    Data: "sample-offer",
    NetworkID: "3829603440984922419",
}
```

Add tests for Survival isolation, unknown recipient IDs, empty/Ping messages, malformed signal text, notifier unsubscription, and concurrent registration/removal.

- [ ] **Step 2: Add failing outgoing signal tests**

After an incoming packet maps client ID to `192.168.1.125:7551`, call Creative's `Signal` with a `CONNECTRESPONSE`. Assert the UDP write decodes as a `MessagePacket` whose sender is Creative's network ID, recipient is the PS5 client ID, destination is `192.168.1.125:7551`, and data equals `signal.String()`.

Add a stale-address test using an injected clock and a five-minute address lifetime. After expiry, `Signal` must return `no address found for network ID`.

- [ ] **Step 3: Run LAN tests and confirm failure**

```powershell
& .\work\toolchains\go\bin\go.exe test .\internal\lan
```

- [ ] **Step 4: Implement the router and adapter**

Extend `Service` with:

```go
type clientAddress struct {
    addr net.Addr
    seen time.Time
}

endpoints map[uint64]*signalingEndpoint
clients   map[uint64]clientAddress
```

Use separate mutexes for endpoint subscriptions and client addresses. Update the client address after every successfully decoded non-self packet. `Credentials` returns `(nil, nil)`. `PongData` is a no-op because Stage 1 advertisement data remains authoritative. Service cancellation closes a shared context used by every adapter.

Parse non-empty message data with `nethernet.Signal.UnmarshalText`, set `Signal.NetworkID` to the decimal sender ID, and notify only the endpoint matching `RecipientID`.

- [ ] **Step 5: Run all LAN tests**

Expected: existing Stage 1 tests and new signalling tests PASS.

### Task 4: Per-Server NetherNet Listener and Identity Wiring

**Files:**
- Create: `internal/proxy/listeners.go`
- Create: `internal/proxy/listeners_test.go`
- Modify: `cmd/nethernet-multi-server/main.go`
- Modify: `cmd/nethernet-multi-server/main_test.go`

**Interfaces:**
- Consumes: `Service.Signaling(uint64)` and `identity.Store.LoadOrCreate`
- Produces: `type Target struct { Name, BackendURL string; NetworkID uint64; Listener net.Listener }`
- Produces: `func StartListeners(ctx context.Context, service *lan.Service, servers []config.Server, identities identity.Store, log *slog.Logger) ([]Target, error)`

- [ ] **Step 1: Write failing listener-construction tests**

Inject a listener factory:

```go
type listenerFactory func(nethernet.ListenConfig, nethernet.Signaling) (net.Listener, error)
```

Assert one call per server, exact network ID, exact backend URL, distinct P-384 keys, persistent fingerprints on a second start, and cleanup of already-created listeners when a later listener fails.

- [ ] **Step 2: Run proxy tests and confirm failure**

```powershell
& .\work\toolchains\go\bin\go.exe test .\internal\proxy
```

- [ ] **Step 3: Implement listener construction**

For each server, call `nethernet.ListenConfig.Listen(service.Signaling(networkID))`. Set `IssueServerIdentity` to a closure that returns `nethernet.GenerateServerIdentity(privateKey, "self")`. Use 15-second negotiation and connection contexts. Do not enable anonymous clients.

Log server name, network ID, backend, and public fingerprint only.

- [ ] **Step 4: Wire listener startup into main**

Create the LAN service before listeners, create the identity store beside the selected configuration, start listeners, and then run the UDP service. On shutdown, close listeners before the shared service/socket.

- [ ] **Step 5: Run all automated tests**

```powershell
& .\work\toolchains\go\bin\go.exe test .\...
```

Expected: PASS.

### Task 5: Stage 2A Windows Build and Inbound WebRTC Proof

**Files:**
- Modify: `scripts/build.ps1`
- Modify: `outputs/README.txt`

**Interfaces:**
- Produces: `outputs/NetherNetMultiServer-Stage2A.exe`

- [ ] **Step 1: Add a Stage 2A output name**

Build the current implementation as `NetherNetMultiServer-Stage2A.exe`; do not replace the standard executable yet.

- [ ] **Step 2: Run the complete automated suite and build**

```powershell
.\scripts\build.ps1
Get-FileHash .\outputs\NetherNetMultiServer-Stage1.exe, .\outputs\NetherNetMultiServer-Stage2A.exe -Algorithm SHA256
```

Expected: tests PASS, the two binaries have different hashes, and the Stage 1 hash remains unchanged.

- [ ] **Step 3: Perform the PS5 Creative acceptance probe**

Run Stage 2A with both BDS instances running and LAN visibility disabled. Select Creative on PS5. Required evidence:

- `CONNECTREQUEST` routes only to Creative.
- A `CONNECTRESPONSE` is sent from Creative's network ID.
- ICE, DTLS, SCTP, and both data channels reach the connected state.
- The Creative listener reports an accepted connection.

If this fails, stop and diagnose the captured negotiation before adding backend code.

- [ ] **Step 4: Repeat the inbound proof for Survival**

Required evidence is identical except that the recipient and accepted listener are Survival. Preserve both console logs in `outputs`.

### Task 6: Backend NetherNet Dial Boundary

**Files:**
- Create: `internal/backend/dialer.go`
- Create: `internal/backend/dialer_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: `type Dialer interface { Dial(context.Context, string, *minecraft.Conn) (*minecraft.Conn, error) }`
- Produces: `type NetherNetDialer struct { NewEndpointClient func() *endpoint.Client }`
- Produces: `func (d NetherNetDialer) Dial(ctx context.Context, backendURL string, inbound *minecraft.Conn) (*minecraft.Conn, error)`

- [ ] **Step 1: Pin gophertunnel and inspect compatibility**

Run:

```powershell
& .\work\toolchains\go\bin\go.exe get github.com/sandertv/gophertunnel@master
& .\work\toolchains\go\bin\go.exe mod tidy
& .\work\toolchains\go\bin\go.exe list -m all
```

Confirm the resolved gophertunnel version uses the same compatible `go-nethernet` major API. If module resolution attempts to downgrade the pinned Stage 1 discovery commit, explicitly restore `github.com/df-mc/go-nethernet@e22519df17b3ace63541d84b50aaceea775d000e` and rerun `go mod tidy`.

- [ ] **Step 2: Write failing backend dial tests**

Use an injected endpoint client/network dial function. Assert the backend URL is exact, the inbound `ClientData()` is copied to `minecraft.Dialer`, context cancellation terminates the dial, and errors include the server URL without tokens.

- [ ] **Step 3: Implement the backend dialer**

Follow gophertunnel's NetherNet proxy example:

```go
client := endpoint.NewClient()
return minecraft.Dialer{
    ClientData: inbound.ClientData(),
}.DialContextNetwork(ctx, minecraft.NetherNet{Signaling: client}, backendURL)
```

Keep endpoint-client construction injectable for tests. Do not add Microsoft account sign-in to the helper.

- [ ] **Step 4: Run backend and full tests**

```powershell
& .\work\toolchains\go\bin\go.exe test .\internal\backend .\...
```

- [ ] **Step 5: Probe both live BDS endpoints independently**

Use a development command or focused integration test that dials each configured backend with a bounded context and logs only the HTTP status/error class. Verify Creative and Survival independently. If BDS does not expose the endpoint format expected by `endpoint.Client`, stop at this boundary and capture the request/response behavior; do not replace go-nethernet's WebRTC implementation.

### Task 7: Bedrock Session Proxy

**Files:**
- Create: `internal/proxy/session.go`
- Create: `internal/proxy/session_test.go`
- Modify: `internal/proxy/listeners.go`

**Interfaces:**
- Consumes: accepted `net.Conn`, selected target, and `backend.Dialer`
- Produces: `func ServeTarget(ctx context.Context, target Target, dialer backend.Dialer, log *slog.Logger) error`
- Produces: `func relay(ctx context.Context, inbound, upstream *minecraft.Conn, listener *minecraft.Listener) error`

- [ ] **Step 1: Write failing lifecycle tests**

Use fake listener/connections to prove:

- Accepted Creative sessions dial only Creative's backend URL.
- Accepted Survival sessions dial only Survival's backend URL.
- Client-controlled packet data cannot alter the backend.
- Backend dial failure closes only the inbound session and continues accepting.
- Cancellation closes the listener and active session.

- [ ] **Step 2: Write failing bidirectional relay tests**

Use packet-channel fakes to assert packets move in both directions, the first direction to fail cancels the peer direction, disconnect errors are propagated, and both connections close exactly once.

- [ ] **Step 3: Implement the Minecraft listener wrapper**

Wrap each target's `go-nethernet.Listener` with gophertunnel's `minecraft.ListenConfig.ListenNetwork`. Accept `*minecraft.Conn` values and pass them with the immutable target to the session handler.

- [ ] **Step 4: Implement session establishment and relay**

Follow the upstream proxy ordering:

```go
upstream, err := dialer.Dial(ctx, target.BackendURL, inbound)
// Concurrently: inbound.StartGame(upstream.GameData()) and upstream.DoSpawn().
// Then relay ReadPacket/WritePacket in both directions until either side ends.
```

Use one cancellation context per session and a buffered result channel so neither relay goroutine leaks. Recover panics at the per-session goroutine boundary, log the server and connection ID, and close both sides.

- [ ] **Step 5: Run proxy and full tests**

```powershell
& .\work\toolchains\go\bin\go.exe test -race .\internal\proxy
& .\work\toolchains\go\bin\go.exe test .\...
```

Expected: PASS. If the Windows toolchain does not support `-race`, record that limitation and still run the normal suite.

### Task 8: Logging Redaction and Operational Hardening

**Files:**
- Create: `internal/logsafe/signal.go`
- Create: `internal/logsafe/signal_test.go`
- Modify: `internal/lan/service.go`
- Modify: `internal/proxy/session.go`

**Interfaces:**
- Produces: `func SignalSummary(*nethernet.Signal) []slog.Attr`
- Produces: `func Error(err error) string`

- [ ] **Step 1: Write redaction tests**

Feed a realistic offer containing `a=ice-pwd`, `a=identity`, JWT-like strings, and SDP. Assert logs retain signal type, connection ID, network IDs, SDP byte count, and candidate count while excluding the password, token, fingerprint assertion body, and raw SDP.

- [ ] **Step 2: Implement structured summaries**

Normal Stage 2 logging must never emit `Signal.Data` or the full signalling datagram. Retain the existing raw UDP diagnostic capability only behind an explicit `diagnostic_raw_udp` configuration flag defaulting to `false`; even when enabled, print at most the existing 256 encrypted bytes, never decrypted tokens.

- [ ] **Step 3: Add failure-isolation tests**

Inject malformed messages, unknown recipients, one backend outage, and a panicking test session. Assert discovery answers continue and a second target accepts a connection afterward.

- [ ] **Step 4: Run the complete test suite**

```powershell
& .\work\toolchains\go\bin\go.exe test .\...
```

Expected: PASS.

### Task 9: Final Build, Documentation, and Console Acceptance

**Files:**
- Modify: `README.md`
- Modify: `outputs/README.txt`
- Modify: `config.example.json`
- Modify: `outputs/config.json`
- Modify: `scripts/build.ps1`

**Interfaces:**
- Produces: `outputs/NetherNetMultiServer.exe`
- Preserves: `outputs/NetherNetMultiServer-Stage1.exe`

- [ ] **Step 1: Update operator documentation**

Document:

- Both BDS instances must use NetherNet with LAN visibility disabled.
- The helper alone owns UDP 7551.
- Windows Firewall must allow the helper on Private networks.
- `identities` must be backed up with `config.json`.
- A changed/deleted identity may cause a one-time console trust prompt.
- Startup, graceful shutdown, backend-unavailable behavior, and rollback command.

- [ ] **Step 2: Run formatting, tests, and build**

```powershell
& .\work\toolchains\go\bin\gofmt.exe -w .\cmd .\internal
& .\work\toolchains\go\bin\go.exe vet .\...
& .\work\toolchains\go\bin\go.exe test .\...
.\scripts\build.ps1
```

Expected: all commands succeed and the final EXE is rebuilt.

- [ ] **Step 3: Verify deliverables and rollback integrity**

```powershell
Get-FileHash .\outputs\NetherNetMultiServer.exe, .\outputs\NetherNetMultiServer-Stage1.exe -Algorithm SHA256
Get-Item .\outputs\NetherNetMultiServer.exe, .\outputs\config.json, .\outputs\README.txt
```

Expected: Stage 1 retains SHA-256 `102B8A7F505FF85E3407DAFD04312B100A10669E173A129CC3742050D1B5F1BE`.

- [ ] **Step 4: PS5 acceptance**

Join Creative and verify the Creative world; join Survival and verify the Survival world. Restart the helper and verify identity fingerprints are unchanged and both joins still work.

- [ ] **Step 5: Nintendo Switch 2 acceptance**

Join Creative and Survival independently and verify the correct worlds.

- [ ] **Step 6: Concurrency and isolation acceptance**

Connect PS5 and Switch 2 to different servers simultaneously. Stop one BDS and verify its connection fails cleanly while the other server remains discoverable and joinable. Restart the stopped BDS and verify it becomes joinable without restarting the helper.

- [ ] **Step 7: Record final evidence**

Save redacted console logs and final hashes in `outputs/STAGE2_VERIFICATION.txt`. Do not include SDP, identity tokens, JWTs, private keys, or raw login payloads.
