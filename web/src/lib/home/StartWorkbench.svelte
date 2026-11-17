<script lang="ts">
  // Home with nothing in flight: one line and the ways to start. Each dialog asks for everything else.
  import type { NewSessionKind } from '../terminal/NewSessionChoice.svelte'
  let { factory, onchoose, onplan }: {
    factory: boolean
    onchoose: (kind: NewSessionKind, opener: HTMLElement) => void
    onplan: () => void
  } = $props()
  const id = $props.id()
  const choices: { kind: NewSessionKind; action: string }[] = [
    { kind: 'factory', action: 'Give Gaffer an outcome' },
    { kind: 'agent', action: 'Start a session' },
    { kind: 'terminal', action: 'Open terminal' },
  ]
</script>

<section class="workbench" aria-labelledby={`${id}-heading`}>
  <h2 id={`${id}-heading`}>Nothing in flight.</h2>
  <div class="choices">
    {#each choices.filter(choice => factory || choice.kind !== 'factory') as choice (choice.kind)}
      <button class="btn primary" type="button" onclick={event => onchoose(choice.kind, event.currentTarget)}>{choice.action}</button>
    {/each}
  </div>
  <button type="button" class="later" onclick={onplan}>Plan a session</button>
</section>

<style>
  .workbench{display:grid;gap:14px;justify-items:center;text-align:center;min-width:0;padding:28px 20px;border:1px dashed var(--line-strong);border-radius:14px;color:var(--ink)}
  h2{margin:0;font:500 1rem/1.4 var(--sans);color:var(--muted);overflow-wrap:anywhere}
  .choices{display:flex;flex-wrap:wrap;justify-content:center;gap:8px;min-width:0;max-width:100%}
  .btn{min-height:44px;max-width:100%}
  .later{border:0;background:none;padding:0 8px;min-height:44px;font:inherit;font-size:.875rem;font-weight:600;color:var(--accent-text);cursor:pointer}
  .later:hover{text-decoration:underline}.later:focus-visible{outline:2px solid var(--accent);outline-offset:2px;border-radius:6px}
</style>
