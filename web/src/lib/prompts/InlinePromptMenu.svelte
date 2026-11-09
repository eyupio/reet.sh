<script lang="ts">
  import { resolvePrompt, searchPrompts, type PromptContext } from '../terminal/prompts'
  import type { TerminalPrompt } from '../terminal/api'
  import { toolName } from '../agentLaunch'

  // The palette above a live composer. It never takes focus: the person keeps typing in the box while
  // the list narrows. The parent forwards arrow keys, Enter and Escape through move(), choose() and dismiss().
  let { prompts, skills, query = '', context = {}, loading = false, error = '', inline = false, oninsert, onclose }: {
    prompts: TerminalPrompt[]
    skills: Record<string, string>
    query?: string
    context?: PromptContext
    loading?: boolean
    error?: string
    /** In-flow below its field (a dialog) instead of floating above a composer. */
    inline?: boolean
    oninsert: (result: { prompt: TerminalPrompt; text: string; unresolved: string[] }) => void
    onclose: () => void
  } = $props()

  const id = $props.id()
  let active = $state(0), listEl = $state<HTMLDivElement>()
  const filtered = $derived(searchPrompts(prompts, query))
  const current = $derived(filtered[Math.min(active, Math.max(0, filtered.length - 1))])
  $effect(() => { if (filtered.length && active >= filtered.length) active = 0 })
  $effect(() => {
    void active
    const row = listEl?.querySelectorAll<HTMLElement>('[role=option]')[Math.min(active, Math.max(0, filtered.length - 1))]
    row?.scrollIntoView({ block: 'nearest' })
  })

  export function move(delta: number) {
    if (!filtered.length) return
    active = (Math.min(active, filtered.length - 1) + delta + filtered.length) % filtered.length
  }

  export function choose() {
    const prompt = filtered[Math.min(active, Math.max(0, filtered.length - 1))]
    if (prompt) pick(prompt)
  }

  export function dismiss() { onclose() }

  function pick(prompt: TerminalPrompt) {
    const resolved = resolvePrompt(prompt.body, context, skills)
    oninsert({ prompt, text: resolved.text, unresolved: resolved.unresolved })
  }
</script>

<div class="menu" class:inline role="dialog" aria-label="Saved prompts">
  <div class="head">
    <span class="title">Prompts</span>
    <a href="/prompts" target="_blank" rel="noopener" onmousedown={(event) => event.preventDefault()}>Manage</a>
  </div>
  {#if loading}
    <p class="note" role="status">Reading your prompts…</p>
  {:else if error}
    <p class="note bad" role="alert">{error}</p>
  {:else if filtered.length === 0}
    <p class="note">{prompts.length ? 'No prompts match.' : 'No prompts yet.'} <a href="/prompts" target="_blank" rel="noopener" onmousedown={(event) => event.preventDefault()}>Write one</a></p>
  {:else}
    <div class="list" id={`${id}-list`} role="listbox" aria-label="Your prompts" bind:this={listEl}>
      {#each filtered as prompt, i (prompt.id)}
        <button type="button" class="option" role="option" tabindex="-1" aria-selected={i === Math.min(active, filtered.length - 1)}
          onmousedown={(event) => event.preventDefault()} onmouseenter={() => (active = i)} onclick={() => pick(prompt)}>
          <span class="option-title">{prompt.title}</span>
          {#if prompt.description}<span class="option-description">{prompt.description}</span>{/if}
          <span class="option-meta">{#if prompt.category}<span class="chip">{prompt.category}</span>{/if}{#if prompt.tool || prompt.model || prompt.effort}<span>{[prompt.tool ? toolName(prompt.tool) : '', prompt.model, prompt.effort].filter(Boolean).join(' · ')}</span>{/if}</span>
        </button>
      {/each}
    </div>
    <p class="sr-only" aria-live="polite">{current?.title ?? ''}</p>
    <p class="foot"><span>{filtered.length} {filtered.length === 1 ? 'prompt' : 'prompts'}</span><span>↑↓ move · ↵ insert · Esc close</span></p>
  {/if}
</div>

<style>
  .menu { position: absolute; z-index: 30; left: 10px; right: 10px; bottom: calc(100% + 8px); display: flex; flex-direction: column; gap: 6px; max-height: min(360px, 48vh); padding: 8px; border: 1px solid var(--line-strong); border-radius: 12px; background: var(--card-raised); box-shadow: 0 16px 40px -18px var(--scrim); overflow: hidden; }
  .menu.inline { position: static; left: auto; right: auto; bottom: auto; max-height: 260px; margin-top: 6px; box-shadow: none; }
  .head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 2px 4px; }
  .title { color: var(--muted); font-size: 13px; font-weight: 600; letter-spacing: .01em; }
  .head a { color: var(--accent-text); font-size: 13px; text-decoration: underline; text-underline-offset: 3px; }
  .list { display: flex; flex-direction: column; gap: 2px; min-height: 0; overflow: auto; }
  .option { display: flex; flex-direction: column; gap: 2px; width: 100%; padding: 7px 9px; border: 0; border-radius: 7px; background: none; color: var(--ink); text-align: left; font: inherit; cursor: pointer; }
  .option:hover, .option[aria-selected='true'] { background: var(--tint); }
  .option-title { font-size: 14px; font-weight: 600; overflow-wrap: anywhere; }
  .option-description { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 1; line-clamp: 1; overflow: hidden; color: var(--muted); font-size: 13px; line-height: 1.4; }
  .option-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 3px 8px; color: var(--muted); font-size: 12px; }
  .chip { padding: 1px 6px; border-radius: 4px; background: var(--tint); }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .foot { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin: 0; padding: 2px 4px 0; border-top: 1px solid var(--line-soft); color: var(--faint); font-size: 12px; }
  .note { margin: 0; padding: 6px 4px; color: var(--muted); font-size: 13px; }
  .note.bad { color: var(--bad); }
  .note a { color: var(--accent-text); text-decoration: underline; text-underline-offset: 3px; }
  @media (max-width: 650px) { .menu { left: 4px; right: 4px; max-height: 42vh; } .option { min-height: 44px; padding: 8px 10px; } }
</style>
