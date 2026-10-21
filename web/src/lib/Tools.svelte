<script lang="ts">
  import { exportAll, getToken, importText, restoreAll } from './api'
  import { bookmarklet } from './bookmarklet'

  let { onchange, ok, fail }: {
    onchange: () => Promise<void>
    ok: (message: string) => void
    fail: (e: unknown) => void
  } = $props()

  let open = $state(false)
  let importBox = $state('')
  const origin = location.origin

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
  async function restore(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0]
    if (!file) return
    try {
      const r = await restoreAll(JSON.parse(await file.text()))
      ok(`Restored ${r.restored} skills`)
      await onchange()
    } catch (err) {
      fail(err)
    }
  }
</script>

<section class="tools">
  <button type="button" class="btn quiet" onclick={() => (open = !open)} aria-expanded={open} aria-controls="tools-panel">
    {open ? 'Hide' : 'Show'} bookmarklets, import and backup
  </button>
  {#if open}
    <div class="panel" id="tools-panel">
      <h2>Bookmarklets</h2>
      <p>Drag these to your bookmarks bar. On any skills.sh skill page, click one to send it here.</p>
      <p class="links">
        <!-- svelte-ignore a11y_invalid_attribute -->
        <a class="pill" href={bookmarklet('like', origin, getToken())}>♡ Like on reet</a>
        <a class="pill" href={bookmarklet('save', origin, getToken())}>☆ To test on reet</a>
        <a class="pill" href={bookmarklet('used', origin, getToken())}>✔ In use on reet</a>
      </p>
      {#if getToken()}<p class="hint">These links contain your access token, so keep them private.</p>{/if}

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
        <label class="btn file">Restore from JSON<input type="file" accept="application/json" onchange={restore} hidden /></label>
      </p>
    </div>
  {/if}
</section>

<style>
  .tools { margin-top: 40px; }
  .panel { margin-top: 8px; padding: 4px 18px 18px; background: var(--card); border: 1px solid var(--line); border-radius: 14px; }
  h2 { font: 600 1.15rem/1.3 var(--serif); margin: 20px 0 4px; }
  p { margin: 4px 0; color: var(--muted); }
  code { font-size: 0.85rem; overflow-wrap: anywhere; }
  .links { display: flex; gap: 8px; flex-wrap: wrap; margin: 8px 0; }
  .pill { padding: 6px 14px; border-radius: 999px; border: 1px dashed var(--accent); color: var(--ink); text-decoration: none; }
  .file { display: inline-flex; align-items: center; }
  .hint { font-size: 0.85rem; }
  .panel textarea { margin: 6px 0; }
</style>
