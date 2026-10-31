<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import { fleetSoftwarePlans, type FleetSoftwareRow } from './terminal/api'
  import { fleetSoftwareLabel, fleetSoftwareWarning } from './software'

  let { onclose }: { onclose: () => void } = $props()
  const id = $props.id()
  let dialog = $state<HTMLDialogElement>()
  let rows = $state<FleetSoftwareRow[]>([])
  let loading = $state(true)
  let error = $state('')
  let selected = $state<Record<string, boolean>>({})
  const chosen = $derived(Object.values(selected).filter(Boolean).length)

  async function load() {
    loading = true
    error = ''
    try {
      rows = await fleetSoftwarePlans()
    } catch (caught) {
      error = caught instanceof Error ? caught.message : "Couldn't read the fleet's software plans."
    } finally {
      loading = false
    }
  }

  onMount(() => {
    dialog?.showModal()
    void load()
    return () => {
      if (dialog?.open) dialog.close()
    }
  })
</script>

<dialog bind:this={dialog} use:shell onclose={onclose} aria-labelledby={`${id}-title`}>
  <div class="sheet" data-shell-body>
    <header>
      <div>
        <p class="eyebrow" data-shell-aside>Software</p>
        <h2 id={`${id}-title`}>Fleet software</h2>
        <p class="lede" data-shell-aside>A plan for each machine, from stored evidence. Nothing is applied from this view.</p>
      </div>
      <button class="icon" type="button" onclick={() => dialog?.close()} aria-label="Close fleet software" data-shell-close>×</button>
    </header>

    {#if loading}
      <p role="status">Reading the stored plans…</p>
    {:else if error}
      <p class="error" role="alert">{error}</p>
      <button class="btn" type="button" onclick={() => void load()}>Try again</button>
    {:else if rows.length === 0}
      <p class="hint">No enrolled machines.</p>
    {:else}
      <ul class="rows">
        {#each rows as row (row.agentId)}
          {@const warning = fleetSoftwareWarning(row)}
          <li data-machine={row.host} data-state={row.state}>
            <input
              type="checkbox"
              aria-label={`Select ${row.host}`}
              bind:checked={selected[row.agentId]}
              disabled={row.state === 'none' || row.state === 'unknown'}
            />
            <div class="copy">
              <strong>{row.host}{#if row.version}<code> {row.version}</code>{/if}</strong>
              <small class="label">{fleetSoftwareLabel(row)}</small>
              <small>{row.sentence}</small>
              {#if warning}<small class="warn">{warning}</small>{/if}
              {#if row.source}<small class="source">{row.source}</small>{/if}
            </div>
          </li>
        {/each}
      </ul>
      <footer>
        <p class="hint">{chosen} selected. Applying across machines is not available yet; open a machine's Settings to apply its plan.</p>
        <button class="btn" type="button" onclick={() => void load()} disabled={loading}>Refresh</button>
      </footer>
    {/if}
  </div>
</dialog>

<style>
  dialog { width: min(720px, calc(100vw - 32px)); max-height: min(88vh, 820px); padding: 0; border: 1px solid var(--line); border-radius: 18px; background: var(--card); color: var(--ink); box-shadow: 0 30px 90px color-mix(in srgb, var(--ink) 28%, transparent); }
  dialog::backdrop { background: color-mix(in srgb, var(--ink) 45%, transparent); backdrop-filter: blur(2px); }
  .sheet { display: grid; gap: 18px; padding: clamp(20px, 4vw, 30px); overflow: auto; max-height: inherit; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
  .eyebrow { margin: 0; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.13em; text-transform: uppercase; }
  h2 { margin: 5px 0 0; font: 500 clamp(1.5rem, 3vw, 2rem)/1.1 var(--serif); letter-spacing: -0.025em; }
  .lede { margin: 8px 0 0; max-width: 54ch; color: var(--muted); font-size: 0.86rem; line-height: 1.55; }
  .icon { width: 38px; height: 38px; flex: none; border: 1px solid var(--line); border-radius: 50%; background: var(--bg); color: var(--muted); font-size: 1.25rem; cursor: pointer; }
  .icon:hover { color: var(--ink); border-color: var(--muted); }
  .hint { margin: 0; color: var(--muted); font-size: 0.8rem; line-height: 1.5; }
  .error { margin: 0; color: var(--bad); font-size: 0.86rem; }
  .rows { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .rows li { display: flex; align-items: flex-start; gap: 12px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .rows input { flex: none; width: 18px; height: 18px; margin-top: 3px; accent-color: var(--accent); }
  .copy { display: grid; gap: 3px; min-width: 0; }
  .copy strong { font-size: 0.9rem; }
  .copy code { font-size: 0.8rem; }
  .copy small { color: var(--muted); font-size: 0.78rem; line-height: 1.5; overflow-wrap: anywhere; }
  .copy .label { color: var(--ink); font-weight: 600; }
  .copy .warn { color: var(--warn); }
  .copy .source { font: 0.72rem/1.4 var(--mono, monospace); }
  footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; padding-top: 14px; border-top: 1px solid var(--line); }
  footer .btn { flex: none; min-height: 36px; padding: 7px 12px; font-size: 0.8rem; }
  @media (max-width: 560px) {
    .rows li { padding: 10px 12px; }
  }
  @media (pointer: coarse) {
    footer .btn { min-height: 44px; }
    .rows input { width: 22px; height: 22px; }
  }
</style>
