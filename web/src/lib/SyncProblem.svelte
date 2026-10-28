<script lang="ts">
  import CopyButton from './CopyButton.svelte'
  import { explainSyncFailure } from './syncProblems'

  let {
    message,
    host = '',
    site = '',
    project = '',
    retrying = false,
    compact = false,
    online = true,
    attempts = 1,
    onretry,
  }: {
    message: string
    host?: string
    site?: string
    project?: string
    retrying?: boolean
    compact?: boolean
    /** False while the machine is not reporting: commands and retry need it reachable. */
    online?: boolean
    /** How many jobs share this exact failure. */
    attempts?: number
    onretry?: () => void
  } = $props()

  const fix = $derived(explainSyncFailure(message, { host, site: site || location.origin, project }))
</script>

<section class="problem" class:compact role="alert" aria-label={fix.title}>
  <header>
    <span class="mark" aria-hidden="true">!</span>
    <div class="copy">
      <p class="title">{fix.title}</p>
      <p class="detail">{fix.detail}</p>
      {#if attempts > 1}<p class="repeat">The same failure on {attempts} jobs.</p>{/if}
    </div>
  </header>
  {#if !online && (fix.commands.length || (onretry && fix.retry))}
    <p class="hint offline">{host || 'The machine'} is offline. The fix runs there, so reconnect it first.</p>
  {/if}
  {#if online && fix.commands.length}
    <div class="commands" aria-label="Commands to fix this">
      <p class="label">Run on {host || 'the machine'}</p>
      {#each fix.commands as command (command)}
        <div class="command">
          <code>{command}</code>
          <CopyButton text={command} label="Copy command" compact ariaLabel={`Copy fix command for ${host || 'the machine'}`} />
        </div>
      {/each}
      {#if fix.hint}<p class="hint">{fix.hint}</p>{/if}
    </div>
  {/if}
  {#if (online && onretry && fix.retry) || fix.href}
    <div class="actions">
      {#if online && onretry && fix.retry}
        <button class="btn primary" type="button" disabled={retrying} onclick={onretry}>
          {retrying ? 'Trying…' : fix.local ? 'I’ve run it — try again' : 'Fix and try again'}
        </button>
      {/if}
      {#if fix.href}<a class="btn" href={fix.href}>{fix.href.startsWith('/machines') ? 'Open Machines' : 'Open Sync'}</a>{/if}
    </div>
  {/if}
</section>

<style>
  .problem {
    display: grid; gap: 10px; min-width: 0; padding: 12px;
    border: 1px solid color-mix(in srgb, var(--bad) 45%, var(--line));
    border-left: 3px solid var(--bad); border-radius: 10px;
    background: color-mix(in srgb, var(--bad) 6%, var(--card));
  }
  .problem.compact { gap: 8px; padding: 10px; }
  header { display: flex; align-items: flex-start; gap: 10px; min-width: 0; }
  .mark {
    display: grid; place-items: center; width: 22px; height: 22px; flex: none;
    border: 1px solid color-mix(in srgb, var(--bad) 50%, var(--line)); border-radius: 50%;
    color: var(--bad); font-size: 0.78rem; font-weight: 700;
  }
  .copy { display: grid; gap: 3px; min-width: 0; }
  .title { margin: 0; color: var(--ink); font-size: 0.88rem; font-weight: 650; line-height: 1.35; }
  .detail { margin: 0; color: var(--muted); font-size: 0.8rem; line-height: 1.55; overflow-wrap: anywhere; }
  .repeat { margin: 0; color: var(--bad); font-size: 0.72rem; font-weight: 600; line-height: 1.45; }
  .commands { display: grid; gap: 6px; }
  .label { margin: 0; color: var(--muted); font-size: 0.7rem; font-weight: 650; letter-spacing: 0.06em; text-transform: uppercase; }
  .command {
    display: flex; align-items: center; gap: 8px; flex-wrap: wrap; min-width: 0;
    padding: 8px 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg);
  }
  .command code { flex: 1 1 200px; min-width: 0; font: 0.76rem/1.5 var(--mono); overflow-wrap: anywhere; }
  .hint { margin: 0; color: var(--muted); font-size: 0.74rem; line-height: 1.5; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .actions :global(.btn) { min-height: 34px; padding: 6px 12px; font-size: 0.8rem; }
  @media (max-width: 560px) {
    .command :global(button) { width: 100%; }
    .actions :global(.btn) { flex: 1 1 auto; }
  }
</style>
