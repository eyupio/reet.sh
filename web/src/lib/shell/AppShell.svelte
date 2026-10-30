<script lang="ts">
  import { onMount, tick, type Snippet } from 'svelte'
  import Logo from '../Logo.svelte'
  import { browserViewport, keyboardChrome, keyboardLikelyOpen, PHONE_SCREEN, ViewportTracker } from '../terminal/sizing'
  import Bell from './Bell.svelte'
  import BottomTabs from './BottomTabs.svelte'
  import { chromeFocus } from './chrome.svelte'
  import JumpSearch from './JumpSearch.svelte'
  import MoreSheet from './MoreSheet.svelte'
  import TopTabs from './TopTabs.svelte'
  import type { JumpItem, Tab } from './tabs'

  // The signed-in top bar and, on a phone, the bottom tab bar (spec: Information architecture and navigation, R13).
  // Desktop (1100 px and wider): logo, every tab, Jump, the bell, More, and the account menu.
  // Tablet (651-1099 px): logo, the bar's tabs and More, a search button, the bell, the account menu.
  // Phone (PHONE_SCREEN): a slim bar with logo, search button, bell and account; the bar's tabs and More along the bottom.
  // `compact` is the terminal workspace's slim header: no bottom bar (the terminal keeps its own height rule), and on a
  // phone a More button in the top bar reaches every page instead.
  let {
    tabs, attention, compact, bar, more, bell = false, jump, account,
  }: {
    tabs: Tab[]
    attention: number
    compact: boolean
    /** The tabs a tablet and a phone keep in view (Home, Sessions, Machines); the rest of `tabs` move into More there. */
    bar: Tab[]
    /** Secondary pages, grouped in More at every screen size. */
    more: Tab[]
    /** The bell and its Needs you count, for a writer in accounts mode only. */
    bell?: boolean
    jump: () => Promise<JumpItem[]>
    account?: Snippet
  } = $props()

  const overflow = $derived(tabs.filter(tab => !bar.some(b => b.href === tab.href)))
  let viewportWidth = $state(window.innerWidth)
  let phoneLayout = $state(matchMedia(PHONE_SCREEN).matches)
  const visibleOverflow = $derived(viewportWidth < 1100 || phoneLayout ? overflow : [])
  const moreCurrent = $derived([...visibleOverflow, ...more].some(tab => tab.current))

  let moreOpen = $state(false)
  let moreOpener = $state<HTMLElement>()
  let moreAll = $state(false)
  // The More sheet lists what is out of view: the overflow tabs and More, or every page from the terminal's slim bar on a phone.
  const sheetItems = $derived(moreAll ? [...tabs, ...more] : [...visibleOverflow, ...more])
  let searchOpen = $state(false)
  let searchButton = $state<HTMLButtonElement>()
  let input = $state<HTMLInputElement>()
  let searchFocused = $state(false)
  let keyboardOpen = $state(false)
  // Jump in the top bar, the launch bar's prompt on Home, or the session screen's terminal.
  const editing = $derived(searchFocused || chromeFocus.launch || chromeFocus.session)
  const barHidden = $derived(keyboardChrome(searchFocused ? 'search' : chromeFocus.launch ? 'launch' : chromeFocus.session ? 'session' : null, keyboardOpen))

  function openMore(opener: HTMLElement, all = false) { moreOpener = opener; moreAll = all; moreOpen = true }
  async function openSearch() {
    searchOpen = true
    await tick()
    input?.focus()
  }
  function closeSearch() {
    searchOpen = false
    searchButton?.focus()
  }

  // `/` focuses Jump from anywhere but a field or an open dialog.
  function shortcut(event: KeyboardEvent) {
    if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey || event.defaultPrevented) return
    const target = event.target
    if (target instanceof HTMLElement && (target.isContentEditable || target.closest('input, textarea, select, [contenteditable], dialog[open]'))) return
    if (document.querySelector('dialog[open]')) return
    event.preventDefault()
    if (matchMedia('(max-width: 1099.98px)').matches) void openSearch()
    else input?.focus()
  }

  // The phone keyboard: the same usable-height rule as the phone terminal (sizing.ts). While Jump, the launch prompt or the
  // session screen's terminal has the keyboard up the bottom tab bar steps aside, and the top bar too for the last two
  // (keyboardChrome).
  let measureKeyboard = () => {}
  onMount(() => {
    const tracker = new ViewportTracker()
    const coarse = matchMedia('(pointer: coarse)'), phone = matchMedia(PHONE_SCREEN)
    const updatePhoneLayout = () => { phoneLayout = phone.matches }
    phone.addEventListener('change', updatePhoneLayout)
    let frame = 0
    measureKeyboard = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const usable = tracker.read(browserViewport())
        keyboardOpen = keyboardLikelyOpen(usable.shrink, { coarse: coarse.matches, narrow: phone.matches, editing, scale: window.visualViewport?.scale ?? 1 })
      })
    }
    measureKeyboard()
    const vv = window.visualViewport
    window.addEventListener('resize', measureKeyboard)
    vv?.addEventListener('resize', measureKeyboard)
    vv?.addEventListener('scroll', measureKeyboard)
    return () => {
      cancelAnimationFrame(frame)
      phone.removeEventListener('change', updatePhoneLayout)
      window.removeEventListener('resize', measureKeyboard)
      vv?.removeEventListener('resize', measureKeyboard)
      vv?.removeEventListener('scroll', measureKeyboard)
    }
  })

  // A field outside the top bar (the launch prompt, the session's terminal) taking or losing focus is measured again, as
  // Jump's focus change is.
  $effect(() => { void chromeFocus.launch; void chromeFocus.session; measureKeyboard() })

  // The page keeps room at its foot for the phone's tab bar (app.css reads this attribute under the phone rule).
  $effect(() => {
    const root = document.documentElement
    // Off while the bar steps aside for the keyboard, so the page does not keep an empty band above it.
    root.toggleAttribute('data-bottom-tabs', !compact && !barHidden)
    // The launch prompt or the session's terminal with the keyboard up takes the top bar's room too (the spec's keyboard-open
    // rule); Jump lives in it, so it stays for Jump.
    root.toggleAttribute('data-chrome-aside', barHidden && !searchFocused && (chromeFocus.launch || chromeFocus.session))
    return () => { root.removeAttribute('data-bottom-tabs'); root.removeAttribute('data-chrome-aside') }
  })
