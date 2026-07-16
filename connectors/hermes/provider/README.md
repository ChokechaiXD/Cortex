# Cortex connector for Hermes

This adapter translates Hermes memory lifecycle calls and tools to the Cortex
v1 HTTP protocol. Cortex remains a separate process and data owner.

Install and configure all profiles with:

```text
cortex connector sync hermes --home <HERMES_HOME>
```

The installer writes the provider under `$HERMES_HOME/plugins/cortex/` and a
profile-specific `$HERMES_HOME/cortex.json`. Raw conversation turns and
built-in `MEMORY.md` writes are not mirrored automatically.

