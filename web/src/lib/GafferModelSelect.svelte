<script lang="ts">
  // A model for one agent, chosen from what the machine actually offers: the person's favourites, the machine's own
  // list and reet's suggestions, with "Other…" for a name none of them has. An empty value is the agent's default.
  import { modelOptions, type MachineModels, type ModelFavourite } from './models'
  import { modelSuggestions } from './agentLaunch'

  let { tool, value = $bindable(''), machine = null, favourites = [], host = '', label, disabled = false, onchange }: {
    tool: string; value?: string; machine?: MachineModels | null; favourites?: ModelFavourite[]; host?: string
    label: string; disabled?: boolean; onchange?: (value: string) => void
  } = $props()
  const OTHER = '\u0000other'
  let other = $state(false)
  const options = $derived(tool ? modelOptions({ tool, current: '', machine, favourites, recent: [], suggestions: modelSuggestions(tool), selectedModel: other ? '' : value, host }) : [])
  const groups = $derived([...new Set(options.filter(option => option.value).map(option => option.group))])
  const listed = $derived(options.some(option => option.value === value))
  // A name the machine did not list cannot be checked here: the agent confirms it at launch and refuses one it does not offer.
  const unchecked = $derived(!!value.trim() && !(machine?.models ?? []).some(entry => entry.id === value.trim()))
  const set = (next: string) => { value = next; onchange?.(next) }
  function pick(event: Event) {
    const chosen = (event.currentTarget as HTMLSelectElement).value
    if (chosen === OTHER) { other = true; set('') } else { other = false; set(chosen) }
  }
  // Another agent's model name means nothing to this one.
  let before = ''
  $effect(() => { if (tool !== before) { if (before) other = false; before = tool } })
</script>

<span class="model">
  <select aria-label={label} {disabled} value={other ? OTHER : listed ? value : ''} onchange={pick}>
    <option value="">Agent’s default model</option>
    {#each groups as group (group)}<optgroup label={group}>
      {#each options.filter(option => option.value && option.group === group) as option (option.value)}<option value={option.value}>{option.label}{option.detail ? ` (${option.detail})` : ''}</option>{/each}
    </optgroup>{/each}
    <option value={OTHER}>Other…</option>
  </select>
  {#if unchecked}<span class="unchecked">Not checked against this machine. If the agent does not offer this model, the stage stops when it starts.</span>{/if}
  {#if other}<input aria-label={`${label} name`} {value} maxlength="200" placeholder="Model name" {disabled} oninput={event => set((event.currentTarget as HTMLInputElement).value)}/>{/if}
</span>

<style>
  .model{display:grid;gap:6px;min-width:0}.unchecked{font-size:.875rem;font-weight:400;color:var(--muted)}
  select,input{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;max-width:100%;min-height:44px;box-sizing:border-box}
  select:focus-visible,input:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
</style>
