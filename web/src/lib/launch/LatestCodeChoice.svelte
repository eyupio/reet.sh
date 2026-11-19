<script module lang="ts">
  export type LatestChoice = 'github' | 'machine'
</script>

<script lang="ts">
  import { onDestroy } from 'svelte'
  import { checkBase } from '../terminal/api'
  import type { BaseState } from '../baseNotice'

  // Where a fresh worktree's code comes from, chosen for one start. GitHub's latest is the commit the repository's base
  // branch has on GitHub; this machine's copy is whatever its local branch holds, which may be behind, ahead of or apart
  // from GitHub. The choice is a fact of this start only: the default lives with the project, else the account.
  //
  // The machine is asked once, without starting anything, where a worktree would start from (the same read the base
  // confirmation uses). A machine too old to answer, a busy one, or one with GitHub out of reach says nothing and the
  // choice still works: it is a courtesy, never a gate.
  let { agentId, host, dir, base = '', value, onchange, disabled = false }: {
    agentId: string
    host: string
    dir: string
    base?: string
    value: LatestChoice
    onchange: (choice: LatestChoice) => void
    disabled?: boolean
  } = $props()

  let baseState = $state<BaseState | null>(null)
  let reading = $state(false)
  let unreachable = $state(false)
  const life = new AbortController()
  let key = $state('')
  let seq = 0
  onDestroy(() => life.abort())

  const worth = (relation: string | undefined) => ['behind', 'ahead', 'diverged', 'fetch_failed', 'same', 'no_local', 'no_remote'].includes(String(relation ?? ''))

  async function read() {
    const next = [agentId, dir, base].join('\u0000')
    if (!agentId || !dir || next === key) return
    key = next
    baseState = null
    unreachable = false
    reading = true
    const mine = ++seq
    try {
      const answer = await checkBase(agentId, dir, base || undefined, life.signal)
      if (mine === seq) baseState = answer
    } catch {
      if (mine === seq) unreachable = true
    } finally {
      if (mine === seq) reading = false
    }
  }

  $effect(() => {
    // Re-read whenever the machine, folder or base changes. The read is read-only and safe to repeat.
    void [agentId, dir, base]
    void read()
  })

  const name = $derived(baseState?.base || base || 'the base branch')
  const relation = $derived(String(baseState?.relation ?? ''))
  const ahead = $derived(relation === 'ahead' || relation === 'diverged')
  const behind = $derived(relation === 'behind')
  const same = $derived(relation === 'same')
  const failed = $derived(relation === 'fetch_failed')

  // One line of plain fact under the choice, in words a person can act on.
  const line = $derived.by(() => {
    if (reading) return `Asking ${host || 'the machine'} where ${name} stands against GitHub…`
    if (unreachable || !worth(relation)) return 'This machine can’t check GitHub just now. Either choice still starts.'
    if (failed) return `Couldn’t reach GitHub from ${host || 'this machine'}, so its copy of ${name} may be out of date.`
    if (same) return `${name} on ${host || 'this machine'} is in step with GitHub.`
    if (behind) return `${name} on ${host || 'this machine'} is behind GitHub. GitHub’s latest has more.`
    if (ahead) return `${name} on ${host || 'this machine'} has commits GitHub doesn’t have. Starting from GitHub’s latest would leave them out.`
    if (relation === 'no_local') return `This machine has no local ${name}; either choice starts from GitHub.`
    return `This machine has no GitHub remote for ${name}. Either choice starts from its own copy.`
  })
  const tone = $derived(reading ? 'reading' : (unreachable || failed) ? 'warn' : ahead ? 'warn' : same ? 'good' : 'plain')
</script>

<fieldset class="latest" {disabled}>
  <legend>Code for this worktree</legend>
  <div class="choice" role="group" aria-label="Where a fresh worktree starts from">
    <button type="button" class:on={value === 'github'} aria-pressed={value === 'github'} onclick={() => onchange('github')} disabled={disabled}>
      <strong>GitHub’s latest</strong>
      <small>Start from what GitHub has now</small>
    </button>
    <button type="button" class:on={value === 'machine'} aria-pressed={value === 'machine'} onclick={() => onchange('machine')} disabled={disabled}>
      <strong>This machine’s copy</strong>
      <small>Keep local commits if it has any</small>
    </button>
  </div>
  <p class="fact {tone}" aria-live="polite">
    <span class="dot" aria-hidden="true"></span>{line}
    {#if !reading && (unreachable || worth(relation))}
      <button type="button" class="again" onclick={() => { key = ''; void read() }} disabled={disabled}>Check again</button>
    {/if}
  </p>
</fieldset>

<style>
  .latest{border:0;margin:0;padding:0;min-width:0}
  legend{padding:0;margin:0 0 8px;font-size:14px;font-weight:600;color:var(--ink)}
  .choice{display:grid;grid-template-columns:1fr 1fr;gap:8px}
  .choice button{display:grid;gap:2px;text-align:left;padding:10px 12px;border:1px solid var(--line);border-radius:10px;background:var(--surface);color:var(--ink);cursor:pointer;transition:border-color .15s ease,background .15s ease,box-shadow .15s ease}
  .choice button:hover:not(:disabled){border-color:var(--ink-soft)}
  .choice button:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .choice button.on{border-color:var(--accent);background:color-mix(in oklab,var(--accent) 8%,var(--surface));box-shadow:inset 0 0 0 1px var(--accent)}
  .choice button:disabled{opacity:.6;cursor:not-allowed}
  .choice strong{font-size:14px;font-weight:600}
  .choice small{font-size:14px;color:var(--muted);line-height:1.35}
  .fact{display:flex;align-items:baseline;gap:8px;flex-wrap:wrap;margin:9px 0 0;font-size:14px;line-height:1.45;color:var(--ink-soft)}
  .dot{flex:none;width:7px;height:7px;border-radius:50%;background:var(--muted);translate:0 -2px}
  .fact.good .dot{background:var(--good)}
  .fact.warn .dot{background:var(--warn)}
  .fact.reading .dot{background:var(--accent);animation:pulse 1.1s ease-in-out infinite}
  @keyframes pulse{0%,100%{opacity:.35;transform:scale(.8)}50%{opacity:1;transform:scale(1.15)}}
  .again{flex:none;border:0;background:none;padding:0;color:var(--muted);font:inherit;font-size:14px;text-decoration:underline;text-underline-offset:2px;cursor:pointer}
  .again:hover{color:var(--ink)}
  @media (max-width:520px){.choice{grid-template-columns:1fr}}
  @media (prefers-reduced-motion:reduce){.fact.reading .dot{animation:none}}
</style>
