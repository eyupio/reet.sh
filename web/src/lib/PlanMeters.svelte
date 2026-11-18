<script lang="ts">
  // What is left of each agent's plan, in one small control: a chip per account (the agent's mark, the share left of
  // its tightest window, and a hairline meter), a summary on hover or keyboard focus, and every window with its reset
  // on a click. The same control sits on Home, the Sessions toolbar, a chat's status strip and a Factory card.
  import { tick } from 'svelte'
  import { limitLines, limitsNote, readAgo, type PlanEntry } from './agentAccounts'
  import { chatToolName } from './terminal/api'
  import AgentMark from './AgentMark.svelte'

  let { entries, now = Date.now(), max = 3, caption = '', wide = false, label = 'Plan limits' }: {
    entries: PlanEntry[]; now?: number
    /** How many chips are drawn before the rest fold into +N. */ max?: number
    /** A word before the chips, where the place does not already say what they are. */ caption?: string
    /** Every window in words beside the mark, for a place with room (a Factory card). */ wide?: boolean
    label?: string
  } = $props()

  const shown = $derived(entries.slice(0, max)), folded = $derived(entries.length - shown.length)
  const name = (entry: PlanEntry) => chatToolName(entry.tool)
  const capital = (word: string) => word ? word[0].toUpperCase() + word.slice(1) : ''
  const line = (entry: PlanEntry) => `${name(entry)}: ${entry.plan.summary}${entry.stale ? ` (read ${readAgo(entry.account.limitsAt, now)})` : ''}`
  const spoken = $derived(`${label}. ${entries.map(line).join('. ')}. Open for every limit and when it resets.`)

  let strip = $state<HTMLButtonElement>(), tip = $state<HTMLDivElement>(), panel = $state<HTMLDivElement>()
  let open = $state(false), tipTimer: ReturnType<typeof setTimeout> | undefined
  const uid = `plan-${Math.random().toString(36).slice(2, 9)}`

  // Both layers are HTML popovers, so nothing clips them: not a scrolling pane, a dialog or the edge of a card.
  function place(layer: HTMLElement) {
    if (!strip) return
    const anchor = strip.getBoundingClientRect(), width = layer.offsetWidth, height = layer.offsetHeight, gap = 6, edge = 8
    const below = anchor.bottom + gap + height <= innerHeight - edge || anchor.top - gap - height < edge
    layer.style.top = `${Math.max(edge, below ? Math.min(anchor.bottom + gap, innerHeight - edge - height) : anchor.top - gap - height)}px`
    layer.style.left = `${Math.max(edge, Math.min(anchor.left + anchor.width / 2 - width / 2, innerWidth - edge - width))}px`
  }
  function showTip() {
    clearTimeout(tipTimer)
    tipTimer = setTimeout(() => {
      if (open || !tip || !strip?.isConnected) return
      try { tip.showPopover() } catch { return }
      place(tip)
    }, 140)
  }
  function hideTip() { clearTimeout(tipTimer); try { tip?.hidePopover() } catch { /* already closed */ } }
  async function toggle() {
    hideTip()
    if (!panel) return
    if (open) { panel.hidePopover(); return }
    panel.showPopover()
    await tick()
    place(panel)
    panel.querySelector<HTMLElement>('[data-plan-close]')?.focus({ preventScroll: true })
  }
  function toggled(event: ToggleEvent) {
    const was = open
    open = event.newState === 'open'
    // Closing with Escape or a click elsewhere returns focus to the chips when it was inside the panel.
    if (was && !open && panel?.contains(document.activeElement)) strip?.focus({ preventScroll: true })
  }
  // The page can scroll or resize under an open layer: it follows its chips, and a tip simply closes.
  $effect(() => {
    if (!open) return
    const follow = () => { if (panel) place(panel) }
    addEventListener('resize', follow); addEventListener('scroll', follow, true)
    return () => { removeEventListener('resize', follow); removeEventListener('scroll', follow, true) }
  })
  $effect(() => () => clearTimeout(tipTimer))
</script>

