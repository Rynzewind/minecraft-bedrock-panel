# User release requirements

For every future installer update, build the single-EXE installer, test and Defender-scan it, then publish a checksum-verified latest copy to `G:\My Drive\Minecraft Server Setup\NetherNetSetup.exe`. Use `scripts/release-single-file-setup.ps1`, which implements that workflow and backs up the previous Drive installer before replacement. Keep the existing installed copy and release package aligned when the user requests an application update.

Snapshot defaults belong under `G:\My Drive\Minecraft Server Setup\Backup Snapshots`, with each snapshot in a dated subfolder. Keep raw snapshot staging and archive construction on the local installation volume because Google Drive rejects protected Windows ACLs. Only publish the verified final ZIP bytes to the selected destination; never weaken staging privacy or delete user worlds/backups while packaging a release.
