<script lang="ts">
  import { queueSyncJob, type SyncState } from './api'
  import { fleetPlan, queueFleet, type FleetPlan } from './fleet'
  import ConfirmDialog from './ConfirmDialog.svelte'
  let { snapshot, canWrite, onupdate }: { snapshot: SyncState; canWrite: boolean; onupdate: (next: SyncState) => void } = $props()
  let pending = $state<{ kind: 'scan' | 'preview'; plan: FleetPlan } | null>(null)
  let busy = $state(false), notice = $state(''), failures = $state<string[]>([])
  const scan = $derived(fleetPlan(snapshot, 'scan'))
  const sync = $derived(fleetPlan(snapshot, 'preview'))
  async function submit() {
    if (!pending || busy) return
    const { plan } = pending
    pending = null; busy = true; notice = ''; failures = []
    const result = await queueFleet(plan, async operation => onupdate(await queueSyncJob(operation.agentId, operation.targetId, operation.kind)))
    notice = `${result.queued.length} of ${plan.operations.length} jobs queued. Machines report results separately; review each sync plan before applying.`
    failures = result.failed; busy = false
  }
</script>
<section class="fleet-actions" aria-label="Fleet controls" aria-busy={busy}>
  <div><strong>Fleet controls</strong><p>Refresh inventories or prepare every saved sync set for review.</p></div>
  {#if canWrite}<div class="buttons"><button class="btn" disabled={busy || !scan.operations.length} onclick={() => pending = { kind: 'scan', plan: scan }}>{busy ? 'Queueing…' : 'Scan all'}</button><button class="btn primary" disabled={busy || !sync.operations.length} onclick={() => pending = { kind: 'preview', plan: sync }}>Sync all · preview</button></div>{/if}
  <p class="hint">{scan.operations.length} {scan.operations.length === 1 ? 'machine' : 'machines'} ready to scan · {sync.operations.length} saved {sync.operations.length === 1 ? 'scope' : 'scopes'} ready to preview. Offline, paused and pending scopes are skipped. <a href="#sync-selection">Choose destinations</a>.</p>
  {#if scan.skipped.length || sync.skipped.length}<details><summary>Skipped operations</summary><ul>{#each [...new Set([...scan.skipped, ...sync.skipped])] as reason}<li>{reason}</li>{/each}</ul></details>{/if}
  {#if notice}<p class="feedback" role="status">{notice}</p>{/if}
  {#if failures.length}<div class="failure" role="alert"><strong>Some jobs could not be queued</strong><ul>{#each failures as failure}<li>{failure}</li>{/each}</ul><p>Refresh status before retrying failed destinations.</p></div>{/if}
</section>
{#if pending}<ConfirmDialog title={pending.kind === 'scan' ? 'Scan all available machines?' : 'Preview sync across your machines?'} body={`${pending.plan.operations.length} jobs will be queued. ${pending.plan.skipped.length} operations are skipped. ${pending.kind === 'scan' ? 'Each available machine will report its installed skills.' : 'Each machine will calculate its exact installs and removals. Installed files change only after you approve an individual plan.'}`} confirmLabel="Queue jobs" onconfirm={() => void submit()} oncancel={() => pending = null}><ul>{#each pending.plan.operations as operation}<li>{operation.label}</li>{/each}</ul></ConfirmDialog>{/if}
<style>
  .fleet-actions{display:flex;align-items:center;flex-wrap:wrap;gap:12px 24px;padding:20px;border:1px solid var(--line);border-radius:12px;background:var(--card);margin:24px 0}.fleet-actions>div:first-child{flex:1}.buttons{display:flex;flex-wrap:wrap;gap:8px}strong{font-size:.87rem}p{color:var(--muted);font-size:.8rem;margin:4px 0 0}.hint,details,.feedback,.failure{flex-basis:100%}.hint{font-size:.75rem}details{font-size:.78rem;color:var(--muted)}summary{cursor:pointer}.failure{color:var(--bad)}li{overflow-wrap:anywhere}ul{max-height:240px;overflow:auto;padding-left:20px}.feedback{color:var(--good)}
  @media(max-width:600px){.buttons{width:100%}.buttons .btn{flex:1}}
</style>
