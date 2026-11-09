<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { shell } from '../dialogShell'
  import { toolName } from '../agentLaunch'
  import { resolvePrompt, searchPrompts, suggestionLine, type PromptContext } from '../terminal/prompts'
  import type { TerminalPrompt } from '../terminal/api'
  import { readPromptLibrary, type PromptLibrary } from './library'

  // A searchable prompt picker shared by the Start dialogs and the terminal palette. The preview is the
  // text that will actually be inserted: variables are filled from `context`, and skill tokens become
  // the shelf's ready-to-use text. Nothing is sent; the parent decides where the text belongs.
  let { opener = null, sheet = false, context = {}, limit = 8000, oninsert, onclose }: {
    opener?: HTMLElement | null
    sheet?: boolean
    context?: PromptContext
    limit?: number
    oninsert: (result: { prompt: TerminalPrompt; text: string; unresolved: string[] }) => void
    onclose: () => void
  } = $props()

  const from = untrack(() => opener)
  const id = $props.id()
  let dialog = $state<HTMLDialogElement>(), search = $state<HTMLInputElement>(), list = $state<HTMLDivElement>()
  let library = $state<PromptLibrary | null>(null)
  let loading = $state(true), error = $state(''), query = $state(''), active = $state(0)
  let life: AbortController | undefined

  const filtered = $derived(library ? searchPrompts(library.prompts, query) : [])
  const selected = $derived(filtered[Math.min(active, Math.max(0, filtered.length - 1))])
  const resolution = $derived(selected && library ? resolvePrompt(selected.body, context, library.skills) : { text: '', unresolved: [] })
  const tooLong = $derived(new TextEncoder().encode(resolution.text).length > limit)
  const suggestion = $derived(selected ? suggestionLine(selected, toolName) : '')
  const status = $derived(loading ? 'Reading your prompts…' : error ? error : filtered.length === 0 ? (query ? 'No prompts match.' : 'No prompts yet.') : `${filtered.length} ${filtered.length === 1 ? 'prompt' : 'prompts'}`)

  $effect(() => { if (filtered.length && active >= filtered.length) active = 0 })

  async function load() {
    life?.abort()
    const mine = life = new AbortController()
    loading = true
    error = ''
    try {
      library = await readPromptLibrary(mine.signal)
    } catch (e) {
      if (!mine.signal.aborted) error = e instanceof Error && e.message ? e.message : 'Prompts could not be read. Try again.'
    } finally {
      if (!mine.signal.aborted) loading = false
    }
  }

  function close() {
    if (dialog?.open) dialog.close()
    if (from?.isConnected) from.focus()
    onclose()
  }

  function insert(prompt = selected) {
    if (!prompt || !library) return
    const resolved = resolvePrompt(prompt.body, context, library.skills)
    if (new TextEncoder().encode(resolved.text).length > limit) return
    oninsert({ prompt, text: resolved.text, unresolved: resolved.unresolved })
    close()
  }

  function rows() { return [...(list?.querySelectorAll<HTMLElement>('[role=option]') ?? [])] }
  function move(delta: number) {
    if (!filtered.length) return
    active = (Math.min(active, filtered.length - 1) + delta + filtered.length) % filtered.length
    void tick().then(() => rows()[active]?.scrollIntoView({ block: 'nearest' }))
  }
  function keys(event: KeyboardEvent) {
    if (event.key === 'ArrowDown') { event.preventDefault(); move(1) }
    else if (event.key === 'ArrowUp') { event.preventDefault(); move(-1) }
    else if (event.key === 'Home') { event.preventDefault(); active = 0 }
    else if (event.key === 'End') { event.preventDefault(); active = Math.max(0, filtered.length - 1) }
    else if (event.key === 'Enter' && event.target === search) { event.preventDefault(); insert() }
  }

  onMount(() => {
    dialog?.showModal()
    void load()
    void tick().then(() => search?.focus())
    return () => { life?.abort(); if (dialog?.open) dialog.close() }
  })
</script>

