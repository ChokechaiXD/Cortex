# Cortex v0.1 Specification

## Product boundary

- Cortex is standalone and stores all user-owned data locally.
- The core does not depend on Hermes or any other agent framework.
- Agent integrations are replaceable connectors over the public HTTP API.
- SQLite is the source of truth. All mutations are auditable and idempotent.
- No cloud service, LLM, embedding model, or background broker is required.

## Memory lifecycle

New agent-written memories start as `candidate`. Candidates do not enter normal
cross-agent recall. A governor can approve them to `active`, promote stable
records to `canonical`, or reject, supersede, and archive them without deleting
history.

## Initial scopes and kinds

Scopes: `global`, `project`, `domain`, `private`.

Kinds: `fact`, `preference`, `decision`, `failed_attempt`, `solution`,
`project_state`.

Private memory is readable only by its owner and an administrator. Shared
memory can be recalled by another agent only after review, unless a caller
explicitly asks to inspect candidates.

## Learning model

Truth and utility are separate scores. Confirmations and contradictions update
truth. Helpful and unhelpful outcomes update utility. Failed attempts remain
valuable warnings rather than being treated as low-quality memories.

Every create, review, recall, and feedback action produces an append-only event.
Existing memory content is revised by adding a revision, never by overwriting
its prior representation.

## Public deep-module interface

The application core exposes five operations:

1. `Remember`
2. `Recall`
3. `Feedback`
4. `Review`
5. `History`

The HTTP API and connectors translate to this interface rather than reaching
into storage directly.

