<script lang="ts">
  import { reporting, worktreesInUse, type FleetCard } from '../home'
  import { bytes } from '../terminal/health'

  // One card per enrolled machine in the person's machine order, then Add a machine. Every figure is labelled with what it
  // measures (R12): the running sessions on that machine of any kind, and how many are ending (the listing cannot say which run an agent), the CPU from
  // a fresh health report or "No reading", the worktrees live sessions there are in ("in use": no record says which other
  // worktrees reet made and has not removed) and the skills line from the last finished sync check. A card opens that
  // machine on Machines.
  let { cards, sessions, addHref, loaded }: {
    cards: FleetCard[]
    /** False on the read-only Home: those sign-ins cannot read sessions, so no count is shown rather than a false 0. */
    sessions: boolean
    /** Where Add a machine goes, or null when this sign-in cannot add one. */
    addHref: string | null
    loaded: boolean
  } = $props()

  const running = (n: number) => `${n} ${n === 1 ? 'session' : 'sessions'} running`
  function name(c: FleetCard): string {
    const parts = [c.name, c.status]
    if (sessions) {
      const count = running(c.sessionsRunning) + (c.sessionsEnding ? ` · ${c.sessionsEnding} ending` : '')
      parts.push(reporting(c) ? count : `${count} (last known, not reporting)`, worktreesInUse(c.worktrees))
    }
    parts.push(`CPU ${c.cpu.text}`, `memory ${c.memory.text}`, `disk ${diskText(c)}`, c.skills.text)
    return parts.join(', ')
  }
  const load = (r: { percent: number | null }) => r.percent === null ? 'muted' : r.percent >= 80 ? 'warn' : 'good'
  const diskText = (c: FleetCard) => c.disk.availableBytes === null ? 'No reading' : `${bytes(c.disk.availableBytes)} free`
  const readings = (c: FleetCard) => [c.cpu, c.memory, { label: 'Disk', percent: c.disk.percent, text: diskText(c) }]
</script>

