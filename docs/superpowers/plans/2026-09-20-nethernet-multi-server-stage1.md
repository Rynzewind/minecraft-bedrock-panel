# NetherNet Multi-Server Discovery Helper Stage 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and verify a Windows console application that answers each valid NetherNet LAN discovery request with two distinct Minecraft Bedrock 1.26.51 server advertisements.

**Architecture:** One UDP socket owns port 7551. The application uses `github.com/df-mc/go-nethernet/discovery` to decode incoming packets and encode one response per configured server, changing the sender Network ID for each response. Configuration and protocol conversion are isolated from the UDP service so both can be tested independently.

**Tech Stack:** Go 1.26.8, `github.com/df-mc/go-nethernet` v1.0.18, standard-library JSON/networking/logging, PowerShell build script, Windows amd64 executable.

**Spec:** `docs/superpowers/specs/2026-09-20-nethernet-multi-server-stage1-design.md`

## Global Constraints

- Stage 1 only: no WebRTC listener, ICE, DTLS, SCTP, Bedrock relay, or backend connection.
- Listen on UDP `0.0.0.0:7551` by default.
- Use `go-nethernet/discovery` for packet authentication, encryption, framing, and `ServerData` serialization.
- Advertise Minecraft Bedrock version `1.26.51` and protocol `2193`.
- Network IDs are non-zero, distinct 64-bit unsigned integers represented as decimal strings in JSON.
- Creative backend is `192.168.1.246:19134`; Survival backend is `192.168.1.247:19136`.
- Both BDS instances retain `enable-lan-visibility=false` while the helper runs.
- Deliver `outputs/NetherNetMultiServer.exe`, `outputs/config.json`, and `outputs/README.txt`.

## Review Focus

- A syntactically valid configuration with duplicate Network IDs must fail before binding UDP 7551; Task 1 tests this.
- A Network ID above JavaScript's safe-integer range must retain all 64 bits; Task 1 tests decimal-string parsing with `18446744073709551615`.
- One malformed or unauthenticated LAN datagram must not terminate discovery; Task 3 tests recovery followed by a valid request.
- A response write failure for one advertisement must not suppress attempts for later advertisements; Task 3 tests all write attempts with a controlled packet connection.
- Cancellation while blocked in `ReadFrom` must close the socket and return promptly without an error exit; Task 3 tests bounded shutdown.

---

## File Structure

- `go.mod`, `go.sum`: module definition and pinned dependency checksums.
- `.gitignore`: local toolchain, build, coverage, and runtime configuration exclusions.
- `internal/config/config.go`: JSON types, loading, defaults, validation, Network ID parsing, and `ServerData` conversion.
- `internal/config/config_test.go`: table-driven configuration and conversion tests.
- `internal/lan/service.go`: UDP packet loop, request dispatch, multi-advertisement response, bounded signal logging, and cancellation.
- `internal/lan/service_test.go`: loopback integration tests plus controlled failure tests.
- `cmd/nethernet-multi-server/main.go`: CLI startup, human-readable banner, logging, OS signal cancellation, and exit codes.
- `cmd/nethernet-multi-server/main_test.go`: banner/output tests without binding the production port.
- `config.example.json`: the two requested BDS advertisements.
- `scripts/build.ps1`: test and Windows amd64 release build.
- `README.md`: developer and LAN acceptance instructions.
- `outputs/README.txt`: concise operator instructions copied into the deliverable directory.

### Task 1: Module and Validated Configuration

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `config.example.json`
- Generated: `go.sum`

**Interfaces:**
- Consumes: upstream `discovery.ServerData` from `github.com/df-mc/go-nethernet/discovery`.
- Produces: `config.Load(path string) (config.Config, error)`, `Config.Validate() error`, `Server.ParsedNetworkID() (uint64, error)`, and `Server.DiscoveryData() (*discovery.ServerData, error)`.

- [ ] **Step 1: Establish the module and dependency**

Create `go.mod`:

```go
module nethernet-multi-server

go 1.26.0

require github.com/df-mc/go-nethernet v1.0.18
```

Create `.gitignore`:

```gitignore
/work/
/bin/
/coverage.out
/config.json
```

