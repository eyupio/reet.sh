<script lang="ts">
  import type { SkillRow } from '../api'
  import { healthLine } from './skillsPage'

  // One skill on the Skills page, collapsed: its name, where it came from, its tags, where it stands on the machines,
  // and Archive or Restore beside Delete. Each action button carries data-act, which the page moves focus to when a row
  // leaves the list. Pressing the name opens the row (`open`); the opened detail is drawn below the row's line. `busy`
  // names the action on its way, so its button says so and neither action is sent twice.
  let { row, selecting, selected, open, machines, canWrite, busy = '', onarchive, onrestore, ondelete, onselect, onopen, onretry, onlike }: {
    row: SkillRow
    selecting: boolean
    selected: boolean
    open: boolean
    /** How many machines follow the library, for the action hints. */
    machines: number
    canWrite: boolean
    busy?: '' | 'archive' | 'restore' | 'delete'
    onarchive: (row: SkillRow) => void
    onrestore: (row: SkillRow) => void
    ondelete: (row: SkillRow) => void
    onselect: (row: SkillRow, selected: boolean) => void
    onopen: (row: SkillRow) => void
    onretry: (row: SkillRow) => void
    onlike: (row: SkillRow) => void
  } = $props()

  const line = $derived(healthLine(row.health, row.status))
  const detailId = $derived(`skill-detail-${row.id}`)
  // Where it came from, in a few words. A link is a reference only; reet installs its own copy.
  const source = $derived.by(() => {
    if (row.originKind === 'written') return 'written by you'
    if (row.originKind === 'kept') return row.origin ? `kept from ${row.origin}` : 'kept from a machine'
    try { return new URL(row.origin).hostname.replace(/^www\./, '') } catch { return row.origin }
  })
  const note = $derived(row.upgradedFrom === 'parked' ? 'was Parked' : row.upgradedFrom === 'removed' ? 'removed before the upgrade' : '')
  const machinesWord = $derived(machines === 1 ? 'your machine' : `your ${machines} machines`)
  const STATES: Record<string, string> = { ok: 'Installed', off: 'Off', installing: 'Installing', removing: 'Removing', failed: 'Failed', offline: 'Offline', blocked: "Can't write skills here" }
  const machineState = (state: string) => STATES[state] ?? state
</script>

