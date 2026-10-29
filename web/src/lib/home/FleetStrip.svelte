<script lang="ts">
  import { tick } from 'svelte'
  import { reporting, worktreesInUse, type FleetCard } from '../home'
  import { bytes } from '../terminal/health'

  // One card per enrolled machine in the person's machine order, then Add a machine. Every figure is labelled with what it
  // measures (R12): the running sessions on that machine of any kind, and how many are ending (the listing cannot say which run an agent), the CPU from
  // a fresh health report or "No reading", the worktrees live sessions there are in ("in use": no record says which other
  // worktrees reet made and has not removed) and the skills line from the last finished sync check. A card opens that
  // machine on Machines. Its + starts a session there (a project on the machine or a terminal); its menu offers the same
  // plus opening the machine and its sync. The pills report what the last scan says is installed.
  let { cards, sessions, addHref, loaded, onnew, onterminal }: {
    cards: FleetCard[]
    /** False on the read-only Home: those sign-ins cannot read sessions, so no count is shown rather than a false 0. */
    sessions: boolean
    /** Where Add a machine goes, or null when this sign-in cannot add one. */
    addHref: string | null
    loaded: boolean
    /** Start a session on this machine (writer only); absent on the read-only Home. */
    onnew?: (card: FleetCard, opener: HTMLElement) => void
    /** Start a plain terminal on this machine (writer only). */
    onterminal?: (card: FleetCard, opener: HTMLElement) => void
  } = $props()

  const id = $props.id()
  const running = (n: number) => `${n} ${n === 1 ? 'session' : 'sessions'} running`
  const agentsText = (c: FleetCard) => c.installed.agents === null ? 'installed agents not reported'
    : `${c.installed.agents.length} ${c.installed.agents.length === 1 ? 'agent' : 'agents'} installed`
  const skillsText = (c: FleetCard) => c.installed.skills === null ? 'installed skills not reported'
    : `${c.installed.skills} ${c.installed.skills === 1 ? 'skill' : 'skills'} installed`
  const mcpText = (c: FleetCard) => c.installed.mcp === null ? 'MCP servers not reported'
    : `${c.installed.mcp} MCP ${c.installed.mcp === 1 ? 'server' : 'servers'}`
  function name(c: FleetCard): string {
    const parts = [c.name, c.status]
    if (sessions) {
      const count = running(c.sessionsRunning) + (c.sessionsEnding ? ` · ${c.sessionsEnding} ending` : '')
      parts.push(reporting(c) ? count : `${count} (last known, not reporting)`, worktreesInUse(c.worktrees))
    }
    parts.push(`CPU ${c.cpu.text}`, `memory ${c.memory.text}`, `disk ${diskText(c)}`, c.skills.text, agentsText(c), skillsText(c), mcpText(c))
    if (c.os) parts.push(osLabel(c))
    return parts.join(', ')
  }
  const osNames: Record<string, string> = { linux: 'Linux', darwin: 'macOS', windows: 'Windows' }
  const osName = (os: string) => osNames[os] ?? os
  const osLabel = (c: FleetCard) => osName(c.os) + (c.osVersion ? ` ${c.osVersion}` : '')
  const load = (r: { percent: number | null }) => r.percent === null ? 'muted' : r.percent >= 80 ? 'warn' : 'good'
  const diskText = (c: FleetCard) => c.disk.availableBytes === null ? 'No reading' : `${bytes(c.disk.availableBytes)} free`
  const readings = (c: FleetCard) => [c.cpu, c.memory, { label: 'Disk', percent: c.disk.percent, text: diskText(c) }]
  const agentTitle = (c: FleetCard) => c.installed.agents === null ? 'No agent scan has reported on this machine yet.'
    : c.installed.agents.length ? `Reported by the machine: ${c.installed.agents.join(', ')}` : 'No agents are installed on this machine.'
  const skillsTitle = (c: FleetCard) => c.installed.skills === null ? 'No agent scan has reported on this machine yet.'
    : c.installed.skills ? `${c.installed.skills} distinct ${c.installed.skills === 1 ? 'skill' : 'skills'} across this machine’s agent scopes.` : 'No skills are installed on this machine.'
  const mcpTitle = (c: FleetCard) => c.installed.mcp === null ? 'No agent reports its MCP servers yet.' : `${c.installed.mcp} MCP ${c.installed.mcp === 1 ? 'server' : 'servers'} reported.`

  // One machine menu at a time: New session, New terminal, Open on Machines, Skills & sync. A right-click opens it where the
  // pointer is; the card's ⋮ opens it under the button; Escape closes it and gives focus back.
  let menuFor = $state<string | null>(null)
  let menuButton: HTMLElement | null = null
  let menuX = $state(0), menuY = $state(0)
  async function openMenu(c: FleetCard, button: HTMLElement, point?: { x: number; y: number }) {
    if (menuFor === c.agentId && !point) { closeMenu(); return }
    menuFor = c.agentId; menuButton = button
    const box = button.getBoundingClientRect()
    const at = point ?? { x: Math.max(8, box.right - 198), y: box.bottom + 4 }
    menuX = Math.min(Math.max(8, at.x), Math.max(8, innerWidth - 208))
    menuY = Math.min(Math.max(8, at.y), Math.max(8, innerHeight - 190))
    await tick()
    document.querySelector<HTMLElement>(`#${CSS.escape(`${id}-menu-${c.agentId}`)} [role=menuitem]`)?.focus()
  }
  function contextMenu(event: MouseEvent, c: FleetCard) {
    if (!onnew) return
    event.preventDefault()
    const slot = event.currentTarget as HTMLElement
    void openMenu(c, slot.querySelector<HTMLElement>('.card') ?? slot, { x: event.clientX, y: event.clientY })
  }
  function closeMenu(returnFocus = false) {
    menuFor = null
    if (returnFocus) menuButton?.focus()
  }
  function menuKeys(event: KeyboardEvent) {
    const items = [...(event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[role=menuitem]')]
    const at = items.indexOf(document.activeElement as HTMLElement)
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); closeMenu(true) }
    else if (event.key === 'ArrowDown') { event.preventDefault(); items[(at + 1) % items.length]?.focus() }
    else if (event.key === 'ArrowUp') { event.preventDefault(); items[(at - 1 + items.length) % items.length]?.focus() }
    else if (event.key === 'Home') { event.preventDefault(); items[0]?.focus() }
    else if (event.key === 'End') { event.preventDefault(); items.at(-1)?.focus() }
    else if (event.key === 'Tab') closeMenu()
  }
  function pick(action: (opener: HTMLElement) => void) {
    const opener = menuButton
    closeMenu()
    if (opener) action(opener)
  }
  function open(path: string) {
    closeMenu()
    location.assign(path)
  }
  function outside(event: PointerEvent) {
    if (menuFor && !(event.target as HTMLElement).closest('.menu, .more')) closeMenu()
  }
