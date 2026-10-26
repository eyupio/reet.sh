<script lang="ts">
  import { onMount } from 'svelte'
  import { getHosting, claimPublishingUsername, saveHostedSkill, renameHostedSkill, deleteHostedSkill, type HostedSkill } from './api'
  import { MAX_BUNDLE_BYTES, addPrefix, prepareImports, renameSkillContent, sanitizeSkillName, uniqueSkillName, type ImportInput, type PreparedImport } from './skillImport'
  import PageHeader from './PageHeader.svelte'
  import CopyButton from './CopyButton.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let { canWrite = true }: { canWrite?: boolean } = $props()
  let username = $state('')
  let prefix = $state('')
  let wantedName = $state('')
  let skills = $state<HostedSkill[]>([])
  let search = $state('')
  let filter = $state<'all' | 'published' | 'draft'>('all')
  let editor = $state<HostedSkill | null>(null)
  let originalSlug = $state('')
  let baseline = $state('')
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let deleting = $state<HostedSkill | null>(null)
  let discardPrompt = $state<{ title: string; body: string; label: string; run: () => void } | null>(null)
  let importing = $state(false)
  let importInputs = $state<ImportInput[]>([])
  let importBundles = $state<PreparedImport[]>([])
  let importError = $state('')
  let importNotice = $state('')
  let importBusy = $state(false)
  let importProgress = $state('')
  let dropActive = $state(false)
  let filePicker = $state<HTMLInputElement>()
  let folderPicker = $state<HTMLInputElement>()
  const command = $derived(`npx skills add ${location.origin}/${username}/${originalSlug}`)
  const dirty = $derived(editor ? fingerprint(editor) !== baseline : false)
  const chosenImports = $derived(importBundles.filter(skill => skill.selected && !skill.problem))
  const publishedCount = $derived(skills.filter(skill => skill.published).length)
  const draftCount = $derived(skills.length - publishedCount)
  const visibleSkills = $derived(
    skills
      .filter(skill => filter === 'all' || (filter === 'published' ? skill.published : !skill.published))
      .filter(skill => !search.trim() || `${skill.slug} ${skill.description}`.toLowerCase().includes(search.trim().toLowerCase())),
  )
  const slugBody = $derived(prefix && editor?.slug.startsWith(prefix) ? editor.slug.slice(prefix.length) : editor?.slug ?? '')
  const renameNeeded = $derived(!!originalSlug && !!editor && editor.slug !== originalSlug)
  const prefixMisplaced = $derived(!!prefix && !!originalSlug && !!editor && !editor.slug.startsWith(prefix))
  const showPrefixChip = $derived(!!prefix && (!originalSlug || (editor?.slug.startsWith(prefix) ?? false)))
  onMount(() => { void load() })
  function fail(e: unknown) { error = e instanceof Error ? e.message : String(e) }
  function fingerprint(skill: HostedSkill) {
    return JSON.stringify({
      slug: skill.slug,
      content: skill.content,
      published: skill.published,
      files: Object.entries(skill.files).sort(([a], [b]) => a.localeCompare(b)),
    })
  }
  function track(skill: HostedSkill) { baseline = fingerprint(skill) }
  async function load() {
    try { const data = await getHosting(); username = data.username; prefix = data.prefix ?? ''; skills = data.skills }
    catch (e) { fail(e) } finally { loading = false }
  }
  async function claim(event: SubmitEvent) {
    event.preventDefault(); busy = true; error = ''; notice = ''
    try { const data = await claimPublishingUsername(wantedName); username = data.username; prefix = data.prefix ?? ''; skills = data.skills; notice = 'Username claimed. Create your first skill.' }
    catch (e) { fail(e) } finally { busy = false }
  }
  function resetImport() {
    importing = false; importInputs = []; importBundles = []
    importError = ''; importNotice = ''; importBusy = false; importProgress = ''; dropActive = false
  }
  function newSkill() {
    resetImport(); originalSlug = ''; error = ''; notice = ''
    const name = `${prefix}my-skill`
    editor = { slug: name, description: '', content: `---\nname: ${name}\ndescription: Describe what this skill does and when to use it.\n---\n\n# My skill\n\nWrite the instructions your agent should follow here.\n`, files: {}, published: false, createdAt: 0, updatedAt: 0 }
    track(editor)
  }
  function guardImports(run: () => void) {
    if (!importing || !importBundles.length) return false
    discardPrompt = {
      title: 'Discard these imports?',
      body: `You have ${importBundles.length} ${importBundles.length === 1 ? 'skill' : 'skills'} ready to import. Leaving now will discard that list.`,
      label: 'Discard imports',
      run,
    }
    return true
  }
  function requestNewSkill() {
    if (busy || importBusy) return
    if (guardImports(newSkill)) return
    if (editor && dirty) {
      discardPrompt = {
        title: 'Discard this skill?',
        body: originalSlug ? `You have unsaved changes to ${originalSlug}. Continuing will discard them.` : 'You are partway through a new skill. Continuing will discard what you have written.',
        label: 'Discard and continue',
        run: newSkill,
      }
      return
    }
    newSkill()
  }
  function edit(skill: HostedSkill) {
    resetImport(); editor = { ...skill, files: { ...skill.files } }; originalSlug = skill.slug
    error = ''; notice = ''; track(editor)
  }
  function requestEdit(skill: HostedSkill) {
    if (busy || importBusy) return
    if (guardImports(() => edit(skill))) return
    if (editor && dirty) {
      discardPrompt = {
        title: 'Discard this skill?',
        body: originalSlug ? `You have unsaved changes to ${originalSlug}. Continuing will discard them.` : 'You are partway through a new skill. Continuing will discard what you have written.',
        label: 'Discard and continue',
        run: () => edit(skill),
      }
      return
    }
    edit(skill)
  }
  function openImport() {
    if (busy || importBusy) return
    importing = true; error = ''; notice = ''; importError = ''; importNotice = ''
  }
  function setSlugBody(value: string) {
    if (!editor) return
    const clean = sanitizeSkillName(value)
    const body = prefix && clean.startsWith(prefix) ? clean.slice(prefix.length) : clean
    const name = prefix ? `${prefix}${body}` : body
    editor.slug = name
    editor.content = renameSkillContent(editor.content, name)
  }

  function applyPrefix() {
    if (editor) setSlugBody(editor.slug)
  }
  async function save(event: SubmitEvent) {
    event.preventDefault(); if (!editor) return
    if (!editor.slug) { error = 'Give the skill a name before saving.'; return }
    if (prefix && (!originalSlug || editor.slug !== originalSlug) && !editor.slug.startsWith(prefix)) {
      error = `Skill names must start with ${prefix}. Add the prefix or change it in Account → Profile.`
      return
    }
    if (!originalSlug && skills.some(s => s.slug === editor?.slug)) { error = 'You already have a skill with this name. Select it from your skills to edit it.'; return }
    busy = true; error = ''; notice = ''
    let didRename = false
    try {
      let current = editor
      if (originalSlug && editor.slug !== originalSlug) {
        didRename = true
        const renamed = await renameHostedSkill(originalSlug, editor.slug)
        current = { ...editor, slug: renamed.slug, content: renameSkillContent(editor.content, renamed.slug) }
        editor = current
        originalSlug = renamed.slug
        skills = [...skills.filter(s => s.slug !== renamed.slug), renamed].sort((a, b) => a.slug.localeCompare(b.slug))
      }
      const saved = await saveHostedSkill(current)
      editor = saved; originalSlug = saved.slug; track(saved)
      skills = [...skills.filter(s => s.slug !== saved.slug), saved].sort((a, b) => a.slug.localeCompare(b.slug))
      notice = didRename
        ? saved.published ? 'Renamed and published. The public URL and install command changed; existing installs are untouched. It is also on your shelf under To test.' : 'Renamed. Update any private links you shared.'
        : saved.published ? 'Skill published. It is also on your shelf so you can test it.' : 'Draft saved. Its page and install files are private.'
    } catch (e) { fail(e) } finally { busy = false }
  }
  async function upload(event: Event, main = false) {
    const input = event.target as HTMLInputElement
    if (!editor || !input.files) return
    error = ''; busy = true
    try {
      for (const file of Array.from(input.files)) {
        if (file.size > MAX_BUNDLE_BYTES) throw new Error('A skill bundle must be 2 MiB or smaller.')
        if (main) {
          editor.content = await file.text()
          if (!originalSlug) {
            const found = editor.content.match(/^name:\s*["']?([a-z0-9-]+)["']?\s*$/m)?.[1]
            if (found) {
              const name = prefix ? addPrefix(sanitizeSkillName(found), prefix) : sanitizeSkillName(found)
              editor.slug = name
              editor.content = renameSkillContent(editor.content, name)
            }
          }
        } else {
          const bytes = new Uint8Array(await file.arrayBuffer())
          let binary = ''
          for (const byte of bytes) binary += String.fromCharCode(byte)
          editor.files = { ...editor.files, [file.name]: btoa(binary) }
        }
      }
    } catch (e) { fail(e) } finally { input.value = ''; busy = false }
  }
  function renameFile(old: string, name: string) {
    if (!editor || old === name) return
    if (!name || name in editor.files) { error = 'Choose a unique file path.'; return }
    const files = { ...editor.files }; files[name] = files[old]; delete files[old]; editor.files = files
  }
  function removeFile(name: string) { if (!editor) return; const files = { ...editor.files }; delete files[name]; editor.files = files }
  function toBase64(bytes: Uint8Array) {
    let binary = ''
    const chunk = 0x8000
    for (let offset = 0; offset < bytes.length; offset += chunk) {
      const end = Math.min(offset + chunk, bytes.length)
      for (let index = offset; index < end; index++) binary += String.fromCharCode(bytes[index])
    }
    return btoa(binary)
  }
  function importablePath(path: string) {
    const parts = path.replace(/\\/g, '/').split('/').filter(Boolean)
    if (parts.some(part => ['.git', '.hg', '.svn', '__MACOSX'].includes(part))) return false
    return !parts[parts.length - 1]?.startsWith('.')
  }
  async function readDirectory(reader: FileSystemDirectoryReader): Promise<FileSystemEntry[]> {
    const entries: FileSystemEntry[] = []
    while (true) {
      const batch = await new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject))
      if (!batch.length) return entries
      entries.push(...batch)
    }
  }
  async function collectEntry(entry: FileSystemEntry, out: { path: string; file: File }[]) {
    if (entry.isFile) {
      const file = await new Promise<File>((resolve, reject) => (entry as FileSystemFileEntry).file(resolve, reject))
      out.push({ path: entry.fullPath.replace(/^\/+/, '') || file.name, file })
      return
    }
    if (entry.isDirectory) {
      const directory = entry as FileSystemDirectoryEntry
      for (const child of await readDirectory(directory.createReader())) await collectEntry(child, out)
    }
  }
  async function droppedFiles(data: DataTransfer) {
    const out: { path: string; file: File }[] = []
    const roots: FileSystemEntry[] = []
    for (const item of Array.from(data.items)) {
      if (item.kind !== 'file') continue
      const entry = typeof item.webkitGetAsEntry === 'function' ? item.webkitGetAsEntry() : null
      if (entry) roots.push(entry)
      else { const file = item.getAsFile(); if (file) out.push({ path: file.name, file }) }
    }
    for (const root of roots) await collectEntry(root, out)
    if (!out.length) for (const file of Array.from(data.files)) out.push({ path: file.name, file })
    return out
  }
  function rebuildImports(preserve: boolean) {
    const previous = new Map(importBundles.map(skill => [skill.key, skill.selected]))
    importBundles = prepareImports(importInputs, skills.map(skill => skill.slug), prefix).map(skill => ({
      ...skill,
      selected: skill.problem ? false : preserve ? previous.get(skill.key) ?? skill.selected : skill.selected,
    }))
  }
  async function loadImports(collected: { path: string; file: File }[]) {
    const kept = collected.filter(item => importablePath(item.path))
    if (!kept.length) { importBundles = []; importInputs = []; importError = 'Choose a skill folder or SKILL.md file.'; return }
    importBusy = true; importError = ''; importNotice = ''; importBundles = []; importInputs = []
    importProgress = 'Reading files…'
    try {
      if (kept.length > 2000) importNotice = 'Only the first 2,000 files were read.'
      const inputs: ImportInput[] = []
      for (const { path, file } of kept.slice(0, 2000)) {
        if (file.size > MAX_BUNDLE_BYTES) { inputs.push({ path, size: file.size, unread: true }); continue }
        const bytes = new Uint8Array(await file.arrayBuffer())
        if (/\.(md|markdown)$/i.test(path)) inputs.push({ path, size: file.size, markdown: new TextDecoder().decode(bytes), base64: toBase64(bytes) })
        else inputs.push({ path, size: file.size, base64: toBase64(bytes) })
      }
      importInputs = inputs
      rebuildImports(false)
      if (!importBundles.length) importError = 'No skills found. Drop a skill folder, a folder of skills, or choose SKILL.md or markdown skill files.'
    } catch (e) { importError = e instanceof Error ? e.message : String(e) }
    finally { importBusy = false; importProgress = '' }
  }
  async function handleDrop(event: DragEvent) {
    event.preventDefault(); dropActive = false
    if (busy || importBusy || !event.dataTransfer) return
    await loadImports(await droppedFiles(event.dataTransfer))
  }
  function dragOver(event: DragEvent) {
    if (!event.dataTransfer?.types.includes('Files')) return
    event.preventDefault(); dropActive = true
  }
  function windowDragOver(event: DragEvent) {
    if (importing && event.dataTransfer?.types.includes('Files')) event.preventDefault()
  }
  function windowDrop(event: DragEvent) {
    if (importing) event.preventDefault()
  }
  function dragLeave(event: DragEvent) {
    const box = event.currentTarget as HTMLElement
    const next = event.relatedTarget as Node | null
    if (!next || !box.contains(next)) dropActive = false
  }
  async function pickImports(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    const files = Array.from(input.files ?? []).map(file => ({ path: file.webkitRelativePath || file.name, file }))
    input.value = ''
    await loadImports(files)
  }
  function selectImport(index: number, selected: boolean) {
    importBundles = importBundles.map((skill, at) => at === index ? { ...skill, selected } : skill)
  }
  function formatSize(bytes: number) {
    return bytes < 1024 ? `${bytes} B` : `${Math.max(1, Math.round(bytes / 1024))} KB`
  }
  async function runImport() {
    const chosen = [...chosenImports]
    if (!chosen.length || importBusy) return
    importBusy = true; importError = ''; importNotice = ''
    const succeeded = new Set<string>()
    const failures: string[] = []
    const current = new Set(skills.map(skill => skill.slug))
    for (const [index, skill] of chosen.entries()) {
      importProgress = `Importing ${index + 1} of ${chosen.length}…`
      const name = uniqueSkillName(skill.name, current)
      try {
        const saved = await saveHostedSkill({ slug: name, description: skill.description, content: renameSkillContent(skill.content, name), files: skill.files, published: false, createdAt: 0, updatedAt: 0 })
        skills = [...skills.filter(existing => existing.slug !== saved.slug), saved].sort((a, b) => a.slug.localeCompare(b.slug))
        current.add(saved.slug); succeeded.add(skill.key)
      } catch (e) { failures.push(`${name}: ${e instanceof Error ? e.message : String(e)}`) }
    }
    importBusy = false; importProgress = ''
    importBundles = importBundles.filter(skill => !succeeded.has(skill.key))
    if (failures.length) importError = `Imported ${succeeded.size} of ${chosen.length}. ${failures.join(' ')}`
    else {
      const count = succeeded.size
      resetImport()
      notice = `Imported ${count} ${count === 1 ? 'skill' : 'skills'} as ${count === 1 ? 'a draft' : 'drafts'}. Review and publish when ready.`
    }
  }
  async function remove() {
    if (!deleting) return
    const slug = deleting.slug; deleting = null; busy = true; error = ''
    try { await deleteHostedSkill(slug); skills = skills.filter(s => s.slug !== slug); if (editor?.slug === slug) editor = null; notice = 'Skill deleted. Its public page and install files are no longer available.' }
    catch (e) { fail(e) } finally { busy = false }
  }
