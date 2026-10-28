# Add-ons overview implementation plan
Approved design: ../specs/2026-10-28-addon-overview-design.md
1. Add verified release evidence and truthful status comparison, with failing tests for missing/stale/mismatched sources, older deployments and failed checks.
2. Build installed-pack overview with existing theme colours and cogwheel-owned settings dialog, selection preservation and safe async lifecycle. Native tests cover dialog placement/masking/status colour.
3. Fresh whole-change review, full suite, installer build/extraction/wizard/corruption/Defender checks, publication and backed-up live update after panel exit.
