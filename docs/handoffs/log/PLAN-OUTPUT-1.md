## PLAN-OUTPUT-1: a factory plan and review get the same output ceiling as other Gaffer tool turns (27 November 2026)

Starting a factory item with Ollama Cloud `deepseek-v4.1-flash:1017` failed at "writing the plan" with "The provider
stopped before the plan was complete". That message is `Event.Cut`: the provider ended at its output ceiling. The plan
request (`executePlan`, `outcomes.go`) and the outcome review (`outcome_manage.go`) asked for 4,096 tokens; a thinking
model can spend all of that on reasoning the adapter never shows, so no tool call arrives and the answer reads as cut.

What changed: both requests use `hostToolMaxTokens` (16,384), the ceiling Gaffer's tool turns already use for the same
reason.

Not checked: against the live provider (no key here), so this is the likely cause from the code, not a reproduction. A
model that reasons past 16,384 tokens still fails with the same message. The decision call (`outcome_decide.go`, 1,024
tokens) has the same shape and was left alone.
