<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { toolLabel, toolTarget, type ChatState } from '../chat'
  import { gafferProgress, gafferQuote, observedTime, recentToolState, type GafferPhase } from '../gafferProgress'
  import GafferAvatar from './GafferAvatar.svelte'

  let { chat, dir = '', agent = 'Agent', replayed = true, ended = false }: { chat: ChatState; dir?: string; agent?: string; replayed?: boolean; ended?: boolean } = $props()
  const view = $derived(gafferProgress(chat, dir, replayed, ended, agent))
  const needsYou = $derived(view.phase === 'approval' || view.phase === 'question')
  const stopped = $derived(['idle', 'error', 'cancelled', 'ended'].includes(view.phase))
  let paused = $state(false), reduced = $state(false), hidden = $state(false)
  let quote = $state(''), lastPhase: GafferPhase | undefined, quoteAt = 0
  let seconds = $state(0), observedTurn = ''
  let observedAt = $state(0)
  let details = $state(false)

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
      if (Date.now() - quoteAt >= 12_000) { quote = gafferQuote(view.phase, agent, quote); quoteAt = Date.now() }
    }, 1000)
    return () => { clearInterval(timer); media.removeEventListener('change', motion); document.removeEventListener('visibilitychange', visibility) }
  })
</script>

{#if view.visible}
  <section class="gaffer-progress" class:needs-you={needsYou} class:problem={view.phase === 'error' || view.phase === 'reconnecting'} class:compact={stopped} class:paused={paused || hidden} aria-label="Gaffer’s work update" data-phase={view.phase}>
    <div class="gaffer-main">
      <div class="portrait"><GafferAvatar active={view.active} paused={paused || hidden} /></div>
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
  .gaffer-progress { --tone: var(--accent-text); flex: none; position: relative; margin: 0 14px 10px; overflow: hidden; border: 1px solid color-mix(in srgb, var(--tone) 27%, var(--line)); border-radius: 13px; background: radial-gradient(ellipse at 0 0, color-mix(in srgb, var(--tone) 7%, transparent), transparent 65%), var(--card); box-shadow: 0 3px 14px color-mix(in srgb, var(--ink) 3%, transparent); container: gaffer / inline-size; }
  .needs-you { --tone: var(--warn); }
  .problem { --tone: var(--bad); }
  .gaffer-main { display: flex; align-items: center; gap: 13px; padding: 14px 12px 11px 14px; }
  .portrait { width: 64px; height: 64px; flex: none; }
  .brief { flex: 1; min-width: 0; }
  .byline { display: flex; align-items: baseline; gap: 9px; color: var(--muted); font: 10px/1.3 var(--sans); }
  .byline > strong { color: var(--ink); font: 600 16px/1.2 var(--serif); }
  .role { font-size: 9px; letter-spacing: .09em; text-transform: uppercase; color: var(--tone); }
  .agent { margin-left: auto; font-family: var(--mono); }
  .live-update { display: flex; align-items: center; gap: 7px; min-width: 0; margin-top: 8px; font: 12px/1.5 var(--sans); }
  .live-update > strong { font-weight: 500; color: var(--ink); flex: none; }
  .target { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; color: var(--muted); font: 11px/1.5 var(--mono); }
  .target::before { content: '· '; }
  .signal { display: inline-flex; gap: 2px; align-items: center; justify-content: center; width: 14px; height: 14px; flex: none; color: var(--tone); font: 600 12px var(--sans); }
  .signal i { width: 2px; height: 4px; border-radius: 2px; background: currentColor; }
  .signal.live i { animation: graft 1.3s ease-in-out infinite; }
  .signal.live i:nth-child(2) { animation-delay: .15s; }
  .signal.live i:nth-child(3) { animation-delay: .3s; }
  .quote { margin: 5px 0 0; color: var(--ink-soft); font: italic 12px/1.5 var(--sans); overflow-wrap: anywhere; }
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
  .compact .portrait { width: 46px; height: 46px; }
  .compact .quote, .compact .role, .compact .pause { display: none; }
  .compact .live-update { margin-top: 4px; }
  .paused .signal i, .paused .activity-line > span { animation-play-state: paused; }
  .vh { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }
  @keyframes graft { 50% { height: 11px; } }
  @keyframes sweep { from { transform: translateX(-110%); } to { transform: translateX(440%); } }
  @container gaffer (max-width: 540px) {
    .gaffer-main { gap: 9px; padding: 11px 10px; }
    .portrait { width: 50px; height: 50px; align-self: flex-start; }
    .agent { display: none; }
    .live-update { flex-wrap: wrap; gap: 2px 6px; margin-top: 6px; }
    .live-update > strong { flex: 1; min-width: 0; }
    .target { flex-basis: 100%; padding-left: 20px; }
    .target::before { content: ''; }
    .quote { font-size: 11.5px; min-height: 3em; }
    .foot { padding-inline: 10px; gap: 2px 8px; }
    .supervising { display: none; }
    .activity { margin-left: auto; }
    .quiet { min-height: 40px; }
  }
  @media (prefers-reduced-motion: reduce) { .signal.live i, .activity-line > span { animation: none; } .activity-line > span { width: 100%; opacity: .35; } }
</style>
