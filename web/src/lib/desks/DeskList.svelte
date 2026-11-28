<script lang="ts">
  import { projectColourHex, type ProjectColour } from '../projectColour'
  import { sessionStatus } from '../sessionStatus'
  import { terminalClosed, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import DeskMenu from './DeskMenu.svelte'
  import type { DeskSummary } from './api'

  // Every desk, one row each: its colour, name, the tabs, panes and machines it holds, and how many of its shells are
  // live, wait for the person or can be reopened. Desks with a shell waiting come first, under Waiting for you, tinted in
  // the product's waiting colour and with an Open button; the rest follow. New desk starts an empty one; From a template
  // copies a template. The templates are a second list, each opening the template's own page, with Rename and Delete.
  let { desks, sessions, machines, failed = false, busy = false, onopen, onnew, onfromtemplate, ondesk, oncolour }: {
    desks: DeskSummary[]
    sessions: TerminalSession[]
    machines: TerminalMachine[]
    failed?: boolean
    busy?: boolean
    onopen: (id: string) => void
    onnew: () => void
    onfromtemplate: (template: DeskSummary) => void
    ondesk: (action: 'rename' | 'duplicate' | 'template' | 'delete', desk: DeskSummary) => void
    oncolour: (desk: DeskSummary, colour: ProjectColour) => void
  } = $props()

  const byId = $derived(new Map(sessions.map(s => [s.id, s])))
  const hostName = $derived(new Map(machines.map(m => [m.agentId, m.host])))
  const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`
  function counts(d: DeskSummary) {
    let live = 0, waiting = 0, reopen = 0
    for (const id of d.sessions) {
      const s = byId.get(id)
      if (!s) continue
      // To reopen: a plain shell that ended or was lost on a machine still enrolled, as Reopen all counts them. A pane
      // never started is not one.
      if (terminalClosed(s.state)) { if (!s.tool && hostName.has(s.agentId)) reopen++; continue }
      live++
      if (sessionStatus(s) === 'waiting') waiting++
    }
    return { live, waiting, reopen }
  }
  /** The machines a desk spans, each once; machines since removed are counted together. */
  function where(d: DeskSummary): string {
    const names = [...new Set(d.hosts.map(h => hostName.get(h)).filter((n): n is string => !!n))]
    const gone = new Set(d.hosts.filter(h => !hostName.has(h))).size
    if (gone) names.push(gone === 1 ? 'a removed machine' : `${gone} removed machines`)
    return names.join(', ')
  }

  const ranked = $derived(desks.filter(d => !d.template).map(d => ({ d, c: counts(d) })))
  const needing = $derived(ranked.filter(r => r.c.waiting))
  const others = $derived(ranked.filter(r => !r.c.waiting))
  const templates = $derived(desks.filter(d => d.template))

  let menu = $state<{ kind: 'card' | 'template' | 'templates'; desk?: DeskSummary; x: number; y: number; opener: HTMLElement } | null>(null)
  function openMenu(event: MouseEvent, kind: 'card' | 'template' | 'templates', desk?: DeskSummary) {
    event.preventDefault()
    const opener = event.currentTarget as HTMLElement, box = opener.getBoundingClientRect()
    menu = { kind, desk, opener, x: kind === 'templates' ? box.left : box.right - 220, y: box.bottom + 4 }
  }
  /** A desk's name opens it in this tab; a modified click is left to the browser. */
  function follow(event: MouseEvent, id: string) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onopen(id)
  }
  function closeMenu(focus: boolean) { const opener = menu?.opener; menu = null; if (focus) opener?.focus() }
  function then(action: 'rename' | 'duplicate' | 'template' | 'delete') { const d = menu?.desk; closeMenu(false); if (d) ondesk(action, d) }
</script>

{#snippet row(d: DeskSummary, c: ReturnType<typeof counts> | null)}
  {@const machinesLine = where(d)}
  <li class="row" class:tpl={!c} data-status={c?.waiting ? 'waiting' : undefined} style={`--pc:${d.colour ? projectColourHex(d.colour) : 'var(--line-strong)'}`}>
    <span class="sw" aria-hidden="true"></span>
    <a class="name" href={`/desks/${encodeURIComponent(d.id)}`} onclick={event => follow(event, d.id)}>{d.name}</a>
    <div class="meta">
      <span>{plural(d.tabs, 'tab')} · {plural(d.panes, 'pane')}</span>
      {#if machinesLine}<span class="on">{machinesLine}</span>{/if}
    </div>
    {#if c && (c.waiting || c.live || c.reopen)}
      <div class="pills">
        {#if c.waiting}<span class="pill" data-status="waiting"><i></i>{plural(c.waiting, 'shell')} {c.waiting === 1 ? 'needs' : 'need'} you</span>{/if}
        {#if c.live}<span class="pill" data-status="working"><i></i>{plural(c.live, 'shell')} live</span>{/if}
        {#if c.reopen}<span class="pill" data-status="ended"><i></i>{plural(c.reopen, 'shell')} to reopen</span>{/if}
      </div>
    {/if}
    <div class="end">
      {#if c?.waiting}<a class="btn primary" href={`/desks/${encodeURIComponent(d.id)}`} aria-label={`Open ${d.name}`} onclick={event => follow(event, d.id)}>Open</a>{/if}
      <button type="button" class="more" aria-label={`Actions for ${d.name}`} aria-haspopup="menu" aria-expanded={menu?.desk?.id === d.id} onclick={event => openMenu(event, c ? 'card' : 'template', d)}>⋮</button>
    </div>
  </li>
{/snippet}

<section class="desk-list" aria-labelledby="desks-title">
  <header class="head">
    <div>
      <h1 id="desks-title">Desks</h1>
      <p class="lede">A desk is a saved set of terminals. Open one to carry on where you left it.</p>
    </div>
    <div class="actions">
      <button type="button" class="btn" aria-haspopup="menu" aria-expanded={menu?.kind === 'templates'} disabled={busy || templates.length === 0}
        title={templates.length ? undefined : 'Save a desk as a template from its menu to start others from it.'} onclick={event => openMenu(event, 'templates')}>From a template ▾</button>
      <button type="button" class="btn primary" disabled={busy} aria-busy={busy} onclick={onnew}>New desk</button>
    </div>
  </header>
  {#if failed}<p class="notice bad" role="alert">The desks could not be read just now. reet will try again shortly.</p>{/if}
  {#if ranked.length === 0 && !failed}
    <div class="empty">
      <h2>No desks yet.</h2>
      <p>A desk keeps a set of terminals together: their tabs, their folders and their history. Start one, then add a tab for each shell.</p>
      <button type="button" class="btn primary" disabled={busy} onclick={onnew}>New desk</button>
    </div>
  {:else}
    {#if needing.length}
      <h2 class="group-title" id="desks-waiting">Waiting for you</h2>
      <ul class="rows" aria-labelledby="desks-waiting">{#each needing as { d, c } (d.id)}{@render row(d, c)}{/each}</ul>
    {/if}
    {#if others.length}
      {#if needing.length}<h2 class="group-title" id="desks-others">Other desks</h2>{/if}
      <ul class="rows" aria-labelledby={needing.length ? 'desks-others' : 'desks-title'}>{#each others as { d, c } (d.id)}{@render row(d, c)}{/each}</ul>
    {/if}
  {/if}
  {#if templates.length}
    <section class="group" aria-labelledby="desk-templates-title">
      <h2 class="group-title" id="desk-templates-title">Templates</h2>
      <p class="hint">Start a new desk from one of these. The template itself is not changed.</p>
      <ul class="rows">{#each templates as d (d.id)}{@render row(d, null)}{/each}</ul>
    </section>
  {/if}
</section>

{#if menu?.kind === 'card' && menu.desk}
  {@const d = menu.desk}
  <DeskMenu x={menu.x} y={menu.y} label={`Actions for ${d.name}`} opener={menu.opener} onclose={closeMenu} colour={d.colour} oncolour={colour => { const t = d; closeMenu(true); oncolour(t, colour) }}>
    <button type="button" role="menuitem" onclick={() => { const id = d.id; closeMenu(false); onopen(id) }}>Open</button>
    <button type="button" role="menuitem" onclick={() => then('rename')}>Rename</button>
    <button type="button" role="menuitem" onclick={() => then('duplicate')}>Duplicate</button>
    <a role="menuitem" href={`/desks/${encodeURIComponent(d.id)}`} target="_blank" rel="noopener noreferrer" onclick={() => closeMenu(false)}>Move to its own tab</a>
    <button type="button" role="menuitem" onclick={() => then('template')}>Save as template</button>
    <button type="button" role="menuitem" class="danger" onclick={() => then('delete')}>Delete</button>
  </DeskMenu>
{:else if menu?.kind === 'template' && menu.desk}
  {@const d = menu.desk}
  <DeskMenu x={menu.x} y={menu.y} label={`Actions for ${d.name}`} opener={menu.opener} onclose={closeMenu}>
    <button type="button" role="menuitem" onclick={() => { const id = d.id; closeMenu(false); onopen(id) }}>Open</button>
    <button type="button" role="menuitem" disabled={busy} onclick={() => { const t = d; closeMenu(false); onfromtemplate(t) }}>New desk from this template</button>
    <button type="button" role="menuitem" onclick={() => then('rename')}>Rename</button>
    <button type="button" role="menuitem" class="danger" onclick={() => then('delete')}>Delete</button>
  </DeskMenu>
{:else if menu?.kind === 'templates'}
  <DeskMenu x={menu.x} y={menu.y} label="From a template" opener={menu.opener} onclose={closeMenu}>
    {#each templates as t (t.id)}
      <button type="button" role="menuitem" onclick={() => { closeMenu(false); onfromtemplate(t) }}><span class="swatch" style={`--swatch:${projectColourHex(t.colour)}`}></span>{t.name}</button>
    {/each}
  </DeskMenu>
{/if}

<style>
  .desk-list { width: 100%; max-width: var(--content-width); margin: 0 auto; padding: 32px 40px 56px; }
  .head { display: flex; align-items: flex-end; gap: 16px; flex-wrap: wrap; }
  .head > div:first-child { flex: 1; min-width: 240px; }
  h1 { margin: 0; font: 600 2rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 6px 0 0; color: var(--muted); }
  .actions { display: flex; gap: 8px; flex-wrap: wrap; }
  .notice { margin: 16px 0 0; }
  .notice.bad { color: var(--bad); }
  .empty { margin-top: 40px; padding: 40px 24px; border: 1px dashed var(--line-strong); border-radius: var(--radius); display: flex; flex-direction: column; align-items: center; gap: 10px; text-align: center; }
  .empty h2 { margin: 0; font: 600 1.375rem/1.2 var(--serif); }
  .empty p { margin: 0; color: var(--muted); max-width: 46ch; }
  .group-title { margin: 32px 0 0; font: 600 1.25rem/1.2 var(--serif); letter-spacing: -0.01em; }
  .group .group-title { margin-top: 40px; }
  .hint { margin: 4px 0 0; color: var(--muted); }
  /* One bordered list, hairlines between rows: no card per desk. A row that waits for the person takes the product's
     waiting tint (the whole row, not an edge) and the Open button, so it reads first. */
  .rows { list-style: none; margin: 14px 0 0; padding: 0; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); }
  .row { display: grid; grid-template-columns: 10px minmax(0, 1.3fr) minmax(0, 1fr) auto auto; grid-template-areas: 'sw name meta pills end'; align-items: center; gap: 4px 16px; padding: 10px 12px 10px 16px; min-height: 60px; min-width: 0; }
  .row + .row { border-top: 1px solid var(--line-soft); }
  .row:first-child { border-radius: calc(var(--radius) - 1px) calc(var(--radius) - 1px) 0 0; }
  .row:last-child { border-radius: 0 0 calc(var(--radius) - 1px) calc(var(--radius) - 1px); }
  .row:only-child { border-radius: calc(var(--radius) - 1px); }
  .row[data-status='waiting'] { background: var(--status-wash); }
  .sw { grid-area: sw; width: 10px; height: 10px; border-radius: 3px; background: var(--pc); }
  .name { grid-area: name; min-width: 0; color: var(--ink); font-weight: 600; font-size: 1rem; text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .name:hover { text-decoration: underline; }
  .meta { grid-area: meta; min-width: 0; display: flex; flex-wrap: wrap; gap: 0 12px; color: var(--muted); font-variant-numeric: tabular-nums; }
  .on { overflow-wrap: anywhere; }
  .pills { grid-area: pills; display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 6px; }
  .pill { display: inline-flex; align-items: center; gap: 6px; padding: 2px 8px; border: 1px solid var(--status-edge); border-radius: 6px; background: var(--status-wash); color: var(--status-ink); font-weight: 600; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .pill i { width: 6px; height: 6px; flex: none; border-radius: 50%; background: var(--status); }
  .end { grid-area: end; display: flex; align-items: center; gap: 4px; justify-self: end; }
  .more { width: 32px; height: 32px; flex: none; border: 0; border-radius: 6px; background: none; color: var(--faint); cursor: pointer; font-size: 16px; }
  .more:hover { color: var(--ink); background: var(--line-soft); }
  @media (max-width: 760px) {
    .desk-list { padding: 24px 16px 40px; }
    h1 { font-size: 1.75rem; }
    .row { grid-template-columns: 10px minmax(0, 1fr) auto; grid-template-areas: 'sw name end' '. meta meta' '. pills pills'; padding: 10px 8px 12px 14px; }
    .pills { justify-content: flex-start; margin-top: 4px; }
  }
  @media (pointer: coarse) { .more { width: 44px; height: 44px; } .name { display: flex; align-items: center; min-height: 44px; } .end .btn { min-height: 44px; } }
</style>
