<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { decideNewSkills } from '../api'
  import type { SessionSkill } from './skills'

  // Keep one skill New on this machine: reet keeps this machine's copy and puts it on every machine. Names are untrusted
  // text from a machine and are only ever rendered as text. Archive, and a choice between differing copies, are on the
  // Skills page's review, which the panel links to.
  let { skill, host, agentId, onclose }: {
    skill: SessionSkill
    host: string
    agentId: string
    onclose: (kept: boolean, note?: string) => void
  } = $props()

  let busy = $state(false)
  let problem = $state('')
  let root = $state<HTMLElement>()
  let alive = true

  async function focusPrimary() {
    await tick()
    if (!alive) return
    ;(root?.querySelector<HTMLElement>('[data-primary]') ?? root)?.focus()
  }

  async function keep() {
    if (busy) return
    busy = true
    problem = ''
    try {
      await decideNewSkills([skill.name], 'keep', agentId || undefined)
      if (!alive) return
      onclose(true, `Kept ${skill.name} · going on every machine`)
    } catch (e) {
      if (!alive) return
      problem = e instanceof Error && e.message ? e.message : `${skill.name} couldn't be kept just now. Try again.`
      void focusPrimary()
    } finally {
      busy = false
    }
  }

  onMount(() => {
    void focusPrimary()
    return () => { alive = false }
  })
</script>

<section class="keep" bind:this={root} tabindex="-1" aria-label={`Keep ${skill.name}`}>
  <h3>Keep {skill.name}</h3>
  <p>reet keeps {host}'s copy of {skill.name} in your library and puts it on every machine.</p>
  <p class="status" role="status">{problem}</p>
  <div class="actions">
    <button type="button" class="primary" data-primary disabled={busy} onclick={keep}>{busy ? 'Keeping…' : 'Keep'}</button>
    <button type="button" disabled={busy} onclick={() => onclose(false)}>Cancel</button>
  </div>
  <p><a href="/skills?review=new" target="_blank" rel="noopener">Review new skills on the Skills page</a></p>
</section>

<style>
  .keep { display: flex; flex-direction: column; gap: 10px; padding: 8px 10px; min-width: 0; overflow-wrap: anywhere; }
  .keep:focus { outline: none; }
  h3 { margin: 0; font: 600 1rem/1.3 var(--serif); }
  p { margin: 0; font-size: 14px; line-height: 1.45; }
  .status { min-height: 1.45em; color: var(--ink); font-size: 14px; }
  .status:empty { min-height: 0; }
  a { color: var(--accent); font-weight: 600; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  button { min-height: 36px; padding: 0 14px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--accent); font: inherit; font-weight: 600; cursor: pointer; }
  button.primary { background: var(--accent); border-color: var(--accent); color: var(--bg); }
  button:disabled { opacity: 0.55; cursor: default; }
  button:focus-visible, a:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (pointer: coarse) {
    button { min-height: 44px; }
    a { display: inline-flex; align-items: center; min-height: 44px; }
  }
</style>
