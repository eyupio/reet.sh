<script lang="ts">
  import CopyButton from './CopyButton.svelte'
  import type { TerminalControllerStatus } from './terminal/api'
  import { controllerDetail } from './terminal/controllerStatus'

  let { controller, machines, setUp, ready, canWrite, onhelp }: {
    controller: TerminalControllerStatus | null
    machines: number
    /** Machines that have done their own part. Unlike `ready`, this does not depend on the server's terminal access. */
    setUp: number
    ready: number
    canWrite: boolean
    onhelp: () => void
  } = $props()

  const serverOn = $derived(controller?.available === true)
  const connected = $derived(machines > 0)
  const allowed = $derived(setUp > 0)
</script>

<section class="readiness" aria-labelledby="readiness-title">
  <p class="eyebrow">Before your first terminal</p>
  <h2 id="readiness-title">Get your first terminal running</h2>
  <ol>
    <li data-step="controller" data-state={serverOn ? 'done' : 'todo'}>
      <span class="mark" aria-hidden="true">{serverOn ? '✓' : '1'}</span>
      <div>
        <strong>Allow terminals on this server{#if serverOn}<span class="vh"> (done)</span>{/if}</strong>
        {#if serverOn}
          <p>Controller access is enabled.</p>
        {:else}
          <p>{controller === null ? 'The server’s terminal status could not be read just now.' : controllerDetail(controller)}</p>
          <div class="command"><code>reet terminal start</code><CopyButton text="reet terminal start" compact ariaLabel="Copy reet terminal start" /></div>
          <small>Run it on the server that hosts reet.sh.</small>
        {/if}
      </div>
    </li>
    <li data-step="machine" data-state={connected ? 'done' : 'todo'}>
      <span class="mark" aria-hidden="true">{connected ? '✓' : '2'}</span>
      <div>
        <strong>Connect a machine{#if connected}<span class="vh"> (done)</span>{/if}</strong>
        {#if connected}
          <p>{machines === 1 ? 'One machine is connected.' : `${machines} machines are connected.`}</p>
        {:else}
          <p>Connect a laptop or server with the reet CLI. It takes about two minutes.</p>
          {#if canWrite}<a class="btn primary" href="/machines#add-machine">Connect your first machine</a>{/if}
        {/if}
      </div>
    </li>
    <li data-step="consent" data-state={allowed ? 'done' : 'todo'}>
      <span class="mark" aria-hidden="true">{allowed ? '✓' : '3'}</span>
      <div>
        <strong>Allow a terminal on the machine{#if allowed}<span class="vh"> (done)</span>{/if}</strong>
        {#if allowed}
          <p>{setUp === 1 ? 'One machine is set up for terminals.' : `${setUp} machines are set up for terminals.`}</p>
        {:else}
          <p>On the machine, install tmux 3.2 or newer, then run this. Shell access stays off until you consent there.</p>
          <div class="command"><code>reet agent terminal setup</code><CopyButton text="reet agent terminal setup" compact ariaLabel="Copy reet agent terminal setup" /></div>
          <button class="btn quiet" type="button" onclick={onhelp}>Terminal setup guide</button>
        {/if}
      </div>
    </li>
    <li data-step="session" data-state="todo">
      <span class="mark" aria-hidden="true">4</span>
      <div>
        <strong>Open your first session</strong>
        {#if serverOn && allowed}
          <p>Everything is ready.</p>
          <a class="btn" href="/sessions">Open sessions →</a>
        {:else}
          <p>Available once the steps above are done.</p>
        {/if}
      </div>
    </li>
  </ol>
</section>

<style>
  .readiness { margin: 24px 0; padding: 22px; border: 1px solid var(--line); border-radius: 14px; background: var(--card); }
  .eyebrow { margin: 0 0 7px; color: var(--accent-text); font-size: 0.68rem; font-weight: 700; letter-spacing: 0.12em; text-transform: uppercase; }
  h2 { margin: 0 0 18px; font: 500 1.7rem/1.15 var(--serif); letter-spacing: -0.02em; }
  ol { list-style: none; display: grid; gap: 4px; margin: 0; padding: 0; }
  li { display: grid; grid-template-columns: 30px minmax(0, 1fr); gap: 14px; padding: 14px 0; border-top: 1px solid var(--line); }
  li:first-child { border-top: 0; }
  .mark { display: grid; place-items: center; width: 28px; height: 28px; border: 1px solid var(--line); border-radius: 50%; color: var(--muted); font: 600 0.78rem var(--sans); }
  li[data-state='done'] .mark { background: var(--good); border-color: var(--good); color: var(--bg); }
  li[data-state='done'] strong { color: var(--muted); }
  strong { display: block; font-size: 0.92rem; }
  p { margin: 5px 0 10px; color: var(--muted); font-size: 0.84rem; line-height: 1.6; max-width: 62ch; }
  small { display: block; margin-top: 6px; color: var(--muted); font-size: 0.76rem; }
  li .btn.quiet { padding-left: 0; }
  .command { display: flex; align-items: center; justify-content: space-between; gap: 10px; max-width: 460px; padding: 6px 6px 6px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  code { font: 0.82rem var(--mono); overflow-wrap: anywhere; }
  @media (max-width: 560px) { .readiness { padding: 18px; } h2 { font-size: 1.45rem; } li { gap: 10px; } }
</style>
