<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { toolLabel, toolState, toolTarget, type ChatState } from '../chat'
  import { gafferProgress, gafferQuote, observedTime, recentToolState, type GafferPhase } from '../gafferProgress'
  import { gafferSummary } from '../gafferSummary'
  import { GAFFER_SETTLE_MS, gafferAnimations, gafferCompact, gafferGestureMs } from '../gafferMotion'
  import { GAFFER_AWAY_AFTER, gafferAside, gafferClickAside, gafferClickRun, gafferGesture, gafferPoke, gafferPose, type GafferPoke, type GafferPose, type GafferReaction } from '../gafferPersonality'
  import GafferAvatar from './GafferAvatar.svelte'

  let { chat, sessionId = '', dir = '', agent = 'Agent', replayed = true, ended = false, sessionFailed = false, reattaching = false }: { chat: ChatState; sessionId?: string; dir?: string; agent?: string; replayed?: boolean; ended?: boolean; sessionFailed?: boolean; reattaching?: boolean } = $props()
  const view = $derived(gafferProgress(chat, dir, replayed, ended, agent, sessionFailed, reattaching))
  const summary = $derived(gafferSummary(chat, dir, replayed, ended))
  const needsYou = $derived(view.phase === 'approval' || view.phase === 'question')
  const stopped = $derived(['idle', 'error', 'cancelled', 'ended'].includes(view.phase))
  let paused = $state(false), reduced = $state(false), hidden = $state(false)
  const still = $derived(paused || reduced || hidden || !$gafferAnimations)
  let quote = $state(''), lastPhase: GafferPhase | undefined, quoteAt = 0
  let seconds = $state(0), observedTurn = ''
  let observedAt = $state(0)
  let details = $state(false)
  // The account preference says how a card starts; this card's own button overrides it for as long as the card is open.
  let foldChoice = $state<boolean | null>(null)
  const minimised = $derived(foldChoice ?? $gafferCompact)
  let sheet = $state(false), handoverMore = $state(false)
  let summaryKey = ''
  const NO_HANDOVER = 'No final handover was recorded. Check the conversation before calling the job done.'
  const handoverText = $derived(summary ? summary.handover || NO_HANDOVER : '')
  let said = $state<HTMLElement>(), clamped = $state(false)
  const longHandover = $derived(handoverMore || clamped)
  const verdict = $derived(!summary ? '' : summary.failed || summary.attention.length ? 'Needs a look' : stopped ? 'Over to you' : '')
  let reaction = $state<GafferReaction>(''), reactionKey = $state(0)
  let welcomePose = $state<GafferPose | null>(null), pendingReturn = $state(false)
  const pose = $derived(needsYou || gafferPose(view.phase) === 'concerned' ? gafferPose(view.phase) : welcomePose ?? gafferPose(view.phase))
  let welcomeTimer: ReturnType<typeof setTimeout> | undefined
  let gestureTimer: ReturnType<typeof setTimeout> | undefined, asideTimer: ReturnType<typeof setTimeout> | undefined
  let lastGestureAt = -Infinity, nextGestureAt = 0, gestureEndsAt = 0
  let clicks = $state({ count: 0, at: -Infinity }), clickAside = $state('')
  const miniQuote = $derived(clickAside || (summary ? handoverText : quote))
  // A nudge is answered by the pose he is already in; pokeKey restarts the acknowledgement even for the same kind twice.
  let pokeKind = $state<GafferPoke | ''>(''), pokeKey = $state(0)

  function gesture(next: GafferReaction) {
    if (still) return
    clearTimeout(gestureTimer)
    reaction = next; reactionKey++; lastGestureAt = Date.now()
    const lifetime = gafferGestureMs(next) + GAFFER_SETTLE_MS
    gestureEndsAt = Date.now() + lifetime
    nextGestureAt = gestureEndsAt + 18_000 + Math.random() * 14_000
    gestureTimer = setTimeout(() => { reaction = ''; gestureEndsAt = 0 }, lifetime)
  }
  function stopGesture() {
    clearTimeout(gestureTimer); gestureEndsAt = 0; reaction = ''
  }
  function clearAside() {
    clearTimeout(asideTimer); clickAside = ''
  }
  function cancelWelcome() {
    clearTimeout(welcomeTimer); welcomePose = null
  }
  function welcome() {
    pendingReturn = false
    stopGesture(); clearAside()
    const resting = gafferPose(view.phase, true)
    quote = gafferAside('return', resting, agent, quote); quoteAt = Date.now()
    if ((resting === 'tea' || resting === 'asleep') && !still) {
      welcomePose = resting
      // Let the sip finish and briefly rest before he notices the user. A doze still wakes promptly.
      const welcomeDelay = resting === 'tea' ? gafferGestureMs('sip') + GAFFER_SETTLE_MS : 900
      welcomeTimer = setTimeout(() => {
        welcomePose = null; gesture(resting === 'asleep' ? 'stretch' : 'startle')
      }, welcomeDelay)
    } else gesture(gafferGesture(view.phase))
  }
  function greet() {
    const click = gafferClickRun(clicks, Date.now())
    if (!click) return
    clicks = click
    const resting = pose
    clickAside = gafferClickAside(resting, agent, click.count, clickAside || quote); quoteAt = Date.now()
    clearTimeout(asideTimer); asideTimer = setTimeout(() => { clickAside = '' }, 12_000)
    // The quip responds immediately, even with motion off. The body answers too, in whatever scene it is already playing.
    pokeKind = gafferPoke(click.count); pokeKey++
  }

  // Offer Read more only when the three-line clamp really hides text; re-measure when the pane resizes.
  $effect(() => {
    const el = said
    if (!el || handoverMore) return
    const measure = () => { clamped = el.scrollHeight - el.clientHeight > 1 }
    measure()
    const watcher = new ResizeObserver(measure); watcher.observe(el)
    return () => watcher.disconnect()
  })
  $effect(() => {
    const key = summary ? `${summary.scope}:${view.turnKey}` : ''
    if (key !== summaryKey) { summaryKey = key; sheet = false; handoverMore = false }
  })
  $effect(() => {
    const phase = view.phase, turn = view.turnKey, visible = view.visible
    const confirmed = replayed && chat.connection === 'open', returning = pendingReturn, isHidden = hidden
    untrack(() => {
      // Stream replay can briefly remove the prompt. Only a confirmed new prompt restarts the timer.
      if (confirmed && turn !== 'unavailable' && turn !== observedTurn) { observedTurn = turn; observedAt = 0; seconds = 0; details = false }
      if (phase !== lastPhase) {
        cancelWelcome(); stopGesture(); clearAside()
        quote = gafferQuote(phase, agent, quote); lastPhase = phase; quoteAt = Date.now()
        if (visible && (needsYou || phase === 'error' || Date.now() - lastGestureAt >= 5_000)) gesture(gafferGesture(phase))
        else reaction = ''
      }
      if (visible && view.active && !observedAt) observedAt = Date.now()
      // Never infer completion or a tea break while history is still replaying.
      if (returning && confirmed && visible && !isHidden) welcome()
    })
  })
  $effect(() => { if (still) untrack(() => { cancelWelcome(); stopGesture() }) })

  onMount(() => {
    const media = matchMedia('(prefers-reduced-motion: reduce)')
    const storageKey = sessionId ? `reet-gaffer-away:${sessionId}` : ''
    let awayAt = 0
    try {
      const previous = storageKey ? Number(sessionStorage.getItem(storageKey)) : 0
      pendingReturn = previous > 0 && Date.now() - previous >= GAFFER_AWAY_AFTER
      if (storageKey) sessionStorage.removeItem(storageKey)
    } catch { /* Private browsing can deny storage; tab visibility still works. */ }
    const motion = () => { reduced = media.matches }
    const leave = () => {
      hidden = true
      if (!awayAt) awayAt = Date.now()
      cancelWelcome(); stopGesture(); clearAside()
      try { if (storageKey && view.visible) sessionStorage.setItem(storageKey, String(awayAt)) } catch { /* optional decoration */ }
    }
    const arrive = () => {
      if (document.hidden) return
      hidden = false
      if (awayAt && Date.now() - awayAt >= GAFFER_AWAY_AFTER) pendingReturn = true
      awayAt = 0; nextGestureAt = Date.now() + 18_000
      try { if (storageKey) sessionStorage.removeItem(storageKey) } catch { /* optional decoration */ }
    }
    const visibility = () => { if (document.hidden) leave(); else arrive() }
    motion(); visibility()
    media.addEventListener('change', motion)
    document.addEventListener('visibilitychange', visibility)
    window.addEventListener('blur', leave); window.addEventListener('focus', arrive)
    window.addEventListener('pagehide', leave)
    nextGestureAt = Date.now() + 18_000 + Math.random() * 14_000
    const timer = setInterval(() => {
      if (view.visible && !stopped && observedAt) seconds = Math.floor((Date.now() - observedAt) / 1000)
      if (hidden || !view.visible) return
      // Banter and live facts continue when animation is disabled, including reduced motion.
      if (view.active && Date.now() - quoteAt >= 30_000) { quote = gafferQuote(view.phase, agent, quote); quoteAt = Date.now() }
      if (!still && view.active && !welcomePose && Date.now() >= nextGestureAt) gesture(gafferGesture(view.phase))
    }, 1000)
    return () => {
      leave(); clearInterval(timer); cancelWelcome(); stopGesture(); clearAside()
      media.removeEventListener('change', motion); document.removeEventListener('visibilitychange', visibility)
      window.removeEventListener('blur', leave); window.removeEventListener('focus', arrive); window.removeEventListener('pagehide', leave)
    }
  })
