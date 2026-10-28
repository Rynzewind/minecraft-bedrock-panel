# NetherNet Account Pool and Windows Admin Design

## Objective

Add a native Windows administration application that manages multiple Microsoft/Xbox identities for the existing NetherNet multi-server proxy. Multiple consoles must be able to join the same BDS instance simultaneously without being rejected as duplicate players.

The proven discovery, UDP 7551 socket, advertisements, server Network IDs, NetherNet listeners, backend mappings, and Minecraft packet relay remain unchanged. The server executable gains only the account-leasing and local administration integration required to select a distinct authenticated outbound identity for each active player.

## User Experience

The deployment contains two executables:

- `NetherNetMultiServer.exe` continues to run the discovery and connection service.
- `NetherNetMultiServerAdmin.exe` is an optional native Windows control panel. Closing it never stops the service or disconnects players.

The control panel is a single administrator window. It does not start, stop,
restart, or otherwise own the server process. The existing
`NetherNetMultiServer.exe` remains an independent application so it can later
be managed by a different launcher or service manager.

The main area is split into two simultaneously visible panels:

- **Account Pool** on the left
- **Console Users** on the right

A narrow status area shows whether the server's local administration channel
is connected, the number of active sessions, Creative and Survival activity,
and a short redacted activity log. It contains no server Start or Stop controls.

Each Account Pool row shows:

- Operator-entered person label
- Optional operator-entered email label
- Authenticated Xbox gamertag and XUID
- Authentication health
- Available or in-use state
- Current player and backend when leased

The email is only a local label. The application never requests or stores a Microsoft password and does not assume that an OAuth token exposes an email address.

Each Console Users row shows:

- Incoming console Xbox gamertag and XUID
- Assigned helper-account label, gamertag, and XUID
- Current status: offline, connecting, Creative, or Survival
- Last-seen time

The operator can add, reauthenticate, relabel, and remove accounts from the
left panel. Selecting a console user and an available account enables a clear
**Assign/Reassign** action. Unpair is also available from the Console Users
panel. Reassignment is disabled while the player or helper account is active.

There are no separate Accounts and Players tabs, command windows, test
launchers, or server controls in the administrator workflow. Ordinary use is:
open the panel, see both lists, and make assignments when required.

## Account Enrollment

Selecting **Add account** asks for a person label and optional email label, then starts Microsoft device-code authentication. The control panel displays the official URL and code. The user completes authentication directly with Microsoft.

After authentication, the service validates that the token can obtain the Minecraft/Xbox credentials required by the existing gophertunnel backend dialer. Only a successfully validated account enters the pool. The UI displays the signed Xbox gamertag and XUID returned by the authentication services.

Passwords, device codes, access tokens, and refresh tokens are never displayed after enrollment or written to ordinary logs.

## Automatic Assignment and Persistent Pairing

The existing inbound Minecraft listener authenticates the console login and exposes the console player's signed Xbox XUID and gamertag before the backend is dialed.

For every accepted connection:

1. Read the authenticated incoming XUID.
2. Look for an existing persistent assignment.
3. If assigned, lease that helper account when it is available.
4. If unassigned, atomically lease the first healthy, unassigned, available helper account and persist the mapping before dialing BDS.
5. Use that account's token source for both the outbound NetherNet identity assertion and BDS Minecraft login.
6. Release the lease when the session ends, while retaining the persistent mapping.

Allocation order is stable, based on the account order shown in the control panel. A helper account cannot be leased to two sessions simultaneously. A console XUID cannot receive two simultaneous leases.

If no account is available, the console receives a clear disconnect message such as `No proxy player account is available`. Discovery and other active sessions continue normally.

Persistent pairing is essential because BDS associates inventory, position, permissions, Ender Chest contents, and other player state with the helper account's authenticated UUID/XUID. Randomly cycling accounts between sessions is prohibited.

## Reassignment

The operator may select an offline player and choose a different unassigned helper account. The UI shows a warning that BDS will treat the player as the newly selected Xbox identity and that inventory, location, permissions, and other stored state will change accordingly.

The previous mapping remains unchanged unless the operator confirms. Active players and currently leased accounts cannot be reassigned. No automatic player-data migration is attempted.

## Storage and Security

Account metadata, ordering, player mappings, and encrypted tokens live in an account-store directory beside `config.json`.

Refresh tokens are encrypted using Windows Data Protection API (DPAPI) for the current Windows user. Copying the encrypted store to another Windows account or machine does not make the tokens usable. Metadata contains no plaintext access or refresh tokens.

The current single `auth-token.json` is migrated only after its token is successfully encrypted into the new store. The original remains in place until migration verification succeeds, then is renamed to a clearly marked backup rather than deleted automatically.

The operator must run both executables as the same Windows user. Account metadata and mappings may be backed up, but DPAPI-protected tokens require reauthentication after moving to a different Windows account or PC.

## Local Control Channel

