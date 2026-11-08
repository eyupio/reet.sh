# Cline billing source (design)

Date: 2026-11-08. Status: design approved in conversation by the product owner; this spec awaits their
review before an implementation plan is written.

A Cline chat in Gaffer can fail with "Insufficient balance" because Cline's signed-in provider is
**Usage-Billing** (Cline credits) while the person's plan is **ClinePass**. reet gives no way to change
that: it shows one model catalogue, the one for whichever provider Cline restored, and drops the
provider setting that Cline's ACP session reports. This design adds a **billing source** choice for
Cline (Usage-Billing, ClinePass, Free) to the live chat and to the Start agent dialog, and lets the
person save a default source.

## Intent and success

**Outcome.** A person with a ClinePass plan and no Cline credits starts or continues a Cline chat on
ClinePass in one choice, and new Cline chats start there without choosing again.

**Success looks like.**

- A failed "insufficient balance" Cline chat can be moved to another source from the chat itself, and the
  next message runs on it.
- Choosing **Cline** in Start an agent shows the source choice beside the model; the chosen source is the
  one the session runs on, confirmed by Cline, not assumed.
- A saved default source applies to new Cline sessions, set with **Make default** in the live chat, the
  Start dialog and Settings › Agents.
- A machine, connector or Cline that cannot switch source says so and leaves everything else working.

**Not in this pass.** Other agents (OpenCode and the rest have their own provider story). A per-project
default source. Automatic fallback to another source when credits run out. Reading Cline's balance or
plan. Storing the source on the session record.

## Mechanism

Cline's ACP `session/new` reports a `provider` option (category `_provider`) beside `model`, `mode` and
`auto_approve`. reet reads and sets it exactly as it does the model:

- **Read.** `readSettings` in `internal/acpagent/controls.go` currently admits only the `model` and
  `thought_level` categories. It also admits `_provider`, surfaced to reet as category `provider`, ids
  only, with at most `MaxACPThoughtLevelValues` values. A provider value must pass the existing
  model-name rule.
- **Set.** A new `SetProvider` sends `session/set_config_option {configId: "provider", value}` and treats
  the answer as confirmed only when the returned `configOptions` read back that value. Invalid
  parameters or an unsupported method are explicit refusals; any other error leaves the source
  *unconfirmed* and reports it, as `SetModel` does. It is refused while a turn is running.
- **Catalogue swap.** A source switch changes which models exist. The reply's `configOptions` carries the
  new model list; the session replaces its model setting from it. If the reply carries no model option,
  the session keeps its previous model values and forgets the current model, which the chat shows as
  Cline's own model, rather than guessing a list for the new source (the measured ACP surface has no call
  that reads the options again). A model that is no longer listed is cleared in the same way.
- **At launch.** Cline ignores a chosen model at `session/new`, so the model is already applied after the
  session opens. The order becomes provider, then model, then effort, each confirmed before the opening
  prompt is sent. A refused or unconfirmed provider refuses the launch the way an unconfirmed model does.
- **Reattach.** `session/load` restores whatever Cline stored. reet reads the provider back and shows it;
  it never re-applies one.

## Protocol

- New negotiated feature `acp-provider-v1`. A connector with it may send a `provider` setting in
  `ACPControls` (the option limit rises from two to three) and accept `Provider` on a create. Without it
  `forConnection` strips the setting, as it trims the model catalogue for an older controller, and a
  create carrying `Provider` is refused as unsupported.
- `ACPControls.Valid` accepts category `provider` (one per snapshot, 16 values at most). `ACPControl`
  `set` already carries any option id, so the live switch reuses it with the provider option's id.
- `Create` gains an optional `Provider`, valid only for Cline, empty meaning Cline's own choice.
- The controller adds `provider` to the create path and passes it through; it does not store it with the
  session.

## Defaults

