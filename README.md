# NetherNet — Minecraft Bedrock Panel

A Windows desktop administrator for personal Minecraft Bedrock servers, with NetherNet LAN discovery and connection proxying.

**[Project website](https://rynzewind.github.io/minecraft-bedrock-panel/)** · **[Windows installer](https://github.com/Rynzewind/minecraft-bedrock-panel/releases/latest)**

## What it does

- Manage Creative and Survival servers from one themed desktop panel.
- Start, stop and restart servers; inspect readiness, player counts and logs.
- Manage helper Microsoft accounts, console-player mappings and roles.
- Import and stage local Bedrock add-ons independently for each server.
- Create protected snapshots, schedule backups and restore to a fresh installation folder.
- Check official stable server updates, verify protocol compatibility, back up before replacement and show update progress.
- See installed add-ons and green/red/amber update statuses; use the cogwheel settings for API access, sources, manual reviews and exact-version staging.
- Run in the Windows tray, with a maximized panel and layouts that support monitor scaling and scrolling.

The current helper supports **Minecraft Bedrock 26.60, protocol 2225**. A future Minecraft protocol change needs a matching NetherNet helper release; changing an advertised version alone is insufficient.

## Install and use

1. Download `NetherNetSetup.exe` and `SHA256.txt` from [Releases](https://github.com/Rynzewind/minecraft-bedrock-panel/releases/latest).
2. Verify the checksum with `Get-FileHash .\NetherNetSetup.exe -Algorithm SHA256`.
3. Run setup on your Windows server PC. Review the installation folder, LAN adapter and server addresses. Use a verified snapshot when restoring an existing deployment.
4. Reserve or exclude the chosen server addresses in your router's DHCP configuration before activation.
5. Open the panel, sign in to your helper accounts and check that the servers report Ready.

The website hosts project information and download links. The panel and Minecraft servers run locally on Windows. Obtain Bedrock Dedicated Server from [Minecraft's official download page](https://www.minecraft.net/en-us/download/server/bedrock); Mojang server binaries and third-party add-ons are not bundled in this repository.

Back up worlds before maintenance. Do not restore over an existing live world. A world opened by a newer Minecraft engine must not be run against an older engine; recovery uses the verified pre-update backup and retains displaced data.

See [Add-on updates](docs/ADDON-UPDATES.md) for manual review, protected API-key settings and release staging.

## Build from source

Requires Windows x64 and Go **1.26 or newer**. Native GUI/tray tests need an interactive Windows desktop.

```powershell
.\scripts\build.ps1
# Or specify your Go executable:
.\scripts\build.ps1 -GoExe 'C:\path\to\go.exe'
```

The build regenerates Windows resources with pinned `github.com/akavel/rsrc@v0.10.2`, runs the tests serially, builds the helper, panel and control tool, then creates a checksum manifest and release ZIP in a dated directory under `outputs`. Go verifies downloaded modules using its normal checksum settings.

To build only a particular command, use `go build ./cmd/nethernet-multi-server-admin` or another `cmd` package. The administrator's Windows GUI release uses `-ldflags '-H windowsgui -s -w'`.

For single-file installer packaging and Defender verification, see [Publishing releases](docs/PUBLISHING.md).

## Repository layout

| Path | Purpose |
| --- | --- |
| `cmd/` | Windows panel, helper, setup and command-line tools |
| `internal/` | Server supervision, proxy, accounts, updates, add-ons and migration |
| `scripts/` | Build, installer, network and maintenance tooling |
| `third_party/gophertunnel/` | Vendored protocol library with world-eligibility preservation patches |
| `docs/` | Operator/developer notes and engineering designs |
| `site/` | Static GitHub Pages project website |
| `config.example.json` | Example configuration; replace all sample addresses and identities |

## Private installation data

Worlds, player/account stores, sign-in tokens, server identity keys, local configuration, logs and backups are intentionally excluded. They belong on the server PC and in your private backup destination, not in a public repository. Example values and test fixtures do not represent a live deployment.

## Dependencies and notices

See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). The vendored Gophertunnel MIT license is preserved. The upstream code and its local modifications remain in `third_party/gophertunnel`; dependency versions are pinned in `go.mod` and `go.sum`.

This is an independent community project and is not affiliated with Mojang Studios or Microsoft.


