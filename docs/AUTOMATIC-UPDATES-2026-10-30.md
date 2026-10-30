# Automatic updates — 30 October 2026

The Updates tab provides independent Automatic server updates and Automatic add-on updates switches. Settings persist in MultiServer/automatic-updates.json and travel in private snapshots. New installations default off; this user's installation opts into both. The helper owns scheduling, so closing the panel does not disable updates.

The worker checks every 15 minutes after every managed world is running, Ready and empty for five minutes, with no active helper lease. Busy snapshots, server update/recovery and pending manual changes defer automatic work. Turning either switch off prevents future work for that category; work already applying finishes recovery/installation normally. Status reports the last check and result.

Server updates use the existing official stable release, NetherNet protocol compatibility, anti-downgrade, clean-stop, verified full backup and recovery pipeline. Empty-player stopping is checked under the supervisor lock immediately before moving a world to Stopping. A player joining during download defers stopping. Previously running worlds restart; manually stopped/unready worlds defer the automatic cycle.

Managed add-ons use existing API credentials and per-pack sources, archive UUID/version verification, engine/dependency validation, tamper-resistant previews and provider staging. Only enabled packs with a newer matching UUID are eligible. Manual and custom/protected packs are excluded; any protected companion in a candidate archive rejects automatic staging. Changes are serialized with other management operations. Targets stop only if empty, then the verified review stages and normal startup applies it with the existing pack/activation backups and rollback path. A partial-stop failure restarts already-stopped worlds without staging. Startup failures retain backups/pending state for review. A source UUID change or an unrelated archive requires manual review rather than automatic replacement.

Server update backups and pack rollback staging remain on the protected local runtime volume; durable complete snapshots are published to the configured Drive Backup Snapshots folder through the existing snapshot workflow. Automatic updates do not change this user's protected RG Shader or Modern Furniture.

Tests cover persistent independent opt-in/out, corrupted settings failing closed, the five-minute grace period and 15-minute schedule, protocol and active-player deferral, disabling during maintenance, protected companions and recovery after a second world refuses an empty-only stop.
