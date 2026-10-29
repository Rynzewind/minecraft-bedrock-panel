# Online runtime candidate — operator handover

This candidate implements real optional Friends broadcasting and an authenticated remote gateway. It has not been installed or activated on the working server PC. No router, firewall, world, installed configuration, Desktop installer or Drive installer has been changed. Actual Xbox publication and external console joining remain unverified.

## Approved routing

On 29 October 2026 the user accepted unknown-location Friends connections through the authenticated gateway. Existing LAN discovery entries remain local. A local device choosing Friends may use the public/hairpin route. This is explicit routing consent, not an access exception. The saved consent defaults false on a new configuration.

## Implemented

- Online Overview, Authorised Players, Connectivity and Logs; Servers remains the sole BDS lifecycle controller.
- Dedicated broadcaster roles and Accounts-only device sign-in, existing DPAPI refresh-token ownership, role-safe older-version migration. Existing helpers retain their mappings and inventories.
- Authenticated Xbox profile resolution, single-use identity review and explicit Creative/Survival XUID permissions. Friendship alone grants nothing.
- Isolated protocol-2225 broadcaster, private inherited-pipe access-token/admission broker, authentication enabled, relay disabled, no device-flow fallback or second token cache.
- Authenticated RakNet gateway that checks verified console XUID before private resource packs, helper leasing and backend dialing. Existing session code preserves the player's assigned helper identity and world data.
- Live ACL rechecks around helper/backend operations and an independent active-session watchdog. Disablement/revocation, readiness loss, child exit and shutdown close owned remote sessions.
- Parent supervision with bounded retries, explicit Accounts reauthentication recovery, Windows suspended creation and kill-on-close job containment. Replacement is blocked until an old worker's exit is confirmed.
- Effective Windows Firewall evidence evaluation for per-BDS host-source restriction across IPv4 and IPv6. Admission rechecks current host addresses and evidence freshness independently of account-lock stalls. No firewall apply operation is included.
- Installer companion integrity support and snapshots retaining role-safe metadata/settings while disabling restored Online intent. Portable snapshots exclude credentials.

## Dependencies

The isolated component pins MIT go-mcxboxbroadcast bb06c2ddc46fb61b2bbbb886f08d6edf11383a3e and compatible Gophertunnel lunar f8448d729edd8abed1b04f37566b54da27b50249 (protocol 2225 / 1.26.60). The newer master removed required APIs; it is not used. Component licences are included. The root module and its modified Gophertunnel stay unchanged. The GPL Java implementation was investigated but not incorporated.

## Activation requirements and outstanding verification

Online defaults disabled and requires explicit routing consent, a healthy dedicated broadcaster, public gateway host and distinct UDP ports, ready BDS with online-mode=true, the pinned worker, and fresh matching backend protection evidence. Missing or denied firewall inspection blocks Online while LAN stays independent. On this PC, read-only ActiveStore inspection returned Access denied under the unelevated user; no effective protection is claimed.

Before any live change: review the generated backend protection plan and rollback inventory, establish an approved least-privilege way to inspect effective policy, and test the proposed BDS host-only restrictions against the existing helper path. Do not expose the existing BDS endpoints. Gateway forwarding, NAT/hairpin/CGNAT, Xbox profile entitlement, Friends publication, console resource packs, authorized remote joining and unauthorized/direct-backend refusal require controlled physical tests. A matching policy snapshot is not proof those tests passed. Local Windows processes/administrators remain trusted; address churn requires stopping access and reconciling policy.

The worker reports Advertising only after library Start succeeds. RemoteVerified remains false: there is no fabricated external success. No automatic publication or installation has occurred.

## Validation and rollback

Tests cover protocol/IPC bounds, credential projection, identity/account/server binding, pre-pack ACL denial, helper/backend gates, active revocation, retry/auth/readiness transitions, failed-stop ownership, stale firewall evidence/IP drift, role-safe persistence, native Online controls and Windows child/descendant shutdown. Full release checks and candidate checksums are recorded beside the candidate installer after packaging.

Existing live binaries remain the rollback baseline. A later approved installer update must back them up first. If non-helper roles have been enrolled, use the matching new Accounts UI to unassign/remove or change those identities before returning to a schema-1 helper, or restore the protected pre-test account-store backup. Do not restore a world snapshot merely to roll back panel binaries. Remove only explicitly recorded generated firewall rules if a future approved test applied them; stop the gateway first. Never delete or overwrite worlds.