<script lang="ts">
  import { onMount } from 'svelte'
  import { createList, type ListDetail } from './api'

  let {
    linkEnabled = true, oncreated, oncancel,
  }: { linkEnabled?: boolean; oncreated: (list: ListDetail) => void; oncancel: () => void } = $props()

  let dialog = $state<HTMLDialogElement>()
  let nameEl = $state<HTMLInputElement>()
  let name = $state('')
  let description = $state('')
  let visibility = $state<'private' | 'members' | 'link'>('members')
  let busy = $state(false)
  let error = $state('')

  onMount(() => {
    dialog?.showModal()
    nameEl?.focus()
    return () => dialog?.close()
  })

  function cancel() {
    if (busy) return
    dialog?.close()
    oncancel()
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!name.trim() || busy) return
    busy = true
    error = ''
    try {
      const list = await createList({ name: name.trim(), description: description.trim(), visibility })
      dialog?.close()
      oncreated(list)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} aria-labelledby="new-list-title" oncancel={(e) => { e.preventDefault(); cancel() }}>
  <form class="box" onsubmit={submit}>
    <p class="eyebrow">Shared lists</p>
    <h2 id="new-list-title">Start a list</h2>
    <p class="lede">Keep a set of skills together, add a note to each, and share it with your team.</p>

    <label class="field">
      <span>Name</span>
      <input bind:this={nameEl} bind:value={name} maxlength="80" placeholder="e.g. Frontend launch pack" autocomplete="off" required />
    </label>
    <label class="field">
      <span>Description <small>optional</small></span>
      <input bind:value={description} maxlength="240" placeholder="What is this list for?" />
    </label>

    <fieldset>
      <legend>Who can see it</legend>
      <label class="choice">
        <input type="radio" bind:group={visibility} value="private" />
        <span><strong>Only people you add</strong><small>Private to the list's members.</small></span>
      </label>
      <label class="choice">
        <input type="radio" bind:group={visibility} value="members" />
        <span><strong>Everyone on this instance</strong><small>Signed-in people can read it; only members can change it.</small></span>
      </label>
      {#if linkEnabled}
        <label class="choice">
          <input type="radio" bind:group={visibility} value="link" />
          <span><strong>Anyone with the link</strong><small>Read-only, no account needed, no member names or emails.</small></span>
        </label>
      {/if}
    </fieldset>
    {#if !linkEnabled}<p class="hint">Read-only links are off. The owner can turn them on under Admin.</p>{/if}

    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <div class="actions">
      <button type="button" class="btn quiet" onclick={cancel} disabled={busy}>Cancel</button>
      <button type="submit" class="btn primary" disabled={busy || !name.trim()}>{busy ? 'Making list…' : 'Make list'}</button>
    </div>
  </form>
</dialog>

<style>
  dialog { max-width: min(500px, calc(100vw - 32px)); padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: rgb(20 18 15 / 0.45); backdrop-filter: blur(2px); }
  .box {
    padding: 28px; background: var(--card); border: 1px solid var(--line); border-radius: var(--radius);
    box-shadow: 0 28px 80px -24px rgb(0 0 0 / 0.45);
  }
  .eyebrow { margin: 0; color: var(--accent); font-size: 0.7rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  h2 { margin: 6px 0 0; font: 500 1.8rem/1.15 var(--serif); letter-spacing: -0.02em; }
  .lede { margin: 8px 0 18px; color: var(--muted); font-size: 0.9rem; }
  .field { display: grid; gap: 6px; margin-top: 12px; font-size: 0.82rem; font-weight: 600; }
  .field small { color: var(--muted); font-weight: 400; }
  .field input { width: 100%; font-weight: 400; }
  fieldset { margin: 18px 0 0; padding: 0; border: 0; }
  legend { padding: 0; margin-bottom: 8px; font-size: 0.82rem; font-weight: 600; }
  .choice {
    display: grid; grid-template-columns: 18px 1fr; gap: 10px; align-items: start;
    padding: 10px 12px; margin-top: 6px; border: 1px solid var(--line); border-radius: 10px; cursor: pointer;
  }
  .choice:has(input:checked) { border-color: var(--accent); background: color-mix(in srgb, var(--tint) 38%, var(--card)); }
  .choice input { margin-top: 3px; accent-color: var(--accent); }
  .choice span { display: grid; gap: 2px; }
  .choice strong { font-size: 0.86rem; }
  .choice small { color: var(--muted); font-size: 0.78rem; line-height: 1.4; }
  .hint { margin: 10px 0 0; color: var(--muted); font-size: 0.78rem; }
  .error { margin: 14px 0 0; color: var(--bad); font-size: 0.85rem; }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 22px; flex-wrap: wrap; }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 14px; }
    .actions { flex-direction: column-reverse; }
    .actions button { width: 100%; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise { from { opacity: 0; transform: scale(0.97); } to { opacity: 1; transform: scale(1); } }
  }
</style>
