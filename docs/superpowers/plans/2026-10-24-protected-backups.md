# Protected backups and scheduled warnings implementation plan

> **For agentic workers:** Use superpowers:executing-plans to implement this approved plan inline.

**Goal:** Keep manual snapshots permanently protected, retain seven verified scheduled snapshots, and warn online players before automatic backups.

**Architecture:** The helper owns provenance, countdown and retention under its existing snapshot operation lock. A local ledger authorizes deletion of exact scheduled archive files; archive metadata and hashes independently verify ownership. The panel and scheduled control command select distinct destination subfolders.

**Tech Stack:** Go, Windows Bedrock console, Walk, existing Task Scheduler and single EXE release scripts.

**Spec:** User-approved design in this conversation: Protected/manual, Scheduled/automatic, seven verified scheduled copies, new backup before oldest removal, five-minute in-game warnings regardless of player count.

## Global constraints

- Never delete protected, legacy, untracked or unrelated files.
- Failed capture, verification or restart must not delete backups.
- Preserve worlds, achievements, identities and existing schedule settings.
- Stage and construct archives locally; publish verified ZIP bytes only.
- Update installed application and Defender-tested single EXE on Drive.

## Review focus

- Corrupt/missing ledger or archive: retain everything and expose a warning.
- Links/traversal/foreign provenance: no deletion outside exact tracked Scheduled files.
- Players remain online or join late: countdown still runs and backup proceeds.
- Cancellation during warning: no stop, cancellation announcement.
- Restart failure: snapshot survives and old backups remain.

### Task 1: Announcement and countdown

Files: internal/supervisor/announce.go and tests; internal/migration/warnings.go and tests; capture.go.
Interface: Announce(context.Context,string,string) error writes one JSON-escaped tellraw to an owned running process. Scheduled warning uses injectable waits for tests.
- [x] Write tests for literal JSON text, command injection rejection, five-to-one warnings despite players, cancellation and announcement failure.
- [x] Observe failing tests, implement console announcements and cancellable countdown before graceful stop.
- [x] Run supervisor and migration tests.

### Task 2: Provenance and retention

Files: internal/migration/retention.go and tests; manifest.go; addon_options.go; capture.go.
Interface: snapshot kind manual/scheduled; persistent installation owner; local retention ledger tracks exact path, job, hash and creation time.
- [x] Write tests for eighth/ninth rotation, protected/unknown preservation, corrupt archive/ledger and path rejection.
- [x] Observe failures, implement full validation before deletion; remove exact archive then empty dated folder only.
- [x] Integrate after successful publication and restart; expose nonfatal warnings.
- [x] Run migration tests.

### Task 3: UI/API/scheduled workflow

Files: adminapi/messages.go; service/migration.go; snapshotschedule/schedule.go and tests; admin migration views.
- [x] Test scheduled kind/path and protected destination, service forwarding, countdown summaries.
- [x] Implement destinations and clear retention/countdown messaging; preserve user schedule.
- [ ] Run focused tests and whole Go suite.

### Task 4: Review, release and installed update

- [x] Obtain independent read-only review and resolve findings.
- [ ] Build all three binaries; commit source; create release/source ZIP and verify hashes.
- [ ] Defender-scan/test single EXE and publish verified latest installer to Drive.
- [ ] Verify no active players or snapshot, gracefully update installed binaries, verify server readiness and endpoints.
- [ ] Do not create a real snapshot or enable a schedule; user will do these.

Execution record: console announcement and countdown tests failed before implementation, then passed. Retention ownership/rotation tests failed before implementation, then passed. Independent review found a blocking console-write risk; a failing blocked-writer regression demonstrated it, and bounded writes plus the pending-write guard resolved it. Focused migration suite and Go vet passed. All production worlds and prior snapshots remain untouched. Ruling: existing enabled schedule is preserved; user will select a new schedule and create their manual protected copy.
