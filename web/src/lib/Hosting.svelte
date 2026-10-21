<script lang="ts">
  import { onMount } from 'svelte'
  import { getHosting, claimPublishingUsername, saveHostedSkill, deleteHostedSkill, type HostedSkill } from './api'
  import PageHeader from './PageHeader.svelte'
  import CopyButton from './CopyButton.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'

  let { canWrite = true }: { canWrite?: boolean } = $props()
  let username = $state('')
  let wantedName = $state('')
  let skills = $state<HostedSkill[]>([])
  let editor = $state<HostedSkill | null>(null)
  let originalSlug = $state('')
  let loading = $state(true)
  let busy = $state(false)
  let error = $state('')
  let notice = $state('')
  let deleting = $state<HostedSkill | null>(null)
  const command = $derived(`npx skills add ${location.origin}/${username}/${originalSlug}`)
  onMount(() => { void load() })
  function fail(e: unknown) { error = e instanceof Error ? e.message : String(e) }
  async function load() {
    try { const data = await getHosting(); username = data.username; skills = data.skills }
    catch (e) { fail(e) } finally { loading = false }
  }
  async function claim(event: SubmitEvent) {
    event.preventDefault(); busy = true; error = ''; notice = ''
    try { const data = await claimPublishingUsername(wantedName); username = data.username; skills = data.skills; notice = 'Username claimed. Create your first skill.' }
    catch (e) { fail(e) } finally { busy = false }
  }
  function newSkill() {
    originalSlug = ''; error = ''; notice = ''
    editor = { slug: 'my-skill', description: '', content: '---\nname: my-skill\ndescription: Describe what this skill does and when to use it.\n---\n\n# My skill\n\nWrite the instructions your agent should follow here.\n', files: {}, published: false, createdAt: 0, updatedAt: 0 }
  }
  function edit(skill: HostedSkill) { editor = { ...skill, files: { ...skill.files } }; originalSlug = skill.slug; error = ''; notice = '' }
  function rename(event: Event) {
    if (!editor) return
    const name = (event.target as HTMLInputElement).value
    editor.content = editor.content.replace(/^(name:\s*).*$/m, (_, prefix) => `${prefix}${name}`)
    editor.slug = name
  }
  async function save(event: SubmitEvent) {
    event.preventDefault(); if (!editor) return
    if (!originalSlug && skills.some(s => s.slug === editor?.slug)) { error = 'You already have a skill with this name. Select it from your skills to edit it.'; return }
    busy = true; error = ''; notice = ''
    try {
      const saved = await saveHostedSkill(editor)
      editor = saved; originalSlug = saved.slug
      skills = [...skills.filter(s => s.slug !== saved.slug), saved].sort((a, b) => a.slug.localeCompare(b.slug))
      notice = saved.published ? 'Skill published. Anyone with the link can view and install it.' : 'Draft saved. Its page and install files are private.'
    } catch (e) { fail(e) } finally { busy = false }
  }
  async function upload(event: Event, main = false) {
    const input = event.target as HTMLInputElement
    if (!editor || !input.files) return
    error = ''; busy = true
    try {
      for (const file of Array.from(input.files)) {
        if (file.size > 2 * 1024 * 1024) throw new Error('A skill bundle must be 2 MiB or smaller.')
        if (main) {
          editor.content = await file.text()
          if (!originalSlug) {
            const name = editor.content.match(/^name:\s*["']?([a-z0-9-]+)["']?\s*$/m)?.[1]
            if (name) editor.slug = name
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
  async function remove() {
    if (!deleting) return
    const slug = deleting.slug; deleting = null; busy = true; error = ''
    try { await deleteHostedSkill(slug); skills = skills.filter(s => s.slug !== slug); if (editor?.slug === slug) editor = null; notice = 'Skill deleted. Its public page and install files are no longer available.' }
    catch (e) { fail(e) } finally { busy = false }
  }
</script>

<PageHeader eyebrow="Publish on reet.sh" title="Host your own skills." lede="Write a skill, give it a home, and share one install command." />
{#if error}<p class="notice bad" role="alert">{error}</p>{/if}
{#if notice}<p class="notice good" role="status">{notice}</p>{/if}
{#if loading}<p role="status">Loading your skills…</p>
{:else if !canWrite}<p class="notice">Your account is read-only. Ask an admin for publishing access.</p>
{:else if !username}
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
  <div class="toolbar"><p>Publishing as <strong>{username}</strong></p><button class="btn primary" onclick={newSkill} disabled={busy}>New skill</button></div>
  <div class="workspace">
    <aside class="panel" aria-label="Your hosted skills">
      <h2>Your skills</h2>
      {#if !skills.length}<p>No skills yet. Create a skill and save it as a draft or publish it.</p>{/if}
      {#each skills as skill (skill.slug)}<button class="skill-choice" aria-current={originalSlug === skill.slug ? 'true' : undefined} onclick={() => edit(skill)} disabled={busy}><strong>{skill.slug}</strong><span>{skill.published ? 'Published' : 'Draft'}</span></button>{/each}
    </aside>
    {#if editor}
      <section class="panel editor">
        <h2>{originalSlug ? 'Edit skill' : 'New skill'}</h2>
        <form onsubmit={save}>
          <label for="hosted-slug">Skill name</label>
          <input id="hosted-slug" value={editor.slug} oninput={rename} required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="64" disabled={busy || !!originalSlug} aria-describedby="slug-help" />
          <small id="slug-help">Used in your URL and the SKILL.md name. Saved skill names are permanent.</small>
          <label for="skill-upload">Upload an existing SKILL.md</label><input id="skill-upload" type="file" accept=".md" onchange={(event) => upload(event, true)} disabled={busy} />
          <label for="hosted-content">SKILL.md</label><textarea id="hosted-content" bind:value={editor.content} rows="18" required spellcheck="false" disabled={busy}></textarea>
          <small>Include YAML frontmatter with a matching name and a description, followed by your instructions.</small>
          <label for="support-upload">Supporting files (optional)</label><input id="support-upload" type="file" multiple onchange={(event) => upload(event)} disabled={busy} />
          <small>Up to 100 files, 2 MiB total including SKILL.md. Edit paths below for folders such as scripts/ or references/.</small>
          {#each Object.keys(editor.files) as name (name)}<div class="file-row"><input aria-label={`Path for ${name}`} value={name} onchange={(event) => renameFile(name, event.currentTarget.value)} disabled={busy} /><button type="button" class="btn quiet" onclick={() => removeFile(name)} disabled={busy}>Remove</button></div>{/each}
          <label class="publish"><input type="checkbox" bind:checked={editor.published} disabled={busy} /> Publish publicly</label>
          <small>Publishing makes the instructions and all supporting files available to everyone. Uncheck and save to unpublish.</small>
          <div class="actions"><button class="btn primary" disabled={busy}>{busy ? 'Saving…' : editor.published ? 'Save and publish' : 'Save draft'}</button>{#if originalSlug}<button type="button" class="btn quiet" disabled={busy} onclick={() => deleting = skills.find(s => s.slug === originalSlug) ?? null}>Delete skill</button>{/if}</div>
        </form>
        {#if originalSlug && skills.find(s => s.slug === originalSlug)?.published}<div class="install"><h3>Share and install</h3><a href={`/${username}/${originalSlug}`} target="_blank" rel="noopener noreferrer">View public skill page ↗</a><code>{command}</code><CopyButton text={command} label="Copy install command" /></div>{/if}
      </section>
    {:else}<section class="panel"><h2>A home for your skills</h2><p>Choose a skill to edit, or create a new one. Drafts stay private until you publish them.</p></section>{/if}
  </div>
{/if}
{#if deleting}<ConfirmDialog title={`Delete ${deleting.slug}?`} body="This permanently deletes its instructions and supporting files. Existing installations remain on users’ machines; the public page and future installs stop working." confirmLabel="Delete skill" oncancel={() => deleting = null} onconfirm={remove} />{/if}

<style>
  .panel { background: var(--card); border: 1px solid var(--line); border-radius: 14px; padding: 24px; min-width: 0; }
  .panel h2 { margin-top: 0; font-size: 1.1rem; }
  .toolbar, .actions, .file-row { display: flex; align-items: center; gap: 12px; }
  .toolbar { justify-content: space-between; margin-bottom: 16px; }
  .workspace { display: grid; grid-template-columns: minmax(180px, 240px) minmax(0, 1fr); align-items: start; gap: 20px; }
  form { display: flex; flex-direction: column; gap: 10px; }
  label { font-weight: 600; margin-top: 10px; }
  small { color: var(--muted); }
  textarea, input { min-width: 0; width: 100%; }
  textarea, code { font-family: var(--mono); }
  textarea { resize: vertical; }
  form > .btn { align-self: flex-start; margin-top: 12px; }
  .skill-choice { width: 100%; text-align: left; padding: 12px; display: flex; flex-direction: column; gap: 4px; margin-top: 8px; overflow-wrap: anywhere; }
  .skill-choice[aria-current='true'] { background: var(--tint); }
  .skill-choice span { font-size: .8rem; color: var(--muted); }
  .publish { display: flex; align-items: center; gap: 10px; }
  .publish input { width: auto; }
  .actions { flex-wrap: wrap; margin-top: 12px; }
  .install { border-top: 1px solid var(--line); margin-top: 24px; padding-top: 12px; }
  .install code { display: block; padding: 16px 0; overflow-wrap: anywhere; }
  @media (max-width: 720px) { .workspace { grid-template-columns: 1fr; } .panel { padding: 16px; } }
</style>
