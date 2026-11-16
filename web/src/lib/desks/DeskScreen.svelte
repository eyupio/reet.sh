<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { projectColourHex, type ProjectColour } from '../projectColour'
  import { sessionStatus } from '../sessionStatus'
  import { createTerminal, endTerminal, renameTerminal, TerminalError, terminalClosed, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { getDesk, patchDesk, type Desk, type DeskSummary } from './api'
  import { createAutosave, type Autosave, type Change, type SaveState } from './autosave'
  import { leaves, MAX_TABS, newId, type Pane, type Tab } from './tree'
  import ClosePaneDialog from './ClosePaneDialog.svelte'
  import DeskMenu from './DeskMenu.svelte'
  import DeskPane from './DeskPane.svelte'
  import DeskTabs from './DeskTabs.svelte'
  import NewTabDialog from './NewTabDialog.svelte'

  // One desk: a header (the desk's name as its switcher, the save state and the desk menu), the tab bar and the active
  // tab's pane. Every edit goes through autosave at once and is saved after a short pause; a save that lost to another
  // window is merged onto the newer copy. Opening a desk starts nothing: a shell starts only from New tab.
  let { id, desks, sessions, machines, ready, onopen, onlist, onnew, ondesk, onrefresh, onsignedout }: {
    id: string
    /** Every desk, for the switcher. */
    desks: DeskSummary[]
    sessions: TerminalSession[]
    machines: TerminalMachine[]
    /** Sessions and machines have been read at least once. */
    ready: boolean
    onopen: (id: string) => void
    onlist: () => void
    onnew: () => void
    /** A desk-level action from the menu: the page owns its dialogs and writes. */
    ondesk: (action: 'rename' | 'duplicate' | 'template' | 'delete', desk: Desk) => void
    /** Read the sessions and machines again (after a shell is started or ended here). */
    onrefresh: () => void
    onsignedout: (e: unknown) => void
  } = $props()

  let desk = $state<Desk | null>(null)
  let saveState = $state<SaveState>('saved')
  let failure = $state('')
  let auto: Autosave | undefined
  const panelId = $props.id()

  function start(deskId: string) {
    // Whatever the desk being left still has waiting is sent before that copy stops; its late answers are ignored below.
    const old = auto
    void old?.flush().finally(() => old.stop())
    desk = null; failure = ''; saveState = 'saved'
    const mine = createAutosave({
      load: () => getDesk(deskId),
      save: (d, expect, keepalive) => patchDesk(d.id, { expect, doc: d.doc }, { keepalive }),
      onState: s => { if (auto === mine) saveState = s },
      onDesk: d => { if (auto === mine) desk = d },
    })
    auto = mine
    mine.ready.then(() => {
      if (auto !== mine) return
      const loaded = mine.desk()
      // A desk written by a newer reet is neither shown nor overwritten.
      if (loaded.schema > 1) { mine.stop(); desk = null; failure = 'This desk was saved by a newer version of reet, so it is not shown here.'; return }
      desk = loaded
    }).catch(e => {
      if (auto !== mine) return
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      failure = e instanceof TerminalError && e.status === 404 ? 'That desk is not on this account any more.' : 'This desk could not be read just now. Try again in a moment.'
    })
  }
  $effect(() => { start(id) })
  export function reload() { void auto?.refresh().catch(() => { /* the next save or poll tries again */ }) }

  onMount(() => {
    // Whatever is waiting is sent as the page goes away.
    const leave = () => { void auto?.flush(true) }
    addEventListener('pagehide', leave)
    const visible = () => { if (document.visibilityState === 'hidden') leave(); else reload() }
    document.addEventListener('visibilitychange', visible)
    // The desk screen takes the window's height to the pixel, as the session workspace does.
    document.documentElement.toggleAttribute('data-desk-screen', true)
    return () => {
      removeEventListener('pagehide', leave)
      document.removeEventListener('visibilitychange', visible)
      document.documentElement.removeAttribute('data-desk-screen')
      const last = auto
      auto = undefined
      void last?.flush().finally(() => last.stop())
    }
  })

  function change(c: Change) {
    if (!auto || !desk) return
    auto.change(c)
    desk = auto.desk()
  }

  const tabs = $derived(desk?.doc.tabs ?? [])
  const activeTab = $derived(tabs.find(t => t.id === desk?.doc.active) ?? tabs[0])
  const activePaneId = $derived(activeTab ? (activeTab.panes[activeTab.active] ? activeTab.active : leaves(activeTab.root)[0] ?? '') : '')
  const activePane = $derived(activeTab && activePaneId ? activeTab.panes[activePaneId] : undefined)
  const sessionById = $derived(new Map(sessions.map(s => [s.id, s])))
  const machineById = $derived(new Map(machines.map(m => [m.agentId, m])))
  const live = (p: Pane) => { const s = p.session ? sessionById.get(p.session) : undefined; return !!s && !terminalClosed(s.state) }
  const panesOf = (t: Tab) => leaves(t.root).map(pid => t.panes[pid]).filter((p): p is Pane => !!p)
  const waiting = $derived(new Set(tabs.filter(t => panesOf(t).some(p => { const s = p.session ? sessionById.get(p.session) : undefined; return !!s && sessionStatus(s) === 'waiting' })).map(t => t.id)))
  const hosts = $derived(new Set(tabs.flatMap(t => Object.values(t.panes).map(p => p.host)).filter(Boolean)))
  const others = $derived(desks.filter(d => !d.template && d.id !== id))
  const saveWord = $derived(saveState === 'saving' ? 'Saving…' : saveState === 'unsaved' ? 'Not saved yet' : 'saved')
  const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`

  // Menus in the header: the switcher (the desk's name) and the desk menu (⋮).
  let menu = $state<{ kind: 'switch' | 'desk'; x: number; y: number; opener: HTMLElement } | null>(null)
  function openMenu(kind: 'switch' | 'desk', event: MouseEvent) {
    const opener = event.currentTarget as HTMLElement, box = opener.getBoundingClientRect()
    menu = { kind, opener, x: kind === 'desk' ? box.right - 220 : box.left, y: box.bottom + 4 }
  }
  function closeMenu(focus: boolean) { const opener = menu?.opener; menu = null; if (focus) opener?.focus() }

  async function colourDesk(colour: ProjectColour) {
    closeMenu(true)
    const current = desk
    if (!current || current.colour === colour) return
    try {
      await patchDesk(current.id, { expect: current.version, colour })
    } catch (e) {
      if (e instanceof TerminalError && e.status === 401) { onsignedout(e); return }
      // A stale write is retried once on the newer copy.
      try { const fresh = await getDesk(current.id); await patchDesk(current.id, { expect: fresh.version, colour }) } catch { notice = 'That colour could not be saved. Try again.'; return }
    }
    reload()
  }

  // Tabs.
  function select(tabId: string) { if (tabId !== desk?.doc.active) change({ kind: 'active', tab: tabId }) }
  function duplicateTab(tabId: string) {
    const t = tabs.find(x => x.id === tabId)
    if (!t || tabs.length >= MAX_TABS) return
    // A copy holds the same machines, folders, names and colours; a session is in one pane only, so its panes start empty.
    const ids = new Map(Object.keys(t.panes).map(pid => [pid, newId()]))
    const copyNode = (n: Tab['root']): Tab['root'] => !n ? null : 'split' in n ? { split: n.split, ratio: n.ratio, a: copyNode(n.a)!, b: copyNode(n.b)! } : { pane: ids.get(n.pane)! }
    const panes: Record<string, Pane> = {}
    for (const [pid, p] of Object.entries(t.panes)) panes[ids.get(pid)!] = { ...p, id: ids.get(pid)!, session: '' }
    const name = [...`${t.name} copy`].slice(0, 80).join('')
    const copy: Tab = { id: newId(), name, colour: t.colour, root: copyNode(t.root), zoomed: '', active: ids.get(t.active) ?? '', panes }
    change({ kind: 'addTab', tab: copy, index: tabs.indexOf(t) + 1 })
  }

  // Closing: a pane or tab with a running shell asks first; one with nothing running closes at once.
  let closing = $state<{ tab: string; pane?: string; name: string; host: string; live: Pane[] } | null>(null)
  let notice = $state('')
  function askClose(tabId: string, paneId?: string) {
    const t = tabs.find(x => x.id === tabId)
    if (!t) return
    const covered = paneId ? [t.panes[paneId]].filter(Boolean) : panesOf(t)
    const running = covered.filter(live)
    const name = paneId ? (t.panes[paneId]?.name || t.name) : t.name
    if (running.length === 0) { removeFromDoc(tabId, paneId); return }
    closing = { tab: tabId, pane: paneId, name: name || 'Untitled', host: machineById.get(running[0].host)?.host ?? 'its machine', live: running }
  }
  function removeFromDoc(tabId: string, paneId?: string) {
    const t = tabs.find(x => x.id === tabId)
    if (!t) return
    // The last pane of a tab takes the tab with it.
    if (!paneId || Object.keys(t.panes).length <= 1) change({ kind: 'closeTab', tab: tabId })
    else change({ kind: 'closePane', tab: tabId, pane: paneId })
  }
  async function endAndClose() {
    const c = closing
    closing = null
    if (!c) return
    removeFromDoc(c.tab, c.pane)
    const results = await Promise.allSettled(c.live.map(p => endTerminal(p.session, crypto.randomUUID(), true)))
    const failed = results.filter(r => r.status === 'rejected')
    if (failed.length) notice = failed.length === 1 ? 'Its shell could not be ended. It is still in Sessions.' : `${failed.length} shells could not be ended. They are still in Sessions.`
    onrefresh()
  }
  function takeOff() {
    const c = closing
    closing = null
    if (c) removeFromDoc(c.tab, c.pane)
  }

  // Panes.
  function renamePane(t: Tab, p: Pane, name: string) {
    change({ kind: 'renamePane', tab: t.id, pane: p.id, name })
    // Sessions and the desk agree on the name.
    if (live(p)) renameTerminal(p.session, name).then(onrefresh, () => { /* the desk keeps its name; Sessions keeps the old one */ })
  }
  async function endPane(p: Pane) {
    if (!p.session) return
    try { await endTerminal(p.session, crypto.randomUUID(), false) } catch (e) { notice = e instanceof Error && e.message ? e.message : 'That shell could not be ended. Try again.' }
    onrefresh()
  }

  // New tab: a plain shell, then the tab that holds it.
  let adding = $state<HTMLElement | null>(null)
  const startAgent = $derived(activePane?.host ?? '')
  const startDir = $derived.by(() => { const s = activePane?.session ? sessionById.get(activePane.session) : undefined; return s?.dir || activePane?.dir || '' })
  async function createTab(agentId: string, dir: string) {
    // The tab goes to the desk the dialog was opened on, even if another desk is shown by the time the shell starts.
    const mine = auto
    const name = [...(dir.split('/').filter(Boolean).pop() || machineById.get(agentId)?.host || 'Shell')].slice(0, 80).join('')
    const created = await createTerminal(agentId, crypto.randomUUID(), name, 100, 30, AbortSignal.timeout(15000), dir ? { dir } : undefined)
    const pid = newId()
    const pane: Pane = { id: pid, name, colour: '', host: agentId, session: created.id, kind: 'shell', dir: created.dir || dir, dirAt: Date.now(), start: '' }
    const tab: Tab = { id: newId(), name, colour: '', root: { pane: pid }, zoomed: '', active: pid, panes: { [pid]: pane } }
    onrefresh()
    if (!mine || auto !== mine) { notice = `The shell “${name}” started, but its desk is no longer open. It is in Sessions.`; return }
    change({ kind: 'addTab', tab })
    change({ kind: 'active', tab: tab.id })
  }
  async function closeNewTab() { const opener = adding; adding = null; await tick(); opener?.focus() }
</script>

<section class="desk-screen" aria-label={desk ? `Desk ${desk.name}` : 'Desk'}>
  {#if failure}
    <div class="gone"><p role="alert">{failure}</p><button type="button" class="btn" onclick={onlist}>All desks</button></div>
  {:else if !desk}
    <div class="dhead"><span class="loading" role="status">Opening the desk…</span></div>
  {:else}
    <header class="dhead" data-keeps-focus>
      <button type="button" class="dname" aria-haspopup="menu" aria-expanded={menu?.kind === 'switch'} aria-label={`${desk.name}, switch desk`} onclick={event => openMenu('switch', event)}>
        <span class="sw" style={`--swatch:${projectColourHex(desk.colour)}`} aria-hidden="true"></span>
        <h1>{desk.name}</h1><span class="chev" aria-hidden="true">▾</span>
      </button>
      <span class="sub">{plural(tabs.length, 'tab')} · {plural(hosts.size, 'machine')} · <span class="save" data-save={saveState} role="status">{saveWord}</span></span>
      <span class="sp"></span>
      <button type="button" class="btn icon" aria-label="Desk menu" aria-haspopup="menu" aria-expanded={menu?.kind === 'desk'} onclick={event => openMenu('desk', event)}>⋮</button>
    </header>
    {#if saveState === 'stale'}<p class="notice" role="status">This desk was changed in another window. Showing the latest.</p>{/if}
    {#if notice}<p class="notice bad" role="alert">{notice} <button type="button" class="btn quiet" onclick={() => (notice = '')}>Dismiss</button></p>{/if}
    <DeskTabs {tabs} active={activeTab?.id ?? ''} {panelId} {waiting} full={tabs.length >= MAX_TABS}
      onselect={select}
      onrename={(tab, name) => change({ kind: 'renameTab', tab, name })}
      oncolour={(tab, colour) => change({ kind: 'colourTab', tab, colour })}
      onduplicate={duplicateTab}
      onclose={tabId => askClose(tabId)}
      onreorder={ids => change({ kind: 'reorderTabs', ids })}
      onnew={opener => (adding = opener)} />
    <div class="area" id={panelId} role="tabpanel" aria-labelledby={activeTab ? `desk-tab-${activeTab.id}` : undefined}>
      {#if activeTab && activePane}
        {#key activePane.id}
          <DeskPane tab={activeTab} pane={activePane} session={activePane.session ? sessionById.get(activePane.session) : undefined} machine={machineById.get(activePane.host)} {ready}
            onrename={name => renamePane(activeTab, activePane, name)}
            oncolour={colour => change({ kind: 'colourPane', tab: activeTab.id, pane: activePane.id, colour })}
            onclose={() => askClose(activeTab.id, activePane.id)}
            onend={() => void endPane(activePane)} />
        {/key}
      {:else}
        <div class="empty">
          <h2>{tabs.length ? 'This tab is empty.' : 'Nowt on this desk yet.'}</h2>
          <p>A tab holds a shell on one of your machines, in the folder you choose.</p>
          <button type="button" class="btn primary" disabled={tabs.length >= MAX_TABS} onclick={event => (adding = event.currentTarget as HTMLElement)}>New tab</button>
        </div>
      {/if}
    </div>
  {/if}
</section>

{#if menu?.kind === 'switch' && desk}
  <DeskMenu x={menu.x} y={menu.y} label="Switch desk" opener={menu.opener} onclose={closeMenu}>
    {#each others as d (d.id)}
      <button type="button" role="menuitem" onclick={() => { closeMenu(false); onopen(d.id) }}><span class="swatch" style={`--swatch:${projectColourHex(d.colour)}`}></span>{d.name}</button>
    {/each}
    {#if others.length}<hr />{/if}
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); onlist() }}>All desks</button>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); onnew() }}>New desk</button>
  </DeskMenu>
{:else if menu?.kind === 'desk' && desk}
  {@const current = desk}
  <DeskMenu x={menu.x} y={menu.y} label="Desk menu" opener={menu.opener} onclose={closeMenu} colour={current.colour} oncolour={c => void colourDesk(c)}>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); ondesk('rename', current) }}>Rename</button>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); ondesk('duplicate', current) }}>Duplicate</button>
    <button type="button" role="menuitem" onclick={() => { closeMenu(false); ondesk('template', current) }}>Save as template</button>
    <button type="button" role="menuitem" class="danger" onclick={() => { closeMenu(false); ondesk('delete', current) }}>Delete</button>
  </DeskMenu>
{/if}

{#if closing}
  <ClosePaneDialog name={closing.name} host={closing.host} count={closing.live.length} onend={() => void endAndClose()} ontakeoff={takeOff} oncancel={() => (closing = null)} />
{/if}

{#if adding}
  <NewTabDialog {machines} agentId={startAgent} dir={startDir} oncreate={createTab} onclose={() => void closeNewTab()} />
{/if}

<style>
  .desk-screen { flex: 1; min-height: 0; display: flex; flex-direction: column; background: var(--bg); }
  .dhead { flex: none; display: flex; align-items: center; gap: 12px; padding: 8px 14px; border-bottom: 1px solid var(--line-soft); min-height: 52px; }
  .dname { display: flex; align-items: center; gap: 8px; min-height: 40px; min-width: 0; padding: 4px 8px; margin-left: -8px; border: 0; border-radius: 8px; background: none; color: var(--ink); cursor: pointer; }
  .dname:hover { background: var(--line-soft); }
  .dname h1 { margin: 0; font: 600 1.0625rem/1.2 var(--sans); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .chev { color: var(--muted); }
  .sw { width: 10px; height: 10px; border-radius: 3px; flex: none; background: var(--swatch); border: 1px solid var(--line-strong); }
  .sub { color: var(--faint); font-size: 14px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
  .save[data-save='unsaved'] { color: var(--warn); }
  .sp { margin-left: auto; }
  .btn.icon { min-width: 40px; padding: 6px 10px; }
  @media (pointer: coarse) { .dname, .btn.icon { min-height: 44px; } .btn.icon { min-width: 44px; } }
  .notice { flex: none; margin: 0; padding: 6px 14px; background: color-mix(in srgb, var(--info) 14%, var(--bg)); border-bottom: 1px solid var(--line-soft); color: var(--ink); font-size: 14px; }
  .notice.bad { background: color-mix(in srgb, var(--bad) 12%, var(--bg)); display: flex; align-items: center; gap: 8px; }
  .area { flex: 1; min-height: 0; display: flex; padding: 6px; background: var(--line-soft); }
  .empty { margin: auto; display: flex; flex-direction: column; align-items: center; gap: 10px; text-align: center; padding: 24px; }
  .empty h2 { margin: 0; font: 600 1.375rem/1.2 var(--serif); }
  .empty p { margin: 0; color: var(--muted); max-width: 44ch; }
  .gone { margin: 48px auto; display: flex; flex-direction: column; align-items: center; gap: 12px; }
  .gone p { margin: 0; color: var(--muted); }
  .loading { color: var(--muted); }
  :global(html[data-desk-screen] #app) { height: 100dvh; min-height: 0; }
  :global(html[data-desk-screen] main#main-content) { min-height: 0; display: flex; flex-direction: column; overflow: hidden; padding-bottom: 0; }
  @media (max-width: 700px) {
    .dhead { padding: 6px 10px; gap: 8px; }
    .sub { display: none; }
    .area { padding: 0; }
  }
</style>
