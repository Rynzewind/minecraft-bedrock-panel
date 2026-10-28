# Add-ons update overview

Approved in conversation: replace the settings-heavy Add-ons page with an installed-pack table (name, library version, Creative, Survival, update status), and move the existing controls into a cogwheel settings dialog. Unknown uses the existing Restart amber (mc.amber), Up to date green and Outdated red. Preserve selection and scroll where needed.

Checks must verify a matching UUID/manifest version from a permitted, hash-verified source archive before reporting green/red. Missing key/source, restricted downloads, failed checks or unknown deployed versions remain Unknown. Display the last check. API checks may inspect a bounded downloaded archive but never stage or activate it. Settings retain existing source/key/manual review and staging controls. No changes to worlds, enablement or server versions.

Implementation: store latest checked file/version evidence with source identity; invalidate evidence after source/channel changes or failed checks. Derive status from library/deployed versions. Build a themed table and owned, scrollable settings dialog. Run status/provider/service/native tests; review, build/test/scan/publish installer and keep installed copy aligned.
