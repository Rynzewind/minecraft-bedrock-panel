# Offline NetherNet compatibility probe

This separate Go module builds `NetherNetOnline.exe`. It imports the actual pinned broadcaster and protocol types without altering the application's modified Gophertunnel module. It never calls broadcaster.New/Start or creates token sources, sockets, accounts, sessions, or plaintext token caches.

- `--probe`: one bounded JSON capability object, exit 0.
- `--validate`: reads at most 16 KiB of JSON from stdin; strict known fields; returns a bounded JSON error and exit 1. It is an admission denial diagnostic, not an activation approval.
- `--run`: always returns `activation_blocked_protocol_locality_and_ingress_unverified`, exit 1. No override exists.

Envelope fields: `protocol`, `game_version`, `upstream_commit`, `online_mode`, `relay`, `remote_ingress_verified`, `lan_first_verified`, `xuid`, `candidates` (objects with `type`, `address`). These contain no credentials. Caller assertions about verification cannot upgrade compiled capabilities. A provided XUID is never treated as authenticated; the configured upstream Allow callback denies every join. Unknown candidates, empty XUID and mismatched versions deny earlier. Inputs and parse errors are never echoed.

The imported fork currently supports protocol 2193 / game 1.26.50, while the existing application target is 2225. Direct Transfer uses a static target; upstream has no per-player direct-transfer resolver. NetherNet RemoteAddr exposes logical identity and ICE evidence, not a reliable physical-LAN assertion. Strict LAN-first cannot be guaranteed for unknown/local users choosing Friends, so unknown classifications must be denied. Protected destination ingress and independently authenticated XUID authorization are also unverified. Consequently Online activation is intentionally incomplete.

Future integration requires a reviewed per-client direct-transfer hook, verified protocol compatibility, protected destination admission that rejects direct public-IP bypass, and a defensible locality proof or explicit user-approved policy change. Keep online-mode=true and Relay=nil. Do not share helper account authorization with remote players. A future wrapper can inject token sources and use protected OS credential storage with ACL-restricted IPC; do not use upstream CLI plaintext JSON cache helpers.

Pinned upstream: https://github.com/HashimTheArab/go-mcxboxbroadcast/tree/bb06c2ddc46fb61b2bbbb886f08d6edf11383a3e

Pinned replacements: Gophertunnel f12973c6db70 and RakNet 2049463566ca, exactly as upstream. See go.mod/go.sum and THIRD_PARTY_NOTICES.md. No live behavior has been tested or claimed.
