# Add-ons overview verification

The installed-pack overview and cogwheel-owned settings dialog were approved in chat. Unknown uses the existing Restart theme's amber; current/outdated use green/red. Native tests verify exact colours, masked settings controls and explicit staging targets.

Status tests cover absent/stale/failed evidence, changed provider files, older deployed versions, source/channel changes and wrong-UUID archives. A source check may download a bounded, permitted archive to verify its manifest, but never imports or activates it. The normal panel refresh keeps deployed versions and evidence expiry visible without resetting unchanged rows.

An independent review found selection preservation and missing-key status regressions; both were reproduced and fixed with native regression tests. The reviewer then reached its service usage limit, so remaining lifecycle and evidence paths were reviewed locally. Async loads ignore stale responses; settings are scrollable, monitor-clamped and block closure while an operation is active. Live CurseForge account access remains unverified until an approved key is entered, and live screenshot capture remains unavailable.
