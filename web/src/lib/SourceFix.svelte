<script lang="ts">
  import { getSyncState, resolveSyncSourceConflicts, startSyncRun, type SyncSourceFix, type SyncState } from './api'

  // Auto fix for a machine that already has a skill from somewhere else, offered wherever that failure is shown. It
  // keeps the copy on the machine and points the sync plan at it, then syncs that folder again. Nothing on the machine
  // is changed. The first press only reads what would change; the second does it.
  let { targetId, agentId, host, online = true, onupdate }: {
    targetId: string
    agentId: string
    host: string
    /** The machine is reporting, so a sync can start as soon as the plan is fixed. */
    online?: boolean
    onupdate?: (state: SyncState) => void
  } = $props()

  let busy = $state(false)
  let plan = $state<SyncSourceFix[] | null>(null)
  let note = $state('')

  async function prepare() {
    if (busy) return
    busy = true; note = ''
    try {
      const preview = await resolveSyncSourceConflicts([targetId], false)
      if (preview.changes.length) plan = preview.changes
      else note = 'Auto fix cannot settle this one: reet holds this skill as its own copy, so there is no other source to follow. Update reet on the machine and sync again (it then skips only this skill), or remove the copy on the machine to use reet’s.'
    } catch (e) { note = e instanceof Error ? e.message : String(e) } finally { busy = false }
  }
  async function apply() {
    if (busy) return
    busy = true
    try {
      await resolveSyncSourceConflicts([targetId], true)
      const next = online ? await startSyncRun(agentId, [targetId]) : await getSyncState()
      plan = null
      note = online ? 'Fixed. Syncing again now.' : `Fixed. ${host} will sync when it is back online.`
      onupdate?.(next)
    } catch (e) { note = e instanceof Error ? e.message : String(e) } finally { busy = false }
  }
</script>

<span class="source-fix" data-source-fix={host}>
  {#if plan}
    <span role="group" aria-label="Confirm auto fix" class="confirm">
      <span>Keep the copy of {plan.map(change => change.name).join(', ')} already on {host} and sync again? Nothing on the machine is changed.</span>
      <span class="actions">
        <button class="btn primary" type="button" disabled={busy} aria-busy={busy} onclick={() => void apply()}>{busy ? 'Fixing…' : 'Auto fix & sync'}</button>
        <button class="btn quiet" type="button" disabled={busy} onclick={() => (plan = null)}>Cancel</button>
      </span>
    </span>
  {:else}
    <button class="btn primary" type="button" disabled={busy} aria-busy={busy} onclick={() => void prepare()}>{busy ? 'Checking…' : 'Auto fix'}</button>
  {/if}
  {#if note}<span role="status" class="note">{note}</span>{/if}
</span>

<style>
  .source-fix { display: inline-grid; gap: 6px; justify-items: start; min-width: 0; }
  .confirm { display: grid; gap: 6px; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .btn { min-height: 44px; }
  .note { color: var(--muted); }
</style>
