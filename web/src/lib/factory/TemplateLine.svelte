<script lang="ts">
  // Says which template and version an outcome was started from. The outcome keeps everything it ran on, so the template
  // may since have been deleted, or the version dropped; the line says so instead of linking to nothing. The name is
  // the one the outcome recorded at its launch, else the template's as read now (older outcomes recorded none).
  import { onMount } from 'svelte'
  import { ApiError, getGafferTemplate } from '../api'

  let { id, version, name: recorded = '', onopen }: { id: string; version: number; name?: string; onopen: (id: string) => void } = $props()
  // Not read yet, read and kept, the template there but that version not kept, the template gone, or unreadable.
  let seen = $state<{ state: 'reading' | 'kept' | 'dropped' | 'deleted' | 'unknown'; name: string }>({ state: 'reading', name: '' })
  const life = new AbortController()
  onMount(() => {
    void (async () => {
      try { seen = { state: 'kept', name: (await getGafferTemplate(id, version, life.signal)).content.name }; return } catch (e) { if (life.signal.aborted) return; if (!(e instanceof ApiError && e.status === 404)) { seen = { state: 'unknown', name: '' }; return } }
      try { seen = { state: 'dropped', name: (await getGafferTemplate(id, 0, life.signal)).content.name } } catch (e) { if (!life.signal.aborted) seen = e instanceof ApiError && e.status === 404 ? { state: 'deleted', name: '' } : { state: 'unknown', name: '' } }
    })()
    return () => life.abort()
  })
</script>

{#if seen.state !== 'reading'}<p class="from">
  {#if seen.state === 'deleted'}{#if recorded}From template “{recorded}”, version {version}. That template has since been deleted.{:else}From a template that has since been deleted, version {version}.{/if}
  {:else if seen.state === 'unknown'}{recorded ? `From template “${recorded}”, version ${version}.` : `From a template, version ${version}.`}
  {:else}From template “<button class="inline" type="button" onclick={() => onopen(id)}>{recorded || seen.name}</button>”, version {version}{seen.state === 'dropped' ? ', which is no longer kept' : ''}.{/if}
</p>{/if}

<style>
  .from{margin:0;color:var(--muted);font-size:.875rem;overflow-wrap:anywhere}
  .inline{border:0;background:none;padding:0;font:inherit;font-weight:600;color:var(--accent-text);cursor:pointer;min-height:24px;text-decoration:underline}.inline:focus-visible{outline:2px solid var(--accent);outline-offset:2px}@media(pointer:coarse){.inline{min-height:44px}}
</style>
