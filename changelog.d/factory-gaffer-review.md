- **Fixes for the Factory and Gaffer**: a plan with more pipelines than Gaffer has room to follow is refused before any
  agent starts, from the browser and over MCP, and a pipeline waits for room instead of starting an agent nobody
  follows. An account whose 100 Gaffer conversations are used up reuses the oldest finished one after a day, so new
  stages are still followed. A busy model provider no longer uses up Gaffer's automatic decisions and reports.
  Approving a hand-planned outcome again after a lost reply starts it once, and sending a chat message again inside a
  pipeline no longer shows a false conflict. Shared session text hides more credentials (`DB_PASSWORD=`, `NPM_TOKEN=`,
  bearer tokens, `postgres://user:pass@`), local models get two minutes to start answering, and a chat that is not
  allowed to share status no longer sends a pipeline's stages. "All tools" can no longer be set to Allow in one step.
  For MCP clients, `reet_list_factory` answers short rows under `items` that page (archived outcomes only with
  `include_archived`), `reet_get_factory_outcome` can read one pipeline with `pipeline` and clips handover notes, the
  Factory controls answer with the outcome in short, and `reet_move_pipeline_on` now requires `stage`.
