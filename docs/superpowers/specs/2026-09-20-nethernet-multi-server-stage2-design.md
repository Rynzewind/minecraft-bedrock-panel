# NetherNet Multi-Server Stage 2 Design

## Objective

Extend the proven Stage 1 Windows helper so PlayStation 5 and Nintendo Switch 2 clients can select either advertised LAN entry and join the matching Minecraft Bedrock Dedicated Server (BDS) instance.

The mappings are:

- Private Creative Server (`13468123134000001934`) to `192.168.1.246:19134`
- Private Survival Server (`13468123136000001936`) to `192.168.1.247:19136`

Both BDS instances continue using `transport=nethernet` and `enable-lan-visibility=false`. The helper remains the only owner of UDP 7551.

## Confirmed Evidence

Stage 1 displays both entries simultaneously on PS5 and Nintendo Switch 2. Windows Firewall, rather than the socket implementation, caused the earlier receive failure.

Selecting Creative on PS5 produced a valid 688-byte discovery message containing:

- Client network ID `3829603440984922419`
- Recipient network ID `13468123134000001934`
- `CONNECTREQUEST` with an SDP offer
- A host ICE candidate for `192.168.1.125`
- ICE credentials, a DTLS fingerprint, and an SCTP data-channel description

This proves the advertised network ID provides an unambiguous routing key before WebRTC negotiation starts.

## Scope

Stage 2 will:

1. Preserve the working shared UDP 7551 discovery service.
2. Route signalling messages by recipient network ID.
3. Terminate each console's NetherNet/WebRTC connection using `go-nethernet`.
4. Open a corresponding NetherNet connection to the configured BDS backend.
5. Relay the Minecraft Bedrock protocol between the console and BDS.
6. Support multiple configured servers and concurrent clients.
7. Persist a distinct server identity for every configured network ID.
8. Produce useful connection diagnostics without exposing credentials or tokens.

Stage 2 will not implement ICE, DTLS, SCTP, WebRTC, cryptography, discovery framing, or Minecraft packet encoding from scratch.

## Chosen Architecture

The helper will terminate and proxy NetherNet connections. This is preferred over transparent signalling rewriting because `go-nethernet` already implements the offer/answer exchange, ICE, DTLS, SCTP, data channels, and NetherNet identity assertions. A protocol-aware proxy also gives each advertised identity an explicit backend mapping and isolates failures between servers.

### Shared LAN Transport

One IPv4 UDP socket remains bound to UDP 7551 using the socket configuration proven in Stage 1. It continues to receive discovery requests and send one advertisement for every configured server.

The LAN service will additionally own a signalling router. Each configured network ID receives a signalling adapter implementing the `go-nethernet.Signaling` contract. All adapters share the same UDP socket; none opens another UDP 7551 listener.

Incoming `MessagePacket` data is decoded with the existing discovery package and routed using `RecipientID`. Outgoing answers, candidates, and errors are encoded as `MessagePacket` values with the configured server network ID as sender and the console network ID as recipient, then sent to the console address associated with that negotiation.

The router tracks a client network ID's most recently observed source address. Negotiations also remain keyed by client network ID and connection ID inside `go-nethernet`, so two consoles or repeated joins do not overwrite each other.

### Per-Server NetherNet Listener

Each configured server receives one `go-nethernet.Listener` backed by its signalling adapter. The listener's network ID exactly matches the ID advertised for that server.

The listener uses the upstream implementation for:

- SDP parsing and answer generation
- ICE candidate gathering and exchange
- DTLS negotiation
- SCTP setup
- Reliable and unreliable data channels
- Client identity assertion validation
- Server identity assertion generation

No custom WebRTC state machine will be added.

### Persistent Server Identities

Every configured network ID has a separate P-384 private key. Keys are loaded from an `identities` directory beside the active configuration file. If a key is absent, the helper creates it once using cryptographically secure randomness and writes it with user-only permissions where Windows permits.

The filename is derived only from the decimal network ID, not from the display name. Renaming a server therefore retains its identity. Changing a network ID intentionally creates a new identity and may cause a one-time Trust On First Use prompt on consoles.

Private keys and identity tokens are never written to console logs.

### Backend Connection

After a per-server listener accepts a console connection, the helper connects to that server's configured BDS NetherNet endpoint. Backend selection is fixed when the listener is created; client-controlled data cannot choose an arbitrary address.

The backend dial uses the maintained NetherNet and gophertunnel integration. The proxy carries the console's Bedrock client and authentication data into the upstream login flow, and validates the Bedrock login data as required by the library. The implementation will first prove that each configured BDS endpoint is dialable before enabling the packet relay.

