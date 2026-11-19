<script lang="ts">
  import { tick } from 'svelte'
  import { listLists, UnauthorizedError, type ListRef, type SkillRow } from '../api'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import CopyButton from '../CopyButton.svelte'
  import UpdateFromOriginalDialog from './UpdateFromOriginalDialog.svelte'
  import { arrivedOn, copyPrompt, installCommandFor } from './skillsPage'

  // A skill, opened: where it is on each machine, its tags and lists, where it came from, its notes and what to do with
  // it. Nothing here chooses machines: it reports them. Each write goes through the page (the props below), which keeps
  // the row and says what happened; a refusal shows here, beside what was pressed.
  let { row, id, canWrite, tagNames = [], onnotes, ontag, onlist, onpublish, onuseversion, onoriginal, onerror }: {
    row: SkillRow
    /** The detail's element id, which the name's aria-controls names. */
    id: string
    canWrite: boolean
    /** Every tag the library uses, offered as the person types one. */
    tagNames?: string[]
    /** Each write resolves once saved and rejects with why it wasn't. */
    onnotes: (row: SkillRow, notes: string) => Promise<void>
    ontag: (row: SkillRow, name: string) => Promise<void>
    onlist: (row: SkillRow, list: ListRef) => Promise<void>
    onpublish: (row: SkillRow, isPublic: boolean) => Promise<void>
    onuseversion: (row: SkillRow) => Promise<void>
    onoriginal: (row: SkillRow, digest: string, revision: string) => Promise<void>
    /** A refusal no one here can answer, such as a sign-in that has ended. */
    onerror: (e: unknown) => void
  } = $props()

  const uid = $props.id()
  let problem = $state('')
  function refused(e: unknown) {
    if (e instanceof UnauthorizedError) { onerror(e); return }
    problem = e instanceof Error ? e.message : String(e)
  }

  // Notes: read, then edited in place. Escape or Cancel leaves them as they were.
  let editingNotes = $state(false)
  let notesDraft = $state('')
  let savingNotes = $state(false)
  let notesEl = $state<HTMLTextAreaElement>()
  let notesButton = $state<HTMLButtonElement>()
  async function startNotes() {
    notesDraft = row.notes ?? ''
    editingNotes = true
    await tick()
    notesEl?.focus()
  }
  function stopNotes() {
    editingNotes = false
    void tick().then(() => notesButton?.focus())
  }
  async function saveNotes(e: SubmitEvent) {
    e.preventDefault()
    if (savingNotes) return
    savingNotes = true
    try {
      await onnotes(row, notesDraft.trim())
      stopNotes()
    } catch {
      // The page says why; the draft stays to try again.
    } finally {
      savingNotes = false
    }
  }

  // Add tag: a field that offers the library's tags.
  let tagging = $state(false)
  let tagDraft = $state('')
  let tagBusy = $state(false)
  let tagEl = $state<HTMLInputElement>()
  let tagButton = $state<HTMLButtonElement>()
  const tagOptions = $derived(tagNames.filter(n => !row.tags.some(t => t.name === n)))
  async function startTag() {
    tagDraft = ''
    tagging = true
    problem = ''
    await tick()
    tagEl?.focus()
  }
  function stopTag() {
    tagging = false
    void tick().then(() => tagButton?.focus())
  }
  async function addTag(e: SubmitEvent) {
    e.preventDefault()
    const name = tagDraft.trim()
    if (!name || tagBusy) return
    tagBusy = true
    try {
      await ontag(row, name)
      stopTag()
    } catch (err) {
      refused(err)
    } finally {
      tagBusy = false
    }
  }

  // Add to list: the person's lists this skill is not on yet, read when asked for.
  let listing = $state(false)
  let lists = $state<ListRef[] | null>(null)
  let listChoice = $state('')
  let listBusy = $state(false)
  let listButton = $state<HTMLButtonElement>()
  let listEl = $state<HTMLSelectElement>()
  const listOptions = $derived((lists ?? []).filter(l => !row.lists.some(o => o.id === l.id)))
  async function startList() {
    listing = true
    problem = ''
    try {
      if (!lists) lists = (await listLists()).lists.map(l => ({ id: l.id, name: l.name }))
      listChoice = listOptions[0] ? String(listOptions[0].id) : ''
      await tick()
      listEl?.focus()
    } catch (err) {
      listing = false
      refused(err)
    }
  }
  function stopList() {
    listing = false
    void tick().then(() => listButton?.focus())
  }
  async function addList(e: SubmitEvent) {
    e.preventDefault()
    const list = listOptions.find(l => String(l.id) === listChoice)
    if (!list || listBusy) return
    listBusy = true
    try {
      await onlist(row, list)
      stopList()
    } catch (err) {
      refused(err)
    } finally {
      listBusy = false
    }
  }

  // Publish asks first: a public page is for anyone. Unpublish does not.
  let asking = $state(false)
  let publishing = $state(false)
  let publishButton = $state<HTMLButtonElement>()
  // A public address takes lowercase letters, numbers and single hyphens; a skill from a machine may have any name.
  const publishable = $derived(row.yours || (/^[a-z0-9]+(-[a-z0-9]+)*$/.test(row.name) && row.name.length <= 64))
  async function publish(isPublic: boolean) {
    asking = false
    if (publishing) return
    publishing = true
    problem = ''
    try {
      await onpublish(row, isPublic)
    } catch (err) {
      refused(err)
    } finally {
      publishing = false
      await tick()
      publishButton?.focus()
    }
  }

  let usingVersion = $state(false)
  async function useVersion() {
    if (usingVersion) return
    usingVersion = true
    problem = ''
    try {
      await onuseversion(row)
    } catch (err) {
      refused(err)
    } finally {
      usingVersion = false
    }
  }

  let updating = $state(false)
  let updateButton = $state<HTMLButtonElement>()
  function closeUpdate() {
    updating = false
    void tick().then(() => updateButton?.focus())
  }

  const install = $derived(installCommandFor(row))
  const editHref = $derived(row.originKind === 'written' ? `/hosting?edit=${encodeURIComponent(row.name)}` : `/hosting?item=${encodeURIComponent(row.id)}`)
  const arrived = $derived(arrivedOn(row.createdAt))
  const link = $derived(/^https?:\/\//i.test(row.origin) ? row.origin : '')
  const linkText = $derived(row.origin.replace(/^https?:\/\//i, '').replace(/^www\./i, '').replace(/\/$/, ''))
  const STATES: Record<string, string> = { ok: 'Installed', off: 'Off', installing: 'Installing', removing: 'Removing', failed: 'Failed', offline: 'Offline', blocked: "Can't write skills here" }
  const TONES: Record<string, string> = { ok: 'ok', installing: 'work', removing: 'work', offline: 'work', failed: 'bad', blocked: 'bad', off: 'off' }
</script>

<div class="detail" {id}>
  {#if row.editedOn}
    <div class="edited" role="group" aria-label="Edited on a machine">
      <p>{row.name} was edited on {row.editedOn.host}; reet's copy put back.</p>
      {#if row.editedOnProblem}
        <p class="why">{row.editedOnProblem}</p>
      {:else if canWrite}
        <button type="button" class="btn" aria-busy={usingVersion} aria-disabled={usingVersion} onclick={useVersion}>{usingVersion ? 'Asking…' : 'Use this version'}</button>
      {/if}
    </div>
  {/if}
  {#if row.description}<p class="full">{row.description}</p>{/if}
  <div class="cols">
    <section aria-labelledby={`${uid}-machines`}>
      <h4 id={`${uid}-machines`}>Machines</h4>
      {#if row.health.machines.length}
        <ul class="machines" aria-label={`Machines for ${row.name}`}>
          {#each row.health.machines as m (m.host)}
            <li><span class="mline"><span class="host">{m.host}</span><span class="state" data-tone={TONES[m.state] ?? 'off'}><span class="dot" aria-hidden="true"></span>{STATES[m.state] ?? m.state}</span></span>{#if m.detail}<span class="mdetail">{m.detail}</span>{/if}</li>
          {/each}
        </ul>
      {:else}
        <p class="none">No machine has reported this skill yet.</p>
      {/if}
      <p class="hint">Goes into each agent's home skills folder.</p>
    </section>

    <div class="stack">
      <section aria-labelledby={`${uid}-tags`}>
        <h4 id={`${uid}-tags`}>Tags</h4>
        <div class="chips">
          {#if row.tags.length}<ul class="chip-list" aria-label={`Tags of ${row.name}`}>{#each row.tags as tag (tag.id)}<li>{tag.name}</li>{/each}</ul>{/if}
          {#if canWrite && row.shelfId && !tagging}<button type="button" class="link" bind:this={tagButton} onclick={startTag}>Add tag</button>{/if}
        </div>
        {#if tagging}
          <form class="inline" onsubmit={addTag}>
            <input bind:this={tagEl} bind:value={tagDraft} list={`${uid}-tag-names`} maxlength="64" autocomplete="off" aria-label={`New tag for ${row.name}`}
              onkeydown={e => { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); stopTag() } }} />
            <datalist id={`${uid}-tag-names`}>{#each tagOptions as name (name)}<option value={name}></option>{/each}</datalist>
            <button type="submit" class="btn primary" aria-busy={tagBusy} disabled={!tagDraft.trim()}>Add</button>
            <button type="button" class="btn" onclick={stopTag}>Cancel</button>
          </form>
        {/if}
      </section>
      <section aria-labelledby={`${uid}-lists`}>
        <h4 id={`${uid}-lists`}>Lists</h4>
        <div class="chips">
          {#if row.lists.length}<ul class="chip-list" aria-label={`Lists with ${row.name}`}>{#each row.lists as list (list.id)}<li>{list.name}</li>{/each}</ul>{/if}
          {#if canWrite && row.shelfId && !listing}<button type="button" class="link" bind:this={listButton} onclick={startList}>Add to list</button>{/if}
        </div>
        {#if listing && lists}
          {#if listOptions.length}
            <form class="inline" onsubmit={addList}>
              <select bind:this={listEl} bind:value={listChoice} aria-label={`List for ${row.name}`}
                onkeydown={e => { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); stopList() } }}>
                {#each listOptions as list (list.id)}<option value={String(list.id)}>{list.name}</option>{/each}
              </select>
              <button type="submit" class="btn primary" aria-busy={listBusy}>Add</button>
              <button type="button" class="btn" onclick={stopList}>Cancel</button>
            </form>
          {:else}
            <p class="none">{lists.length ? `${row.name} is on all your lists.` : 'No lists yet.'} <a class="link" href="/lists">{lists.length ? 'Your lists' : 'Make a list'}</a> <button type="button" class="link" onclick={stopList}>Close</button></p>
          {/if}
        {/if}
      </section>
      {#if row.originKind === 'imported' || row.originKind === 'kept'}
        <section aria-labelledby={`${uid}-origin`}>
          <h4 id={`${uid}-origin`}>{row.originKind === 'imported' ? 'Imported from' : 'Kept from'}</h4>
          <p class="origin">{#if link}<a href={link} target="_blank" rel="noopener noreferrer">{linkText}</a>{:else}<span>{row.origin || (row.originKind === 'kept' ? 'a machine' : 'a link')}</span>{/if}{#if arrived}<span class="when">{` · ${arrived}`}</span>{/if}</p>
          <p class="hint">For reference. reet installs its own copy.</p>
          {#if row.originKind === 'imported' && canWrite}<button type="button" class="link" bind:this={updateButton} onclick={() => { problem = ''; updating = true }}>Update from original</button>{/if}
        </section>
      {/if}
    </div>

    <div class="stack">
      <section aria-labelledby={`${uid}-notes`}>
        <h4 id={`${uid}-notes`}>Notes</h4>
        {#if editingNotes}
          <form class="notes-form" onsubmit={saveNotes}>
            <textarea bind:this={notesEl} bind:value={notesDraft} rows="3" maxlength="4000" aria-label={`Notes on ${row.name}`} onkeydown={e => { if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); stopNotes() } }}></textarea>
            <div class="row-acts">
              <button type="submit" class="btn primary" aria-busy={savingNotes}>{savingNotes ? 'Saving…' : 'Save note'}</button>
              <button type="button" class="btn" onclick={stopNotes}>Cancel</button>
            </div>
          </form>
        {:else}
          {#if row.notes}<p class="note-text">{row.notes}</p>{:else}<p class="none">No notes yet.</p>{/if}
          {#if canWrite && row.shelfId}<button type="button" class="link" bind:this={notesButton} onclick={startNotes}>{row.notes ? 'Edit note' : 'Add a note'}</button>{/if}
        {/if}
      </section>
      <div class="uses">
        {#if canWrite}<a class="btn" href={editHref} aria-label={`Edit ${row.name}`}>Edit</a>{/if}
        <CopyButton text={copyPrompt(row)} label="Copy prompt" ariaLabel={`Copy prompt for ${row.name}`} />
        {#if canWrite && row.published}
          <button type="button" class="btn" bind:this={publishButton} aria-label={`Unpublish ${row.name}`} aria-busy={publishing} aria-disabled={publishing} onclick={() => publish(false)}>{publishing ? 'Unpublishing…' : 'Unpublish'}</button>
        {:else if canWrite && publishable}
          <button type="button" class="btn" bind:this={publishButton} aria-label={`Publish ${row.name}`} aria-busy={publishing} aria-disabled={publishing} onclick={() => { if (!publishing) asking = true }}>{publishing ? 'Publishing…' : 'Publish'}</button>
        {/if}
        {#if install}<CopyButton text={install} label="Copy install command" ariaLabel={`Copy install command for ${row.name}`} />{/if}
      </div>
      {#if row.published && row.publicProblem}<p class="behind">{row.publicProblem}</p>{/if}
      {#if !row.published && !publishable}
        <p class="hint">reet.sh can only publish a skill whose name uses lowercase letters, numbers and single hyphens, so {row.name} stays private.</p>
      {:else if !install}
        <p class="hint">Private. Copy install command appears once it's published.</p>
      {/if}
      {#if problem}<p class="problem" role="alert">{problem}</p>{/if}
    </div>
  </div>
</div>

{#if asking}
  <ConfirmDialog title={`Publish ${row.name}?`} confirmLabel="Publish"
    body={`Its instructions and files go on a public page at reet.sh that anyone can read and install from.${row.originKind === 'imported' ? ' The page credits the link it was imported from.' : ''} Unpublish takes the page down again.`}
    onconfirm={() => void publish(true)}
    oncancel={() => { asking = false; void tick().then(() => publishButton?.focus()) }} />
{/if}

{#if updating}
  <UpdateFromOriginalDialog name={row.name} id={row.id} onerror={onerror}
    onupdate={(digest, revision) => onoriginal(row, digest, revision)}
    onclose={closeUpdate} />
{/if}

<style>
  .detail { padding: 0 20px 20px; display: flex; flex-direction: column; gap: 14px; }
  .full { margin: 0; color: var(--ink-soft); max-width: 72ch; overflow-wrap: anywhere; }
  .edited { display: flex; align-items: center; gap: 8px 14px; flex-wrap: wrap; padding: 10px 14px; border-radius: 10px; background: color-mix(in srgb, var(--warn) 10%, var(--card)); border: 1px solid color-mix(in srgb, var(--warn) 30%, var(--line)); }
  .edited p { margin: 0; flex: 1 1 260px; color: var(--ink); font-size: 0.9375rem; line-height: 1.45; overflow-wrap: anywhere; }
  .edited .why { color: var(--ink-soft); font-size: 0.875rem; }
  .cols { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 24px 28px; }
  .stack { display: flex; flex-direction: column; gap: 18px; min-width: 0; }
  section { min-width: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 4px; }
  h4 { margin: 0 0 4px; color: var(--muted); font: 500 0.875rem/1.3 var(--mono); letter-spacing: 0.06em; text-transform: uppercase; }
  .machines { list-style: none; margin: 0; padding: 0; width: 100%; }
  .machines li { display: flex; flex-direction: column; gap: 6px; padding: 10px 0; border-top: 1px solid var(--line-soft); font-size: 0.875rem; }
  .machines li:first-child { border-top: 0; padding-top: 2px; }
  .mline { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
  .host { font-family: var(--mono); overflow-wrap: anywhere; }
  .state { display: inline-flex; align-items: center; gap: 8px; font-weight: 500; text-align: right; }
  .state[data-tone='ok'] { color: var(--good); }
  .state[data-tone='work'] { color: var(--info); }
  .state[data-tone='bad'] { color: var(--bad); }
  .state[data-tone='off'] { color: var(--muted); }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: currentColor; flex: none; }
  .mdetail { padding: 8px 10px; border-radius: 8px; background: color-mix(in srgb, var(--bad) 8%, var(--card)); color: var(--ink-soft); line-height: 1.45; overflow-wrap: anywhere; }
  .chips { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .chip-list { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; min-width: 0; }
  .chip-list li { padding: 4px 9px; border-radius: 999px; background: var(--bg-sunk); color: var(--ink-soft); font: 0.875rem/1.3 var(--mono); overflow-wrap: anywhere; }
  .origin { margin: 0; color: var(--ink-soft); font-size: 0.9375rem; overflow-wrap: anywhere; }
  .origin a { color: var(--accent-text); display: inline-flex; align-items: center; min-height: 44px; }
  .hint { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.45; }
  .none { margin: 0; color: var(--muted); font-size: 0.875rem; }
  .note-text { margin: 0; color: var(--ink); font-size: 0.9375rem; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
  .link {
    display: inline-flex; align-items: center; min-height: 44px; padding: 0; border: 0; background: transparent; color: var(--accent-text);
    font: inherit; font-size: 0.875rem; font-weight: 600; cursor: pointer; text-decoration: underline; text-underline-offset: 3px;
  }
  .inline { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; width: 100%; }
  .inline input, .inline select { flex: 1 1 140px; min-width: 0; min-height: 44px; padding: 0 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font: inherit; font-size: 1rem; box-sizing: border-box; }
  .notes-form { display: flex; flex-direction: column; gap: 8px; width: 100%; }
  .notes-form textarea { width: 100%; min-height: 88px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--ink); font: inherit; font-size: 1rem; resize: vertical; box-sizing: border-box; }
  .row-acts, .uses { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
  .detail :global(.btn) { min-height: 44px; display: inline-flex; align-items: center; justify-content: center; text-decoration: none; border-radius: 10px; }
  .detail :global(.btn[aria-disabled='true']) { opacity: 0.6; cursor: progress; }
  .behind { margin: 0; padding: 8px 10px; border-left: 3px solid var(--warn); border-radius: 6px; background: color-mix(in srgb, var(--warn) 10%, var(--card)); color: var(--ink); font-size: 0.875rem; line-height: 1.45; overflow-wrap: anywhere; }
  .problem { margin: 0; color: var(--bad); font-size: 0.875rem; overflow-wrap: anywhere; }
  /* A narrower screen: the machines beside everything else, then one column on a phone. */
  @media (max-width: 1000px) {
    .cols { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .cols > .stack:last-child { grid-column: 1 / -1; }
  }
  @media (max-width: 700px) {
    .detail { padding: 0 16px 16px; }
    .cols { grid-template-columns: minmax(0, 1fr); gap: 18px; }
    .uses :global(.btn) { flex: 1 1 auto; }
  }
</style>
