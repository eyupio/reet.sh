<script lang="ts">
  import {
    addSkill, exportAll, getToken, importText, listSkills, patchSkill, refreshSkill,
    removeSkill, resolveSkill, restoreAll,
    type Counts, type Skill, type Sort, type Tab,
  } from './lib/api'
  import { bookmarklet } from './lib/bookmarklet'

  let skills = $state<Skill[]>([])
  let counts = $state<Counts>({ all: 0, liked: 0, bookmarked: 0, used: 0 })
  let tab = $state<Tab>('all')
  let q = $state('')
  let sort = $state<Sort>('recent')
  let addInput = $state('')
  let message = $state('')
  let error = $state('')
  let loading = $state(true)
  let editing = $state<string | null>(null)
  let draft = $state('')
  let linking = $state<string | null>(null)
  let linkInput = $state('')
  let showTools = $state(false)
  let importBox = $state('')

  const tabs: { id: Tab; label: string }[] = [
    { id: 'all', label: 'All' },
    { id: 'liked', label: 'Liked' },
    { id: 'bookmarked', label: 'Bookmarked' },
    { id: 'used', label: 'In use' },
  ]

  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
    message = ''
  }
  function ok(m: string) {
    message = m
    error = ''
  }

  async function load() {
    try {
      const r = await listSkills(tab, q, sort)
      skills = r.skills
      counts = r.counts
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      loading = false
    }
  }

  $effect(() => {
    void [tab, q, sort]
    const t = setTimeout(load, 150)
    return () => clearTimeout(t)
  })

  async function add(e: SubmitEvent) {
    e.preventDefault()
    if (!addInput.trim()) return
    try {
      const s = await addSkill(addInput.trim())
      addInput = ''
      ok(`Added ${s.name}`)
      await load()
      setTimeout(load, 2500) // pick up fetched description
    } catch (e) {
      fail(e)
    }
  }

  async function toggle(s: Skill, key: 'liked' | 'bookmarked' | 'used') {
    try {
      await patchSkill(s.id, { [key]: !s[key] })
      await load()
    } catch (e) {
      fail(e)
    }
  }

  function startNotes(s: Skill) {
    editing = s.id
    draft = s.notes
  }
  async function saveNotes(s: Skill) {
    try {
      await patchSkill(s.id, { notes: draft })
      editing = null
      await load()
    } catch (e) {
      fail(e)
    }
  }
  async function refresh(s: Skill) {
    try {
      await refreshSkill(s.id)
      ok(`Refreshed ${s.name}`)
      await load()
    } catch (e) {
      fail(e)
    }
  }
  async function remove(s: Skill) {
    if (!confirm(`Remove ${s.name} from your list?`)) return
    try {
      await removeSkill(s.id)
      await load()
    } catch (e) {
      fail(e)
    }
  }
  async function link(s: Skill) {
    try {
      const r = await resolveSkill(s.id, linkInput.trim())
      linking = null
      linkInput = ''
      ok(`Linked to ${r.id}`)
      await load()
    } catch (e) {
      fail(e)
    }
  }
  async function runImport() {
    try {
      const r = await importText(importBox)
      importBox = ''
      ok(`Imported ${r.imported} as in use` + (r.errors.length ? `, skipped ${r.errors.length}: ${r.errors.join('; ')}` : ''))
      await load()
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
      await load()
    } catch (err) {
      fail(err)
    }
  }

  const fmt = (n: number) => (n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1e3 ? (n / 1e3).toFixed(1) + 'K' : String(n))
  const origin = location.origin
</script>

