- **Gaffer works with more providers and counts their tokens fully**: OpenAI's newer models, which refused the old
  output-limit setting, now work; on an Anthropic provider the answer after a tool or a worker instruction no longer
  risks being refused, and token counts include cached input. A provider that cannot be reached is no longer reported
  as taking more than three minutes. A turn that offers reet's tools now asks for up to 16,384 output tokens, so a
  large outcome can be started from chat; a model that allows less output may refuse such a turn.
