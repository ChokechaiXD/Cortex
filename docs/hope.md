# HOPE Mem

HOPE Mem is the product name for the shared-memory system. Cortex remains in
some paths and package names for compatibility, but the product is no longer an
agent operating platform.

## Current scope

- shared memory for all agents
- reviewable lifecycle: candidate, active, canonical, rejected, superseded,
  archived
- traceable truth and utility scores
- deterministic curator
- optional model review through a loopback OpenAI-compatible endpoint
- Skill Mem so agents can receive relevant skill recommendations in context
  packs
- Hermes connector sync for existing agents

## Removed from product scope

- work modes
- service launcher
- 9Router process control
- Hermes gateway start/stop controls
- Telegram UI aggregation
- project/folder launcher
- Hermes cron control

These features are not part of the memory product. They can be rebuilt later as
separate adapters if they earn their maintenance cost.

## Skill Mem

Skill Mem keeps small skill metadata records and usage feedback. It is designed
for agent context, not for replacing Hermes skills.

An agent calls `/v1/context-packs` with a task. HOPE Mem returns:

1. relevant reviewed memory
2. a small list of matching skills
3. route evidence explaining whether the match was deterministic or model
   assisted

The agent remains responsible for loading or invoking the actual skill through
its runtime.
