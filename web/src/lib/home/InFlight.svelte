<script lang="ts">
  import { tick } from 'svelte'
  import { toolName } from '../agentLaunch'
  import { sessionHref, type MachineHealth, type TerminalMachine } from '../terminal/api'
  import { shortAge, stateWord, type GroupBy, type Group, type HomeSession, type SessionOwner } from '../home'
  import { sessionStatus } from '../sessionStatus'
  import SessionActions from '../sessions/SessionActions.svelte'
  import SessionHoverCard from '../terminal/SessionHoverCard.svelte'

  // In flight: live sessions, and finished ones for a day, grouped by Project, Machine or State. Each row says the state word
  // and where it came from (R10): a terminal session reports no steps, so there is no progress bar. A row opens the session;
  // its actions menu (a right-click, Shift+F10 or the menu key) offers the same actions as on the Sessions page.
  let { heading = 'In flight', groups, owners, machines, offline, now, loaded, failed = false, groupBy, onchoose, projectColours = new Map<string, string>(),
    terminals = [], removeDefault = false, onchanged, onnotice, onsignedout }: {
    heading?: string
    groups: Group[]
    owners: Map<string, SessionOwner>
    machines: MachineHealth[]
    /** Machines that are not reporting while some of their sessions are listed: those rows are their last known state. */
    offline: { agentId: string; host: string }[]
    now: number
    loaded: boolean
    /** The sessions could not be read and none were read before. */
    failed?: boolean
    groupBy: GroupBy
    onchoose: (groupBy: GroupBy) => void
    /** Folder project id → its rail colour id, ""/absent for none. */
    projectColours?: Map<string, string>
    /** The machines' terminal records, for a session whose machine has the server turned off. */
    terminals?: TerminalMachine[]
    /** The profile's choice for a session that ends: remove it from history too. */
    removeDefault?: boolean
    /** A session action went through: read the sessions again. */
    onchanged: () => void | Promise<void>
    onnotice: (notice: { text: string; bad?: boolean } | null) => void
    onsignedout: () => void
  } = $props()

  let actions = $state<SessionActions>()
  const offlineIds = $derived(new Set(offline.map(m => m.agentId)))

  const PAGE_SIZE = 5
  let limit = $state(PAGE_SIZE), expanded = $state<string[]>([])
  const total = $derived(groups.reduce((sum, group) => sum + group.sessions.length, 0))
  const groupKey = $derived(JSON.stringify([groupBy, groups.map(group => group.key)]))
  $effect(() => { void groupKey; limit = PAGE_SIZE })
  const shownGroups = $derived.by(() => {
    let remaining = limit
    return groups.map(group => { const sessions = group.sessions.slice(0, remaining); remaining -= sessions.length; return {...group, sessions} }).filter(group => group.sessions.length)
  })
  async function showItems(amount: number, opener: HTMLButtonElement) {
    const panel = opener.parentElement
    limit = amount
    await tick()
    if (!opener.isConnected) panel?.querySelector('button')?.focus()
  }
  const id = $props.id()
  const choices: { value: GroupBy; label: string }[] = [{ value: 'project', label: 'Project' }, { value: 'machine', label: 'Machine' }, { value: 'state', label: 'State' }]
  const hostOf = (agentId: string) => machines.find(m => m.agentId === agentId)?.host || agentId
  const lastKnown = (s: HomeSession) => offline.some(m => m.agentId === s.agentId)
  // The row's card on hover or keyboard focus, with the machine's last readings and the way in. The row stays the link.
  let card = $state<{ item: HomeSession; x: number; y: number } | null>(null)
  function showCard(el: HTMLElement, item: HomeSession) { if (!matchMedia('(hover: hover)').matches) return; const box = el.getBoundingClientRect(), width = 336, height = 270; card = { item, x: Math.min(Math.max(8, box.right + 10), Math.max(8, innerWidth - width - 8)), y: Math.min(Math.max(8, box.top - 8), Math.max(8, innerHeight - height - 8)) } }
  function hideCard() { card = null }
</script>

