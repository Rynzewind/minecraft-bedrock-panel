# Add-on source audit — 30 October 2026

The original archive collection contains 20 .mcaddon/.mcpack files. Copies are retained privately in Drive under Add-on Originals. Installed manifests and English language files identify the generic pack.name entries as Airplanes and Grappling Hook. Source matching used author, description, filename and API archive UUID verification; names alone were not treated as proof.

| Add-on | Creator | CurseForge project | Evidence/result |
| --- | --- | --- | --- |
| Airplanes | TryzzMC / FALL Studios | [1592029](https://www.curseforge.com/minecraft-bedrock/addons/airplanes) | Both installed UUIDs verified |
| Machine Craft | DiomarBR | [1595389](https://www.curseforge.com/minecraft-bedrock/addons/machine-craft) | Both UUIDs verified |
| Alp's Complete Capitator | AlpDBang / AlpDSaymore | [1566344](https://www.curseforge.com/minecraft-bedrock/addons/alps-complete-capitator) | Installed UUID verified |
| Grappling Hook Add-on | Utskiin | [1482971](https://www.curseforge.com/minecraft-bedrock/addons/grappling-hook-add-on) | Both UUIDs verified; controls and description match |
| BioScape V.3 | Halo333XYT / El cordero / Kosmos | [1320257](https://www.curseforge.com/minecraft-bedrock/addons/bioscape-v-3) | Both UUIDs verified |
| Dynamic Light | RisabGamerz | [1369480](https://www.curseforge.com/minecraft-bedrock/addons/dynamic-lightning-lights) | Both UUIDs verified; original download provenance points to file 8601053 |
| Simple Storage | Existing project mapping | [1672786](https://www.curseforge.com/minecraft-bedrock/addons/simple-storage-addon) | Both UUIDs verified, release file 9268180 |
| Better Backpacks | HoneyStudios | [1494846](https://www.curseforge.com/minecraft-bedrock/addons/better-backpacks) | Author/project match; latest API release 9068416 lacks the installed UUIDs. Keep Unknown; do not bypass UUID checks |
| Modern Furniture | trotamundos872 | [829201](https://www.curseforge.com/minecraft-bedrock/addons/modern-furniture-survival-addon) | Suspected local modifications per user. Protect both packs; upstream reference only |
| RG Shader | RisabGamerz | [1238974](https://www.curseforge.com/minecraft-bedrock/texture-packs/rg-shader-v2-1-renderdragon-support-2) | User-confirmed local Skin Safe modifications. Protect current pack; upstream reference only |

Coverage: all 28 library packs now have source references. 23 have matching API archive UUID evidence; two Better Backpacks packs remain unverified against the current upstream release; three pack entries (RG Shader and Modern Furniture BP/RP) are protected custom packs. Verification evidence expires normally; matching UUIDs establishes identity, not compatibility or equality of locally modified files.

Only source settings and backups were changed. No downloaded add-on update was installed, staged or applied to either world. Original and locally modified archives stay out of GitHub. RG modifications are confirmed; Modern Furniture modifications are suspected, not independently proven by this audit.

The panel now resolves pack names/descriptions from texts/en_US.lang with en_GB fallback, including existing library metadata on helper startup. Manifest bytes, UUIDs and versions are not rewritten. Results are sorted Up to date, Unknown, Out of date, Custom / protected, with independent counted collapse/expand controls. Update all verified still considers hidden eligible rows. A custom-pack checkbox in source settings controls the persistent locally_modified flag. The helper refuses API checking/download/staging for protected packs, including cached provider reviews; explicit local archive review remains available for deliberate manual maintenance.
