# CodeGraph setup (AI token optimization)

DocForge uses [CodeGraph](https://github.com/colbymchenry/codegraph) so AI agents (Cursor and others) can query a local code knowledge graph instead of burning tokens on file crawls.

## Why

Agents without a graph typically Grep/Read many files to reconstruct call flows. CodeGraph returns relevant symbols, call paths, and blast radius in one query — fewer tool calls and fewer tokens for discovery.

## Project wiring (already committed)

- `.cursor/mcp.json` — Cursor MCP server entry (`npx @colbymchenry/codegraph serve --mcp`)
- `.cursor/rules/docforge-codegraph.mdc` — always-on rule: prefer `codegraph_explore`
- `AGENTS.md` — CLI fallback guidance for subagents / non-MCP harnesses

## One-time per clone

```bash
npx -y @colbymchenry/codegraph init
```

This creates `.codegraph/codegraph.db` (gitignored). Auto-sync keeps it fresh while you edit.

Optional PATH install:

```bash
npm i -g @colbymchenry/codegraph
# or: curl -fsSL https://raw.githubusercontent.com/colbymchenry/codegraph/main/install.sh | sh
```

Then restart Cursor so MCP reloads.

## Useful commands

```bash
npx -y @colbymchenry/codegraph status
npx -y @colbymchenry/codegraph explore "how does PDF upload create a job"
npx -y @colbymchenry/codegraph sync
npx -y @colbymchenry/codegraph telemetry off
```

## Privacy

Project MCP config sets `CODEGRAPH_TELEMETRY=0`. You can also run `codegraph telemetry off`.