{#if entries.length}
<span class="plan-meters" class:wide data-plan-left>
  <button type="button" class="strip" bind:this={strip} aria-haspopup="dialog" aria-expanded={open} aria-controls={`${uid}-panel`} aria-label={spoken}
    onclick={() => void toggle()} onpointerenter={event => { if (event.pointerType === 'mouse') showTip() }} onpointerleave={hideTip}
    onfocus={event => { if (event.currentTarget.matches(':focus-visible')) showTip() }} onblur={hideTip} onkeydown={event => { if (event.key === 'Escape') hideTip() }}>
    {#if caption}<span class="caption">{caption}</span>{/if}
    {#each shown as entry (entry.key)}
      <span class="chip" class:low={entry.plan.low} class:stale={entry.stale}>
        <AgentMark tool={entry.tool} size={wide ? 16 : 14} />
        {#if wide}<span class="words">{entry.plan.summary}</span>
        {:else}<span class="value">{entry.plan.left}%</span><span class="bar" style={`--fill:${entry.plan.left}%`} aria-hidden="true"></span>{/if}
      </span>
    {/each}
    {#if folded > 0}<span class="more">+{folded}</span>{/if}
  </button>

  <div class="tip" popover="manual" role="tooltip" bind:this={tip}>
    <strong>{label}</strong>
    <ul>{#each entries as entry (entry.key)}
      <li class:low={entry.plan.low}><AgentMark tool={entry.tool} size={14} /><span class="who">{name(entry)}</span><span class="what">{entry.plan.summary}</span></li>
    {/each}</ul>
    <p>Click for every limit and when it resets.</p>
  </div>

  <div class="panel" popover="auto" role="dialog" aria-label={label} id={`${uid}-panel`} bind:this={panel} ontoggle={toggled}>
    <header><h3>{label}</h3><button type="button" class="close" data-plan-close aria-label={`Close ${label.toLowerCase()}`} onclick={() => panel?.hidePopover()}>×</button></header>
    {#each entries as entry (entry.key)}
      {@const note = limitsNote(entry.account, name(entry), now)}
      <section class:stale={entry.stale} aria-label={`${name(entry)}${entry.account.account ? `, ${entry.account.account}` : ''}`}>
        <div class="head">
          <AgentMark tool={entry.tool} size={18} /><span class="agent">{name(entry)}</span>
          {#if entry.account.plan}<span class="pill">{capital(entry.account.plan)}</span>{/if}
          {#if entry.account.account}<span class="account" title={entry.account.account}>{entry.account.account}</span>{/if}
        </div>
        {#each limitLines(entry.account, now) as limit (limit.key)}
          <div class="row" class:low={limit.low}>
            <span class="window">{limit.name}</span>
            {#if limit.resets}<span class="resets">{capital(limit.resets)}</span>{/if}
            <span class="left">{limit.left}% left</span>
            <span class="meter" role="meter" aria-label={`${limit.name} left`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={limit.left} aria-valuetext={`${limit.left}% left`}><span style={`width:${limit.left > 0 ? `max(4px, ${limit.left}%)` : '0'}`}></span></span>
          </div>
        {/each}
        <p class="source">Read on {entry.host}{readAgo(entry.account.limitsAt, now) ? ` ${readAgo(entry.account.limitsAt, now)}` : ''}.</p>
        {#if note}<p class="note" class:bad={entry.account.limitsState === 'renew' || entry.account.limitsState === 'refused'}>{note}</p>{/if}
      </section>
    {/each}
    <footer><span>The provider’s own figures, as each machine last read them.</span><span class="links"><a href="/usage" data-see-usage>See all usage</a><a href="/machines">Machines</a></span></footer>
  </div>
</span>
{/if}

<style>
  .plan-meters { display: inline-flex; min-width: 0; max-width: 100%; }
  .strip { display: inline-flex; align-items: center; gap: 8px; min-width: 0; max-width: 100%; min-height: 28px; padding: 2px 6px; border: 1px solid transparent; border-radius: 8px; background: none; color: var(--ink-soft); font: inherit; cursor: pointer; -webkit-tap-highlight-color: transparent; }
  .strip:hover, .strip[aria-expanded=true] { background: var(--tint); border-color: var(--line-soft); }
  .strip:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .caption { flex: none; color: var(--muted); font-size: 14px; }
  .chip { position: relative; flex: none; display: inline-flex; align-items: center; gap: 5px; min-height: 22px; padding-bottom: 3px; }
  .value, .more { font: 500 14px/1 var(--mono); font-variant-numeric: tabular-nums; color: var(--ink); }
  .more { flex: none; color: var(--muted); }
  /* A hairline under the chip, filled to what is left. */
  .bar { position: absolute; left: 0; right: 0; bottom: 0; height: 2px; border-radius: 2px; background: linear-gradient(to right, var(--good) var(--fill), var(--line) var(--fill)); }
  .chip.low .value, .chip.low .words { color: var(--warn); font-weight: 600; }
  .chip.low .bar { background: linear-gradient(to right, var(--warn) var(--fill), var(--line) var(--fill)); }
  /* Figures kept from an earlier read are a record, not an alarm: they dim and lose the warning colour. */
  .chip.stale { opacity: .6; }
  .chip.stale .value, .chip.stale .words { color: var(--muted); font-weight: 500; }
  .chip.stale .bar { background: linear-gradient(to right, var(--muted) var(--fill), var(--line) var(--fill)); }
  .wide .strip { padding: 2px 6px 2px 2px; margin-left: -2px; }
  .wide .chip { padding-bottom: 0; }
  .words { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ink); font-size: 14px; }

  .tip, .panel { position: fixed; inset: auto; margin: 0; box-sizing: border-box; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card-raised, var(--card)); color: var(--ink); box-shadow: var(--shadow-dock, var(--shadow)); font: 14px/1.4 var(--sans); }
  .tip { width: max-content; max-width: min(360px, calc(100vw - 16px)); padding: 10px 12px; pointer-events: none; }
  .tip strong { display: block; margin-bottom: 6px; font-size: 14px; color: var(--muted); font-weight: 600; }
  .tip ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 5px; }
  .tip li { display: grid; grid-template-columns: auto auto minmax(0, 1fr); align-items: center; gap: 6px 8px; }
  .tip .who { font-weight: 600; }
  .tip .what { color: var(--muted); font-variant-numeric: tabular-nums; }
  .tip li.low .what { color: var(--warn); }
  .tip p { margin: 8px 0 0; font-size: 14px; color: var(--muted); }

  .panel { width: min(360px, calc(100vw - 16px)); max-height: min(520px, calc(100dvh - 16px)); overflow: auto; padding: 0; }
  .panel header { position: sticky; top: 0; display: flex; align-items: center; justify-content: space-between; padding: 10px 8px 8px 14px; background: inherit; border-bottom: 1px solid var(--line-soft); }
  .panel h3 { margin: 0; font: 600 14px/1.3 var(--sans); }
  .close { width: 32px; height: 32px; border: 0; border-radius: 8px; background: none; color: var(--muted); font: 400 20px/1 var(--sans); cursor: pointer; }
  .close:hover { background: var(--tint); color: var(--ink); }
  .close:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  .panel section { padding: 12px 14px; border-bottom: 1px solid var(--line-soft); }
  .head { display: flex; align-items: center; gap: 8px; min-width: 0; margin-bottom: 10px; }
  .agent { flex: none; font-weight: 600; }
  .pill { flex: none; padding: 1px 7px; border: 1px solid var(--line); border-radius: 999px; font-size: 14px; line-height: 18px; color: var(--muted); }
  .account { flex: 1 1 0; min-width: 0; margin-left: auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: right; color: var(--muted); font-size: 14px; }
  .row { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: baseline; gap: 2px 8px; margin-top: 8px; }
  .window { font-weight: 500; }
  .resets { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--muted); font-size: 14px; }
  .left { grid-column: 3; font: 500 14px/1.4 var(--mono); font-variant-numeric: tabular-nums; }
  .meter { grid-column: 1 / -1; display: block; height: 4px; border-radius: 4px; background: var(--line); overflow: hidden; }
  .meter > span { display: block; height: 100%; border-radius: 4px; background: var(--good); }
  .row.low .left { color: var(--warn); font-weight: 600; }
  .row.low .meter > span { background: var(--warn); }
  section.stale .left { color: var(--muted); font-weight: 500; }
  section.stale .meter > span { background: var(--muted); }
  .source, .note { margin: 10px 0 0; font-size: 14px; color: var(--muted); }
  .note.bad { color: var(--bad); }
  .panel footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 14px; font-size: 14px; color: var(--muted); }
  .panel footer .links { flex: none; display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 4px 12px; }
  .panel footer a { flex: none; display: inline-flex; align-items: center; min-height: 32px; color: var(--accent-text); }
  .panel footer a:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 4px; }
  /* A finger gets a larger target, and there is no hover to explain the chips: the panel is one tap away. */
  @media (pointer: coarse) { .strip { min-height: 36px; } .close { width: 44px; height: 44px; } .panel footer a { min-height: 44px; } }
  @media (prefers-reduced-motion: no-preference) { .tip, .panel { transition: opacity .12s ease; } }
</style>
