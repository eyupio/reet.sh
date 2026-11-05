<script lang="ts">
  import { elapsedWord } from '../sessionInsights'
  import type { FixKind } from './fixMessages'
  import type { Stages, StepState } from './overviewStages'
  import { fixButtonText } from './fixDelivery'
  import InsightIcon from './InsightIcon.svelte'

  // `terminal` is a session with no composer to fill, so the button copies; `note` is what the last click did, said politely and in words.
  // `stale` is the "Last known" marker when the GitHub read behind the Next line can no longer be vouched for; empty when it is current.
  let { id, stages, now, onsession, onfix, terminal = false, note = '', stale = '' }: { id: string; stages: Stages; now: number; onsession: () => void; onfix: (kind: FixKind) => void; terminal?: boolean; note?: string; stale?: string } = $props()

  // Every state has a word as well as an icon and a colour: colour is never the only signal.
  const WORDS: Record<StepState, string> = { done: 'Done', current: 'Now', attention: 'Needs you', upcoming: 'Not yet', unknown: 'Not read' }
  // The button's name says what it is for; the visible text is the part that fits a 300 px panel.
  const FIX: Record<FixKind, { what: string; short: string }> = {
    conflicts: { what: 'fix the conflicts', short: 'Ask the agent to fix' },
    checks: { what: 'fix the checks', short: 'Ask the agent to fix' },
    threads: { what: 'address the threads', short: 'Ask the agent to address' },
  }
  // Labels a five-across row shortens rather than cut mid-word; the cell is too narrow for "Planning" only between two widths (see the style).
  const SHORT: Record<string, string> = { Planning: 'Plan' }
  // One step is "the step", the one the Next line talks about: the first that needs the person, else the first in progress.
  // Failing checks with a review still to come announce Checks, not Review.
  const here = $derived.by(() => {
    const needs = stages.steps.findIndex(s => s.state === 'attention')
    return needs >= 0 ? needs : stages.steps.findIndex(s => s.state === 'current')
  })
  const next = $derived(stages.next)
  const primary = $derived(next.primary)
  const fix = $derived(next.fix ? FIX[next.fix] : undefined)
  // The accessible name keeps the kind, and for a terminal still contains the visible words.
  const fixName = $derived(fix ? (terminal ? `Copy message for the agent to ${fix.what}` : `Ask the agent to ${fix.what}`) : '')
</script>

