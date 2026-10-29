# Online metadata and Friends signaling

Two distinct gateway ports originally answered list pings with Gophertunnel's
defaults: Minecraft Server, Gophertunnel, Survival and a dynamic one-player
capacity. This did not mean both gateways used the same backend; it meant the
gateway listener had no per-server status provider.

The gateway now reports its configured world label, level, game mode and
capacity. The parent also passes mode and capacity through the inherited worker
bootstrap. The worker validates these optional fields and maps its canonical
world type to the broadcaster constants. Existing authentication, deny-by-default
XUID policy, helper identity mapping, private credential handling and backend
protection are unchanged.

Separate broadcaster processes now explicitly use Player Messaging/JSON-RPC
signaling (SupportedConnections type 7), documented by the pinned protocol
library as the normal current-client path. The upstream default selected the
older type-3 WebSocket path. No sub-accounts, friendship mutation, invitations,
privacy changes or automatic following are enabled by this change.

Physical client visibility and joining remain acceptance tests, not inferred
from publication, online presence, mutual social relationships or external ping.
The reported client issue was a matching-version console seeing online accounts
but no joinable worlds. Do not mark RemoteVerified until the real tests pass.

Regression coverage includes real authenticated gateway pongs for distinct
Creative/Survival/Adventure metadata and pure status/capacity checks, plus worker
bootstrap metadata and explicit signaling selection. The real-ping test requires
NETHERNET_GATEWAY_NETWORK_TEST=1 because authenticated listener startup performs
Minecraft service discovery; it never admits players or dials a backend.
