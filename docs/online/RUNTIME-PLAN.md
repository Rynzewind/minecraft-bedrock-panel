# Authenticated Friends gateway implementation

User decision, 29 October 2026: preserve existing LAN entries and permit an unknown-location Friends join through the authenticated remote gateway. A local device choosing Friends may use the Internet/hairpin route. This is explicit consent to that routing limitation, not an identity or access-control exception.

The working installation stays unchanged until the isolated runtime has passed review. Implement:

1. A protocol-2225 isolated broadcaster with private inherited-pipe token and authorization requests. Accounts remains the only owner of refresh credentials and sign-in.
2. A separately bound online-authenticated gateway that checks the real console XUID before acquiring an existing helper lease or opening a backend connection. Revocation and disabling close affected gateway sessions.
3. A fail-closed backend protection plan and inspection of effective Windows Firewall policy. Prepare network changes for review; never activate them automatically.
4. Parent-owned worker supervision tied to server readiness, protection evidence, dedicated account health, policy and explicit routing consent. Stop workers on disable, backend stop, maintenance, shutdown and stale protection evidence.
5. Accurate UI states and diagnostics, isolated tests, full regression checks, review and a candidate installer. External console and firewall/LAN verification must be identified separately from simulated tests.

No live installation, public installer replacement, router changes, firewall mutation or claim of verified remote joining is authorized by merely saving settings.