</script>

<svelte:window ondragover={windowDragOver} ondrop={windowDrop} />
<PageHeader eyebrow="Publish on reet.sh" title="Host your own skills." lede="Write a skill, give it a home, and share one install command." />
{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
{#if notice}<p class="notice good" role="status">{notice}</p>{/if}
{#if loading}<p role="status">Loading your skills…</p>
{:else if !canWrite}<p class="notice">Your account is read-only. Ask an admin for publishing access.</p>
{:else if !username}
  <section class="panel">
    <h2>What hosting gives you</h2>
    <p>Write a skill here and reet.sh serves it at its own address, ready to install with one command. For example:</p>
    <pre><code>npx skills add {location.origin}/your-username/skill-name</code></pre>
    <p>Your username is the first part of that address.</p>
  </section>
  <section class="panel">
    <h2>Choose your publishing username</h2>
    <p>Your skills will live at <code>{location.host}/username/skill-name</code>. This username is permanent, so shared install links keep their owner.</p>
    <form onsubmit={claim}>
      <label for="publisher-name">Username</label>
      <input id="publisher-name" bind:value={wantedName} required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="64" autocomplete="off" placeholder="your-username" disabled={busy} />
      <small>Lowercase letters, numbers and single hyphens.</small>
      <button class="btn primary" disabled={busy}>{busy ? 'Claiming…' : 'Claim username'}</button>
    </form>
  </section>
{:else}
  <div class="toolbar">
    <div class="publisher">
      <p>Publishing as <strong>{username}</strong></p>
      <div class="stats">
        <span class="stat">{publishedCount} published</span>
        <span class="stat">{draftCount} {draftCount === 1 ? 'draft' : 'drafts'}</span>
        {#if prefix}<span class="stat">names start with <code>{prefix}</code></span>{/if}
      </div>
    </div>
    <div class="toolbar-actions"><button class="btn" onclick={openImport} disabled={busy || importBusy}>Import skills</button><button class="btn primary" onclick={requestNewSkill} disabled={busy || importBusy}>New skill</button></div>
  </div>
  <div class="workspace" class:with-dock={editor && !importing}>
    <aside class="panel skills-panel" aria-label="Your hosted skills">
      <div class="panel-head"><h2>Your skills</h2></div>
      {#if skills.length}
        <input class="search" type="search" bind:value={search} placeholder="Search your skills…" aria-label="Search your hosted skills" />
        <div class="filters" role="group" aria-label="Filter hosted skills">
          <button type="button" class="chip" class:on={filter === 'all'} onclick={() => filter = 'all'}>All</button>
          <button type="button" class="chip" class:on={filter === 'published'} onclick={() => filter = 'published'}>Published</button>
          <button type="button" class="chip" class:on={filter === 'draft'} onclick={() => filter = 'draft'}>Drafts</button>
        </div>
      {/if}
      {#if !skills.length}
        <div class="empty"><strong>No skills yet</strong><p>Create a skill or import a folder to get started.</p></div>
      {:else if !visibleSkills.length}
        <p class="hint">No skills match that search.</p>
      {/if}
      <div class="skill-list">
        {#each visibleSkills as skill (skill.slug)}
          <button class="skill-choice" aria-current={originalSlug === skill.slug ? 'true' : undefined} onclick={() => requestEdit(skill)} disabled={busy || importBusy}>
            <span class="choice-top"><strong>{skill.slug}</strong><span class="pill" class:published={skill.published}>{skill.published ? 'Published' : 'Draft'}</span></span>
            <small>{skill.description || 'No description yet'}</small>
            <span class="choice-meta">Updated {new Date(skill.updatedAt).toLocaleDateString()}</span>
          </button>
        {/each}
      </div>
    </aside>
    {#if importing}
      <section class="panel import-panel" aria-labelledby="import-heading">
        <div class="panel-head"><h2 id="import-heading">Import skills</h2><button type="button" class="btn quiet" onclick={resetImport} disabled={importBusy}>Cancel</button></div>
        <p>Drop one or more skill folders, a folder of skill folders, or SKILL.md files. Each skill is saved as a private draft you can review before publishing.</p>
        <button type="button" class="dropzone" class:active={dropActive} onclick={() => filePicker?.click()} ondragover={dragOver} ondragleave={dragLeave} ondrop={handleDrop} disabled={importBusy}>
          <strong>{importBusy ? importProgress || 'Reading files…' : 'Drop files or folders here'}</strong>
          <span>Choose several SKILL.md files, or drop folders that contain SKILL.md and supporting files.</span>
        </button>
        <div class="import-pickers"><button type="button" class="btn" onclick={() => filePicker?.click()} disabled={importBusy}>Choose files</button><button type="button" class="btn" onclick={() => folderPicker?.click()} disabled={importBusy}>Choose folder</button></div>
        <input class="vh" bind:this={filePicker} type="file" multiple accept=".md,.markdown,text/markdown" onchange={pickImports} />
        <input class="vh" bind:this={folderPicker} type="file" multiple webkitdirectory onchange={pickImports} />
        {#if importError}<p class="notice bad" role="alert">{importError}</p>{/if}
        {#if importNotice}<p class="notice good" role="status">{importNotice}</p>{/if}
        {#if importBundles.length}
          {#if prefix}
            <p class="hint">Imported names start with <code>{prefix}</code> from your profile. <a href="/account/profile">Change prefix</a>.</p>
          {:else}
            <p class="hint">Your profile has the hosted skill prefix turned off, so imported names are used as they are. <a href="/account/profile">Add a prefix</a>.</p>
          {/if}
          <ul class="import-list">
            {#each importBundles as skill, index (skill.key)}
              <li class:invalid={!!skill.problem}>
                <label class="import-check">
                  <input type="checkbox" checked={skill.selected} disabled={!!skill.problem || importBusy} onchange={(event) => selectImport(index, event.currentTarget.checked)} />
                  <span><strong>{skill.name}</strong><small>{skill.description || 'No description found'}</small></span>
                </label>
                <div class="import-meta"><span>{skill.fileCount} {skill.fileCount === 1 ? 'file' : 'files'}</span><span>{formatSize(skill.size)}</span>{#if skill.problem}<span class="problem">{skill.problem}</span>{/if}</div>
              </li>
            {/each}
          </ul>
          <div class="actions"><button type="button" class="btn primary" onclick={runImport} disabled={importBusy || !chosenImports.length}>{importBusy ? importProgress || 'Importing…' : `Import ${chosenImports.length} ${chosenImports.length === 1 ? 'skill' : 'skills'}`}</button><button type="button" class="btn quiet" onclick={() => { importBundles = []; importInputs = []; importError = ''; importNotice = '' }} disabled={importBusy}>Clear</button></div>
        {/if}
      </section>
    {:else if editor}
      <section class="panel editor">
        <div class="panel-head">
          <div>
            <h2>{originalSlug ? 'Edit skill' : 'New skill'}</h2>
            <p class="hint">{originalSlug ? `Editing ${originalSlug}` : 'Save a draft first, then publish when it is ready.'}</p>
          </div>
          <span class="pill" class:published={editor.published}>{editor.published ? 'Published' : 'Draft'}</span>
        </div>
        <form id="skill-form" onsubmit={save}>
          <div class="form-block">
            <h3>Name and location</h3>
            <label for="hosted-slug">Skill name</label>
            <div class="slug-row">
              {#if showPrefixChip}<span class="slug-prefix">{prefix}</span>{/if}
              <input id="hosted-slug" value={slugBody} oninput={(event) => setSlugBody(event.currentTarget.value)} required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength={showPrefixChip ? 64 - prefix.length : 64} disabled={busy} aria-describedby="slug-help" placeholder="my-skill" />
            </div>
            <small id="slug-help">Used in your public URL and the SKILL.md name. Renaming changes the install link; existing installs are untouched.</small>
            {#if prefix}<small>Your profile prefix <code>{prefix}</code> is added automatically. <a href="/account/profile">Change it</a>.</small>{/if}
            {#if prefixMisplaced}<p class="notice warn" role="status">This skill predates your <code>{prefix}</code> prefix. <button type="button" class="btn quiet" onclick={applyPrefix} disabled={busy}>Add prefix</button></p>{/if}
            {#if renameNeeded}<p class="notice warn" role="status">Renaming <code>{originalSlug}</code> to <code>{editor.slug}</code> changes its public URL and install command. Your shelf, lists and tags move with it.</p>{/if}
          </div>
          <div class="form-block">
            <h3>Instructions</h3>
            <label for="skill-upload">Upload an existing SKILL.md</label>
            <input id="skill-upload" type="file" accept=".md" onchange={(event) => upload(event, true)} disabled={busy} />
            <label for="hosted-content">SKILL.md</label>
            <textarea id="hosted-content" bind:value={editor.content} rows="16" required spellcheck="false" disabled={busy}></textarea>
            <small>Include YAML frontmatter with a matching name and a description, followed by your instructions.</small>
          </div>
          <div class="form-block">
            <h3>Supporting files</h3>
            <label for="support-upload">Add files</label>
            <input id="support-upload" type="file" multiple onchange={(event) => upload(event)} disabled={busy} />
            <small>Up to 100 files, 2 MiB total including SKILL.md. Edit paths for folders such as scripts/ or references/.</small>
            {#if Object.keys(editor.files).length}
              <ul class="file-list">
                {#each Object.keys(editor.files) as name (name)}
                  <li class="file-row"><input aria-label={`Path for ${name}`} value={name} onchange={(event) => renameFile(name, event.currentTarget.value)} disabled={busy} /><button type="button" class="btn quiet" onclick={() => removeFile(name)} disabled={busy}>Remove</button></li>
                {/each}
              </ul>
            {:else}<p class="hint">No supporting files yet.</p>{/if}
          </div>
          <div class="form-block visibility">
            <h3>Visibility</h3>
            <label class="publish"><input type="checkbox" bind:checked={editor.published} disabled={busy} /> Publish publicly</label>
            <small>Publishing makes the instructions and supporting files available to everyone, and puts the skill on your shelf under To test. Uncheck and save to unpublish.</small>
          </div>
          <div class="actions">
            <button class="btn primary" disabled={busy}>{busy ? 'Saving…' : renameNeeded ? 'Rename and save' : editor.published ? 'Save and publish' : 'Save draft'}</button>
            {#if originalSlug}<button type="button" class="btn quiet" disabled={busy} onclick={() => deleting = skills.find(s => s.slug === originalSlug) ?? null}>Delete skill</button>{/if}
          </div>
        </form>
        {#if originalSlug && skills.find(s => s.slug === originalSlug)?.published}
          <div class="install">
            <h3>Share and install</h3>
            <div class="install-links"><a href={`/${username}/${originalSlug}`} target="_blank" rel="noopener noreferrer">View public skill page ↗</a><a href="/">Open your shelf</a></div>
            <code>{command}</code>
            <CopyButton text={command} label="Copy install command" />
          </div>
        {/if}
      </section>
    {:else}<section class="panel"><h2>A home for your skills</h2><p>Choose a skill to edit, or create a new one. Drafts stay private until you publish them.</p></section>{/if}
  </div>
  {#if editor && !importing}
    <div class="save-dock" role="region" aria-label="Save your skill">
      <span class="save-state" aria-live="polite">{busy ? 'Saving…' : dirty ? 'Unsaved changes' : originalSlug ? 'All changes saved' : 'Not saved yet'}</span>
      <button type="submit" form="skill-form" class="btn primary" disabled={busy}>{busy ? 'Saving…' : renameNeeded ? 'Rename and save' : editor.published ? 'Save and publish' : 'Save draft'}</button>
    </div>
  {/if}
{/if}
{#if deleting}<ConfirmDialog title={`Delete ${deleting.slug}?`} body="This permanently deletes its instructions and supporting files. Existing installations remain on users’ machines; the public page and future installs stop working." confirmLabel="Delete skill" oncancel={() => deleting = null} onconfirm={remove} />{/if}
{#if discardPrompt}<ConfirmDialog title={discardPrompt.title} body={discardPrompt.body} confirmLabel={discardPrompt.label} danger oncancel={() => discardPrompt = null} onconfirm={() => { const prompt = discardPrompt; discardPrompt = null; prompt?.run() }} />{/if}

<style>
  .panel { background: var(--card); border: 1px solid var(--line); border-radius: 14px; padding: 24px; min-width: 0; }
  .panel h2 { margin-top: 0; font-size: 1.1rem; }
  .panel + .panel { margin-top: 16px; }
  .panel pre { margin: 12px 0; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); overflow: auto; font: 0.82rem/1.6 var(--mono); }
  .toolbar, .actions, .file-row { display: flex; align-items: center; gap: 12px; }
  .toolbar { justify-content: space-between; margin-bottom: 16px; }
  .publisher { display: grid; gap: 6px; }
  .publisher p { margin: 0; }
  .stats { display: flex; gap: 8px; flex-wrap: wrap; }
  .stat { padding: 3px 8px; border: 1px solid var(--line); border-radius: 999px; font-size: .75rem; color: var(--muted); }
  .stat code { color: var(--ink); }
  .toolbar-actions, .import-pickers { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .workspace { display: grid; grid-template-columns: minmax(180px, 240px) minmax(0, 1fr); align-items: start; gap: 20px; }
  .workspace.with-dock { padding-bottom: 76px; }
  form { display: flex; flex-direction: column; gap: 10px; }
  label { font-weight: 600; margin-top: 10px; }
  small { color: var(--muted); }
  textarea, input { min-width: 0; width: 100%; }
  textarea, code { font-family: var(--mono); }
  textarea { resize: vertical; }
  form > .btn { align-self: flex-start; margin-top: 12px; }
  .skills-panel { display: flex; flex-direction: column; gap: 10px; position: sticky; top: 16px; }
  .skills-panel h2 { margin: 0; }
  .search { margin: 0; }
  .filters { display: flex; gap: 6px; flex-wrap: wrap; }
  .chip { width: auto; padding: 4px 10px; border: 1px solid var(--line); border-radius: 999px; background: var(--bg); color: var(--muted); font-size: .76rem; cursor: pointer; }
  .chip.on { border-color: var(--accent); color: var(--ink); background: var(--tint); }
  .empty { padding: 8px 0; }
  .empty p { margin: 4px 0 0; color: var(--muted); font-size: .84rem; }
  .skill-list { display: grid; gap: 6px; }
  .skill-choice { width: 100%; text-align: left; padding: 10px; display: flex; flex-direction: column; gap: 4px; overflow-wrap: anywhere; border: 1px solid transparent; border-radius: 10px; background: transparent; }
  .skill-choice:hover:not(:disabled) { background: var(--bg); border-color: var(--line); }
  .skill-choice[aria-current='true'] { background: var(--tint); border-color: color-mix(in srgb, var(--accent) 35%, var(--line)); }
  .skill-choice span { font-size: .8rem; color: var(--muted); }
  .choice-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  .choice-top strong { color: var(--ink); }
  .choice-meta { font-size: .74rem !important; }
  .skill-choice small { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
  .pill { flex: none; padding: 2px 8px; border: 1px solid var(--line); border-radius: 999px; font-size: .68rem !important; font-weight: 600; text-transform: uppercase; letter-spacing: .04em; }
  .pill.published { border-color: color-mix(in srgb, var(--good) 45%, var(--line)); color: var(--good); }
  .form-block { display: grid; gap: 8px; padding: 16px; border: 1px solid var(--line); border-radius: 12px; background: var(--bg); }
  .form-block h3 { margin: 0; font-size: .9rem; }
  .form-block > label:first-of-type { margin-top: 0; }
  .slug-row { display: flex; align-items: stretch; }
  .slug-row input { flex: 1; }
  .slug-prefix { display: flex; align-items: center; padding: 0 10px; border: 1px solid var(--line); border-right: 0; border-radius: 8px 0 0 8px; background: var(--tint); color: var(--muted); font-family: var(--mono); font-size: .85rem; }
  .slug-row .slug-prefix + input { border-radius: 0 8px 8px 0; }
  .file-list { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
  .install-links { display: flex; gap: 14px; flex-wrap: wrap; margin: 6px 0 2px; font-size: .88rem; }
  .notice.warn { border-color: color-mix(in srgb, var(--accent) 45%, var(--line)); background: color-mix(in srgb, var(--accent) 7%, var(--card)); }
  .publish { display: flex; align-items: center; gap: 10px; }
  .publish input { width: auto; }
  .actions { flex-wrap: wrap; margin-top: 12px; }
  .install { border-top: 1px solid var(--line); margin-top: 24px; padding-top: 12px; }
  .install code { display: block; padding: 16px 0; overflow-wrap: anywhere; }
  .import-panel { display: flex; flex-direction: column; gap: 12px; }
  .panel-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .panel-head h2 { margin: 0; }
  .dropzone { display: grid; gap: 6px; width: 100%; padding: 28px 20px; border: 1px dashed var(--line); border-radius: 12px; background: var(--bg); color: var(--ink); text-align: center; cursor: pointer; }
  .dropzone:hover:not(:disabled), .dropzone.active { border-color: var(--accent); background: var(--tint); }
  .dropzone:disabled { opacity: .65; cursor: default; }
  .dropzone strong { font-size: 1rem; }
  .dropzone span { color: var(--muted); font-size: .84rem; }
  .hint { margin: 0; color: var(--muted); font-size: .82rem; }
  .import-list { display: grid; gap: 8px; list-style: none; margin: 4px 0 0; padding: 0; }
  .import-list li { display: grid; gap: 6px; padding: 12px; border: 1px solid var(--line); border-radius: 10px; }
  .import-list li.invalid { border-color: color-mix(in srgb, var(--bad) 45%, var(--line)); background: color-mix(in srgb, var(--bad) 5%, var(--card)); }
  .import-check { display: flex; align-items: flex-start; gap: 10px; margin: 0; cursor: pointer; }
  .import-check input { width: auto; margin-top: 3px; }
  .import-check span { display: grid; gap: 2px; min-width: 0; }
  .import-check small { color: var(--muted); }
  .import-meta { display: flex; gap: 12px; flex-wrap: wrap; padding-left: 26px; color: var(--muted); font-size: .78rem; }
  .import-meta .problem { color: var(--bad); }
  .save-dock { position: fixed; right: 20px; bottom: calc(20px + var(--bottom-chrome, 0px)); z-index: 14; display: flex; align-items: center; gap: 14px; padding: 10px 12px 10px 16px; border: 1px solid var(--line); border-left: 3px solid var(--accent); border-radius: 12px; background: var(--card); box-shadow: 0 18px 50px -20px rgb(0 0 0 / .4); }
  .save-state { font-size: .8rem; color: var(--muted); }
  @media (max-width: 720px) { .workspace { grid-template-columns: 1fr; } .panel { padding: 16px; } }
  @media (max-width: 560px) {
    .toolbar { align-items: flex-start; flex-direction: column; }
    .toolbar-actions { width: 100%; }
    .toolbar-actions .btn { flex: 1; }
    .workspace.with-dock { padding-bottom: 88px; }
    .save-dock { left: 12px; right: 12px; bottom: calc(12px + var(--bottom-chrome, 0px)); justify-content: space-between; }
    .save-dock .btn { flex: 1; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .save-dock { transition: opacity 180ms var(--ease-out), transform 180ms var(--ease-out); }
    @starting-style { .save-dock { opacity: 0; transform: translateY(8px); } }
  }
</style>
