<script lang="ts">
 import type { ContentReceipt } from './api'
 import { OWNERSHIP_SENTENCE } from './contentReview'

 // The ownership records for this folder, each with Release ownership. A record that can no longer be read shows its
 // name and the release only: nothing about what it claimed. Presentational: the shell asks the question and sends.
 let { receipts, down, busy, revisionOf, onrelease, onretry }: {
  receipts: ContentReceipt[]
  down: boolean
  busy: boolean
  revisionOf: (r: ContentReceipt) => string
  onrelease: (r: ContentReceipt) => Promise<void>
  onretry: () => void
 } = $props()
 const uid = $props.id()
 let heading = $state<HTMLElement>()
 const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)
 const when = (ms: number) => new Date(ms).toLocaleString()
 /** Focus has nowhere to stay once the button that was pressed is gone: the shell puts it here. */
 export function focus(): boolean { heading?.focus(); return !!heading }
</script>

{#if down || receipts.length}
 <section class="records" aria-labelledby={`${uid}-h`}>
  <h3 id={`${uid}-h`} tabindex="-1" bind:this={heading}>Ownership records in this folder</h3>
  {#if down}
   <p class="hint">Ownership records couldn't be read right now.</p>
   <p><button type="button" class="btn" disabled={busy} onclick={onretry}>Try again</button></p>
  {:else}
   <ul aria-labelledby={`${uid}-h`}>
    {#each receipts as r (r.id)}
     <li>
      <div class="what">
       <strong>{r.name}</strong>
       {#if r.damaged}
        <span>This record can't be read. It can still be released.</span>
       {:else}
        <span>{cap(revisionOf(r))} · {r.fileCount} {r.fileCount === 1 ? 'file' : 'files'} · Recorded {when(r.createdAt)}</span>
        <small>{OWNERSHIP_SENTENCE}</small>
       {/if}
      </div>
      <button type="button" class="btn" disabled={busy} aria-label={`Release ownership of ${r.name}`} onclick={() => void onrelease(r)}>Release ownership</button>
     </li>
    {/each}
   </ul>
  {/if}
 </section>
{/if}

<style>
 .records { margin: 22px 0 0; padding: 14px 0 0; border-top: 1px solid var(--line); }
 h3 { margin: 0 0 8px; font-size: 1rem; }
 h3:focus { outline: none; }
 h3:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }
 .hint { color: var(--muted); font-size: .8rem; margin: 6px 0; }
 ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
 li { display: flex; flex-wrap: wrap; gap: 8px 12px; align-items: center; justify-content: space-between; padding: 10px 12px; border: 1px solid var(--line); border-radius: 8px; }
 .what { display: grid; gap: 2px; min-width: 0; flex: 1 1 220px; font-size: .85rem; overflow-wrap: anywhere; }
 small { color: var(--muted); font-size: .75rem; }
</style>
