<script lang="ts">
  import { onMount, tick } from 'svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import Skeleton from '../Skeleton.svelte'
  import SkillRowView from './SkillRow.svelte'
  import NewOnMachineDialog from './NewOnMachineDialog.svelte'
  import SkillsBulkBar from './SkillsBulkBar.svelte'
  import UndoToast, { type UndoMessage } from './UndoToast.svelte'
  import { addSkill, ApiError, deleteSkills, patchSkill, retrySkill, setSkillStatus, skillNotice, tagSkills, undeleteSkills, UnauthorizedError, type SkillRow } from '../api'
  import { deleteConfirm, focusAfterRemoval, syncSentence, toastText } from './skillsPage'
  import { other, SkillsPageState, TABS, type Tab } from './skillsState.svelte'

  // The Skills page: Active and Archived, one list, pruning in place. Archive, Delete and Restore never switch the tab,
  // scroll the page or open another one: the row leaves the list where it is, focus moves to the next row's button
  // (the one before when it was last, the tab when none is left), and a toast names the skill and the machines with Undo.
  // What the page holds, and how it reads and writes it, is in skillsState.svelte.ts.
  let { canWrite }: { canWrite: boolean } = $props()

  type Flags = { written: boolean; noCopy: boolean }
  type Act = 'archive' | 'restore' | 'delete'
  // How often the page reads where its skills stand on the machines.
  const POLL_MS = 15000

  const page = new SkillsPageState()

  let selecting = $state(false)
  let selected = $state<string[]>([])
  let openId = $state('')

  let addInput = $state('')
  let adding = $state(false)
  let addError = $state('')
  let addEl = $state<HTMLInputElement>()

  type Ask = { title: string; body: string; rows: SkillRow[]; flags: Flags; order: string[] }
  let ask = $state<Ask | null>(null)
  let reviewOpen = $state(false)

  let toast = $state<UndoMessage | null>(null)
  let toastSeq = 0

  let listEl = $state<HTMLElement>()
  let reviewEl = $state<HTMLButtonElement>()
  let tabEls = $state<Record<Tab, HTMLButtonElement | undefined>>({ active: undefined, archived: undefined })
  // Rows with a change on its way, so a second press on the same row is not sent twice.
  const pending = new Set<string>()

  // The machines that need a look and those still working, from every row the page holds.
  const sync = $derived.by(() => {
    const failed = new Set<string>(), working = new Set<string>()
    let failedSomewhere = false, workingSomewhere = false
    for (const r of page.everyRow) {
      const h = r.health
      if (h.machines.length) {
        for (const m of h.machines) {
          if (m.state === 'failed' || m.state === 'blocked') failed.add(m.host)
          else if (m.state === 'installing' || m.state === 'removing' || m.state === 'offline') working.add(m.host)
        }
      } else if (h.state === 'failed' || h.state === 'blocked' || h.state === 'needs-update') {
        for (const host of h.hosts) failed.add(host)
        failedSomewhere ||= h.hosts.length === 0
      } else if (h.state === 'installing' || h.state === 'removing' || h.state === 'offline') {
        for (const host of h.hosts) working.add(host)
        workingSomewhere ||= h.hosts.length === 0
      }
    }
    for (const host of failed) working.delete(host)
    const m = { failed: Math.max(failed.size, failedSomewhere ? 1 : 0), working: Math.max(working.size, workingSomewhere ? 1 : 0) }
    if (!m.failed && !m.working && page.machines === 0) return { text: 'No machines yet', tone: 'off' }
    return { text: syncSentence(m), tone: m.failed ? 'bad' : m.working ? 'work' : 'ok' }
  })

  const lede = $derived(page.machines === 0
    ? 'Everything Active goes on each machine you connect. Archive a skill to keep it here and off your machines. Delete it to be rid of it.'
    : page.machines === 1
      ? 'Everything Active is on your machine. Archive a skill to take it off and keep it here. Delete it to be rid of it.'
      : `Everything Active is on all ${page.machines} of your machines. Archive a skill to take it off them and keep it here. Delete it to be rid of it.`)

  const newHosts = $derived([...new Set(page.newOnMachines.flatMap(n => n.copies.map(c => c.host)))])
  const newWhere = $derived(newHosts.length === 1 ? newHosts[0] : newHosts.length === 2 ? `${newHosts[0]} and ${newHosts[1]}` : `${newHosts.length} machines`)
  const newTitle = $derived(`${page.newOnMachines.length === 1 ? '1 new skill' : `${page.newOnMachines.length} new skills`} turned up on ${newWhere}`)

  const noticeText = $derived.by(() => {
    const notice = page.notice
    if (!notice) return ''
    const parts: string[] = []
    if (notice.parked) parts.push(notice.parked === 1 ? '1 Parked skill is now Archived and came off your machines.' : `${notice.parked} Parked skills are now Archived and came off your machines.`)
    if (notice.removed) parts.push(notice.removed === 1 ? 'The skill you removed before the upgrade is here too.' : `The ${notice.removed} you removed before the upgrade are here too.`)
    return parts.join(' ')
  })

  const emptyText = $derived(page.filtering
    ? page.q.trim() ? `Nowt matches “${page.q.trim()}”.` : 'Nowt matches these filters.'
    : page.tab === 'archived' ? 'Nowt archived. Archive a skill to take it off your machines and keep it here.'
    : canWrite ? 'Nowt active. Paste a skill link above to get cracking.' : 'Nowt active yet.')

  const read = (status: Tab) => page.read(status)
  const write = <T,>(fn: () => Promise<T>) => page.write(fn)
  const fail = (e: unknown) => page.fail(e)

  function say(text: string, undo?: () => Promise<void>) {
    const id = ++toastSeq
    toast = { id, text, undo: undo && (async () => {
      toast = null
      try { await undo() } catch (e) { fail(e) }
    }) }
  }

  // Without machines the toast says only what changed.
  function pruneText(action: 'archived' | 'deleted' | 'restored', names: string[], onMachines: boolean): string {
    const text = toastText(action, names, page.machines)
    return onMachines && page.machines > 0 ? text : text.split(' · ')[0]
  }

  const rowEl = (id: string) => listEl?.querySelector<HTMLElement>(`[data-skill="${CSS.escape(id)}"]`) ?? null
  // Focus moves without scrolling: the page stays exactly where it was.
  function focusQuietly(el: HTMLElement | null | undefined) { el?.focus({ preventScroll: true }) }
  async function refocus(order: string[], removed: string[], act: Act) {
    await tick()
    const id = focusAfterRemoval(order, removed)
    const row = id ? rowEl(id) : null
    focusQuietly(row?.querySelector<HTMLElement>(`[data-act="${act}"]`) ?? row?.querySelector<HTMLElement>('[data-act]') ?? tabEls[page.tab])
  }
  async function focusRow(id: string, act: Act) {
    await tick()
    const row = rowEl(id)
    focusQuietly(row?.querySelector<HTMLElement>(`[data-act="${act}"]`) ?? row?.querySelector<HTMLElement>('[data-act]'))
  }

  function endSelecting() {
    selecting = false
    selected = []
  }

  function claim(ids: string[]): boolean {
    if (ids.some(id => pending.has(id))) return false
    for (const id of ids) pending.add(id)
    return true
  }
  function release(ids: string[]) { for (const id of ids) pending.delete(id) }

  // Archive (to archived) or Restore (to active), one row or the selection.
  async function changeStatus(targets: SkillRow[], to: Tab, act: 'archive' | 'restore') {
    const ids = targets.map(r => r.id)
    if (!ids.length || !claim(ids)) return
    const names = targets.map(r => r.name), from = page.tab, order = page.shown.map(r => r.id)
    try {
      const data = await write(() => setSkillStatus(ids, to, from))
      page.apply(data, from, ids)
    } catch (e) {
      fail(e)
      return
    } finally {
      release(ids)
    }
    endSelecting()
    if (ids.includes(openId)) openId = ''
    void read(other(from))
    await refocus(order, ids, act)
    say(pruneText(to === 'archived' ? 'archived' : 'restored', names, true), async () => {
      const back = await write(() => setSkillStatus(ids, from, page.tab))
      page.apply(back, page.tab)
      void read(other(page.tab))
      await focusRow(ids[0], act)
    })
  }

  // Delete asks first only when deleteConfirm says so: a skill you wrote, or one reet has no copy of.
  function askDelete(targets: SkillRow[]) {
    if (!targets.length) return
    const order = page.shown.map(r => r.id)
    const dialog = deleteConfirm(targets)
    if (!dialog) {
      void remove(targets, { written: false, noCopy: false }, order)
      return
    }
    ask = { title: dialog.title, body: dialog.body, rows: targets, flags: { written: dialog.writtenConfirmed, noCopy: dialog.noCopyConfirmed }, order }
  }

  async function remove(targets: SkillRow[], flags: Flags, order: string[]) {
    const ids = targets.map(r => r.id)
    if (!claim(ids)) return
    const names = targets.map(r => r.name), from = page.tab
    try {
      await write(() => deleteSkills(ids, { writtenConfirmed: flags.written, noCopyConfirmed: flags.noCopy }))
    } catch (e) {
      // The server knows better than the page: it refused a skill you wrote, or one reet has no copy of. The same
      // dialog asks, and the delete goes again with that flag only once the person agrees. A refusal for a flag already
      // sent is not asked again.
      const code = e instanceof ApiError && e.status === 409 ? e.code : ''
      if ((code === 'written' && !flags.written) || (code === 'no-copy' && !flags.noCopy)) {
        const named = new Set((e as ApiError).names)
        const marked = targets.map(r => !named.has(r.name) ? r : code === 'written' ? { ...r, yours: true } : { ...r, hasCopy: false })
        const dialog = deleteConfirm(marked)
        if (dialog) {
          ask = { title: dialog.title, body: dialog.body, rows: marked, flags: { written: flags.written || dialog.writtenConfirmed, noCopy: flags.noCopy || dialog.noCopyConfirmed }, order }
          return
        }
      }
      fail(e)
      return
    } finally {
      release(ids)
    }
    page.held[from] = (page.held[from] ?? []).filter(r => !ids.includes(r.id))
    page.counts = { ...page.counts, [from]: Math.max(0, page.counts[from] - ids.length) }
    endSelecting()
    if (ids.includes(openId)) openId = ''
    await refocus(order, ids, 'delete')
    const onMachines = targets.some(r => r.status === 'active' || ['removing', 'offline', 'failed', 'blocked'].includes(r.health.state))
    say(pruneText('deleted', names, onMachines), async () => {
      await write(() => undeleteSkills(ids))
      await read(page.tab)
      void read(other(page.tab))
      await focusRow(ids[0], 'delete')
    })
  }

  async function retry(row: SkillRow) {
    try {
      await write(() => retrySkill(row.id))
      say(`Trying ${row.name} again.`)
      void read(page.tab)
    } catch (e) {
      fail(e)
    }
  }

  async function like(row: SkillRow) {
    const liked = !row.liked
    page.setRow(row.id, { liked })
    try {
      await write(() => patchSkill(row.shelfId, { liked }))
    } catch (e) {
      page.setRow(row.id, { liked: !liked })
      fail(e)
    }
  }

  const selectedRows = () => page.shown.filter(r => selected.includes(r.id))
  function pick(row: SkillRow, on: boolean) {
    selected = on ? [...new Set([...selected, row.id])] : selected.filter(id => id !== row.id)
  }
  function toggleSelecting() {
    if (selecting) endSelecting()
    else selecting = true
  }

  async function bulkTag(name: string) {
    const targets = selectedRows()
    const shelfIds = targets.map(r => r.shelfId).filter(Boolean)
    if (!name || !shelfIds.length) return
    try {
      await write(() => tagSkills(shelfIds, [name], []))
      say(`Sorted. Tagged ${shelfIds.length === 1 ? targets.find(r => r.shelfId)!.name : `${shelfIds.length} skills`} with ${name}.`)
      endSelecting()
      await read(page.tab)
      void read(other(page.tab))
    } catch (err) {
      fail(err)
    }
  }

  function friendlyAddError(e: unknown): string {
    const text = e instanceof Error ? e.message : String(e)
    if (text.toLowerCase().includes('expected a skills.sh')) {
      return "That doesn't look like a skill yet. Paste a skills.sh link like skills.sh/anthropics/skills/pdf, or an npx skills add command."
    }
    return text
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    const value = addInput.trim()
    if (!value || adding) return
    adding = true
    addError = ''
    try {
      const added = await write(() => addSkill(value))
      addInput = ''
      say(!added.created ? `${added.name} is already in your library.` : page.machines ? `Added ${added.name} · going on every machine` : `Added ${added.name} to your library.`)
      await read(page.tab)
      void read(other(page.tab))
    } catch (err) {
      if (err instanceof UnauthorizedError) {
        fail(err)
        return
      }
      addError = friendlyAddError(err)
      await tick()
      addEl?.focus()
      addEl?.select()
    } finally {
      adding = false
    }
  }

  async function restoreParked() {
    const parked = (page.held.archived ?? []).filter(r => r.upgradedFrom === 'parked')
    try {
      await write(() => skillNotice('restore-parked'))
    } catch (e) {
      fail(e)
      return
    }
    page.notice = null
    await read(page.tab)
    void read(other(page.tab))
    focusQuietly(tabEls[page.tab])
    if (!parked.length) return
    const ids = parked.map(r => r.id)
    say(pruneText('restored', parked.map(r => r.name), true), async () => {
      const back = await write(() => setSkillStatus(ids, 'archived', page.tab))
      page.apply(back, page.tab)
      void read(other(page.tab))
    })
  }

  async function dismissNotice() {
    try {
      await write(() => skillNotice('dismiss'))
      page.notice = null
      focusQuietly(tabEls[page.tab])
    } catch (e) {
      fail(e)
    }
  }

  function choose(next: Tab) {
    if (next === page.tab) return
    page.tab = next
    selected = []
    openId = ''
    void read(next)
  }
  // The tabs follow the arrow keys, Home and End, as tabs do.
  function tabKeys(e: KeyboardEvent) {
    const order = TABS.map(t => t.id)
    const at = order.indexOf(page.tab)
    const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? order.length - 1 : 0
    const to = step ? order[(at + step) % order.length] : e.key === 'Home' ? order[0] : e.key === 'End' ? order[order.length - 1] : null
    if (!to) return
    e.preventDefault()
    choose(to)
    tabEls[to]?.focus()
  }

  // A filter that hides rows lets go of them.
  $effect(() => {
    void [page.q, page.onlyLiked, page.tagFilter, page.listFilter]
    selected = []
  })

  // A Jump result opens this page with ?skill=<id>: the row is brought into view once, and marked for a moment.
  async function flash(id: string) {
    await tick()
    const el = rowEl(id)
    if (!el) return
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    el.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'center' })
    el.classList.remove('flash')
    void el.offsetWidth
    el.classList.add('flash')
    setTimeout(() => el.classList.remove('flash'), 2000)
  }

  async function start() {
    const params = new URLSearchParams(location.search)
    const jump = params.get('skill') ?? ''
    await read('active')
    page.loaded = true
    const archived = read('archived')
    if (jump) {
      const match = (list: SkillRow[] | null) => list?.find(r => r.id === jump || r.shelfId === jump)
      let found = match(page.held.active)
      if (!found) {
        await archived
        found = match(page.held.archived)
        if (found) page.tab = 'archived'
      }
      if (found) await flash(found.id)
    }
    // Home's Waiting for you links to ?review=new.
    if (params.get('review') === 'new') {
      params.delete('review')
      const rest = params.toString()
      window.history.replaceState(window.history.state, '', `${location.pathname}${rest ? `?${rest}` : ''}${location.hash}`)
      if (canWrite && page.newOnMachines.length) reviewOpen = true
    }
  }

  onMount(() => {
    void start()
    const poll = setInterval(() => {
      if (!page.loaded || page.writing || ask || reviewOpen || document.visibilityState !== 'visible') return
      void read(page.tab)
      void read(other(page.tab))
    }, POLL_MS)
    const changed = () => { void read(page.tab); void read(other(page.tab)) }
    window.addEventListener('reet-shelf-changed', changed)
    return () => {
      clearInterval(poll)
      window.removeEventListener('reet-shelf-changed', changed)
    }
  })