<section class="flight" aria-labelledby={`${id}-title`}>
  <header>
    <h2 id={`${id}-title`}>{heading}</h2>
    <div class="group-by" role="radiogroup" aria-label="Group by">
      <span class="lead" aria-hidden="true">Group by</span>
      <span class="segments">
        {#each choices as c (c.value)}
          <label class:on={groupBy === c.value}><input type="radio" name={`${id}-group`} value={c.value} checked={groupBy === c.value} onchange={() => onchoose(c.value)} />{c.label}</label>
        {/each}
      </span>
    </div>
  </header>
  {#each offline.filter(machine => shownGroups.some(group => group.sessions.some(session => session.agentId === machine.agentId))) as m (m.agentId)}<p class="offline">{m.host} is not reporting, so its sessions show their last known state.</p>{/each}
  {#if !loaded && failed}
    <p class="none">Couldn't read your sessions just now.</p>
  {:else if !loaded}
    <p class="none">Reading your sessions…</p>
  {:else if groups.length === 0}
    <p class="none">Nothing in flight. Sessions you start show here, and finished ones stay for a day.</p>
  {:else}
    <div class="list">
      {#each shownGroups as g (g.key)}
        <div class="group">
          <h3>{g.label}{' '}<span class="count">{g.count}</span></h3>
          <ul>
            {#each g.sessions as s (s.id)}
              {@const word = stateWord(s)}
              {@const status = sessionStatus(s)}
              {@const owner = owners.get(s.id)}
              <li data-status={status} class:expanded={expanded.includes(s.id)}>
                <div class="summary">
                <a class="row" href={sessionHref(s)} onmouseenter={(event) => showCard(event.currentTarget, s)} onmouseleave={hideCard} onfocus={(event) => showCard(event.currentTarget, s)} onblur={hideCard}
                  oncontextmenu={(event) => { hideCard(); void actions?.openMenu(event, s) }}
                  onkeydown={(event) => { if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) { hideCard(); void actions?.openMenu(event, s) } }}>
                  {#if status === 'finished'}<span class="status-tick" aria-hidden="true"></span>{:else}<span class="dot" aria-hidden="true"></span>{/if}
                  <span class="main">
                    <span class="title">{s.title}{#if s.yolo}{' '}<span class="badge yolo">YOLO</span>{/if}</span>
                    <span class="where">{owner?.name || hostOf(s.agentId)}<span class="mobile-age"> · {shortAge(Math.max(0, now - s.updatedAt))} ago</span></span>
                  </span>
                  <span class="state">
                    <span class="word">{word.word}{lastKnown(s) ? ' · last known' : ''}</span>
                    <span class="source">{word.source}</span>
                  </span>
                  <span class="age"><span class="vh">changed </span>{shortAge(Math.max(0, now - s.updatedAt))}<span class="vh"> ago</span></span>
                </a>
                <button type="button" class="more disclosure" aria-label={`${expanded.includes(s.id) ? 'Hide' : 'Show'} details for ${s.title}`} aria-expanded={expanded.includes(s.id)} aria-controls={`${id}-${s.id}-details`} onclick={() => { hideCard(); expanded = expanded.includes(s.id) ? expanded.filter(key => key !== s.id) : [...expanded, s.id] }}><span aria-hidden="true">{expanded.includes(s.id) ? '⌃' : '⌄'}</span></button>
                <button type="button" class="more" aria-label={`Actions for ${s.title}`} aria-haspopup="menu" onclick={event => { hideCard(); void actions?.openMenu(event, s) }}><span aria-hidden="true">···</span></button>
                </div>
                <div class="item-details" hidden={!expanded.includes(s.id)} id={`${id}-${s.id}-details`}>
                  <p>{s.tool ? toolName(s.tool) : 'Terminal'}{s.model ? ` · ${s.model}` : ''}{s.yolo ? ' · YOLO' : ''}</p>
                  <p>{hostOf(s.agentId)}{s.dir ? ` · ${s.dir}` : ''}</p>
                  {#if s.worktreeBranch}<p><code>{s.worktreeBranch}</code></p>{/if}
                  {#if s.promptExcerpt}<p>{s.promptExcerpt}{s.promptMore ? '…' : ''}</p>{/if}
                  <p>{word.source}{lastKnown(s) ? ' · last known' : ''} · Changed {shortAge(Math.max(0, now - s.updatedAt))} ago</p>
                  <a href={sessionHref(s)}>Open session →</a>
                </div>
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    </div>
    {#if total > PAGE_SIZE}<div class="list-window"><span>Showing {Math.min(limit, total)} of {total} sessions</span>
      {#if limit < total}<button type="button" onclick={event => void showItems(limit + PAGE_SIZE, event.currentTarget)}>Show {Math.min(PAGE_SIZE, total - limit)} more</button><button type="button" onclick={event => void showItems(total, event.currentTarget)}>View all {total}</button>
      {:else}<button type="button" onclick={async event => { const panel = event.currentTarget.parentElement; limit = PAGE_SIZE; expanded = []; await tick(); panel?.querySelector('button')?.focus() }}>Show fewer</button>{/if}
      <a href="/sessions?view=list">All session history →</a>
    </div>{/if}
  {/if}
</section>
{#if card}
  {@const tip = card}
  <div class="session-card-pop" style={`left:${tip.x}px;top:${tip.y}px`}><SessionHoverCard session={tip.item} host={hostOf(tip.item.agentId)} project={owners.get(tip.item.id)?.name} colour={tip.item.projectId ? projectColours.get(tip.item.projectId) : ''} health={machines.find(m => m.agentId === tip.item.agentId)} serverNow={now} href={sessionHref(tip.item)} /></div>
{/if}
<SessionActions bind:this={actions} {machines} offline={offlineIds} {terminals} {removeDefault} {onsignedout} {onchanged} {onnotice} />

<style>
  .flight { min-width: 0; display: flex; flex-direction: column; gap: 8px; }
  .session-card-pop { position: fixed; z-index: 60; pointer-events: none; }
  header { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  h2 { margin: 0; flex-grow: 1; font: 600 16px/1.3 var(--sans); }
  .group-by { display: flex; align-items: center; gap: 10px; font-size: 14px; }
  .lead { color: var(--muted); }
  .segments { display: flex; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
  .segments label { position: relative; display: flex; align-items: center; padding: 5px 10px; color: var(--muted); cursor: pointer; }
  .segments label.on { background: var(--line-soft); color: var(--ink); }
  .segments label:has(:focus-visible) { outline: 2px solid var(--accent); outline-offset: -2px; }
  .segments input { position: absolute; inset: 0; opacity: 0; margin: 0; cursor: pointer; }
  .offline { margin: 0; font-size: 14px; color: var(--muted); }
  .none { margin: 0; padding: 16px; border: 1px solid var(--line); border-radius: 13px; background: var(--card); color: var(--muted); }
  .list { border-top: 1px solid var(--line); border-bottom: 1px solid var(--line); overflow: hidden; }
  .group + .group { border-top: 1px solid var(--line); }
  h3 { display: flex; align-items: baseline; gap: 8px; margin: 0; padding: 14px 12px 6px; font-size: 13px; font-weight: 500; color: var(--muted); }
  .count { font-weight: 400; font-variant-numeric: tabular-nums; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { display: block; border-top: 1px solid var(--line-soft); }
  li:first-child { border-top: 0; }
  li:hover { background: var(--card-raised); }
  .more { flex: none; align-self: center; width: 44px; min-height: 44px; border: 0; border-radius: 7px; background: transparent; color: var(--muted); font: 600 19px/1 var(--sans); cursor: pointer; }
  .more:hover { background: var(--line-soft); color: var(--ink); }
  .more:focus-visible, .row:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  .summary { display: flex; align-items: stretch; min-width: 0; }
  .item-details { padding: 0 14px 12px; color: var(--muted); font-size: 14px; overflow-wrap: anywhere; }
  .item-details p { margin: 6px 0; }
  .item-details a { display: inline-flex; align-items: center; min-height: 44px; color: var(--ink); }
  .list-window { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 16px; font-size: 13px; color: var(--muted); }
  .list-window button, .list-window a { min-height: 44px; align-content: center; border: 0; padding: 4px 0; background: transparent; color: var(--ink); font: inherit; cursor: pointer; text-decoration: underline; }
  .list-window button:focus-visible, .item-details a:focus-visible { outline: 2px solid var(--accent); }
  .row { flex: 1; min-width: 0; display: flex; align-items: center; gap: 14px; min-height: 44px; padding: 10px 16px; color: var(--ink); text-decoration: none; }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; background: var(--status); }
  li[data-status='waiting'], li[data-status='failed'] { box-shadow: inset 2px 0 var(--status); }
  .main { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; }
  .title { font-weight: 500; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .badge.yolo { margin-left: 4px; padding: 0 5px; border: 1px solid var(--warn); border-radius: 5px; color: var(--warn); font-size: 14px; font-weight: 600; vertical-align: 1px; }
  .where { font-size: 14px; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .item-details code { font-family: var(--mono); font-size: 14px; }
  .state { flex: none; display: flex; flex-direction: column; align-items: flex-end; text-align: right; max-width: 40%; }
  .word { font-size: 14px; font-weight: 500; color: var(--status); padding: 2px 0; }
  .mobile-age { display: none; }
  .source { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  .age { flex: none; width: 34px; text-align: right; font-size: 14px; color: var(--muted); }

  @media (pointer: coarse), (max-width: 1099px) {
    .segments label { min-height: 44px; padding: 0 14px; }
    .row { min-height: 44px; padding: 8px 12px; }
  }
  @media (max-width: 1099px) {
    h2 { font-size: 16px; }
  }
  /* Phone: the word stays; where it came from is still read out but not drawn, to keep the title readable. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .row { gap: 12px; min-height: 44px; padding: 8px 10px; }
    .mobile-age { display: none; }
  .source { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
    .age { display: none; }
    .mobile-age { display: inline; }
    .group-by .lead { display: none; }
    header { gap: 8px; }
  }
</style>
