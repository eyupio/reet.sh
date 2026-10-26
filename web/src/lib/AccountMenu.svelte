<script lang="ts">
  import { applyTheme, saveProfile } from './api'

  let { name = '', email = '', admin = false, accounts = true, active = false, unread = 0, onsignout }: {
    name?: string
    email?: string
    admin?: boolean
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

  // Light, dark or follow the device. The page's current theme is read when the menu opens, because the saved one loads after sign-in.
  type Theme = 'system' | 'light' | 'dark'
  const themes: { id: Theme; label: string }[] = [{ id: 'system', label: 'Match this device' }, { id: 'light', label: 'Light' }, { id: 'dark', label: 'Dark' }]
  const currentTheme = (): Theme => {
    const value = document.documentElement.dataset.theme
    return value === 'light' || value === 'dark' ? value : 'system'
  }
  let theme = $state<Theme>(currentTheme())
  async function chooseTheme(next: Theme) {
    const previous = theme
    theme = next
    applyTheme(next)
    // Only an account has somewhere to keep the choice; a sign-in without accounts keeps it for this visit.
    if (!accounts) return
    try { await saveProfile({ theme: next }) } catch { theme = previous; applyTheme(previous) }
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
  <details bind:this={menu} ontoggle={() => { if (menu?.open) theme = currentTheme() }}>
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
      <div class="theme" role="group" aria-label="Theme">
        {#each themes as option (option.id)}
          <button type="button" aria-pressed={theme === option.id} onclick={() => chooseTheme(option.id)}>{option.label}</button>
        {/each}
      </div>
      <a href="/notifications">Notifications{#if unread > 0}<span class="count">{unreadLabel}</span>{/if}</a>
      {#if accounts}<a href="/account/profile">Account &amp; preferences</a>{/if}
      {#if admin}<a href="/admin/overview">Instance administration</a>{/if}
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
  .theme { display: flex; gap: 4px; padding: 6px 4px 8px; margin-bottom: 4px; border-bottom: 1px solid var(--line); }
  .panel .theme button { flex: 1; width: auto; min-height: 36px; justify-content: center; padding: 6px 4px; border: 1px solid var(--line); font-size: 0.74rem; text-align: center; }
  .panel .theme button[aria-pressed='true'] { background: var(--tint); border-color: var(--accent); font-weight: 650; }
  @media (max-width: 1000px) {
    .who { display: none; }
    summary { padding: 4px; border-radius: 50%; }
  }
</style>
