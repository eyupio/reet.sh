<script lang="ts">
  // The acceptance criteria of one pipeline, as a list of one-line fields: add, edit and remove. Empty rows are dropped
  // when the list is sent (`cleanCriteria`), so a row left blank is never a problem. The same editor is used where the
  // plan is approved, where pipelines are added, and on a pipeline's delivery review.
  import { MAX_CRITERIA, MAX_CRITERION_BYTES } from '../gafferOutcome'

  let { items = $bindable(), ids = $bindable(), who, disabled = false, explain = true }: {
    /** The rows. A parent that has none yet passes undefined; the editor then starts the list. */
    items?: string[]
    /** Where an existing criterion keeps its ID: kept level with `items` as rows are added and removed. */
    ids?: (string | undefined)[]
    /** What the criteria are for, in the fields' names: "pipeline 1", "Export API". */
    who: string
    disabled?: boolean
    /** One line under the heading saying what criteria do. */
    explain?: boolean
  } = $props()
  const uid = $props.id()
  const rows = $derived(items ?? [])
  const full = $derived(rows.length >= MAX_CRITERIA)
  let root = $state<HTMLElement>()
  // Where the focus goes once the rows have been drawn: a row's index, or -1 for the add button.
  let focusAt = $state<number | null>(null)
  $effect(() => {
    if (focusAt === null || !root) return
    const fields = root.querySelectorAll<HTMLInputElement>('li input')
    const want = focusAt
    focusAt = null
    if (want >= 0 && fields[want]) fields[want].focus(); else root.querySelector<HTMLButtonElement>('button.add')?.focus()
  })
  function add() {
    if (full || disabled) return
    const at = rows.length
    items = [...rows, '']
    if (ids) ids = [...ids, undefined]
    focusAt = at
  }
  function remove(index: number) {
    const left = rows.filter((_, at) => at !== index)
    items = left
    if (ids) ids = ids.filter((_, at) => at !== index)
    // The focus stays in the list: on the field that took the removed one's place, else the one before it, else the add button.
    focusAt = Math.min(index, left.length - 1)
  }
</script>

<fieldset class="criteria" {disabled} bind:this={root}>
  <legend>Acceptance criteria</legend>
  {#if explain}<p class="hint" id={`${uid}-why`}>What must be true for this to count as delivered. The pipeline stops for a delivery review against these before it merges or finishes.</p>{/if}
  {#if rows.length}
    <ul aria-label={`Acceptance criteria for ${who}`}>
      {#each rows as _, index (index)}
        <li>
          <input type="text" aria-label={`Criterion ${index + 1} for ${who}`} aria-describedby={explain ? `${uid}-why` : undefined} bind:value={items![index]} maxlength={MAX_CRITERION_BYTES} placeholder="The export downloads as CSV."
            onkeydown={event => { if (event.key === 'Enter') { event.preventDefault(); add() } }}/>
          <button class="link" type="button" aria-label={`Remove criterion ${index + 1} for ${who}`} onclick={() => remove(index)}>Remove</button>
        </li>
      {/each}
    </ul>
  {/if}
  <button class="btn quiet add" type="button" disabled={full} onclick={add}>{rows.length ? 'Add another criterion' : 'Add a criterion'}</button>
  {#if full}<p class="hint">That is the most a pipeline takes ({MAX_CRITERIA}).</p>{/if}
</fieldset>

<style>
  .criteria{border:0;margin:0;padding:0;display:grid;gap:6px;min-width:0}
  legend{padding:0;font-weight:600;margin-bottom:2px}
  .hint{margin:0;color:var(--muted);font-size:0.875rem;overflow-wrap:anywhere}
  ul{list-style:none;margin:0;padding:0;display:grid;gap:6px}
  li{display:flex;align-items:center;gap:6px;min-width:0}
  input{flex:1 1 auto;min-width:0;font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-height:44px;box-sizing:border-box}
  .link{flex:none;border:0;background:none;color:var(--accent-text);font:600 14px var(--sans);cursor:pointer;min-height:44px;padding:0 4px}
  .add{justify-self:start;min-height:44px}
  input:focus-visible,button:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
</style>
