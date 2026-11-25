<script lang="ts">
  import { onMount } from 'svelte'
  import { listGafferPathways, UnauthorizedError } from '../../api'
  import PageHeader from '../../PageHeader.svelte'
  import Skeleton from '../../Skeleton.svelte'
  import type { Pathway } from '../pathways'
  import PathwayEditor from './PathwayEditor.svelte'
  import PathwayList from './PathwayList.svelte'

  // Factory › Pathways: /factory/pathways lists the person's pathways, /factory/pathways/new and /factory/pathways/<id>
  // open the editor. Moving between them stays in the page (history.pushState), and browser back and forward follow.
  let { onsignedout }: { userId: number; onsignedout: (e: unknown) => void } = $props()
  // As the server holds it (internal/store MaxGafferPathways).
  const MAX_PATHWAYS = 50

  type Route = { kind: 'list' } | { kind: 'new' } | { kind: 'edit'; id: string }
  function parse(): Route {
    const m = location.pathname.match(/^\/factory\/pathways\/([^/]+)\/?$/)
    if (!m) return { kind: 'list' }
    if (m[1] === 'new') return { kind: 'new' }
    try { return { kind: 'edit', id: decodeURIComponent(m[1]) } } catch { return { kind: 'list' } }
  }
  let route = $state<Route>(parse())
  // What a new pathway starts from; only New pathway and the empty state's two buttons choose it.
  let start = $state<'blank' | 'standard'>('blank')

  let pathways = $state<Pathway[]>([])
  let loaded = $state(false), failed = $state(false), retrying = $state(false)
  const life = new AbortController()
  const atLimit = $derived(pathways.length >= MAX_PATHWAYS)

  let reads = 0
  async function load() {
    const mine = ++reads
    try {
      const read = await listGafferPathways(AbortSignal.any([life.signal, AbortSignal.timeout(15_000)]))
      if (mine !== reads) return
      pathways = read; loaded = true; failed = false
    } catch (e) {
      if (life.signal.aborted || mine !== reads) return
      if (e instanceof UnauthorizedError) { onsignedout(e); return }
      failed = true
    }
  }
  async function retry() { retrying = true; await load(); retrying = false }

  onMount(() => {
    if (route.kind === 'list') void load()
    const back = () => { route = parse(); if (route.kind === 'list') void load() }
    addEventListener('popstate', back)
    return () => { removeEventListener('popstate', back); life.abort() }
  })

  function go(next: Route) {
    const path = next.kind === 'list' ? '/factory/pathways' : `/factory/pathways/${next.kind === 'new' ? 'new' : encodeURIComponent(next.id)}`
    history.pushState(null, '', path)
    route = next
    scrollTo({ top: 0 })
    if (next.kind === 'list') void load()
  }
  const open = (id: string) => go({ kind: 'edit', id })
  function create(from: 'blank' | 'standard') { start = from; go({ kind: 'new' }) }
</script>

{#if route.kind === 'list'}
  <div class="pathways-page">
    <PageHeader group="Factory" groupHref="/factory" title="Pathways" lede="Your own lines of stages, each with its model, skills and prompt. Pick one when you start work, or let Gaffer suggest one.">
      {#snippet actions()}
        <!-- With nothing saved, the empty state offers its own two ways to start. -->
        {#if !loaded || pathways.length}
          <button class="btn primary" type="button" disabled={!loaded && !failed || atLimit} aria-describedby={atLimit ? 'pathways-limit' : undefined} onclick={() => create('blank')}>New pathway</button>
        {/if}
      {/snippet}
      {#snippet meta()}
        {#if atLimit}<p class="limit" id="pathways-limit">You have {MAX_PATHWAYS} pathways, the most you can keep. Delete one to make another.</p>{/if}
      {/snippet}
    </PageHeader>
    {#if loaded}
      <PathwayList {pathways} onopen={open} onnew={create} onchange={load} />
    {:else if failed}
      <div class="failed" role="alert">
        <p><strong>Your pathways could not be read.</strong> Nothing is shown because nothing was read, not because there is nothing.</p>
        <button class="btn" type="button" disabled={retrying} aria-busy={retrying} onclick={retry}>Retry</button>
      </div>
    {:else}
      <Skeleton label="Reading your pathways…" shape="rows" count={3} />
    {/if}
  </div>
{:else}
  <PathwayEditor id={route.kind === 'edit' ? route.id : null} {start} onclose={() => go({ kind: 'list' })} onsaved={p => open(p.id)} />
{/if}

<style>
  /* The shell gives this route the whole width (as it does the Factory); the page keeps a reading measure of its own. */
  .pathways-page { width: 100%; max-width: 920px; margin: 0 auto; padding: 36px 40px calc(56px + var(--bottom-chrome, 0px)); box-sizing: border-box; }
  .limit { margin: 0; color: var(--muted); }
  .failed { display: flex; align-items: center; justify-content: space-between; gap: 12px 16px; flex-wrap: wrap; padding: 14px 16px; border: 1px solid var(--bad); border-radius: var(--radius); background: var(--card); }
  .failed p { flex: 1 1 280px; margin: 0; color: var(--muted); }
  .failed strong { color: var(--bad); font-weight: 600; }
  @media (max-width: 1000px) { .pathways-page { padding: 24px 24px calc(40px + var(--bottom-chrome, 0px)); } }
  @media (max-width: 560px) { .pathways-page { padding: 20px 16px calc(36px + var(--bottom-chrome, 0px)); } }
</style>