<main>
  <header>
    <h1>reet<span>.sh</span></h1>
    <p class="tag">Your shelf for the skills on skills.sh: like them, save them, track what you use.</p>
  </header>

  <form class="add" onsubmit={add}>
    <input
      placeholder="Paste a skills.sh URL or owner/repo/skill"
      bind:value={addInput}
      aria-label="Skill URL or slug"
    />
    <button class="primary" type="submit">Add</button>
  </form>

  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  {#if message}<p class="notice good" role="status">{message}</p>{/if}

  <nav class="bar">
    <div class="tabs" role="tablist">
      {#each tabs as t}
        <button role="tab" aria-selected={tab === t.id} class:on={tab === t.id} onclick={() => (tab = t.id)}>
          {t.label} <small>{counts[t.id]}</small>
        </button>
      {/each}
    </div>
    <div class="filters">
      <input type="search" placeholder="Search" bind:value={q} aria-label="Search skills" />
      <select bind:value={sort} aria-label="Sort">
        <option value="recent">Recent</option>
        <option value="name">Name</option>
        <option value="installs">Installs</option>
      </select>
    </div>
  </nav>

  {#if loading}
    <p class="empty">Loading…</p>
  {:else if skills.length === 0}
    <p class="empty">
      {counts.all === 0 ? 'Nothing here yet. Paste a skills.sh link above, or import what you have installed.' : 'No skills match.'}
    </p>
  {:else}
    <ul class="list">
      {#each skills as s (s.id)}
        <li class="card">
          <div class="head">
            <div class="title">
              {#if s.url}
                <a href={s.url} target="_blank" rel="noopener noreferrer">{s.name}</a>
              {:else}
                <span>{s.name}</span>
              {/if}
              <code>{s.sourceType === 'local' ? 'unlinked' : s.source}</code>
            </div>
            {#if s.installs > 0}<span class="installs">{fmt(s.installs)} installs</span>{/if}
          </div>
          {#if s.description}<p class="desc">{s.description}</p>{/if}

          <div class="actions">
            <button class="toggle" class:on={s.liked} aria-pressed={s.liked} onclick={() => toggle(s, 'liked')}>
              {s.liked ? '♥' : '♡'} Like
            </button>
            <button class="toggle" class:on={s.bookmarked} aria-pressed={s.bookmarked} onclick={() => toggle(s, 'bookmarked')}>
              {s.bookmarked ? '★' : '☆'} Save
            </button>
            <button class="toggle" class:on={s.used} aria-pressed={s.used} onclick={() => toggle(s, 'used')}>
              {s.used ? '✔' : '○'} In use
            </button>
            <span class="spacer"></span>
            <button class="ghost" onclick={() => (editing === s.id ? (editing = null) : startNotes(s))}>Notes</button>
            {#if s.sourceType === 'local'}
              <button class="ghost" onclick={() => (linking = linking === s.id ? null : s.id)}>Link</button>
            {:else}
              <button class="ghost" onclick={() => refresh(s)}>Refresh</button>
            {/if}
            <button class="ghost danger" onclick={() => remove(s)}>Remove</button>
          </div>

          {#if editing === s.id}
            <div class="inline">
              <textarea rows="3" bind:value={draft} placeholder="Why you like it, how you use it…"></textarea>
              <button class="primary" onclick={() => saveNotes(s)}>Save notes</button>
            </div>
          {:else if s.notes}
            <p class="notes">{s.notes}</p>
          {/if}

          {#if linking === s.id}
            <form class="inline" onsubmit={(e) => { e.preventDefault(); link(s) }}>
              <input bind:value={linkInput} placeholder="skills.sh URL for {s.name}" aria-label="skills.sh URL" />
              <button class="primary" type="submit">Link</button>
            </form>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  <section class="tools">
    <button class="ghost" onclick={() => (showTools = !showTools)} aria-expanded={showTools}>
      {showTools ? 'Hide' : 'Show'} bookmarklets, import and backup
    </button>
    {#if showTools}
      <div class="panel">
        <h2>Bookmarklets</h2>
        <p>Drag these to your bookmarks bar. On any skills.sh skill page, click one to send it here.</p>
        <p class="links">
          <!-- svelte-ignore a11y_invalid_attribute -->
          <a class="pill" href={bookmarklet('like', origin, getToken())}>♡ Like on reet</a>
          <a class="pill" href={bookmarklet('save', origin, getToken())}>☆ Save on reet</a>
          <a class="pill" href={bookmarklet('used', origin, getToken())}>✔ In use on reet</a>
        </p>
        {#if getToken()}<p class="hint">These links contain your access token, so keep them private.</p>{/if}

        <h2>Import what you have installed</h2>
        <p>
          Run <code>reet scan</code> on the machine with your skills, or paste names, <code>owner/repo/skill</code> slugs or
          skills.sh URLs here (one per line). They are marked as in use; existing likes and notes are kept.
        </p>
        <textarea rows="4" bind:value={importBox} placeholder="find-skills&#10;anthropics/skills/pptx"></textarea>
        <button class="primary" onclick={runImport} disabled={!importBox.trim()}>Import as in use</button>

        <h2>Backup</h2>
        <p class="links">
          <button class="ghost" onclick={backup}>Download JSON</button>
          <label class="ghost file">Restore from JSON<input type="file" accept="application/json" onchange={restore} hidden /></label>
        </p>
      </div>
    {/if}
  </section>
</main>

<style>
  :global(:root) {
    --bg: #f7f6f2; --card: #fff; --ink: #1d1c1a; --muted: #6b6860; --line: #e4e1d8;
    --accent: #c2410c; --accent-ink: #fff; --good: #15803d; --bad: #b91c1c; --on: #fde8d8;
    color-scheme: light dark;
  }
  @media (prefers-color-scheme: dark) {
    :global(:root) {
      --bg: #151413; --card: #1f1e1c; --ink: #efece6; --muted: #a09b90; --line: #33312d;
      --accent: #fb923c; --accent-ink: #1a1208; --good: #4ade80; --bad: #f87171; --on: #3a2615;
    }
  }
  :global(body) { margin: 0; background: var(--bg); color: var(--ink); font: 16px/1.5 system-ui, sans-serif; }
  main { max-width: 820px; margin: 0 auto; padding: 32px 16px 64px; }
  h1 { margin: 0; font-size: 2rem; letter-spacing: -0.02em; }
  h1 span { color: var(--accent); }
  h2 { font-size: 1rem; margin: 20px 0 4px; }
  .tag { margin: 4px 0 20px; color: var(--muted); }
  input, select, textarea, button { font: inherit; color: inherit; }
  input, select, textarea {
    background: var(--card); border: 1px solid var(--line); border-radius: 8px; padding: 9px 12px; min-width: 0;
  }
  textarea { width: 100%; box-sizing: border-box; resize: vertical; }
  :global(:focus-visible) { outline: 2px solid var(--accent); outline-offset: 2px; }
  button { cursor: pointer; border-radius: 8px; border: 1px solid var(--line); background: var(--card); padding: 7px 12px; }
  button:disabled { opacity: 0.5; cursor: default; }
  .primary { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); font-weight: 600; }
  .ghost { background: transparent; border-color: transparent; color: var(--muted); }
  .ghost:hover { color: var(--ink); }
  .danger:hover { color: var(--bad); }
  .add, .inline { display: flex; gap: 8px; }
  .add input, .inline input, .inline textarea { flex: 1; }
  .inline { margin-top: 10px; align-items: flex-start; }
  .notice { margin: 12px 0 0; padding: 8px 12px; border-radius: 8px; border: 1px solid var(--line); background: var(--card); }
  .good { border-color: var(--good); } .bad { border-color: var(--bad); }
  .bar { display: flex; flex-wrap: wrap; gap: 12px; justify-content: space-between; margin: 24px 0 12px; }
  .tabs { display: flex; gap: 4px; flex-wrap: wrap; }
  .tabs button { border-color: transparent; background: transparent; color: var(--muted); }
  .tabs button.on { background: var(--on); color: var(--ink); font-weight: 600; }
  .tabs small { opacity: 0.7; }
  .filters { display: flex; gap: 8px; }
  .list { list-style: none; margin: 0; padding: 0; display: grid; gap: 10px; }
  .card { background: var(--card); border: 1px solid var(--line); border-radius: 12px; padding: 14px 16px; }
  .head { display: flex; justify-content: space-between; gap: 12px; align-items: baseline; }
  .title { display: flex; gap: 10px; align-items: baseline; flex-wrap: wrap; min-width: 0; }
  .title a, .title span { font-weight: 650; color: var(--ink); text-decoration: none; overflow-wrap: anywhere; }
  .title a:hover { color: var(--accent); }
  code { font-size: 0.8rem; color: var(--muted); overflow-wrap: anywhere; }
  .installs { color: var(--muted); font-size: 0.85rem; white-space: nowrap; }
  .desc { margin: 6px 0 0; color: var(--muted); }
  .notes { margin: 10px 0 0; padding: 8px 12px; border-left: 3px solid var(--accent); background: var(--bg); border-radius: 4px; white-space: pre-wrap; }
  .actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 12px; align-items: center; }
  .spacer { flex: 1; }
  .toggle.on { background: var(--on); border-color: var(--accent); font-weight: 600; }
  .empty { text-align: center; color: var(--muted); padding: 40px 0; }
  .tools { margin-top: 32px; }
  .panel { margin-top: 8px; padding: 4px 16px 16px; background: var(--card); border: 1px solid var(--line); border-radius: 12px; }
  .panel p { margin: 4px 0; color: var(--muted); }
  .links { display: flex; gap: 8px; flex-wrap: wrap; margin: 8px 0; }
  .pill { padding: 6px 12px; border-radius: 999px; border: 1px dashed var(--accent); color: var(--ink); text-decoration: none; }
  .file { display: inline-block; border: 1px solid var(--line); cursor: pointer; padding: 7px 12px; border-radius: 8px; }
  .hint { font-size: 0.85rem; }
  @media (max-width: 560px) { .add, .inline { flex-direction: column; } .filters { width: 100%; } .filters input { flex: 1; } }
</style>
