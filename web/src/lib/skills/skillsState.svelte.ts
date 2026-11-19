import { loadSkillsPage, UnauthorizedError, type NewSkill, type SkillRow, type SkillsPageData, type SkillWork } from '../api'
import { healthLine, matchesSearch } from './skillsPage'

export type Tab = 'active' | 'archived'
export type Sort = 'name' | 'attention' | 'liked'
export const TABS: { id: Tab; label: string }[] = [{ id: 'active', label: 'Active' }, { id: 'archived', label: 'Archived' }]
export const other = (t: Tab): Tab => (t === 'active' ? 'archived' : 'active')

const byName = (a: SkillRow, b: SkillRow) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }) || a.id.localeCompare(b.id)
const rank = (r: SkillRow) => ({ bad: 0, work: 1, ok: 2, off: 3 })[healthLine(r.health).tone]

/**
 * What the Skills page holds and how it reads and writes it: each tab's rows as last read, the counts, the skills new on
 * a machine, the upgrade notice, the filters and the rows they show. The page's flows (focus, the toast, dialogs) stay
 * in SkillsPage.svelte.
 *
 * A write moves `generation` on as it starts and as it ends, so a read that was on its way while the write ran never
 * puts back what the write changed. Writes answer in any order: the page shows a page answer only when no newer page
 * answer has been shown, and a tab whose answer may miss another write's change is read again once every write has
 * answered (writePage, markStale, catchUp). A change shown ahead of its answer (a Like) is kept over every answer that
 * lands while any write is on its way, so an older answer never flicks the row back (writeRow); so is a row a write took
 * off a tab, until an Undo brings it back (takeOff, release).
 */
export class SkillsPageState {
  tab = $state<Tab>('active')
  // Each tab's rows as last read; null until read. The other tab is read in the background, so switching shows it at once.
  held = $state<Record<Tab, SkillRow[] | null>>({ active: null, archived: null })
  counts = $state({ active: 0, archived: 0 })
  newOnMachines = $state<NewSkill[]>([])
  machines = $state(0)
  notice = $state<SkillsPageData['notice']>(null)
  loaded = $state(false)
  /** What the machines are doing over every skill, as the server counts it; every answer with page data carries it. */
  work = $state<SkillWork>({ failed: 0, installing: 0, removing: 0, updating: 0 })
  /** An answer has brought the counts and `work`, which cover both tabs, so they can be shown. */
  readOk = $state(false)
  error = $state('')

  q = $state('')
  onlyLiked = $state(false)
  tagFilter = $state('')
  listFilter = $state('')
  sort = $state<Sort>('name')

  writing = 0
  /** The page has gone: an answer that lands after that starts no read. */
  destroyed = false
  #generation = 0
  // Writes are numbered as they start. `#finishedPage` is the newest one with page data (Archive, Restore and their
  // Undo) whose answer has been shown.
  #started = 0
  #finishedPage = 0
  // Tabs whose rows may miss a write's change: read again once no write is on its way, and until a read of them lands.
  #stale = new Set<Tab>()
  #catchingUp: Promise<void> | null = null
  // Asked for again while one ran: a write may have come and gone meanwhile, and the read it overtook was dropped.
  #catchUpAgain = false
  // Each row's latest change shown ahead of its answer (writeRow), put back over any answer that lands while a write is
  // on its way. Let go once none is: by then a tab whose answer may miss one is marked stale and read again.
  #ahead = new Map<string, Partial<SkillRow>>()
  // Rows a write has taken off a tab (a Delete, an Archive, a Restore), kept off it over any answer that lands while a
  // write is on its way: an answer to an older write cannot know they went. Let go as #ahead is, or by an Undo.
  #gone = new Map<string, Tab>()
  // Waiting for no write to be on its way (settled).
  #idle: (() => void)[] = []

