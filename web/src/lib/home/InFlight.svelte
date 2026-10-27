<script lang="ts">
  import { CATALOGUE } from '../catalogue'
  import type { MachineHealth } from '../terminal/api'
  import { shortAge, stateWord, type GroupBy, type Group, type HomeSession, type SessionOwner } from '../home'
  import SessionHoverCard from '../terminal/SessionHoverCard.svelte'

  // In flight: live sessions, and finished ones for a day, grouped by Project, Machine or State. Each row says the state word
  // and where it came from (R10): a terminal session reports no steps, so there is no progress bar. A row opens the session.
  let { groups, owners, machines, offline, now, loaded, failed = false, groupBy, onchoose, projectColours = new Map<string, string>() }: {
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
  } = $props()

  const id = $props.id()
  const choices: { value: GroupBy; label: string }[] = [{ value: 'project', label: 'Project' }, { value: 'machine', label: 'Machine' }, { value: 'state', label: 'State' }]
  const hostOf = (agentId: string) => machines.find(m => m.agentId === agentId)?.host || agentId
  const toolName = (tool: string) => CATALOGUE.find(agent => agent.id === tool)?.name ?? tool
  const lastKnown = (s: HomeSession) => offline.some(m => m.agentId === s.agentId)
  const href = (s: HomeSession) => `/terminals?agent=${encodeURIComponent(s.agentId)}&session=${encodeURIComponent(s.id)}`
  // The row's card on hover or keyboard focus, with the machine's last readings and the way in. The row stays the link.
  let card = $state<{ item: HomeSession; x: number; y: number } | null>(null)
  function showCard(el: HTMLElement, item: HomeSession) { const box = el.getBoundingClientRect(), width = 336, height = 270; card = { item, x: Math.min(Math.max(8, box.right + 10), Math.max(8, innerWidth - width - 8)), y: Math.min(Math.max(8, box.top - 8), Math.max(8, innerHeight - height - 8)) } }
  function hideCard() { card = null }
</script>

