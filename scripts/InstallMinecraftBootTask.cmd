@echo off
fltmc >nul 2>&1
if errorlevel 1 (
  powershell.exe -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
  exit /b
)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "C:\MinecraftServers\Install-MinecraftBootTask.ps1"
if errorlevel 1 (
  echo.
  echo Installation failed. The error is shown above.
  pause
)
