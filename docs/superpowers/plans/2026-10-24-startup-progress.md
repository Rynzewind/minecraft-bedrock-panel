# Startup progress and animation repair

Show the existing modal progress dialog on ordinary panel launch; retain silent --startup logon behaviour. Animate the custom Minecraft progress bar with its own window timer, stopping that timer when marquee mode ends or the control is destroyed. Show elapsed time and live Minecraft output during fleet operations.

1. Add native regression coverage for timer advancement and stopping.
2. Update theme timer lifecycle, startup visibility and fleet output polling.
3. Run panel tests and vet; build, checksum and scan updated installer.
4. Back up and update installed panel without restarting Minecraft servers.
