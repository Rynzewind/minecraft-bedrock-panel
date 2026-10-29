# Optional authenticated Online path

Existing LAN client -> existing authenticated NetherNet listener -> assigned helper lease -> existing authenticated NetherNet BDS.

Friends client -> isolated Xbox broadcaster (verified login + parent XUID policy) -> direct Transfer to public authenticated gateway -> verified console XUID ACL -> same assigned-helper session relay -> existing BDS.

The broadcaster has no refresh-token store and no backend access. Its parent supplies only access tokens over inherited anonymous pipes, bound to one server/account. Stdout accepts only bounded typed RPC and five fixed event codes. The parent verifies the pinned component protocol/capabilities before binding a gateway. Windows starts it suspended, assigns a kill-on-close job, then resumes it; process exit is observed independently of pipe EOF.

The gateway authenticates before resource packs and checks permissions again before/after helper leasing and backend dialing. Its own watchdog closes revoked sessions independently of the account-manager reconciliation loop. Existing helper mappings preserve inventory identity. No upstream insecure relay or online-mode=false shortcut is used.

The coordinator owns only Online listeners/workers. It follows Servers readiness and its synchronous stopping transition, has bounded recovery, holds failed authentication for Accounts reauthentication, and cannot replace an unconfirmed surviving worker. Online loading/startup failure cannot stop LAN startup.

Backend protection is a separate real policy prerequisite: generated per-program rules block inbound sources outside the complete host address set over IPv4/IPv6. Effective ActiveStore evidence includes rules/filters, profiles, filtering services, address inventory and age. Each admission re-evaluates fresh evidence/current addresses. A snapshot does not prove packet behavior: host-local helper connectivity, direct external backend denial, router and console joining still need actual tests. No firewall mutations are performed by the runtime.

User-approved routing limitation: LAN entries stay local, while unknown-location Friends clients may use the authenticated public gateway, including local hairpin routing. New configurations require explicit consent. ICE/private candidates never claim physical-locality proof. RemoteVerified stays false until a separately evidenced external test workflow exists.