</script>

<div class="skills-page" class:selecting>
  <header class="head">
    <div class="copy">
      <p class="eyebrow">EY UP. YOUR SKILLS.</p>
      <h1>Skills<span aria-hidden="true">.</span></h1>
      <p class="lede">{lede}</p>
    </div>
    {#if page.loaded}<a class="sync-chip" href="/sync" data-tone={sync.tone}><span class="dot" aria-hidden="true"></span>{sync.text}</a>{/if}
  </header>

  {#if canWrite}
    <form class="add" onsubmit={add} aria-busy={adding}>
      <label class="vh" for="add-skill">Add a skill</label>
      <input id="add-skill" bind:this={addEl} bind:value={addInput} type="text" autocomplete="off" placeholder="Paste a skills.sh link or an npx skills add command"
        aria-invalid={addError ? 'true' : undefined} aria-describedby={addError ? 'add-skill-error' : undefined} />
      <button type="submit" class="btn primary" disabled={adding || !addInput.trim()} aria-busy={adding}>{adding ? 'Adding…' : 'Add'}</button>
      <a class="btn" href="/hosting">Write a skill</a>
    </form>
    {#if addError}<p class="add-error" id="add-skill-error" role="alert">{addError}</p>{/if}
  {/if}

  {#if page.error}<p class="problem" role="alert">{page.error}</p>{/if}

  {#if canWrite && page.newOnMachines.length}
    <section class="new" aria-label="New on a machine">
      <svg class="mark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="M12 7.5v5.5M12 16.5v.01"></path></svg>
      <div class="new-copy">
        <strong>{newTitle}</strong>
        <span>{page.newOnMachines.map(n => n.name).join(', ')}. {newHosts.length === 1 ? `They stay on ${newHosts[0]} only until you decide.` : 'They stay where they are until you decide.'}</span>
      </div>
      <button type="button" class="btn primary" bind:this={reviewEl} onclick={() => (reviewOpen = true)}>Review</button>
    </section>
  {/if}

  <div class="toolbar">
    <div class="tabs" role="tablist" aria-label="Skill state">
      {#each TABS as t (t.id)}
        <button type="button" role="tab" id={`skills-tab-${t.id}`} class="tab" aria-selected={page.tab === t.id} aria-controls="skills-panel" tabindex={page.tab === t.id ? 0 : -1}
          bind:this={tabEls[t.id]} onclick={() => choose(t.id)} onkeydown={tabKeys}>{t.label}{#if page.loaded}<small>{page.counts[t.id]}</small>{/if}</button>
      {/each}
    </div>
    <div class="find" role="group" aria-label="Find skills">
      <div class="search">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7"></circle><path d="M20 20l-3.5-3.5"></path></svg>
        <input type="search" aria-label="Search skills" placeholder="Search" bind:value={page.q} />
      </div>
      <button type="button" class="toggle liked" aria-pressed={page.onlyLiked} onclick={() => (page.onlyLiked = !page.onlyLiked)}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true"><path d="M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z"></path></svg>Liked
      </button>
      {#if page.tagNames.length || page.tagFilter}
        <select aria-label="Tag" bind:value={page.tagFilter}>
          <option value="">All tags</option>
          {#each page.tagNames as name (name)}<option value={name}>{name}</option>{/each}
        </select>
      {/if}
      {#if page.lists.length || page.listFilter}
        <select aria-label="List" bind:value={page.listFilter}>
          <option value="">All lists</option>
          {#each page.lists as list (list.id)}<option value={String(list.id)}>{list.name}</option>{/each}
        </select>
      {/if}
      <select aria-label="Sort by" bind:value={page.sort}>
        <option value="name">Name</option>
        <option value="attention">Needs a look first</option>
        <option value="liked">Liked first</option>
      </select>
      {#if canWrite}<button type="button" class="toggle" aria-pressed={selecting} onclick={toggleSelecting}>Select</button>{/if}
    </div>
  </div>

  {#if page.tab === 'archived' && page.notice && noticeText && canWrite}
    <section class="notice" aria-label="Upgrade">
      <p>{noticeText}</p>
      {#if page.notice.parked}<button type="button" class="btn" onclick={restoreParked}>Restore the Parked ones</button>{/if}
      <button type="button" class="btn quiet" onclick={dismissNotice}>Got it</button>
    </section>
  {/if}

  <div class="panel" id="skills-panel" role="tabpanel" aria-labelledby={`skills-tab-${page.tab}`}>
    {#if page.held[page.tab] === null}
      {#if !page.error}<Skeleton label="Loading your skills" shape="rows" count={4} />{/if}
    {:else if page.shown.length}
      <ul class="list" bind:this={listEl} aria-label={page.tab === 'active' ? 'Active skills' : 'Archived skills'}>
        {#each page.shown as row (row.id)}
          <SkillRowView {row} {selecting} selected={selected.includes(row.id)} open={openId === row.id} machines={page.machines} {canWrite}
            onarchive={r => changeStatus([r], 'archived', 'archive')}
            onrestore={r => changeStatus([r], 'active', 'restore')}
            ondelete={r => askDelete([r])}
            onselect={pick}
            onopen={r => (openId = openId === r.id ? '' : r.id)}
            onretry={retry}
            onlike={like} />
        {/each}
      </ul>
    {:else}
      <div class="empty">
        <p>{emptyText}</p>
        {#if page.filtering}<button type="button" class="btn" onclick={() => page.clearFilters()}>Clear filters</button>{/if}
      </div>
    {/if}
  </div>
</div>

{#if selecting && canWrite}
  {#key page.tab}
    <SkillsBulkBar count={selected.length} tab={page.tab} tagNames={page.tagNames}
      onarchive={() => changeStatus(selectedRows(), 'archived', 'archive')}
      onrestore={() => changeStatus(selectedRows(), 'active', 'restore')}
      ondelete={() => askDelete(selectedRows())}
      ontag={bulkTag}
      oncancel={endSelecting} />
  {/key}
{/if}

<UndoToast {toast} raised={selecting} onclose={() => (toast = null)} />

{#if ask}
  <ConfirmDialog title={ask.title} body={ask.body} confirmLabel="Delete" danger
    onconfirm={() => { const a = ask!; ask = null; void remove(a.rows, a.flags, a.order) }}
    oncancel={() => (ask = null)} />
{/if}

{#if reviewOpen}
  <NewOnMachineDialog items={page.newOnMachines} onerror={fail}
    ondecided={() => { void read(page.tab); void read(other(page.tab)) }}
    onclose={() => { reviewOpen = false; void tick().then(() => focusQuietly(reviewEl ?? tabEls[page.tab])) }} />
{/if}

<style>
  .skills-page { display: flex; flex-direction: column; gap: 24px; }
  .skills-page.selecting { padding-bottom: 88px; }
  .head { display: flex; justify-content: space-between; align-items: flex-end; gap: 16px 24px; flex-wrap: wrap; }
  .copy { display: flex; flex-direction: column; gap: 10px; max-width: 640px; min-width: 0; }
  .eyebrow { margin: 0; color: var(--accent-text); font: 500 0.875rem/1.4 var(--mono); letter-spacing: 0.14em; }
  h1 { margin: 0; font: 600 clamp(2.5rem, 5vw, 3.5rem)/1 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 0; color: var(--ink-soft); font-size: 1.0625rem; line-height: 1.5; }
  .sync-chip {
    display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 0 16px; border-radius: 999px; text-decoration: none;
    font-weight: 600; font-size: 0.9375rem; border: 1px solid var(--line); background: var(--card); color: var(--muted);
  }
  .sync-chip[data-tone='ok'] { color: var(--good); border-color: color-mix(in srgb, var(--good) 35%, var(--line)); background: color-mix(in srgb, var(--good) 8%, var(--card)); }
  .sync-chip[data-tone='work'] { color: var(--info); border-color: color-mix(in srgb, var(--info) 35%, var(--line)); background: color-mix(in srgb, var(--info) 8%, var(--card)); }
  .sync-chip[data-tone='bad'] { color: var(--bad); border-color: color-mix(in srgb, var(--bad) 35%, var(--line)); background: color-mix(in srgb, var(--bad) 8%, var(--card)); }
  .sync-chip:hover { text-decoration: underline; text-underline-offset: 3px; }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: currentColor; flex: none; }

  .add { display: flex; gap: 10px; flex-wrap: wrap; }
  .add input { flex: 1 1 360px; min-height: 48px; border-radius: 10px; padding: 0 14px; font-size: 1rem; }
  .add .btn { min-height: 48px; border-radius: 10px; padding: 0 20px; }
  .add-error { margin: -14px 0 0; color: var(--bad); font-size: 0.875rem; }
  .problem { margin: 0; padding: 10px 14px; border: 1px solid var(--bad); border-radius: 10px; background: var(--card); color: var(--ink); }

  .new { display: flex; align-items: center; gap: 16px; padding: 16px 16px 16px 20px; border-radius: 12px; background: var(--tint); border: 1px solid color-mix(in srgb, var(--accent) 25%, var(--tint)); flex-wrap: wrap; }
  .new .mark { width: 24px; height: 24px; color: var(--accent-text); flex: none; }
  .new-copy { flex: 1 1 320px; min-width: 0; display: flex; flex-direction: column; gap: 2px; overflow-wrap: anywhere; }
  .new-copy strong { font-size: 1rem; }
  .new-copy span { font-size: 0.9375rem; color: var(--ink-soft); overflow-wrap: anywhere; }
  .new .btn { min-height: 44px; border-radius: 10px; }

  .toolbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; padding-top: 4px; }
  .tabs { display: flex; gap: 4px; padding: 4px; border-radius: 12px; background: var(--bg-sunk); }
  .tab { min-height: 44px; padding: 0 16px; border: 0; border-radius: 10px; background: transparent; color: var(--muted); font-size: 0.9375rem; font-weight: 600; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; gap: 8px; }
  .tab small { font: 400 0.875rem var(--mono); font-variant-numeric: tabular-nums; }
  .tab:hover { color: var(--ink); }
  .tab[aria-selected='true'] { background: var(--tint); color: var(--accent-text); }
  .find { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
  .search { position: relative; display: flex; }
  .search svg { position: absolute; left: 12px; top: 50%; width: 18px; height: 18px; transform: translateY(-50%); color: var(--muted); pointer-events: none; }
  .search input { min-height: 44px; width: 220px; border-radius: 10px; padding-left: 38px; }
  .find select { min-height: 44px; border-radius: 10px; }
  .toggle { min-height: 44px; padding: 0 12px; border: 1px solid var(--line-strong); border-radius: 10px; background: var(--card); color: var(--muted); cursor: pointer; display: inline-flex; align-items: center; justify-content: center; gap: 6px; font-size: 0.9375rem; }
  .toggle svg { width: 18px; height: 18px; }
  .toggle:hover { color: var(--ink); }
  .toggle[aria-pressed='true'] { background: var(--tint); color: var(--ink); border-color: var(--accent); font-weight: 600; }
  .toggle.liked[aria-pressed='true'] { background: var(--tint); color: var(--ink); border-color: var(--heart); }
  .toggle.liked[aria-pressed='true'] svg { color: var(--heart); fill: currentColor; }

  .notice { display: flex; align-items: center; gap: 12px 16px; padding: 14px 16px 14px 20px; border-radius: 12px; background: var(--card); border: 1px solid var(--line); flex-wrap: wrap; }
  .notice p { margin: 0; flex: 1 1 320px; font-size: 0.9375rem; line-height: 1.5; }
  .notice .btn { min-height: 44px; border-radius: 10px; }

  .panel { border: 1px solid var(--line); border-radius: 14px; background: var(--card); overflow: hidden; }
  .panel :global(.skeleton) { padding: 16px 20px; }
  .list { list-style: none; margin: 0; padding: 0; }
  .list :global([data-skill].flash) { background: var(--tint); }
  @media (prefers-reduced-motion: no-preference) {
    .list :global([data-skill].flash) { animation: flash 2s var(--ease-out); }
    @keyframes flash { 0%, 60% { background: var(--tint); } 100% { background: transparent; } }
  }
  .empty { padding: 40px 20px; text-align: center; color: var(--muted); display: grid; justify-items: center; gap: 12px; }
  .empty p { margin: 0; font-size: 1rem; }

  @media (max-width: 760px) {
    .toolbar { align-items: stretch; }
    .tabs { width: 100%; }
    .tab { flex: 1; }
    .find { width: 100%; }
    .search { flex: 1 1 100%; }
    .search input { width: 100%; }
    .find select, .find .toggle { flex: 1 1 calc(50% - 4px); min-width: 0; }
  }
  @media (max-width: 560px) {
    .skills-page { gap: 16px; }
    .copy { gap: 6px; }
    .lede { display: none; }
    .add input { flex: 1 1 100%; }
    .add .btn { flex: 1; }
    .add-error { margin-top: -8px; }
    .new { padding: 12px 12px 12px 14px; gap: 12px; }
    .new .mark { display: none; }
    .new .btn { width: 100%; }
  }
</style>
