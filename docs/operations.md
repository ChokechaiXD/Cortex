# HOPE operations

## Installed layout

```text
%LOCALAPPDATA%\Cortex\
├── bin\cortex.exe
├── config.json
├── cortex.db
├── hope.db
├── hope\skills\
├── launcher.key
├── logs\9router.log
└── backups\
```

Cortex accepts loopback listeners only; the default is `127.0.0.1:7777`.
Windows autostart is registered in
the current user's `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` key;
administrator permission is not required.

## Daily use

Open **HOPE Dashboard** from the Windows Start Menu. The shortcut checks
`/v1/health`, starts the installed service only when needed, and opens the
configured loopback URL with a 30-second, one-time local session. It does not
place an agent bearer token in the browser or command output. Use the dashboard
for memory search/review, grouped Candidate Inbox review, per-agent project or
domain routing and token budgets, Hermes-agent discovery, restart, and graceful
stop. Selected-memory bulk decisions are atomic: an invalid item rolls back the
whole selection.

Alternatively, double-click `Start HOPE.bat` from the repository. It uses
the same installed executable and health-aware `open` command as the Start Menu
shortcut.

For a first local installation, double-click `Install HOPE.bat`. It initializes
only missing Cortex data, disables the optional browser PIN, syncs Hermes when
present, installs user-level autostart, starts HOPE, and opens the browser. It
does not start 9Router or Hermes gateways automatically.

The **Today** page owns daily startup. A Work Mode starts only its selected
integrations and agent profiles. Existing healthy services are reused. The stop
button acts only on processes opened by HOPE whose PID and creation time still
match the ownership ledger.

The commands below remain operator diagnostics, not daily requirements:

```powershell
Invoke-RestMethod http://127.0.0.1:7777/v1/health
bin\cortex.exe service status
hermes memory status
```

`service status` reports autostart registration. The health endpoint above is
the authoritative runtime readiness check.

Open `http://127.0.0.1:7777/` directly only when diagnosing the shortcut.
Generate a new administrator login token when needed:

```powershell
bin\cortex.exe agent token --id mika
```

Only the printed token is sensitive. Cortex persists its SHA-256 hash.

Direct loopback access is passwordless by default. To enable a 4–8 digit
dashboard-only PIN, use Settings or the fallback command below. It is stored as
a hash and is deliberately rejected by every agent API endpoint:

```powershell
bin\cortex.exe dashboard pin --value 4826
```

Disable it again with `bin\cortex.exe dashboard pin --off`. Either setting
affects the browser only; agent API tokens remain mandatory and hashed at rest.

## Agent, project, skill, and automation operations

- **Agent Center** starts or stops individual Hermes profiles, opens configured
  Telegram URLs, and can create a profile before connector sync.
- **Project Center** scans only configured roots to depth three, skips dependency
  and build folders, and stores references instead of repository contents.
- **Skill Center** scans metadata on demand. HOPE skills are editable sources;
  Hermes skills are read-only. Deployment writes an atomic copy plus a HOPE
  manifest into the Hermes shared skill directory.
- **Automations** reads `hermes cron list` and exposes explicit run, pause, and
  resume actions. Hermes remains the scheduler.
- **Connections** shows external, HOPE-managed, stopped, missing, degraded, and
  port-conflict states without copying another product's settings UI.

## Add or refresh Hermes profiles

Press **Discover & connect agents** in the dashboard after creating a profile.
It uses the same transactional connector sync as the operator command:

```powershell
bin\cortex.exe connector sync hermes `
  --home "$env:LOCALAPPDATA\hermes"
```

Existing valid profile tokens are reused. New profiles receive a distinct
credential and the running Cortex process reloads it without restart. A newly
granted governor role requires restart because governance membership is loaded
when the hub opens.

The command prints `backup=...` before its profile list. Cortex creates that
timestamped snapshot before changing credentials or any Hermes profile. It
contains prior profile configs, connector files, and legacy Holographic
database files. If any profile fails, Cortex restores every changed profile and
its own credential config before returning an error.

The dashboard's **Agent learning** section changes only Cortex connector
settings. It preserves unknown Hermes configuration fields, never displays
profile bearer tokens, and backs up the profile before each write. Changes are
picked up when that agent starts its next Hermes session.

## Holographic migration

The importer opens Holographic SQLite in read-only/query-only mode. Imported
records stay candidates until reviewed.

```powershell
bin\cortex.exe import holographic `
  --database "$env:LOCALAPPDATA\hermes\memory_store.db" `
  --agent mika
```

Repeating an unchanged import is safe and reports the records as replayed.

## Update the installed binary

Stop the currently running `cortex.exe` after verifying its path points inside
`%LOCALAPPDATA%\Cortex\bin`, then run:

```powershell
bin\cortex.exe service install
bin\cortex.exe service start
```

The registry entry is replaced atomically and continues to launch Cortex in a
hidden detached process at the next sign-in.

## Rollback to Holographic

Every connector sync creates a timestamped pre-migration snapshot under
`%LOCALAPPDATA%\Cortex\backups`. Keep the `backup=...` path printed by the
sync command.

1. Remove Cortex autostart with `bin\cortex.exe service uninstall`.
2. Stop the verified Cortex process.
3. Restore each profile's matching `hermes\<agent>\config.yaml` from the
   timestamped backup to its Hermes home.
4. Start a new Hermes session and run `hermes memory status`.

The original `memory_store.db`, WAL, and SHM files are never modified by the
importer, so rollback does not require converting Cortex data back to
Holographic.
