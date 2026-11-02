<script lang="ts">
  // The states every piece of remote data has, in one place: waiting, failed, empty, and the content. Written in that
  // order, because the first three are most of what a person sees of a slow or absent connection.
  import type { Snippet } from 'svelte'
  import Skeleton from './Skeleton.svelte'

  let {
    loading = false,
    error = '',
    empty = false,
    onretry,
    label = 'Loading…',
    shape = 'lines',
    count = 3,
    height,
    emptyState,
    children,
  }: {
    loading?: boolean
    /** What went wrong, for a person: shown with a Try again when `onretry` is given. */
    error?: string
    /** The request succeeded and returned nothing: `emptyState` is rendered instead of `children`. */
    empty?: boolean
    onretry?: () => void
    label?: string
    shape?: 'lines' | 'rows' | 'cards'
    count?: number
    height?: string
    emptyState?: Snippet
    children: Snippet
  } = $props()
</script>

{#if loading}
  <Skeleton {label} {shape} {count} {height} />
{:else if error}
  <div class="failed" role="alert">
    <p>{error}</p>
    {#if onretry}<button type="button" class="btn" onclick={onretry}>Try again</button>{/if}
  </div>
{:else if empty && emptyState}
  {@render emptyState()}
{:else}
  {@render children()}
{/if}

<style>
  .failed { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 14px 16px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); }
  .failed p { margin: 0; color: var(--muted); }
</style>
