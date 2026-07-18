# HOPE and Cortex architecture

## Boundary

Cortex is the memory owner. HOPE is a separate control plane composed beside it.
Hermes, future agents, dashboards, and importers are clients of stable
interfaces. The memory core never imports an agent framework or control adapter.

```text
Hermes / future agents ── connector ──► HTTP v1 + token identity
                                            │
                                            ▼
                              Cortex kernel (`cortex.db`)

Human ── browser ──► HOPE dashboard ──► control modules (`hope.db`)
                         │
                         ├── Hermes CLI adapter
                         ├── 9Router adapter
                         └── Telegram link adapter
```

## Module map

```text
cmd/cortex/                      CLI composition and process lifecycle
connectors/hermes/               embedded, replaceable Hermes adapter
internal/autostart/              Windows user-level startup adapter
internal/config/                 local config and hashed agent credentials
internal/controlcenter/          serialized local runtime and connector controls
internal/controlplane/           HOPE application facade and UI-facing use cases
internal/cortex/                 memory domain and deep-module interface
internal/hope/                   HOPE schema, catalogs, routing evidence, process ledger
internal/hermes/                 connector discovery, install, activation
internal/hermesruntime/          narrow official Hermes CLI client
internal/httpapi/                HTTP translation and management dashboard
internal/integrationhub/         adapter registry, per-target locks, action audit
internal/integrations/           Hermes, 9Router, and Telegram adapters
internal/intelligence/           optional loopback model-advisor adapter
internal/importer/holographic/   read-only legacy adapter
internal/launcher/               validated Windows dashboard opener
internal/localauth/              HMAC launcher proof and one-time UI codes
internal/projectcenter/           bounded project discovery and Windows folder opening
internal/skillcenter/             HOPE skill files, Hermes discovery, atomic deployment
internal/workmodes/               reversible multi-system start/stop orchestration
internal/automations/             Hermes cron visibility and explicit controls
docs/                            product and architecture contracts
```

Each operation has its own focused implementation file. `service.go` only
composes the hub; it does not accumulate endpoint, SQL, and connector logic.
CLI commands are split by identity, integration, process, and autostart seams;
`main.go` contains dispatch and usage only.

## Storage model

### Cortex memory kernel

- `memories` holds identity, scope, lifecycle, current revision, and scores.
- `memory_revisions` holds immutable content revisions.
- `memory_events` is the append-only usage and governance ledger.
- `memory_fts` indexes only current content for keyword recall.
- `recalls` and `recall_items` make tracked recalls durable and idempotent.
- `requests` maps idempotency keys to resources.
- `PRAGMA user_version` drives ordered schema migrations.

### HOPE control plane

- `agents` and `work_modes` hold no-code runtime composition.
- `projects` and `project_roots` hold references only.
- `skills` and `skill_fts` index skill metadata, not full prompt bodies.
- `context_packs`, `context_pack_skills`, and `skill_feedback_events` make
  recommendations and their outcomes inspectable and idempotent.
- `managed_processes` proves which external processes HOPE may stop.
- `action_events` is the human-facing control ledger.

The database is opened with foreign keys, WAL, one writer connection, and a
busy timeout. A binary refuses to open a database whose schema version is newer
than it understands.

## Invariants

1. New and imported memories are candidates.
2. Candidates do not enter ordinary cross-agent recall.
3. Only configured governors can change lifecycle state.
4. Private memory is visible only to its owner and governors.
5. Caller-supplied `agent_id` never overrides authenticated identity.
6. Truth evidence and utility evidence update different scores.
7. Failed attempts remain useful warnings; failure content is not low utility
   by definition.
8. Mutating operations are idempotent.
9. Stable memory keys append revisions; prior content is never overwritten.
10. Project and domain memory require an exact recall scope.
11. Browser sessions cannot authenticate HTTP API requests.
12. Legacy databases are imported read-only and remain untouched.
13. Recall token budgets are enforced before recall items and usage events are
    persisted; omitted context cannot influence learning scores.
14. The local launcher never transmits its long-lived key; signed proofs and UI
    codes expire after 30 seconds and cannot be replayed.
15. A batch review validates every selected transition before one transaction
    mutates any memory; one invalid transition leaves the entire batch unchanged.
16. HOPE and Cortex use different databases and migrations.
17. HOPE never stops an external process without matching ownership evidence.
18. A Context Pack records only skill metadata and outcome; skill bodies remain
    lazy-loaded by Hermes.
19. The optional AI tie-breaker may reorder supplied IDs only. Deterministic
    routing remains the fallback and source of candidates.
20. Passwordless dashboard access is accepted only from a loopback remote
    address; agent APIs always require bearer-token identity.

## Evolution points

Add capabilities behind the existing operations before adding new public
surface area:

- Semantic search can become an optional recall scorer beside FTS5.
- Session extraction can become an optional connector/extractor adapter.
- Project membership and richer ACLs can extend visibility checks.
- Procedures that repeatedly succeed can produce reviewable skill proposals.
- Other frameworks install separate connectors against HTTP v1.

## Curator governance

Curator is a deep module inside `internal/cortex`: callers ask for a preview,
update a small policy record, or run one guarded curation pass. Evidence
counting, protected memory classes, risk detection, lifecycle actions, and run
auditing stay behind that interface. The dashboard translates the result into
human language and does not reproduce policy logic.

Automatic mode is intentionally narrow. It can approve a sourced project or
domain candidate after distinct-agent agreement, but it cannot create a
canonical rule or mutate global, private, preference, project-state, or
imported memory. Active and canonical records whose truth or utility decays are
flagged for a governor instead of silently remaining golden forever.

The optional model seam begins after deterministic analysis. A model can
summarize or challenge a suggestion, but cannot weaken the code-level rules or
become required for core operation. The adapter accepts only loopback HTTP
endpoints, caps external responses, enforces input/output budgets, and validates
model output against the IDs it sent. See [Curator](curator.md) and
[Model advisor](model-advisor.md).

The implementation intentionally omits automatic raw-turn mirroring, automatic
skill mutation, vector databases, and distributed coordination. Add them only
when measured usage shows the simpler path is insufficient.