If the current BDS version exposes a signalling endpoint format that differs from the existing endpoint client, this is treated as a backend compatibility finding. The working discovery build remains untouched while that boundary is investigated; the implementation must not replace the upstream WebRTC stack with a custom one.

### Bedrock Relay

Once both sides are established, the helper performs the standard gophertunnel NetherNet-to-NetherNet proxy sequence:

1. Accept and decode the console's Bedrock login.
2. Dial the selected BDS using the console's client data and authenticated identity.
3. Exchange start-game and spawn state.
4. Relay decoded Bedrock packets in both directions.
5. Propagate a meaningful disconnect message when either side closes.
6. Close both sides and release all negotiation state.

Each connection runs independently. A panic or error in one session is recovered and logged at the session boundary and must not stop discovery, listeners, or other active sessions.

## Configuration

Existing Stage 1 fields remain valid. Stage 2 may add optional fields only when required by the upstream backend dialer. Unknown fields remain rejected.

The existing `backend` value remains the authoritative routing target. Network IDs must continue to be unique, non-zero decimal uint64 strings. Backend hosts remain literal IP addresses with explicit ports.

Identity storage is automatic and does not require users to paste keys into JSON.

## Logging and Privacy

Startup output lists the UDP listener, each advertised server, its backend, its network ID, and its identity fingerprint.

Per-connection logs include:

- Source IP and client network ID
- Selected server name and network ID
- Signalling phase and connection ID
- ICE/WebRTC establishment result
- Backend dial result
- Session start, duration, and disconnect reason

Logs must not contain SDP identity tokens, ICE passwords, complete authentication JWTs, private keys, or raw Minecraft login payloads. The Stage 1 raw UDP diagnostic log remains available behind a diagnostic setting or is redacted for signalling packets before final release.

## Error Handling

- Unknown recipient network IDs are logged and ignored.
- Malformed discovery or signalling packets are rejected without stopping the service.
- Duplicate offers are left to `go-nethernet` negotiation ownership rules.
- Negotiation and backend dial operations have bounded timeouts.
- A missing or invalid identity key prevents only the affected server listener from starting and is reported clearly at startup.
- An unavailable backend produces a client-visible disconnect and does not affect other servers.
- Shutdown cancels negotiations, closes active sessions, closes listeners, and finally closes UDP 7551.

## Delivery Stages

### Stage 2A: Inbound Connection

Route the captured PS5 `CONNECTREQUEST` into the Creative listener, return a valid `CONNECTRESPONSE`, complete ICE/DTLS/SCTP, and log an accepted NetherNet connection. No backend relay is claimed at this point.

### Stage 2B: Backend Probe

Verify the helper can dial Creative and Survival independently using their configured NetherNet endpoints. Record precise failures without changing discovery behavior.

### Stage 2C: Protocol Relay

Connect accepted console sessions to their selected backend and relay Bedrock packets bidirectionally.

### Stage 2D: Console Acceptance

Verify:

1. PS5 joins Creative and reaches the Creative world.
2. PS5 joins Survival and reaches the Survival world.
3. Nintendo Switch 2 joins Creative.
4. Nintendo Switch 2 joins Survival.
5. Two clients can use different servers simultaneously.
6. Restarting the helper retains identity fingerprints and does not repeat trust prompts.
7. Stopping one BDS affects only its corresponding entry and connection attempts.

## Testing Strategy

Automated tests cover:

- Recipient network ID routing
- Multiple clients sharing a network ID without address confusion
- Outgoing message sender/recipient IDs
- Unknown and malformed signalling messages
- Identity creation, reload, uniqueness, and permissions behavior
- Configuration compatibility and validation
- Backend selection that cannot be overridden by client input
- Session cleanup and isolated failure handling
- Logging redaction for SDP credentials and authentication tokens

Integration tests use in-memory signalling adapters and upstream `go-nethernet` listeners where possible. Windows socket tests remain separate from protocol tests. Real-console checks are required because PS5 and Nintendo Switch 2 behavior cannot be fully simulated.

## Build and Rollback

The proven Stage 1 executable is preserved before Stage 2 artifacts are published. Development builds use a distinct filename until inbound negotiation and backend routing pass their acceptance checks.

The final deliverables are:

- `NetherNetMultiServer.exe`
- `config.json`
- Automatically created `identities` directory on first run
- Updated operator instructions covering Windows Firewall, identity backup, startup, shutdown, and troubleshooting

No Git commit is required for delivery, per the existing project instruction.

## Success Criteria

Stage 2 is complete only when both consoles can select either visible entry and reach the correct BDS world, including simultaneous use, while one helper remains the sole owner of UDP 7551 and no custom implementation replaces the maintained NetherNet/WebRTC protocol libraries.
