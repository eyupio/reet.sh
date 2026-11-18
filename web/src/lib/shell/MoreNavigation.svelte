<script lang="ts">
  import type { Tab } from './tabs'

  let { id, items, opener, open = $bindable(false) }: {
    id: string
    items: (Tab & { group: string })[]
    opener?: HTMLElement
    open?: boolean
  } = $props()

  const groups = $derived([...new Set(items.map(item => item.group))]
    .map(label => ({ label, items: items.filter(item => item.group === label) })))
  let panel = $state<HTMLElement>()
  const atBottom = $derived(!!opener?.closest('.bottom-tabs'))

  function position() {
    if (!panel || !opener) return
    const anchor = opener.getBoundingClientRect()
    panel.style.setProperty('--trigger-right', `${anchor.right}px`)
    panel.style.setProperty('--trigger-edge', `${atBottom ? document.documentElement.clientHeight - anchor.top : anchor.bottom}px`)
  }

  $effect(() => {
    position()
    if (!open) return
    window.addEventListener('resize', position)
    window.addEventListener('scroll', position, true)
    return () => {
      window.removeEventListener('resize', position)
      window.removeEventListener('scroll', position, true)
    }
  })
</script>

<nav {id} popover="auto" aria-label="More pages" class:bottom={atBottom} bind:this={panel}
  ontoggle={event => { open = event.newState === 'open' }}>
  {#each groups as group (group.label)}
    <section aria-label={group.label} class:tools={group.label === 'Skills & tools'}>
      <h2>{group.label}</h2>
      {#each group.items as item (item.href)}
        {@const external = item.href.startsWith('http')}
        <a href={item.href} aria-current={item.current ? 'page' : undefined}
          target={external ? '_blank' : undefined} rel={external ? 'noopener noreferrer' : undefined}
          onclick={() => panel?.hidePopover()}>{item.label}</a>
      {/each}
    </section>
  {/each}
</nav>

<style>
  nav {
    --panel-width: min(440px, calc(100% - 16px));
    position: fixed; inset: auto; margin: 0;
    top: calc(var(--trigger-edge, 64px) + 6px);
    left: clamp(8px, calc(var(--trigger-right, 440px) - var(--panel-width)), calc(100% - var(--panel-width) - 8px));
    width: var(--panel-width); max-height: calc(100dvh - var(--trigger-edge, 64px) - 14px); overflow: auto;
    padding: 10px; border: 1px solid var(--line); border-radius: 12px;
    background: var(--card-raised); color: var(--ink); box-shadow: 0 12px 30px var(--scrim);
  }
  nav:popover-open { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
  .bottom { top: auto; bottom: calc(var(--trigger-edge, 64px) + 6px); }
  section { min-width: 0; }
  section.tools { grid-row: span 2; }
  h2 { margin: 3px 10px 4px; color: var(--muted); font: 600 .875rem/1.35 var(--sans); }
  a {
    display: flex; align-items: center; min-height: 38px; padding: 6px 10px; border-radius: 8px;
    color: var(--ink); font-size: .875rem; text-decoration: none; overflow-wrap: anywhere;
  }
  a:hover, a[aria-current='page'] { background: var(--tint); }
  a[aria-current='page'] { font-weight: 600; box-shadow: inset 3px 0 0 var(--accent-text); }
  a:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; }
  @media (pointer: coarse) { a { min-height: 44px; } }
  @media (max-width: 650px) { nav { --panel-width: calc(100% - 16px); } }
  @media (max-width: 359px) {
    nav:popover-open { grid-template-columns: minmax(0, 1fr); }
    section.tools { grid-row: auto; }
  }
</style>
