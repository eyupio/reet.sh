<script lang="ts">
  let { name = '', email = '', admin = false, accounts = true, active = false, onsignout }: {
    name?: string
    email?: string
    admin?: boolean
    accounts?: boolean
    active?: boolean
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

  function close(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !menu?.open) return
    menu.open = false
    menu.querySelector('summary')?.focus()
    event.preventDefault()
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
  <details bind:this={menu}>
    <summary class:active aria-label={`Account menu for ${label}`}>
      <span class="avatar" aria-hidden="true">{initials}</span>
      <span class="who"><strong>{name || 'Your shelf'}</strong>{#if email}<small>{email}</small>{/if}</span>
      <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="m6 9 6 6 6-6" /></svg>
    </summary>
    <div class="panel">
      <div class="panel-who">
        <strong>{name || 'Your shelf'}</strong>
        {#if email}<span>{email}</span>{/if}
      </div>
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
    width: 30px; height: 30px; display: grid; place-items: center; flex: none;
    border-radius: 50%; background: var(--tint); color: var(--accent);
    font-size: 0.7rem; font-weight: 700; letter-spacing: 0.03em;
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
  @media (max-width: 1000px) {
    .who { display: none; }
    summary { padding: 4px; border-radius: 50%; }
  }
</style>
