<script lang="ts">
  import ConfirmDialog from './ConfirmDialog.svelte'
  import { statusHint, type AddStatus } from './addStatus'

  // Adding skills to the library as Active, preselected, or Archived: Community's Save and a list's Add all. The title
  // can follow the choice, so Add all says how many skills go on how many machines.
  let { title, body, confirmLabel, onconfirm, oncancel }: {
    title: (status: AddStatus) => string
    body: string
    confirmLabel: string
    onconfirm: (status: AddStatus) => void
    oncancel: () => void
  } = $props()

  const name = $props.id()
  let status = $state<AddStatus>('active')
  const choices: AddStatus[] = ['active', 'archived']
</script>

<ConfirmDialog title={title(status)} {body} {confirmLabel} onconfirm={() => onconfirm(status)} {oncancel}>
  <fieldset class="choice">
    <legend>Add as</legend>
    {#each choices as value (value)}
      <label class="option">
        <input type="radio" {name} {value} bind:group={status} />
        <span class="words"><span class="label">{value === 'active' ? 'Active' : 'Archived'}</span><span class="hint">{statusHint[value]}</span></span>
      </label>
    {/each}
  </fieldset>
</ConfirmDialog>

<style>
  .choice { display: grid; gap: 8px; margin: 16px 0 0; padding: 0; border: 0; min-width: 0; }
  legend { margin: 0 0 8px; padding: 0; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .option { display: flex; align-items: center; gap: 12px; min-height: 44px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); color: var(--ink); cursor: pointer; box-sizing: border-box; }
  .option:has(input:checked) { border-color: var(--accent); background: var(--tint); }
  .option:has(input:focus-visible) { outline: 2px solid var(--accent); outline-offset: 2px; }
  input { flex: none; width: 18px; height: 18px; margin: 0; accent-color: var(--accent); }
  .words { display: grid; gap: 2px; min-width: 0; }
  .label { font-weight: 600; }
  .hint { color: var(--muted); font-size: 0.875rem; overflow-wrap: anywhere; }
</style>