<section class="fleet" aria-label="Your machines">
  <div class="cards">
    {#each cards as c (c.agentId)}
      <a class="card" href={`/machines?machine=${encodeURIComponent(c.name)}`} aria-label={name(c)}>
        <span class="head">
          <span class="dot {c.status}" aria-hidden="true"></span>
          <span class="name">{c.name}</span>
          <span class="status {c.status}">{c.status}</span>
          {#if c.os}<span class="os">{c.os}</span>{/if}
        </span>
        {#if sessions}
          <span class="sessions"><span class="figure">{c.sessionsRunning}</span> <span class="unit">{c.sessionsRunning === 1 ? 'session running' : 'sessions running'}</span>{#if c.sessionsEnding}{' '}<span class="ending">· {c.sessionsEnding} ending</span>{/if}</span>
          {#if !reporting(c)}<span class="caption">Last known count. {c.name} is not reporting.</span><span class="chip-note" aria-hidden="true">· last known</span>{/if}
        {/if}
        <span class="resources">
          {#each readings(c) as reading (reading.label)}
            <span class="metric">
              <span class="cpu"><span class="label">{reading.label}</span> <span class="value">{reading.text}</span></span>
              <span class="bar {load(reading)}" aria-hidden="true">{#if reading.percent !== null}<span style:width={`${Math.max(0, Math.min(100, reading.percent))}%`}></span>{/if}</span>
            </span>
          {/each}
        </span>
        <span class="foot">{#if sessions}<span class="worktrees">{worktreesInUse(c.worktrees)}</span>{/if}<span class="skills {c.skills.tone}">{c.skills.text}</span></span>
      </a>
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

<style>
  .fleet { min-width: 0; }
  .cards { display: flex; gap: 14px; overflow-x: auto; padding-bottom: 2px; scrollbar-width: thin; }
  .card {
    flex: 1 0 220px; min-width: 0; display: flex; flex-direction: column; gap: 10px; padding: 14px 16px;
    border: 1px solid var(--line); border-radius: 13px; background: var(--card); color: var(--ink); text-decoration: none;
  }
  .card:hover { border-color: var(--line-strong); }
  .head { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; background: var(--muted); }
  .dot.online { background: var(--good); }
  /* Shape as well as colour: offline is a hollow ring, reconnecting a half-filled one, paused a grey square. */
  .dot.offline { background: transparent; border: 2px solid var(--muted); }
  .dot.paused { border-radius: 2px; }
  .dot.reconnecting { background: linear-gradient(90deg, var(--warn) 50%, transparent 50%); border: 2px solid var(--warn); }
  .name { font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .status { font-size: 12px; color: var(--muted); }
  .status.online { color: var(--good); }
  .status.reconnecting { color: var(--warn); }
  .os { margin-left: auto; font-size: 12px; color: var(--muted); white-space: nowrap; }
  .sessions { display: flex; align-items: baseline; gap: 6px; }
  .figure { font: 600 28px/1 var(--serif); }
  .unit { color: var(--muted); }
  .ending { color: var(--muted); white-space: nowrap; }
  .caption { margin-top: -6px; font-size: 12px; color: var(--muted); }
  .resources { display: grid; gap: 6px; }
  .metric { display: grid; gap: 3px; }
  .cpu { display: flex; justify-content: space-between; font-size: 12px; color: var(--muted); }
  .cpu .value { color: var(--ink); }
  .bar { display: block; height: 6px; border-radius: 3px; background: var(--line-soft); overflow: hidden; }
  .bar > span { display: block; height: 100%; border-radius: 3px; background: var(--good); }
  .bar.warn > span { background: var(--warn); }
  .foot { display: flex; flex-wrap: wrap; gap: 2px 12px; font-size: 12px; color: var(--muted); }
  .skills { font-size: 12px; color: var(--muted); }
  .skills.good { color: var(--good); }
  .skills.warn { color: var(--warn); }
  .add {
    flex: 0 0 150px; align-items: center; justify-content: center; gap: 6px; border-style: dashed; border-color: var(--line-strong);
    background: transparent; color: var(--muted); text-align: center;
  }
  .add svg { width: 20px; height: 20px; }
  .add:hover { color: var(--ink); }
  .none { margin: 8px 0 0; color: var(--muted); }
  .chip-note { display: none; }

  /* Tablet: the cards keep their width and scroll sideways; each shows its name, the count and the skills line. */
  @media (max-width: 1099px) {
    .cards { gap: 12px; }
    .card { flex: 0 0 236px; gap: 8px; }
    .add { flex-basis: 150px; min-height: 44px; }
    .os { display: none; }
    .resources { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
    .cpu { font-size: 11px; }
    .figure { font-size: 26px; }
    .caption { margin-top: -4px; }
  }
  /* Phone: machine chips, 44 px tall, scrolling sideways and fading at the edge. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .cards { gap: 8px; margin-right: -16px; padding-right: 16px; mask-image: linear-gradient(90deg, var(--ink) 85%, transparent); -webkit-mask-image: linear-gradient(90deg, var(--ink) 85%, transparent); }
    .card { flex: none; flex-direction: row; align-items: center; gap: 8px; min-height: 44px; padding: 0 14px; border-radius: 999px; }
    .foot, .caption, .status.online, .resources { display: none; }
    .status { white-space: nowrap; }
    .sessions { gap: 4px; color: var(--muted); font-size: 13px; }
    .figure { font: inherit; color: var(--muted); }
    .unit { white-space: nowrap; }
    .chip-note { display: inline; font-size: 13px; color: var(--muted); white-space: nowrap; }
    .dot { width: 8px; height: 8px; }
    .add { flex: none; flex-direction: row; padding: 0 14px; }
    .add svg { width: 16px; height: 16px; }
  }
</style>
