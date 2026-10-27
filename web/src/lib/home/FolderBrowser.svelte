<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { breadcrumbs, childPath, countLabel, repoMessage, topFolder } from '../folderBrowser'
  import { browseFolder, TerminalError, type BrowseEntry, type BrowseQuery, type BrowseResult } from '../terminal/api'

  // One machine's folders, listed live by the machine (browse-v1): folders only, never file names. Clicking a folder enters
  // it; the breadcrumb goes back, never above the home. A request that fails (the machine went offline, a refusal, a slow
  // down) says so with Try again and leaves the breadcrumb and list as they were. Nothing is kept once the dialog closes.
  let { agentId, host, session, start, chosen, serial, onuse }: {
    agentId: string
    /** The machine's name, for the list's label. */
    host: string
    /** The opened dialog's browse id: its pages share one audit row. */
    session: string
    /** Where to open: a folder path, or '' for the home. */
    start: string
    /** The folder the form holds now, marked in the list when it is shown. */
    chosen: string
    /** Runs one browse at a time for the dialog (the machine answers a second one at once with "already listing"). */
    serial: <T>(job: () => Promise<T>) => Promise<T>
    /** Use this folder, or the repository's top folder. */
    onuse: (picked: Pick<BrowseResult, 'path' | 'home' | 'repo' | 'repoTop'>) => void
  } = $props()

  const id = $props.id()
  const life = new AbortController()
  let result = $state<BrowseResult | null>(null)
  let entries = $state<BrowseEntry[]>([])
  let busy = $state(false)
  let failure = $state<{ text: string; retry: () => void } | null>(null)
  let filter = $state(''), hidden = $state(false)
  let list = $state<HTMLUListElement>()
  let seq = 0, timer: ReturnType<typeof setTimeout> | undefined

  const crumbs = $derived(result ? breadcrumbs(result.path, result.home) : [])
  const note = $derived(result ? repoMessage(result) : null)
  const top = $derived(result ? topFolder(result) : null)

  const sentence = (e: unknown): string => (e instanceof TerminalError ? e.message : "That didn't reach reet. Check your connection, then try again.")

  // The latest request wins: an answer to an earlier one is dropped. A page (after) adds to the list; anything else replaces it.
  // `focus` is set when the person moved (entered a folder, a crumb, Show more), never on the first list or a filter.
  async function load(path: string, after = '', focus = false) {
    const mine = ++seq
    const query: BrowseQuery = { path: path || undefined, after: after || undefined, filter: filter || undefined, hidden, session }
    busy = true
    failure = null
    try {
      const r = await serial(() => browseFolder(agentId, query, life.signal))
      if (mine !== seq || life.signal.aborted) return
      const dirs = r.entries.filter(e => e.dir)
      const before = entries.length
      entries = after ? [...entries, ...dirs] : dirs
      result = r
      busy = false
      if (!focus) return
      // The rows are enabled again once the list is drawn.
      await tick()
      // After Show more, the first new folder; after entering one, the first in it.
      list?.querySelectorAll<HTMLButtonElement>('button.row')[after ? before : 0]?.focus({ preventScroll: !after })
    } catch (e) {
      if (mine !== seq || life.signal.aborted) return
      failure = { text: sentence(e), retry: () => void load(path, after, focus) }
    } finally {
      if (mine === seq) busy = false
    }
  }
  function enter(path: string) {
    clearTimeout(timer)
    filter = ''
    void load(path, '', true)
  }
  // The machine filters; one request once typing pauses.
  function filtered() {
    clearTimeout(timer)
    timer = setTimeout(() => void load(result?.path ?? start), 300)
  }
  const here = () => result?.path ?? start

  onMount(() => {
    void load(start)
    return () => { clearTimeout(timer); life.abort() }
  })
</script>

