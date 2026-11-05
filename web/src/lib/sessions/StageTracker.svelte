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
  // One step is "the step", the one the Next line talks about: the first that needs the person, else the first in progress.
  // Failing checks with a review still to come announce Checks, not Review.
  const here = $derived.by(() => {
    const needs = stages.steps.findIndex(s => s.state === 'attention')
    return needs >= 0 ? needs : stages.steps.findIndex(s => s.state === 'current')
  })
  // Labels that a narrow panel shortens rather than cut off mid-word.
  const SHORT: Record<string, string> = { Planning: 'Plan' }
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
    <!-- The Next slot is the height of the status notice above, whatever it says. -->
    <div class="next" data-tone={next.tone}>
      <!-- The sentence stays, as the last thing known to be true; the marker before it keeps it from reading as current. -->
      <p title={stale ? `${stale}. ${next.text}` : next.text}>{#if stale}<span class="stale">{stale}.</span> {/if}{next.text}</p>
      {#if primary?.kind === 'session'}<button type="button" class="primary" aria-label="Review the question in the session" onclick={() => onsession()}>{primary.label} <InsightIcon name="arrow" /></button>
      {:else if primary?.kind === 'link' && primary.href}<a class="primary" href={primary.href} target="_blank" rel="noopener noreferrer">{primary.label} <InsightIcon name="arrow" /></a>{/if}
    </div>
    <!-- Held empty when there is nothing to ask, so a pull request that changes does not move the blocks below. -->
    <div class="fix-row">{#if fix && next.fix}{@const kind = next.fix}<button type="button" class="fix" aria-label={fixName} onclick={() => onfix(kind)}>{fixButtonText(!terminal, fix.short)}</button>{/if}<span class="fix-note" role="status" aria-live="polite">{note}</span></div>
  {/if}
</div>

<style>
  .tracker { --steps: 60px; --gap: 10px; --next: 60px; --fix: 34px; container-type: inline-size; min-width: 0; }
  h4 { margin: 0 0 10px; font: 600 11px var(--sans); letter-spacing: 0.07em; text-transform: uppercase; color: var(--muted); }
  ol { list-style: none; margin: 0 0 var(--gap); padding: 0; display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 4px; height: var(--steps); }
  li { --tone: var(--muted); min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 3px; font-size: 11px; color: var(--muted); }
  .bar { align-self: stretch; height: 3px; margin-bottom: 4px; border-radius: 2px; background: var(--line); }
  .icon { display: inline-flex; height: 16px; color: var(--tone); }
  .label, .word { max-width: 100%; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; line-height: 14px; }
  .label { font-weight: 600; color: var(--ink); }
  .word { font-size: 10px; color: var(--tone); }
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
  li { position: relative; }
  @container (max-width: 290px) { li { font-size: 10px; } .word { font-size: 9px; } }
  .short { display: none; }
  /* The tracker is about 48 px narrower than the panel, so this is a panel under 340 px. */
  @container (max-width: 291px) { .long { display: none; } .short { display: inline; } }

  .next, .merged { box-sizing: border-box; display: flex; align-items: center; gap: 10px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--tint); color: var(--ink-soft); font-size: 11px; line-height: 1.5; overflow: hidden; }
  .next { height: var(--next); }
  .next p { flex: 1; min-width: 0; margin: 0; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
  .stale { color: var(--muted); font-weight: 600; }
  .next[data-tone='bad'] { border-color: color-mix(in srgb, var(--bad) 40%, transparent); background: color-mix(in srgb, var(--bad) 7%, transparent); color: var(--ink); }
  .next[data-tone='warn'] { border-color: color-mix(in srgb, var(--warn) 40%, transparent); background: color-mix(in srgb, var(--warn) 7%, transparent); color: var(--ink); }
  .next[data-tone='good'] { border-color: color-mix(in srgb, var(--good) 40%, transparent); background: color-mix(in srgb, var(--good) 7%, transparent); color: var(--ink); }
  .primary { flex: none; max-width: 52%; box-sizing: border-box; display: inline-flex; align-items: center; justify-content: center; gap: 5px; min-height: 34px; padding: 0 10px; border: 0; border-radius: 7px; background: var(--accent); color: var(--accent-ink); font: inherit; font-size: 12px; text-decoration: none; cursor: pointer; white-space: nowrap; overflow: hidden; }
  .primary:hover { filter: brightness(1.06); }
  .fix-row { height: var(--fix); display: flex; align-items: center; margin-top: 2px; }
  .fix { font: inherit; font-size: 11px; cursor: pointer; display: inline-flex; align-items: center; border: 0; background: none; padding: 5px 0; min-height: 30px; color: var(--ink); text-decoration: underline; text-underline-offset: 2px; }
  .fix-note { flex: 1; min-width: 0; margin-left: 10px; font-size: 10px; line-height: 12px; color: var(--ink-soft); overflow-wrap: anywhere; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; }
  /* The merged row is as tall as the tracker, the Next slot and the fix row together, so merging moves nothing below. */
  .merged { height: calc(var(--steps) + var(--gap) + var(--next) + var(--fix) + 2px); }
  .merged strong { color: var(--ink); font-size: 13px; }
  .done-icon { display: inline-flex; color: var(--good); }
  .when { flex: 1; min-width: 0; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .merged a { margin-left: auto; flex: none; display: inline-flex; align-items: center; gap: 5px; min-height: 30px; color: var(--ink); text-decoration: underline; text-underline-offset: 2px; }
  .merged strong + a { margin-left: auto; }
  :is(button, a):focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer: coarse) { .tracker { --fix: 44px; } .primary, .merged a { min-height: 44px; min-width: 44px; } .fix { min-height: 44px; min-width: 44px; } }
</style>
