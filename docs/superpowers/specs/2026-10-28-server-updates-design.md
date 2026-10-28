# Manual server updates

Add a compact Updates action to the existing Windows panel without increasing its minimum window size. It shows installed Minecraft and supported helper versions, checks official stable releases, and offers an explicit update preview. No background installation or preview releases.

## Approaches

The user selected a manual panel action over a separate updater. Automatic installation was rejected because updates interrupt play and can affect add-ons. Keep the existing Go/Walk application and its administration service.

## Compatibility

Treat the Bedrock server and NetherNet proxy as one compatibility set. The current server is 1.26.51.1 and the proxy supports protocol 2193; the console is 26.60. An update must use a tested helper supporting the target protocol. Changing advertised version numbers alone is insufficient. If helper support is unavailable, explain the blocker before stopping servers or modifying files.

## Installation

Download official stable server software to protected local staging, validate the archive and release metadata, reject unsafe archive paths, and show source, target version, affected servers, and backup location. Require no connected players and prevent conflicting maintenance operations. Stop affected servers gracefully, verify process exit, and create verified local backups of worlds, custom packs, configuration and previous software before modification.

Replace version-owned software while retaining custom add-ons, worlds, server.properties, permissions, allow lists, account credentials, identities, network bindings and existing backups. Install matching proxy software when required and update only relevant version/protocol metadata. Restart only servers previously running and check their startup version and network readiness.

## Recovery

Retain a transaction journal and previous software. Before a new engine opens a world, failures may restore previous software automatically. After a world has been opened by the new engine, do not run an older engine against that world: stop safely and offer explicit recovery using the pre-update world backup. Never silently discard post-update play.

## Delivery and validation

First provide the matching 26.60 update, then deliver the reusable panel workflow in the same release. Test preservation, compatibility refusal, corrupt downloads, graceful-stop failure, restart failure, recovery and normal update. Build and Defender-scan the installer and publish checksum-verified Desktop and Drive copies with previous releases retained. Actual console join remains a user verification step.
