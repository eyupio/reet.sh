<script lang="ts">
  import { elapsedWord } from '../sessionInsights'
  import type { FixKind } from './fixMessages'
  import type { Stages, StepState } from './overviewStages'
  import { fixButtonText } from './fixDelivery'
  import { stageItems, type ActiveFollow, type FollowItem } from './stageActions'
  import { tick } from 'svelte'
  import InsightIcon from './InsightIcon.svelte'

  // `terminal` is a session with no composer to fill, so the button copies; `note` is what the last click did, said politely and in words.
  // `stale` is the "Last known" marker when the GitHub read behind the Next line can no longer be vouched for; empty when it is current.
  // `onfollow` is told which follow-up item a person picked from a stage's menu; what it does with it is the caller's business.
  // `following` is the goal Gaffer is looking after this session with, when there is one; `onunfollow` stops it.
  let { id, stages, now, onsession, onfix, onfollow, following = null, onunfollow, unfollowBusy = false, terminal = false, note = '', stale = '' }: { id: string; stages: Stages; now: number; onsession: () => void; onfix: (kind: FixKind) => void; onfollow?: (item: FollowItem) => void; following?: ActiveFollow | null; onunfollow?: () => void; unfollowBusy?: boolean; terminal?: boolean; note?: string; stale?: string } = $props()

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

  // A stage's menu: click, right-click, Enter, Space or the Menu key on its cell. One menu is open at a time, below the row.
  let menuFor = $state(''), menuLabel = $state(''), opener: HTMLElement | null = null
  let menuEl = $state<HTMLElement>()
  const items = $derived(menuFor ? stageItems(menuFor) : [])
  async function openMenu(event: Event, key: string, label: string) {
    event.preventDefault()
    if (menuFor === key) { closeMenu(true); return }
    opener = event.currentTarget as HTMLElement
    menuFor = key; menuLabel = label
    await tick()
    menuEl?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
  }
  function closeMenu(refocus: boolean) {
    const back = opener
    menuFor = ''; opener = null
    if (refocus) back?.focus()
  }
  function pick(item: FollowItem) {
    closeMenu(true)
    onfollow?.(item)
  }
  function stop() {
    closeMenu(true)
    onunfollow?.()
  }
  // A goal whose state needs a word beside "Following", so a paused or stuck goal never reads as running.
  const followWord = $derived(following && following.state !== 'active' ? (following.state === 'needs_user' ? 'needs you' : following.state === 'limit' ? 'limit reached' : following.state) : '')
  function menuKey(event: KeyboardEvent) {
    const all = [...(menuEl?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])]
    const at = all.indexOf(document.activeElement as HTMLElement)
    const move = (to: number) => { event.preventDefault(); all[(to + all.length) % all.length]?.focus() }
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeMenu(true) }
    else if (event.key === 'ArrowDown') move(at + 1)
    else if (event.key === 'ArrowUp') move(at - 1)
    else if (event.key === 'Home') move(0)
    else if (event.key === 'End') move(all.length - 1)
    // The menu goes away on Tab; focus is put back on the stage by hand, so a parent's own Tab handling cannot move it to a control that is about to be removed.
    else if (event.key === 'Tab') { event.preventDefault(); event.stopPropagation(); closeMenu(true) }
  }
  // A press anywhere else closes it; one on the menu or on a stage's own button is that control's business.
  function outside(event: PointerEvent) {
    if (!menuFor) return
    const target = event.target as Node
    if (menuEl?.contains(target) || (target as Element).closest?.('.hit')) return
    closeMenu(false)
  }
</script>

<svelte:window onpointerdown={outside} />