If `go` is unavailable, download the official `go1.26.8.windows-amd64.zip`, verify SHA-256 `b92c3b2adae85a11ba71fe7216daf0d84e82af4c8ab6c5625807f28622043a59`, extract it beneath `work/toolchains/`, and use that `go.exe` for every command. Do not install machine-wide software.

Run: `go mod download github.com/df-mc/go-nethernet@v1.0.18`

Expected: dependency download succeeds and `go.sum` is created.

- [ ] **Step 2: Write failing configuration tests**

Create tests covering one valid server and these exact invalid cases:

```go
func TestServerParsedNetworkIDPreservesUint64(t *testing.T) {
	s := Server{NetworkID: "18446744073709551615"}
	got, err := s.ParsedNetworkID()
	if err != nil || got != math.MaxUint64 {
		t.Fatalf("ParsedNetworkID() = %d, %v; want %d, nil", got, err, uint64(math.MaxUint64))
	}
}

func TestValidateRejectsDuplicateNetworkIDs(t *testing.T) {
	cfg := validConfig()
	cfg.Servers = append(cfg.Servers, cfg.Servers[0])
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate network ID") {
		t.Fatalf("Validate() error = %v; want duplicate network ID", err)
	}
}
```

The table must also assert rejection of: empty server list, zero/non-decimal/overflow Network ID, empty name, invalid backend host:port, protocol below 1, empty version, unknown game type, negative player count, maximum below current count, and connection type below 1.

Run: `go test ./internal/config -run 'Test(ServerParsedNetworkIDPreservesUint64|Validate)' -v`

Expected: FAIL because the package and symbols do not exist.

- [ ] **Step 3: Implement configuration types and validation**

Define these JSON-facing types:

```go
type Config struct {
	Listen  string   `json:"listen"`
	Servers []Server `json:"servers"`
}

type Server struct {
	Name                  string `json:"name"`
	LevelName             string `json:"level_name"`
	Backend               string `json:"backend"`
	NetworkID             string `json:"network_id"`
	Protocol              int32  `json:"protocol"`
	Version               string `json:"version"`
	GameType              string `json:"game_type"`
	PlayerCount           int32  `json:"player_count"`
	MaxPlayerCount        int32  `json:"max_player_count"`
	AcceptsOnlineAuth     bool   `json:"accepts_online_auth"`
	AcceptsSelfSignedAuth bool   `json:"accepts_self_signed_auth"`
	Nonce                 string `json:"nonce,omitempty"`
	ConnectionType        int32  `json:"connection_type"`
}
```

`Load` must decode exactly one JSON value, reject unknown fields with `Decoder.DisallowUnknownFields`, apply `0.0.0.0:7551` only when `listen` is empty, then call `Validate`.

`ParsedNetworkID` must use `strconv.ParseUint(s.NetworkID, 10, 64)` and reject zero. `Validate` must use `net.SplitHostPort`, reject duplicate parsed IDs, and return errors that include `servers[N]`.

Run: `go test ./internal/config -v`

Expected: parsing/validation tests PASS; conversion tests are not present yet.

- [ ] **Step 4: Write failing `ServerData` conversion tests**

Add a table with Creative and Survival. Assert every field, including `GameTypeCreative`/`GameTypeSurvival`, protocol 2193, version 1.26.51, both auth flags, nonce, and connection type 4. Include one unknown game type and require an error containing `game_type`.

Run: `go test ./internal/config -run DiscoveryData -v`

Expected: FAIL because `DiscoveryData` does not exist.

- [ ] **Step 5: Implement `DiscoveryData` and example configuration**

Map case-insensitive `creative`, `survival`, and `adventure` to the upstream constants, then return:

```go
return &discovery.ServerData{
	ServerName:            s.Name,
	Protocol:              s.Protocol,
	Version:               s.Version,
	LevelName:             s.LevelName,
	GameType:              gameType,
	PlayerCount:           s.PlayerCount,
	MaxPlayerCount:        s.MaxPlayerCount,
	EditorWorld:           false,
	Hardcore:              false,
	AcceptsOnlineAuth:     s.AcceptsOnlineAuth,
	AcceptsSelfSignedAuth: s.AcceptsSelfSignedAuth,
	Nonce:                 s.Nonce,
	ConnectionType:        s.ConnectionType,
}, nil
```

