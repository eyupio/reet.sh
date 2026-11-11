<script lang="ts">
  // The person's own words to a pipeline: an instruction for its working agent, or the reason a stage is sent back.
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'

  let { title, intro, label, placeholder = '', confirmLabel, stages = [], busy = false, oncancel, onconfirm }: {
    title: string; intro: string; label: string; placeholder?: string; confirmLabel: string
    /** For sending back: the stages that can be done again, the one in hand last. */
    stages?: { kind: string; label: string }[]
    busy?: boolean; oncancel: () => void; onconfirm: (text: string, stage: string) => void
  } = $props()
  const formId = $props.id()
  let text = $state(''), stage = $state('')
  $effect(() => { if (stages.length && !stages.some(item => item.kind === stage)) stage = stages[stages.length - 1].kind })
  const bytes = $derived(new TextEncoder().encode(text).length)
  const limit = $derived(stages.length ? 2000 : 8000)
  const blocked = $derived(!text.trim() || bytes > limit)
</script>

<WorkspaceDialog {title} onclose={oncancel}>
  <form class="note" onsubmit={event => { event.preventDefault(); if (!blocked && !busy) onconfirm(text.trim(), stage) }}>
    <p>{intro}</p>
    {#if stages.length}<label>Stage to do again<select aria-label="Stage to do again" bind:value={stage} disabled={busy}>
      {#each stages as item (item.kind)}<option value={item.kind}>{item.label}</option>{/each}
    </select></label>{/if}
    <label for={`${formId}-text`}>{label}</label>
    <textarea id={`${formId}-text`} bind:value={text} rows="5" {placeholder} disabled={busy}></textarea>
    {#if bytes > limit}<p class="warning" role="status">Keep it within {limit.toLocaleString('en-GB')} UTF-8 bytes.</p>{/if}
    <div class="actions"><button class="btn primary" type="submit" disabled={busy || blocked}>{confirmLabel}</button><button class="btn quiet" type="button" disabled={busy} onclick={oncancel}>Cancel</button></div>
  </form>
</WorkspaceDialog>

<style>
  .note{display:grid;gap:12px;min-width:0;font-size:14px;line-height:1.5}p{margin:0;overflow-wrap:anywhere}.warning{color:var(--bad)}
  label{font-weight:600;display:grid;gap:6px;min-width:0}
  textarea,select{font:inherit;font-weight:400;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:8px;padding:10px 12px;min-width:0;width:100%;min-height:44px;box-sizing:border-box}textarea{resize:vertical}
  .actions{display:flex;flex-wrap:wrap;gap:8px}.btn{min-height:44px}
  button:focus-visible,select:focus-visible,textarea:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
</style>