<dialog bind:this={dialog} use:shell={sheet ? { sheet: 'bottom' } : undefined} aria-labelledby={`${id}-title`}
  oncancel={(event) => { event.preventDefault(); close() }}
  onclick={(event) => { if (event.target === dialog) close() }}>
  <div class="card" data-shell-card>
    <header data-shell-head>
      <div>
        <p class="eyebrow">Your prompt library</p>
        <h2 id={`${id}-title`}>Use a prompt</h2>
      </div>
      <button type="button" class="close" data-shell-close aria-label="Close" onclick={close}>
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
      </button>
    </header>

    <div class="body" data-shell-body>
      <label class="search">
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
        <span class="vh">Search prompts</span>
        <input bind:this={search} bind:value={query} type="search" placeholder="Search by name, category or words" autocomplete="off" spellcheck="false"
          aria-controls={`${id}-list`} onkeydown={keys} />
      </label>

      {#if loading}
        <p class="note" role="status">Reading your prompts…</p>
      {:else if error}
        <div class="note" role="alert"><p>{error}</p><button type="button" class="btn quiet" onclick={() => void load()}>Try again</button></div>
      {:else if filtered.length === 0}
        <div class="empty">
          <p class="empty-title">{query ? 'No prompts match.' : 'Your prompt library is empty.'}</p>
          <p>{query ? 'Try a different word, or clear the search.' : 'Write the prompts you reach for once, then use them here or with a backslash in a session.'}</p>
          <a class="btn" href="/prompts" target="_blank" rel="noopener">{query ? 'Open your prompts' : 'Write your first prompt'}</a>
        </div>
      {:else}
        <div class="columns">
          <div class="list" id={`${id}-list`} role="listbox" aria-label="Your prompts" bind:this={list}>
            {#each filtered as prompt, i (prompt.id)}
              <button type="button" class="option" role="option" aria-selected={i === Math.min(active, filtered.length - 1)}
                onclick={() => (active = i)} ondblclick={() => insert(prompt)}
                onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); insert() } else keys(event) }}>
                <span class="option-title">{prompt.title}</span>
                {#if prompt.description}<span class="option-description">{prompt.description}</span>{/if}
                <span class="option-meta">
                  {#if prompt.category}<span class="chip">{prompt.category}</span>{/if}
                  {#if prompt.tool}<span>{toolName(prompt.tool)}</span>{/if}
                </span>
              </button>
            {/each}
          </div>

          <article class="preview" aria-live="polite">
            {#if selected}
              <div class="preview-head">
                <h3>{selected.title}</h3>
                {#if selected.category}<span class="chip">{selected.category}</span>{/if}
              </div>
              {#if selected.description}<p class="description">{selected.description}</p>{/if}
              {#if suggestion}<p class="suggestion">{suggestion}</p>{/if}
              {#if selected.body.includes('{{skill:')}<p class="shelf-note">Skill prompts are filled from your shelf as they are now.</p>{/if}
              <pre class="text">{resolution.text}</pre>
              {#if resolution.unresolved.length}
                <p class="warn" role="status">{resolution.unresolved.join(', ')} {resolution.unresolved.length === 1 ? 'is' : 'are'} not known here; {resolution.unresolved.length === 1 ? 'it stays' : 'they stay'} as written. You can edit {resolution.unresolved.length === 1 ? 'it' : 'them'} before starting.</p>
              {/if}
              {#if tooLong}<p class="warn" role="alert">This prompt is too long for this box. Shorten it, or paste it after the box opens.</p>{/if}
            {:else}
              <p class="note">Choose a prompt to see what will be inserted.</p>
            {/if}
          </article>
        </div>
      {/if}
    </div>

    <footer data-shell-foot>
      <p class="status" role="status">{status}</p>
      <a class="manage" href="/prompts" target="_blank" rel="noopener">Manage prompts</a>
      <button type="button" class="btn quiet" onclick={close}>Cancel</button>
      <button type="button" class="btn primary" disabled={!selected || tooLong || loading || !!error} onclick={() => insert()}>Insert prompt</button>
    </footer>
  </div>
</dialog>

<style>
  dialog { width: min(860px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; border: 1px solid var(--line); border-radius: 16px; background: var(--card); color: var(--ink); overflow: hidden; }
  dialog[open] { display: flex; flex-direction: column; }
  dialog::backdrop { background: color-mix(in srgb, var(--ink) 45%, transparent); }
  .card { display: flex; flex-direction: column; min-height: 0; max-height: calc(100dvh - 28px); }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 18px 20px 14px; border-bottom: 1px solid var(--line); }
  .eyebrow { margin: 0 0 4px; color: var(--accent-text); font: 500 14px/1.4 var(--mono); }
  h2 { margin: 0; font: 600 1.45rem/1.2 var(--serif); }
  .close { display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 8px; background: none; color: var(--muted); cursor: pointer; }
  .close:hover { background: var(--tint); color: var(--ink); }
  .close svg { width: 20px; height: 20px; }
  .body { display: flex; flex-direction: column; gap: 12px; min-height: 0; flex: 1; overflow: hidden; padding: 14px 20px; }
  .search { display: flex; align-items: center; gap: 8px; min-height: 40px; padding: 0 10px; border: 1px solid var(--line); border-radius: 9px; background: var(--bg); }
  .search:focus-within { border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent); }
  .search svg { width: 18px; height: 18px; color: var(--muted); flex: none; }
  .search input { flex: 1; min-width: 0; border: 0; outline: none; background: none; color: var(--ink); font: inherit; font-size: 15px; }
  .columns { display: grid; grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.05fr); gap: 14px; min-height: 0; flex: 1; }
  .list { display: flex; flex-direction: column; gap: 4px; min-height: 0; overflow: auto; padding: 2px; }
  .option { display: flex; flex-direction: column; gap: 3px; width: 100%; min-height: 56px; padding: 9px 10px; border: 1px solid transparent; border-radius: 9px; background: none; color: var(--ink); text-align: left; font: inherit; cursor: pointer; }
  .option:hover { background: var(--tint); }
  .option[aria-selected='true'] { border-color: color-mix(in srgb, var(--accent) 55%, var(--line)); background: color-mix(in srgb, var(--accent) 9%, var(--card)); }
  .option:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  .option-title { font-weight: 600; overflow-wrap: anywhere; }
  .option-description { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; color: var(--muted); font-size: 14px; line-height: 1.4; }
  .option-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; color: var(--muted); font-size: 13px; }
  .chip { display: inline-flex; align-items: center; max-width: 100%; padding: 2px 7px; border-radius: 5px; background: var(--tint); color: var(--muted); font-size: 13px; overflow-wrap: anywhere; }
  .preview { display: flex; flex-direction: column; gap: 8px; min-height: 0; overflow: auto; padding: 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .preview-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  h3 { margin: 0; font: 600 1.05rem/1.3 var(--sans); overflow-wrap: anywhere; }
  .description { margin: 0; color: var(--muted); font-size: 14px; line-height: 1.5; }
  .suggestion { margin: 0; color: var(--accent-text); font-size: 14px; }
  .shelf-note { margin: 0; color: var(--muted); font-size: 13px; }
  .text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: 14px/1.65 var(--sans); color: var(--ink); }
  .warn { margin: 0; padding: 8px 10px; border-radius: 8px; background: color-mix(in srgb, var(--warn) 12%, transparent); color: var(--warn); font-size: 14px; line-height: 1.45; }
  .note, .empty p { margin: 0; color: var(--muted); font-size: 14px; line-height: 1.5; }
  .note p { margin: 0 0 8px; }
  .empty { display: grid; justify-items: start; gap: 8px; padding: 24px 12px; }
  .empty-title { color: var(--ink) !important; font-weight: 600; font-size: 16px !important; }
  footer { display: flex; align-items: center; flex-wrap: wrap; gap: 10px 12px; padding: 12px 20px; border-top: 1px solid var(--line); }
  .status { flex: 1 1 10em; min-width: 0; margin: 0; color: var(--muted); font-size: 14px; }
  .manage { margin-left: auto; color: var(--ink-soft); font-size: 14px; text-decoration: underline; text-underline-offset: 3px; }
  @media (max-width: 720px) {
    .columns { grid-template-columns: minmax(0, 1fr); overflow: auto; }
    .list { max-height: 42vh; }
    .preview { max-height: 34vh; }
    header, .body, footer { padding-inline: 16px; }
    .manage { order: 4; width: 100%; }
  }
  @media (pointer: coarse) {
    .option { min-height: 60px; padding: 10px 12px; }
    .search { min-height: 44px; }
    footer .btn { min-height: 44px; }
  }
</style>
