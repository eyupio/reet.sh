<script lang="ts">
  import { onMount, tick } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import Skeleton from './Skeleton.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import Toast from './Toast.svelte'
  import { shell } from './dialogShell'
  import SkillPromptPicker from './home/SkillPromptPicker.svelte'
  import { CATALOGUE } from './catalogue'
  import { elapsed } from './time'
  import { toolName } from './agentLaunch'
  import type { Skill } from './api'
  import { readPromptLibrary, type PromptLibrary } from './prompts/library'
  import { removePrompt, savePrompt, TerminalError, type TerminalPrompt, type TerminalPromptWrite } from './terminal/api'
  import { MAX_PROMPT_BODY_BYTES, resolvePrompt, searchPrompts, suggestionLine, validatePrompt, type PromptErrors } from './terminal/prompts'

  let { canWrite = true }: { canWrite?: boolean } = $props()

  type Draft = TerminalPromptWrite & { id: string; revision: number }
  const blank = (): Draft => ({ id: '', revision: 0, title: '', description: '', body: '', category: '', tool: '', model: '', effort: '' })
  const toDraft = (prompt: TerminalPrompt): Draft => ({
    id: prompt.id, revision: prompt.revision, title: prompt.title, description: prompt.description,
    body: prompt.body, category: prompt.category, tool: prompt.tool, model: prompt.model, effort: prompt.effort,
  })

  let prompts = $state<TerminalPrompt[]>([])
  let skillTexts = $state<Record<string, string>>({})
  let loading = $state(true), error = $state('')
  let search = $state(''), category = $state('')
  let life: AbortController | undefined

  let form = $state<Draft>(blank())
  let formErrors = $state<PromptErrors>({})
  let editorOpen = $state(false), saving = $state(false), editorError = $state('')
  let baseline = $state(''), discardOpen = $state(false), dirty = $derived(JSON.stringify(form) !== baseline)
  let dialog = $state<HTMLDialogElement>(), titleInput = $state<HTMLInputElement>(), bodyInput = $state<HTMLTextAreaElement>()
  let skillOpen = $state(false), skillButton = $state<HTMLButtonElement>()
  let confirmDelete = $state<TerminalPrompt | null>(null), deleting = $state(false)
  let importing = $state(false), editorOpener: HTMLElement | null = null
  let toast = $state<{ id: number; text: string; bad: boolean } | null>(null), toastSeq = 0

  const categories = $derived([...new Set(prompts.map(prompt => prompt.category).filter(Boolean))].sort((a, b) => a.localeCompare(b)))
  const filtered = $derived(searchPrompts(prompts, search).filter(prompt => !category || prompt.category === category))
  const previewContext = { project: 'Your project', branch: 'main', folder: '/home/you/project', sessionTitle: 'New session' }
  const preview = $derived(resolvePrompt(form.body, previewContext, skillTexts))
  const suggestion = $derived(suggestionLine(form, toolName))
  const bodyBytes = $derived(new TextEncoder().encode(form.body).length)
  const status = $derived(loading ? 'Reading your prompts…' : error ? 'Prompts unavailable' : `${filtered.length} ${filtered.length === 1 ? 'prompt' : 'prompts'}${category ? ` in ${category}` : ''}`)

  function notify(text: string, bad = false) { toast = { id: ++toastSeq, text, bad } }

  async function load() {
    life?.abort()
    const mine = life = new AbortController()
    loading = true; error = ''
    try {
      const library: PromptLibrary = await readPromptLibrary(mine.signal)
      if (mine.signal.aborted) return
      prompts = library.prompts
      skillTexts = library.skills
    } catch (e) {
      if (!mine.signal.aborted) error = e instanceof Error && e.message ? e.message : 'Could not read your prompts. Try again in a moment.'
    } finally {
      if (!mine.signal.aborted) loading = false
    }
  }

  function openEditor(prompt?: TerminalPrompt, copy = false) {
    editorOpener = document.activeElement instanceof HTMLElement ? document.activeElement : null
    form = prompt ? toDraft(prompt) : blank()
    if (copy && prompt) { form.id = ''; form.revision = 0; form.title = `${prompt.title} copy` }
    formErrors = {}; editorError = ''; skillOpen = false
    baseline = JSON.stringify(form)
    editorOpen = true
  }

  function closeEditor(force = false) {
    if (!force && dirty) { discardOpen = true; return }
    discardOpen = false; editorOpen = false; skillOpen = false
    void tick().then(() => { if (editorOpener?.isConnected) editorOpener.focus() })
  }

  async function save() {
    if (saving) return
    formErrors = validatePrompt(form)
    if (Object.keys(formErrors).length) { void tick().then(() => titleInput?.focus()); return }
    const current = form.id ? prompts.find(prompt => prompt.id === form.id) : undefined
    if (form.id && !current) { editorError = 'This prompt is no longer in your library. Reload the list before saving it again.'; return }
    saving = true; editorError = ''
    try {
      const saved = await savePrompt({
        title: form.title, description: form.description, body: form.body, category: form.category,
        tool: form.tool, model: form.model, effort: form.effort,
      }, current)
      prompts = [...prompts.filter(prompt => prompt.id !== saved.id), saved].sort((a, b) => a.title.localeCompare(b.title))
      closeEditor(true)
      notify(`Saved “${saved.title}”.`)
    } catch (e) {
      editorError = e instanceof TerminalError && e.status === 409
        ? 'This prompt changed in another browser. Reload the list, then make your change again.'
        : e instanceof Error && e.message ? e.message : 'The prompt was not saved. Try again.'
    } finally {
      saving = false
    }
  }

  async function remove() {
    const item = confirmDelete
    if (!item || deleting) return
    deleting = true
    try {
      await removePrompt(item)
      prompts = prompts.filter(prompt => prompt.id !== item.id)
      notify(`Removed “${item.title}”.`)
    } catch (e) {
      notify(e instanceof TerminalError && e.status === 409 ? 'That prompt changed in another browser. Refresh to see the latest version.' : e instanceof Error && e.message ? e.message : 'The prompt was not removed.', true)
    } finally {
      deleting = false; confirmDelete = null
    }
  }

  function insertToken(token: string) {
    const at = bodyInput?.selectionStart ?? form.body.length
    const end = bodyInput?.selectionEnd ?? at
    form.body = form.body.slice(0, at) + token + form.body.slice(end)
    const caret = at + token.length
    void tick().then(() => { bodyInput?.focus(); bodyInput?.setSelectionRange(caret, caret) })
  }

  function insertSkill(skill: Skill) {
    const key = skill.slug || skill.id
    if (key) insertToken(`{{skill:${key}}}`)
  }

  function clearFilters() { search = ''; category = '' }
  function updatedLabel(ts: number) { const age = elapsed(ts); return age === 'just now' ? 'Updated just now' : `Updated ${age} ago` }

  function exportPrompts() {
    if (!prompts.length) return
    const payload = {
      kind: 'reet-prompts', version: 1, exportedAt: new Date().toISOString(),
      prompts: prompts.map(({ title, description, body, category, tool, model, effort }) => ({ title, description, body, category, tool, model, effort })),
    }
    const url = URL.createObjectURL(new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' }))
    const link = document.createElement('a')
    link.href = url; link.download = 'reet-prompts.json'; link.click()
    setTimeout(() => URL.revokeObjectURL(url), 0)
  }

  async function importPrompts(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    input.value = ''
    if (!file || importing) return
    importing = true
    try {
      const raw: unknown = JSON.parse(await file.text())
      const list = Array.isArray(raw) ? raw : raw && typeof raw === 'object' && Array.isArray((raw as { prompts?: unknown }).prompts) ? (raw as { prompts: unknown[] }).prompts : null
      if (!list) { notify('That file is not a reet prompts file.', true); return }
      const valid: TerminalPromptWrite[] = []
      let skipped = 0
      for (const item of list) {
        if (!item || typeof item !== 'object') { skipped++; continue }
        const value = item as Record<string, unknown>
        const prompt: TerminalPromptWrite = {
          title: typeof value.title === 'string' ? value.title : '',
          description: typeof value.description === 'string' ? value.description : '',
          body: typeof value.body === 'string' ? value.body : '',
          category: typeof value.category === 'string' ? value.category : '',
          tool: typeof value.tool === 'string' ? value.tool : '',
          model: typeof value.model === 'string' ? value.model : '',
          effort: typeof value.effort === 'string' ? value.effort : '',
        }
        if (Object.keys(validatePrompt(prompt)).length) { skipped++; continue }
        valid.push(prompt)
      }
      if (!valid.length) { notify(`Nothing to import.${skipped ? ` ${skipped} ${skipped === 1 ? 'entry was' : 'entries were'} not valid.` : ''}`, true); return }
      let imported = 0
      for (const prompt of valid) {
        try { await savePrompt(prompt) }
        catch (e) {
          const why = e instanceof TerminalError && e.status === 409 ? 'Your library is full.' : e instanceof Error ? e.message : 'A prompt could not be saved.'
          notify(`Imported ${imported}. Stopped: ${why}`, true)
          return
        }
        imported++
      }
      await load()
      notify(`Imported ${imported} ${imported === 1 ? 'prompt' : 'prompts'}${skipped ? `; skipped ${skipped} that ${skipped === 1 ? 'was' : 'were'} not valid` : ''}.`)
    } catch {
      notify('That file is not valid JSON.', true)
    } finally {
      importing = false
    }
  }

  $effect(() => {
    if (editorOpen && dialog && !dialog.open) { dialog.showModal(); void tick().then(() => titleInput?.focus()) }
    if (!editorOpen && dialog?.open) dialog.close()
  })

  onMount(() => {
    void load()
    return () => life?.abort()
  })
</script>

<PageHeader eyebrow="Ey up. Your words, ready." title="Prompts" lede="Write the prompts you reach for once. Use them in a session with a backslash, or pick one when you start an agent.">
  {#snippet actions()}
    {#if canWrite}<button class="btn primary" type="button" onclick={() => openEditor()}>New prompt</button>{/if}
    {#if prompts.length}<button class="btn" type="button" onclick={exportPrompts}>Export</button>{/if}
    {#if canWrite}<label class="btn import" class:busy={importing} aria-busy={importing}>Import<input type="file" accept="application/json,.json" disabled={importing} onchange={(event) => void importPrompts(event)} /></label>{/if}
    <a class="btn" href="/skills">Skills</a>
  {/snippet}
</PageHeader>

{#if !canWrite}
  <p class="notice" role="status">Prompts need an account that can write. Your role can still read the rest of the workspace.</p>
{:else}
  <section class="library" aria-labelledby="prompts-library-title">
    <div class="toolbar">
      <div class="title-row">
        <h2 id="prompts-library-title">Your library</h2>
        <p class="count">{status}</p>
      </div>
      <label class="search">
        <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
        <span class="vh">Search prompts</span>
        <input type="search" bind:value={search} placeholder="Search prompts" autocomplete="off" />
      </label>
    </div>
    {#if categories.length}
      <div class="categories" role="group" aria-label="Filter by category">
        <button type="button" class="chip" aria-pressed={category === ''} onclick={() => (category = '')}>All</button>
        {#each categories as item (item)}
          <button type="button" class="chip" aria-pressed={category === item} onclick={() => (category = item)}>{item}</button>
        {/each}
      </div>
    {/if}

    {#if loading}
      <Skeleton label="Reading your prompts…" shape="rows" count={3} />
    {:else if error}
      <div class="empty" role="alert">
        <p>{error}</p>
        <button type="button" class="btn" onclick={() => void load()}>Try again</button>
      </div>
    {:else if prompts.length === 0}
      <div class="empty">
        <h2>A prompt is a head start.</h2>
        <p>Write the instructions you repeat most. A prompt can fill in the project, branch, folder, session title and the date, and it can pull in a skill's prompt from your shelf.</p>
        <button type="button" class="btn primary" onclick={() => openEditor()}>Write your first prompt</button>
      </div>
    {:else if filtered.length === 0}
      <div class="empty">
        <p>{search ? `No prompts match “${search}”.` : `No prompts in ${category}.`}</p>
        <button type="button" class="btn" onclick={clearFilters}>Clear filters</button>
      </div>
    {:else}
      <ul class="list">
        {#each filtered as prompt (prompt.id)}
          <li class="prompt">
            <div class="copy">
              <div class="prompt-title">
                <h3>{prompt.title}</h3>
                {#if prompt.category}<span class="chip static">{prompt.category}</span>{/if}
              </div>
              {#if prompt.description}<p class="description">{prompt.description}</p>{/if}
              <p class="meta">
                <span>{prompt.body.length.toLocaleString('en-GB')} characters</span>
                {#if suggestionLine(prompt, toolName)}<span>{suggestionLine(prompt, toolName)}</span>{/if}
                <span>{updatedLabel(prompt.updatedAt)}</span>
              </p>
            </div>
            <div class="actions">
              <button type="button" class="btn quiet" onclick={() => openEditor(prompt)}>Edit</button>
              <button type="button" class="btn quiet" onclick={() => openEditor(prompt, true)}>Duplicate</button>
              <button type="button" class="btn quiet danger" onclick={() => (confirmDelete = prompt)}>Delete</button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

{#if editorOpen}
  <dialog bind:this={dialog} use:shell aria-labelledby="prompt-editor-title"
    oncancel={(event) => { event.preventDefault(); closeEditor() }}
    onclick={(event) => { if (event.target === dialog) closeEditor() }}>
    <div class="editor-card" data-shell-card>
      <header data-shell-head>
        <div>
          <p class="eyebrow">{form.id ? 'Edit prompt' : 'New prompt'}</p>
          <h2 id="prompt-editor-title">{form.title.trim() || 'Untitled prompt'}</h2>
        </div>
        <button type="button" class="close" data-shell-close aria-label="Close" onclick={() => closeEditor()}>
          <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M6 6l12 12M18 6L6 18" /></svg>
        </button>
      </header>

      <form class="editor" onsubmit={(event) => { event.preventDefault(); void save() }}>
        <div class="body" data-shell-body>
          <div class="fields">
            <label class="field">
              <span>Title <em>Required</em></span>
              <input bind:this={titleInput} bind:value={form.title} maxlength="80" required aria-invalid={formErrors.title ? 'true' : undefined} placeholder="Fix a flaky test" />
              {#if formErrors.title}<small class="reason" role="alert">{formErrors.title}</small>{/if}
            </label>

            <label class="field">
              <span>Description <em>Optional</em></span>
              <textarea bind:value={form.description} rows="2" maxlength="500" aria-invalid={formErrors.description ? 'true' : undefined} placeholder="When this prompt is useful, in one line."></textarea>
              {#if formErrors.description}<small class="reason" role="alert">{formErrors.description}</small>{/if}
            </label>

            <label class="field">
              <span>Category <em>Optional</em></span>
              <input bind:value={form.category} maxlength="40" list="prompt-categories" aria-invalid={formErrors.category ? 'true' : undefined} placeholder="Quality, Release, Research…" />
              <datalist id="prompt-categories">{#each categories as item (item)}<option value={item}></option>{/each}</datalist>
              {#if formErrors.category}<small class="reason" role="alert">{formErrors.category}</small>{/if}
            </label>

            <div class="field wide">
              <label for="prompt-body">Prompt <em>Required</em></label>
              <textarea id="prompt-body" bind:this={bodyInput} bind:value={form.body} rows="10" required aria-invalid={formErrors.body ? 'true' : undefined} placeholder="What should the agent do? Include the context, the outcome you want, and anything it should keep in mind."></textarea>
              <span class="body-foot">
                <span class="tokens" role="group" aria-label="Insert a variable">
                  {#each ['project', 'branch', 'folder', 'sessionTitle', 'date'] as token (token)}
                    <button type="button" class="token" onclick={() => insertToken(`{{${token}}}`)}>{`{{${token}}}`}</button>
                  {/each}
                  <button type="button" class="token skill-token" bind:this={skillButton} aria-haspopup="listbox" aria-expanded={skillOpen} onclick={() => (skillOpen = !skillOpen)}>Insert a skill…</button>
                </span>
                <span class="bytes" class:over={bodyBytes > MAX_PROMPT_BODY_BYTES}>{bodyBytes.toLocaleString('en-GB')} / {MAX_PROMPT_BODY_BYTES.toLocaleString('en-GB')} bytes</span>
              </span>
              {#if formErrors.body}<small class="reason" role="alert">{formErrors.body}</small>{/if}
            </div>

            <div class="suggested wide">
              <div class="suggested-head">
                <strong>Suggested start</strong>
                <span>Prefilled when you pick this prompt, never forced.</span>
              </div>
              <div class="suggested-grid">
                <label class="field">
                  <span>Agent</span>
                  <select bind:value={form.tool} aria-invalid={formErrors.tool ? 'true' : undefined}>
                    <option value="">Any agent</option>
                    {#if form.tool && !CATALOGUE.some(agent => agent.id === form.tool && agent.enabled)}<option value={form.tool}>{form.tool}</option>{/if}
                    {#each CATALOGUE.filter(agent => agent.enabled) as agent (agent.id)}<option value={agent.id}>{agent.name}</option>{/each}
                  </select>
                  {#if formErrors.tool}<small class="reason" role="alert">{formErrors.tool}</small>{/if}
                </label>
                <label class="field">
                  <span>Model</span>
                  <input bind:value={form.model} maxlength="200" aria-invalid={formErrors.model ? 'true' : undefined} placeholder="Agent default" />
                  {#if formErrors.model}<small class="reason" role="alert">{formErrors.model}</small>{/if}
                </label>
                <label class="field">
                  <span>Effort</span>
                  <input bind:value={form.effort} maxlength="64" aria-invalid={formErrors.effort ? 'true' : undefined} placeholder="Agent default" />
                  {#if formErrors.effort}<small class="reason" role="alert">{formErrors.effort}</small>{/if}
                </label>
              </div>
            </div>
          </div>

          <aside class="preview" aria-label="Prompt preview">
            <div class="preview-head"><strong>Preview</strong><span>The project, branch and folder are samples.</span></div>
            {#if preview.text.trim()}<pre class="preview-text">{preview.text}</pre>{:else}<p class="preview-empty">Your prompt appears here as you write.</p>{/if}
            {#if preview.unresolved.length}<p class="warn" role="status">{preview.unresolved.join(', ')} {preview.unresolved.length === 1 ? 'is' : 'are'} not filled in the preview. {preview.unresolved.length === 1 ? 'It stays' : 'They stay'} as written when the prompt is used, until the place fills it.</p>{/if}
            {#if suggestion}<p class="preview-suggestion">{suggestion}</p>{/if}
          </aside>
        </div>

        <footer data-shell-foot>
          <p class="editor-status" class:bad={!!editorError} role="status">{editorError || (dirty ? 'Unsaved changes.' : form.id ? 'Saved in your account.' : 'Not saved yet.')}</p>
          <button type="button" class="btn" disabled={saving} onclick={() => closeEditor()}>Cancel</button>
          <button type="submit" class="btn primary" disabled={saving || bodyBytes > MAX_PROMPT_BODY_BYTES} aria-busy={saving}>{saving ? 'Saving…' : form.id ? 'Save changes' : 'Save prompt'}</button>
        </footer>
      </form>
    </div>
  </dialog>
{/if}

{#if skillOpen}
  <SkillPromptPicker opener={skillButton ?? null} sheet={true} oninsert={insertSkill} onclose={() => { skillOpen = false; void tick().then(() => bodyInput?.focus()) }} />
{/if}

{#if discardOpen}
  <ConfirmDialog title="Discard this prompt?" body="Your unsaved changes will be lost." confirmLabel="Discard changes" danger
    onconfirm={() => closeEditor(true)} oncancel={() => (discardOpen = false)} />
{/if}

{#if confirmDelete}
  <ConfirmDialog title={`Delete “${confirmDelete.title}”?`} body="The prompt is removed from your account. Sessions that already used it are not changed." confirmLabel="Delete prompt" danger
    onconfirm={() => void remove()} oncancel={() => (confirmDelete = null)} />
{/if}

{#if toast}
  {#key toast.id}<Toast message={toast.text} tone={toast.bad ? 'bad' : 'ok'} placement="right" ondismiss={() => (toast = null)} />{/key}
{/if}

<style>
  .library { display: grid; gap: 16px; }
  .import { position: relative; cursor: pointer; }
  .import input { position: absolute; width: 1px; height: 1px; opacity: 0; pointer-events: none; }
  .import.busy { opacity: .6; cursor: default; }
  .toolbar { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
  .title-row { display: flex; align-items: baseline; gap: 12px; flex-wrap: wrap; }
  h2 { margin: 0; font: 600 1.25rem var(--sans); }
  .count { margin: 0; color: var(--muted); font-size: 14px; }
  .search { display: flex; align-items: center; gap: 8px; min-width: min(340px, 100%); min-height: 40px; padding: 0 11px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); }
  .search:focus-within { border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent); }
  .search svg { width: 18px; height: 18px; color: var(--muted); flex: none; }
  .search input { flex: 1; min-width: 0; border: 0; outline: none; background: none; color: var(--ink); font: inherit; font-size: 15px; }
  .categories { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { display: inline-flex; align-items: center; min-height: 32px; padding: 0 10px; border: 1px solid var(--line); border-radius: 999px; background: transparent; color: var(--muted); font: inherit; font-size: 14px; cursor: pointer; }
  .chip[aria-pressed='true'] { border-color: color-mix(in srgb, var(--accent) 55%, var(--line)); background: color-mix(in srgb, var(--accent) 10%, var(--card)); color: var(--ink); }
  .chip.static { cursor: default; min-height: 0; padding: 2px 8px; font-size: 13px; }
  .list { display: grid; gap: 8px; list-style: none; margin: 0; padding: 0; }
  .prompt { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 14px 16px; border: 1px solid var(--line); border-radius: 11px; background: var(--card); }
  .copy { display: grid; gap: 5px; min-width: 0; }
  .prompt-title { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  h3 { margin: 0; font: 600 1rem/1.35 var(--sans); overflow-wrap: anywhere; }
  .description { margin: 0; color: var(--ink-soft); font-size: 14px; line-height: 1.5; overflow-wrap: anywhere; }
  .meta { display: flex; flex-wrap: wrap; gap: 4px 12px; margin: 0; color: var(--muted); font-size: 13px; }
  .actions { display: flex; align-items: center; gap: 4px; flex: none; }
  .actions .btn { min-height: 34px; padding: 0 10px; font-size: 14px; }
  .danger { color: var(--bad); }
  .empty { display: grid; justify-items: start; gap: 10px; padding: 32px 20px; border: 1px dashed var(--line-strong); border-radius: 12px; background: var(--card); }
  .empty h2 { font: 600 1.15rem var(--sans); }
  .empty p { margin: 0; max-width: 70ch; color: var(--muted); font-size: 14px; line-height: 1.6; }
  .notice { margin: 0 0 16px; padding: 14px 16px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); color: var(--muted); font-size: 14px; }

  dialog { width: min(1080px, calc(100vw - 28px)); max-height: calc(100dvh - 28px); padding: 0; border: 1px solid var(--line); border-radius: 16px; background: var(--card); color: var(--ink); overflow: hidden; }
  dialog[open] { display: flex; flex-direction: column; }
  dialog::backdrop { background: color-mix(in srgb, var(--ink) 45%, transparent); }
  .editor-card { display: flex; flex-direction: column; min-height: 0; max-height: calc(100dvh - 28px); }
  .editor-card > header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 16px 20px 12px; border-bottom: 1px solid var(--line); }
  .eyebrow { margin: 0 0 3px; color: var(--accent-text); font: 500 13px/1.4 var(--mono); }
  .editor-card h2 { font: 600 1.35rem/1.2 var(--serif); overflow-wrap: anywhere; }
  .close { display: grid; place-items: center; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 8px; background: none; color: var(--muted); cursor: pointer; }
  .close:hover { background: var(--tint); color: var(--ink); }
  .close svg { width: 20px; height: 20px; }
  .editor { display: grid; grid-template-rows: minmax(0, 1fr) auto; min-height: 0; }
  .body { display: grid; grid-template-columns: minmax(0, 1.05fr) minmax(0, .95fr); gap: 18px; min-height: 0; overflow: auto; padding: 16px 20px; align-content: start; }
  .fields { display: grid; gap: 12px; min-width: 0; align-content: start; }
  .field { display: grid; gap: 5px; min-width: 0; }
  .field > span:first-child, .field > label:first-child { font-size: 14px; font-weight: 600; }
  .field em { margin-left: 6px; color: var(--muted); font-size: 12px; font-style: normal; font-weight: 400; }
  input, textarea, select { width: 100%; box-sizing: border-box; padding: 9px 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); color: var(--ink); font: inherit; font-size: 14px; }
  textarea { resize: vertical; line-height: 1.6; }
  input:focus-visible, textarea:focus-visible, select:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  [aria-invalid='true'] { border-color: var(--bad); }
  .reason { color: var(--bad); font-size: 13px; line-height: 1.4; }
  .body-foot { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
  .tokens { display: flex; flex-wrap: wrap; gap: 4px; }
  .token { min-height: 28px; padding: 0 7px; border: 1px solid var(--line); border-radius: 6px; background: transparent; color: var(--accent-text); font: 500 12px/1 var(--mono); cursor: pointer; }
  .token:hover { background: var(--tint); }
  .bytes { color: var(--muted); font-size: 12px; font-variant-numeric: tabular-nums; }
  .bytes.over { color: var(--bad); font-weight: 600; }
  .suggested { display: grid; gap: 8px; padding: 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .suggested-head { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
  .suggested-head strong { font-size: 14px; }
  .suggested-head span { color: var(--muted); font-size: 13px; }
  .suggested-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
  .preview { display: flex; flex-direction: column; gap: 8px; min-height: 240px; min-width: 0; padding: 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .preview-head { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
  .preview-head strong { font-size: 14px; }
  .preview-head span { color: var(--muted); font-size: 12px; }
  .preview-text { margin: 0; flex: 1; min-height: 0; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; font: 14px/1.65 var(--sans); }
  .preview-empty { margin: 0; color: var(--muted); font-size: 14px; }
  .warn { margin: 0; padding: 7px 9px; border-radius: 7px; background: color-mix(in srgb, var(--warn) 12%, transparent); color: var(--warn); font-size: 13px; line-height: 1.45; }
  .preview-suggestion { margin: 0; color: var(--accent-text); font-size: 13px; }
  footer { display: flex; align-items: center; flex-wrap: wrap; gap: 10px 12px; padding: 12px 20px; border-top: 1px solid var(--line); }
  .editor-status { flex: 1 1 12em; min-width: 0; margin: 0; color: var(--muted); font-size: 14px; }
  .editor-status.bad { color: var(--bad); }
  @media (max-width: 880px) {
    .body { grid-template-columns: minmax(0, 1fr); }
    .preview { min-height: 180px; }
    .suggested-grid { grid-template-columns: minmax(0, 1fr); }
  }
  @media (max-width: 650px) {
    .prompt { flex-direction: column; }
    .actions { width: 100%; }
    .toolbar { align-items: stretch; }
    .search { min-width: 100%; }
    .editor-card > header, .body, footer { padding-inline: 16px; }
    .actions .btn { flex: 1; min-height: 44px; }
  }
  @media (pointer: coarse) {
    .chip, .token, .search, .actions .btn { min-height: 44px; }
  }
</style>
