<script lang="ts">
  // The agent's own approval mode for a chat that offers one (Codex: read-only and agent). The wording says what each mode
  // means for the person, because "agent" never asks and a refused request is silent. The mode is shown only as the machine last
  // confirmed it; after a switch the machine did not confirm, no mode is shown as active and the person is told to choose one.
  import { yoloModeMeaning } from '../launch/yoloWording'
  let { tool, modes, current, disabled = false, yolo = false, onchoose }: {
    tool: string
    modes: string[]
    /** The mode the machine last confirmed, or '' when it is not known. */
    current: string
    disabled?: boolean
    /** YOLO answers the requests Codex asks for in the sandbox mode, so that mode cannot promise that the person is asked. In the agent mode it answers none. */
    yolo?: boolean
    onchoose: (mode: string) => Promise<void>
  } = $props()

  const LABELS: Record<string, { label: string; meaning: (tool: string) => string }> = {
    'read-only': { label: 'Ask before leaving the sandbox', meaning: t => `${t} runs commands inside its sandbox and asks before one leaves it.` },
    agent: { label: 'Let Codex decide', meaning: t => `${t} decides for itself what needs your approval. You may not be asked.` },
    act: { label: 'Make changes', meaning: t => `${t} explores and changes the project, and asks before each command or edit.` },
    plan: { label: 'Plan only', meaning: t => `${t} explores and plans without changing files.` }
  }
  const labelOf = (mode: string) => LABELS[mode]?.label ?? mode
  const meaning = $derived((yolo && yoloModeMeaning(current, tool)) || (current ? LABELS[current]?.meaning(tool) ?? `${tool} is in ${current} mode.` : `${tool}’s mode isn’t confirmed. Choose one before you send anything else.`))
  let busy = $state(''), problem = $state('')
  const groupId = $props.id()

  async function choose(mode: string) {
    if (busy || disabled || mode === current) return
    busy = mode; problem = ''
    try { await onchoose(mode) }
    catch (e) { problem = e instanceof Error ? e.message : 'The mode could not be changed. Try again.' }
    finally { busy = '' }
  }
</script>

<div class="mode-control" role="radiogroup" aria-labelledby={`${groupId}-label`} aria-describedby={`${groupId}-meaning`}>
  <span class="label" id={`${groupId}-label`}>Approvals</span>
  <div class="options">
    {#each modes as mode (mode)}
      <button type="button" role="radio" aria-checked={current === mode} class="option" class:on={current === mode} disabled={disabled || !!busy} onclick={() => void choose(mode)}>{busy === mode ? 'Changing…' : labelOf(mode)}</button>
    {/each}
  </div>
  <p class="meaning" id={`${groupId}-meaning`} role="status">{meaning}</p>
  {#if problem}<p class="problem" role="alert">{problem}</p>{/if}
</div>

<style>
  .mode-control { display: grid; gap: 4px; min-width: 0; }
  .label { font: 600 14px var(--sans); letter-spacing: .09em; text-transform: uppercase; color: var(--faint); }
  .options { display: flex; flex-wrap: wrap; gap: 6px; }
  .option { min-height: 32px; padding: 0 10px; border: 1px solid var(--line); border-radius: 7px; background: transparent; color: var(--ink); font: 500 14px/1 var(--sans); cursor: pointer; }
  .option.on { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 14%, transparent); }
  .option:disabled { opacity: .55; cursor: default; }
  .meaning { margin: 0; font-size: 14px; color: var(--muted); overflow-wrap: anywhere; }
  .problem { margin: 0; font-size: 14px; color: var(--bad); overflow-wrap: anywhere; }
  @media (pointer: coarse) { .option { min-height: 44px; } }
</style>
