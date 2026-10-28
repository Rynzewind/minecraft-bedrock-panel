# Dated snapshots and weekly full snapshots

Default the migration page to G:\My Drive\Minecraft Server Setup. Manual and scheduled snapshots target a local-time YYYY-MM-DD_HH-mm-ss child directory, with the original base folder retained in the page. Add a persisted checkbox that registers a current-user Windows weekly task for Monday 00:00. The task calls the installed control executable and always includes addons, waits for verified snapshot completion, and logs failures. It can run without the panel window; it requires the Windows user signed in and the destination available. StartWhenAvailable permits a missed run after the PC becomes available. Disabled by default until the user checks it. The checkbox explains the brief graceful server downtime. Preserve existing snapshot validation and world safety. No helper changes or live server restart required.

1. Add tested destination/settings and full-snapshot runner in internal/snapshotschedule.
2. Add project-owned Windows task registration/removal and a scheduled snapshot control command.
3. Add migration-page default, dated manual destination, persisted folder and schedule checkbox.
4. Test affected packages and native UI; rebuild admin/control, update installed copies without restarting servers, rebuild/scan ZIP and single-EXE installer.