Create `config.example.json` with listen address `0.0.0.0:7551`, the requested names/backends, IDs `13468123134000001934` and `13468123136000001936`, Creative/Survival modes, protocol 2193, version 1.26.51, zero players, ten maximum players, both authentication flags true, empty nonces, and connection type 4.

Run: `go test ./internal/config -v`

Expected: PASS.

- [ ] **Step 6: Commit the configuration unit**

```powershell
git add go.mod go.sum .gitignore internal/config config.example.json
git commit -m "feat: add validated server configuration"
```

### Task 2: Multi-Advertisement Discovery Service

**Files:**
- Create: `internal/lan/service.go`
- Create: `internal/lan/service_test.go`

**Interfaces:**
- Consumes: `net.PacketConn`, `discovery.Marshal`, `discovery.Unmarshal`, `discovery.ResponsePacket`, and validated server data.
- Produces: `lan.Advertisement`, `lan.New(conn net.PacketConn, advertisements []Advertisement, logger *slog.Logger) *Service`, and `(*Service).Serve(ctx context.Context) error`.

- [ ] **Step 1: Write the failing two-response loopback test**

Construct a UDP server on `127.0.0.1:0`, create two advertisements, start `Serve`, and send `discovery.Marshal(&discovery.RequestPacket{}, clientID)`. Read exactly two datagrams and decode both:

```go
type received struct {
	id   uint64
	data discovery.ServerData
}

for range 2 {
	n, _, err := client.ReadFromUDP(buf)
	if err != nil { t.Fatal(err) }
	packet, senderID, err := discovery.Unmarshal(buf[:n])
	if err != nil { t.Fatal(err) }
	response, ok := packet.(*discovery.ResponsePacket)
	if !ok { t.Fatalf("packet type = %T; want *discovery.ResponsePacket", packet) }
	var data discovery.ServerData
	if err := data.UnmarshalBinary(response.ApplicationData); err != nil { t.Fatal(err) }
	got = append(got, received{id: senderID, data: data})
}
```

Assert that the IDs and names match the two advertisements regardless of receive order, no third response arrives before a 150 ms deadline, and the test cancels the service cleanly.

Run: `go test ./internal/lan -run TestServiceRespondsWithEveryAdvertisement -v`

Expected: FAIL because `lan.New` and related types do not exist.

- [ ] **Step 2: Implement request decoding and response encoding**

Define:

```go
type Advertisement struct {
	Name       string
	Backend    string
	NetworkID  uint64
	ServerData discovery.ServerData
}

type Service struct {
	conn           net.PacketConn
	advertisements []Advertisement
	log            *slog.Logger
}
```

`New` substitutes `slog.New(slog.NewTextHandler(io.Discard, nil))` for a nil logger and copies the advertisement slice.

`Serve` starts one cancellation watcher that closes the socket when `ctx.Done()` fires, reads into a 65,535-byte buffer, and calls `handleDatagram`. If `ReadFrom` fails after cancellation, return nil; otherwise return an error wrapping `read discovery datagram`.

`handleDatagram` decodes with `discovery.Unmarshal`. On `*discovery.RequestPacket`, marshal each `ServerData`, put it in `discovery.ResponsePacket`, encode with that advertisement's `NetworkID`, and `WriteTo` the request source. Log request and response fields using `slog`.

Run: `go test ./internal/lan -run TestServiceRespondsWithEveryAdvertisement -v`

Expected: PASS.

- [ ] **Step 3: Add packet-field assertions**

Extend the loopback test to assert both decoded advertisements preserve protocol, version, level, game type, player counts, authentication flags, and connection type. Assert the response sender IDs do not equal the client sender ID.

Run: `go test ./internal/lan -v`

Expected: PASS.

- [ ] **Step 4: Commit the core discovery service**

```powershell
git add internal/lan
git commit -m "feat: advertise multiple NetherNet LAN servers"
```

### Task 3: Resilience, Signalling Observation, and Shutdown

**Files:**
- Modify: `internal/lan/service.go`
- Modify: `internal/lan/service_test.go`

**Interfaces:**
- Consumes: Task 2's service API.
- Produces: non-fatal datagram handling, bounded message logging, continued write attempts, and prompt context-driven shutdown.

- [ ] **Step 1: Write the failing malformed-datagram recovery test**

Start the service, send `[]byte("not a NetherNet packet")`, then send a valid request. Require two valid responses and a captured log entry containing `rejected discovery datagram`.

