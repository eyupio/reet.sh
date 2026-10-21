<script lang="ts">
  import { onMount } from 'svelte'
  import AudiencePicker from './AudiencePicker.svelte'
  import { audienceLabel, type Audience } from './api'
  import PageHeader from './PageHeader.svelte'
  import {
    addListMember, addListSkill, addListToShelf, deleteList, getList, listSkills,
    removeListMember, removeListSkill, transferList, updateList, updateListMember, updateListSkill,
    type ListDetail, type ListMember, type ListSkill, type Skill,
  } from './api'
  import ConfirmDialog from './ConfirmDialog.svelte'
  import CopyButton from './CopyButton.svelte'
  import { copyText } from './clipboard'
  import { installCommand } from './install'
  import { fmt } from './shelf'

  let {
    id, canWrite, userRole, onerror,
  }: { id: number; canWrite: boolean; userRole: string; onerror?: (e: unknown) => void } = $props()

  let detail = $state<ListDetail | null>(null)
  let loading = $state(true)
  let error = $state('')
  let status = $state('')
  let busy = $state(false)
  let shelf = $state<Skill[]>([])
  let addID = $state('')
  let addInput = $state('')
  let addNote = $state('')
  let noteDrafts = $state<Record<string, string>>({})
  let editingNote = $state('')
  let removed = $state<{ skill: ListSkill; note: string } | null>(null)
  let settingsOpen = $state(false)
  let peopleOpen = $state(false)
  let form = $state<{ name: string; description: string; visibility: string }>({ name: '', description: '', visibility: 'members' })
  let audience = $state<Audience>({ people: [], excluded: [], groupRole: 'viewer' })
  let memberEmail = $state('')
  let memberRole = $state('viewer')
  let transferTo = $state(0)
  let confirmDelete = $state(false)
  let confirmTransfer = $state<ListMember | null>(null)
  let reviewingPublication = $state(false)

  const link = $derived(detail?.slug ? `${location.origin}/l/${detail.slug}` : '')
  const available = $derived(shelf.filter((s) => !(detail?.skills ?? []).some((x) => x.id === s.id)))
  const commands = $derived((detail?.skills ?? []).map(installCommand).filter((x): x is string => Boolean(x)))

  onMount(() => {
    document.title = 'List · reet.sh'
    void load()
    void loadShelf()
    return () => {
      document.title = 'reet.sh – your shelf for AI skills'
    }
  })

  function fail(e: unknown) {
    error = e instanceof Error ? e.message : String(e)
    onerror?.(e)
  }

  function apply(d: ListDetail) {
    detail = d
    document.title = `${d.name} · reet.sh`
    resetSettings()
    noteDrafts = Object.fromEntries(d.skills.map((s) => [s.id, s.note ?? '']))
    if (!d.members?.some((m) => m.id === transferTo)) transferTo = d.members?.find((m) => m.role !== 'owner')?.id ?? 0
  }

  function resetSettings() {
    if (!detail) return
    form = { name: detail.name, description: detail.description, visibility: detail.visibility }
    // Keep the draft independent of the saved audience, including nested collaborators.
    audience = structuredClone($state.snapshot(detail.audience ?? { people: [], excluded: [], groupRole: 'viewer' }))
    reviewingPublication = false
  }

  function toggleSettings() {
    resetSettings()
    settingsOpen = !settingsOpen
    peopleOpen = false
  }

  function cancelSettings() {
    resetSettings()
    settingsOpen = false
  }

  async function load() {
    try {
      apply(await getList(id))
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      loading = false
    }
  }

  async function loadShelf() {
    try {
      shelf = (await listSkills({ tab: 'all', q: '', sort: 'name' })).skills.filter((s) => s.sourceType !== 'local')
    } catch {
      /* the paste field still works when the shelf cannot be read */
    }
  }

  function say(message: string) {
    status = message
    error = ''
    removed = null
  }

  async function add(e: SubmitEvent) {
    e.preventDefault()
    const input = addInput.trim()
    const chosen = addID
    if (!input && !chosen) return
    busy = true
    try {
      apply(await addListSkill(id, input ? { input, note: addNote } : { id: chosen, note: addNote }))
      addInput = ''
      addID = ''
      addNote = ''
      say(input ? 'Added from the reference.' : 'Added to the list.')
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function removeEntry(skill: ListSkill) {
    busy = true
    try {
      apply(await removeListSkill(id, skill.id))
      removed = { skill, note: skill.note ?? '' }
      status = `Removed ${skill.name} from the list.`
      error = ''
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function undoRemove() {
    const item = removed
    if (!item) return
    busy = true
    try {
      apply(await addListSkill(id, { id: item.skill.id, note: item.note }))
      removed = null
      say(`Put ${item.skill.name} back.`)
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function saveNote(skill: ListSkill) {
    busy = true
    try {
      apply(await updateListSkill(id, skill.id, noteDrafts[skill.id] ?? ''))
      editingNote = ''
      say('Sorted. Note saved.')
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function addAll() {
    busy = true
    try {
      const r = await addListToShelf(id)
      say(r.added === 0 ? 'Every skill was already on your shelf. Nothing changed.' : `Added ${r.added} ${r.added === 1 ? 'skill' : 'skills'} to your shelf as To test.`)
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function copyAll() {
    if (!commands.length) {
      error = 'There are no install commands on this list yet.'
      return
    }
    if (await copyText(commands.join('\n'))) say(`Copied ${commands.length} install ${commands.length === 1 ? 'command' : 'commands'}.`)
    else error = 'Could not copy to the clipboard.'
  }

  async function saveSettings(e: SubmitEvent) {
    e.preventDefault()
    if (form.visibility === 'public' && detail?.visibility !== 'public') {
      reviewingPublication = true
      return
    }
    await persistSettings()
  }

  async function persistSettings() {
    reviewingPublication = false
    if (busy) return
    busy = true
    try {
      apply(await updateList(id, { ...form, audience }))
      settingsOpen = false
      say('Sorted. List settings saved.')
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function removeList() {
    confirmDelete = false
    busy = true
    try {
      await deleteList(id)
      location.assign('/')
    } catch (e) {
      fail(e)
      busy = false
    }
  }

  async function addMember(e: SubmitEvent) {
    e.preventDefault()
    if (!memberEmail.trim()) return
    busy = true
    try {
      apply(await addListMember(id, memberEmail.trim(), memberRole))
      memberEmail = ''
      say('Person added to the list.')
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function changeRole(member: ListMember, role: string) {
    if (role === member.role) return
    busy = true
    try {
      apply(await updateListMember(id, member.id, role))
      say(`${member.name} is now ${role === 'editor' ? 'an editor' : 'a viewer'}.`)
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function removeMember(member: ListMember) {
    busy = true
    try {
      apply(await removeListMember(id, member.id))
      say(`Removed ${member.name} from the list.`)
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  async function handOver() {
    const member = confirmTransfer
    confirmTransfer = null
    if (!member) return
    busy = true
    try {
      apply(await transferList(id, member.id))
      say(`${member.name} now owns this list. You stay on as an editor.`)
    } catch (e) {
      fail(e)
    } finally {
      busy = false
    }
  }

  function visibilityLabel(v: string): string {
    return audienceLabel(v)
  }

  function roleLabel(role: string): string {
    if (role === 'owner') return 'you own this list'
    if (role === 'admin') return 'admin access'
    if (role === 'editor') return 'you can edit'
    if (role === 'viewer') return 'view only'
    return 'shared with you'
  }

  function useTitle(skill: ListSkill): string {
    return `${skill.useCount} of ${skill.memberCount} list members marked this in use`
  }
</script>

{#if !detail && loading}
  <p class="empty" role="status">Loading list…</p>
{:else if !detail}
  <section class="start">
    <p class="eyebrow">Shared list</p>
    <h1>That list is not here.</h1>
    <p class="lede">{error || 'It may have been deleted, or it is private.'}</p>
    <p><a class="back" href="/">Back to your shelf</a></p>
  </section>
{:else}
  <article class="listpage">
    <PageHeader eyebrow={visibilityLabel(detail.visibility)} title={detail.name} lede={detail.description}>
      {#snippet meta()}{#if detail}<span class="badge">{detail.skillCount} {detail.skillCount === 1 ? 'skill' : 'skills'} · {detail.memberCount} {detail.memberCount === 1 ? 'person' : 'people'} · {roleLabel(detail.role ?? '')}</span>{/if}{/snippet}
    </PageHeader>

    <div class="actions">
      {#if canWrite}
        <button type="button" class="btn primary" onclick={addAll} disabled={busy || detail.skillCount === 0}>Add all to my shelf</button>
      {/if}
      <button type="button" class="btn" onclick={copyAll} disabled={!commands.length}>Copy install commands</button>
      {#if detail.canManage}
        <button type="button" class="btn quiet" aria-expanded={settingsOpen} onclick={toggleSettings} disabled={busy}>Settings</button>
        {#if detail.visibility !== 'personal'}<button type="button" class="btn quiet" aria-expanded={peopleOpen} onclick={() => { peopleOpen = !peopleOpen; cancelSettings() }} disabled={busy}>People</button>{/if}
      {/if}
    </div>

    {#if status}
      <p class="status" role="status">
        {status}
        {#if removed}<button type="button" class="inline" onclick={undoRemove} disabled={busy}>Undo</button>{/if}
      </p>
    {/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}

    {#if settingsOpen && detail.canManage}
      <section class="panel" aria-labelledby="list-settings">
        <h2 id="list-settings">List settings</h2>
        <form onsubmit={saveSettings}>
          <label class="field"><span>Name</span><input bind:value={form.name} maxlength="80" required /></label>
          <label class="field"><span>Description</span><input bind:value={form.description} maxlength="240" placeholder="What is this list for?" /></label>
          <AudiencePicker bind:visibility={form.visibility} bind:audience linkEnabled={detail.linkEnabled} legacyAllowed={detail.visibility === 'members'} />
          <div class="row">
            <button type="submit" class="btn primary" disabled={busy}>{busy ? 'Saving…' : form.visibility === 'public' && detail.visibility !== 'public' ? 'Publish to public community' : 'Save settings'}</button>
            <button type="button" class="btn quiet" onclick={cancelSettings} disabled={busy}>Cancel</button>
          </div>
        </form>
        {#if detail.visibility === 'link' || detail.visibility === 'public'}
          <div class="linkbox">
            <p>{detail.visibility === 'public' ? 'Published in the public community. Anyone can discover and read the skills and list notes.' : detail.linkEnabled ? 'Anyone with this link can read the list. No account, names or emails.' : 'Link sharing is turned off by the instance owner. Existing links stop working until it is turned back on.'}</p>
            {#if detail.slug}
              <div class="linkline"><code>{link}</code><CopyButton text={link} compact /></div>
            {/if}
          </div>
        {/if}
        <div class="dangerrow">
          <div>
            <strong>Delete this list</strong>
            <p>The skills stay in the catalogue. This cannot be undone.</p>
          </div>
          <button type="button" class="btn quiet danger" onclick={() => (confirmDelete = true)} disabled={busy}>Delete list</button>
        </div>
      </section>
    {/if}

    {#if peopleOpen && detail.canManage}
      <section class="panel" aria-labelledby="list-people">
        <h2 id="list-people">People</h2>
        <p class="panel-lede">Editors can add skills and notes. Viewers can read the list. The owner can also change who sees it.</p>
        <ul class="people">
          {#each detail.members ?? [] as member (member.id)}
            <li>
              <div class="who">
                <strong>{member.name}{#if member.id === detail.ownerId} <span class="tag-owner">owner</span>{/if}</strong>
                <small>{member.email}</small>
              </div>
              {#if member.role === 'owner'}
                <span class="role">owner</span>
              {:else}
                <select value={member.role} onchange={(e) => changeRole(member, e.currentTarget.value)} disabled={busy} aria-label="Role for {member.name}">
                  <option value="editor">Editor</option>
                  <option value="viewer">Viewer</option>
                </select>
                <button type="button" class="btn quiet danger" onclick={() => removeMember(member)} disabled={busy}>Remove</button>
              {/if}
            </li>
          {/each}
        </ul>
        <form class="addperson" onsubmit={addMember}>
          <label class="vh" for="member-email">Email of a person on this instance</label>
          <input id="member-email" type="email" bind:value={memberEmail} placeholder="sam@example.com" autocomplete="off" />
          <label class="vh" for="member-role">Their role</label>
          <select id="member-role" bind:value={memberRole}>
            <option value="viewer">Viewer</option>
            <option value="editor">Editor</option>
          </select>
          <button type="submit" class="btn" disabled={busy || !memberEmail.trim()}>Add person</button>
        </form>
        {#if detail.canTransfer && (detail.members?.length ?? 0) > 1}
          <div class="handover">
            <label for="transfer-to">Hand this list to</label>
            <select id="transfer-to" bind:value={transferTo}>
              {#each detail.members ?? [] as member (member.id)}
                {#if member.role !== 'owner'}<option value={member.id}>{member.name}</option>{/if}
              {/each}
            </select>
            <button
              type="button" class="btn quiet"
              onclick={() => { const m = detail?.members?.find((x) => x.id === transferTo); if (m) confirmTransfer = m }}
              disabled={busy || !transferTo}
            >Hand over</button>
          </div>
        {/if}
      </section>
    {/if}

    {#if detail.canEdit}
      <form class="addskill" onsubmit={add}>
        <div class="addrow">
          {#if available.length}
            <label class="vh" for="from-shelf">Skill from your shelf</label>
            <select id="from-shelf" bind:value={addID}>
              <option value="">From your shelf…</option>
              {#each available as skill (skill.id)}<option value={skill.id}>{skill.name}</option>{/each}
            </select>
            <span class="or">or</span>
          {/if}
          <label class="vh" for="list-input">Skill install command, skills.sh URL or owner/repo/skill</label>
          <input id="list-input" bind:value={addInput} placeholder="Paste an install command, skills.sh URL or owner/repo/skill" autocomplete="off" />
        </div>
        <div class="addrow second">
          <label class="vh" for="list-note">Why it belongs on this list</label>
          <input id="list-note" bind:value={addNote} maxlength="500" placeholder="Add a note for the team (optional)" />
          <button type="submit" class="btn primary" disabled={busy || (!addInput.trim() && !addID)}>Add to list</button>
        </div>
      </form>
    {/if}

    {#if detail.skills.length === 0}
      <p class="empty">{detail.canEdit ? 'Nowt here yet. Add the first skill above.' : 'This list is empty.'}</p>
    {:else}
      <ol class="entries">
        {#each detail.skills as skill (skill.id)}
          <li class="entry">
            <div class="entry-head">
              <div>
                <h2>
                  {#if skill.url.startsWith('https://www.skills.sh/')}
                    <a href={skill.url} target="_blank" rel="noopener noreferrer">{skill.name}<span class="vh"> (opens skills.sh)</span></a>
                  {:else}{skill.name}{/if}
                </h2>
                <p class="source"><code>{skill.source}</code>{#if skill.installs > 0}<span>· {fmt(skill.installs)} installs</span>{/if}</p>
              </div>
              {#if skill.useCount > 0}
                <span class="adoption" title={useTitle(skill)}>{skill.useCount} of {skill.memberCount} use this</span>
              {/if}
            </div>
            {#if skill.description}<p class="desc">{skill.description}</p>{/if}
            {#if skill.note && editingNote !== skill.id}
              <p class="note">{skill.note}</p>
            {/if}
            {#if skill.addedByName}<p class="added">Added by {skill.addedByName}</p>{/if}

            {#if detail.canEdit && editingNote === skill.id}
              <div class="noteedit">
                <label class="vh" for="note-{skill.id}">List note for {skill.name}</label>
                <textarea id="note-{skill.id}" rows="2" maxlength="500" bind:value={noteDrafts[skill.id]} placeholder="Why it belongs here…"></textarea>
                <div class="row">
                  <button type="button" class="btn primary" onclick={() => saveNote(skill)} disabled={busy}>Save note</button>
                  <button type="button" class="btn quiet" onclick={() => (editingNote = '')}>Cancel</button>
                </div>
              </div>
            {/if}

            <div class="entry-actions">
              {#if installCommand(skill)}
                <div class="command"><code>{installCommand(skill)}</code><CopyButton text={installCommand(skill)!} compact /></div>
              {/if}
              {#if detail.canEdit}
                <button type="button" class="btn quiet" onclick={() => (editingNote = editingNote === skill.id ? '' : skill.id)}>{skill.note ? 'Edit note' : 'Add note'}</button>
                <button type="button" class="btn quiet danger" onclick={() => removeEntry(skill)} disabled={busy}>Remove</button>
              {/if}
            </div>
          </li>
        {/each}
      </ol>
    {/if}
  </article>
{/if}

{#if confirmDelete}
  <ConfirmDialog
    title="Delete this list?"
    body="The skills stay in the catalogue, but the list, its notes and its people go away."
    confirmLabel="Delete list"
    danger
    onconfirm={removeList}
    oncancel={() => (confirmDelete = false)}
  />
{/if}
{#if reviewingPublication}
  <ConfirmDialog
    title="Publish this list to everyone?"
    body="Anyone, including people without an account, will be able to discover and read the content below. Your personal shelf notes, people and machine reports stay private. Copies others make may remain after you unpublish."
    confirmLabel="Publish to public community"
    onconfirm={persistSettings}
    oncancel={() => (reviewingPublication = false)}
  >
    {#snippet children()}
      <div class="publication-preview">
        <h3>{form.name}</h3>
        {#if form.description}<p>{form.description}</p>{/if}
        <p><strong>{detail?.skillCount ?? 0} skills and their shared list notes</strong></p>
        <ul>{#each detail?.skills ?? [] as skill (skill.id)}<li><strong>{skill.name}</strong><p>{skill.description}</p><code>{skill.source}</code><p class="public-note">{skill.note || 'No shared note.'}</p></li>{/each}</ul>
      </div>
    {/snippet}
  </ConfirmDialog>
{/if}
{#if confirmTransfer}
  <ConfirmDialog
    title={`Hand this list to ${confirmTransfer.name}?`}
    body="They become the owner. You stay on the list as an editor."
    confirmLabel="Hand over"
    onconfirm={handOver}
    oncancel={() => (confirmTransfer = null)}
  />
{/if}

<style>
  .listpage { margin-top: 0; }
  .publication-preview { margin-top: 18px; padding: 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); overflow-wrap: anywhere; font-size: 0.85rem; }
  .publication-preview h3 { margin: 0; font: 500 1.3rem/1.3 var(--serif); }
  .publication-preview ul { list-style: none; padding: 0; }
  .publication-preview li { border-top: 1px solid var(--line); padding-top: 10px; margin-top: 10px; }
  .publication-preview p { margin: 6px 0; }
  .public-note { white-space: pre-wrap; border-left: 2px solid var(--accent); padding-left: 8px; }
  .back { color: var(--muted); font-size: 0.85rem; text-decoration: none; }
  .back:hover { color: var(--ink); text-decoration: underline; text-underline-offset: 3px; }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.7rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  h1 { margin: 6px 0 0; font: 600 clamp(1.9rem, 5vw, 2.6rem)/1.06 var(--serif); letter-spacing: -0.025em; overflow-wrap: anywhere; }
  .lede { margin: 10px 0 0; color: var(--muted); max-width: 58ch; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 18px; }
  .status { margin: 14px 0 0; color: var(--good); font-size: 0.88rem; }
  .inline { background: none; border: 0; padding: 0 2px; color: var(--accent); font: inherit; font-weight: 650; text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
  .error { margin: 14px 0 0; color: var(--bad); font-size: 0.88rem; }
  .empty { text-align: center; color: var(--muted); font: italic 1.1rem/1.5 var(--serif); padding: 44px 0; }
  .start { margin-top: 28px; padding: 24px; background: var(--card); border: 1px solid var(--line); border-radius: 16px; }
  .start h1 { margin: 8px 0 0; }

  .panel { margin-top: 18px; padding: 18px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--bg) 38%, var(--card)); }
  .panel h2 { margin: 0 0 4px; font: 500 1.5rem/1.2 var(--serif); letter-spacing: -0.01em; }
  .panel-lede { margin: 0 0 12px; color: var(--muted); font-size: 0.86rem; }
  .field { display: grid; gap: 5px; margin-top: 10px; font-size: 0.8rem; font-weight: 600; }
  .field input { width: 100%; font-weight: 400; }
  .row { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
  .linkbox { margin-top: 16px; padding-top: 14px; border-top: 1px dashed var(--line); }
  .linkbox p { margin: 0 0 8px; color: var(--muted); font-size: 0.84rem; }
  .linkline { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  .linkline code { flex: 1 1 240px; min-width: 0; padding: 8px 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); font-size: 0.78rem; overflow-wrap: anywhere; }
  .dangerrow { display: flex; gap: 14px; align-items: center; justify-content: space-between; margin-top: 18px; padding-top: 14px; border-top: 1px solid var(--line); }
  .dangerrow strong { font-size: 0.88rem; }
  .dangerrow p { margin: 2px 0 0; color: var(--muted); font-size: 0.8rem; }

  .people { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .people li { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 8px 0; border-bottom: 1px dashed var(--line); }
  .who { flex: 1 1 200px; display: grid; gap: 1px; min-width: 0; }
  .who strong { font-size: 0.9rem; overflow-wrap: anywhere; }
  .who small { color: var(--muted); font-size: 0.78rem; overflow-wrap: anywhere; }
  .tag-owner { margin-left: 4px; padding: 1px 6px; border-radius: 999px; background: var(--tint); color: var(--ink); font-size: 0.65rem; font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; }
  .role { color: var(--muted); font-size: 0.82rem; }
  .people select { width: auto; padding: 6px 10px; }
  .addperson { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 14px; }
  .addperson input { flex: 1 1 220px; }
  .handover { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; margin-top: 16px; padding-top: 14px; border-top: 1px solid var(--line); font-size: 0.84rem; }
  .handover select { width: auto; padding: 6px 10px; }

  .addskill { display: grid; gap: 8px; margin-top: 18px; padding: 14px; border: 1px dashed var(--line); border-radius: 12px; }
  .addrow { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  .addrow select { flex: 0 1 220px; }
  .addrow input { flex: 1 1 240px; }
  .addrow.second input { flex: 1 1 320px; }
  .or { color: var(--muted); font-size: 0.78rem; text-transform: uppercase; letter-spacing: 0.08em; }

  .entries { list-style: none; margin: 18px 0 0; padding: 0; display: grid; gap: 14px; }
  .entry { padding: 22px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); box-shadow: 0 18px 50px -48px rgb(0 0 0 / 0.4); }
  .entry-head { display: flex; gap: 12px; align-items: flex-start; justify-content: space-between; }
  .entry h2 { margin: 0; font: 500 1.4rem/1.25 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; }
  .entry h2 a { color: inherit; text-decoration: none; }
  .entry h2 a:hover { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }
  .source { display: flex; flex-wrap: wrap; gap: 6px; margin: 4px 0 0; color: var(--muted); font-size: 0.76rem; }
  .source code { overflow-wrap: anywhere; }
  .adoption { flex: none; padding: 3px 8px; border: 1px solid color-mix(in srgb, var(--good) 35%, var(--line)); border-radius: 999px; color: var(--good); background: color-mix(in srgb, var(--good) 7%, var(--card)); font-size: 0.7rem; font-weight: 650; white-space: nowrap; }
  .desc { margin: 8px 0 0; color: var(--muted); max-width: 64ch; font-size: 0.9rem; }
  .note { margin: 10px 0 0; padding: 6px 0 6px 12px; border-left: 2px solid var(--accent); font: italic 0.95rem/1.5 var(--serif); white-space: pre-wrap; }
  .added { margin: 6px 0 0; color: var(--muted); font-size: 0.72rem; }
  .noteedit { margin-top: 10px; display: grid; gap: 8px; }
  .noteedit textarea { width: 100%; }
  .entry-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 10px; }
  .command { flex: 1 1 260px; display: flex; align-items: flex-start; gap: 8px; min-width: 0; }
  .command code { flex: 1; min-width: 0; font-size: 0.78rem; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
  @media (max-width: 560px) {
    .entry-head { flex-direction: column; }
    .adoption { white-space: normal; }
    .command { flex-basis: 100%; }
    .dangerrow { flex-direction: column; align-items: flex-start; }
    .actions .btn, .row .btn { flex: 1 1 100%; }
  }
  @media (pointer: coarse) {
    .btn, .inline, .back { min-height: 44px; }
    .inline { display: inline-flex; align-items: center; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .entry { animation: rise 220ms var(--ease-out) both; }
    @keyframes rise { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: translateY(0); } }
  }
</style>
