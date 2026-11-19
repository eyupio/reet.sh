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
 * answer has been shown, and when another write answered first, the tab is read again (writePage).
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
  #generation = 0
  // Writes are numbered as they start. `#finishedAny` is the newest write of any kind whose answer has come back;
  // `#finishedPage` the newest of those that answer with page data (Archive, Restore and their Undo).
  #started = 0
  #finishedAny = 0
  #finishedPage = 0

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
    this.held[status] = without.length ? data.skills.filter(r => !without.includes(r.id)) : data.skills
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
    const at = this.#generation
    try {
      const data = await loadSkillsPage(status)
      if (at !== this.#generation) return false
      this.apply(data, status)
      if (status === this.tab) this.error = ''
      return true
    } catch (e) {
      if (at === this.#generation && (!quiet || e instanceof UnauthorizedError)) this.fail(e)
      return false
    }
  }

  /** `done` hears the answer and the write's number. */
  async write<T>(fn: () => Promise<T>, done?: (out: T, seq: number) => void): Promise<T> {
    const seq = ++this.#started
    this.#generation++
    this.writing++
    try {
      const out = await fn()
      this.error = ''
      done?.(out, seq)
      return out
    } finally {
      this.#finishedAny = Math.max(this.#finishedAny, seq)
      this.writing--
      this.#generation++
    }
  }

  /**
   * A write that answers with a tab of the page (Archive, Restore and their Undo). Its answer is shown unless a newer
   * page answer has been shown already, when it only takes its own rows out of the list. Only page answers count as
   * newer: a Like or a Delete that answers first must not hold back an Undo. When any write that started after this one
   * answered first, this answer may not show that write's change, so the tab is read again.
   */
  async writePage(fn: () => Promise<SkillsPageData>, status: Tab, without: string[] = []): Promise<void> {
    let stale = false
    await this.write(fn, (data, seq) => {
      if (seq > this.#finishedPage) {
        this.#finishedPage = seq
        this.apply(data, status, without)
      } else {
        this.held[status] = (this.held[status] ?? []).filter(r => !without.includes(r.id))
      }
      stale = this.#finishedAny > seq
    })
    if (stale) void this.read(status, true)
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
