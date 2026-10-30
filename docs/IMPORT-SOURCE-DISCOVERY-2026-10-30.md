# Import source discovery — 30 October 2026

Import add-ons now attempts to fill the CurseForge project link and numeric ID automatically using the saved API key. Source lookup runs after a successful import. A failed/missing API lookup does not undo the import and is reported separately in the completion summary.

Identification uses Windows' retained CDN download URL/file ID where available, then CurseForge's exact fingerprint lookup, then at most three normalized name searches with ten popularity-sorted candidates each. Names and download metadata are hints only. A source is accepted only when file length and all supported official archive checksums match, the project belongs to Minecraft Bedrock, and its official project URL is valid. Fuzzy matches, ambiguous matches and changed archives remain manual. The archive contents are never uploaded; only a file ID, normalized fingerprint or public pack search name is sent to the API.

Actual testing found CurseForge's fingerprint endpoint returned no match for a known Bedrock original whose file metadata fingerprint was correct. Consequently it is only one optional lookup route. Both the download-origin route and checksum-verified name-search fallback successfully identified the original Dynamic Light file as project 1369480, release 8601053.

The imported raw archive SHA256 is retained in new pack metadata. Extraction uses the inspected byte snapshot, then releases its archive bytes. A source can be assigned only to the same imported UUID/version/raw-archive hash. Existing source settings, legacy entries lacking import provenance, and locally modified protections are preserved. Imported behavior/resource companions in the same archive get the same verified project reference. Source identification records the imported release ID but never claims an older imported release is the latest; regular update checks establish that separately.

Missing browser provenance, absent search candidates, old files not in the returned latest-file metadata, API restrictions/rate limits or a missing API key may still need a one-time manual project link. Protected RG Shader and Modern Furniture remain manual. Existing managed source mappings and automatic-update preferences are unchanged.

Primary API reference: https://docs.curseforge.com/rest-api/