</script>

{#if view.visible && minimised}
  <section class="gaffer-progress mini" class:needs-you={needsYou} class:problem={view.phase === 'error' || view.phase === 'reconnecting'} class:paused={still} aria-label="Gaffer’s work update" data-phase={view.phase} data-mode="minimised" data-poke-count={clicks.count}>
    <button type="button" class="mini-portrait" aria-label="Say ey up to Gaffer" title="Say ey up. Give him another nudge; he’s got opinions." onclick={greet}><GafferAvatar variant="bust" active={view.active} phase={view.phase} {pose} {reaction} {reactionKey} {pokeKind} {pokeKey} paused={still} /></button>
    <div class="mini-status" role="status" aria-live="polite" aria-atomic="true">
      <span class="signal" class:live={view.active} aria-hidden="true">{#if needsYou}?{:else if view.phase === 'idle'}✓{:else if view.phase === 'ended' || view.phase === 'cancelled'}<i class="sq"></i>{:else if view.phase === 'error' || view.phase === 'reconnecting'}!{:else}<i></i><i></i><i></i>{/if}</span>
      <strong>{view.label}</strong>
    </div>
    <p class="mini-quote" title={miniQuote} aria-live={clickAside ? 'polite' : 'off'}><span aria-hidden="true">“</span>{miniQuote}<span aria-hidden="true">”</span></p>
    {#if summary}<span class="mini-count" class:snag={summary.failed || summary.stopped} title="Recorded tool results">{summary.completed} done{#if summary.failed} · {summary.failed} failed{/if}{#if summary.stopped} · {summary.stopped} unfinished{/if}</span>{/if}
    <button type="button" class="quiet fold" aria-label="Expand Gaffer’s card" aria-expanded="false" title="Show the full card" onclick={() => foldChoice = false}>
      <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
    </button>
    {#if view.active}<div class="activity-line" aria-hidden="true"><span></span></div>{/if}
  </section>
{:else if view.visible}
  <section class="gaffer-progress" class:needs-you={needsYou} class:problem={view.phase === 'error' || view.phase === 'reconnecting'} class:compact={stopped} class:paused={still} aria-label="Gaffer’s work update" data-phase={view.phase} data-poke-count={clicks.count}>
    <div class="gaffer-main">
      <button type="button" class="portrait" aria-label="Say ey up to Gaffer" title="Say ey up. Give him another nudge; he’s got opinions." onclick={greet}><GafferAvatar variant="bust" active={view.active} phase={view.phase} {pose} {reaction} {reactionKey} {pokeKind} {pokeKey} paused={still} /></button>
      <div class="brief">
        <div class="byline"><strong>Gaffer</strong><span class="role">The boss</span><span class="agent">{agent}</span></div>
        <div class="live-update" role="status" aria-live="polite" aria-atomic="true">
          <span class="signal" class:live={view.active} aria-hidden="true">{#if needsYou}?{:else if view.phase === 'idle'}✓{:else if view.phase === 'ended' || view.phase === 'cancelled'}<i class="sq"></i>{:else if view.phase === 'error' || view.phase === 'reconnecting'}!{:else}<i></i><i></i><i></i>{/if}</span>
          <strong>{view.label}</strong>
          {#if view.target}<span class="target" title={view.target}>{view.target}</span>{/if}
        </div>
        {#if summary}
          <div class="handover" class:open={handoverMore}>
            {#if summary.handover}<span class="lead">{agent}’s handover, via Gaffer</span>{/if}
            <p class="said" bind:this={said} role="status" aria-live="polite" aria-atomic="true"><span aria-hidden="true">“</span>{handoverText}<span aria-hidden="true">”</span></p>
            {#if longHandover}<button type="button" class="more" aria-expanded={handoverMore} onclick={() => handoverMore = !handoverMore}>{handoverMore ? 'Show less' : 'Read more'}</button>{/if}
          </div>
        {:else}
          <p class="quote" role={clickAside ? 'status' : undefined} aria-live={clickAside ? 'polite' : 'off'}><span aria-hidden="true">“</span>{clickAside || quote}<span aria-hidden="true">”</span></p>
        {/if}
        {#if summary && clickAside}<p class="quote click-aside" role="status" aria-live="polite"><span aria-hidden="true">“</span>{clickAside}<span aria-hidden="true">”</span></p>{/if}
      </div>
      <div class="controls">
      <button type="button" class="quiet fold" aria-label="Minimise Gaffer’s card to one line" aria-expanded="true" title="Minimise to one line; Gaffer keeps talking" onclick={() => foldChoice = true}>
        <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m4 10 4-4 4 4" /></svg>
      </button>
      <button type="button" class="quiet pause" aria-label={paused ? 'Resume Gaffer animation' : 'Pause Gaffer animation'} title={!$gafferAnimations ? 'Animation is off in Account → Profile' : reduced ? 'Your device requests reduced motion' : paused ? 'Resume animation' : 'Pause animation; quotes and updates continue'} aria-pressed={paused || !$gafferAnimations || reduced} disabled={!$gafferAnimations || reduced} onclick={() => paused = !paused}>
        <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">{#if paused}<path d="m7 4 9 6-9 6z" />{:else}<rect x="5" y="4" width="3" height="12" rx="1" /><rect x="12" y="4" width="3" height="12" rx="1" />{/if}</svg>
      </button>
      </div>
    </div>
    {#if summary}
      <ul class="chips" aria-label="Recorded tool results">
        <li class="chip ok" title="Tool calls completed">{summary.completed} done</li>
        {#if summary.failed}<li class="chip bad">{summary.failed} failed</li>{/if}
        {#if summary.stopped}<li class="chip warn">{summary.stopped} unfinished</li>{/if}
        {#if summary.files.length}<li class="chip" title="Files changed">{summary.files.length} file{summary.files.length === 1 ? '' : 's'}</li>{/if}
      </ul>
    {/if}
    <div class="foot">
      <span class="supervising"><span aria-hidden="true">↳</span> {needsYou ? 'Waiting for your say-so' : view.phase === 'reconnecting' ? 'Waiting for a live connection' : verdict || (stopped ? 'Over to you' : 'Gaffer’s keeping an eye on it')}</span>
      {#if observedAt}<span class="elapsed" title="Time since this view first observed the agent active; includes waiting. Earlier work may have happened before you opened it." aria-live="off">{observedTime(seconds)} in view</span>{/if}
      {#if summary}
        <button type="button" class="quiet activity" aria-expanded={sheet} aria-controls="job-sheet-{sessionId || 'x'}" onclick={() => sheet = !sheet}>
          Job sheet
          <svg class:open={sheet} viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
        </button>
      {:else if view.recent.length}
        <button type="button" class="quiet activity" aria-expanded={details} onclick={() => details = !details}>
          {view.phase === 'reconnecting' ? 'Last seen: ' : view.partial ? 'Recent: ' : ''}{view.completed} done{#if view.running} · {view.running} active{/if}{#if view.failed} · {view.failed} failed{/if}
          <svg class:open={details} viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
          <span class="vh"> · Tool activity</span>
        </button>
      {/if}
    </div>
    {#if summary && sheet}
      <div class="sheet" id="job-sheet-{sessionId || 'x'}" role="region" aria-label="Gaffer’s job sheet">
        <p class="sheet-scope">{summary.scope === 'session' ? 'Whole session' : 'This turn'}{#if summary.partial} · Based on retained activity. Earlier history is unavailable, so this may be incomplete.{/if}</p>
        {#if summary.attention.length}<div class="sheet-block attention"><h4>Needs a look</h4>{#each summary.attention as issue}<p>{issue}</p>{/each}</div>{/if}
        <div class="sheet-block next"><h4>Next</h4><p>{summary.next}</p></div>
        {#if summary.files.length}<div class="sheet-block"><h4>Files touched</h4>{#each summary.files as file}<code>{file}</code>{/each}</div>{/if}
        {#if summary.commands.length}
          <div class="sheet-block"><h4>Commands &amp; results ({summary.commands.length})</h4>
            <p class="hint">Recorded command status; see the conversation for output and test results.</p>
            <ul>{#each summary.commands as command (command.key)}<li><code>{command.target}</code><span class:snag={command.status !== 'completed'}>{toolState(command.status).word}</span></li>{/each}</ul>
          </div>
        {/if}
      </div>
    {/if}
    {#if !summary && details && view.recent.length}
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
  .portrait { width: 64px; height: 64px; flex: none; border: 0; padding: 0; background: none; cursor: pointer; border-radius: 8px; }
  .portrait:hover { background: var(--line-soft); }
  .portrait:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .pause:disabled { opacity: .55; cursor: default; }
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
  .signal i.sq { width: 8px; height: 8px; border-radius: 2px; }
  .signal.live i { height: 8px; }
  .signal.live i:nth-child(2) { height: 11px; }
  .signal.live i:nth-child(3) { height: 5px; }
  .quote { margin: 4px 0 0; color: var(--ink-soft); font: 11px/1.5 var(--sans); overflow-wrap: anywhere; }
  .quote > span { color: var(--tone); }
  .quiet { display: inline-flex; align-items: center; justify-content: center; flex: none; gap: 5px; border: 0; border-radius: 5px; padding: 0 6px; background: none; color: var(--muted); cursor: pointer; font: 11px var(--sans); }
  .quiet:hover { color: var(--ink); background: var(--line-soft); }
  .quiet:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 1px; }
  .controls { display: flex; align-self: flex-start; margin: -6px -4px 0 0; }
  .pause, .fold { width: 40px; height: 40px; }
  .fold svg { width: 14px; height: 14px; }
  /* Minimised: one 40px line. The portrait, status, Gaffer's own words and the tally all stay; the chips, job sheet and byline fold away. */
  .mini { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 5px 4px 5px 8px; }
  .mini.needs-you { border-left: 3px solid var(--warn); padding-left: 6px; background: color-mix(in srgb, var(--warn) 5%, var(--card)); }
  .mini.problem { border-left: 3px solid var(--bad); padding-left: 6px; }
  .mini-portrait { width: 32px; height: 32px; flex: none; border: 0; padding: 0; background: none; border-radius: 7px; cursor: pointer; }
  .mini-portrait:hover { background: var(--line-soft); }
  .mini-portrait:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 2px; }
  .mini-status { display: flex; align-items: center; gap: 7px; flex: none; max-width: 40%; font: 12px/1.5 var(--sans); }
  .mini-status > strong { font-weight: 500; color: var(--ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mini-quote { flex: 1; min-width: 0; margin: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ink-soft); font: 11px/1.5 var(--sans); }
  .mini-quote > span { color: var(--tone); }
  .mini-count { flex: none; padding: 1px 9px; border: 1px solid var(--line); border-radius: 999px; color: var(--good); font: 10.5px/1.7 var(--sans); font-variant-numeric: tabular-nums; white-space: nowrap; background: color-mix(in srgb, var(--bg-sunk) 50%, transparent); }
  .mini-count.snag { color: var(--warn); border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); }
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
  .activity-line > span { display: block; height: 100%; width: 100%; opacity: .35; background: linear-gradient(90deg, transparent, var(--tone), transparent); }
  .compact .gaffer-main { padding-block: 9px; }
  .compact .role { display: none; }
  .compact .live-update { margin-top: 4px; }
  .compact .quote { font-size: 11px; min-height: 0; }
  .handover { margin-top: 4px; }
  .lead { display: block; margin-bottom: 1px; color: var(--muted); font: 10px/1.5 var(--sans); }
  .said { margin: 0; color: var(--ink-soft); font: 11px/1.55 var(--sans); overflow-wrap: anywhere; white-space: pre-wrap; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 3; line-clamp: 3; overflow: hidden; }
  .handover.open .said { display: block; -webkit-line-clamp: unset; line-clamp: unset; max-height: min(160px, 24dvh); overflow: auto; overscroll-behavior: contain; }
  .said > span { color: var(--tone); }
  .more { border: 0; background: none; padding: 0 2px; margin: 0 0 -2px -2px; min-height: 28px; color: var(--tone); font: 11px var(--sans); cursor: pointer; border-radius: 4px; }
  .more:hover { text-decoration: underline; }
  .more:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 1px; }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; list-style: none; margin: 0; padding: 0 12px 10px 74px; }
  .chip { border: 1px solid var(--line); border-radius: 999px; padding: 1px 9px; color: var(--ink-soft); font: 10.5px/1.7 var(--sans); background: color-mix(in srgb, var(--bg-sunk) 50%, transparent); font-variant-numeric: tabular-nums; }
  .chip.ok { color: var(--good); }
  .chip.warn { color: var(--warn); border-color: color-mix(in srgb, var(--warn) 40%, var(--line)); }
  .chip.bad { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 40%, var(--line)); }
  .sheet { max-height: min(240px, 40dvh); overflow: auto; overscroll-behavior: contain; scrollbar-gutter: stable; padding: 11px 15px 13px; border-top: 1px solid var(--line-soft); color: var(--ink-soft); font: 11px/1.6 var(--sans); }
  .sheet-scope { margin: 0 0 8px; color: var(--muted); font-size: 10px; }
  .sheet-block { margin: 0 0 10px; }
  .sheet-block:last-child { margin-bottom: 0; }
  .sheet-block h4 { margin: 0 0 3px; color: var(--muted); font: 500 10px/1.5 var(--sans); letter-spacing: .03em; }
  .sheet-block p { margin: 0; overflow-wrap: anywhere; white-space: pre-wrap; }
  .sheet-block code { display: block; overflow-wrap: anywhere; font: 10px/1.8 var(--mono); }
  .sheet-block .hint { margin-bottom: 4px; color: var(--muted); font-size: 10px; }
  .sheet-block ul { list-style: none; margin: 0; padding: 0; }
  .sheet-block li { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; padding: 2px 0; }
  .sheet-block li > span { flex: none; font-size: 10px; color: var(--muted); }
  .sheet-block li > span.snag { color: var(--bad); }
  .sheet-block.attention { border-left: 2px solid var(--warn); padding-left: 9px; }
  .sheet-block.next { border-left: 2px solid var(--tone); padding-left: 9px; }
  .snag { color: var(--bad); }
  .paused .signal i, .paused .activity-line > span { animation-play-state: paused; }
  .vh { position: absolute; width: 1px; height: 1px; margin: -1px; padding: 0; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }
  @container gaffer (max-width: 540px) {
    .gaffer-main { gap: 9px; padding: 11px 10px; }
    .portrait { width: 48px; height: 48px; align-self: flex-start; }
    .agent { display: none; }
    .live-update { flex-wrap: wrap; gap: 2px 6px; margin-top: 6px; }
    .live-update > strong { flex: 1; min-width: 0; }
    .target { flex-basis: 100%; padding-left: 20px; }
    .target::before { content: ''; }
    .quote { font-size: 11px; }
    .mini { gap: 8px; }
    .mini-count { display: none; }
    .mini-status { max-width: 46%; }
    .foot { padding-inline: 10px; gap: 2px 8px; }
    .supervising { display: none; }
    .activity { margin-left: auto; }
    .quiet { min-height: 40px; }
    .chips { padding-left: 10px; }
    .more { min-height: 40px; margin-block: -6px -8px; }
    .sheet { padding: 10px; }
  }
  @media (prefers-reduced-motion: reduce) { .signal.live i, .activity-line > span { animation: none; } .activity-line > span { width: 100%; opacity: .35; } }
</style>
