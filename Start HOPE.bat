@echo off
setlocal

set "HOPE_EXE=%LOCALAPPDATA%\Cortex\bin\cortex.exe"
if not exist "%HOPE_EXE%" (
  title HOPE
  echo HOPE is not installed yet.
  echo Run the installer once, then try again.
  pause
  exit /b 1
)

powershell.exe -NoProfile -NonInteractive -WindowStyle Hidden -Command ^
  "$exe = Join-Path $env:LOCALAPPDATA 'Cortex\bin\cortex.exe'; Start-Process -WindowStyle Hidden -FilePath $exe -ArgumentList 'open'"

if errorlevel 1 (
  title HOPE
  echo HOPE could not be opened.
  pause
  exit /b 1
)

exit /b 0
