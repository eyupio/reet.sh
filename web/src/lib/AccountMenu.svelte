<script lang="ts">
  import { applyTheme, saveProfile } from './api'
  import ThemePicker from './ThemePicker.svelte'

  let { name = '', email = '', accounts = true, active = false, unread = 0, onsignout }: {
    name?: string
    email?: string
    accounts?: boolean
    active?: boolean
    unread?: number
    onsignout: () => void
  } = $props()

  let root = $state<HTMLElement>()
  let menu = $state<HTMLDetailsElement>()

  const initials = $derived((name.trim() || email.trim() || 'You')
    .split(/\s+/)
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase())
  const label = $derived(name.trim() || 'your account')
  const unreadLabel = $derived(unread > 99 ? '99+' : String(unread))

  function close(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !menu?.open) return
    menu.open = false
    menu.querySelector('summary')?.focus()
    event.preventDefault()
  }

  // Light, dark or follow the device: the same control as the public header. The choice applies at
  // once; only an account has somewhere to keep it, and a save that fails puts the previous theme back.
  type Theme = 'system' | 'light' | 'dark'
  const currentTheme = (): Theme => {
    const value = document.documentElement.dataset.theme
    return value === 'light' || value === 'dark' ? value : 'system'
  }
  let previousTheme: Theme = currentTheme()
  async function saveTheme(next: Theme) {
    const previous = previousTheme
    previousTheme = next
    if (!accounts) return
    try { await saveProfile({ theme: next }) } catch { previousTheme = previous; applyTheme(previous) }
  }

  function signOut() {
    if (menu) menu.open = false
    onsignout()
  }
</script>

<svelte:window onkeydown={close} onclick={(event) => {
  if (event.target instanceof Node && !root?.contains(event.target)) {
    root?.querySelectorAll<HTMLDetailsElement>('details[open]').forEach((detail) => (detail.open = false))
  }
}} />

<div class="account" bind:this={root}>
  <details bind:this={menu} ontoggle={() => { if (menu?.open) previousTheme = currentTheme() }}>
    <summary class:active aria-label={`Account menu for ${label}${unread > 0 ? `, ${unread} unread notifications` : ''}`}>
      <span class="avatar" aria-hidden="true">{initials}{#if unread > 0}<span class="badge">{unreadLabel}</span>{/if}</span>
      <span class="who"><strong>{name || 'Your account'}</strong>{#if email}<small>{email}</small>{/if}</span>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="m6 9 6 6 6-6" /></svg>
    </summary>
    <div class="panel">
      <div class="panel-who">
        <strong>{name || 'Your account'}</strong>
        {#if email}<span>{email}</span>{/if}
      </div>
      <div class="theme"><ThemePicker onchoose={saveTheme} /></div>
      <a href="/notifications">Notifications{#if unread > 0}<span class="count">{unreadLabel}</span>{/if}</a>
      {#if accounts}<a href="/account/profile">Account &amp; preferences</a>{/if}
      <button type="button" onclick={signOut}>Sign out</button>
    </div>
  </details>
</div>

<style>
  .account { position: relative; }
  details { position: relative; }
  summary {
    list-style: none; display: flex; align-items: center; gap: 8px; min-height: 44px;
    padding: 4px 8px 4px 4px; border: 1px solid var(--line); border-radius: 999px;
    background: var(--card); cursor: pointer; color: var(--muted);
  }
  summary::-webkit-details-marker { display: none; }
  summary:hover, summary.active { border-color: var(--muted); }
  summary.active .avatar { background: var(--accent); color: var(--accent-ink); }
  .avatar {
    position: relative; width: 30px; height: 30px; display: grid; place-items: center; flex: none;
    border-radius: 50%; background: var(--tint); color: var(--accent-text);
    font-size: 0.7rem; font-weight: 700; letter-spacing: 0.03em;
  }
  .badge {
    position: absolute; top: -5px; right: -7px; min-width: 18px; height: 18px; padding: 0 5px; display: grid; place-items: center;
    border-radius: 999px; background: var(--accent); color: var(--accent-ink); border: 2px solid var(--card);
    font-size: 0.62rem; font-weight: 700; letter-spacing: 0; line-height: 1;
  }
  .count {
    margin-left: auto; min-width: 20px; height: 20px; padding: 0 6px; display: inline-grid; place-items: center;
    border-radius: 999px; background: var(--accent); color: var(--accent-ink); font-size: 0.7rem; font-weight: 700;
  }
  .who { display: grid; line-height: 1.15; min-width: 0; }
  .who strong {
    max-width: 12ch; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    color: var(--ink); font-size: 0.8rem; font-weight: 600;
  }
  .who small {
    max-width: 20ch; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    color: var(--muted); font-size: 0.66rem;
  }
  summary svg { width: 14px; height: 14px; flex: none; }
  details[open] summary svg { transform: rotate(180deg); }
  .panel {
    position: absolute; right: 0; top: calc(100% + 6px); z-index: 30;
    min-width: 240px; padding: 8px; border: 1px solid var(--line); border-radius: 12px;
    background: var(--card); box-shadow: 0 12px 28px rgb(0 0 0 / 0.14);
  }
  .panel-who { display: grid; gap: 1px; padding: 8px 10px 10px; margin-bottom: 4px; border-bottom: 1px solid var(--line); }
  .panel-who strong { color: var(--ink); font-size: 0.85rem; }
  .panel-who span { color: var(--muted); font-size: 0.72rem; overflow-wrap: anywhere; }
  .panel a, .panel button {
    display: flex; align-items: center; width: 100%; min-height: 40px; padding: 8px 10px;
    border: 0; border-radius: 8px; background: none; color: var(--ink);
    font: inherit; font-size: 0.82rem; text-align: left; text-decoration: none; cursor: pointer;
  }
  .panel a:hover, .panel button:hover { background: var(--tint); }
  .theme { display: flex; justify-content: center; padding: 6px 4px 10px; margin-bottom: 4px; border-bottom: 1px solid var(--line); }
  /* The top bar shows the initials only, as drawn; the name and email are at the top of the panel. */
  .who { display: none; }
  summary { min-width: 36px; min-height: 36px; padding: 0; border: 0; border-radius: 50%; background: none; justify-content: center; }
  .avatar { width: 36px; height: 36px; background: var(--accent-wash); font-size: 13px; font-weight: 600; letter-spacing: 0; }
  summary:focus-visible { outline: 2px solid var(--accent-text); outline-offset: 3px; }
  @media (pointer: coarse) { summary { min-width: 44px; min-height: 44px; } }
  summary > svg { display: none; }
  .panel { max-height: calc(100dvh - 90px); overflow: auto; }
  @media (pointer: coarse) { .panel a, .panel button { min-height: 44px; } }
</style>
