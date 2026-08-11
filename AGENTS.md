# Agent notes

## CodeGraph (token-efficient code discovery)

<!-- codegraph:start -->
Prefer CodeGraph for structural discovery instead of crawling the repo with Grep/Glob/Read.

- MCP (Cursor): use `codegraph_explore` when the CodeGraph MCP server is connected (see `.cursor/mcp.json`).
- CLI:
  ```bash
  npx -y @colbymchenry/codegraph init      # once per clone
  npx -y @colbymchenry/codegraph explore "how does upload enqueue a job"
  npx -y @colbymchenry/codegraph sync     # if index looks stale
  ```
- Treat explore results as already-read source. Do not re-verify with broad file reads unless you are about to edit.
<!-- codegraph:end -->

Also follow:
- `docs/PROJECT_SPEC.md`
- `docs/CURSOR_INSTRUCTIONS.md`
- `.cursor/rules/`