</script>

<svelte:window onkeydown={shortcut} bind:innerWidth={viewportWidth} />

<div class="shell-bar" class:compact>
  <a class="home" href="/" aria-label="reet.sh home"><Logo /></a>
  <TopTabs {tabs} {bar} {moreCurrent} {moreOpen} onmore={openMore} />
  <span class="grow"></span>
  <JumpSearch load={jump} open={searchOpen} bind:input onclose={closeSearch}
    onfocuschange={(focused) => { searchFocused = focused; measureKeyboard() }} />
  <button type="button" class="icon search-button" bind:this={searchButton} aria-label="Jump to a session, project or machine" aria-expanded={searchOpen}
    onclick={() => (searchOpen ? closeSearch() : void openSearch())}>
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="11" cy="11" r="7" /><path d="M20 20l-3.5-3.5" /></svg>
  </button>
  {#if bell}<Bell count={attention} />{/if}
  {#if compact}
    <button type="button" class="icon compact-more" class:current={moreCurrent} aria-haspopup="dialog" aria-expanded={moreOpen}
      aria-label={moreCurrent ? 'More, current page inside' : 'More'} onclick={(event) => openMore(event.currentTarget, true)}>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="currentColor"><circle cx="5" cy="12" r="1.6" /><circle cx="12" cy="12" r="1.6" /><circle cx="19" cy="12" r="1.6" /></svg>
    </button>
  {/if}
  {@render account?.()}
</div>
{#if !compact}<BottomTabs tabs={bar} {moreCurrent} {moreOpen} hidden={barHidden} onmore={openMore} />{/if}
{#if moreOpen}<MoreSheet items={sheetItems} opener={moreOpener} onclose={() => (moreOpen = false)} />{/if}

<style>
  .shell-bar { position: relative; display: flex; align-items: center; gap: 28px; height: 60px; line-height: 1.3; padding: 0 24px; }
  .shell-bar > :global(.top-tabs) { margin-left: 0; }
  .home { display: flex; align-items: center; flex: none; color: inherit; text-decoration: none; }
  .home :global(.logo) { width: 120px; }
  /* A finger-sized target on touch screens; the bar is taller than this at every width. */
  @media (pointer: coarse) { .home { min-height: 44px; } }
  .grow { flex: 1; min-width: 0; }
  .icon {
    display: none; place-items: center; flex: none; width: 44px; height: 44px; padding: 0;
    border: 1px solid var(--line); border-radius: 10px; background: none; color: var(--muted); cursor: pointer;
  }
  .icon:hover, .icon[aria-expanded='true'] { color: var(--ink); background: var(--card); }
  .icon.current { position: relative; color: var(--accent-text); border-color: var(--accent-text); }
  .icon svg { width: 20px; height: 20px; }
  @media (max-width: 1099.98px) {
    .shell-bar { height: 64px; gap: 20px; font-size: 15px; }
    .shell-bar > :global(.top-tabs) { margin-left: 0; }
    .search-button { display: grid; }
  }
  /* Keep all destinations and search reachable on narrow tablets. */
  @media (min-width: 651px) and (max-width: 799px) { .shell-bar { gap: 8px; padding: 0 16px; } }
  @media (max-width: 650px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape) {
    .shell-bar { height: 56px; padding: 0 16px; gap: 8px; }
    .compact-more { display: grid; }
  }
  .shell-bar.compact { height: 56px; padding: 0 16px; }
  /* The terminal's slim bar holds one more button (More) on a phone, so the logo is smaller there to fit 320 px. */
  @media (max-width: 700px) { .shell-bar.compact { height: 54px; padding: 0 10px; gap: 6px; } .shell-bar.compact .home :global(.logo) { width: 88px; } }
</style>
