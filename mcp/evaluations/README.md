# Frozen shelf evaluations

`shelf.xml` contains ten independent, read-only tasks. Answers refer to the
disposable fixture in `test/fixture`, not to any user's current shelf. Skill
names, tags, flags, inventories and list contents are fixed; scan timestamps and
database IDs are intentionally not answers. No task requires a write tool.

From the repository root, start a fresh fixture:

```sh
go run ./mcp/test/fixture
```

It prints its localhost URL. Keep that process running and configure the MCP
client with `REET_URL` set to that URL and `REET_API_TOKEN=fixture-token`.
The fixture's token is a public test value, not a real credential. The process
uses a fresh temporary SQLite directory, never the instance's `data/`, and
removes it on graceful shutdown. External metadata fetching is disabled.

Build the MCP server (`cd mcp && npm ci && npm run build`), then connect an MCP
client to `node /absolute/path/to/mcp/dist/index.js`. Inspect `tools/list` and
answer each XML question using read tools only. Use small limits and follow
`next_offset`. Compare the final answer exactly with its `<answer>` value.
Do not use mutation tools on this fixture during evaluation. Restart the fixture
to reset it if needed.

## Verification record

An independent evaluator inspected the MCP tool definitions and answered all ten
tasks with **59 read-only MCP calls**, paging with `limit=2`. It did not inspect
the server or fixture implementation and made no write tool calls.

| Task | Evidence used | Calls |
| --- | --- | ---: |
| 1 | Interface kit, laptop inventory, to-test shelf sorted by installs | 7 |
| 2 | Both lists and both complete inventories | 11 |
| 3 | Desktop plan's missing IDs intersected with Security-tagged shelf | 4 |
| 4 | Missing skills' shelf install counts, 300 and 600 | 3 |
| 5 | Backend tag, liked/one-machine filter and laptop inventory | 6 |
| 6 | Interface kit, incomplete environment, shelf flags and personal notes | 9 |
| 7 | Release kit, to-test shelf and recorded installations | 4 |
| 8 | Complete environment inventories and common skill's liked flag | 6 |
| 9 | Desktop plan's extra ID located in accessible lists | 6 |
| 10 | Sum of shelf install counts for desktop missing IDs | 3 |

This verifies task solvability and expected answers. It is not a model accuracy
benchmark: no automated LLM evaluation harness or paid model calls were run.
The separate `npm test` suite verifies protocol and API behavior without an LLM.
