<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { toolLabel, toolState, toolTarget, type ChatState } from '../chat'
  import { gafferProgress, gafferQuote, observedTime, recentToolState, type GafferPhase } from '../gafferProgress'
  import { gafferSummary } from '../gafferSummary'
  import GafferAvatar from './GafferAvatar.svelte'

  let { chat, dir = '', agent = 'Agent', replayed = true, ended = false }: { chat: ChatState; dir?: string; agent?: string; replayed?: boolean; ended?: boolean } = $props()
  const view = $derived(gafferProgress(chat, dir, replayed, ended, agent))
  const summary = $derived(gafferSummary(chat, dir, replayed, ended))
  const needsYou = $derived(view.phase === 'approval' || view.phase === 'question')
  const stopped = $derived(['idle', 'error', 'cancelled', 'ended'].includes(view.phase))
  let paused = $state(false), reduced = $state(false), hidden = $state(false)
  let quote = $state(''), lastPhase: GafferPhase | undefined, quoteAt = 0
  let seconds = $state(0), observedTurn = ''
  let observedAt = $state(0)
  let details = $state(false)
  let summaryExpanded = $state(true)
  let summaryKey = ''

  $effect(() => {
    const key = summary ? `${summary.scope}:${view.turnKey}` : ''
    if (key !== summaryKey) { summaryKey = key; summaryExpanded = true }
  })

  $effect(() => {
    const phase = view.phase, turn = view.turnKey, visible = view.visible, isPaused = paused
    const confirmed = replayed && chat.connection === 'open'
    untrack(() => {
      // A stream reset briefly removes the prompt while history replays. Only a confirmed new prompt starts a new timer.
      if (confirmed && turn !== 'unavailable' && turn !== observedTurn) { observedTurn = turn; observedAt = 0; seconds = 0; details = false }
      if (phase !== lastPhase && !isPaused) { quote = gafferQuote(phase, agent, quote); lastPhase = phase; quoteAt = Date.now() }
      if (visible && view.active && !observedAt) observedAt = Date.now()
    })
  })
  onMount(() => {
    const media = matchMedia('(prefers-reduced-motion: reduce)')
    const motion = () => { reduced = media.matches }
    const visibility = () => { hidden = document.hidden }
    motion(); visibility()
    media.addEventListener('change', motion)
    document.addEventListener('visibilitychange', visibility)
    const timer = setInterval(() => {
      if (view.visible && !stopped && observedAt) seconds = Math.floor((Date.now() - observedAt) / 1000)
      if (!view.active || paused || reduced || hidden) { quoteAt = Date.now(); return }
      if (Date.now() - quoteAt >= 8_000) { quote = gafferQuote(view.phase, agent, quote); quoteAt = Date.now() }
    }, 1000)
    return () => { clearInterval(timer); media.removeEventListener('change', motion); document.removeEventListener('visibilitychange', visibility) }
  })
</script>

