<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import { applyMachineSoftware, fleetSoftwarePlans, type FleetSoftwareRow } from './terminal/api'
  import { fleetSoftwareCanApply, fleetSoftwareLabel, fleetSoftwareWarning } from './software'

  let { onclose }: { onclose: () => void } = $props()
  const id = $props.id()
  let dialog = $state<HTMLDialogElement>()
  let confirmDialog = $state<HTMLDialogElement>()
  let rows = $state<FleetSoftwareRow[]>([])
  let loading = $state(true)
  let error = $state('')
  let applying = $state('')
  let applyError = $state<Record<string, string>>({})
  let notice = $state<Record<string, string>>({})
  let confirm = $state<FleetSoftwareRow | null>(null)

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

  function openApply(row: FleetSoftwareRow) {
    if (!row.action || !fleetSoftwareCanApply(row)) return
    applyError[row.agentId] = ''
    confirm = row
    confirmDialog?.showModal()
  }

  async function applyConfirm() {
    if (!confirm || applying || !fleetSoftwareCanApply(confirm)) return
    const row = confirm
    const action = row.action
    if (!action) return
    applying = row.agentId
    applyError[row.agentId] = ''
    try {
      const answer = await applyMachineSoftware(row.agentId, 'opencode', action)
      notice[row.agentId] = `${answer.result.action === 'install' ? 'Installed' : 'Updated'} OpenCode ${answer.result.version} on ${row.host}.`
      confirmDialog?.close()
      await load()
    } catch (caught) {
      applyError[row.agentId] = caught instanceof Error ? caught.message : "That install didn't finish."
    } finally {
      applying = ''
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
        <p class="lede" data-shell-aside>A plan for each machine, from stored evidence. Apply one machine at a time; rollback and remove stay in a machine's Settings.</p>
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
          {@const canApply = fleetSoftwareCanApply(row)}
          <li data-machine={row.host} data-state={row.state}>
            <div class="copy">
              <strong>{row.host}{#if row.version}<code> {row.version}</code>{/if}</strong>
              <small class="label">{fleetSoftwareLabel(row)}</small>
              <small>{row.sentence}</small>
              {#if warning}<small class="warn">{warning}</small>{/if}
              {#if notice[row.agentId]}<small class="notice-line">{notice[row.agentId]}</small>{/if}
              {#if applyError[row.agentId]}<small class="error">{applyError[row.agentId]}</small>{/if}
              {#if row.source}<small class="source">{row.source}</small>{/if}
            </div>
            {#if row.action && (row.state === 'install' || row.state === 'update')}
              <button
                class="btn apply"
                type="button"
                disabled={!canApply || applying === row.agentId}
                onclick={() => openApply(row)}
                aria-label={`Apply ${row.action} plan on ${row.host}`}
              >
                {applying === row.agentId ? 'Applying…' : 'Apply plan'}
              </button>
            {/if}
          </li>
        {/each}
      </ul>
      <footer>
        <p class="hint">One machine at a time. Rollback and remove live in a machine's Settings.</p>
        <button class="btn" type="button" onclick={() => void load()} disabled={loading}>Refresh</button>
      </footer>
    {/if}
  </div>
</dialog>

<dialog bind:this={confirmDialog} class="confirm" aria-labelledby={`${id}-confirm`} onclose={() => { confirm = null }}>
  <div class="confirm-body">
    <h2 id={`${id}-confirm`}>Apply this {confirm?.action}?</h2>
    {#if confirm}
      <p>{confirm.sentence}</p>
      <dl>
        <dt>Machine</dt><dd>{confirm.host}</dd>
        <dt>Version</dt><dd>{confirm.target || confirm.version}</dd>
        <dt>Source</dt><dd><code>{confirm.source}</code></dd>
      </dl>
      {#if fleetSoftwareWarning(confirm)}<p class="warn">{fleetSoftwareWarning(confirm)}</p>{/if}
      <p class="hint">reet downloads the artifact, verifies its recorded checksum, and only then makes it current. Only this machine's plan runs.</p>
      {#if applyError[confirm.agentId]}<p class="error">{applyError[confirm.agentId]}</p>{/if}
    {/if}
    <div class="confirm-actions">
      <button class="btn" type="button" onclick={() => confirmDialog?.close()} disabled={!!applying}>Cancel</button>
      <button class="btn primary" type="button" onclick={() => void applyConfirm()} disabled={!!applying || !confirm || !fleetSoftwareCanApply(confirm)}>{applying ? 'Applying…' : 'Apply now'}</button>
    </div>
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
  .copy { display: grid; gap: 3px; min-width: 0; flex: 1 1 auto; }
  .copy strong { font-size: 0.9rem; }
  .copy code { font-size: 0.8rem; }
  .copy small { color: var(--muted); font-size: 0.78rem; line-height: 1.5; overflow-wrap: anywhere; }
  .copy .label { color: var(--ink); font-weight: 600; }
  .copy .warn { color: var(--warn); }
  .copy .notice-line { color: var(--good); }
  .copy .source { font: 0.72rem/1.4 var(--mono, monospace); }
  .apply { flex: none; min-height: 36px; padding: 7px 12px; font-size: 0.8rem; }
  footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; padding-top: 14px; border-top: 1px solid var(--line); }
  footer .btn { flex: none; min-height: 36px; padding: 7px 12px; font-size: 0.8rem; }
  .confirm { width: min(520px, calc(100vw - 32px)); border-radius: 16px; }
  .confirm-body { display: grid; gap: 12px; padding: clamp(20px, 4vw, 28px); }
  .confirm h2 { margin: 0; font: 500 clamp(1.3rem, 3vw, 1.7rem)/1.15 var(--serif); letter-spacing: -0.02em; }
  .confirm p { margin: 0; color: var(--ink); font-size: 0.88rem; line-height: 1.55; }
  .confirm .warn { color: var(--warn); }
  .confirm .hint { color: var(--muted); font-size: 0.8rem; }
  .confirm dl { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; margin: 0; font-size: 0.82rem; }
  .confirm dt { color: var(--muted); }
  .confirm dd { margin: 0; overflow-wrap: anywhere; }
  .confirm-actions { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
  .confirm-actions .btn { min-height: 38px; padding: 8px 14px; }
  .confirm-actions .primary { border-color: var(--accent); background: var(--accent); color: var(--accent-ink); }
  @media (max-width: 560px) {
    .rows li { padding: 10px 12px; flex-wrap: wrap; }
    .apply { width: 100%; }
  }
  @media (pointer: coarse) {
    footer .btn, .apply { min-height: 44px; }
  }
</style>
