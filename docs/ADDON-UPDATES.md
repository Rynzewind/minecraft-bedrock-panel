# Add-on updates

Open **Updates > Add-ons**. Select a library pack to see its deployed Creative/Survival versions. Changes are staged for explicitly selected servers; staging never stops servers or changes their running pack folders.

## Manual source review

1. Save the author's HTTPS project page and optional review notes.
2. Open the source and download its Bedrock `.mcaddon`, `.mcpack` or `.zip` archive.
3. Select Creative and/or Survival, then **Review local archive**.
4. Review the pack UUIDs, exact manifest versions, targets and archive SHA-256. Accept to stage.
5. Stop the chosen server and use **Content Packs > Apply**. Its normal pack backup/rollback remains in effect.

Provider file IDs are acknowledged only after successful staging; previously staged files are labeled during later checks. A manual check records a timestamp and notes. It does not prove that an installed version is current. Archives must contain a newer selected UUID, compatible engine requirements and a connected dependency bundle. Unrelated additions and downgrades are rejected. Apply/cancel existing pending pack changes before reviewing a new update. The selected pack must already be enabled on each chosen target; enable disabled packs separately. Required dependency enablement is listed in the review. Legacy deployments with unknown version provenance require explicit pack/version selection before an unrelated Apply.

## CurseForge API access

After receiving a developer API key, enter it in the masked field, choose **Save / replace key**, then **Test saved key**. The saved key is never displayed. It is encrypted using Windows DPAPI for the current user and held in a protected local `MultiServer/provider-credentials` directory. Do not put a key in source URLs, source control, an installer or a portable snapshot. Same-PC application updates preserve the local credential directory; a new PC/profile needs key entry again.

Link a Minecraft Bedrock CurseForge project URL or its numeric project ID. Stable releases are the default; beta/alpha channels require explicit selection. **Check selected source** lists available candidates; **Check all linked sources** checks each linked CurseForge source on demand. There is no background checking or automatic activation. Up to the latest 100 eligible files are listed. File dates/names are candidates, not proof of a newer installed manifest.

Choose a release and **Download selected release / review**. Downloads must have matching API length/checksum metadata and pass the same archive review as local files. API authentication goes only to the official API; a separate keyless client downloads from approved CurseForge CDN hosts. Author distribution restrictions are respected. If download access is unavailable, use **Open source** and the manual flow.

Reviews expire after 30 minutes, are single-use, and reject changed archive bytes, changed pack state or a changed engine version. Restarting the helper invalidates open reviews. Existing enabled versions are retained separately per server, so importing a newer library version does not implicitly upgrade the other server during an unrelated Apply.

API behavior is covered by simulated HTTP responses, including pagination, channels, restrictions, invalid metadata, authentication failures, checksums and key isolation. Live CurseForge access still needs confirmation with an approved developer key.

