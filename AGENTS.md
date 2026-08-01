# Project Agent Instructions

## Code search

This repository is indexed by CodeGraph (`.codegraph/` exists).

- For semantic or structural code search, locating symbols, understanding implementations, tracing call paths and dependencies, or assessing change impact, use `codegraph_explore` first.
- If the CodeGraph MCP tool is unavailable in the current session, use `codegraph explore "<query>"` from the repository root.
- Treat line-numbered source returned by CodeGraph as already read. When more context is needed, issue a narrower follow-up CodeGraph query.
- Use `rg` first for exact text searches, file-name searches, configuration, documentation, generated files, and content CodeGraph does not index.
- Fall back to `rg` and direct file reads when CodeGraph is unavailable or does not provide the required detail.

