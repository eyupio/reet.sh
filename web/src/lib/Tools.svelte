<script lang="ts">
  import { onMount } from 'svelte'
  import { exportAll, importText, listIgnored, listSkills, restoreAll, unignore, type Backup, type Ignored, type SkillQuery } from './api'
  import Bookmarklets from './Bookmarklets.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import ManageTags from './ManageTags.svelte'

  let { onchange, ok, fail, token, filters, canManageTags = true, startOpen = false, hideBookmarklets = false }: {
    onchange: () => Promise<void>
    ok: (message: string) => void
    fail: (e: unknown) => void
    token: string
    filters: SkillQuery
    canManageTags?: boolean
    startOpen?: boolean
    hideBookmarklets?: boolean
  } = $props()

  let open = $state(false)
  let importBox = $state('')
  let pending = $state<{ data: Backup; overwrite: number } | null>(null)
  let includeHistory = $state(true)
  let format = $state<'json' | 'md' | 'csv'>('json')
  let ignored = $state<Ignored[]>([])
  let showTags = $state(false)
  let restoreInput = $state<HTMLInputElement>()
  const title = $derived(hideBookmarklets ? 'tools' : 'bookmarklets and tools')

  onMount(() => {
    open = startOpen
    void loadIgnored()
  })

  async function runImport() {
    try {
      const r = await importText(importBox)
      importBox = ''
      const ignored = r.ignored ? `, skipped ${r.ignored} ignored` : ''
      ok(`Imported ${r.imported} as in use${ignored}` + (r.errors.length ? `, skipped ${r.errors.length}: ${r.errors.join('; ')}` : ''))
      await onchange()
    } catch (e) {
      fail(e)
    }
  }
  async function loadIgnored() {
    try {
      const r = await listIgnored()
      ignored = r.ignored
    } catch (e) {
      fail(e)
    }
  }

  async function allowAgain(id: string) {
    try {
      await unignore(id)
      await loadIgnored()
      await onchange()
    } catch (e) {
      fail(e)
    }
  }

  async function backup() {
    try {
      const blob = await exportAll(includeHistory, format, filters)
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = format === 'json' ? 'reet-export.json' : `reet-shelf.${format}`
      a.click()
      URL.revokeObjectURL(a.href)
      await onchange()
    } catch (e) {
      fail(e)
    }
  }
  async function pickRestore(e: Event) {
    const input = e.currentTarget as HTMLInputElement
    const file = input.files?.[0]
    input.value = ''
    if (!file) return
    try {
      const data = JSON.parse(await file.text()) as Backup
      if (!Array.isArray(data.skills)) throw new Error('That file is not a reet backup.')
      const current = await listSkills({ tab: 'all', q: '', sort: 'recent' })
      const ids = new Set(current.skills.map((s) => s.id))
      const overwrite = data.skills.filter((s) => ids.has(s.id)).length
      pending = { data, overwrite }
    } catch (err) {
      fail(err)
    }
  }

  async function runRestore() {
    const p = pending
    if (!p) return
    pending = null
    try {
      const r = await restoreAll(p.data)
      ok(`Restored ${r.restored} ${r.restored === 1 ? 'skill' : 'skills'}`)
      await onchange()
    } catch (err) {
      fail(err)
    }
  }
</script>

