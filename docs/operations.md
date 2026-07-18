# HOPE Mem operations

HOPE Mem runs locally on `127.0.0.1:7777` by default and keeps existing data
under `%LOCALAPPDATA%\Cortex` for compatibility.

## Start

Double-click `Start HOPE Mem.bat` or the compatibility `Start HOPE.bat`.

Both call the installed `%LOCALAPPDATA%\Cortex\bin\cortex.exe open`, reuse the
configured loopback port, start the service only when needed, and open the
browser with a short-lived local dashboard session.

## Install

Double-click `Install HOPE Mem.bat` or the compatibility `Install HOPE.bat`.
The installer initializes config if missing, disables the optional dashboard PIN
for loopback-only use, syncs the Hermes connector when Hermes exists, installs
user-level autostart, starts the service, and opens the dashboard.

It does not start external model routers, Telegram, Hermes gateways, work modes,
projects, or cron jobs.

## Daily dashboard use

- review candidate memory
- search memory
- promote stable records to canonical
- reject, archive, or supersede bad records
- run deterministic curation
- optionally request manual model review
- configure agent memory budgets

## Agent integration

Agents use bearer tokens against:

- `POST /v1/memories`
- `POST /v1/recalls`
- `POST /v1/context-packs`
- `POST /v1/context-packs/{pack}/skills/{skill}/feedback`
- `POST /v1/memories/{id}/feedback`

Hermes profiles can be refreshed with:

```powershell
bin\cortex.exe connector sync hermes --home "$env:LOCALAPPDATA\hermes"
```

The sync command writes rollback backups before changing profile files.

## Rollback

1. Stop the service with `bin\cortex.exe service uninstall` if needed.
2. Restore `%LOCALAPPDATA%\Cortex\cortex.db` or `hope.db` from backup.
3. Restore Hermes profile backups if connector sync changed them.
4. Start HOPE Mem again.
