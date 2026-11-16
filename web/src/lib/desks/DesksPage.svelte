<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { getProfile, UnauthorizedError } from '../api'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import { shell } from '../dialogShell'
  import type { ProjectColour } from '../projectColour'
  import { endTerminal, terminalClosed, terminalMachines, terminalSessions, TerminalError, type TerminalMachine, type TerminalSession } from '../terminal/api'
  import { createDesk, deleteDesk, listDesks, patchDesk, StaleDesk, type Desk, type DeskSummary } from './api'
  import DeskList from './DeskList.svelte'
  import DeskScreen from './DeskScreen.svelte'

  // The Desks tab: /desks lists the desks and /desks/<id> opens one. /desks opens the desk last used in this browser
  // when it is still there (the one id kept in this browser), else the list. The sessions and machines the panes need
  // are read here, every 15 seconds while the page is in view, as Sessions does.
  let { userId, onsignedout, onscreen }: {
    userId: number
    onsignedout: (e: unknown) => void
    /** Told whether a desk (rather than the list) is open, so the shell can give it the whole window. */
    onscreen?: (desk: boolean) => void
  } = $props()

  const POLL_MS = 15_000
  // svelte-ignore state_referenced_locally
  const lastKey = `reet.desks.last.${userId}`
  const readLast = () => { try { return localStorage.getItem(lastKey) ?? '' } catch { return '' } }
  const writeLast = (id: string) => { try { if (id) localStorage.setItem(lastKey, id); else localStorage.removeItem(lastKey) } catch { /* only a convenience */ } }

  type Route = { kind: 'list' } | { kind: 'desk'; id: string }
  function parse(): Route {
    const m = location.pathname.match(/^\/desks\/([^/]+)\/?$/)
    if (m) { try { return { kind: 'desk', id: decodeURIComponent(m[1]) } } catch { /* a malformed address is the list */ } }
    return { kind: 'list' }
  }
  let route = $state<Route>(parse())
  // /desks waits for the list before it decides between the last desk and the list, so the list does not flash.
  // svelte-ignore state_referenced_locally
  let deciding = $state(route.kind === 'list')
  let desks = $state<DeskSummary[]>([]), listed = $state(false), listFailed = $state(false)
  let sessions = $state<TerminalSession[]>([]), machines = $state<TerminalMachine[]>([]), ready = $state(false)
  let busy = $state(false), notice = $state<{ text: string; bad: boolean } | null>(null)
  // The person's setting for a session that ends: it leaves Sessions, or stays there as ended (the safe default keeps it).
  let removeOnClose = $state(false)
  let screen = $state<DeskScreen>()
  const life = new AbortController()

  $effect(() => { onscreen?.(route.kind === 'desk') })
  $effect(() => { if (route.kind === 'desk') writeLast(route.id) })

  function signedOut(e: unknown): boolean {
    if (e instanceof UnauthorizedError) { onsignedout(e); return true }
    if (e instanceof TerminalError && e.status === 401) { onsignedout(new UnauthorizedError()); return true }
    return false
  }
  const bounded = () => AbortSignal.any([life.signal, AbortSignal.timeout(15_000)])

  async function loadDesks() {
    try {
      // A desk written by a newer reet is never listed (the controller leaves it out; this is the browser's own check).
      desks = (await listDesks()).filter(d => !((d as { schema?: number }).schema! > 1))
      listed = true; listFailed = false
    } catch (e) {
      if (signedOut(e)) return
      listFailed = true
    }
  }
  async function loadLive() {
    const [s, m] = await Promise.allSettled([terminalSessions(bounded()), terminalMachines(bounded())])
    if (s.status === 'fulfilled') sessions = s.value; else if (signedOut(s.reason)) return
    if (m.status === 'fulfilled') machines = m.value; else if (signedOut(m.reason)) return
    if (s.status === 'fulfilled' && m.status === 'fulfilled') ready = true
  }
  const refreshLive = () => { void loadLive() }

  onMount(() => {
    void getProfile().then(p => { removeOnClose = p.profile.terminalRemoveOnClose ?? false }).catch(() => { /* the safe default keeps history */ })
    void loadLive()
    void loadDesks().then(() => {
      if (route.kind === 'list' && deciding) {
        const last = readLast()
        if (last && desks.some(d => d.id === last && !d.template)) go({ kind: 'desk', id: last }, true)
        else if (last) writeLast('')
      }
      deciding = false
    })
    const poll = () => { if (document.visibilityState === 'visible') { void loadLive(); void loadDesks() } }
    const timer = setInterval(poll, POLL_MS)
    document.addEventListener('visibilitychange', poll)
    const back = () => { route = parse(); deciding = false }
    addEventListener('popstate', back)
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', poll); removeEventListener('popstate', back); life.abort() }
  })

  function go(next: Route, replace = false) {
    const path = next.kind === 'desk' ? `/desks/${encodeURIComponent(next.id)}` : '/desks'
    if (replace) history.replaceState(history.state, '', path); else history.pushState(null, '', path)
    route = next
    notice = null
  }
  const open = (id: string) => go({ kind: 'desk', id })
  const showList = () => { void loadDesks(); go({ kind: 'list' }) }

  /** A name no other desk has: `base`, then `base 2`, `base 3`… (names are unique, ignoring case). */
  function freeName(base: string): string {
    const taken = new Set(desks.map(d => d.name.toLocaleLowerCase()))
    const cut = (s: string) => [...s].slice(0, 80).join('')
    if (!taken.has(cut(base).toLocaleLowerCase())) return cut(base)
    for (let n = 2; ; n++) { const name = `${cut(base).slice(0, 76)} ${n}`; if (!taken.has(name.toLocaleLowerCase())) return name }
  }
  function failed(e: unknown, fallback: string) {
    if (signedOut(e)) return
    notice = { text: e instanceof Error && e.message ? e.message : fallback, bad: true }
  }

  async function newDesk() {
    if (busy) return
    busy = true; notice = null
    try {
      await loadDesks()
      const created = await createDesk({ name: freeName('Desk') })
      await loadDesks()
      open(created.id)
    } catch (e) { failed(e, 'A new desk could not be made just now. Try again.') } finally { busy = false }
  }
  async function fromTemplate(t: Pick<DeskSummary, 'id' | 'name' | 'colour'>) {
    if (busy) return
    busy = true; notice = null
    try {
      const created = await createDesk({ name: freeName(t.name.replace(/ template$/i, '') || 'Desk'), colour: t.colour, from: t.id })
      await loadDesks()
      open(created.id)
    } catch (e) { failed(e, 'That template could not be used just now. Try again.') } finally { busy = false }
  }

  // A write to a desk's own record (name, colour). A stale one is tried once more on the newer version.
  async function patchRecord(id: string, version: number, input: { name?: string; colour?: ProjectColour }) {
    try { await patchDesk(id, { expect: version, ...input }) } catch (e) {
      if (!(e instanceof StaleDesk)) throw e
      await patchDesk(id, { expect: e.desk.version, ...input })
    }
    await loadDesks()
    screen?.reload()
  }
  async function colour(d: DeskSummary, c: ProjectColour) {
    try { await patchRecord(d.id, d.version, { colour: c }) } catch (e) { failed(e, 'That colour could not be saved. Try again.') }
  }

  type Target = Pick<DeskSummary, 'id' | 'name' | 'colour' | 'version'> & { sessions: string[] }
  const target = (d: DeskSummary | Desk): Target => 'sessions' in d ? d : { id: d.id, name: d.name, colour: d.colour, version: d.version, sessions: d.doc.tabs.flatMap(t => Object.values(t.panes).map(p => p.session)).filter(Boolean) }

  let renaming = $state<Target | null>(null), renameDraft = $state(''), renameError = $state(''), renameDialog = $state<HTMLDialogElement>(), renameInput = $state<HTMLInputElement>()
  let deleting = $state<Target | null>(null)
  let opener: HTMLElement | null = null
  async function desk(action: 'rename' | 'duplicate' | 'template' | 'delete', d: DeskSummary | Desk) {
    const t = target(d)
    opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
    if (action === 'rename') {
      renaming = t; renameDraft = t.name; renameError = ''
      await tick()
      renameDialog?.showModal(); renameInput?.select()
    } else if (action === 'delete') deleting = t
    else if (!busy) {
      busy = true; notice = null
      const template = action === 'template'
      try {
        await loadDesks()
        const created = await createDesk({ name: freeName(template ? `${t.name} template` : `${t.name} copy`), colour: t.colour, from: t.id, template })
        await loadDesks()
        notice = { text: template ? `Sorted. “${created.name}” is saved as a template.` : `Sorted. “${created.name}” added.`, bad: false }
      } catch (e) { failed(e, template ? 'That desk could not be saved as a template. Try again.' : 'That desk could not be copied. Try again.') } finally { busy = false }
    }
  }
  function closeRename() { renameDialog?.close(); renaming = null; opener?.focus() }
  async function saveRename(event: SubmitEvent) {
    event.preventDefault()
    const t = renaming, name = renameDraft.trim()
    if (!t || busy) return
    if (!name) { renameError = 'Give the desk a name.'; return }
    if ([...name].length > 80) { renameError = 'Keep the name to 80 characters.'; return }
    if (name === t.name) { closeRename(); return }
    busy = true
    try { await patchRecord(t.id, t.version, { name }); closeRename() }
    catch (e) { if (!signedOut(e)) renameError = e instanceof Error && e.message ? e.message : 'That name could not be saved. Try again.' }
    finally { busy = false }
  }

  const liveIn = (t: Target) => t.sessions.filter(id => { const s = sessions.find(x => x.id === id); return !!s && !terminalClosed(s.state) }).length
  async function remove(endSessions: boolean) {
    const t = deleting
    deleting = null
    if (!t) return
    busy = true; notice = null
    try {
      let version = t.version, sessionIds = t.sessions
      let answer: { left?: string[] }
      try { answer = await deleteDesk(t.id, { expect: version, endSessions }) } catch (e) {
        if (!(e instanceof StaleDesk)) throw e
        version = e.desk.version
        sessionIds = target(e.desk).sessions
        answer = await deleteDesk(t.id, { expect: version, endSessions })
      }
      if (readLast() === t.id) writeLast('')
      // A pane whose shell had already ended is gone for good: its machine forgets the pane's own command history, and
      // the session stays in Sessions as it is. Nothing is said if that fails (the file is the person's own).
      for (const id of sessionIds) {
        const s = sessions.find(x => x.id === id)
        if (s && terminalClosed(s.state)) void endTerminal(id, crypto.randomUUID(), false, '', true).catch(() => { /* see above */ })
      }
      await loadDesks()
      const left = Array.isArray(answer?.left) ? answer.left.length : 0
      notice = left
        ? { text: `Sorted. “${t.name}” is deleted. ${left === 1 ? 'One of its shells could not be ended. It is still in Sessions.' : `${left} of its shells could not be ended. They are still in Sessions.`}`, bad: true }
        : { text: `Sorted. “${t.name}” is deleted.`, bad: false }
      if (route.kind === 'desk' && route.id === t.id) go({ kind: 'list' }, true)
      if (endSessions) void loadLive()
    } catch (e) { failed(e, 'That desk could not be deleted. Try again.') } finally { busy = false }
  }
