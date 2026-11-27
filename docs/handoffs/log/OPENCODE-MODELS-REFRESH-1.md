## OPENCODE-MODELS-REFRESH-1: OpenCode's model listing may refresh its catalogue (27 November 2026)

A model that a provider had added (for example `inkling-2-small` on Ollama Cloud) was missing from the crew, Start an
agent and New outcome pickers. Those pickers show what `opencode models` prints on the machine, and the connector ran
it with `OPENCODE_DISABLE_MODELS_FETCH=1`, so only OpenCode's bundled catalogue was read.

What changed.

- `internal/terminalhost/catalogue.go`: the opt-out is gone from OpenCode's listing environment. The listing now
  makes OpenCode's own catalogue request (one outbound call from the machine, as the probe report recorded).
  `OPENCODE_DISABLE_AUTOUPDATE` and the other opt-outs stay. `catalogue_test.go` no longer expects the removed entry.

Not done. A Gaffer provider's own list (Ollama Cloud's `/v1/models`) does not feed these pickers: a stage runs an agent
CLI, not the provider, so a model only that provider serves would fail at launch. A model shows only if OpenCode on the
machine has the provider set up. Not checked against a real OpenCode here.
