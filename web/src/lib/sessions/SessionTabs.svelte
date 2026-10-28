<script lang="ts">
  import { tick } from 'svelte'
  import { stateWord } from '../home'
  import { sessionHref, type TerminalSession } from '../terminal/api'
  import { overflowEdges } from '../terminal/sizing'

  // The strip of open sessions (R20): the sessions opened in this browser tab since it loaded, at most eight, in the order
  // they were opened. Each tab is a link to its session with its own close button; middle-click closes it too. Closing a tab
  // closes the view, never the session. + goes to Home with the launch bar focused.
  let { tabs, current, onselect, onclose }: {
    tabs: TerminalSession[]; current: string
    onselect: (s: TerminalSession) => void; onclose: (s: TerminalSession) => void
  } = $props()

  let strip = $state<HTMLElement>()
  // A plain click switches in place; a click with a modifier opens the link as the browser would (a new window or tab).
  function choose(event: MouseEvent, s: TerminalSession) {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    onselect(s)
  }
  // Tabs that do not fit scroll sideways: the row fades at the edge that hides some (the terminal toolbar's hint), the
  // current tab is kept in view, and a tab scrolls into view when it takes focus.
  function edges(row: HTMLElement) {
    const update = () => { const { start, end } = overflowEdges(row.scrollLeft, row.scrollWidth, row.clientWidth); row.classList.toggle('more-start', start); row.classList.toggle('more-end', end) }
    const sizes = new ResizeObserver(update); sizes.observe(row)
    const content = new MutationObserver(update); content.observe(row, { childList: true, subtree: true })
    row.addEventListener('scroll', update, { passive: true }); update()
    return { destroy() { sizes.disconnect(); content.disconnect(); row.removeEventListener('scroll', update) } }
  }
  $effect(() => {
    const id = current, count = tabs.length
    void tick().then(() => { if (id && count) strip?.querySelector<HTMLElement>('a.tab[aria-current=page]')?.scrollIntoView({ block: 'nearest', inline: 'nearest' }) })
  })
  async function close(s: TerminalSession) {
    const at = tabs.findIndex(t => t.id === s.id)
    onclose(s)
    await tick()
    // Focus stays in the strip: on the tab now in the closed one's place, else the one before, else +.
    const links = [...(strip?.querySelectorAll<HTMLElement>('a.tab') ?? [])]
    ;(links[Math.min(at, links.length - 1)] ?? strip?.querySelector<HTMLElement>('a.new'))?.focus()
  }
</script>

<nav class="session-tabs" aria-label="Open sessions" bind:this={strip}>
  <ul use:edges>
    {#each tabs as s (s.id)}
      {@const word = stateWord(s)}
      <li class:current={s.id === current}>
        <a class="tab" href={sessionHref(s)} aria-current={s.id === current ? 'page' : undefined} title={s.title}
          onclick={event => choose(event, s)}
          onmousedown={event => { if (event.button === 1) event.preventDefault() }}
          onauxclick={event => { if (event.button === 1) { event.preventDefault(); void close(s) } }}>
          <span class="dot {word.tone}" aria-hidden="true"></span><span class="name">{s.title}</span><span class="vh">, {word.word}</span>
        </a>
        <button type="button" class="close" aria-label={`Close ${s.title}`} title="Close the tab (the session keeps running)" onclick={() => void close(s)}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M7 7l10 10M17 7L7 17" /></svg>
        </button>
      </li>
    {/each}
  </ul>
  <a class="new" href="/#launch" aria-label="New session" title="New session">
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" aria-hidden="true"><path d="M12 5v14M5 12h14" /></svg>
  </a>
</nav>

<style>
  /* The mock-up's strip: 46 px on the sunk background, tabs 36 px with their top corners rounded, the current one raised. */
  /* The mock-up's strip of tabs, in the workspace's own toolbar row (so the terminal keeps its height): tabs 36 px with their
     top corners rounded and flush with the row's bottom edge, the current one raised. */
  .session-tabs { flex: 1 1 auto; align-self: stretch; margin: -8px 0; display: flex; align-items: flex-end; gap: 6px; min-width: 0; }
  ul { list-style: none; margin: 0; padding: 0; display: flex; align-items: flex-end; gap: 6px; min-width: 0; overflow-x: auto; scrollbar-width: none; }
  li { flex: 0 1 auto; min-width: 120px; display: flex; align-items: center; height: 36px; border: 1px solid transparent; border-bottom: none; border-radius: 9px 9px 0 0; color: var(--muted); }
  li.current { background: var(--bg); border-color: var(--line); margin-bottom: -1px; height: 37px; color: var(--ink); }
  li:hover:not(.current) { color: var(--ink-soft); }
  .tab { display: flex; align-items: center; gap: 8px; min-width: 0; height: 100%; padding: 0 4px 0 14px; color: inherit; text-decoration: none; white-space: nowrap; font-size: 14px; }
  .name { overflow: hidden; text-overflow: ellipsis; max-width: 220px; }
  .tab:focus-visible, .close:focus-visible, .new:focus-visible { outline: 2px solid var(--accent-text); outline-offset: -2px; border-radius: 7px; }
  .dot { width: 8px; height: 8px; border-radius: 50%; flex: none; background: var(--muted); }
  .dot.good { background: var(--good); }
  .dot.warn { background: var(--warn); }
  .dot.bad { background: var(--bad); }
  .close { flex: none; display: grid; place-items: center; width: 28px; height: 28px; margin-right: 4px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--muted); cursor: pointer; opacity: 0; }
  li.current .close, li:hover .close, .close:focus-visible { opacity: 1; }
  .close:hover { background: var(--tint); color: var(--ink); }
  .close svg { width: 14px; height: 14px; }
  .new { flex: none; display: grid; place-items: center; width: 40px; height: 36px; color: var(--muted); }
  .new:hover { color: var(--ink); }
  .new svg { width: 16px; height: 16px; }
  .vh { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  /* Touch: the close button is always shown and the targets are 44 px wide. */
  ul:global(.more-end) { -webkit-mask-image: linear-gradient(to right, var(--ink) calc(100% - 36px), transparent); mask-image: linear-gradient(to right, var(--ink) calc(100% - 36px), transparent); }
  ul:global(.more-start) { -webkit-mask-image: linear-gradient(to left, var(--ink) calc(100% - 36px), transparent); mask-image: linear-gradient(to left, var(--ink) calc(100% - 36px), transparent); }
  ul:global(.more-start.more-end) { -webkit-mask-image: linear-gradient(to right, transparent, var(--ink) 36px, var(--ink) calc(100% - 36px), transparent); mask-image: linear-gradient(to right, transparent, var(--ink) 36px, var(--ink) calc(100% - 36px), transparent); }
  /* Touch: 44 px targets below the tab's 1 px top edge. */
  @media (pointer: coarse) { li { height: 45px; min-width: 160px; } li.current { height: 46px; } .close { opacity: 1; width: 44px; height: 44px; margin-right: 0; } .new { width: 44px; height: 44px; } }
</style>