<div class="tracker" role="group" aria-labelledby={`${id}-progress`}>
  <h4 id={`${id}-progress`}>Progress</h4>
  {#if stages.merged}
    <div class="merged">
      <span class="done-icon" aria-hidden="true"><InsightIcon name="check" /></span>
      <strong>Merged</strong>
      {#if stages.merged.at}<span class="when">{elapsedWord(stages.merged.at, now)}</span>{/if}
      {#if stages.merged.url}<a href={stages.merged.url} target="_blank" rel="noopener noreferrer">Open pull request <InsightIcon name="arrow" /></a>{/if}
    </div>
  {:else}
    <ol>
      {#each stages.steps as step, i (step.key)}
        <li data-step={step.key} data-state={step.state} aria-current={i === here ? 'step' : undefined} title={step.detail}>
          <span class="bar" aria-hidden="true"></span>
          <span class="icon" aria-hidden="true">
            {#if step.state === 'done'}<InsightIcon name="check" />
            {:else if step.state === 'current'}<InsightIcon name="clock" />
            {:else if step.state === 'attention'}<InsightIcon name="conflict" />
            {:else}<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="8" stroke-dasharray={step.state === 'unknown' ? '3 3' : undefined} />{#if step.state === 'unknown'}<path d="M10 10a2 2 0 1 1 3 1.7c-.6.4-1 .8-1 1.5M12 16.5v.1" />{/if}</svg>{/if}
          </span>
          <span class="label">{#if SHORT[step.label]}<span class="long">{step.label}</span><span class="short">{SHORT[step.label]}</span>{:else}{step.label}{/if}</span>
          <span class="word">{WORDS[step.state]}</span>
          <span class="sr">{step.detail}</span>
        </li>
      {/each}
    </ol>
    <!-- The Next slot has one height whatever it says: the sentence, one row for its buttons, and the line for what the last fix click did,
         held empty when there is nothing to ask or say, so a pull request that changes does not move the blocks below. -->
    <div class="next" class:terminal data-tone={next.tone}>
      <!-- The sentence stays, as the last thing known to be true; the marker before it keeps it from reading as current. -->
      <p title={stale ? `${stale}. ${next.text}` : next.text}>{#if stale}<span class="stale">{stale}.</span> {/if}{next.text}</p>
      {#if primary?.kind === 'session'}<button type="button" class="primary" aria-label="Review the question in the session" onclick={() => onsession()}>{primary.label}<span class="arrow"><InsightIcon name="arrow" /></span></button>
      {:else if primary?.kind === 'link' && primary.href}<a class="primary" href={primary.href} target="_blank" rel="noopener noreferrer">{primary.label}<span class="arrow"><InsightIcon name="arrow" /></span></a>{/if}
      {#if fix && next.fix}{@const kind = next.fix}<button type="button" class="fix" aria-label={fixName} onclick={() => onfix(kind)}>{fixButtonText(!terminal, fix.short)}</button>{/if}
      <span class="fix-note" role="status" aria-live="polite" title={note || undefined}>{note}</span>
    </div>
  {/if}
</div>

<style>
  /* The type is 14px like the rest of the UI and quiet labels are quiet in colour and weight, never in size. The layout is set by
     the tracker's own width (a container), from measured 14px words: "Planning" 57 px, "Review" 47 px, "Needs you" 66 px, and the
     Next buttons ("Open failing check" 116 px of text, "Ask the agent to fix" 120, "Copy message for the agent" 174).
       - 356 px and wider: five steps across with one-line words. At every width from 290 px the Next slot is the sentence (two
         lines at most) above one row holding both buttons, and a line for the fix note under the box; under 400 px the button's
         arrow goes, as its words already say where it leads.
       - 258 to 355 px (the default 360 px panel is 312 px here): five across, and a state word may take two lines ("Needs / you").
         Under 306 px a cell is too narrow for "Planning" (58 px), so it reads "Plan".
       - Under 290 px, or under 338 px for a terminal session's longer "Copy message for the agent", the two buttons take a row each.
       - Under 258 px (a panel under about 306 px, so the 300 px minimum): three steps over two, each icon beside its label, with
         "Planning" whole again.
     Every one of these has fixed heights, so a pull request that changes moves nothing below at any one width. */
  .tracker { --steps: 72px; --gap: 8px; --btn: 34px; --next: calc(var(--btn) + 76px); container-type: inline-size; min-width: 0; }
  h4 { margin: 0 0 10px; font: 600 14px var(--sans); letter-spacing: 0.07em; text-transform: uppercase; color: var(--muted); }
  ol { list-style: none; margin: 0 0 var(--gap); padding: 0; display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); grid-auto-rows: 72px; gap: 6px 3px; height: var(--steps); }
  li { --tone: var(--muted); position: relative; min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 3px; font-size: 14px; color: var(--muted); }
  .bar { align-self: stretch; height: 3px; margin-bottom: 4px; border-radius: 2px; background: var(--line); }
  .icon { display: inline-flex; height: 16px; color: var(--tone); }
  .label, .word { max-width: 100%; overflow: hidden; white-space: nowrap; line-height: 20px; }
  .label { font-weight: 600; color: var(--ink); }
  .short { display: none; }
  .word { color: var(--tone); height: 20px; }
  li[data-state='done'] { --tone: var(--good); }
  li[data-state='done'] .bar { background: var(--good); }
  li[data-state='current'] { --tone: var(--accent-text); }
  li[data-state='current'] .bar { background: var(--accent); }
  li[data-state='attention'] { --tone: var(--bad); }
  li[data-state='attention'] .bar { background: var(--bad); }
  li[data-state='unknown'] .bar { background: repeating-linear-gradient(90deg, var(--muted) 0 4px, transparent 4px 7px); }
  li[data-state='current'] .icon { animation: pulse 2.4s ease-in-out infinite; }
  @keyframes pulse { 50% { opacity: 0.45; } }
  @media (prefers-reduced-motion: reduce) { li[data-state='current'] .icon { animation: none; } }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

  /* One grid for the sentence, the buttons and the note. The tinted box is drawn behind the sentence and the buttons only and stops
     above the note's line, so an empty note leaves no empty band inside the box. Absent buttons leave their places empty. */
  .next { box-sizing: border-box; position: relative; isolation: isolate; height: var(--next); padding: 6px 10px 0; display: grid; grid-template-columns: auto auto minmax(0, 1fr); grid-template-rows: 40px 4px var(--btn) 6px 20px; grid-template-areas: 'text text text' '. . .' 'primary fix .' '. . .' 'note note note'; align-items: center; color: var(--ink-soft); font-size: 14px; line-height: 20px; overflow: hidden; }
  .next::before { content: ''; position: absolute; inset: 0 0 20px 0; z-index: -1; box-sizing: border-box; border: 1px solid var(--line); border-radius: 8px; background: var(--tint); }
  .next p { grid-area: text; min-width: 0; margin: 0; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; align-self: center; }
  .stale { color: var(--muted); font-weight: 600; }
  .next[data-tone='bad'], .next[data-tone='warn'], .next[data-tone='good'] { color: var(--ink); }
  .next[data-tone='bad']::before { border-color: color-mix(in srgb, var(--bad) 40%, transparent); background: color-mix(in srgb, var(--bad) 7%, transparent); }
  .next[data-tone='warn']::before { border-color: color-mix(in srgb, var(--warn) 40%, transparent); background: color-mix(in srgb, var(--warn) 7%, transparent); }
  .next[data-tone='good']::before { border-color: color-mix(in srgb, var(--good) 40%, transparent); background: color-mix(in srgb, var(--good) 7%, transparent); }
  .primary { grid-area: primary; justify-self: start; margin-right: 10px; box-sizing: border-box; display: inline-flex; align-items: center; justify-content: center; gap: 5px; min-height: var(--btn); padding: 0 8px; border: 0; border-radius: 7px; background: var(--accent); color: var(--accent-ink); font: inherit; font-size: 14px; text-decoration: none; cursor: pointer; white-space: nowrap; overflow: hidden; }
  .primary:hover { filter: brightness(1.06); }
  .arrow { display: inline-flex; }
  .fix { grid-area: fix; justify-self: start; font: inherit; font-size: 14px; cursor: pointer; display: inline-flex; align-items: center; border: 0; background: none; padding: 0; min-height: var(--btn); color: var(--ink); text-decoration: underline; text-underline-offset: 2px; white-space: nowrap; }
  .fix-note { grid-area: note; min-width: 0; font-size: 14px; line-height: 20px; color: var(--ink-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  /* The merged row is as tall as the steps and the Next slot together, so merging moves nothing below. */
  .merged { box-sizing: border-box; height: calc(var(--steps) + var(--gap) + var(--next)); display: flex; align-items: center; gap: 10px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--tint); color: var(--ink-soft); font-size: 14px; line-height: 20px; overflow: hidden; }
  .merged strong { color: var(--ink); font-size: 14px; }
  .done-icon { display: inline-flex; color: var(--good); }
  .when { flex: 1; min-width: 0; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .merged a { margin-left: auto; flex: none; display: inline-flex; align-items: center; gap: 5px; min-height: 30px; color: var(--ink); text-decoration: underline; text-underline-offset: 2px; white-space: nowrap; }
  .merged strong + a { margin-left: auto; }

  @container (max-width: 399px) {
    .arrow { display: none; }
    .merged { flex-wrap: wrap; align-content: center; row-gap: 6px; }
    .merged a, .merged strong + a { flex-basis: 100%; margin-left: 0; }
  }
  @container (max-width: 355px) {
    /* Five cells of about 50 to 68 px: a state word may break at its space, so its place is two lines tall in every state. */
    .tracker > * { --steps: 92px; }
    ol { grid-auto-rows: 92px; }
    .word { white-space: normal; height: 40px; }
  }
  @container (max-width: 337px) {
    .next.terminal { --next: calc(2 * var(--btn) + 80px); grid-template-columns: minmax(0, 1fr); grid-template-rows: 40px 4px var(--btn) 4px var(--btn) 6px 20px; grid-template-areas: 'text' '.' 'primary' '.' 'fix' '.' 'note'; }
    .next.terminal .primary { margin-right: 0; }
  }
  @container (max-width: 305px) { .long { display: none; } .short { display: inline; } }
  @container (max-width: 289px) {
    .tracker > * { --next: calc(2 * var(--btn) + 80px); }
    .next { grid-template-columns: minmax(0, 1fr); grid-template-rows: 40px 4px var(--btn) 4px var(--btn) 6px 20px; grid-template-areas: 'text' '.' 'primary' '.' 'fix' '.' 'note'; }
    .primary { margin-right: 0; }
  }
  @container (max-width: 257px) {
    /* Five cells would be 47 px, the width of "Review" itself: three over two instead. A cell of 80 px or more holds the icon and
       the whole label side by side ("Planning" is 77 px with its icon) and every state word on one line beneath them. */
    .tracker > * { --steps: 114px; }
    ol { grid-template-columns: repeat(3, minmax(0, 1fr)); grid-auto-rows: 54px; }
    li { display: grid; grid-template-columns: 16px minmax(0, 1fr); grid-template-rows: 3px 20px 20px; grid-template-areas: 'bar bar' 'icon label' 'word word'; column-gap: 4px; row-gap: 5px; align-items: center; }
    .bar { grid-area: bar; margin: 0; }
    .icon { grid-area: icon; }
    .label { grid-area: label; }
    .word { grid-area: word; white-space: nowrap; height: 20px; }
    .long { display: inline; } .short { display: none; }
  }
  :is(button, a):focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer: coarse) { .tracker { --btn: 44px; } .primary, .merged a { min-height: 44px; min-width: 44px; } .fix { min-height: 44px; min-width: 44px; } }
</style>