<li class="row" class:sel={selected} class:open data-skill={row.id}>
  <div class="line">
    <div class="who">
      <div class="title">
        {#if selecting}
          <!-- The label is the target a finger presses: 44 px around the box. -->
          <label class="pick"><input type="checkbox" class="check" checked={selected} aria-label={`Select ${row.name}`} onchange={e => onselect(row, e.currentTarget.checked)} /></label>
        {/if}
        <button type="button" class="name" aria-expanded={open} aria-controls={open ? detailId : undefined} onclick={() => onopen(row)}>{row.name}</button>
        {#if source}<span class="src">{source}</span>{/if}
        {#if note}<span class="note">{note}</span>{/if}
      </div>
      {#if row.description}<p class="desc">{row.description}</p>{/if}
    </div>
    {#if row.tags.length}
      <ul class="tags" aria-label={`Tags on ${row.name}`}>
        {#each row.tags as tag (tag.id)}<li>{tag.name}</li>{/each}
      </ul>
    {:else}
      <span class="tags" aria-hidden="true"></span>
    {/if}
    <div class="where">
      <p class="health" data-tone={line.tone}><span class="dot" aria-hidden="true"></span><span>{line.text}</span></p>
      {#if line.retry && canWrite}<button type="button" class="retry" aria-label={`Retry ${row.name}`} onclick={() => onretry(row)}>Retry</button>{/if}
    </div>
    {#if canWrite && row.shelfId}
      <button type="button" class="heart" class:on={row.liked} aria-pressed={row.liked} aria-label={`Like ${row.name}`} onclick={() => onlike(row)}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true"><path d="M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z"></path></svg>
      </button>
    {:else if row.liked}
      <span class="heart on" role="img" aria-label="Liked"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true"><path d="M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z"></path></svg></span>
    {:else}
      <span class="heart" aria-hidden="true"></span>
    {/if}
    {#if canWrite}
      <div class="acts">
        {#if row.status === 'active'}
          <button type="button" class="btn" data-act="archive" aria-label={`Archive ${row.name}`} title={machines ? `Take it off ${machinesWord} and keep it here` : 'Keep it here, off your machines'} aria-busy={busy === 'archive'} aria-disabled={!!busy} onclick={() => busy || onarchive(row)}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" stroke-linecap="round" aria-hidden="true"><rect x="3" y="4" width="18" height="5" rx="1"></rect><path d="M5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4"></path></svg>Archive
          </button>
        {:else}
          <button type="button" class="btn" data-act="restore" aria-label={`Restore ${row.name}`} title={machines ? `Put it back on ${machinesWord}` : 'Make it Active again'} aria-busy={busy === 'restore'} aria-disabled={!!busy} onclick={() => busy || onrestore(row)}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" stroke-linecap="round" aria-hidden="true"><path d="M9 14l-5-5 5-5M4 9h10a6 6 0 0 1 0 12h-3"></path></svg>Restore
          </button>
        {/if}
        <button type="button" class="btn danger" data-act="delete" aria-label={`Delete ${row.name}`} aria-busy={busy === 'delete'} aria-disabled={!!busy} onclick={() => busy || ondelete(row)}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" stroke-linecap="round" aria-hidden="true"><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"></path></svg>Delete
        </button>
      </div>
    {/if}
  </div>
  {#if open}
    <!-- The opened skill: its tags and where it is on each machine. Lists, notes, Edit, Copy prompt and Publish join it here. -->
    <div class="detail" id={detailId}>
      {#if row.description}<p class="full">{row.description}</p>{/if}
      {#if row.tags.length}
        <div class="open-tags">
          <span class="label">Tags</span>
          <ul class="tag-list" aria-label={`Tags of ${row.name}`}>{#each row.tags as tag (tag.id)}<li>{tag.name}</li>{/each}</ul>
        </div>
      {/if}
      {#if row.health.machines.length}
        <ul class="machines" aria-label={`Machines for ${row.name}`}>
          {#each row.health.machines as m}
            <li><span class="host">{m.host}</span><span class="state">{machineState(m.state)}{m.detail ? `: ${m.detail}` : ''}</span></li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</li>

<style>
  .row { border-top: 1px solid var(--line-soft); scroll-margin: 96px 0 120px; }
  .row:first-child { border-top: 0; }
  .row.sel { background: var(--accent-wash); }
  .row.open { background: color-mix(in srgb, var(--bg) 45%, var(--card)); }
  .line {
    display: grid; align-items: center; gap: 8px 14px; padding: 14px 16px 14px 20px;
    grid-template-columns: minmax(0, 1fr) 110px minmax(180px, 236px) 44px auto;
    grid-template-areas: 'who tags where heart acts';
  }
  .who { grid-area: who; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .title { display: flex; align-items: center; gap: 4px 10px; flex-wrap: wrap; min-width: 0; }
  .pick { display: inline-flex; align-items: center; justify-content: center; flex: none; min-width: 44px; min-height: 44px; margin: 0 -8px 0 -12px; cursor: pointer; }
  .check { width: 20px; height: 20px; margin: 0; }
  .name {
    min-height: 44px; padding: 0; border: 0; background: transparent; color: var(--ink); cursor: pointer; text-align: left;
    font: 600 1.25rem/1.2 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; min-width: 44px;
  }
  .name:hover { color: var(--accent-text); }
  .src { color: var(--muted); font: 0.875rem/1.4 var(--mono); overflow-wrap: anywhere; }
  .note { padding: 2px 8px; border-radius: 999px; background: var(--tint); color: var(--warn); font-size: 0.875rem; }
  .desc { margin: 0; color: var(--muted); font-size: 0.9375rem; line-height: 1.45; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
  .tags { grid-area: tags; display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; min-width: 0; }
  .tags li, .tag-list li { padding: 4px 9px; border-radius: 999px; background: var(--bg-sunk); color: var(--ink-soft); font: 0.875rem/1.3 var(--mono); overflow-wrap: anywhere; }
  .open-tags { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .open-tags .label { color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .tag-list { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; min-width: 0; }
  .where { grid-area: where; display: flex; align-items: center; gap: 4px; flex-wrap: wrap; min-width: 0; }
  .health { margin: 0; display: inline-flex; align-items: center; gap: 8px; font-size: 0.875rem; font-weight: 500; line-height: 1.35; min-width: 0; overflow-wrap: anywhere; }
  .health[data-tone='ok'] { color: var(--good); }
  .health[data-tone='work'] { color: var(--info); }
  .health[data-tone='bad'] { color: var(--bad); }
  .health[data-tone='off'] { color: var(--muted); }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: currentColor; flex: none; }
  .retry {
    min-height: 44px; min-width: 44px; padding: 0 6px; border: 0; background: transparent; cursor: pointer;
    color: var(--accent-text); font-weight: 600; font-size: 0.875rem; text-decoration: underline; text-underline-offset: 3px;
  }
  .heart { grid-area: heart; width: 44px; height: 44px; display: inline-flex; align-items: center; justify-content: center; border: 0; border-radius: 10px; background: transparent; color: var(--muted); cursor: pointer; }
  .heart svg { width: 20px; height: 20px; }
  span.heart { cursor: default; }
  .heart.on { color: var(--heart); }
  .heart.on svg { fill: currentColor; }
  button.heart:hover { background: var(--tint); }
  .acts { grid-area: acts; display: flex; gap: 8px; }
  .acts .btn { min-height: 44px; padding: 0 14px; border-radius: 10px; white-space: nowrap; font-weight: 500; }
  .acts .btn svg { width: 18px; height: 18px; flex: none; }
  .acts .danger { color: var(--bad); }
  .acts .btn[aria-disabled='true'] { opacity: 0.6; cursor: progress; }
  /* The ring that says a change is on its way takes the icon's place. */
  .acts .btn[aria-busy='true'] svg { display: none; }
  .detail { padding: 0 20px 18px; display: grid; gap: 10px; }
  .full { margin: 0; color: var(--ink-soft); max-width: 72ch; overflow-wrap: anywhere; }
  .machines { list-style: none; margin: 0; padding: 0; max-width: 480px; }
  .machines li { display: flex; justify-content: space-between; gap: 12px; padding: 8px 0; border-top: 1px solid var(--line-soft); font-size: 0.875rem; }
  .machines li:first-child { border-top: 0; }
  .host { font-family: var(--mono); overflow-wrap: anywhere; }
  .state { color: var(--muted); text-align: right; }
  @media (prefers-reduced-motion: no-preference) {
    .row { transition: background-color 0.15s; }
    .health[data-tone='work'] .dot { animation: pulse 1.3s ease-in-out infinite; }
    @keyframes pulse { 50% { opacity: 0.25; } }
  }
  /* A narrower screen drops the tags column; they are in the opened skill. */
  @media (max-width: 1000px) {
    .line { grid-template-columns: minmax(0, 1fr) minmax(150px, 200px) 44px auto; grid-template-areas: 'who where heart acts'; }
    .tags { display: none; }
  }
  /* A phone: the name and the heart, where it stands, then the two actions side by side. */
  @media (max-width: 700px) {
    .line { grid-template-columns: minmax(0, 1fr) 44px; grid-template-areas: 'who heart' 'where where' 'acts acts'; gap: 4px 8px; padding: 10px 12px 12px 16px; }
    .name { font-size: 1.1875rem; }
    .src, .desc { display: none; }
    .acts .btn { flex: 1; justify-content: center; }
    .detail { padding: 0 16px 16px; }
  }
</style>
