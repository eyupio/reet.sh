<script lang="ts">
  // Usage: for each agent, how much of each signed-in plan is left and what its sessions reported using in a period;
  // Gaffer's own model requests; and the sessions behind the figures. Every agent always has a block, and one that has
  // nothing to show says why in a sentence, so the page is never a silent blank.
  import { onMount, untrack } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import Skeleton from './Skeleton.svelte'
  import AgentMark from './AgentMark.svelte'
  import { UnauthorizedError } from './api'
  import { readAgo } from './agentAccounts'
  import { machinePlans } from './planLimits'
  import { formatWhen } from './time'
  import { terminalMachines, TerminalError, usageFor } from './terminal/api'
  import {
    PERIODS, agentPlans, agentTokens, contextLeftText, keepPeriod, periodRange, planLines, planNote, planSentence, readPeriod,
    sessionHref, sessionTitle, tokens, totalsLine, type MachinePlanRead, type UsageAnswer, type UsagePeriod,
  } from './usage'

  let { userId = 0, timeZone = '', dateFormat = 'locale', onsignedout }: {
    userId?: number; timeZone?: string; dateFormat?: string; onsignedout?: (e: unknown) => void
  } = $props()

  const REFRESH = 60_000
  const store = () => { try { return localStorage } catch { return undefined } }

  let period = $state<UsagePeriod>(untrack(() => readPeriod(store(), userId)))
  let answer = $state<UsageAnswer | null>(null), answerPeriod = $state<UsagePeriod | null>(null)
  let loading = $state(true), failed = $state(false)
  let machines = $state<MachinePlanRead[] | null>(null), machinesFailed = $state(false)
  let now = $state(Date.now()), readAt = 0
  let narrow = $state(typeof matchMedia === 'function' && matchMedia('(max-width: 699px)').matches)

  const life = new AbortController()
  let request: AbortController | undefined, sequence = 0
  const refused = (e: unknown) => e instanceof TerminalError && e.status === 401
  const signOut = () => { life.abort(); onsignedout?.(new UnauthorizedError()) }

  const stale = $derived(answer !== null && answerPeriod !== period)
  const totals = $derived(answer ? totalsLine(answer, timeZone, dateFormat) : '')
  const periodName = $derived(PERIODS.find(p => p.id === period)?.label ?? 'Today')
  const nameOf = (tool: string) => answer?.agents.find(a => a.tool === tool)?.name ?? tool

  /** Reads the period's figures. A quiet read (the minute's refresh) keeps what is shown when it fails. */
  async function load(quiet = false) {
    request?.abort()
    const mine = request = new AbortController(), turn = ++sequence
    if (!quiet) loading = true
    const asked = period, range = periodRange(asked, timeZone, Date.now())
    try {
      const next = await usageFor(range.from, range.to, AbortSignal.any([life.signal, mine.signal]))
      if (turn !== sequence) return
      answer = next; answerPeriod = asked; failed = false; readAt = Date.now()
    } catch (e) {
      if (life.signal.aborted || mine.signal.aborted || turn !== sequence) return
      if (refused(e)) { signOut(); return }
      failed = true
    } finally {
      if (turn === sequence) loading = false
      now = Date.now()
    }
  }

  /** Reads which machines report sign-ins and each one's last answer, as the plan chips do. */
  async function loadPlans() {
    try {
      const list = await terminalMachines(life.signal)
      const reads = await Promise.all(list.map(async machine => {
        const readsSignIns = machine.capabilities?.agentAccounts === true
        const kept = readsSignIns ? await machinePlans(machine.agentId) : null
        return { host: machine.host, reads: readsSignIns, terminalOff: machine.capabilities?.terminalEnabled === false, answer: kept, error: readsSignIns && kept === null }
      }))
      if (life.signal.aborted) return
      machines = reads; machinesFailed = false; now = Date.now()
    } catch (e) {
      if (life.signal.aborted) return
      if (refused(e)) { signOut(); return }
      machinesFailed = true
    }
  }

  function choose(next: UsagePeriod) {
    if (next === period) return
    period = next
    keepPeriod(store(), userId, next)
    void load()
  }

  onMount(() => {
    void load(); void loadPlans()
    const media = typeof matchMedia === 'function' ? matchMedia('(max-width: 699px)') : null
    const width = () => { narrow = media?.matches ?? false }
    media?.addEventListener?.('change', width)
    // A quiet refresh each minute while the page is in view, and on coming back to it after a minute or more away.
    const tick = () => { if (document.visibilityState === 'visible' && !loading) { void load(true); void loadPlans() } }
    const timer = setInterval(tick, REFRESH)
    const visible = () => { if (document.visibilityState === 'visible' && Date.now() - readAt >= REFRESH) tick() }
    document.addEventListener('visibilitychange', visible)
    return () => { life.abort(); request?.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', visible); media?.removeEventListener?.('change', width) }
  })
</script>

<PageHeader title="Usage" lede="What each agent has left of its plan, and the tokens your sessions and Gaffer reported.">
  {#snippet actions()}
    <div class="periods" role="group" aria-label="Period">
      {#each PERIODS as choice (choice.id)}
        <button type="button" class="period" aria-pressed={period === choice.id} onclick={() => choose(choice.id)}>{choice.label}</button>
      {/each}
    </div>
  {/snippet}
</PageHeader>

<p class="totals" data-usage-totals aria-live="polite">{#if answer && !stale}{totals}{:else if loading}Reading usage for {periodName.toLowerCase() === 'today' ? 'today' : `the last ${periodName}`}…{/if}</p>

{#if failed}
  <div class="notice bad" role="alert">
    <p>Usage could not be read.{#if answer} The figures below are from the last read{stale ? ', for another period' : ''}.{/if}</p>
    <button type="button" class="btn" onclick={() => void load()} disabled={loading}>{loading ? 'Trying…' : 'Try again'}</button>
  </div>
{/if}

{#if !answer && loading}
  <Skeleton label="Reading usage…" shape="cards" count={3} height="160px" />
{:else if answer}
  {@const view = answer}
  <div class="body" class:dim={stale} aria-busy={loading}>
    <section class="agents" aria-labelledby="usage-agents">
      <h2 id="usage-agents">Agents</h2>
      <div class="grid">
        {#each view.agents as agent (agent.tool)}
          {@const plans = machines ? agentPlans(agent.tool, machines, now) : []}
          {@const used = agentTokens(agent)}
          <section class="card agent" data-usage-agent={agent.tool} aria-labelledby={`usage-agent-${agent.tool}`}>
            <h3 id={`usage-agent-${agent.tool}`}><AgentMark tool={agent.tool} size={20} /><span>{agent.name}</span></h3>
            <div class="spent" data-tokens>
              {#if used.figure}<p class="figure" data-figure>{used.figure}</p>{/if}
              <p class="line">{used.line}</p>
              {#if used.detail}<p class="detail">{used.detail}</p>{/if}
              {#if used.note}<p class="detail" data-retired>{used.note}</p>{/if}
            </div>
            <div class="plans" data-plans>
              <h4>Plan left</h4>
              {#if plans.length}
                {#each plans as entry (entry.key)}
                  {@const note = planNote(entry, agent.name, now)}
                  <div class="plan" class:stale={entry.stale} data-plan={entry.account.account ?? entry.host}>
                    <p class="who">
                      {#if entry.account.account}<span class="account">{entry.account.account}</span>{/if}
                      {#if entry.account.plan}<span class="pill">{entry.account.plan[0].toUpperCase() + entry.account.plan.slice(1)} plan</span>{/if}
                    </p>
                    <ul class="windows" aria-label={`${agent.name} plan left`}>
                      {#each planLines(entry, now) as line (line.key)}
                        <li class:low={line.low}>
                          <span class="words">{line.words}</span>
                          <span class="meter" role="meter" aria-label={`${line.name} left`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={line.left} aria-valuetext={`${line.left}% left`}><span style:width={line.left > 0 ? `max(4px, ${line.left}%)` : '0'}></span></span>
                        </li>
                      {/each}
                    </ul>
                    <p class="source">Read on {entry.host}{readAgo(entry.account.limitsAt, now) ? ` ${readAgo(entry.account.limitsAt, now)}` : ''}.</p>
                    {#if note}<p class="source note" data-plan-note>{note}</p>{/if}
                  </div>
                {/each}
              {:else if machines}
                <p class="why" data-plan-why>{planSentence(agent.tool, agent.name, machines, now)}</p>
              {:else if machinesFailed}
                <p class="why" data-plan-why>Your machines could not be read just now, so neither could their sign-ins.</p>
              {:else}
                <p class="why muted">Reading sign-ins…</p>
              {/if}
            </div>
          </section>
        {/each}
      </div>
    </section>

    <section class="card gaffer" data-usage-gaffer aria-labelledby="usage-gaffer">
      <h2 id="usage-gaffer">Gaffer</h2>
      {#if view.gaffer.length}
        <ul class="models">
          {#each view.gaffer as row (`${row.provider}/${row.model}`)}
            <li>
              <span class="model">{row.provider}{#if row.model}{` · ${row.model}`}{/if}</span>
              <span class="num">{tokens(row.input + row.output)} {row.input + row.output === 1 ? 'token' : 'tokens'}</span>
              <span class="detail">{row.replies.toLocaleString('en-GB')} {row.replies === 1 ? 'reply' : 'replies'} · Input {tokens(row.input)} · Output {tokens(row.output)}</span>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="line">Gaffer made no model requests in this period.</p>
      {/if}
    </section>

    <section class="sessions" data-usage-sessions aria-labelledby="usage-sessions">
      <h2 id="usage-sessions">Sessions in this period</h2>
      {#if !view.sessions.length}
        <p class="line">No session reported tokens in this period.</p>
      {:else if narrow}
        <ul class="cards" aria-labelledby="usage-sessions">
          {#each view.sessions as row (row.id)}
            {@const href = sessionHref(row)}
            {@const context = contextLeftText(row)}
            <li class="card row" data-session={row.id}>
              <p class="title">
                {#if href}<a {href}>{sessionTitle(row, nameOf(row.tool))}</a>{:else}<span>{sessionTitle(row, nameOf(row.tool))}</span>{/if}
                {#if row.factory}<span class="mark">Factory</span>{/if}
              </p>
              {#if row.archived}<p class="removed">Removed from history</p>{/if}
              <p class="meta">{nameOf(row.tool)}{row.model ? ` · ${row.model}` : ''}{row.host ? ` · ${row.host}` : ''}</p>
              <dl>
                <div><dt>Tokens</dt><dd class="num">{tokens(row.input + row.output)}</dd></div>
                <div><dt>Context</dt><dd class="num">{context || '—'}</dd></div>
                <div><dt>Last activity</dt><dd>{formatWhen(row.lastAt, timeZone, dateFormat)}</dd></div>
              </dl>
            </li>
          {/each}
        </ul>
      {:else}
        <div class="table-wrap">
          <table aria-labelledby="usage-sessions">
            <thead>
              <tr><th scope="col">Session</th><th scope="col">Agent</th><th scope="col">Model</th><th scope="col">Machine</th><th scope="col" class="num">Tokens</th><th scope="col" class="num">Context left</th><th scope="col">Last activity</th></tr>
            </thead>
            <tbody>
              {#each view.sessions as row (row.id)}
                {@const href = sessionHref(row)}
                <tr data-session={row.id}>
                  <th scope="row" class="title">
                    {#if href}<a {href}>{sessionTitle(row, nameOf(row.tool))}</a>{:else}<span>{sessionTitle(row, nameOf(row.tool))}</span>{/if}
                    {#if row.factory}<span class="mark">Factory</span>{/if}
                    {#if row.archived}<span class="removed">removed from history</span>{/if}
                  </th>
                  <td>{nameOf(row.tool)}</td>
                  <td class="model">{row.model || '—'}</td>
                  <td>{row.host || '—'}</td>
                  <td class="num">{tokens(row.input + row.output)}</td>
                  <td class="num">{contextLeftText(row) || '—'}</td>
                  <td class="when">{formatWhen(row.lastAt, timeZone, dateFormat)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      {#if view.more}<p class="more">Showing the latest {view.sessions.length} sessions; more reported tokens in this period.</p>{/if}
    </section>
  </div>
{/if}

<p class="closing">These are the agents’ own counts, passed on as numbers; reet attaches no price. Plain terminals and agents that report nothing are not counted, and nor is a report made while a machine was disconnected. Counting started on the day this version of reet was installed, and each count is kept for 90 days.</p>

<style>
  .periods { display: inline-flex; padding: 3px; gap: 2px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); }
  .period { min-height: 36px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--muted); font: 500 14px/1 var(--sans); cursor: pointer; white-space: nowrap; }
  .period:hover { color: var(--ink); background: var(--tint); }
  .period[aria-pressed=true] { background: var(--ink); color: var(--bg); }
  .period:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer: coarse) { .period { min-height: 44px; } }

  .totals { min-height: 1.5em; margin: -8px 0 20px; color: var(--ink); font: 500 1rem/1.5 var(--sans); font-variant-numeric: tabular-nums; }
  .notice { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; margin: 0 0 20px; padding: 10px 12px; border: 1px solid var(--bad); border-radius: 8px; background: var(--card); }
  .notice p { margin: 0; color: var(--bad); font-size: 14px; }
  .body.dim { opacity: .6; }
  h2 { display: flex; align-items: center; gap: 8px; margin: 0 0 12px; font: 600 1.05rem/1.3 var(--sans); }
  h3 { display: flex; align-items: center; gap: 8px; margin: 0; font: 600 1rem/1.3 var(--sans); }
  .agents, .gaffer, .sessions { margin-bottom: 28px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr)); gap: 12px; }
  .card { min-width: 0; padding: 14px 16px; border: 1px solid var(--line-soft); border-radius: 12px; background: var(--card); }
  .agent { display: flex; flex-direction: column; gap: 12px; }
  .plans { display: grid; gap: 12px; }
  .plan { display: grid; gap: 6px; }
  .who { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; margin: 0; min-width: 0; }
  .account { min-width: 0; overflow-wrap: anywhere; font-weight: 500; }
  .pill { flex: none; padding: 1px 8px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 14px; line-height: 20px; }
  .windows { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
  .windows li { display: grid; gap: 4px; }
  .words { font-size: 14px; font-variant-numeric: tabular-nums; }
  .meter { display: block; height: 4px; border-radius: 4px; background: var(--line); overflow: hidden; }
  .meter > span { display: block; height: 100%; border-radius: 4px; background: var(--good); }
  .windows li.low .words { color: var(--warn); font-weight: 600; }
  .windows li.low .meter > span { background: var(--warn); }
  .plan.stale .words { color: var(--muted); font-weight: 500; }
  .plan.stale .meter > span { background: var(--muted); }
  .source, .why, .detail, .more, .removed { margin: 0; color: var(--muted); font-size: 14px; overflow-wrap: anywhere; }
  .why { color: var(--ink); }
  .why.muted { color: var(--muted); }
  .spent { display: grid; gap: 2px; }
  .figure { margin: 0; font: 600 1.5rem/1.2 var(--sans); font-variant-numeric: tabular-nums; letter-spacing: -0.01em; }
  .plans { padding-top: 10px; border-top: 1px solid var(--line-soft); }
  h4 { margin: 0; color: var(--muted); font: 600 14px/1.3 var(--sans); }
  .line { margin: 0; font-variant-numeric: tabular-nums; }
  .spent .detail { font-variant-numeric: tabular-nums; }
  .muted { color: var(--muted); }

  .models { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .models li { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 2px 12px; }
  .models .model { min-width: 0; overflow-wrap: anywhere; }
  .models .detail { grid-column: 1 / -1; font-variant-numeric: tabular-nums; }
  .num { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }

  .table-wrap { overflow-x: auto; border: 1px solid var(--line-soft); border-radius: 12px; background: var(--card); }
  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  thead th { padding: 10px 12px; border-bottom: 1px solid var(--line); color: var(--muted); font-weight: 600; text-align: left; white-space: nowrap; }
  thead th.num { text-align: right; }
  tbody th, tbody td { padding: 10px 12px; border-bottom: 1px solid var(--line-soft); vertical-align: top; text-align: left; font-weight: 400; }
  tbody tr:last-child th, tbody tr:last-child td { border-bottom: 0; }
  tbody td.num { text-align: right; }
  .title { min-width: 9rem; overflow-wrap: anywhere; font-weight: 500; }
  td.model { white-space: nowrap; color: var(--muted); }
  td.when { min-width: 7rem; color: var(--muted); }
  .title a { color: var(--ink); text-decoration: underline; text-decoration-color: var(--line-strong); text-underline-offset: 3px; }
  .title a:hover { text-decoration-color: currentColor; }
  .title a:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 3px; }
  .mark { display: inline-block; margin-left: 6px; padding: 0 7px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 14px; line-height: 20px; font-weight: 500; vertical-align: 1px; }
  .title .removed { display: block; font-weight: 400; }

  .cards { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .row .title { margin: 0; min-width: 0; }
  .row .title a { display: inline-flex; align-items: center; min-height: 44px; }
  .row .meta { margin: 2px 0 0; color: var(--muted); font-size: 14px; overflow-wrap: anywhere; }
  .row dl { display: grid; grid-template-columns: repeat(3, minmax(0, auto)); justify-content: space-between; gap: 8px 12px; margin: 10px 0 0; }
  .row dl div { min-width: 0; }
  .row dt { color: var(--muted); font-size: 14px; }
  .row dd { margin: 0; font-size: 14px; font-variant-numeric: tabular-nums; text-align: left; white-space: normal; }

  .closing { max-width: 72ch; margin: 8px 0 0; color: var(--muted); font-size: 14px; line-height: 1.55; }
  @media (max-width: 420px) { .row dl { grid-template-columns: 1fr 1fr; } }
</style>
