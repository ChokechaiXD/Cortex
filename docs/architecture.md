# HOPE Mem architecture

HOPE Mem is a memory-first system. The deep module is the memory kernel; every
UI, connector, importer, and agent integration crosses that interface instead
of owning storage directly.

```text
Hermes / future agents ── connector ──► HTTP v1 + token identity
                                      │
                                      ▼
                         HOPE Mem memory kernel (`cortex.db`)
                                      │
                                      ├── recall / feedback / review history
                                      ├── curator and optional model advice
                                      └── Skill Mem sidecar (`hope.db`)
```

## Product boundary

HOPE Mem owns:

- durable memories and revisions
- truth and utility scores
- review lifecycle and audit events
- token-budgeted recall
- deterministic curation
- optional manual model review
- Skill Mem metadata, routing, and usage feedback

HOPE Mem does not own:

- inference loops
- tool loops
- Telegram bot runtime
- model-provider lifecycle
- Hermes gateway lifecycle
- project launcher or folder management
- cron or automation scheduling

## Main modules

```text
cmd/cortex/                     CLI entrypoint kept for compatibility
internal/cortex/                memory kernel and public domain interface
internal/httpapi/               HTTP protocol and dashboard
internal/hope/                  Skill Mem tables, routing, and skill feedback
internal/hermes/                transactional Hermes connector sync
connectors/hermes/              Hermes memory provider adapter
internal/importer/holographic/  read-only importer from legacy memory stores
```

The old operating-hub modules may remain in the repository during migration, but
the default `serve` path no longer composes them.

## Stable interface

The memory kernel exposes five core operations:

1. `Remember`
2. `Recall`
3. `Feedback`
4. `Review`
5. `History`

Context packs add a thin Skill Mem layer:

1. Recall reviewed memory within a token budget
2. Route enabled skills deterministically by text, agent, project, and past
   outcome
3. Track whether the recommended skill was used, successful, or failed

## Data policy

- Existing database paths remain under `%LOCALAPPDATA%\Cortex` for compatibility
- `cortex.db` remains the source of truth for memory
- `hope.db` is now treated as Skill Mem storage, not a control-plane database
- Agent bearer tokens remain SHA-256 hashed at rest
- UI sessions are browser-only and cannot authenticate agent API calls

## Invariants

1. HOPE Mem must be usable without Hermes.
2. HOPE Mem must be usable without 9Router or any LLM.
3. Agent identity comes from the bearer token, not request JSON.
4. Candidate memory is not recalled cross-agent until reviewed.
5. Canonical memory is revisable when newer evidence contradicts it.
6. Skill recommendations are advisory context, not automatic execution.
7. Skill feedback is idempotent and scoped to the context pack owner.
