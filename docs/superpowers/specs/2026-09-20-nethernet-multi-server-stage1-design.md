# NetherNet Multi-Server Discovery Helper — Stage 1 Design

## Purpose

Build a Windows console application that proves whether PlayStation 5 and Nintendo Switch 2 clients can display two simultaneous NetherNet LAN server entries advertised by one Windows PC.

The helper owns UDP port 7551 while both Bedrock Dedicated Server instances run with `enable-lan-visibility=false`. It advertises two independent server identities but does not forward connections in Stage 1.

## Success Criteria

Stage 1 is successful when:

1. `NetherNetMultiServer.exe` binds `0.0.0.0:7551` on Windows.
2. A valid NetherNet discovery request is decoded with `github.com/df-mc/go-nethernet/discovery`.
3. The helper sends two valid discovery responses to the requesting client.
4. Each response has a distinct, stable 64-bit NetherNet Network ID.
5. The advertisements identify `Private Creative Server` and `Private Survival Server` and describe Minecraft Bedrock 1.26.51 correctly.
6. Incoming requests, outgoing advertisements, and later discovery/signalling packets are logged.
7. The helper can be built as a standalone Windows executable and configured through JSON.
8. Real PS5 and Switch 2 clients show both entries at the same time. This final criterion requires a user-run LAN test and cannot be established by automated tests alone.

Selecting an entry is not expected to connect during Stage 1. Connection handling belongs to Stage 2.

## Protocol Basis

The implementation will depend on the existing `github.com/df-mc/go-nethernet/discovery` package for:

- authenticated and encrypted discovery packet decoding;
- authenticated and encrypted discovery packet encoding;
- `RequestPacket`, `ResponsePacket`, and `MessagePacket` structures; and
- Minecraft `ServerData` serialization.

The application will not reimplement AES, HMAC, discovery framing, or Minecraft server-data encoding.

The upstream discovery listener stores one Network ID and one response payload per UDP listener. Stage 1 instead uses one ordinary UDP socket and the package's exported `Unmarshal`, `Marshal`, and packet types. This allows one process to send multiple responses with different sender Network IDs through the single socket that owns port 7551.

## Architecture

### Configuration

`config.json` contains the listen address and a list of server advertisements. Network IDs are decimal strings so all 64 bits survive JSON parsing without floating-point conversion.

Each server entry contains:

- display name;
- level name;
- backend IP and port, retained for diagnostics and future Stage 2 routing;
- Network ID;
- Minecraft protocol number and version string;
- game type;
- current and maximum player counts;
- authentication flags;
- connection type; and
- optional nonce.

The supplied default configuration uses protocol `2193`, version `1.26.51`, connection type `4`, and two non-zero, distinct Network IDs. Configuration validation rejects duplicate or zero IDs, invalid addresses, invalid game types, invalid counts, and an empty server list before the UDP socket is opened.

### Discovery Service

The service binds one `net.PacketConn` to the configured address, normally `0.0.0.0:7551`, and runs a blocking receive loop until interrupted.

For each datagram:

1. Decode it with `discovery.Unmarshal`.
2. Log the source address, sender Network ID, and decoded packet type.
3. For a `RequestPacket`, send one `ResponsePacket` per configured server directly to the source address. Encode each response with that server's Network ID.
4. For a `MessagePacket`, log its recipient ID and signalling payload safely, with an explicit marker that Stage 1 does not answer it.
5. For a `ResponsePacket`, log that another LAN host was observed and continue.
6. For malformed or unauthenticated packets, log the decode error and continue serving.

Responses are written sequentially for deterministic logs. A failure sending one advertisement is logged but does not prevent attempts for the remaining advertisements.

### Console UX

Startup output shows:

- application title and Stage 1 status;
- listening address;
- all configured advertisements, Network IDs, and backend addresses;
- the warning that selection/connection is not implemented; and
- `Waiting for LAN clients...`.

Runtime logs show timestamps, request source addresses, client Network IDs, advertised server names, advertised Network IDs, byte counts, and observed signalling messages.

Ctrl+C causes an orderly socket close and clean process exit.

## Components

- `cmd/nethernet-multi-server/main.go`: process startup, console presentation, signal handling, and exit status.
- `internal/config`: JSON loading, defaults, semantic validation, and conversion to discovery server data.
- `internal/discovery`: UDP receive loop, packet dispatch, multi-response behavior, and structured logging.
- `config.example.json`: ready-to-copy configuration for the Creative and Survival instances.
- `README.md`: build, configuration, Windows Firewall, BDS settings, run, and physical-console test instructions.
- `scripts/build.ps1`: reproducible Windows build producing `NetherNetMultiServer.exe`.

The package name `internal/discovery` refers to the application service; the upstream codec is imported with an explicit alias to avoid confusion.

## Error Handling

- Startup failures such as missing configuration, invalid values, or port 7551 already being occupied are fatal and produce actionable console messages.
- Individual malformed packets and response-send failures are non-fatal.
- The listener uses bounded UDP buffers and never trusts packet-supplied lengths outside the upstream decoder.
- Signalling payloads are logged with a length limit so arbitrary LAN input cannot flood the console.
- Backend addresses are validated but are not contacted in Stage 1.

## Testing

Automated tests cover:

1. Valid and invalid JSON configuration.
2. Duplicate and zero Network IDs.
3. Server-data field conversion for Creative and Survival.
4. A discovery request encoded by `go-nethernet`, sent through a local UDP socket to the service, producing exactly two decodable responses.
5. Distinct response sender Network IDs and correct server names, levels, game types, protocol, and version.
6. Continued operation after a malformed datagram.
7. Logging/observation of a `MessagePacket` without a Stage 1 reply.
8. Graceful cancellation and socket shutdown.

The release check builds and tests on the installed Go toolchain, then cross-builds or natively builds the Windows executable. A local loopback integration test proves packet correctness but cannot prove console UI behavior; the README therefore includes a short PS5/Switch 2 acceptance checklist.

## Physical LAN Acceptance Test

1. Set `enable-lan-visibility=false` on both BDS instances and start both normally.
2. Ensure no other process owns UDP 7551.
3. Start `NetherNetMultiServer.exe` from a console, allowing its Windows Firewall prompt for private networks if shown.
4. Open the Friends/LAN server list on PS5 and Switch 2.
5. Confirm both named entries appear simultaneously.
6. Select each entry once and retain the helper logs for Stage 2 protocol analysis. Connection failure is expected at this stage.

## Stage Boundary

Stage 1 does not create WebRTC listeners, generate DTLS identities, negotiate ICE, relay Bedrock packets, or contact the configured BDS backends. Those features will be designed only after the dual-entry discovery result and captured signalling behavior are known.

## Dependencies and Versioning

The Go module pins a released `go-nethernet` version rather than following its moving `main` branch. The chosen release must support the current version-7 `ServerData` layout used by Bedrock 1.26.51. Dependency checksums are committed in `go.sum`.

Protocol references:

- <https://github.com/df-mc/go-nethernet>
- <https://github.com/df-mc/nethernet-spec>
- <https://github.com/GameParrot/netherconnect>
- <https://mojang.github.io/bedrock-protocol-docs/changelog/>
