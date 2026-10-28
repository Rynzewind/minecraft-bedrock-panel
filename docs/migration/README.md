# Minecraft snapshots and Windows migration

This release preserves the confirmed Xbox LAN answer compatibility change and the 90-second backend login allowance from the October 8 build. It adds a Migration / Snapshots tab to the administrator and a separate Windows setup application.

## Make a snapshot

Open the administrator on the server PC and select **Migration / Snapshots**. Choose a folder outside the Minecraft installation, then click **Create migration snapshot**. The confirmation shows connected players. Running servers stop cleanly while their files are copied, then previously running servers restart before compression and verification. A server that was already stopped stays stopped. Keep the completed ZIP and the single-file `NetherNetSetup.exe` installer together for a move.

The ZIP includes both installed Bedrock servers and worlds, resource/behavior packs and add-ons, permissions and allowlists, persistent identities, panel configuration, account IDs and player mappings, launchers, and the installed helper/admin/control/setup binaries. SHA256 checksums cover every payload file. World database transaction logs are retained. Old backups, diagnostic logs, diagnostic builds and Microsoft login tokens are excluded. The engineering source is included as a separate ZIP inside the snapshot when installed with this release.

Snapshots are private, not encrypted. They contain world/player data and server identity keys. The application restricts local Windows permissions, but storage/transfer tools can change those permissions. Treat the ZIP like your private server data.

Cancelling waits for any pending graceful stop to settle, then restores the original running set. If a restart fails, the panel reports it separately from archive validity. Interrupted captures are marked failed on the next helper start and their own recorded scratch files are cleaned up. Check server states after a power failure; the application does not assume a crashed helper restored every server.

## Move to another Windows x64 PC

1. Make a fresh snapshot. Keep the old installation until the move has been verified.
2. Copy the snapshot ZIP and `NetherNetSetup.exe` to the destination PC. The EXE contains the installer assets and opens the restore wizard automatically.
3. Obtain Microsoft's official Visual C++ v14 x64 runtime installer in advance if the destination lacks it and must be installed offline. Setup checks the installed DLLs and accepts the separately obtained, signed Microsoft installer. It is not bundled in this release. Official page: https://learn.microsoft.com/en-us/cpp/windows/latest-supported-vc-redist/
4. Run setup as the Windows user who will operate the panel and authenticate the Microsoft accounts. Do not use Run as administrator for the initial restore; setup requests elevation only for the Windows configuration step.
5. Choose the snapshot, a NEW server folder, and the actual LAN interface. Setup verifies the archive and restores it without starting the servers. Existing destinations are rejected rather than overwritten. Backend addresses, discovery broadcast and configured paths are rewritten for the new PC.
6. Stop the old deployment, including any startup task that could restart it. Confirm this in setup before activating the new one. The two deployments preserve the same server identities and must not advertise simultaneously.
7. Select manual start, start at sign-in, or start at Windows boot. Boot startup requests the intended user's Windows PASSWORD, not a PIN; the password is passed directly to Windows Task Scheduler. Startup tasks run with limited privileges as that same user. Setup does not silently change a Public connection to Private: if asked, set the trusted LAN to Private in Windows Settings and retry configuration.
8. Click **Configure Windows and activate**. Firewall access is limited to the restored executables, UDP, Private networks and LocalSubnet. If configuration fails or a runtime installation requires a reboot, correct the issue and retry this step using the already restored folder; do not restore over it.
9. Open the restored admin panel. Reauthenticate each Microsoft account: old-PC DPAPI credentials cannot be reused. Account IDs and player mappings remain intact. If you opened the panel before activation, stop that helper with StopMinecraftServers.bat and reopen it after configuration so LAN listeners start.
10. Check Xbox joins to Creative AND Survival, a previously working console, packs, permissions, multiple players and the selected startup behavior. Account sign-in needs internet even when file restore is offline. Reserve the chosen LAN IP in your router or keep it stable; re-run migration setup with a fresh restore if moving to a different network later.

For current-server starts use StartMinecraftServers.bat, then open MultiServer/NetherNetMultiServerAdmin.exe. StopMinecraftServers.bat gracefully stops the servers and helper. Do not launch Bedrock separately from the helper if you want managed snapshots.

## Install this panel update on an existing installation

Use Install-Panel-Update.ps1 from the release folder. It verifies release hashes, refuses to interrupt connected players, backs up the replaced binaries and source ZIP, and preserves running/stopped state. Close the administrator first. The update does not replace world data, server properties, accounts or identities.

For a rollback, stop the managed deployment cleanly, copy the previous helper/admin/control files from the reported MultiServer/backups/migration-panel-* folder back into MultiServer, and use the original launcher. Retain the new setup executable separately if still needed. Never replace the helper with the original pre-Xbox-fix or timeout-only handover build.

## Command-line tools

These modes do not configure firewall rules or startup tasks:

```
# Historical developer commands for the legacy setup binary (not the current single-file installer):
NetherNetMultiServerSetup.exe --verify SNAPSHOT.zip
NetherNetMultiServerSetup.exe --restore SNAPSHOT.zip NEW_FOLDER IPv4 BROADCAST_IP
NetherNetMultiServerSetup.exe --capture-offline SERVER_ROOT OUTPUT_FOLDER
```

For scripted use, launch this GUI executable with PowerShell `Start-Process -Wait -PassThru` and check its ExitCode; a bare invocation may return before the operation finishes.

The offline capture mode refuses an externally running server and is intended for a fully stopped installation. It does not start any servers. Run it as the same Windows user who owns the account store. Restore writes an activation marker; complete the setup wizard before LAN advertising is enabled.

## Verification boundary

Automated checks cover archive integrity/exclusions, portable account metadata, private Windows permissions, stop/restart/cancellation recovery, interrupted-job cleanup, mutation gating, fresh-path/subnet restore, activation failure/retry behavior and window creation. The complete Go suite and Go vet are run for the release. The existing Xbox compatibility and backend timeout regression tests remain in the suite. New-PC UAC interaction, real startup-task registration/reboot and physical console joins require destination-machine acceptance. No test result should be read as a new physical Xbox or boot verification.

## Implementation decisions

- Work uses a separate extracted repository; the old confirmed source remains available.
- Management mutations are blocked for the full snapshot job, including compression; gameplay can resume once copying ends.
- Restored accounts are unhealthy with an empty JSON credential object protected locally by DPAPI. Zero-byte input was rejected by Windows DPAPI; no old credential is exported.
- Microsoft runtime is supplied separately because redistribution entitlement has not been established. Setup links to Microsoft and validates the signature of an optional supplied x64 installer.
- Implementation and final review were performed by the same agent at the user's request. No independent code review was performed.


Windows configuration uses native UAC and readable, checksum-verified PowerShell scripts bundled inside `NetherNetSetup.exe`. The installer extracts its own assets; carry the single EXE and snapshot ZIP to the destination.


On the existing server PC the current installer lives in `MultiServer/installer`. Use that complete folder, or the supplied Ready setup ZIP. Older setup executables at the top of MultiServer and earlier test packages are superseded; preserved rollback files are not the current installer.


Automatic full snapshots can be enabled in the Migration / Snapshots tab. Select daily, every 2-6 days or weekly, choose the local hour/minute and weekly day, then click Save schedule. Windows retains the interval between runs and the panel shows the last scheduled attempt and next run. The default remains Monday 00:00. Saving a schedule does not force an extra snapshot. All automatic snapshots include the full addon library.

