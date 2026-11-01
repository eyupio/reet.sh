<script lang="ts">
  import { onMount } from 'svelte'
  import { shell } from '../dialogShell'
  import { PROJECT_COLOURS, projectColour, projectColourHex, type ProjectColour } from '../projectColour'
  import type { ProjectGroup } from '../terminal/api'

  let { group, busy = false, onsave, onclose }: {
    group?: ProjectGroup
    busy?: boolean
    onsave: (name: string, colour: ProjectColour) => void
    onclose: () => void
  } = $props()

  const titleId = $props.id()
  let dialog = $state<HTMLDialogElement>(), input = $state<HTMLInputElement>()
  // svelte-ignore state_referenced_locally
  let name = $state(group?.name ?? '')
  // svelte-ignore state_referenced_locally
  let colour = $state<ProjectColour>(projectColour(group?.colour))
  const error = $derived(name.trim() ? '' : 'Give the group a name.')
  const valid = $derived(name.trim().length > 0 && Array.from(name.trim()).length <= 80)

  onMount(() => {
    dialog?.showModal()
    input?.focus()
    input?.select()
    return () => dialog?.close()
  })

  function close() { dialog?.close(); onclose() }
  function save() {
    if (!valid || busy) return
    onsave(name.trim(), colour)
  }
</script>

<dialog bind:this={dialog} use:shell={{ sheet: 'bottom' }} aria-labelledby={titleId} oncancel={(e) => { e.preventDefault(); if (!busy) close() }}>
  <div class="box" data-shell-card>
    <header>
      <span class="mark" style={`--project-colour:${projectColourHex(colour)}`} aria-hidden="true"></span>
      <div>
        <h2 id={titleId}>{group ? 'Rename group' : 'New group'}</h2>
        <p>Groups keep projects together in the rail. They don't change a project.</p>
      </div>
    </header>
    <label class="field">
      <span>Group name</span>
      <input bind:this={input} bind:value={name} maxlength="80" autocomplete="off" placeholder="e.g. Client work" aria-invalid={name.length > 0 && !valid} onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); save() } }} />
    </label>
    {#if error && name.length > 0}<p class="hint bad">{error}</p>{:else}<p class="hint">{Array.from(name.trim()).length}/80</p>{/if}
    <fieldset class="colours">
      <legend>Colour</legend>
      <div>
        {#each PROJECT_COLOURS as choice}
          <button type="button" role="radio" aria-checked={colour === choice.id} aria-label={choice.name} title={choice.name}
            class:on={colour === choice.id} style={`--swatch:${choice.hex}`} onclick={() => (colour = choice.id)}>{choice.id === '' ? '×' : ''}</button>
        {/each}
      </div>
    </fieldset>
    <div class="actions" data-shell-foot>
      <button type="button" class="btn quiet" disabled={busy} onclick={close}>Cancel</button>
      <button type="button" class="btn primary" disabled={!valid || busy} onclick={save}>{busy ? 'Saving…' : group ? 'Save group' : 'Create group'}</button>
    </div>
  </div>
</dialog>

<style>
  dialog { max-width: min(430px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow-y: auto; padding: 0; border: 0; background: transparent; color: var(--ink); }
  dialog::backdrop { background: var(--scrim); backdrop-filter: blur(2px); }
  .box { padding: 26px; background: var(--card); border: 1px solid var(--line); border-radius: 14px; box-shadow: 0 28px 80px -24px var(--scrim); }
  header { display: flex; gap: 13px; align-items: flex-start; }
  .mark { flex: none; width: 9px; height: 34px; border-radius: 99px; background: var(--project-colour, var(--accent)); margin-top: 2px; }
  h2 { margin: 0; font: 600 1.35rem/1.2 var(--serif); letter-spacing: -0.02em; }
  header p { margin: 6px 0 0; color: var(--muted); font-size: 13px; line-height: 1.5; }
  .field { display: flex; flex-direction: column; gap: 7px; margin-top: 22px; }
  .field > span { font-size: 11px; font-weight: 650; letter-spacing: .08em; text-transform: uppercase; color: var(--muted); }
  input { width: 100%; box-sizing: border-box; min-height: 42px; padding: 9px 12px; border: 1px solid var(--line-mid); border-radius: 9px; background: var(--bg); color: var(--ink); font: inherit; font-size: 14px; }
  input:focus { outline: 2px solid var(--accent); outline-offset: 1px; border-color: transparent; }
  input[aria-invalid='true'] { border-color: var(--bad); }
  .hint { margin: 6px 0 0; font-size: 11px; color: var(--muted); text-align: right; }
  .hint.bad { color: var(--bad); text-align: left; }
  .colours { border: 0; padding: 0; margin: 20px 0 0; }
  legend { font-size: 11px; font-weight: 650; letter-spacing: .08em; text-transform: uppercase; color: var(--muted); padding: 0; margin-bottom: 9px; }
  .colours div { display: flex; gap: 9px; }
  .colours button { width: 28px; height: 28px; padding: 0; border: 1px solid var(--line-strong); border-radius: 50%; background: var(--swatch); color: var(--ink); font-size: 13px; line-height: 1; cursor: pointer; }
  .colours button.on { box-shadow: 0 0 0 2px var(--card), 0 0 0 4px var(--accent); }
  .colours button:first-child { color: var(--muted); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 24px; }
  @media (max-width: 560px) { .box { padding: 20px; } .actions { flex-direction: column-reverse; } .actions button { width: 100%; } }
  @media (prefers-reduced-motion: no-preference) { .box { animation: rise 180ms var(--ease-out) both; } @keyframes rise { from { opacity: 0; transform: translateY(6px) scale(.985); } to { opacity: 1; transform: none; } } }
</style>
