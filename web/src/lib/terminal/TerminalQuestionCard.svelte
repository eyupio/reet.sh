<script lang="ts">
  import { tick, untrack } from 'svelte'
  import { signalAttentionChanged } from '../attentionSignal'
  import { answerTerminalQuestion, chatToolName, type TerminalQuestion } from './api'
  import { optionText, outcomeOf, standingPermission, type AnswerOutcome } from './terminalQuestion'

  // The dialog a terminal agent is waiting on, answered from reet: the agent's own words and options, one button each, all
  // of equal weight (nothing invites a reflex tap). Everything in `question` was read off a terminal screen, so it is
  // drawn as text only. Shown on the session screen and inside the session's item in Home's Waiting for you. Both holders
  // already show the session's title and machine, so the visible heading stays short; the group's name and its one polite
  // announcement carry them, to tell several waiting sessions apart.
  let { session, question, title = '', machine = '', href = '', onopen, onreread }: {
    session: { id: string; tool?: string }
    question: TerminalQuestion
    title?: string
    machine?: string
    /** Where the terminal opens, when the card is not already beside it (Home). */
    href?: string
    /** Takes the person to the terminal on this page (the session screen). */
    onopen?: () => void
    /** Read the sessions again now: after an answer, and when the question changed under the person. */
    onreread?: () => void | Promise<void>
  } = $props()

  const name = $derived(session.tool ? chatToolName(session.tool) : 'The agent')
  const lines = $derived(question.question.split('\n'))
  const where = $derived(`${title ? ` in ${title}` : ''}${machine ? ` on ${machine}` : ''}`)
  // A list read hands over a new object each time: only a different fingerprint is a different question.
  const fingerprint = $derived(question.fingerprint)

  let pressed = $state<number | null>(null)
  let outcome = $state<AnswerOutcome | null>(null)
  let sending = $state(false) // a second tap before the first reply sends nothing
  let result = $state<HTMLElement>()
  let keepOutcome = false

  const done = $derived(!!outcome?.done)
  const locked = $derived(!!outcome?.locked)
  const heading = $derived(`${name} ${done ? 'was asked' : 'is asking'}`)

  // A different question starts clean; the sentence about a change stays over the new one, once.
  $effect(() => {
    void fingerprint
    untrack(() => {
      pressed = null; sending = false
      if (keepOutcome) keepOutcome = false; else outcome = null
    })
  })

  // Said once, politely, when this question first appears: the live region is empty at first and filled a moment later.
  let announced = $state('')
  $effect(() => {
    void fingerprint
    announced = ''
    void tick().then(() => { announced = untrack(() => `${name} is asking${where}. ${lines.at(-1)}`) })
  })

  // The buttons go (disabled, or gone): focus waits on the result line, never on the page.
  async function look() { await tick(); result?.focus() }

  async function choose(index: number, label: string) {
    if (sending || locked) return
    sending = true
    pressed = index
    outcome = { say: 'Sending your answer…', tone: 'note', done: false, locked: false, reread: false }
    void look()
    const next = outcomeOf(await answerTerminalQuestion(session.id, fingerprint, index), label)
    outcome = next; pressed = null; sending = false
    void look()
    if (next.done) signalAttentionChanged()
    if (next.reread) keepOutcome = true
    if (next.done || next.reread) await onreread?.()
  }
</script>

<div class="card" role="group" aria-label="{name} {done ? 'was asked' : 'is asking'}{where}" data-terminal-question>
  <h3>{heading}</h3>
  <div class="said">
    {#if lines.length > 1}<div class="subjects">{#each lines.slice(0, -1) as line, i (i)}<p class="line subject">{line}</p>{/each}</div>{/if}
    <p class="line">{lines.at(-1)}</p>
  </div>
  {#if question.truncated && !done}
    <p class="more"><span class="mark" aria-hidden="true">!</span><span>Part of this is only in the terminal. Read it there before answering.{#if href}{' '}<a href={href}>Open the terminal</a>{:else if onopen}{' '}<button type="button" class="go" onclick={onopen}>Go to the terminal</button>{/if}</span></p>
  {/if}
  {#if !done}
    <div class="options">
      {#each question.options as option (`${fingerprint}:${option.index}`)}
        <button type="button" class="btn option" disabled={sending || locked}
          aria-busy={pressed === option.index ? 'true' : undefined} onclick={() => void choose(option.index, option.label)}>
          <span class="label">{optionText(option)}{#if option.selected}<span class="default">{' · terminal’s default'}</span>{/if}</span>
          {#if standingPermission(option.label)}<span class="standing">Keeps applying after this answer.</span>{/if}
        </button>
      {/each}
    </div>
  {/if}
  <p class="vh" role="status">{announced}</p>
  <p class="result {outcome?.tone ?? ''}" role="status" tabindex="-1" bind:this={result}>{#if outcome}{#if outcome.tone === 'done'}<span aria-hidden="true">✓ </span>{:else if outcome.tone === 'error'}<span class="mark" aria-hidden="true">!</span>{/if}<span>{outcome.say}</span>{/if}</p>
</div>

<style>
  .card { display: flex; flex-direction: column; gap: 8px; min-width: 0; box-sizing: border-box; padding: 12px 14px; border: 1px solid var(--line-strong, var(--line)); border-radius: 10px; background: var(--card-raised, var(--card)); color: var(--ink); }
  h3 { margin: 0; font: 600 14px/1.3 var(--sans); }
  .said, .subjects { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .said { gap: 6px; }
  .line { margin: 0; min-width: 0; font-size: 14px; line-height: 1.4; white-space: pre-wrap; overflow-wrap: anywhere; unicode-bidi: isolate; }
  .subject { font-family: var(--mono, monospace); }
  .more, .result { display: flex; align-items: flex-start; gap: 8px; margin: 0; min-width: 0; font-size: 14px; line-height: 1.4; overflow-wrap: anywhere; }
  .more { color: var(--ink); font-weight: 500; }
  .mark { flex: none; width: 20px; height: 20px; display: grid; place-items: center; border: 1px solid currentColor; border-radius: 50%; font-size: 14px; font-weight: 700; line-height: 1; }
  .go { display: inline; min-height: 0; padding: 0; border: 0; background: none; color: var(--accent-text); font: inherit; text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
  .options { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .option { flex-direction: column; align-items: flex-start; justify-content: center; gap: 2px; width: 100%; min-width: 0; min-height: 40px; padding: 8px 12px; text-align: left; font: 500 14px/1.35 var(--sans); }
  .option:active:not(:disabled) { background: var(--line-soft); }
  .label, .standing { min-width: 0; max-width: 100%; overflow-wrap: anywhere; white-space: pre-wrap; unicode-bidi: isolate; }
  .default, .standing { color: var(--muted); font-weight: 400; }
  .option[aria-busy='true']:disabled { opacity: 1; }
  .result { color: var(--ink); padding: 2px 6px; margin: 0 -6px; }
  .result:empty { display: none; }
  .result:focus { outline: none; }
  .result:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 4px; }
  .result.error { color: var(--bad); }
  .result.done { font-weight: 600; }
  @media (pointer: coarse), (max-width: 650px) { .option { min-height: 48px; } }
  /* The subject lines scroll inside their own 30% of the screen, so the options stay close below; the question sentence itself never scrolls away. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) { .subjects { max-height: 30dvh; overflow-y: auto; overscroll-behavior: contain; } }
</style>
