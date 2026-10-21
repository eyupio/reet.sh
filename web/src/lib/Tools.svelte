<script lang="ts">
  import { onMount } from 'svelte'
  import { exportAll, importText, listSkills, restoreAll, type Skill } from './api'
  import Bookmarklets from './Bookmarklets.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let { onchange, ok, fail, token, startOpen = false, hideBookmarklets = false }: {
    onchange: () => Promise<void>
    ok: (message: string) => void
    fail: (e: unknown) => void
    token: string
    startOpen?: boolean
    hideBookmarklets?: boolean
  } = $props()

  let open = $state(false)
  let importBox = $state('')
  let pending = $state<{ skills: Skill[]; overwrite: number } | null>(null)
  const title = $derived(hideBookmarklets ? 'import and backup' : 'bookmarklets, import and backup')

  onMount(() => {
    open = startOpen
  })

  async function runImport() {
    try {
      const r = await importText(importBox)
      importBox = ''
      ok(`Imported ${r.imported} as in use` + (r.errors.length ? `, skipped ${r.errors.length}: ${r.errors.join('; ')}` : ''))
      await onchange()
    } catch (e) {
      fail(e)
    }
  }
  async function backup() {
    try {
      const blob = await exportAll()
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = 'reet-export.json'
      a.click()
      URL.revokeObjectURL(a.href)
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
      const data = JSON.parse(await file.text()) as { skills?: Skill[] }
      if (!Array.isArray(data.skills)) throw new Error('That file is not a reet backup.')
      const current = await listSkills('all', '', 'recent')
      const ids = new Set(current.skills.map((s) => s.id))
      const overwrite = data.skills.filter((s) => ids.has(s.id)).length
      pending = { skills: data.skills, overwrite }
    } catch (err) {
      fail(err)
    }
  }

  async function runRestore() {
    const p = pending
    if (!p) return
    pending = null
    try {
      const r = await restoreAll({ version: 1, exportedAt: Date.now(), skills: p.skills })
      ok(`Restored ${r.restored} ${r.restored === 1 ? 'skill' : 'skills'}`)
      await onchange()
    } catch (err) {
      fail(err)
    }
  }
</script>

<section class="tools">
  <button type="button" class="btn quiet" onclick={() => (open = !open)} aria-expanded={open} aria-controls="tools-panel">
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

      <h2>Backup</h2>
      <p class="links">
        <button type="button" class="btn" onclick={backup}>Download JSON</button>
        <label class="btn file">Restore from JSON<input type="file" accept="application/json" onchange={pickRestore} hidden /></label>
      </p>
    </div>
  {/if}

  {#if pending}
    <ConfirmDialog
      title="Restore this backup?"
      body={`Restore ${pending.skills.length} ${pending.skills.length === 1 ? 'skill' : 'skills'}? ${
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
  .file { display: inline-flex; align-items: center; }
  .panel textarea { margin: 6px 0; }
</style>
