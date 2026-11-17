<script lang="ts">
  // The Projects button of a tablet or phone: it says which project the page is narrowed to and opens a short list of
  // them, each with what waits and works in it. It only chooses; adding, grouping and editing projects stay on Home.
  import WorkspaceDialog from '../terminal/WorkspaceDialog.svelte'
  type Count = { waiting: number; working: number }
  let { projects, counts, chosen = '', onchoose }: { projects: { id: string; name: string }[]; counts: Map<string, Count>; chosen?: string; onchoose: (id: string) => void } = $props()
  let open = $state(false), opener = $state<HTMLButtonElement>()
  const say = (count: Count | undefined) => `${count?.waiting ?? 0} waiting · ${count?.working ?? 0} working`
  const everything = $derived.by(() => { let waiting = 0, working = 0; for (const c of counts.values()) { waiting += c.waiting; working += c.working } return { waiting, working } })
  const name = $derived(chosen ? projects.find(p => p.id === chosen)?.name ?? 'one project' : 'all')
  function choose(id: string) { open = false; onchoose(id); opener?.focus() }
  function close() { open = false; opener?.focus() }
</script>

<button class="btn pick" type="button" bind:this={opener} aria-haspopup="dialog" onclick={() => (open = true)}>Projects: {name}</button>
{#if open}
  <WorkspaceDialog title="Projects" onclose={close}>
    <ul class="choices">
      <li><button type="button" class="choice" aria-pressed={!chosen} onclick={() => choose('')}><span class="name">All projects</span><span class="says">{say(everything)}</span></button></li>
      {#each projects as p (p.id)}
        <li><button type="button" class="choice" aria-pressed={chosen === p.id} onclick={() => choose(p.id)}><span class="name">{p.name}</span><span class="says">{say(counts.get(p.id))}</span></button></li>
      {/each}
    </ul>
  </WorkspaceDialog>
{/if}

<style>
  .pick{min-height:44px;font-weight:600}
  .choices{list-style:none;margin:0;padding:0;display:grid;gap:8px}
  .choice{display:grid;gap:2px;width:100%;min-height:56px;padding:8px 14px;text-align:left;font:inherit;color:var(--ink);background:var(--card);border:1px solid var(--line);border-radius:10px;cursor:pointer}
  .choice:hover{background:var(--line-soft)}
  .choice[aria-pressed="true"]{border-color:var(--accent);background:var(--tint)}
  .choice:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
  .name{font-weight:600;overflow-wrap:anywhere}
  .says{font-size:0.875rem;color:var(--muted);font-variant-numeric:tabular-nums}
</style>
