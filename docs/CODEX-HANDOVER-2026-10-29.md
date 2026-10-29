# NetherNet handover — 29 October 2026

This is a deliberately non-sensitive continuation note. It contains no passwords,
Microsoft account addresses, authentication tokens, API keys, public IP addresses,
world files, or player data.

## Repository and live installation

- GitHub repository: `Rynzewind/minecraft-bedrock-panel`.
- Current work branch: `codex/online-private-access`.
- The Windows deployment is under `C:\MinecraftServers`; it is separate from this
  repository. Never overwrite an existing world during restore or deployment.
- The administrator panel, helper, server worlds, account stores, identities,
  backups, logs, and provider credentials must remain outside Git. `.gitignore`
  is intentionally configured for that.

## What is implemented

- A maximized, scaled Windows administrator panel with server, content-pack,
  accounts, player-history, snapshot, updates, and Online sections.
- Protected snapshots and restore-to-a-fresh-folder workflow.
- Manual Minecraft Bedrock server update workflow with backup and progress UI.
- Add-on source tracking, protected CurseForge API-key storage, verified update
  checks, amber **Unknown** state, reviewable staging, and selected/all outdated
  update actions that offer restart-and-apply after staging.
- Online Xbox Friends gateway preparation with separate Creative and Survival
  broadcasters, account-based player access, and an Online authorised-player
  selector that assigns Creative, Survival, or both from existing authenticated
  accounts.
- Fail-closed Windows firewall inspection for the Online gateway. It must never
  advertise or report a remote route as verified unless protection evidence is
  current and a real outside-network test has succeeded.

## Current Online test status

The Bedrock servers and the local gateway configuration are healthy, but both
Online entries currently report **Blocked** in this Codex-hosted Windows session.

Reason: the Codex Windows sandbox installs a broad inbound firewall block. Windows
gives that block priority over NetherNet's narrow gateway allow rules. NetherNet is
correct to fail closed: advertising in this state would be misleading, because
remote UDP traffic cannot reach the gateway.

Important:

- Do **not** disable Windows Firewall to work around this. That would also remove
  the host-only Bedrock protection that Online requires.
- Do **not** remove or bypass the Codex sandbox firewall rule from NetherNet.
- Do **not** mark a remote route verified until an approved Xbox account has joined
  from a genuinely separate internet connection (for example, mobile data).
- A restart did not clear the block in this environment. Use a Windows host/session
  without the Codex sandbox for the real remote test, or consider an external relay
  only as a separately designed future project.

## Safe next steps

1. Preserve this repository and the live Minecraft folders as separate assets.
2. If Codex is uninstalled for a clean external-network test, first retain this
   handover and the repository. Reinstall Codex later for further development.
3. On a non-Codex host/session, confirm Online changes to **Advertising** before
   testing Xbox Friends access.
4. Test only from an approved player account and a separate internet connection.
5. Record the outcome, including which world appeared and whether joining worked;
   only then consider the remote route verified.

## Verification before future changes

Run the focused Go test suites before building or deploying:

```powershell
go test ./internal/onlineguard ./internal/service ./internal/addonupdates ./cmd/nethernet-multi-server ./cmd/nethernet-multi-server-admin
```

Keep any new real credentials and deployment-only files out of Git. Review
`git diff --check` and `git status` before every commit and push.