- The account profile gains `defaultClineSource` (a provider id, empty meaning Cline's own choice). It is
  read by any signed-in account, written by writers only, validated by the model-name rule, and audited by
  tool and value only. It is part of the profile backup and restore. An unknown value is accepted and shown
  as given, because the id comes from Cline.
- **Precedence.** An explicit choice, then the saved default source, then Cline's own.
- **The default model follows the source.** The agent's default model (the starred favourite) prefills only
  while the chosen source equals the saved default source, or when no default source is saved, which is
  today's behaviour. Choosing another source resets the model to Cline's own for that source, so a
  `minimax/...` model is never sent to ClinePass. The Start dialog cannot list a Cline source's models before
  launch, so a mismatch that slips through is refused by Cline and reported like any refused model.
- A starred model can still be made the default for Cline as before; the two defaults are independent.

## Web

One shared source control (labels in `web/src/lib`, one place) used in three places:

- **Live chat toolbar**, beside **Chat model**: the sources the session reports, labelled
  Usage-Billing, ClinePass and Free where reet knows the id, and by Cline's own id otherwise. A star sets or
  clears the default source. It is disabled mid-turn, while reconnecting and while a change is in flight,
  with a reason, like the model picker. Changing source then shows the new model list in **Chat model**.
- **Start agent dialog**, when the agent is Cline and the machine has `acp-provider-v1`: the same control,
  preselected from the saved default, sent as `Provider`. Cline's refusal is shown in the dialog's usual
  refusal sentence and never disables Start for other agents.
- **Settings › Agents**: the saved default source for Cline.

The credit and balance error hint in the chat names the other sources when the session offers them:
"This source has no credit. Switch to ClinePass or Free above, or add credits with Cline, then send a message
to continue." Without a provider control the existing wording stands.

## Compatibility and failure

- An old connector, a Cline that reports no `provider` option, or an option with no values hides the
  control in the live chat. In the Start dialog it is hidden for a machine without the feature, with the
  reason shown where the model status line already explains such a state.
- A switch that Cline refuses or that cannot be confirmed leaves the chat on its previous source with a
  plain sentence, never a silently different source.
- A source switch may be stored by Cline for the whole machine rather than for one chat. The control's
  wording says "Source", not "for this chat only", until that is checked (see the gate below).

## Verification gate

This design was written without a signed-in Cline, so three facts are unverified. None changes the shape
above; each decides a detail, and the user-facing docs will say they are unchecked until a signed-in Cline
confirms them.

1. **`provider` is settable.** The earlier probe only proved `model` and `mode`. If Cline refuses
   `set_config_option` for `provider`, the control stays hidden everywhere and the work stops at the
   connector.
2. **How Free is exposed.** Cline's auth methods are `cline`, `cline-pass` and `openai-codex`. Either Free is
   its own provider id, which is added to the one label table; or Free is a filter over the `cline`
   provider's models, in which case it ships as a "Free models" view in the model list inside
   Usage-Billing, not as a third source.
3. **Scope of a switch.** Whether a switch is per chat or written to `~/.cline` for the whole machine,
   which sets the control's wording and whether the docs must warn that other Cline sessions follow.

A short probe script for these is part of the plan's first task. Task 1 of the connector work lands behind
the feature flag whatever it finds.

## Testing

- Go, with fake Cline sessions in the style of `internal/acpagent/cline_test.go`: a confirmed switch, a
  refused one, an unconfirmed one, a switch during a turn, a catalogue swap that clears a stale model, the
  launch order, and a reattach that does not re-apply a source.
- Protocol: `ACPControls` limits with three options, `forConnection` stripping for an old peer, `Create`
  with `Provider` for Cline and for another agent.
- Store and API: the profile field's validation, writer-only edit, audit row, and backup round trip.
- Web unit tests for the label table, default resolution and the default-model rule; Playwright for the
  Start dialog (`start-agent.e2e.mjs`) and the live chat (`chat-cline.e2e.mjs`), including the credit
  error path.
- No real provider or paid model is used, as in the existing model work.

## Docs

Update `docs/agent-models.md` (defaults and the models UI), `docs/start-agent-session.md` (the dialog's
Cline source row) and `docs/structured-sessions.md` (the live chat control), and extend
`docs/research/cline-acp-probe.md` with whatever the gate establishes.

## Delivery

Pull-request-sized steps, each shippable on its own: (1) connector and protocol behind
`acp-provider-v1`; (2) profile default, API and audit; (3) live chat control and error hint; (4) Start
dialog and Settings › Agents; (5) docs and the probe results.
