<script lang="ts">
  import { tick, untrack } from 'svelte'
  import { signalAttentionChanged } from '../attentionSignal'
  import { answerTerminalQuestion, chatToolName, type TerminalQuestion } from './api'
  import { optionText, outcomeOf, standingPermission } from './terminalQuestion'

  // The dialog a terminal agent is waiting on, answered from reet: the agent's own words and options, one button each, all
  // of equal weight (nothing invites a reflex tap). Everything in `question` was read off a terminal screen, so it is
  // drawn as text only. Shown on the session screen and inside the session's item in Home's Waiting for you.
  let { session, question, href = '', onreread }: {
    session: { id: string; tool?: string }
    question: TerminalQuestion
    /** Where the terminal itself opens, when the card is not already beside it (Home). */
    href?: string
    /** Read the sessions again now: after an answer, and when the question changed under the person. */
    onreread?: () => void | Promise<void>
  } = $props()

  const name = $derived(session.tool ? chatToolName(session.tool) : 'The agent')
  const lines = $derived(question.question.split('\n'))
  const heading = $derived(`${name} is asking`)
  const uid = $props.id()
  // A list read hands over a new object each time: only a different fingerprint is a different question.
  const fingerprint = $derived(question.fingerprint)

  let pressed = $state<number | null>(null)
  let say = $state('')
  let done = $state(false)
  let locked = $state(false)
  let keepSay = false
  let sending = $state(false) // a second tap before the first reply sends nothing

  // A different question starts clean; the sentence about a change stays over the new one, once.
  $effect(() => {
    void fingerprint
    untrack(() => {
      pressed = null; done = false; locked = false; sending = false
      if (keepSay) keepSay = false; else say = ''
    })
  })

  // Said once, politely, when this question first appears: the live region is empty at first and filled a moment later.
  let announced = $state('')
  $effect(() => {
    void fingerprint
    announced = ''
    void tick().then(() => { announced = untrack(() => `${heading}. ${lines.at(-1)}`) })
  })

  async function choose(index: number, label: string) {
    if (sending || locked) return
    sending = true
    pressed = index
    say = ''
    const outcome = outcomeOf(await answerTerminalQuestion(session.id, fingerprint, index), label)
    say = outcome.say; done = outcome.done; locked = outcome.locked; pressed = null; sending = false
    if (outcome.done) signalAttentionChanged()
    if (outcome.reread) keepSay = true
    if (outcome.done || outcome.reread) await onreread?.()
  }
</script>

<div class="card" role="group" aria-labelledby="{uid}-h" data-terminal-question>
  <h3 id="{uid}-h">{heading}</h3>
  <div class="said">
    {#each lines as line, i (i)}<p class="line" class:subject={i < lines.length - 1}>{line}</p>{/each}
  </div>
  {#if question.truncated}<p class="more">Part of this is only in the terminal.{#if href}{' '}<a href={href}>Open the terminal</a>{/if}</p>{/if}
  {#if !done}
    <div class="options">
      {#each question.options as option (option.index)}
        <button type="button" class="btn option" data-standing={standingPermission(option.label) || undefined} disabled={sending || locked}
          aria-busy={pressed === option.index ? 'true' : undefined} onclick={() => void choose(option.index, option.label)}>
          <span class="label">{optionText(option)}</span>{#if option.selected}<span class="default">Default</span>{/if}
        </button>
      {/each}
    </div>
  {/if}
  <p class="vh" role="status">{announced}</p>
  <p class="result" class:done class:bad={!done && !!say} role="status">{#if done}<span aria-hidden="true">✓ </span>{/if}{say}</p>
</div>

<style>
  .card { display: flex; flex-direction: column; gap: 8px; min-width: 0; box-sizing: border-box; padding: 12px 14px; border: 1px solid var(--line-strong, var(--line)); border-radius: 10px; background: var(--card-raised, var(--card)); color: var(--ink); }
  h3 { margin: 0; font: 600 14px/1.3 var(--sans); }
  .said { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .line { margin: 0; min-width: 0; font-size: 14px; line-height: 1.4; white-space: pre-wrap; overflow-wrap: anywhere; unicode-bidi: isolate; }
  .subject { font-family: var(--mono, monospace); }
  .more { margin: 0; font-size: 14px; color: var(--muted); }
  .options { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .option { justify-content: space-between; gap: 10px; width: 100%; min-width: 0; min-height: 40px; padding: 8px 12px; text-align: left; font: 500 14px/1.35 var(--sans); }
  .label { min-width: 0; overflow-wrap: anywhere; white-space: pre-wrap; unicode-bidi: isolate; }
  .default { flex: none; padding: 1px 8px; border: 1px solid var(--line-strong, var(--line)); border-radius: 999px; font-size: 14px; font-weight: 600; color: var(--muted); }
  .option[aria-busy='true']:disabled { opacity: 1; }
  .result { margin: 0; min-height: 0; font-size: 14px; color: var(--muted); overflow-wrap: anywhere; }
  .result:empty { display: none; }
  .result.bad { color: var(--bad); }
  .result.done { color: var(--ink); font-weight: 600; }
  @media (pointer: coarse), (max-width: 650px) { .option { min-height: 48px; } }
</style>
