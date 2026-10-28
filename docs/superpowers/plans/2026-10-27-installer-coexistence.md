# Installer coexistence repair implementation plan

Goal: install persistent server aliases while preserving active DHCP networking, and preserve original coexistence state during rollback/uninstall.
Architecture: read active and persistent flags independently, set and verify both before alias creation, restore both from new baselines while supporting legacy records. Keep existing primary/ownership validation.
Constraints: no world/account/production configuration edits; use local source copy. Preserve release hashes, packaging workflow and backup prior published EXE.

- [ ] Extend network harness to model active/persistent state separately and fail when aliases added without active coexistence. Add mixed-state rollback and uninstall scenarios; confirm failures before fix.
- [ ] Fix read/enable/restore logic and journal fields with legacy fallback; verify all network harness scenarios.
- [ ] Investigate Accounts button visibility on current panel. Only change layout if reproduced; preserve account IDs and assignments.
- [ ] Update pinned script hashes. Run full Go tests and vet, Windows UI tests and release package harness; Defender scan with protection enabled.
- [ ] Publish verified installer preserving previous EXE; align installed app only with safe graceful maintenance if needed.
