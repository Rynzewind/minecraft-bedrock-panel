# Current-client Online gateway transport repair

The original public gateway accepted RakNet game connections on UDP only. A
Bedrock status ping could succeed while a current client's address-join failed:
current clients discover NetherNet through HTTP GET /v1/join, then submit a signed
SDP offer and establish WebRTC. The old gateway had no TCP signaling listener.

Each gateway now owns TCP signaling and one ICE-multiplexed UDP socket on the
same configured port. Router forwarding and Windows Firewall must permit both
TCP and UDP for those gateway ports only. Backend endpoints remain host-only.

The UDP socket binds to the configured adapter's literal IPv4 address. Its ICE
host candidate uses the IPv4 resolved from the public gateway hostname and the
same forwarded port. DNS/WAN address changes require gateway reconciliation;
fresh host protection evidence is still independently checked. No STUN/TURN
configuration or third-party relay is introduced.

The operator identity is loaded from the exact identity directory used by LAN,
including when the selected config is outside the executable directory. Keys
remain local and persistent; no world/account identity is replaced.

Client SDP tokens must pass Minecraft OIDC signature, issuer, audience and expiry
verification, plus the current per-world XUID policy, before the public key is
used to verify the SDP fingerprint signature. Anonymous offers are rejected.
Minecraft Login independently authenticates and rechecks policy before private
resource packs. Existing assigned-helper leasing, backend dialing, inventory
identity, session revocation and protection checks remain in force.

The protocol SDK can attach full SDP offers to negotiation errors. Its log handler
therefore discards all raw messages/attributes and emits fixed diagnostic stages
only. Tokens and SDP bodies never enter helper logs.

Legacy UDP server-list pings receive a bounded status-only response through the
ICE mux's unhandled-packet callback. Legacy RakNet game sessions are not accepted.
The callback never blocks: requests are copied into a bounded queue, with excess
requests dropped. This retains public status metadata without another UDP port.

HTTP header/body/time bounds remain enforced. Closing or disabling a gateway
closes HTTP signaling, the game listener, UDP mux and active sessions, without
owning LAN or BDS lifecycle. Tests verify discovery metadata, valid/invalid signed
token claims, ACL revocation, anonymous SDP rejection, log redaction, interface
selection, preserved status pongs and TCP/UDP cleanup. Real external console
joining is a separate acceptance test and must not be inferred from these tests.

Reference: https://mojang.github.io/bedrock-protocol-docs/guides/nether-net-onboarding-guide/
