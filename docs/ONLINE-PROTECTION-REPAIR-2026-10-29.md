# Online protection validation repair — 29 October 2026

The earlier handover's conclusion that Codex must be removed was incomplete.
Live diagnostics identified independent validation failures:

- The installed cached audit collector represented absent optional CIM values
  as `[""]`. The checker treated an empty excluded-interface name as a real
  firewall exclusion, and empty dynamic-address fields as rule narrowing.
- Windows reports singleton address ranges as individual addresses (for example,
  `::` instead of `::-::`). Equivalent coverage must compare equally.
- Additional-block checks ignored LocalUser conditions. The observed block was
  scoped to another account, while the helper and both BDS processes ran as the
  operator. A rule name is not evidence of applicability.
- Protection rules contained old IPv6 privacy addresses. Both configured backend
  targets used IPv4, so those IPv6 host-source exceptions were unnecessary.

The repair normalizes only harmless optional empty entries and singleton range
formatting. It proves a simple additional-block LocalUser SID disjoint by reading
every configured running backend's actual token user and all group SIDs. Unknown
SDDL, missing or unreadable processes, restricted tokens, and unknown enumeration
results remain blocking. Required protection rules never receive this exception.

Generated rules now exempt host addresses only in the families used by the
program's backend targets, plus both explicit loopbacks. Shared-program IPv4 and
IPv6 targets retain both families. IPv4-only targets block other IPv6 sources;
rotating IPv6 privacy addresses no longer require their rules to change. Complete
host inventory and evidence freshness checks remain in place.

Deployment requires replacing the helper with the tested matching build,
refreshing only the existing NetherNet-owned protection rules from its current
plan, and updating the cached audit collector from the matching source. Back up
the previous binaries and rules first. Do not remove Codex, edit its rules,
disable filtering, forward backend endpoints, or overwrite worlds.

Regression coverage includes empty-value normalization, genuine interface
exclusions, actual user/group token scope, unreadable/restricted tokens, missing
backends, required-rule mismatch, IPv6 privacy rotation and shared-program
dual-family endpoints. Successful local diagnostics still do not establish
external Xbox publication/joining or set RemoteVerified.
