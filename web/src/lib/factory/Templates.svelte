<script lang="ts">
  // The person's private templates: the list, and one template's plan read only with its versions. A template is used,
  // renamed (which makes a new version) or deleted here. Changing the plan itself is done by starting from it, adjusting,
  // and saving the finished run as a new version.
  import { onMount, tick } from 'svelte'
  import { ApiError, deleteGafferTemplate, diffGafferTemplate, editGafferTemplate, getGafferTemplate, listGafferTemplates, type Template, type TemplateDiff, type TemplateSummary } from '../api'
  import { formatWhen } from '../time'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  import ConfirmDialog from '../ConfirmDialog.svelte'
  import TemplatePlan from './TemplatePlan.svelte'
  import TemplateChanges from './TemplateChanges.svelte'
  import { attemptIds, mayHaveArrived } from './delivery'
  import { conflictWords, usedWords } from './templates'

  let { openId = '', timeZone = '', dateFormat = 'locale', onuse, onclose }: { openId?: string; timeZone?: string; dateFormat?: string; onuse: (id: string, version: number) => void; onclose: () => void } = $props()
  const uid = $props.id()
  const life = new AbortController()
  let list = $state<TemplateSummary[] | null>(null), error = $state(''), current = $state<Template | null>(null), reading = $state(false)
  let notice = $state(''), bad = $state(false)
  const say = (text: string, failed = false) => { notice = text; bad = failed }
  const when = (at: number) => formatWhen(at, timeZone, dateFormat)
  let heading = $state<HTMLElement>(), listEl = $state<HTMLElement>()
  const focusList = () => listEl?.querySelector<HTMLElement>('.open')?.focus({ preventScroll: true })

  async function loadList() {
    try { list = await listGafferTemplates(life.signal); error = '' } catch (e) { if (!life.signal.aborted) error = e instanceof Error ? e.message : 'The templates could not be read.' }
  }
  // The latest version, or one asked for. A template that is gone leaves the person on the list, with the reason.
  async function open(id: string, version = 0) {
    reading = true
    try {
      current = await getGafferTemplate(id, version, life.signal); editing = false; void compare()
      await tick(); heading?.focus({ preventScroll: true })
    } catch (e) {
      if (life.signal.aborted) return
      current = null; say(e instanceof ApiError && e.status === 404 ? 'That template or version is not kept any more.' : e instanceof Error ? e.message : 'The template could not be read.', true)
      await loadList()
    } finally { reading = false }
  }
  onMount(() => { void loadList(); if (openId) void open(openId); return () => life.abort() })
  async function back() {
    current = null; compared = null; notice = ''
    await loadList(); await tick(); focusList()
  }

  // Comparing two versions of the one open.
  let from = $state(0), to = $state(0), compared = $state<TemplateDiff | null>(null), compareError = $state('')
  async function compare() {
    const one = current
    if (!one || one.versions.length < 2) { compared = null; return }
    if (!from || !to || !one.versions.includes(from) || !one.versions.includes(to)) { to = one.latest; from = one.versions.includes(one.lastUsedVersion ?? 0) && one.lastUsedVersion !== to ? one.lastUsedVersion! : one.versions[one.versions.length - 2] }
    try { compared = await diffGafferTemplate(one.id, from, to, life.signal); compareError = '' } catch (e) { if (!life.signal.aborted) { compared = null; compareError = e instanceof Error ? e.message : 'The versions could not be compared.' } }
  }

  // Renaming makes a new version with the same plan; the request ID is kept while the same edit is tried again.
  let editing = $state(false), name = $state(''), description = $state(''), saving = $state(false), editProblem = $state('')
  const ids = attemptIds()
  function startEdit() { if (!current) return; name = current.content.name; description = current.content.description; editProblem = ''; editing = true }
  async function saveEdit() {
    const one = current
    if (!one || saving || !name.trim()) return
    saving = true; editProblem = ''
    const body = { baseVersion: one.latest, content: { ...one.content, name: name.trim(), description: description.trim() } }
    try {
      current = await editGafferTemplate(one.id, { ...body, requestId: ids.idFor(JSON.stringify(body)) }, life.signal)
      ids.clear(); editing = false; say(`Saved as version ${current.version}. Runs already started keep the version they used.`); void compare(); void loadList()
    } catch (e) {
      if (life.signal.aborted) return
      const code = e instanceof ApiError ? e.code : ''
      if (!mayHaveArrived(e)) ids.clear()
      if (code === 'template_changed') { await open(one.id); editing = true; editProblem = conflictWords(code, '') }
      else editProblem = e instanceof Error && e.message ? conflictWords(code, e.message) + (mayHaveArrived(e) ? ' It may not have arrived. Trying again is safe: it is saved once.' : '') : 'The template was not saved.'
    } finally { saving = false }
  }

  let deleting = $state(false)
  async function remove() {
    const one = current
    deleting = false
    if (!one) return
    try { await deleteGafferTemplate(one.id, one.latest, life.signal); current = null; say(`Deleted “${one.content.name}”. Runs already started from it were not touched.`); await loadList(); await tick(); focusList() }
    catch (e) {
      if (life.signal.aborted) return
      const code = e instanceof ApiError ? e.code : ''
      if (code === 'template_changed') { await open(one.id); say(conflictWords(code, ''), true) } else say(e instanceof Error ? e.message : 'The template was not deleted.', true)
    }
  }
