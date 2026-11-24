<script lang="ts">
  import Sentence from '../Sentence.svelte'
  import { tick, untrack } from 'svelte'
  import { signalAttentionChanged } from '../attentionSignal'
  import { answerTerminalQuestion, chatToolName, type TerminalQuestion } from './api'
  import { needsConfirm, nextTap, optionText, outcomeOf, standingPermission, type AnswerOutcome } from './terminalQuestion'

  // The dialog a terminal agent is waiting on, answered from reet: the agent's own words and options, one button each, all
  // of equal weight (nothing invites a reflex tap). Everything in `question` was read off a terminal screen, so it is
  // drawn as text only. Shown on the session screen and inside the session's item in Home's Waiting for you. Both holders
  // already show the session's title and machine, so the visible heading stays short; the group's name and its one polite
  // announcement carry them, to tell several waiting sessions apart.
  let { session, question, title = '', machine = '', href = '', locked: lockedReason = '', bare = false, onopen, onreread }: {
    session: { id: string; tool?: string }
    question: TerminalQuestion
    title?: string
    machine?: string
    /** Why answering is not possible from here, when the page already knows (the machine is not connected): the options start disabled. Read-only accounts never reach this page, and no machine capability says whether a machine can answer, so those cases answer after a tap. */
    locked?: string
    /** Drawn in the row it sits in, without its own box (Home). */
    bare?: boolean
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
  const sessionId = $derived(session.id)

  let pressed = $state<number | null>(null)
  let outcome = $state<AnswerOutcome | null>(null)
  let sending = $state(false) // a second tap before the first reply sends nothing
  let result = $state<HTMLElement>()
  let confirming = $state<number | null>(null) // the option waiting for its second tap
  let viaKey = $state(false) // the last input was the keyboard: only then does the result line show a focus ring
  let keepOutcome = false

  const done = $derived(!!outcome?.done)
  const locked = $derived(!!outcome?.locked || !!lockedReason)
  const shown = $derived(outcome ?? (lockedReason ? { say: lockedReason, tone: 'note' as const } : null))
  const heading = $derived(`${name} ${done ? 'was asked' : 'is asking'}`)

  // A different question starts clean; the sentence about a change stays over the new one, once.
  $effect(() => {
    void fingerprint; void sessionId
    untrack(() => {
      pressed = null; sending = false; confirming = null
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

  // One tap sends a one-off option. A standing-permission option, or one whose label was cut, asks for a second tap on
  // the same button first (Enter and Space are taps too); another option, Escape or a new question cancels it.
  function tap(option: TerminalQuestion['options'][number]) {
    if (sending || locked) return
    const step = nextTap(confirming, option.index, needsConfirm(option))
    confirming = step.pending
    if (step.send) void choose(option.index, option.label)
    else announced = `Tap again to confirm: ${optionText(option)}`
  }
  function key(event: KeyboardEvent) {
    viaKey = true
    if (event.key === 'Escape' && confirming !== null) { confirming = null; announced = 'Not confirmed. Nothing was sent.' }
  }

  async function choose(index: number, label: string) {
    sending = true
    pressed = index
    outcome = { say: 'Sending your answer…', tone: 'note', waitingForWords: false, done: false, locked: false, reread: false }
    void look()
    const asked = { id: sessionId, fingerprint }
    const next = outcomeOf(await answerTerminalQuestion(asked.id, asked.fingerprint, index), label, { agent: name, machine })
    // The question may have changed while the answer was on its way: what came back is about the old one. The session list
    // did change if it went through, so that is still said; nothing else about it is shown on the new question.
    if (asked.id !== sessionId || asked.fingerprint !== fingerprint) {
      if (next.done) { signalAttentionChanged(); await onreread?.() }
      return
    }
    outcome = next; pressed = null; sending = false
    void look()
    if (next.done) signalAttentionChanged()
    if (next.reread) keepOutcome = true
    if (next.done || next.reread) await onreread?.()
  }
</script>

<svelte:window onkeydown={key} onpointerdown={() => (viaKey = false)} />

{#snippet way()}{#if href}<a class="way" href={href}>Open the terminal</a>{:else if onopen}<button type="button" class="go way" onclick={onopen}>Go to the terminal</button>{/if}{/snippet}

<div class="card" class:bare role="group" aria-label="{name} {done ? 'was asked' : 'is asking'}{where}" data-terminal-question>
  <h3>{heading}</h3>
  <div class="said">
    {#if lines.length > 1}<div class="subjects">{#each lines.slice(0, -1) as line, i (i)}<p class="line subject">{line}</p>{/each}</div>{/if}
    <p class="line">{lines.at(-1)}</p>
  </div>
  {#if question.truncated && !done}
    <p class="more"><span class="mark" aria-hidden="true">!</span><span>Part of this is only in the terminal. Read it there before answering.{@render way()}</span></p>
  {/if}
  {#if !done}
    <div class="options">
      {#each question.options as option (`${fingerprint}:${option.index}`)}
        <button type="button" class="btn option" class:confirming={confirming === option.index} disabled={sending || locked}
          aria-busy={pressed === option.index ? 'true' : undefined} onclick={() => tap(option)}>
          {#if confirming === option.index}<span class="confirm">Tap again to confirm</span>{/if}
          <span class="label">{optionText(option)}{#if option.selected}<span class="default">{' · terminal’s default'}</span>{/if}</span>
          {#if standingPermission(option.label)}<span class="standing">Keeps applying after this answer.</span>{/if}
          {#if option.truncated}<span class="cut"><span class="mark" aria-hidden="true">!</span><span>This option is cut short. Read it in the terminal before choosing.</span></span>{/if}
        </button>
      {/each}
    </div>
  {/if}
  <p class="vh" role="status">{announced}</p>
  <p class="result {shown?.tone ?? ''}" class:ring={viaKey} role="status" tabindex="-1" bind:this={result}>{#if shown}{#if shown.tone === 'done'}<span aria-hidden="true">✓ </span>{:else if shown.tone === 'error'}<span class="mark" aria-hidden="true">!</span>{/if}<span><Sentence text={shown.say} />{#if outcome?.waitingForWords}{@render way()}{/if}</span>{/if}</p>
</div>

<style>
  .card { display: flex; flex-direction: column; gap: 8px; min-width: 0; box-sizing: border-box; padding: 12px 14px; border: 1px solid var(--line-strong, var(--line)); border-radius: 10px; background: var(--card-raised, var(--card)); color: var(--ink); }
  .card.bare { padding: 0; border: 0; background: none; border-radius: 0; }
  h3 { margin: 0; font: 600 14px/1.3 var(--sans); }
  .said, .subjects { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
  .said { gap: 6px; }
  .line { margin: 0; min-width: 0; font-size: 14px; line-height: 1.4; white-space: pre-wrap; overflow-wrap: anywhere; unicode-bidi: isolate; }
  .subject { font-family: var(--mono, monospace); }
  .more, .result { display: flex; align-items: flex-start; gap: 8px; margin: 0; min-width: 0; font-size: 14px; line-height: 1.4; overflow-wrap: anywhere; }
  .more { color: var(--ink); font-weight: 500; }
  .mark { flex: none; width: 20px; height: 20px; display: grid; place-items: center; border: 1px solid currentColor; border-radius: 50%; font-size: 14px; font-weight: 700; line-height: 1; }
  .way { display: flex; align-items: center; width: max-content; max-width: 100%; margin-top: 2px; color: var(--accent-text); font-weight: 500; text-decoration: underline; text-underline-offset: 3px; }
  .go { min-height: 0; padding: 0; border: 0; background: none; font: inherit; font-weight: 500; cursor: pointer; }
  .options { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .option { flex-direction: column; align-items: flex-start; justify-content: center; gap: 2px; width: 100%; min-width: 0; min-height: 40px; padding: 8px 12px; text-align: left; font: 500 14px/1.35 var(--sans); }
  .option.confirming { border-color: var(--ink); box-shadow: inset 0 0 0 1px var(--ink); }
  .confirm { font-weight: 700; }
  .cut { display: flex; align-items: flex-start; gap: 8px; color: var(--ink); font-weight: 500; }
  .option:active:not(:disabled) { background: var(--line-soft); }
  .label, .standing { min-width: 0; max-width: 100%; overflow-wrap: anywhere; white-space: pre-wrap; unicode-bidi: isolate; }
  .default, .standing { color: var(--muted); font-weight: 400; }
  .option[aria-busy='true']:disabled { opacity: 1; }
  .result { color: var(--ink); padding: 2px 6px; margin: 0 -6px; }
  .result:empty { display: none; }
  .result:focus { outline: none; }
  /* The product's own ring, and only while the keyboard is what focused it. */
  .result.ring:focus { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 4px; }
  .result.error { color: var(--bad); }
  .result.done { font-weight: 600; }
  .result .way { font-weight: 500; }
  /* The subject lines scroll inside their own box at any width, so the options stay close below; the question sentence itself never scrolls away. */
  .subjects { max-height: min(30dvh, 12em); overflow-y: auto; overscroll-behavior: contain; }
  @media (pointer: coarse), (max-width: 650px) { .option { min-height: 48px; } .way { min-height: 44px; } }
</style>
