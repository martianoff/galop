# galop — Claude Code instructions

@AGENTS.md

---

Keep project rules in `AGENTS.md`; this file only adds what is specific to Claude Code.

- The project enables the GALA Claude Code plugin (`.claude/settings.json`). Use its
  `gala-lint` skill on changed `.gala` files and its LSP for inferred types.
- Before writing GALA, use the `gala-code` skill if it is available.
