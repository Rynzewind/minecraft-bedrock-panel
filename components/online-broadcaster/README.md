# Isolated brokered NetherNet Friends broadcaster

This separate Go module builds `NetherNetOnline.exe` without changing the application's modified Gophertunnel module. Runtime publishes a Friends session and transfers admitted clients to a public authenticated gateway, never directly to BDS. The gateway independently verifies Xbox identity and server authorization; a Transfer packet grants no gateway capability.

- `--probe`: one bounded JSON capability object, exit 0.
- `--validate`: reads at most 16 KiB of JSON from stdin; strict known fields; returns a bounded JSON error and exit 1. It is an admission denial diagnostic, not an activation approval.
- `--run`: actual upstream broadcaster with parent-brokered access tokens and verified-XUID admission. Private inherited stdin/stdout pipes only; no network IPC endpoint or runtime override flag.

The legacy `--validate` envelope remains a conservative offline admission-denial diagnostic, not runtime approval. Caller assertions cannot upgrade probe capabilities. Runtime uses the separate bootstrap protocol below.

The compiled fork supports protocol 2225 / game 1.26.60. Probe adds brokered_tokens=true and fork_commit, but per_client_route, lan_first_verified and remote_ingress_verified remain false. Direct Transfer uses one static target. User explicitly accepts unknown-locality Friends joins through the authenticated public gateway. Existing LAN entries remain the LAN route; a local device choosing Friends may use WAN/hairpin routing. This component makes no locality or external-reachability assertion.

Config.AuthenticationDisabled=false, Relay=nil, FriendSync=nil. After genuine upstream login/OIDC and transport-key verification, Allow requires a fresh affirmative parent response for a valid XUID. Missing/invalid XUID, timeout, malformed response or parent loss denies. Player ClientData and ICE candidates never authenticate a player. Parent must keep helper-account authorization separate and independently protect gateway admission. No code calls upstream device-code, refresh-token or plaintext token/cache helpers.

Pinned upstream: https://github.com/HashimTheArab/go-mcxboxbroadcast/tree/bb06c2ddc46fb61b2bbbb886f08d6edf11383a3e

Pinned replacements: Gophertunnel lunar **f8448d729edd8abed1b04f37566b54da27b50249**, RakNet 2049463566ca. Protocol-2225 master 25acc41eb24af21cfebf27ffa5ace37069838eb3 was rejected because it removed room/p2p/signaling/authclient APIs required by broadcaster bb06. Reviewed lunar handleLogin retains login/OIDC validation, XboxLiveAuthenticated enforcement and transport-public-key verification before Allow. Exact dependency versions/checksums and licence inventory are included. Tests use mocks and pipes only; no external authentication, session publication, firewall change, live installation or console join has been performed.

## Private parent IPC version 1

One UTF-8 JSON object per line, max 64 KiB excluding newline. Unknown fields, including nested refresh_token, malformed/oversize frames and parent EOF cancel operation. Never log inherited stdin. Up to 64 concurrent requests, each bounded to 20 seconds, use monotonically increasing uint64 IDs. Stale/unmatched response IDs cannot authorize other requests. No credentials appear in command-line arguments or disk files.

First parent frame, before resource construction or credentials:

```json
{"type":"bootstrap","version":1,"server_id":"server-1","account_id":"account-1","gateway_host":"gateway.yourdomain.com","gateway_port":19140,"world_name":"My World","expected_protocol":2225,"allow_unknown_friends_route":true}
```

IDs are 1–128 ASCII letters/digits/underscore/hyphen. Host must be a public address or syntactically valid fully qualified DNS name; loopback/private/documentation IPs, localhost and example/default hosts reject. Ports are 1–65535; world name is 1–128 bytes without control characters. Syntax checking is not DNS/routing verification. Mismatched protocol or false opt-in rejects before auth. Binding cannot change during the process lifetime.

Child requests and parent replies:

```json
{"type":"token_request","id":1,"server_id":"server-1","account_id":"account-1"}
{"type":"token_response","id":1,"token":{"access_token":"REDACTED","token_type":"Bearer","expiry":"2026-10-29T12:00:00Z"}}
{"type":"token_response","id":1,"error_code":"authentication_required"}
{"type":"authorize_request","id":2,"server_id":"server-1","account_id":"account-1","xuid":"123456789"}
{"type":"authorize_response","id":2,"allowed":false}
{"type":"stop"}
```

Only nonempty Bearer access tokens with over 30 seconds remaining are accepted; the child never requests/accepts/saves/emits refresh tokens and has no auth fallback. A valid parent token is required before library New/Start; subsequent access-token requests are brokered as needed. Parent owns its protected credential flow. Response errors are never echoed.

Child stdout contains typed RPC requests and events `{type:"event",code:...}` with fixed codes starting, advertising, stopped, authentication_required, or error. Upstream logger is io.Discard. Advertising means Start succeeded, not a successful external join. Authorization replies are admission decisions and do not assert Transfer-sent or backend-connected. Stop/EOF cancels requests and closes upstream resources. No raw errors, tokens, names or messages are forwarded.
