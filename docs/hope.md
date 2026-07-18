# HOPE Agent Operating Hub

HOPE is the human-facing control plane. Cortex is the memory kernel. Hermes is
the agent runtime. 9Router is an optional model/provider router. Telegram stays
an existing conversation surface. None of these systems is copied into another.

```text
HOPE dashboard
├── Work Modes and system control
├── Agent Center
├── Project Center
├── Skill Center and Skill Router
├── Knowledge Center (Cortex)
├── Automations (Hermes cron)
└── Connections
    ├── Hermes runtime adapter
    ├── 9Router process adapter
    └── Telegram link adapter
```

## Data ownership

- `cortex.db` owns memories, immutable revisions, recalls, feedback, scores,
  review state, and memory audit events.
- `hope.db` owns agents, work modes, project references, skill metadata,
  recommendation outcomes, managed-process ownership, and control actions.
- User-created skill files live under `%LOCALAPPDATA%\Cortex\hope\skills`.
- Hermes receives deployed copies under its shared skill directory. Removing a
  deployed copy never removes the HOPE source.
- Project Center stores paths and metadata, not repository contents.
- Telegram tokens and model-provider secrets are not stored in `hope.db`.

Both databases use SQLite WAL, foreign keys, a busy timeout, one writer
connection, and ordered `PRAGMA user_version` migrations.

## Process ownership

HOPE probes before starting a service. A healthy external process is reused and
shown as external. HOPE records PID and process start time only for a process it
starts. Stop is allowed only while both still match. A stale ledger entry, a
reused port, or an unverifiable PID causes a safe refusal instead of a broad
process kill.

Hermes gateways remain separate per profile. Work Modes call the official
Hermes CLI and never mutate the Hermes agent loop. 9Router remains a standalone
application and continues to own its provider configuration and UI.

## Context Pack and self-learning

Hermes asks one authenticated endpoint for a Context Pack:

1. Cortex retrieves reviewed memory within the requested token budget.
2. HOPE scores skill metadata with deterministic text, agent, project, and
   historical-result signals.
3. Only when the leading rule scores are close may the configured loopback
   model advisor choose among the already-approved candidate IDs.
4. The provider loads only the selected skill through Hermes `skill_view`.
5. `used`, `success`, or `failure` feedback is stored idempotently and improves
   later deterministic routing.

The model never creates a skill ID, weakens access rules, approves memory, or
becomes required. Full skill bodies are not placed in every prompt.

## Adding an agent

Agent Center accepts an ID, display name, role, Hermes profile, and optional
Telegram URL. It can create the Hermes profile and run the existing transactional
Cortex connector sync. A new agent can then be selected in any Work Mode without
adding Go code.

## Adding an integration

An integration implements the small `integrationhub.Adapter` interface:
`ID`, `Probe`, and `Execute`. Keep installation, health detection, start/stop,
and ownership details inside that adapter. The dashboard and Work Mode manager
depend on the interface, not on 9Router or Hermes internals.

## Deliberate omissions

HOPE does not implement an inference loop, tool loop, Telegram bot runtime,
provider router, distributed scheduler, vector database, or general process
manager. It links and observes those systems through narrow adapters. Add a new
dependency only when measured use shows that the standard library and existing
modules are insufficient.
