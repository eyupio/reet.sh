<script lang="ts">
  import { onMount } from 'svelte'
  import type { Machine } from './api'
  import { agentGroups, groupLabel, inventoryGroups, compareInventories } from './machines'
  let { machines, canWrite, onreference }: { machines: Machine[]; canWrite: boolean; onreference: (source: string, target: string) => void } = $props()
  let leftHost = $state(''), rightHost = $state('')
  let leftGroup = $state(''), rightGroup = $state('')
  let now = $state(Date.now())
  onMount(() => { const timer = setInterval(() => now = Date.now(), 30000); return () => clearInterval(timer) })
  const left = $derived(machines.find(m => m.host === leftHost))
  const right = $derived(machines.find(m => m.host === rightHost))
  const a = $derived(inventoryGroups(left, leftGroup)), b = $derived(inventoryGroups(right, rightGroup))
  const diff = $derived(compareInventories(a, b, now))
  const ready = $derived(!!left && !!right && leftHost !== rightHost)
  const canReference = $derived(ready && diff.confirmed && !diff.unknownSources && a.length === 1 && b.length === 1 && ['agents', 'claude', 'cursor', 'opencode'].includes(b[0].agent) && (!b[0].project || b[0].project.startsWith('/')))
</script>

<section class="comparison" aria-labelledby="compare-title">
  <h3 id="compare-title">Compare machines</h3>
  <p>Compare the latest reported inventories, then decide which machine should follow which. Nothing changes while comparing.</p>
  <div class="pickers">
    <div><label>Reference machine <select aria-label="Reference machine" bind:value={leftHost} onchange={() => leftGroup = ''}><option value="">Choose a machine</option>{#each machines as m}<option value={m.host} disabled={m.host === rightHost}>{m.host}</option>{/each}</select></label>
      {#if left}<label>Reference agent group <select aria-label="Reference agent group" bind:value={leftGroup}><option value="">All agent groups (unique skills)</option>{#each agentGroups(left) as g}<option value={g.id}>{groupLabel(g)} · {g.skillCount ?? 0}</option>{/each}</select></label>{/if}
    </div>
    <div><label>Compare with machine <select aria-label="Compare with machine" bind:value={rightHost} onchange={() => rightGroup = ''}><option value="">Choose a machine</option>{#each machines as m}<option value={m.host} disabled={m.host === leftHost}>{m.host}</option>{/each}</select></label>
      {#if right}<label>Target agent group <select aria-label="Target agent group" bind:value={rightGroup}><option value="">All agent groups (unique skills)</option>{#each agentGroups(right) as g}<option value={g.id}>{groupLabel(g)} · {g.skillCount ?? 0}</option>{/each}</select></label>{/if}
    </div>
  </div>
  {#if ready}
    <div class="result" class:attention={!diff.confirmed || diff.unknownSources || diff.leftOnly.length > 0 || diff.rightOnly.length > 0} aria-live="polite">
      <strong>{!diff.confirmed ? 'Not confirmed — a complete, recent scan is needed' : diff.unknownSources ? 'Needs review — some skill sources are unknown' : diff.leftOnly.length || diff.rightOnly.length ? 'Reported inventories differ' : 'Reported skill sets match'}</strong>
      <p>{diff.common.length} shared · {diff.leftOnly.length} only on {leftHost} · {diff.rightOnly.length} only on {rightHost}</p>
      {#if !diff.confirmed}<p>Rescan both machines with <code>reet scan --env MACHINE_NAME --prune</code> and refresh. A server heartbeat does not confirm an agent inventory.</p>{/if}
      {#if diff.unknownSources}<p>Link unknown/local skills to their install sources before applying a sync.</p>{/if}
      <p>Comparison uses skill identity and source, not installed file contents or versions. “All agent groups” compares each machine's combined skill set; choose agent groups to check their placement.</p>
    </div>
    <div class="differences">
      <details open={diff.leftOnly.length > 0}><summary>Only on {leftHost} ({diff.leftOnly.length})</summary><ul>{#each diff.leftOnly as s}<li><strong>{s.name}</strong> <code>{s.source || 'Unknown source'}</code></li>{:else}<li>No unique skills.</li>{/each}</ul></details>
      <details open={diff.rightOnly.length > 0}><summary>Only on {rightHost} ({diff.rightOnly.length})</summary><ul>{#each diff.rightOnly as s}<li><strong>{s.name}</strong> <code>{s.source || 'Unknown source'}</code></li>{:else}<li>No unique skills.</li>{/each}</ul></details>
      <details><summary>Shared skills ({diff.common.length})</summary><ul>{#each diff.common as s}<li>{s.name} <code>{s.source || 'Unknown source'}</code></li>{:else}<li>No shared skills.</li>{/each}</ul></details>
    </div>
    {#if canWrite}<button type="button" class="btn" disabled={!canReference} onclick={() => onreference(a[0].id, b[0].id)}>Use these groups for a sync reference</button><p class="hint">Choose one agent group on each machine to prepare a reference. You will review and save the snapshot below; applying it still requires confirmation on the target machine.</p>{/if}
  {:else}<p class="hint">Select two different machines. Agent installations are grouped inside each machine.</p>{/if}
</section>

<style>
 .comparison { padding:20px; margin:24px 0; border:1px solid var(--line); border-radius:10px; background:var(--bg); }
 h3 { margin:0; } p { color:var(--muted); font-size:.85rem; line-height:1.6; }
 .pickers,.differences { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; }
 label { display:grid; gap:6px; margin:10px 0; font-size:.85rem; font-weight:600; }
 select { width:100%; min-width:0; } .result { padding:14px; margin:16px 0; border:1px solid var(--good); border-radius:8px; } .result p:last-child { margin-bottom:0; } .attention { border-color:var(--accent); }
 details { border-top:1px solid var(--line); padding:12px 0; min-width:0; } summary { cursor:pointer; font-size:.85rem; font-weight:600; overflow-wrap:anywhere; }
 ul { padding-left:20px; max-height:260px; overflow:auto; } li { font-size:.8rem; padding:4px; overflow-wrap:anywhere; } code { color:var(--muted); overflow-wrap:anywhere; }
 .hint { font-size:.8rem; } .btn { margin-top:12px; }
 @media(max-width:600px) { .pickers,.differences { grid-template-columns:1fr; } .comparison { padding:14px; } }
</style>