Run: `go test ./internal/lan -run TestServiceContinuesAfterMalformedDatagram -v`

Expected: FAIL because the current handler returns the decode error to the receive loop.

- [ ] **Step 2: Make per-datagram errors non-fatal**

Change `Serve` so `handleDatagram` errors are logged with source address and processing continues. Reserve returned errors for socket-level read failures only.

Run: `go test ./internal/lan -run TestServiceContinuesAfterMalformedDatagram -v`

Expected: PASS.

- [ ] **Step 3: Write failing signalling-observation tests**

Send a `discovery.MessagePacket{RecipientID: firstServerID, Data: "CONNECTREQUEST 42 sample-offer"}` encoded with a client ID. Assert no reply before a 150 ms deadline and logs containing packet type, client ID, recipient ID, and the payload.

Send a second message with 2,000 `x` characters and assert the logged payload is at most 512 characters plus a visible truncation suffix.

Run: `go test ./internal/lan -run TestServiceObservesMessageWithoutReply -v`

Expected: FAIL because message packets are not logged explicitly.

- [ ] **Step 4: Implement bounded packet observation**

Add packet cases for `*discovery.MessagePacket` and `*discovery.ResponsePacket`. For messages, log `sender_network_id`, `recipient_network_id`, and `signal`; apply:

```go
func bounded(value string, limit int) string {
	if len(value) <= limit { return value }
	return value[:limit] + "...[truncated]"
}
```

Do not send a response for either packet type.

Run: `go test ./internal/lan -run TestServiceObservesMessageWithoutReply -v`

Expected: PASS.

- [ ] **Step 5: Write controlled write-failure and cancellation tests**

Create a small `net.PacketConn` test double whose `ReadFrom` returns one valid request, whose first `WriteTo` returns `errors.New("injected write failure")`, and whose later writes are recorded. Require two total write attempts and a log entry naming the failed advertisement.

Create another blocking test connection, cancel the context, and require `Serve` to return nil within one second and `Close` to have been called exactly once.

Run: `go test ./internal/lan -run 'TestService(AttemptsAllWrites|StopsOnCancellation)' -v`

Expected: FAIL until write failures are handled per advertisement and shutdown ownership is correct.

- [ ] **Step 6: Implement failure isolation and idempotent shutdown**

Log each response-write failure and continue the advertisement loop. Use a `sync.Once`-guarded local close function shared by the cancellation watcher and deferred cleanup so the packet connection is closed once. Ensure the watcher terminates when `Serve` returns by selecting on an internal `done` channel as well as `ctx.Done()`.

Run: `go test -race ./internal/lan -v`

Expected: PASS with no races.

- [ ] **Step 7: Commit resilience behavior**

```powershell
git add internal/lan
git commit -m "feat: log signalling and survive bad LAN traffic"
```

### Task 4: Windows Console Entry Point

**Files:**
- Create: `cmd/nethernet-multi-server/main.go`
- Create: `cmd/nethernet-multi-server/main_test.go`

**Interfaces:**
- Consumes: `config.Load`, `Server.ParsedNetworkID`, `Server.DiscoveryData`, and `lan.New`.
- Produces: `run(ctx context.Context, args []string, stdout, stderr io.Writer) int` and the application executable entry point.

- [ ] **Step 1: Write failing banner and configuration-error tests**

Call `run` with a temporary valid JSON file and replace socket creation through a package variable:

```go
var listenPacket = net.ListenPacket
```

The test replacement returns a controlled packet connection that blocks until context cancellation. Assert output contains the title, `Stage 1 discovery proof of concept`, listen address, both server names, both backends, both Network IDs, the connection-not-implemented warning, and `Waiting for LAN clients...`.

For a missing file, assert a non-zero exit code and stderr containing `load configuration`.

Run: `go test ./cmd/nethernet-multi-server -v`

Expected: FAIL because `run` does not exist.

- [ ] **Step 2: Implement `run` and `main`**

`run` accepts either no path (use `config.json`) or one path; more arguments return exit code 2 with usage text. Load configuration before binding the socket. Convert each configured server to `lan.Advertisement`, bind with `listenPacket("udp", cfg.Listen)`, print the banner, and invoke the service.

Use `slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelInfo})`. Return 1 for configuration, conversion, bind, or unexpected service errors; return 0 after context cancellation.