  rows = $derived(this.held[this.tab] ?? [])
  everyRow = $derived([...(this.held.active ?? []), ...(this.held.archived ?? [])])
  tagNames = $derived([...new Set(this.everyRow.flatMap(r => r.tags.map(t => t.name)))].sort((a, b) => a.localeCompare(b)))
  lists = $derived([...new Map(this.everyRow.flatMap(r => r.lists).map(l => [l.id, l])).values()].sort((a, b) => a.name.localeCompare(b.name)))
  filtering = $derived(this.q.trim() !== '' || this.onlyLiked || this.tagFilter !== '' || this.listFilter !== '')
  shown = $derived.by(() => {
    // Search as the export searches, so Markdown and CSV hold the rows shown.
    const found = this.rows.filter(r => (!this.onlyLiked || r.liked)
      && (!this.tagFilter || r.tags.some(t => t.name === this.tagFilter))
      && (!this.listFilter || r.lists.some(l => String(l.id) === this.listFilter))
      && matchesSearch(r, this.q))
    const order = this.sort === 'attention' ? (a: SkillRow, b: SkillRow) => rank(a) - rank(b) || byName(a, b)
      : this.sort === 'liked' ? (a: SkillRow, b: SkillRow) => Number(b.liked) - Number(a.liked) || byName(a, b)
      : byName
    return [...found].sort(order)
  })