The service owns a Windows named pipe dedicated to administration. Its access control list permits only the current Windows user and LocalSystem. It does not open a TCP port or expose administration to the LAN.

The protocol uses length-bounded JSON request and event messages. Supported operations include:

- List accounts and players
- Begin and complete device-code enrollment
- Reauthenticate an account
- Update labels
- Reorder accounts
- Reassign or unpair an offline player
- Subscribe to account, pairing, lease, and session-status changes

The service validates every request independently. The admin application never reads or decrypts token files directly; all token operations occur inside the service.

If the service is not running, the control panel opens in a disconnected state and explains how to start it. Reconnecting the control panel does not restart or alter active game sessions.

## Native Windows Control Panel

`NetherNetMultiServerAdmin.exe` uses native Windows controls and ships as a standalone executable. Its primary window contains:

- A read-only service connection/status area and active-session count
- A left-hand Account Pool table with Add, Reauthenticate, Edit label, and Remove actions
- A right-hand Console Users table with Assign/Reassign and Unpair actions
- A small event/status area containing redacted operational messages

The two tables remain visible together. The server continues normally if the
panel is opened, closed, disconnected, or restarted.

Destructive or identity-changing actions require confirmation. Token values, raw discovery packets, SDP, ICE credentials, and Minecraft login payloads never appear in the UI.

## Service Integration Boundaries

The existing backend `Dialer` interface changes from one process-wide token source to a lease selected per inbound authenticated player. Account selection occurs after the inbound `minecraft.Conn` is accepted and before the existing backend dial call.

No account-pool code may change:

- Discovery request parsing or advertisement encoding
- UDP 7551 binding behavior
- Advertised Network IDs
- Per-server identity keys
- Signalling routing
- ICE, DTLS, SCTP, or WebRTC implementations
- Creative and Survival backend selection
- Bidirectional Minecraft packet relay semantics

The account manager is isolated behind interfaces for lookup, lease, release, enrollment, and reassignment. If account-store initialization fails, the service reports the failure before binding UDP 7551 rather than silently falling back to a shared identity.

## Concurrency and Failure Handling

Lease and assignment changes are transactional under one account-manager lock and are persisted atomically. A process crash cannot leave a durable account marked in use; leases are in-memory and reconstructed as free on restart, while persistent player assignments remain.

An expired token is refreshed before leasing. If refresh fails, that account becomes unhealthy and allocation tries the next eligible account. Existing healthy accounts and active sessions remain unaffected.

Removing an account is allowed only when it is offline and unassigned. Reauthentication replaces encrypted credentials only after the new credentials validate successfully.

## Compatibility and Migration

The existing `config.json`, identity keys, advertisements, and server mappings remain compatible. No account email fields are added to the server configuration.

On first account-pool startup:

1. Detect the existing `auth-token.json`.
2. Validate and identify its Xbox account.
3. Import it as the first pool account with an operator-editable label.
4. Encrypt the refresh token with DPAPI and verify it can be decrypted.
5. Rename the plaintext source to `auth-token.migrated-backup.json`.

The service does not require the control panel to remain open after enrollment and configuration.

## Testing

Automated tests cover:

- DPAPI round trips and wrong-user/unavailable-decryption errors where testable
- Token-store atomic writes and absence of plaintext tokens
- Enrollment validation and failed-auth rollback
- Stable automatic allocation order
- Persistent console-XUID-to-helper-account mapping
- Lease exclusion under concurrent joins
- Lease release after every session exit path
- Pool exhaustion and unhealthy-account fallback
- Reassignment rules and confirmation data
- Named-pipe authorization, message bounds, and malformed requests
- Admin disconnected/reconnected state
- Migration from the current single token
- Regression coverage proving discovery, Network IDs, backend routing, and relay remain unchanged

Real-device acceptance requires:

1. Enroll at least two helper Microsoft accounts.
2. Join the same Creative BDS concurrently from PS5 and Switch 2.
3. Confirm distinct in-world helper gamertags and stable inventories.
4. Repeat on Survival.
5. Restart the service and verify mappings persist.
6. Reassign one offline player and verify the UI warning and resulting BDS identity change.

## Delivery

The final deployment at `C:\MinecraftServers\MultiServer` contains:

- Updated `NetherNetMultiServer.exe`
- `NetherNetMultiServerAdmin.exe`
- Existing `config.json`
- Existing `identities` directory
- DPAPI-protected account store and mapping metadata
- Updated operator instructions

The proven single-account executable is preserved as a rollback artifact until concurrent same-server acceptance passes.

## Success Criteria

- PS5 and Switch 2 can join the same BDS simultaneously using distinct leased helper identities.
- First-time players are automatically assigned an available account and retain it across reconnects and restarts.
- The operator can clearly see who is mapped to which account and can safely reassign offline players.
- Closing the control panel has no effect on discovery or connected players.
- No plaintext Microsoft credentials or sensitive signalling payloads are stored or logged.
- Existing two-server discovery, naming, routing, and relay behavior remains unchanged.