{#if view.visible}
  <section class="gaffer-progress" class:needs-you={needsYou} class:problem={view.phase === 'error' || view.phase === 'reconnecting'} class:compact={stopped} class:paused={paused || hidden} aria-label="Gaffer’s work update" data-phase={view.phase}>
    <div class="gaffer-main">
      <div class="portrait"><GafferAvatar active={view.active} phase={view.phase} paused={paused || hidden} /></div>
      <div class="brief">
        <div class="byline"><strong>Gaffer</strong><span class="role">The boss</span><span class="agent">{agent}</span></div>
        <div class="live-update" role="status" aria-live="polite" aria-atomic="true">
          <span class="signal" class:live={view.active} aria-hidden="true">{#if needsYou}?{:else if view.phase === 'idle'}✓{:else if view.phase === 'error'}!{:else}<i></i><i></i><i></i>{/if}</span>
          <strong>{view.label}</strong>
          {#if view.target}<span class="target" title={view.target}>{view.target}</span>{/if}
        </div>
        <p class="quote" aria-live="off"><span aria-hidden="true">“</span>{quote}<span aria-hidden="true">”</span></p>
      </div>
      <button type="button" class="quiet pause" aria-label={paused ? 'Resume Gaffer animation and quotes' : 'Pause Gaffer animation and quotes'} title={paused ? 'Resume animation and quotes' : 'Pause animation and quotes'} aria-pressed={paused} onclick={() => paused = !paused}>
        <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">{#if paused}<path d="m7 4 9 6-9 6z" />{:else}<rect x="5" y="4" width="3" height="12" rx="1" /><rect x="12" y="4" width="3" height="12" rx="1" />{/if}</svg>
      </button>
    </div>
    <div class="foot">
      <span class="supervising"><span aria-hidden="true">↳</span> {needsYou ? 'Waiting for your say-so' : view.phase === 'reconnecting' ? 'Waiting for a live connection' : stopped ? 'Over to you' : 'Gaffer’s keeping an eye on it'}</span>
      {#if observedAt}<span class="elapsed" title="Time since this view first observed the agent active; includes waiting. Earlier work may have happened before you opened it." aria-live="off">{observedTime(seconds)} in view</span>{/if}
      {#if view.recent.length}
        <button type="button" class="quiet activity" aria-expanded={details} onclick={() => details = !details}>
          {view.phase === 'reconnecting' ? 'Last seen: ' : view.partial ? 'Recent: ' : ''}{view.completed} done{#if view.running} · {view.running} active{/if}{#if view.failed} · {view.failed} failed{/if}
          <svg class:open={details} viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
          <span class="vh"> · Tool activity</span>
        </button>
      {/if}
    </div>
    {#if summary}
      <section class="executive-summary" aria-label="Gaffer’s executive summary">
        <div class="summary-heading"><h3>Executive summary</h3><span>{summary.scope === 'session' ? 'Whole session' : 'This turn'}</span><button type="button" class="quiet" aria-label={summaryExpanded ? 'Collapse executive summary' : 'Expand executive summary'} aria-expanded={summaryExpanded} onclick={() => summaryExpanded = !summaryExpanded}>{summaryExpanded ? 'Hide' : 'Show'}</button></div>
        {#if summaryExpanded}
        {#if summary.partial}<p class="summary-partial">Based on retained activity. Earlier history is unavailable, so this summary may be incomplete.</p>{/if}
        <p class="handover"><strong>{summary.handover ? `${agent}’s handover` : 'Handover'}</strong>{summary.handover || 'No final handover was recorded. Review the conversation before treating the job as complete.'}</p>
        {#if summary.attention.length}<div class="summary-attention"><strong>Needs a look</strong>{#each summary.attention as issue}<p>{issue}</p>{/each}</div>{/if}
        <p class="summary-next"><strong>Next</strong>{summary.next}</p>
        <div class="summary-counts" aria-label="Recorded tool results">
          <span>{summary.completed} tool call{summary.completed === 1 ? '' : 's'} completed</span>
          {#if summary.failed}<span class="snag">{summary.failed} failed</span>{/if}
          {#if summary.stopped}<span class="snag">{summary.stopped} unfinished</span>{/if}
        </div>
        {#if summary.files.length}
          <div class="summary-files"><strong>Files touched</strong>{#each summary.files.slice(0, 3) as file}<code>{file}</code>{/each}{#if summary.files.length > 3}<span>+{summary.files.length - 3} more in the conversation</span>{/if}</div>
        {/if}
        {#if summary.commands.length}
          <details class="summary-commands"><summary>Commands & results ({summary.commands.length})</summary>
            <p>Recorded command status; see the conversation for output and test results.</p>
            <ul>{#each summary.commands as command (command.key)}<li><code>{command.target}</code><span class:snag={command.status !== 'completed'}>{toolState(command.status).word}</span></li>{/each}</ul>
          </details>
        {/if}
        {/if}
      </section>
    {/if}
    {#if details && view.recent.length}
      <div class="activity-detail">
        <p>Latest tool calls this turn{view.partial ? ' · earlier activity may be unavailable' : ''}. Full output is in the conversation.</p>
        <ol>
          {#each view.recent as item (item.key)}
            <li><span class="kind">{toolLabel(item.toolKind)}</span><span class="activity-target" title={toolTarget(item, dir)}>{toolTarget(item, dir)}</span><span class="tool-state" class:bad={item.status === 'failed'}>{recentToolState(item)}</span></li>
          {/each}
        </ol>
      </div>
    {/if}
    {#if view.active}<div class="activity-line" aria-hidden="true"><span></span></div>{/if}
  </section>
{/if}

<style>
  .gaffer-progress { --tone: var(--accent-text); flex: none; position: relative; margin: 0 14px 10px; overflow: hidden; border: 1px solid var(--line); border-radius: 9px; background: var(--card); container: gaffer / inline-size; }
  .needs-you { --tone: var(--warn); }
  .problem { --tone: var(--bad); }
  .gaffer-main { display: flex; align-items: center; gap: 10px; padding: 11px 12px; }
  .portrait { width: 48px; height: 48px; flex: none; }
  .brief { flex: 1; min-width: 0; }
  .byline { display: flex; align-items: baseline; gap: 9px; color: var(--muted); font: 10px/1.3 var(--sans); }
  .byline > strong { color: var(--ink); font: 600 12px/1.3 var(--mono); }
  .role { font-size: 8px; letter-spacing: .07em; text-transform: uppercase; color: var(--muted); }
  .agent { margin-left: auto; font-family: var(--mono); }
  .live-update { display: flex; align-items: center; gap: 7px; min-width: 0; margin-top: 4px; font: 12px/1.5 var(--sans); }
  .live-update > strong { font-weight: 500; color: var(--ink); flex: none; }
  .target { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; color: var(--muted); font: 11px/1.5 var(--mono); }
  .target::before { content: '· '; }
  .signal { display: inline-flex; gap: 2px; align-items: center; justify-content: center; width: 14px; height: 14px; flex: none; color: var(--tone); font: 600 12px var(--sans); }
  .signal i { width: 2px; height: 4px; border-radius: 2px; background: currentColor; }
  .signal.live i { animation: graft 1.3s ease-in-out infinite; }
  .signal.live i:nth-child(2) { animation-delay: .15s; }
  .signal.live i:nth-child(3) { animation-delay: .3s; }
  .quote { margin: 4px 0 0; color: var(--ink-soft); font: 11px/1.5 var(--sans); overflow-wrap: anywhere; }
  .quote > span { color: var(--tone); }
  .quiet { display: inline-flex; align-items: center; justify-content: center; flex: none; gap: 5px; border: 0; border-radius: 5px; padding: 0 6px; background: none; color: var(--muted); cursor: pointer; font: 11px var(--sans); }
  .quiet:hover { color: var(--ink); background: var(--line-soft); }
  .quiet:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 1px; }
  .pause { width: 40px; height: 40px; align-self: flex-start; margin: -6px -4px 0 0; }
  .pause svg { width: 13px; height: 13px; }
  .foot { display: flex; flex-wrap: wrap; align-items: center; gap: 3px 12px; min-height: 35px; padding: 3px 12px 3px 15px; border-top: 1px solid color-mix(in srgb, var(--tone) 10%, var(--line-soft)); background: color-mix(in srgb, var(--bg-sunk) 35%, transparent); color: var(--muted); font: 10px/1.5 var(--sans); }
  .supervising { margin-right: auto; }
  .supervising > span { margin-right: 4px; color: var(--tone); }
  .elapsed { font: 10px var(--mono); font-variant-numeric: tabular-nums; }
  .activity { min-height: 28px; }
  .activity > svg { width: 12px; height: 12px; }
  .activity > svg.open { transform: rotate(180deg); }
  .activity-detail { max-height: 150px; overflow: auto; padding: 9px 14px 12px; border-top: 1px solid var(--line-soft); }
  .activity-detail p { margin: 0 0 6px; color: var(--muted); font: 10px/1.5 var(--sans); }
  .activity-detail ol { list-style: none; padding: 0; margin: 0; }
  .activity-detail li { display: flex; align-items: baseline; gap: 10px; padding: 4px 0; font: 11px/1.5 var(--mono); }
  .kind { min-width: 40px; color: var(--muted); font: 500 10px var(--sans); }
  .activity-target { flex: 1; min-width: 0; overflow-wrap: anywhere; color: var(--ink-soft); }
  .tool-state { flex: none; color: var(--muted); font: 11px var(--sans); }
  .tool-state.bad { color: var(--bad); }
  .activity-line { position: absolute; inset: auto 0 0; height: 2px; overflow: hidden; background: color-mix(in srgb, var(--tone) 10%, transparent); }
  .activity-line > span { display: block; height: 100%; width: 30%; background: linear-gradient(90deg, transparent, var(--tone), transparent); animation: sweep 3.8s ease-in-out infinite; }
  .compact .gaffer-main { padding-block: 9px; }
  .compact .portrait { width: 40px; height: 40px; }
  .compact .role { display: none; }
  .compact .live-update { margin-top: 4px; }
  .compact .quote { font-size: 11px; min-height: 0; }
  .executive-summary { max-height: min(280px, 34dvh); overflow: auto; overscroll-behavior: contain; scrollbar-gutter: stable; padding: 12px 15px; border-top: 1px solid var(--line-soft); color: var(--ink-soft); font: 11px/1.6 var(--sans); }
  .summary-heading { display: flex; align-items: baseline; gap: 10px; justify-content: space-between; margin-bottom: 8px; }
  .summary-heading h3 { margin: 0; color: var(--ink); font: 600 14px/1.4 var(--serif); }
  .summary-heading > span { margin-left: auto; color: var(--muted); font: 9px var(--mono); }
  .summary-heading > button { min-height: 40px; min-width: 40px; }
  .handover, .summary-next, .summary-attention p { margin: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
  .handover > strong, .summary-files > strong, .summary-attention > strong { display: block; margin-bottom: 3px; color: var(--muted); font: 500 10px/1.5 var(--sans); }
  .summary-counts { display: flex; flex-wrap: wrap; gap: 4px 12px; margin: 9px 0; color: var(--muted); font-size: 10px; }
  .snag { color: var(--bad); }
  .summary-files { margin: 9px 0; }
  .summary-files code { display: block; overflow-wrap: anywhere; font: 10px/1.8 var(--mono); }
  .summary-files > span { color: var(--muted); font-size: 10px; }
  .summary-commands { margin: 9px 0; }
  .summary-commands summary { display: list-item; min-height: 28px; align-content: center; cursor: pointer; color: var(--ink); }
  .summary-commands summary:focus-visible, .executive-summary:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .summary-commands p { margin: 3px 0 6px; color: var(--muted); font-size: 10px; }
  .summary-commands ul { list-style: none; margin: 0; padding: 0; }
  .summary-commands li { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; padding: 3px 0; }
  .summary-commands code { min-width: 0; overflow-wrap: anywhere; font: 10px/1.6 var(--mono); }
  .summary-commands li > span { flex: none; font-size: 10px; }
  .summary-attention { border-left: 2px solid var(--warn); padding-left: 9px; margin: 9px 0; }
  .summary-attention p + p { margin-top: 4px; }
  .summary-next { margin-top: 9px; padding-top: 8px; border-top: 1px solid var(--line-soft); }
  .summary-next strong { margin-right: 7px; color: var(--tone); }
  .summary-partial { margin: 0 0 7px; color: var(--muted); font-size: 10px; }
  .paused .signal i, .paused .activity-line > span { animation-play-state: paused; }
  .vh { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }
  @keyframes graft { 50% { height: 11px; } }
  @keyframes sweep { from { transform: translateX(-110%); } to { transform: translateX(440%); } }
  @container gaffer (max-width: 540px) {
    .gaffer-main { gap: 9px; padding: 11px 10px; }
    .portrait { width: 44px; height: 44px; align-self: flex-start; }
    .agent { display: none; }
    .live-update { flex-wrap: wrap; gap: 2px 6px; margin-top: 6px; }
    .live-update > strong { flex: 1; min-width: 0; }
    .target { flex-basis: 100%; padding-left: 20px; }
    .target::before { content: ''; }
    .quote { font-size: 11px; }
    .foot { padding-inline: 10px; gap: 2px 8px; }
    .supervising { display: none; }
    .activity { margin-left: auto; }
    .quiet { min-height: 40px; }
    .executive-summary { padding: 10px; max-height: 30dvh; }
    .summary-commands summary { min-height: 40px; }
  }
  @media (prefers-reduced-motion: reduce) { .signal.live i, .activity-line > span { animation: none; } .activity-line > span { width: 100%; opacity: .35; } }
</style>
