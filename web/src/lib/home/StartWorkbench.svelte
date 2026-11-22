<script lang="ts">
  // Home with nothing in flight: one line and one obvious next step. With no machine connected that step is the
  // Machines page; otherwise it is an agent session. The other ways to start stay one quiet click away, and each
  // dialog asks for everything else.
  import type { NewSessionKind } from '../terminal/NewSessionChoice.svelte'
  let { factory, machines, canAdd, machinesHref, addHref, onchoose, onplan, quiet = false }: {
    factory: boolean
    /** How many machines the account has, and how many of them are connected; undefined until the first read. */
    machines?: { total: number; connected: number }
    canAdd: boolean
    /** The Machines page, and its Add a machine step. */
    machinesHref: string
    addHref: string
    onchoose: (kind: NewSessionKind, opener: HTMLElement) => void
    onplan: () => void
    /** Something else on Home is the one filled step for now: this one is drawn plain. */
    quiet?: boolean
  } = $props()
  const id = $props.id()
  const none = $derived(machines !== undefined && machines.connected === 0)
  const nothingEnrolled = $derived(machines !== undefined && machines.total === 0)
  const others: { kind: NewSessionKind; action: string }[] = [
    { kind: 'factory', action: 'New Factory outcome' },
    { kind: 'terminal', action: 'Open terminal' },
  ]
</script>

<section class="workbench" aria-labelledby={`${id}-heading`}>
  <h2 id={`${id}-heading`}>Nothing in flight.</h2>
  {#if none}
    <p class="guidance">{nothingEnrolled ? 'A session runs on one of your machines, and none is connected yet.' : 'None of your machines is connected right now.'}</p>
    {#if nothingEnrolled && canAdd}<a class="btn primary" href={addHref}>Add a machine</a>
    {:else}<a class="btn primary" href={machinesHref}>Open Machines</a>{/if}
  {:else}
    <button class="btn" class:primary={!quiet} type="button" onclick={event => onchoose('agent', event.currentTarget)}>Start an agent session</button>
    {#if factory}<p class="guidance">Or give Gaffer a goal, approve the plan and follow the work in Factory.</p>{/if}
  {/if}
  <div class="more">
    {#if !none}
      {#each others.filter(choice => factory || choice.kind !== 'factory') as choice (choice.kind)}
        <button type="button" class="later" onclick={event => onchoose(choice.kind, event.currentTarget)}>{choice.action}</button>
      {/each}
    {/if}
    <button type="button" class="later" onclick={onplan}>Plan a session</button>
  </div>
</section>

<style>
  .workbench{display:grid;gap:10px;justify-items:center;text-align:center;min-width:0;padding:16px 20px;border:1px dashed var(--line-strong);border-radius:14px;color:var(--ink)}
  h2{margin:0;font:500 1rem/1.4 var(--sans);color:var(--muted);overflow-wrap:anywhere}
  .btn{min-height:44px;max-width:100%}
  .guidance{margin:0;color:var(--muted);font-size:.875rem;max-width:52ch}
  .more{display:flex;flex-wrap:wrap;justify-content:center;gap:0 4px;min-width:0;max-width:100%}
  .later{border:0;background:none;padding:0 8px;min-height:44px;font:inherit;font-size:.875rem;font-weight:600;color:var(--accent-text);cursor:pointer}
  @media (max-width:500px){.workbench{gap:8px;padding:14px}}
  .later:hover{text-decoration:underline}.later:focus-visible{outline:2px solid var(--accent);outline-offset:2px;border-radius:6px}
</style>
