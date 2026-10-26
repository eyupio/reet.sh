<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from './dialogShell'
  import AudiencePicker from './AudiencePicker.svelte'
  import { createList, type ListDetail, type Audience } from './api'

  let {
    linkEnabled = true, oncreated, oncancel,
  }: { linkEnabled?: boolean; oncreated: (list: ListDetail) => void; oncancel: () => void } = $props()

  let dialog = $state<HTMLDialogElement>()
  let nameEl = $state<HTMLInputElement>()
  let name = $state('')
  let description = $state('')
  let visibility = $state('personal')
  let audience = $state<Audience>({ people: [], excluded: [], groupRole: 'viewer' })
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
      const list = await createList({ name: name.trim(), description: description.trim(), visibility, audience })
      dialog?.close()
      oncreated(list)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} use:shell aria-labelledby="new-list-title" oncancel={(e) => { e.preventDefault(); cancel() }}>
  <form class="box" data-shell-card onsubmit={submit}>
    <p class="eyebrow" data-shell-aside>Your lists</p>
    <h2 id="new-list-title">Start a list</h2>
    <p class="lede" data-shell-aside>Keep a set of skills together, add a note to each, and choose exactly who can access it.</p>

    <label class="field">
      <span>Name</span>
      <input bind:this={nameEl} bind:value={name} maxlength="80" placeholder="e.g. Frontend launch pack" autocomplete="off" required />
    </label>
    <label class="field">
      <span>Description <small>optional</small></span>
      <input bind:value={description} maxlength="240" placeholder="What is this list for?" />
    </label>

    <AudiencePicker bind:visibility bind:audience {linkEnabled} />

    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" onclick={cancel} disabled={busy}>Cancel</button>
      <button type="submit" class="btn primary" disabled={busy || !name.trim()}>{busy ? 'Saving…' : visibility === 'public' ? 'Publish to public community' : 'Create list'}</button>
    </div>
  </form>
</dialog>

<style>
  dialog { max-width: min(600px, calc(100vw - 32px)); max-height:calc(100dvh - 32px); overflow:auto; padding: 0; border: 0; background: transparent; color: var(--ink); }
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
  .error { margin: 14px 0 0; color: var(--bad); font-size: 0.85rem; }
  .actions { position:sticky; bottom:-28px; background:var(--card); padding:14px 0; display: flex; justify-content: flex-end; gap: 8px; margin-top: 22px; flex-wrap: wrap; }
  @media (max-width: 560px) {
    .box { padding: 20px; border-radius: 14px; }
    .actions { flex-direction: column-reverse; bottom:-20px; }
    .actions button { width: 100%; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .box { animation: rise 200ms var(--ease-out) both; }
    @keyframes rise { from { opacity: 0; transform: scale(0.97); } to { opacity: 1; transform: scale(1); } }
  }
</style>
