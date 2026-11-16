<script lang="ts">
  import { projectColourHex, type ProjectColour } from '../projectColour'
  import { sessionStatus } from '../sessionStatus'
  import { terminalClosed, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import DeskMenu from './DeskMenu.svelte'
  import type { DeskSummary } from './api'

  // Every desk, as cards in the person's order: name and colour, tab and pane counts, the machines it spans, how many of
  // its shells are live and how many wait for the person. New desk starts an empty one; From a template copies a template.
  // The templates follow as a second group of cards, each opening the template's own page, with Rename and Delete.
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

  const cards = $derived(desks.filter(d => !d.template))
  const templates = $derived(desks.filter(d => d.template))
  const byId = $derived(new Map(sessions.map(s => [s.id, s])))
  const hostName = $derived(new Map(machines.map(m => [m.agentId, m.host])))
  const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`
  function counts(d: DeskSummary) {
    let live = 0, waiting = 0
    for (const id of d.sessions) {
      const s = byId.get(id)
      if (!s || terminalClosed(s.state)) continue
      live++
      if (sessionStatus(s) === 'waiting') waiting++
    }
    return { live, waiting, reopen: d.panes - live }
  }

  let menu = $state<{ kind: 'card' | 'template' | 'templates'; desk?: DeskSummary; x: number; y: number; opener: HTMLElement } | null>(null)
  function openMenu(event: MouseEvent, kind: 'card' | 'template' | 'templates', desk?: DeskSummary) {
    event.preventDefault()
    const opener = event.currentTarget as HTMLElement, box = opener.getBoundingClientRect()
    menu = { kind, desk, opener, x: kind === 'templates' ? box.left : box.right - 220, y: box.bottom + 4 }
  }
  /** A card's name opens it in this tab; a modified click is left to the browser. */
  function follow(event: MouseEvent, id: string) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onopen(id)
  }
  function closeMenu(focus: boolean) { const opener = menu?.opener; menu = null; if (focus) opener?.focus() }
  function then(action: 'rename' | 'duplicate' | 'template' | 'delete') { const d = menu?.desk; closeMenu(false); if (d) ondesk(action, d) }
</script>

<section class="desk-list" aria-labelledby="desks-title">
  <header class="head">
    <div>
      <h1 id="desks-title">Desks</h1>
      <p class="lede">Saved sets of terminals. Each one comes back as you left it.</p>
    </div>
    <div class="actions">
      <button type="button" class="btn" aria-haspopup="menu" aria-expanded={menu?.kind === 'templates'} disabled={busy || templates.length === 0}
        title={templates.length ? undefined : 'Save a desk as a template from its menu to start others from it.'} onclick={event => openMenu(event, 'templates')}>From a template ▾</button>
      <button type="button" class="btn primary" disabled={busy} aria-busy={busy} onclick={onnew}>New desk</button>
    </div>
  </header>
  {#if failed}<p class="notice bad" role="alert">The desks could not be read just now. reet will try again shortly.</p>{/if}
  {#if cards.length === 0 && !failed}
    <div class="empty">
      <h2>No desks yet.</h2>
      <p>A desk keeps a set of terminals together: their tabs, their folders and their history.</p>
      <button type="button" class="btn primary" disabled={busy} onclick={onnew}>New desk</button>
    </div>
  {:else}
    <ul class="cards">
      {#each cards as d (d.id)}
        {@const c = counts(d)}
        <li class="card" style={`--pc:${d.colour ? projectColourHex(d.colour) : 'var(--line-strong)'}`}>
          <div class="t">
            <span class="sw" aria-hidden="true"></span>
            <a href={`/desks/${encodeURIComponent(d.id)}`} onclick={event => follow(event, d.id)}>{d.name}</a>
            <button type="button" class="more" aria-label={`Actions for ${d.name}`} aria-haspopup="menu" aria-expanded={menu?.desk?.id === d.id} onclick={event => openMenu(event, 'card', d)}>⋮</button>
          </div>
          <div class="mini" aria-hidden="true">{#each Array.from({ length: Math.max(1, Math.min(d.tabs, 6)) }) as _, i (i)}<i></i>{/each}</div>
          <div class="meta">
            <span>{plural(d.tabs, 'tab')} · {plural(d.panes, 'pane')}</span>
            {#if d.hosts.length}<span>{d.hosts.map(h => hostName.get(h) ?? 'a removed machine').join(', ')}</span>{/if}
          </div>
          {#if c.waiting || c.live || (d.panes && c.reopen)}
            <div class="pills">
              {#if c.waiting}<span class="pill w">{c.waiting} need{c.waiting === 1 ? 's' : ''} you</span>{/if}
              {#if c.live}<span class="pill">{c.live} live</span>{/if}
              {#if d.panes && c.reopen}<span class="pill">{c.reopen} to reopen</span>{/if}
            </div>
          {/if}
        </li>
      {/each}
      <li class="card new">
        <button type="button" disabled={busy} onclick={onnew}><b>New desk</b><span>{templates.length ? `Start empty, or from “${templates[0].name}”.` : 'Start empty, then add a tab for each shell.'}</span></button>
      </li>
    </ul>
  {/if}
  {#if templates.length}
    <section class="group" aria-labelledby="desk-templates-title">
      <h2 id="desk-templates-title">Templates</h2>
      <ul class="cards">
        {#each templates as d (d.id)}
          <li class="card tpl" style={`--pc:${d.colour ? projectColourHex(d.colour) : 'var(--line-strong)'}`}>
            <div class="t">
              <span class="sw" aria-hidden="true"></span>
              <a href={`/desks/${encodeURIComponent(d.id)}`} onclick={event => follow(event, d.id)}>{d.name}</a>
              <button type="button" class="more" aria-label={`Actions for ${d.name}`} aria-haspopup="menu" aria-expanded={menu?.desk?.id === d.id} onclick={event => openMenu(event, 'template', d)}>⋮</button>
            </div>
            <div class="mini" aria-hidden="true">{#each Array.from({ length: Math.max(1, Math.min(d.tabs, 6)) }) as _, i (i)}<i></i>{/each}</div>
            <div class="meta">
              <span>{plural(d.tabs, 'tab')} · {plural(d.panes, 'pane')}</span>
              {#if d.hosts.length}<span>{d.hosts.map(h => hostName.get(h) ?? 'a removed machine').join(', ')}</span>{/if}
            </div>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
</section>

{#if menu?.kind === 'card' && menu.desk}
  {@const d = menu.desk}
  <DeskMenu x={menu.x} y={menu.y} label={`Actions for ${d.name}`} opener={menu.opener} onclose={closeMenu} colour={d.colour} oncolour={colour => { const t = d; closeMenu(true); oncolour(t, colour) }}>
    <button type="button" role="menuitem" onclick={() => { const id = d.id; closeMenu(false); onopen(id) }}>Open</button>
    <button type="button" role="menuitem" onclick={() => then('rename')}>Rename</button>
    <button type="button" role="menuitem" onclick={() => then('duplicate')}>Duplicate</button>
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
  .cards { list-style: none; margin: 24px 0 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 14px; }
  .card { background: var(--card); border: 1px solid var(--line); border-radius: var(--radius); padding: 14px; display: flex; flex-direction: column; gap: 10px; min-width: 0; }
  .t { display: flex; align-items: center; gap: 8px; font-weight: 600; font-size: 1rem; min-width: 0; }
  .t a { color: var(--ink); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; flex: 1; }
  .t a:hover { text-decoration: underline; }
  .sw { width: 10px; height: 10px; border-radius: 3px; flex: none; background: var(--pc); }
  .more { margin-left: auto; width: 32px; height: 32px; flex: none; border: 0; border-radius: 6px; background: none; color: var(--faint); cursor: pointer; font-size: 16px; }
  .more:hover { color: var(--ink); background: var(--line-soft); }
  .mini { display: grid; grid-auto-flow: column; grid-auto-columns: 1fr; gap: 2px; height: 56px; padding: 2px; border-radius: 6px; background: var(--line-soft); }
  .mini i { border-radius: 3px; background: var(--bg-sunk); border-left: 2px solid var(--pc); }
  .meta { display: flex; flex-wrap: wrap; gap: 4px 12px; color: var(--muted); font-size: 14px; }
  .pills { display: flex; flex-wrap: wrap; gap: 6px; }
  .pill { font-size: 14px; border-radius: 999px; padding: 0 9px; border: 1px solid var(--line); color: var(--muted); }
  .pill.w { border-color: var(--status-waiting); color: var(--status-waiting); }
  .card.new { padding: 0; border-style: dashed; background: none; }
  .card.new button { flex: 1; min-height: 150px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 4px; padding: 14px; border: 0; border-radius: var(--radius); background: none; color: var(--muted); cursor: pointer; text-align: center; }
  .card.new button:hover:not(:disabled) { color: var(--ink); background: var(--card); }
  .card.new b { color: var(--ink); }
  .group { margin-top: 32px; }
  .group h2 { margin: 0; font: 600 1.25rem/1.2 var(--serif); letter-spacing: -0.01em; }
  .group .cards { margin-top: 14px; }
  @media (max-width: 760px) { .desk-list { padding: 24px 16px 40px; } h1 { font-size: 1.75rem; } }
  @media (pointer: coarse) { .more { width: 44px; height: 44px; } .t a { line-height: 44px; } }
</style>