</script>

{#if notice}<p class="page-notice" class:bad={notice.bad} role={notice.bad ? 'alert' : 'status'}>{notice.text}</p>{/if}
{#if route.kind === 'desk'}
  <DeskScreen bind:this={screen} id={route.id} {desks} {sessions} {machines} {ready} {removeOnClose}
    onopen={open} onlist={showList} onnew={() => void newDesk()} ondesk={(a, d) => void desk(a, d)} onfromtemplate={t => void fromTemplate(t)} onrefresh={refreshLive} onsignedout={e => signedOut(e) || onsignedout(e)} />
{:else if deciding || !listed && !listFailed}
  <div class="loading" role="status">Reading your desks…</div>
{:else}
  <DeskList {desks} {sessions} {machines} failed={listFailed} {busy} onopen={open} onnew={() => void newDesk()} onfromtemplate={t => void fromTemplate(t)} ondesk={(a, d) => void desk(a, d)} oncolour={(d, c) => void colour(d, c)} />
{/if}

{#if renaming}
  <dialog bind:this={renameDialog} class="rename" use:shell={{ sheet: 'bottom' }} aria-labelledby="desk-rename-title" oncancel={event => { event.preventDefault(); closeRename() }}>
    <form class="box" data-shell-card onsubmit={saveRename}>
      <h2 id="desk-rename-title">Rename “{renaming.name}”</h2>
      <label class="field"><span>Name</span><input bind:this={renameInput} bind:value={renameDraft} maxlength="80" aria-invalid={renameError ? 'true' : undefined} autocomplete="off" /></label>
      {#if renameError}<p class="bad" role="alert">{renameError}</p>{/if}
      <div class="actions" data-shell-foot>
        <button type="button" class="btn quiet" onclick={closeRename}>Cancel</button>
        <button type="submit" class="btn primary" disabled={busy} aria-busy={busy}>Rename</button>
      </div>
    </form>
  </dialog>
{/if}

{#if deleting}
  {@const running = liveIn(deleting)}
  {#if running}
    <ConfirmDialog title={`Delete “${deleting.name}”?`} body={`${running === 1 ? 'One of its shells is' : `${running} of its shells are`} still running. End them, or leave them running in Sessions.`}
      confirmLabel="End its shells" danger secondaryLabel="Leave them running in Sessions" onconfirm={() => void remove(true)} onsecondary={() => void remove(false)} oncancel={() => { deleting = null; opener?.focus() }}>
      <p class="end-note"><b>End its shells:</b> Its shells are ended and leave Sessions, and their panes' command history is removed.</p>
    </ConfirmDialog>
  {:else}
    <ConfirmDialog title={`Delete “${deleting.name}”?`} body="Its tabs and panes are removed. Nothing is running in it." confirmLabel="Delete" danger onconfirm={() => void remove(false)} oncancel={() => { deleting = null; opener?.focus() }} />
  {/if}
{/if}

<style>
  .page-notice { flex: none; margin: 0; padding: 8px 16px; border-bottom: 1px solid var(--line-soft); background: color-mix(in srgb, var(--good) 12%, var(--bg)); font-size: 14px; }
  .page-notice.bad { background: color-mix(in srgb, var(--bad) 12%, var(--bg)); color: var(--ink); }
  .loading { margin: 48px auto; color: var(--muted); }
  dialog { width: min(440px, calc(100vw - 32px)); max-width: none; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); }
  .box { display: flex; flex-direction: column; gap: 12px; padding: 24px; background: var(--card-raised); border: 1px solid var(--line-strong); border-radius: 16px; box-shadow: var(--shadow); }
  h2 { margin: 0; font: 600 1.5rem/1.15 var(--serif); letter-spacing: -0.02em; overflow-wrap: anywhere; }
  .field { display: flex; flex-direction: column; gap: 6px; font-weight: 600; }
  .bad { margin: 0; color: var(--bad); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
  .end-note { margin: 8px 0 0; color: var(--muted); overflow-wrap: anywhere; }
  .end-note b { color: var(--ink); font-weight: 600; }
</style>