</script>

<WorkspaceDialog title={current ? current.content.name : 'Templates'} subtitle={current ? '' : 'Plans you saved from finished outcomes'} wide {onclose}>
  <div class="templates">
    {#if notice}<p class="notice" class:bad role={bad ? 'alert' : 'status'}>{notice}</p>{/if}
    {#if current}
      {@const one = current}
      <nav class="bar" aria-label="Template"><button class="btn quiet" type="button" onclick={() => void back()}>‹ All templates</button></nav>
      <div class="head">
        <h3 tabindex="-1" bind:this={heading}>{one.content.name}</h3>
        {#if one.content.description}<p class="description">{one.content.description}</p>{/if}
        <p class="meta">Version {one.version}{one.version !== one.latest ? ` (the latest is ${one.latest})` : ''} · {one.content.pipelines.length} {one.content.pipelines.length === 1 ? 'pipeline' : 'pipelines'} · saved {when(one.createdAt)} · {usedWords(one.lastUsedVersion)}</p>
      </div>
      {#if one.versions.length > 1}<div class="field"><label for={`${uid}-version`}>Version</label>
        <select id={`${uid}-version`} value={one.version} onchange={event => void open(one.id, Number(event.currentTarget.value))}>{#each [...one.versions].reverse() as n (n)}<option value={n}>Version {n}{n === one.latest ? ' (latest)' : ''}{n === one.lastUsedVersion ? ' (last used)' : ''}</option>{/each}</select></div>{/if}
      <div class="actions">
        <button class="btn primary" type="button" onclick={() => onuse(one.id, one.version)}>Use version {one.version}</button>
        {#if one.version === one.latest}<button class="btn" type="button" disabled={editing} onclick={startEdit}>Rename or change the description</button>{/if}
        <button class="btn quiet" type="button" onclick={() => (deleting = true)}>Delete template…</button>
      </div>
      <p class="hint">Using it starts a new outcome from this plan. You choose the project, agents and settings then. To change the plan, start from it, adjust, and save the finished run as a new version.</p>
      {#if editing}<form class="edit" aria-label="Rename or change the description" onsubmit={event => { event.preventDefault(); void saveEdit() }}>
        <label for={`${uid}-name`}>Name</label><input id={`${uid}-name`} bind:value={name} maxlength="80" disabled={saving} required />
        <label for={`${uid}-description`}>Description <span class="hint">(optional)</span></label><textarea id={`${uid}-description`} bind:value={description} rows="3" maxlength="1000" disabled={saving}></textarea>
        <p class="hint">This saves version {one.latest + 1} with the same plan.</p>
        {#if editProblem}<p class="warning" role="alert">{editProblem}</p>{/if}
        <div class="actions"><button class="btn primary" type="submit" disabled={saving || !name.trim()}>{saving ? 'Saving…' : `Save as version ${one.latest + 1}`}</button><button class="btn quiet" type="button" disabled={saving} onclick={() => (editing = false)}>Cancel</button></div>
      </form>{/if}
      {#if one.skillGaps?.length}<p class="hint" role="status">{one.skillGaps.length === 1 ? 'A skill' : `${one.skillGaps.length} skills`} in this plan {one.skillGaps.length === 1 ? 'is' : 'are'} no longer approved at the revision saved. Starting it asks you to start without {one.skillGaps.length === 1 ? 'it' : 'them'}.</p>{/if}
      <TemplatePlan content={one.content} gaps={one.skillGaps} />
      {#if one.versions.length > 1}<section class="compare" aria-label="Compare versions">
        <h4>Compare versions</h4>
        <div class="pair">
          <label>From<select value={from} onchange={event => { from = Number(event.currentTarget.value); void compare() }}>{#each one.versions as n (n)}<option value={n}>Version {n}</option>{/each}</select></label>
          <label>To<select value={to} onchange={event => { to = Number(event.currentTarget.value); void compare() }}>{#each one.versions as n (n)}<option value={n}>Version {n}</option>{/each}</select></label>
        </div>
        {#if compareError}<p class="warning" role="alert">{compareError}</p>
        {:else if compared?.changed}<TemplateChanges diff={compared} title={`Version ${compared.from} to version ${compared.to}`} />
        {:else if compared}<p class="hint" role="status">{compared.from === compared.to ? 'Choose two different versions.' : `Versions ${compared.from} and ${compared.to} are the same.`}</p>{/if}
      </section>{/if}
    {:else if reading}<p role="status">Reading the template…</p>
    {:else if error}<p class="warning" role="alert">{error}</p><div class="actions"><button class="btn" type="button" onclick={() => void loadList()}>Try again</button></div>
    {:else if !list}<p role="status">Reading your templates…</p>
    {:else if !list.length}<div class="empty"><p><strong>No templates yet.</strong></p><p class="hint">Finish an outcome, then choose Save as template on it. Only you can see your templates.</p></div>
    {:else}
      <ul class="list" aria-label="Your templates" bind:this={listEl}>
        {#each list as item (item.id)}<li>
          <div class="what">
            <button class="open" type="button" onclick={() => void open(item.id)}>{item.name}</button>
            {#if item.description}<p class="description">{item.description}</p>{/if}
            <p class="meta">Version {item.version} · {item.pipelines} {item.pipelines === 1 ? 'pipeline' : 'pipelines'} · changed {when(item.updatedAt)} · {usedWords(item.lastUsedVersion)}</p>
          </div>
          <button class="btn primary" type="button" aria-label={`Use ${item.name}`} onclick={() => onuse(item.id, item.version)}>Use</button>
        </li>{/each}
      </ul>
    {/if}
  </div>
</WorkspaceDialog>
{#if deleting && current}<ConfirmDialog title={`Delete “${current.content.name}”?`} body={`This removes the template and its ${current.versions.length === 1 ? 'version' : `${current.versions.length} versions`}. Runs already started from it keep their own inputs and are not touched.`} confirmLabel="Delete template" danger oncancel={() => (deleting = false)} onconfirm={() => void remove()}/>{/if}

<style>
  .templates{display:grid;gap:14px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.hint,.meta{color:var(--muted)}.warning{color:var(--bad)}
  .notice{padding:10px 14px;border:1px solid var(--line);border-radius:10px;background:var(--tint);font-weight:600}.notice.bad{color:var(--bad)}
  .head{display:grid;gap:4px}.head h3{margin:0;font:600 1.1rem/1.3 var(--sans);overflow-wrap:anywhere}.head h3:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  h4{margin:0;font:600 1rem/1.3 var(--sans)}
  .bar{display:flex}.actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  .list{list-style:none;margin:0;padding:0;display:grid;gap:8px}.list li{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:12px;border:1px solid var(--line);border-radius:12px;min-width:0}.what{display:grid;gap:2px;min-width:0}
  .open{border:0;background:none;padding:0;min-height:44px;justify-self:start;text-align:left;font:600 1rem var(--sans);color:var(--accent-text);cursor:pointer;overflow-wrap:anywhere}
  .field,.pair label{display:grid;gap:6px;font-weight:600;min-width:0}.pair{display:grid;grid-template-columns:1fr 1fr;gap:12px}
  .edit{display:grid;gap:8px;padding:12px;border:1px solid var(--line);border-radius:12px}.edit label{font-weight:600}
  input,select,textarea{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;max-width:100%;min-height:44px;box-sizing:border-box}textarea{resize:vertical}
  .compare{display:grid;gap:10px}
  button:focus-visible,select:focus-visible,textarea:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  @media(max-width:479px){.list li{flex-wrap:wrap}.list li>.btn{flex:1 1 auto}.pair{grid-template-columns:1fr}}
</style>
