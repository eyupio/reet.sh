<script lang="ts">
  // Saves a finished outcome's plan as a private template, or as the next version of one the person already has.
  // The request ID is kept while the same request is tried again, so a lost answer saves once.
  import { onMount, untrack } from 'svelte'
  import { ApiError, listGafferTemplates, saveGafferTemplate, type GafferOutcome, type Template, type TemplateSummary } from '../api'
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  import { attemptIds, mayHaveArrived } from './delivery'
  import { conflictWords } from './templates'

  let { record, onclose, onsaved }: { record: GafferOutcome; onclose: () => void; onsaved: (template: Template) => void } = $props()
  const id = $props.id()
  const life = new AbortController()
  const first = untrack(() => record.objective.trim().replace(/\.+$/, '').slice(0, 80))
  let existing = $state<TemplateSummary[]>([]), target = $state(''), name = $state(first), description = $state('')
  // A field the person has typed in is theirs: choosing another template to follow never writes over it.
  let nameEdited = false, descriptionEdited = false
  let busy = $state(false), problem = $state(''), notice = $state('')
  const ids = attemptIds()
  const chosen = $derived(existing.find(item => item.id === target))
  const read = () => listGafferTemplates(life.signal).then(found => { existing = found }).catch(() => { /* saving as a new template still works */ })
  onMount(() => { void read(); return () => life.abort() })
  // A version is named after what it follows, so choosing one starts the fields from that template, where they are still untouched.
  function choose(next: string) { target = next; const one = existing.find(item => item.id === next); if (!nameEdited) name = one ? one.name : first; if (!descriptionEdited) description = one ? one.description : '' }
  async function save() {
    if (busy || !name.trim()) return
    busy = true; problem = ''; notice = ''
    const body = { outcomeId: record.id, name: name.trim(), description: description.trim(), ...(chosen ? { templateId: chosen.id, baseVersion: chosen.version } : {}) }
    try {
      const saved = await saveGafferTemplate({ ...body, requestId: ids.idFor(JSON.stringify(body)) }, life.signal)
      ids.clear(); onsaved(saved)
    } catch (e) {
      if (life.signal.aborted) return
      const code = e instanceof ApiError ? e.code : ''
      // A refusal is final for that request; a lost answer is not, and keeps its ID so trying again is safe.
      if (!mayHaveArrived(e)) ids.clear()
      if (code === 'template_changed') { await read(); const now = existing.find(item => item.id === body.templateId); notice = conflictWords(code, '', now?.version ?? 0) }
      else problem = e instanceof Error && e.message ? conflictWords(code, e.message) + (mayHaveArrived(e) ? ' It may not have arrived. Trying again is safe: it is saved once.' : '') : 'The template was not saved.'
    } finally { busy = false }
  }
</script>

<WorkspaceDialog title="Save as template" {onclose}>
  <form onsubmit={event => { event.preventDefault(); void save() }}>
    <p class="hint">Keeps the plan: each pipeline’s prompt, stages and criteria, and the skills approved for it. It does not keep the machine, folder, agents or sign-ins. You choose those when you use it.</p>
    {#if existing.length}
      <label for={`${id}-target`}>Save as</label>
      <select id={`${id}-target`} value={target} disabled={busy} onchange={event => choose(event.currentTarget.value)}>
        <option value="">A new template</option>
        {#each existing as item (item.id)}<option value={item.id}>A new version of {item.name}</option>{/each}
      </select>
    {/if}
    <label for={`${id}-name`}>Name</label>
    <input id={`${id}-name`} bind:value={name} oninput={() => (nameEdited = true)} maxlength="80" disabled={busy} required />
    <label for={`${id}-description`}>Description <span class="hint">(optional)</span></label>
    <textarea id={`${id}-description`} bind:value={description} oninput={() => (descriptionEdited = true)} rows="3" maxlength="1000" disabled={busy}></textarea>
    <p class="warning" class:none={!notice} role="status">{notice}</p>
    {#if problem}<p class="warning" role="alert">{problem}</p>{/if}
    <div class="actions"><button class="btn primary" type="submit" disabled={busy || !name.trim()}>{busy ? 'Saving…' : chosen ? `Save as version ${chosen.version + 1}` : 'Save template'}</button><button class="btn quiet" type="button" disabled={busy} onclick={onclose}>Cancel</button></div>
  </form>
</WorkspaceDialog>

<style>
  form{display:grid;gap:10px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.hint{color:var(--muted);font-weight:400}.warning{color:var(--bad)}
  label{font-weight:600}
  input,select,textarea{font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;max-width:100%;min-height:44px;box-sizing:border-box}textarea{resize:vertical}
  .none{position:absolute;width:1px;height:1px;margin:-1px;overflow:hidden;clip-path:inset(50%)}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,select:focus-visible,textarea:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
</style>
