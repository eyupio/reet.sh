<script lang="ts">
  // Cline's billing source, in one place: the sources reet knows, the saved default, and the star that
  // saves or clears it. Start an agent and Give Gaffer an outcome use this same control.
  import { saveProfile, UnauthorizedError } from './api'
  import { KNOWN_SOURCES, sourceLabel } from './clineSource'

  let { value = '', saved = '', label = 'Source', offered = KNOWN_SOURCES.map(source => source.id), disabled = false, note = '', onchange, onsaved, onsignedout }: {
    value?: string
    saved?: string
    label?: string
    offered?: readonly string[]
    disabled?: boolean
    note?: string
    onchange?: (value: string) => void
    onsaved?: (saved: string) => void
    onsignedout?: (e: unknown) => void
  } = $props()

  const id = $props.id()
  let busy = $state(false), error = $state('')
  const options = $derived.by(() => {
    const ids = [...offered]
    // A saved id Cline reported but reet has no label for is shown as given and still applies.
    if (saved && !ids.includes(saved)) ids.push(saved)
    return ids.map(source => ({ id: source, label: sourceLabel(source) }))
  })
  const isDefault = $derived(!!value && value === saved)

  function pick(event: Event) {
    const next = (event.currentTarget as HTMLSelectElement).value
    if (next !== saved) error = ''
    onchange?.(next)
  }

  async function toggleDefault() {
    if (!value || busy || disabled) return
    busy = true; error = ''
    try {
      const answer = await saveProfile({ defaultClineSource: isDefault ? '' : value })
      onsaved?.(answer.profile.defaultClineSource ?? '')
    } catch (e) {
      if (e instanceof UnauthorizedError) { onsignedout?.(e); return }
      error = e instanceof Error && e.message ? e.message : 'Couldn’t save the default source just now. Try again.'
    } finally { busy = false }
  }
</script>

<div class="cline-source">
  <label for={id}>{label}</label>
  <div class="source-row">
    <select {id} value={value} {disabled} onchange={pick}>
      <option value="">Cline’s own choice</option>
      {#each options as choice (choice.id)}<option value={choice.id}>{choice.label}</option>{/each}
    </select>
    <button type="button" class="star" class:on={isDefault} disabled={disabled || !value || busy} aria-pressed={isDefault} aria-label={isDefault ? `${sourceLabel(value)} is the default source. Clear it` : value ? `Make ${sourceLabel(value)} the default source` : 'Make a source the default'} title={isDefault ? 'New Cline sessions start on this source. Click to clear.' : 'Make this the source new Cline sessions start on'} onclick={() => void toggleDefault()}>
      <svg viewBox="0 0 24 24" width="15" height="15" fill={isDefault ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.8" stroke-linejoin="round" aria-hidden="true"><path d="M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.9L12 16.9 6.8 19.7l1-5.9L3.5 9.7l5.9-.8z" /></svg>
      <span>{busy ? 'Saving…' : isDefault ? 'Default' : 'Make default'}</span>
    </button>
  </div>
  {#if note}<p class="hint">{note}</p>{/if}
  {#if error}<p class="reason" role="alert">{error}</p>{/if}
</div>

<style>
  .cline-source { display: grid; gap: 6px; min-width: 0; }
  label { font-size: 0.875rem; font-weight: 600; }
  .source-row { display: flex; align-items: center; gap: 8px; min-width: 0; }
  select { flex: 1 1 auto; min-width: 0; width: 100%; min-height: 42px; padding: 8px 12px; font: inherit; color: var(--ink); background: var(--bg); border: 1px solid var(--line); border-radius: 8px; }
  .star { display: inline-flex; align-items: center; gap: 4px; flex: none; min-height: 42px; padding: 0 10px; border: 1px solid var(--line); border-radius: 8px; background: none; color: var(--muted); font: inherit; font-size: 0.875rem; cursor: pointer; }
  .star:hover:not(:disabled) { color: var(--ink); background: var(--tint); }
  .star.on { color: var(--accent-text); border-color: color-mix(in srgb, var(--accent) 35%, transparent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
  .star:disabled { opacity: .5; cursor: default; }
  .star:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .hint { margin: 0; color: var(--muted); font-size: 0.875rem; line-height: 1.5; }
  .reason { margin: 0; color: var(--bad); font-size: 0.875rem; line-height: 1.5; }
  @media (max-width: 560px) { select { font-size: 16px; } }
</style>
