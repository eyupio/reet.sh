package terminalproto

import "time"

// ACPBackgroundFeature lets a machine say that a chat's turn ended while work
// it started (a tool call the agent never reported finished) is still running:
// an idle status then carries the count, and so does a controls snapshot. An
// older controller's strict decoder refuses the field, so it is sent only to a
// controller that advertises the feature.
const ACPBackgroundFeature = "acp-background-v1"

// BackgroundHoldLimit is how long a turn's unfinished work may keep a chat
// from reading as finished. A task that never reports its end must not hold a
// session, or a pipeline waiting on it, for good.
const BackgroundHoldLimit = 30 * time.Minute

// ACPCodeUnwatched marks an idle status after which reet stopped waiting for
// work the agent never said had finished: it may still be running. Sent only
// with acp-background-v1, like the count.
const ACPCodeUnwatched = "unwatched"

// MaxACPBackground bounds the count a machine may report.
const MaxACPBackground = 1000

func validBackground(n int) bool { return n >= 0 && n <= MaxACPBackground }
