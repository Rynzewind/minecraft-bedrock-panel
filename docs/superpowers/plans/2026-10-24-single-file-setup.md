# Single-file setup packaging

Bundle the verified release ZIP inside a Windows IExpress self-extracting EXE with a small PowerShell launcher. The launcher verifies the embedded ZIP SHA256, extracts to its own unique temporary directory, runs the existing setup wizard synchronously, and cleans up only that temporary directory. Preserve the original installer, uninstall utility, hashes and sources. Server data/addons remain in a separate snapshot ZIP selected in the wizard, as requested.

Validation: test launcher checksum rejection and normal smoke setup without machine changes; build EXE, extract it without installation and compare payload bytes; run extracted launcher smoke test; Defender-scan final EXE and record SHA256. No installed application or server changes are required.
