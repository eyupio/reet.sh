<script lang="ts">
  let { label, items, primary = false }: { label: string; items: { label: string; href: string; current: boolean; children?: { label: string; href: string; current: boolean }[] }[]; primary?: boolean } = $props()
  let root = $state<HTMLElement>()
  let menu = $state<HTMLDetailsElement>()
  let trigger = $state<HTMLElement>()
  const current = $derived(items.find(item => item.current)?.label)
  function close(event: KeyboardEvent) {
    if (event.key !== 'Escape') return
    const open = [...(root?.querySelectorAll<HTMLDetailsElement>('details[open]') ?? [])].at(-1)
    if (open) { open.open = false; open.querySelector('summary')?.focus(); event.preventDefault() }
  }
</script>

<svelte:window onkeydown={close} onclick={event => { if (event.target instanceof Node && !root?.contains(event.target)) root?.querySelectorAll<HTMLDetailsElement>('details[open]').forEach(detail => detail.open = false) }} />
{#snippet links()}
  {#each items as item (item.href)}
    {#if item.children?.length}
      <details class="nav-group" name={`${label}-groups`}>
        <summary class:current={item.current}>{item.label}<svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="m6 9 6 6 6-6" /></svg></summary>
        <div class="group-links">{#each item.children as child (child.href)}<a href={child.href} aria-current={child.current ? 'page' : undefined}>{child.label}</a>{/each}</div>
      </details>
    {:else}<a href={item.href} aria-current={item.current ? 'page' : undefined}>{item.label}</a>{/if}
  {/each}
{/snippet}
<nav bind:this={root} aria-label={label} class:primary>
  <div class="desktop">{@render links()}</div>
  <details class="mobile" bind:this={menu}>
    <summary bind:this={trigger}>{primary ? 'Menu' : label}<span>{current}</span><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="m6 9 6 6 6-6" /></svg></summary>
    <div class="links">{@render links()}</div>
  </details>
</nav>

<style>
  nav { min-width: 0; }
  .nav-group { position: relative; }
  .nav-group > summary { list-style: none; display: flex; align-items: center; gap: 6px; min-height: 44px; padding: 8px 12px; color: var(--muted); border-radius: 8px; font-size: 0.82rem; cursor: pointer; }
  .nav-group > summary::-webkit-details-marker { display: none; }
  .nav-group > summary.current { background: var(--tint); color: var(--ink); }
  .nav-group svg { width: 14px; height: 14px; }
  .group-links { position: absolute; right: 0; top: 100%; display: grid; gap: 4px; z-index: 25; min-width: 200px; padding: 8px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); box-shadow: 0 12px 28px rgb(0 0 0 / 0.12); }
  .desktop { display: flex; gap: 4px; flex-wrap: wrap; }
  a { display: inline-flex; align-items: center; min-height: 44px; padding: 8px 12px; color: var(--muted); font-size: 0.82rem; font-weight: 500; text-decoration: none; border-radius: 8px; }
  a:hover { background: var(--card); color: var(--ink); }
  a[aria-current='page'] { color: var(--ink); background: var(--tint); font-weight: 600; }
  nav:not(.primary) { border-bottom: 1px solid var(--line); padding-bottom: 12px; margin: 12px 0 20px; }
  .mobile { display: none; }
  @media (max-width: 1000px) {
    .desktop { display: none; }
    .mobile { display: block; position: relative; }
    summary { list-style: none; display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; cursor: pointer; font-size: 0.85rem; font-weight: 600; background: var(--card); }
    summary::-webkit-details-marker { display: none; }
    summary span { color: var(--muted); font-size: 0.8rem; font-weight: 400; margin-left: auto; }
    summary svg { width: 16px; height: 16px; flex: none; }
    details[open] summary svg { transform: rotate(180deg); }
    .links { display: grid; gap: 4px; padding: 8px; border: 1px solid var(--line); border-radius: 10px; background: var(--card); box-shadow: 0 12px 28px rgb(0 0 0 / 0.1); margin-top: 6px; }
    .group-links { position: static; min-width: 0; box-shadow: none; margin: 4px 0; border: 0; padding: 0 0 0 12px; }
    /* Anchored to the header, not the button: the button sits mid-screen, so a button-anchored panel ran off the left edge. */
    .primary .mobile { position: static; }
    .primary .links { position: absolute; z-index: 20; top: 100%; left: 16px; right: 16px; margin: 6px 0 0 auto; max-width: 320px; }
    .primary summary span { display: none; }
  }
</style>