<div class="tracker" class:terminal role="group" aria-labelledby={`${id}-progress`}>
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
        <li data-step={step.key} data-state={step.state} data-following={following?.stage === step.key ? 'true' : undefined} aria-current={i === here ? 'step' : undefined} title={following?.stage === step.key ? `${step.detail} Gaffer is following this stage.` : step.detail}>
          {#if following?.stage === step.key}<span class="follow-dot" aria-hidden="true"></span>{/if}
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
          <button type="button" class="hit" aria-haspopup="menu" aria-expanded={menuFor === step.key} aria-controls={menuFor === step.key ? `${id}-menu` : undefined}
            aria-label={`${step.label}: goal options${following?.stage === step.key ? ', Gaffer is following' : ''}`} onclick={(event) => openMenu(event, step.key, step.label)} oncontextmenu={(event) => openMenu(event, step.key, step.label)}></button>
        </li>
      {/each}
    </ol>
    <!-- Takes no room: the menu hangs from here over the Next slot, so opening it moves nothing below. -->
    <div class="menu-anchor">
      {#if menuFor && items.length}
        <div class="menu" bind:this={menuEl} onkeydown={menuKey} role="presentation">
          <p class="menu-head" id={`${id}-menu-head`}>{terminal ? 'Copy a message for the agent' : 'Ask Gaffer to look after this'}</p>
          <div class="menu-list" id={`${id}-menu`} role="menu" aria-label={`${menuLabel}: goal options`} aria-describedby={`${id}-menu-head`}>
            {#each items as item (item.id)}
              {@const active = following?.itemId === item.id}
              <button type="button" role="menuitem" class="menu-item" class:active aria-current={active ? 'true' : undefined} aria-label={terminal ? `Copy message for the agent: ${item.label}` : `${item.label}${active ? ', following' : ''}`} onclick={() => pick(item)}>
                {#if active}<span class="tick" aria-hidden="true"><InsightIcon name="check" /></span>{/if}<span>{item.label}</span>{#if active}<span class="sr">, following</span>{/if}
              </button>
            {/each}
            {#if following?.stage === menuFor && onunfollow}
              <button type="button" role="menuitem" class="menu-item stop" disabled={unfollowBusy} onclick={stop}>{unfollowBusy ? 'Stopping…' : `Stop following: ${following.label}`}</button>
            {/if}
          </div>
        </div>
      {/if}
    </div>
    <!-- What Gaffer is looking after, when a goal is running: the same item a person picked from a stage, with a way to stop it. -->
    {#if following}
      <div class="following" role="status" data-state={following.state}>
        <span class="follow-icon" aria-hidden="true"><InsightIcon name="check" /></span>
        <p class="follow-text"><span class="follow-verb">Following{followWord ? ` · ${followWord}` : ''}</span> <strong title={following.label}>{following.label}</strong></p>
        {#if onunfollow}<button type="button" class="follow-stop" disabled={unfollowBusy} title="Stop Gaffer looking after this session. Queued goal instructions are removed; the working agent keeps running." onclick={() => onunfollow?.()}>{unfollowBusy ? 'Stopping…' : 'Stop'}</button>{/if}
      </div>
    {/if}
    <!-- The Next slot has one height whatever it says: the sentence, one row for its buttons, and the line for what the last fix click did,
         held empty when there is nothing to ask or say, so a pull request that changes does not move the blocks below. -->
    <div class="next" data-tone={next.tone}>
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
       - Under 345 px for a terminal session (its "Copy message for the agent" and a primary need 316 px), the two buttons take a
         row each; under 290 px for every session, and the fix note then has two lines (a failed copy's note is 244 px).
       - Under 258 px (a panel under about 306 px, so the 300 px minimum): three steps over two, each icon beside its label, with
         "Planning" whole again.
     Every one of these has fixed heights, so a pull request that changes moves nothing below at any one width. */
  .tracker { --steps: 72px; --gap: 8px; --btn: 34px; --next: calc(var(--btn) + 76px); container-type: inline-size; min-width: 0; }
  h4 { margin: 0 0 10px; font: 600 14px var(--sans); letter-spacing: 0.01em; color: var(--muted); }
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
  /* A stage Gaffer is following keeps its state word and gains a dot: the dot never replaces the word or the colour. */
  .follow-dot { position: absolute; top: 1px; right: 1px; width: 7px; height: 7px; border-radius: 50%; background: var(--accent); box-shadow: 0 0 0 2px var(--bg); z-index: 1; }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  /* The whole cell is the target; the cell's own text stays as it was. */
  .hit { position: absolute; inset: 0; width: 100%; padding: 0; border: 0; border-radius: 6px; background: none; cursor: pointer; }
  .hit:hover { background: color-mix(in srgb, var(--ink) 6%, transparent); }
  .hit[aria-expanded='true'] { background: color-mix(in srgb, var(--accent) 12%, transparent); }
  .menu-anchor { position: relative; height: 0; z-index: 6; }
  .menu { position: absolute; top: 0; left: 0; right: 0; box-sizing: border-box; padding: 6px; border: 1px solid var(--line); border-radius: 8px; background: var(--card, var(--bg)); box-shadow: var(--shadow); }
  .menu-list { display: grid; gap: 2px; }
  .menu-head { margin: 0; padding: 4px 8px 6px; font-size: 14px; color: var(--muted); }
  .menu-item { display: flex; align-items: center; gap: 6px; font: inherit; font-size: 14px; text-align: left; min-height: 36px; padding: 6px 8px; border: 0; border-radius: 6px; background: none; color: var(--ink); cursor: pointer; overflow-wrap: anywhere; }
  .menu-item:hover, .menu-item:focus-visible { background: color-mix(in srgb, var(--accent) 14%, transparent); }
  .menu-item.active { color: var(--accent-text); font-weight: 600; }
  .menu-item .tick { display: inline-flex; flex: none; color: var(--accent-text); }
  .menu-item.stop { margin-top: 2px; border-top: 1px solid var(--line-soft); border-radius: 0 0 6px 6px; color: var(--bad); }
  .menu-item.stop:disabled { opacity: .6; cursor: default; }
  /* The following strip is only as tall as its words; it appears when a goal starts and goes when it stops. */
  .following { box-sizing: border-box; display: flex; align-items: center; gap: 8px; min-height: 36px; margin-bottom: var(--gap); padding: 4px 8px; border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--line)); border-radius: 8px; background: color-mix(in srgb, var(--accent) 7%, transparent); color: var(--ink-soft); font-size: 14px; line-height: 20px; }
  .follow-icon { display: inline-flex; flex: none; color: var(--accent-text); }
  .follow-text { flex: 1; min-width: 0; margin: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .follow-verb { color: var(--muted); }
  .follow-text strong { color: var(--ink); }
  .follow-stop { flex: none; min-height: 28px; padding: 0 8px; border: 1px solid var(--line); border-radius: 6px; background: var(--card, var(--bg)); color: var(--bad); font: inherit; font-size: 14px; cursor: pointer; }
  .follow-stop:hover { background: color-mix(in srgb, var(--bad) 8%, transparent); }
  .follow-stop:disabled { opacity: .6; cursor: default; }

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
  @container (max-width: 344px) {
    /* Set on the tracker's children, not on the Next slot alone, so the merged row that replaces them is just as tall. */
    .tracker.terminal > * { --next: calc(2 * var(--btn) + 80px); }
    .tracker.terminal .next { grid-template-columns: minmax(0, 1fr); grid-template-rows: 40px 4px var(--btn) 4px var(--btn) 6px 20px; grid-template-areas: 'text' '.' 'primary' '.' 'fix' '.' 'note'; }
    .tracker.terminal .primary { margin-right: 0; }
  }
  @container (max-width: 305px) { .long { display: none; } .short { display: inline; } }
  @container (max-width: 289px) {
    /* A row each for the buttons, and two lines for the note: "Could not copy the message. Try again." is 244 px. */
    .tracker > *, .tracker.terminal > * { --next: calc(2 * var(--btn) + 100px); }
    .next, .tracker.terminal .next { grid-template-columns: minmax(0, 1fr); grid-template-rows: 40px 4px var(--btn) 4px var(--btn) 6px 40px; grid-template-areas: 'text' '.' 'primary' '.' 'fix' '.' 'note'; }
    .primary, .tracker.terminal .primary { margin-right: 0; }
    .fix-note { align-self: start; white-space: normal; overflow-wrap: anywhere; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }
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
  @media (pointer: coarse) { .tracker { --btn: 44px; } .primary, .merged a { min-height: 44px; min-width: 44px; } .fix { min-height: 44px; min-width: 44px; } .menu-item { min-height: 44px; } .follow-stop { min-height: 40px; min-width: 44px; } }
</style>