`main` creates `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`, defers cancellation, and calls `os.Exit(run(...))`.

Run: `go test ./cmd/nethernet-multi-server -v`

Expected: PASS.

- [ ] **Step 3: Add argument and port-conflict coverage**

Test extra arguments return 2 without attempting a socket bind. Test a socket factory error containing `address already in use` returns 1 and prints an actionable message mentioning UDP 7551, both BDS `enable-lan-visibility=false` settings, and checking for another owner.

Run: `go test ./cmd/nethernet-multi-server -v`

Expected: PASS.

- [ ] **Step 4: Commit the console application**

```powershell
git add cmd/nethernet-multi-server
git commit -m "feat: add Windows console entry point"
```

### Task 5: Build, Documentation, and Release Verification

**Files:**
- Create: `scripts/build.ps1`
- Create: `README.md`
- Create: `outputs/README.txt`
- Generate: `outputs/NetherNetMultiServer.exe`
- Generate: `outputs/config.json`

**Interfaces:**
- Consumes: all prior application packages and `config.example.json`.
- Produces: operator-ready Windows artifacts and a reproducible build/test command.

- [ ] **Step 1: Write the PowerShell build script**

The script must stop on errors, create `outputs`, run all tests, build an amd64 Windows binary without CGO, and copy the example config:

```powershell
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$outputDir = Join-Path $projectRoot 'outputs'
New-Item -ItemType Directory -Force -Path $outputDir | Out-Null

Push-Location $projectRoot
try {
    go test ./...
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    go build -trimpath -ldflags '-s -w' -o (Join-Path $outputDir 'NetherNetMultiServer.exe') ./cmd/nethernet-multi-server
    Copy-Item -Force 'config.example.json' (Join-Path $outputDir 'config.json')
} finally {
    Pop-Location
}
```

Run: `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1`

Expected: tests pass and the EXE/config appear in `outputs`.

- [ ] **Step 2: Write developer and operator documentation**

`README.md` must include:

- exact Stage 1 purpose and limitation;
- architecture and dependency attribution;
- Go 1.26+ build instructions;
- JSON field reference and Network ID uniqueness rule;
- the supplied Creative/Survival addresses;
- BDS `enable-lan-visibility=false` prerequisite;
- Windows Firewall private-network allowance for inbound UDP 7551;
- how to identify a port conflict with `Get-NetUDPEndpoint -LocalPort 7551`;
- start command `./NetherNetMultiServer.exe ./config.json`;
- PS5 and Switch 2 two-entry acceptance steps; and
- instructions to select each entry once and retain logs for Stage 2.

`outputs/README.txt` must be a concise plain-text operator version of the same run/test instructions and clearly state that selection is expected to fail during Stage 1.

- [ ] **Step 3: Format and run the complete verification suite**

Run:

```powershell
gofmt -w cmd internal
go mod tidy
go vet ./...
go test -race ./...
go test -coverprofile coverage.out ./...
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1
```

Expected: every command exits 0, all packages pass, and `coverage.out` is generated.

- [ ] **Step 4: Inspect the release artifacts**

Run:

```powershell
Get-Item outputs/NetherNetMultiServer.exe | Select-Object Name, Length
Get-FileHash outputs/NetherNetMultiServer.exe -Algorithm SHA256
Get-Content -Raw outputs/config.json | ConvertFrom-Json | Out-Null
```

Expected: non-zero EXE size, a SHA-256 digest, and valid JSON.

Start the EXE with the generated config, wait for the listening banner, then stop it. Confirm UDP 7551 was bound while running and released after exit. Do not start it if either BDS instance still has LAN visibility enabled.

- [ ] **Step 5: Commit the verified deliverables**

```powershell
git add go.mod go.sum cmd internal scripts README.md config.example.json outputs
git commit -m "build: deliver Stage 1 Windows discovery helper"
```

## Completion Boundary

Automated completion means tests, vet, race checks, a Windows build, configuration parsing, loopback dual-response behavior, malformed-packet recovery, signalling observation, and release artifact inspection all pass.

The project is not declared physically proven until the user runs the documented PS5 and Nintendo Switch 2 acceptance test and confirms that both entries are simultaneously visible. Those results determine whether Stage 2 connection-routing design begins.