<section class="browser" aria-label={`Folders on ${host}`}>
  <div class="crumb-row">
    <nav aria-label="Folder path">
      {#each crumbs as c, i (c.path)}
        {#if i > 0}<span class="sep" aria-hidden="true">/</span>{/if}
        {#if i === crumbs.length - 1}<span class="crumb current" class:home={i === 0} aria-current="location">{c.label}</span>
        {:else}<button type="button" class="crumb" class:home={i === 0} onclick={() => enter(c.path)} disabled={busy}>{c.label}</button>{/if}
      {/each}
    </nav>
    <span class="count">{result ? countLabel(entries.length, result.more) : busy ? 'Listing…' : ''}</span>
  </div>
  <div class="tools">
    <input class="filter" type="text" enterkeyhint="search" aria-label="Filter folders" placeholder="Type to filter…" autocomplete="off" spellcheck="false" maxlength="64" bind:value={filter} oninput={filtered} />
    <label class="hidden"><input type="checkbox" bind:checked={hidden} onchange={() => { clearTimeout(timer); void load(here()) }} /><span class="box" aria-hidden="true"></span>Show hidden folders</label>
  </div>
  <div class="scroll">
    {#if result && entries.length === 0}<p class="empty">{filter ? 'No folders match.' : 'No folders here.'}</p>{/if}
    <ul bind:this={list} aria-label={`Folders in ${result ? crumbs.at(-1)?.label ?? '~' : '~'}`} aria-busy={busy}>
      {#each entries as e (e.name)}
        {@const path = childPath(result?.path ?? '', e.name)}
        <li>
          <button type="button" class="row" class:repo={e.repoRoot} class:selected={path === chosen} aria-current={path === chosen ? 'true' : undefined}
            aria-label={e.repoRoot ? `${e.name}, git repository` : e.name} onclick={() => enter(path)} disabled={busy}>
            <svg class="icon" viewBox="0 0 24 24" aria-hidden="true">
              {#if e.repoRoot}<path d="M12 2l10 10-10 10L2 12z" />{:else}<path d="M3 6a2 2 0 012-2h4l2 2h8a2 2 0 012 2v10a2 2 0 01-2 2H5a2 2 0 01-2-2z" />{/if}
            </svg>
            <span class="name">{e.name}</span>
            {#if e.repoRoot}<span class="badge" aria-hidden="true">REPO</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
    {#if result?.more}<button type="button" class="more" onclick={() => void load(here(), entries.at(-1)?.name ?? '', true)} disabled={busy}>Show more</button>{/if}
  </div>
  <div class="foot" aria-live="polite">
    {#if failure}
      <div class="browse-error"><p>{failure.text}</p><button type="button" class="ghost" onclick={failure.retry}>Try again</button>{#if !result}<button type="button" class="ghost" onclick={() => enter('')}>Open the home folder</button>{/if}</div>
    {/if}
    {#if result}
      <div class="here-row">
        <p class="here">{result.path}{#if result.repo === 'root'}{' '}<span class="repo-word">is a git repository</span>{/if}</p>
        <button type="button" class="use" onclick={() => result && onuse(result)} disabled={busy}>Use this folder</button>
      </div>
      {#if note}
        <div class="note-row"><p class="repo-note">{note}</p>{#if top}<button type="button" class="ghost" onclick={() => result && top && onuse({ path: top, home: result.home, repo: 'root', repoTop: '' })}>Use the top folder</button>{/if}</div>
      {/if}
      {#if result.partial}<p class="partial">Some folders weren't checked for a repository in time, so they show as plain folders.</p>{/if}
    {/if}
  </div>
</section>

<style>
  .browser { display: flex; flex-direction: column; min-height: 0; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .crumb-row { display: flex; align-items: center; gap: 6px; padding: 10px 14px; font-size: 13px; color: var(--ink-soft); border-bottom: 1px solid var(--line-soft); }
  nav { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; min-width: 0; }
  .crumb { display: inline-flex; align-items: center; padding: 0; border: 0; background: none; color: inherit; font: inherit; cursor: pointer; overflow-wrap: anywhere; text-align: left; }
  .crumb:hover:not(:disabled) { color: var(--ink); text-decoration: underline; text-underline-offset: 3px; }
  .crumb.home { color: var(--muted); }
  .current { overflow-wrap: anywhere; }
  .current.home { color: var(--muted); }
  .sep { color: var(--line-deep); }
  .count { margin-left: auto; flex: none; font-size: 12px; color: var(--muted); }
  .tools { display: flex; flex-direction: column; gap: 10px; padding: 10px 14px; border-bottom: 1px solid var(--line-soft); }
  .tools input.filter { width: 100%; height: 36px; min-height: 36px; padding: 0 12px; border: 1px solid var(--line); border-radius: 8px; background: transparent; font: 13px var(--mono); color: var(--ink); }
  .filter::placeholder { color: var(--faint); }
  .hidden { display: flex; align-items: center; gap: 8px; color: var(--muted); font-size: 13px; cursor: pointer; width: fit-content; }
  /* The box is drawn, the real checkbox lies over it (so it is the control that is clicked and focused). */
  .hidden { position: relative; }
  .hidden input { position: absolute; left: 0; top: 0; bottom: 0; z-index: 1; width: 16px; height: 16px; margin: auto 0; opacity: 0; cursor: pointer; }
  .box { width: 16px; height: 16px; flex: none; border: 1px solid var(--line-deep); border-radius: 3px; display: grid; place-items: center; }
  .hidden input:checked + .box { background: var(--accent); border-color: var(--accent); }
  .hidden input:checked + .box::after { content: ''; width: 4px; height: 8px; margin-top: -2px; border: solid var(--accent-ink); border-width: 0 2px 2px 0; transform: rotate(45deg); }
  .hidden input:focus-visible + .box { outline: 2px solid var(--accent); outline-offset: 2px; }
  .scroll { height: 300px; overflow-y: auto; overscroll-behavior: contain; }
  ul { list-style: none; margin: 0; padding: 0; }
  .row { display: flex; align-items: center; gap: 12px; width: 100%; height: 44px; padding: 0 14px; border: 0; background: transparent; color: var(--ink); text-align: left; cursor: pointer; font: inherit; }
  .row:hover:not(:disabled) { background: var(--card); }
  .row.selected { background: var(--line-soft); }
  .row:disabled { cursor: progress; }
  .row:focus-visible { outline-offset: -2px; }
  .icon { width: 16px; height: 16px; flex: none; fill: var(--muted); }
  .row.repo .icon { fill: var(--repo); }
  .name { flex-grow: 1; min-width: 0; font: 400 13px var(--mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .row.repo .name { font-weight: 600; }
  .badge { flex: none; font: 600 10px var(--sans); letter-spacing: .08em; color: var(--repo); border: 1px solid var(--repo-line); border-radius: 4px; padding: 2px 6px; }
  .empty { margin: 0; padding: 12px 14px; color: var(--muted); font-size: 13px; }
  .more { margin: 6px 14px 10px; height: 36px; padding: 0 14px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--ink); font-weight: 500; cursor: pointer; }
  .foot { border-top: 1px solid var(--line-soft); }
  .foot:empty { display: none; }
  .here-row, .note-row, .browse-error { display: flex; align-items: center; gap: 10px; padding: 10px 14px; }
  .note-row, .browse-error { padding-top: 0; }
  .browse-error { padding-top: 10px; }
  .here { flex-grow: 1; min-width: 0; margin: 0; font: 13px var(--mono); color: var(--ink-soft); overflow-wrap: anywhere; }
  .repo-word { color: var(--repo); }
  .repo-note, .browse-error p, .partial { flex-grow: 1; min-width: 0; margin: 0; font-size: 13px; color: var(--muted); line-height: 1.45; }
  .browse-error p { color: var(--bad); }
  .partial { padding: 0 14px 10px; }
  .use, .ghost { flex: none; height: 36px; padding: 0 14px; border-radius: 8px; font-weight: 500; cursor: pointer; background: transparent; }
  .use { border: 1px solid var(--accent-text); color: var(--accent-text); }
  .ghost { border: 1px solid var(--line); color: var(--ink); }
  .use:disabled, .more:disabled, .crumb:disabled { opacity: .6; cursor: progress; }
  @media (pointer: coarse) { .tools input.filter, .use, .ghost, .more { height: 44px; min-height: 44px; } .hidden { min-height: 44px; } button.crumb { min-height: 44px; min-width: 24px; } }

  /* Phone: the sheet's own rows instead of a boxed panel; the folders a card of 52 px rows; fields 16 px so focusing never zooms. */
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .browser { border: 0; border-radius: 0; background: transparent; gap: 12px; }
    .crumb-row { padding: 0; border: 0; font-size: 14px; min-height: 36px; }
    .tools { padding: 0; border: 0; gap: 12px; }
    .tools input.filter { height: 48px; min-height: 48px; padding: 0 14px; border-radius: 10px; background: var(--card); font-size: 16px; }
    .hidden { min-height: 44px; gap: 10px; color: var(--ink-soft); font-size: 14px; }
    .hidden input, .box { width: 22px; height: 22px; }
    .box { border-radius: 5px; }
    .scroll { height: auto; overflow: visible; border: 1px solid var(--line); border-radius: 13px; background: var(--card); }
    .row { height: auto; min-height: 52px; border-bottom: 1px solid var(--line-soft); }
    li:last-child .row { border-bottom: 0; }
    .icon { width: 18px; height: 18px; }
    .name { font-size: 14px; }
    .foot { border: 0; }
    .here-row, .note-row, .browse-error { padding: 0; flex-wrap: wrap; }
    .note-row, .browse-error { padding-top: 10px; }
  }
</style>