<section class="flight" aria-labelledby={`${id}-title`}>
  <header>
    <h2 id={`${id}-title`}>In flight</h2>
    <div class="group-by" role="radiogroup" aria-label="Group by">
      <span class="lead" aria-hidden="true">Group by</span>
      <span class="segments">
        {#each choices as c (c.value)}
          <label class:on={groupBy === c.value}><input type="radio" name={`${id}-group`} value={c.value} checked={groupBy === c.value} onchange={() => onchoose(c.value)} />{c.label}</label>
        {/each}
      </span>
    </div>
  </header>
  {#each offline as m (m.agentId)}<p class="offline">{m.host} is not reporting, so its sessions show their last known state.</p>{/each}
  {#if !loaded && failed}
    <p class="none">Couldn't read your sessions just now.</p>
  {:else if !loaded}
    <p class="none">Reading your sessions…</p>
  {:else if groups.length === 0}
    <p class="none">Nothing in flight. Sessions you start show here, and finished ones stay for a day.</p>
  {:else}
    <div class="list">
      {#each groups as g (g.key)}
        <div class="group">
          <h3>{g.label}{' '}<span class="count">{g.count}</span></h3>
          <ul>
            {#each g.sessions as s (s.id)}
              {@const word = stateWord(s)}
              {@const owner = owners.get(s.id)}
              <li>
                <a class="row" href={href(s)} onmouseenter={(event) => showCard(event.currentTarget, s)} onmouseleave={hideCard} onfocus={(event) => showCard(event.currentTarget, s)} onblur={hideCard}>
                  <span class="dot {word.tone}" aria-hidden="true"></span>
                  <span class="main">
                    <span class="title">{s.title}</span>
                    <span class="where">{#if s.tool}{toolName(s.tool)}{' · '}{/if}{owner ? `${owner.name} · ` : ''}{hostOf(s.agentId)}{#if s.worktreeBranch}{' · '}<code class="branch">{s.worktreeBranch}</code>{/if}</span>
                  </span>
                  <span class="state">
                    <span class="word {word.tone}">{word.word}{lastKnown(s) ? ' · last known' : ''}</span>
                    <span class="source">{word.source}</span>
                  </span>
                  <span class="age"><span class="vh">changed </span>{shortAge(Math.max(0, now - s.updatedAt))}<span class="vh"> ago</span></span>
                </a>
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    </div>
  {/if}
</section>
{#if card}
  {@const tip = card}
  <div class="session-card-pop" style={`left:${tip.x}px;top:${tip.y}px`}><SessionHoverCard session={tip.item} host={hostOf(tip.item.agentId)} project={owners.get(tip.item.id)?.name} colour={tip.item.projectId ? projectColours.get(tip.item.projectId) : ''} health={machines.find(m => m.agentId === tip.item.agentId)} serverNow={now} href={href(tip.item)} /></div>
{/if}

<style>
  .flight { min-width: 0; display: flex; flex-direction: column; gap: 8px; }
  .session-card-pop { position: fixed; z-index: 60; pointer-events: none; }
  header { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  h2 { margin: 0; flex-grow: 1; font: 600 20px/1.3 var(--serif); }
  .group-by { display: flex; align-items: center; gap: 10px; font-size: 13px; }
  .lead { color: var(--muted); }
  .segments { display: flex; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
  .segments label { position: relative; display: flex; align-items: center; padding: 5px 10px; color: var(--muted); cursor: pointer; }
  .segments label.on { background: var(--line-soft); color: var(--ink); }
  .segments label:has(:focus-visible) { outline: 2px solid var(--accent); outline-offset: -2px; }
  .segments input { position: absolute; inset: 0; opacity: 0; margin: 0; cursor: pointer; }
  .offline { margin: 0; font-size: 13px; color: var(--muted); }
  .none { margin: 0; padding: 16px; border: 1px solid var(--line); border-radius: 13px; background: var(--card); color: var(--muted); }
  .list { border: 1px solid var(--line); border-radius: 13px; background: var(--card); overflow: hidden; }
  .group + .group { border-top: 1px solid var(--line); }
  h3 { margin: 0; padding: 8px 16px 4px; font-size: 12px; font-weight: 600; letter-spacing: 0.04em; color: var(--muted); }
  .count { font-weight: 500; }
  ul { list-style: none; margin: 0; padding: 0; }
  .row { display: flex; align-items: center; gap: 14px; min-height: 44px; padding: 10px 16px; color: var(--ink); text-decoration: none; border-top: 1px solid var(--line-soft); }
  li:first-child .row { border-top: 0; }
  .row:hover { background: var(--card-raised); }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; background: var(--muted); }
  .dot.good { background: var(--good); }
  .dot.warn { background: var(--warn); }
  .dot.bad { background: var(--bad); }
  .main { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; }
  .title { font-weight: 500; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .where { font-size: 12px; color: var(--muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .branch { font-family: var(--mono); font-size: 12px; }
  .state { flex: none; display: flex; flex-direction: column; align-items: flex-end; text-align: right; max-width: 40%; }
  .word { font-size: 13px; font-weight: 500; color: var(--muted); }
  .word.good { color: var(--good); }
  .word.warn { color: var(--warn); }
  .word.bad { color: var(--bad); }
  .source { font-size: 11px; color: var(--muted); }
  .age { flex: none; width: 34px; text-align: right; font-size: 12px; color: var(--muted); }

  @media (pointer: coarse), (max-width: 1099px) {
    .segments label { min-height: 44px; padding: 0 14px; }
    .row { min-height: 60px; padding: 8px 16px; }
  }
  @media (max-width: 1099px) {
    h2 { font-size: 21px; }
  }
  /* Phone: the word stays; where it came from is still read out but not drawn, to keep the title readable. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .row { gap: 12px; min-height: 56px; padding: 8px 14px; }
    .source { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
    .age { display: none; }
    .group-by .lead { display: none; }
    header { gap: 8px; }
  }
</style>
