# Add-on updates and CurseForge integration

## Intent and chosen approach

Extend Updates → Add-ons into a useful manual updater and a ready-to-use CurseForge integration. The user selected implementing automatic checks and selected-release download/reviewable staging now, tested with simulated responses until an approved API key is available. The alternative of key settings/source tracking only was considered and not selected.

Use the existing Go service, authenticated local administration pipe, Walk UI, DPAPI protector and content-pack validation/backup machinery. Keep the established Minecraft theme, scrollable content and compact action groups. No unattended installation, server restart or automatic activation.

## Key settings and installation

The Add-ons subtab has a masked API-key entry, Save/Replace, Test connection and Remove. The helper stores the key with user-scoped Windows DPAPI and a protected local credential file; APIs return only configured/test status, never the key. Clear the UI input after saving. Do not put the key in command lines, errors, logs, Git, release packages or snapshot archives. Never send it to a project website or download CDN; API requests use only the official CurseForge API host and bounded timeouts. Fail safely on redirects and sanitize failures without echoing response bodies or request headers.

The installer includes the updated application. Key entry happens in the installed panel; the installer never contains a preconfigured key. Reinstall/update on the same PC retains existing local credentials. Restoring to another PC requires entering the key again. Existing worlds, accounts, identities, settings and add-on state remain preserved.

## Sources and manual workflow

Migrate the existing UUID → HTTPS source-link file without losing links. Store structured provider records per pack: source URL, optional CurseForge project ID, release channel (stable by default), last checked time/result and last explicitly acknowledged file/version. Preserve useful non-secret metadata in snapshots. Report unreadable state as an error rather than an empty/up-to-date list.

Show installed library version, deployed versions/enablement on each server, source/provider and update state. Unknown, unlinked, manual-review-needed, API-unconfigured, access-denied, incompatible and update-available are distinct outcomes. A failed or missing check is never shown as current.

Manual actions: edit/open source, record a manual check with release/version notes, choose a local .mcaddon/.mcpack/.zip, and preview an update. Archive inspection must confirm matching selected UUID, increasing versions, paired resource/behavior packs, declared UUID dependencies and minimum engine version. Unrelated pack additions and UUID replacements cannot silently pass as updates. Source HTML is not scraped and provider download restrictions are not bypassed.

## CurseForge provider

Implement a provider interface and an official-API client. Discover the Minecraft Bedrock game through API metadata instead of guessing an ID. Resolve explicit project IDs or exact source slugs, verify the game and project match, paginate release files, and select stable compatible releases by default. Beta/alpha selection must be explicit. Compare provider file IDs with acknowledged releases; use actual pack manifests for installed version decisions rather than trusting a display filename.

Checks are read-only. A user chooses a candidate release, which is downloaded into protected local staging with bounded size and timeout and validated against provider hash/size metadata and normal archive limits. The download client never receives the API key. Missing or author-restricted download URLs keep the source-page/manual path available. Handle 401/403/404/429, provider downtime, pagination and incompatible files explicitly.

## Preview, staging and activation

Manual and provider downloads converge on one immutable update preview: archive hash, current/target UUID versions, paired packs/dependencies, affected selected servers, pending operations and warnings. Recheck archive hash and current library/deployed state at commit; stale previews must be regenerated.

Staging requires explicit confirmation and selected target servers, rejects conflicting maintenance and pending edits, and pins exact reviewed versions. Retain old library versions and per-server deployed version metadata. Unselected or disabled servers retain their current state; later unrelated Apply operations must not pull newer library versions implicitly. Migrate legacy state conservatively using existing world pack selections where available.

Import/stage state changes are transactional. Failure leaves the previous library pointers, enabled selections and pending operations intact; staging never writes into running server pack/world folders. Existing Apply/Restart performs activation only while stopped with verified backups and rollback. No server is stopped merely to check, download or preview an add-on update.

## Validation and delivery

Test legacy source/state migration, DPAPI round trips, no plaintext key persistence or API reflection, credential exclusion from snapshots/release packaging, malicious URLs/redirects and sanitized failures. Simulate provider authentication/authorization/rate limits, pagination, stable-channel selection, game/project mismatch and restricted downloads. Test wrong UUID, unchanged/downgrade versions, paired packs, missing dependencies, incompatible engine versions, corrupt archives, stale preview rejection, transactional failure and per-server version isolation.

Native UI tests cover scrollability, masked entry/status, review actions and selection preservation. Update project documentation and website to distinguish implemented provider support from live-key validation. Build and test the single-EXE installer, Defender-scan with protection enabled, checksum-publish to the established Drive/Desktop destinations and GitHub release, and keep installed application/source in sync. Real CurseForge connectivity remains unverified until the user's approved key is entered locally. No key is required to use the manual path.

## Builder-check resolutions (implementation choices)

- Checks are on demand through Check selected / Check all linked packs. “Automatic” means API lookup/comparison, not a background schedule in this release. No periodic polling is added while the panel is closed.
- A manual check records a timestamp and optional notes; it never proves a pack current or suppresses a newer candidate. Provider file identity is stored with an accepted staged archive as provenance. Installed/deployed manifest versions remain authoritative, and a different provider file is a candidate for review until its manifest is inspected.
- The selected installed pack must increase in version. A bundle may include unchanged companions, but they are retained/skipped rather than silently replacing same-version contents. New UUIDs are allowed only as explicitly reviewed required dependencies connected to the selected pack; unrelated additions are rejected from the update path. A required dependency absent from the bundle/library blocks staging.