<section class="tools">
  <button type="button" class="btn quiet" onclick={() => { open = !open; if (open) void loadIgnored() }} aria-expanded={open} aria-controls="tools-panel">
    {open ? 'Hide' : 'Show'} {title}
  </button>
  {#if open}
    <div class="panel" id="tools-panel">
      {#if !hideBookmarklets}
        <h2>Bookmarklets</h2>
        <p>Drag these to your bookmarks bar. On any skills.sh skill page, click one to send it here.</p>
        <Bookmarklets {token} />
      {/if}

      <h2>Import what you have installed</h2>
      <p>
        Run <code>reet scan</code> on the machine with your skills, or paste names, <code>owner/repo/skill</code> slugs or
        skills.sh URLs here (one per line). They are marked as in use; existing likes and notes are kept.
      </p>
      <label class="vh" for="import-box">Skills to import</label>
      <textarea id="import-box" rows="4" bind:value={importBox} placeholder="find-skills&#10;anthropics/skills/pptx"></textarea>
      <button type="button" class="btn primary" onclick={runImport} disabled={!importBox.trim()}>Import as in use</button>

      <h2>Backup and export</h2>
      <div class="export-row">
        <label for="export-format">Format</label>
        <select id="export-format" bind:value={format}>
          <option value="json">JSON (full backup)</option>
          <option value="md">Markdown (current filter)</option>
          <option value="csv">CSV (current filter)</option>
        </select>
        <button type="button" class="btn" onclick={backup}>Download</button>
      </div>
      {#if format === 'json'}
        <label class="check">
          <input type="checkbox" bind:checked={includeHistory} />
          Include local history in the backup
        </label>
      {:else}
        <p class="export-hint">Markdown and CSV follow the current search, stage, tag and environment filters.</p>
      {/if}
      <p class="links">
        <button type="button" class="btn" onclick={() => restoreInput?.click()}>Restore from JSON</button>
        <input bind:this={restoreInput} type="file" accept="application/json" aria-label="Choose a JSON backup" onchange={pickRestore} hidden />
      </p>

      {#if canManageTags}
        <h2>Tags</h2>
        <p>Rename or merge tags across the whole shelf.</p>
        <button type="button" class="btn" onclick={() => (showTags = true)}>Manage tags</button>
      {/if}

      <h2>Ignored skills</h2>
      {#if ignored.length === 0}
        <p>Nothing ignored. Removing an installed skill can ignore it in future scans.</p>
      {:else}
        <ul class="ignored">
          {#each ignored as item}
            <li>
              <span>{item.name || item.id}</span>
              <button type="button" class="btn quiet" onclick={() => allowAgain(item.id)}>Allow again</button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  {#if showTags}
    <ManageTags onclose={() => (showTags = false)} onchanged={onchange} onerror={fail} />
  {/if}

  {#if pending}
    <ConfirmDialog
      title="Restore this backup?"
      body={`Restore ${pending.data.skills.length} ${pending.data.skills.length === 1 ? 'skill' : 'skills'}? ${
        pending.overwrite === 0
          ? 'Nothing on the shelf will be overwritten.'
          : `${pending.overwrite} ${pending.overwrite === 1 ? 'skill' : 'skills'} already on the shelf will be overwritten.`
      } The backup's flags and notes replace what is stored here.`}
      confirmLabel="Restore"
      danger
      onconfirm={runRestore}
      oncancel={() => (pending = null)}
    />
  {/if}
</section>

<style>
  .tools { margin-top: 40px; }
  .panel { margin-top: 8px; padding: 4px 18px 18px; background: var(--card); border: 1px solid var(--line); border-radius: 14px; }
  h2 { font: 600 1.15rem/1.3 var(--serif); margin: 20px 0 4px; }
  p { margin: 4px 0; color: var(--muted); }
  code { font-size: 0.85rem; overflow-wrap: anywhere; }
  .links { display: flex; gap: 8px; flex-wrap: wrap; margin: 8px 0; }
  .panel textarea { margin: 6px 0; }
  .check { display: flex; align-items: center; gap: 8px; margin-top: 8px; color: var(--muted); font-size: 0.9rem; }
  .export-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: 8px 0; }
  .export-row label { color: var(--muted); font-size: 0.9rem; }
  .export-row select { max-width: 230px; }
  .export-hint { margin: 6px 0 0; font-size: 0.85rem; }
  .check input { min-height: 0; }
  .ignored { list-style: none; margin: 8px 0 0; padding: 0; }
  .ignored li { display: flex; align-items: center; justify-content: space-between; gap: 10px; border-bottom: 1px solid var(--line); min-height: 44px; }
  .ignored li:last-child { border-bottom: 0; }
  .ignored span { min-width: 0; overflow-wrap: anywhere; }
</style>