</script>

<svelte:window onpointerdown={outside} />

<section class="fleet" aria-label="Your machines">
  <div class="cards">
    {#each cards as c (c.agentId)}
      <div class="slot" role="group" aria-label={`${c.name} machine`} oncontextmenu={(event) => contextMenu(event, c)}>
        <a class="card" href={`/machines?machine=${encodeURIComponent(c.name)}`} aria-label={name(c)}
          onkeydown={(event) => { if (onnew && (event.key === 'ContextMenu' || event.shiftKey && event.key === 'F10')) void openMenu(c, event.currentTarget) }}>
          <span class="head">
            <span class="dot {c.status}" aria-hidden="true"></span>
            <span class="name">{c.name}</span>
            <span class="status {c.status}">{c.status}</span>
            {#if c.os}<span class="os" title={osLabel(c)}>{osLabel(c)}</span>{/if}
          </span>
          <span class="pills">
            <span class="pill" class:empty={c.installed.agents === null || c.installed.agents.length === 0} title={agentTitle(c)}>{c.installed.agents === null ? 'Agents —' : `Agents ${c.installed.agents.length}`}</span>
            <span class="pill" class:empty={c.installed.skills === null || c.installed.skills === 0} title={skillsTitle(c)}>{c.installed.skills === null ? 'Skills —' : `Skills ${c.installed.skills}`}</span>
            <span class="pill" class:empty={c.installed.mcp === null || c.installed.mcp === 0} title={mcpTitle(c)}>{c.installed.mcp === null ? 'MCP —' : `MCP ${c.installed.mcp}`}</span>
          </span>
          {#if sessions}
            <span class="sessions"><span class="figure">{c.sessionsRunning}</span> <span class="unit">{c.sessionsRunning === 1 ? 'session running' : 'sessions running'}</span>{#if c.sessionsEnding}{' '}<span class="ending">· {c.sessionsEnding} ending</span>{/if}</span>
            {#if !reporting(c)}<span class="caption">Last known count. {c.name} is not reporting.</span><span class="chip-note" aria-hidden="true">· last known</span>{/if}
          {/if}
          {#if reporting(c)}
          <span class="resources">
            {#each readings(c) as reading (reading.label)}
              <span class="metric">
                <span class="reading" class:cpu={reading.label === 'CPU'}><span class="label">{reading.label}</span> <span class="value">{reading.text}</span></span>
                <span class="bar {load(reading)}" aria-hidden="true">{#if reading.percent !== null}<span style:width={`${Math.max(0, Math.min(100, reading.percent))}%`}></span>{/if}</span>
              </span>
            {/each}
          </span>
          {:else}
            <span class="unavailable">Live stats unavailable while {c.status}.</span>
          {/if}
          <span class="foot">{#if sessions}<span class="worktrees">{worktreesInUse(c.worktrees)}</span>{/if}<span class="skills {c.skills.tone}">{c.skills.text}</span></span>
        </a>
        {#if onnew}
          <button type="button" class="quick" aria-label={`New session on ${c.name}`} title={`Start a session on ${c.name}`} onclick={(event) => onnew(c, event.currentTarget)}>
            <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M12 5v14M5 12h14" /></svg>
          </button>
          <button type="button" class="more" aria-label={`Actions for ${c.name}`} aria-haspopup="menu" aria-expanded={menuFor === c.agentId}
            aria-controls={menuFor === c.agentId ? `${id}-menu-${c.agentId}` : undefined} onclick={(event) => openMenu(c, event.currentTarget)}>
            <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
          </button>
        {/if}
      </div>
    {/each}
    {#if addHref}
      <a class="card add" href={addHref}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 5v14M5 12h14" /></svg>
        Add a machine
      </a>
    {/if}
  </div>
  {#if loaded && cards.length === 0}<p class="none">No machines yet. Add one to see it here.</p>{/if}
</section>

{#if menuFor}
  {@const card = cards.find(c => c.agentId === menuFor)}
  {#if card}
    <!-- svelte-ignore a11y_interactive_supports_focus -->
    <div class="menu" id={`${id}-menu-${card.agentId}`} style:left={`${menuX}px`} style:top={`${menuY}px`} role="menu" aria-label={card.name} tabindex="-1" onkeydown={menuKeys}>
      <button type="button" role="menuitem" onclick={() => { const target = card; pick((opener) => onnew?.(target, opener)) }}>New session…</button>
      {#if onterminal}<button type="button" role="menuitem" onclick={() => { const target = card; pick((opener) => onterminal?.(target, opener)) }}>New terminal</button>{/if}
      <button type="button" role="menuitem" onclick={() => open(`/machines?machine=${encodeURIComponent(card.name)}`)}>Open on Machines</button>
      <button type="button" role="menuitem" onclick={() => open(`/sync?machine=${encodeURIComponent(card.name)}`)}>Skills &amp; sync</button>
    </div>
  {/if}
{/if}

<style>
  .fleet { min-width: 0; }
  .cards { display: flex; gap: 14px; overflow-x: auto; padding-bottom: 2px; scrollbar-width: thin; }
  .slot { position: relative; display: flex; flex: 1 0 220px; min-width: 0; }
  .card {
    flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 10px; padding: 14px 16px;
    border: 1px solid var(--line); border-radius: 13px; background: var(--card); color: var(--ink); text-decoration: none;
  }
  .card:hover { border-color: var(--line-strong); }
  .head { display: flex; align-items: center; gap: 8px; min-width: 0; padding-right: 60px; }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; background: var(--muted); }
  .dot.online { background: var(--good); }
  /* Shape as well as colour: offline is a hollow ring, reconnecting a half-filled one, paused a grey square. */
  .dot.offline { background: transparent; border: 2px solid var(--muted); }
  .dot.paused { border-radius: 2px; }
  .dot.reconnecting { background: linear-gradient(90deg, var(--warn) 50%, transparent 50%); border: 2px solid var(--warn); }
  /* The name is the card's label: a long OS string gives way first and never cuts it. */
  .name { flex-shrink: 0; max-width: 100%; font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .status { font-size: 12px; color: var(--muted); }
  .status.online { color: var(--good); }
  .status.reconnecting { color: var(--warn); }
  .os { margin-left: auto; flex-shrink: 4; font-size: 12px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .pills { display: flex; flex-wrap: wrap; gap: 5px; }
  .pill { padding: 1px 8px; border: 1px solid var(--line-mid); border-radius: 99px; font-size: 11px; line-height: 1.5; color: var(--ink-soft); white-space: nowrap; }
  .pill.empty { color: var(--muted); border-color: var(--line-soft); }
  .sessions { display: flex; align-items: baseline; gap: 6px; }
  .figure { font: 600 28px/1 var(--serif); }
  .unit { color: var(--muted); }
  .ending { color: var(--muted); white-space: nowrap; }
  .caption { margin-top: -6px; font-size: 12px; color: var(--muted); }
  .unavailable { color: var(--muted); font-size: 12px; line-height: 1.5; }
  .resources { display: grid; gap: 6px; }
  .metric { display: grid; gap: 3px; }
  .reading { display: flex; flex-wrap: wrap; gap: 2px 8px; justify-content: space-between; font-size: 12px; color: var(--muted); }
  .cpu .value { color: var(--ink); }
  .bar { display: block; height: 6px; border-radius: 3px; background: var(--line-soft); overflow: hidden; }
  .bar > span { display: block; height: 100%; border-radius: 3px; background: var(--good); }
  .bar.warn > span { background: var(--warn); }
  .foot { display: flex; flex-wrap: wrap; gap: 2px 12px; font-size: 12px; color: var(--muted); }
  .skills { font-size: 12px; color: var(--muted); }
  .skills.good { color: var(--good); }
  .skills.warn { color: var(--warn); }
  .quick, .more {
    position: absolute; top: 10px; display: grid; place-items: center; width: 26px; height: 26px; padding: 0;
    border: 1px solid var(--line); border-radius: 7px; background: var(--card-raised); color: var(--ink-soft); cursor: pointer;
  }
  .quick { right: 12px; }
  .more { right: 44px; }
  .quick svg, .more svg { width: 15px; height: 15px; }
  .quick:hover, .more:hover, .more[aria-expanded='true'] { border-color: var(--muted); color: var(--ink); }
  .add {
    flex: 0 0 150px; align-items: center; justify-content: center; gap: 6px; border-style: dashed; border-color: var(--line-strong);
    background: transparent; color: var(--muted); text-align: center;
  }
  .add svg { width: 20px; height: 20px; }
  .add:hover { color: var(--ink); }
  .none { margin: 8px 0 0; color: var(--muted); }
  .chip-note { display: none; }
  .menu { position: fixed; z-index: 30; min-width: 188px; display: flex; flex-direction: column; padding: 4px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card-raised); box-shadow: 0 12px 30px var(--scrim); }
  .menu button { min-height: 34px; padding: 0 12px; border: 0; border-radius: 7px; background: transparent; color: var(--ink); font: inherit; font-size: 13px; text-align: left; cursor: pointer; }
  .menu button:hover, .menu button:focus-visible { background: var(--line-soft); }

  /* Tablet: the cards keep their width and scroll sideways; each shows its name, the count and the skills line. */
  @media (max-width: 1099px) {
    .cards { gap: 12px; }
    .slot { flex: 0 0 236px; }
    .card { gap: 8px; }
    .quick, .more { width: 44px; height: 44px; top: 8px; }
    .quick { right: 8px; }
    .more { right: 58px; }
    .head { padding-right: 104px; }
    .add { flex-basis: 150px; min-height: 44px; }
    .os { display: none; }
    .resources { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
    .reading { font-size: 11px; }
    .figure { font-size: 26px; }
    .caption { margin-top: -4px; }
  }
  /* Phone: machine chips, 44 px tall, scrolling sideways and fading at the edge. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .cards { gap: 8px; margin-right: -16px; padding-right: 16px; mask-image: linear-gradient(90deg, var(--ink) 85%, transparent); -webkit-mask-image: linear-gradient(90deg, var(--ink) 85%, transparent); }
    .slot { flex: none; }
    .card { flex-direction: row; align-items: center; gap: 8px; min-height: 44px; padding: 0 48px 0 14px; border-radius: 999px; }
    .foot, .caption, .status.online, .resources, .unavailable, .pills, .more { display: none; }
    .status { white-space: nowrap; }
    .sessions { gap: 4px; color: var(--muted); font-size: 13px; }
    .figure { font: inherit; color: var(--muted); }
    .unit { white-space: nowrap; }
    .chip-note { display: inline; font-size: 13px; color: var(--muted); white-space: nowrap; }
    .dot { width: 8px; height: 8px; }
    .quick { top: 50%; right: 2px; transform: translateY(-50%); width: 44px; height: 44px; }
    .head { padding-right: 0; }
    .add { flex: none; flex-direction: row; padding: 0 14px; }
    .add svg { width: 16px; height: 16px; }
  }
</style>