  apply(data: SkillsPageData, status: Tab, without: string[] = []) {
    const off = new Set(without)
    for (const [id, tab] of this.#gone) if (tab === status) off.add(id)
    const rows = off.size ? data.skills.filter(r => !off.has(r.id)) : data.skills
    this.held[status] = this.#ahead.size ? rows.map(r => { const change = this.#ahead.get(r.id); return change ? { ...r, ...change } : r }) : rows
    this.counts = data.counts
    this.newOnMachines = data.newOnMachines ?? []
    this.machines = data.machines
    this.notice = data.notice
    this.work = data.work ?? { failed: 0, installing: 0, removing: 0, updating: 0 }
    this.readOk = true
  }

  /**
   * Reads one tab. A read the person waits for (the page opening, a tab chosen, an Undo) says when it fails; a `quiet`
   * one in the background (the poll, the other tab) does not, unless the sign-in has ended. A read of the tab in view
   * that works clears whatever the page last said had gone wrong.
   */
  async read(status: Tab, quiet = false): Promise<boolean> {
    if (this.destroyed) return false
    const at = this.#generation
    try {
      const data = await loadSkillsPage(status)
      if (at !== this.#generation) return false
      this.apply(data, status)
      this.#stale.delete(status)
      if (status === this.tab) this.error = ''
      return true
    } catch (e) {
      if (at === this.#generation && (!quiet || e instanceof UnauthorizedError)) this.fail(e)
      return false
    }
  }

  /** `done` hears the answer and the write's number. Once no write is on its way, stale tabs are read again. */
  async write<T>(fn: () => Promise<T>, done?: (out: T, seq: number) => void): Promise<T> {
    const seq = ++this.#started
    this.#generation++
    this.writing++
    try {
      const out = await fn()
      this.error = ''
      done?.(out, seq)
      // The rest of the app (the Jump list) reads the library again.
      if (typeof window !== 'undefined') window.dispatchEvent(new Event('reet-shelf-changed'))
      return out
    } finally {
      this.writing--
      this.#generation++
      if (this.writing === 0) {
        this.#ahead.clear()
        this.#gone.clear()
        if (this.#stale.size) void this.catchUp()
        for (const wake of this.#idle.splice(0)) wake()
      }
    }
  }

  /**
   * Takes rows off a tab for a write that has answered (a Delete, an Archive, a Restore): they leave the list now, and
   * stay off that tab over any answer that lands while a write is on its way. Call it from the write's `done`.
   */
  takeOff(status: Tab, ids: string[]) {
    if (!ids.length) return
    for (const id of ids) this.#gone.set(id, status)
    this.held[status] = (this.held[status] ?? []).filter(r => !ids.includes(r.id))
  }

  /** An Undo brings rows back: answers that land from now on may show them again. */
  release(ids: string[]) { for (const id of ids) this.#gone.delete(id) }

  /**
   * Resolves once no write is on its way and the tabs that may miss a change have been read again (or tried), so the
   * rows on the page are the server's: what an Undo waits on before it puts focus on the row it brought back.
   */
  async settled(): Promise<void> {
    while (!this.destroyed) {
      if (!this.writing) {
        await this.catchUp()
        if (!this.writing) return
      }
      await new Promise<void>(resolve => this.#idle.push(resolve))
    }
  }

  /**
   * A change to a row's own fields that shows at once, ahead of its answer (a Like). While any write is on its way it
   * is put back over every answer that lands, so an answer to an older write, which cannot know it, never flicks the
   * row back. If it fails, the row goes back to `before`, unless a later change to the row has taken its place.
   */
  async writeRow(id: string, change: Partial<SkillRow>, before: Partial<SkillRow>, fn: () => Promise<unknown>): Promise<void> {
    const mine = { ...change }
    this.#ahead.set(id, mine)
    this.setRow(id, mine)
    await this.write(async () => {
      try {
        return await fn()
      } catch (e) {
        if (this.#ahead.get(id) === mine) {
          this.#ahead.delete(id)
          this.setRow(id, before)
        }
        throw e
      }
    })
  }

  /** The tab's rows may miss a change: read it again once no write is on its way. */
  markStale(status: Tab) { this.#stale.add(status) }

  /**
   * Reads each stale tab again, once no write is on its way. A read that a new write overtakes is dropped and its tab
   * stays stale. One catch-up runs at a time: asked for while one runs (a write that started and ended during its
   * read), it goes round again once that pass ends, if no write is on its way and a tab is still stale; a write still
   * on its way then asks again when it ends. Callers may wait on it, the next pass included.
   */
  catchUp(): Promise<void> {
    if (this.writing || !this.#stale.size) return Promise.resolve()
    if (this.#catchingUp) {
      this.#catchUpAgain = true
      return this.#catchingUp
    }
    this.#catchingUp = (async () => {
      try {
        do {
          this.#catchUpAgain = false
          for (const tab of [...this.#stale]) await this.read(tab, true)
        } while (this.#catchUpAgain && !this.writing && this.#stale.size)
      } finally {
        this.#catchingUp = null
        this.#catchUpAgain = false
      }
    })()
    return this.#catchingUp
  }

  /**
   * A write that answers with a tab of the page (Archive, Restore and their Undo). Its answer is shown unless a newer
   * page answer has been shown already; either way its own rows, `without`, are taken off the tab (takeOff). Only page answers count as
   * newer: a Like or a Delete that answers first must not hold back an Undo. Any write that started after this one,
   * answered or not, may be missing from this answer (a Like on its way is undone by it), so the tab is marked stale
   * and read again once every write has answered.
   */
  async writePage(fn: () => Promise<SkillsPageData>, status: Tab, without: string[] = []): Promise<void> {
    await this.write(fn, (data, seq) => {
      this.takeOff(status, without)
      if (seq > this.#finishedPage) {
        this.#finishedPage = seq
        this.apply(data, status)
      }
      if (this.#started > seq) this.markStale(status)
    })
  }

  fail(e: unknown) {
    // The page cannot sign anyone in: the app around it shows the sign-in screen.
    if (e instanceof UnauthorizedError) {
      window.dispatchEvent(new CustomEvent('reet-signed-out'))
      return
    }
    this.error = e instanceof Error ? e.message : String(e)
  }

  setRow(id: string, patch: Partial<SkillRow>) {
    for (const t of ['active', 'archived'] as Tab[]) this.held[t] = this.held[t]?.map(r => (r.id === id ? { ...r, ...patch } : r)) ?? null
  }

  clearFilters() {
    this.q = ''
    this.onlyLiked = false
    this.tagFilter = ''
    this.listFilter = ''
  }
}
