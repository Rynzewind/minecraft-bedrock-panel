# Publishing source, website and releases

The application source lives on `main`. The project website is a static site in `site/`, deployed to the `gh-pages` branch. GitHub Pages publishes the root of that branch. Engineering documentation stays on the source branch.

## Build and verify a Windows release

Run `scripts/build.ps1` from an interactive Windows desktop with Go 1.26 or newer. The command prints the dated release ZIP path. Create the installer with:

```powershell
.\scripts\release-single-file-setup.ps1 `
  -ReleaseZip 'outputs\release-YYYYMMDD-HHMMSS.zip' `
  -BuildDirectory 'work\iexpress-YYYYMMDD-HHMMSS' `
  -OutputExe 'outputs\installer-YYYYMMDD-HHMMSS\NetherNetSetup.exe' `
  -PublishDirectory 'outputs\published'
```

The script checks extraction, payload integrity, the native wizard and corrupt-payload rejection, then runs Microsoft Defender with protection enabled and writes `SHA256.txt`. Output paths must be new. The existing maintainer workflow may additionally publish to its configured private Drive folder; use the explicit local publish directory on other PCs.

Upload only the tested application installer, checksum and scan result to a GitHub Release. Never upload a live server directory, snapshot, account store or server identities.

## Website

Edit `site/`. All URLs and assets are static and work under the repository subpath. Deploy the contents to `gh-pages` without including the application source or private runtime data. The release download URL points to the latest GitHub Release.

The website is publicly accessible. Do not put credentials, live server addresses, player lists or private screenshots in it.
