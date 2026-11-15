<script lang="ts">
  // Notifications on this device: turn them on, choose what it hears about, send a test, and see the other devices
  // that are on. Shown on the Notifications page, for an instance with accounts.
  import { onMount } from 'svelte'
  import { removePushDevice, savePushDevice, testPush, type PushDevice, type PushStatus } from './api'
  import { browserEnvironment, currentSubscription, disablePush, enablePush, readPush } from './push'
  import { pushSupport, type PushSupport } from './pushSupport'
  import { elapsed } from './time'

  let status = $state<PushStatus | null>(null)
  let device = $state<PushDevice | null>(null)
  let support = $state<PushSupport>('ok')
  let busy = $state('')
  let error = $state('')
  let notice = $state('')
  const since = (at: number) => { const ago = elapsed(at); return ago === 'just now' ? ago : `${ago} ago` }
  const others = $derived(status ? status.devices.filter(d => d.id !== device?.id) : [])

  async function load() {
    try {
      const read = await readPush()
      status = read.status
      device = read.device
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }
  onMount(() => { support = pushSupport(browserEnvironment()); void load() })

  async function run(name: string, work: () => Promise<void>) {
    busy = name
    error = ''
    notice = ''
    try { await work() } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { busy = '' }
  }
  const turnOn = () => run('on', async () => {
    if (!status) return
    await enablePush(status, { waiting: true, ended: true })
    await load()
    notice = 'On. This device will hear when a session needs you.'
  })
  const turnOff = () => run('off', async () => {
    await disablePush()
    await load()
    notice = 'Off for this device.'
  })
  const choose = (key: 'waiting' | 'ended', value: boolean) => run(key, async () => {
    const subscription = await currentSubscription()
    if (!subscription || !device) return
    device = await savePushDevice({ endpoint: subscription.endpoint, label: device.label, waiting: device.waiting, ended: device.ended, [key]: value })
  })
  const sendTest = () => run('test', async () => {
    const result = await testPush()
    notice = result.sent === result.devices
      ? `Sent to ${result.sent === 1 ? 'your device' : `your ${result.sent} devices`}. It should arrive within a few seconds.`
      : `Sent to ${result.sent} of ${result.devices} devices. A device that did not get it may have turned notifications off.`
    await load()
  })
  const remove = (item: PushDevice) => run(`remove-${item.id}`, async () => {
    await removePushDevice({ id: item.id })
    await load()
  })
</script>

{#if status?.available}
<section class="card" aria-labelledby="device-push-title">
  <div class="head">
    <h2 id="device-push-title">On this device</h2>
    {#if device}<span class="state on"><i aria-hidden="true"></i>On</span>{:else if support === 'ok'}<span class="state">Off</span>{/if}
  </div>
  <p class="muted">Hear when a session needs you, or has ended, while reet is closed. A tap on the notification opens that session.</p>

  {#if support === 'insecure'}
    <p class="blocked">Notifications need reet to be opened over https. Open it at its https address, then turn this on.</p>
  {:else if support === 'install'}
    <p class="blocked">On an iPhone or iPad, add reet to your Home Screen first: tap Share, then <strong>Add to Home Screen</strong>. Open reet from there and turn this on.</p>
  {:else if support === 'unsupported'}
    <p class="blocked">This browser cannot show notifications from a website, so there is nothing to turn on here.</p>
  {:else if device}
    <fieldset disabled={!!busy}>
      <legend>Tell this device when</legend>
      <label class="check"><input type="checkbox" checked={device.waiting} onchange={e => choose('waiting', e.currentTarget.checked)} /><span><strong>A session needs you</strong><small>An agent is waiting for an answer or an approval.</small></span></label>
      <label class="check"><input type="checkbox" checked={device.ended} onchange={e => choose('ended', e.currentTarget.checked)} /><span><strong>A session has ended</strong><small>It finished, or stopped with a problem, and you have not looked yet.</small></span></label>
    </fieldset>
    <div class="actions">
      <button type="button" class="btn" disabled={!!busy} aria-busy={busy === 'test'} onclick={sendTest}>{busy === 'test' ? 'Sending…' : 'Send a test'}</button>
      <button type="button" class="btn quiet" disabled={!!busy} aria-busy={busy === 'off'} onclick={turnOff}>{busy === 'off' ? 'Turning off…' : 'Turn off'}</button>
    </div>
  {:else}
    <div class="actions">
      <button type="button" class="btn primary" disabled={!!busy} aria-busy={busy === 'on'} onclick={turnOn}>{busy === 'on' ? 'Turning on…' : 'Turn on notifications'}</button>
    </div>
    <p class="muted small">Your browser will ask whether reet may show notifications.</p>
  {/if}

  <p class="said" role="status">{#if error}<span class="bad">{error}</span>{:else}{notice}{/if}</p>

  {#if others.length}
    <h3>Your other devices</h3>
    <ul class="devices">
      {#each others as item (item.id)}
        <li>
          <span><strong>{item.label || 'A browser'}</strong><small>Turned on {since(item.createdAt)}</small></span>
          <button type="button" class="btn quiet" disabled={!!busy} aria-label={`Remove ${item.label || 'this browser'}`} onclick={() => remove(item)}>Remove</button>
        </li>
      {/each}
    </ul>
  {/if}
  <p class="muted small">Nothing about a session passes through your browser’s push service. The device is woken, then asks this reet what to show.</p>
</section>
{/if}

<style>
  .card { border: 1px solid var(--line); border-radius: 12px; background: var(--card); padding: 20px; min-width: 0; }
  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; }
  h2 { margin: 0; font: 500 1.4rem/1.2 var(--serif); letter-spacing: -0.02em; }
  h3 { margin: 20px 0 0; font: 600 0.875rem/1.4 var(--sans); color: var(--muted); }
  .muted { margin: 8px 0 0; color: var(--muted); font-size: 0.875rem; line-height: 1.6; }
  .small { margin-top: 12px; }
  .state { display: inline-flex; align-items: center; gap: 6px; padding: 2px 10px; border: 1px solid var(--line); border-radius: 999px; color: var(--muted); font-size: 0.875rem; }
  .state.on { color: var(--good); border-color: color-mix(in srgb, var(--good) 45%, var(--line)); font-weight: 600; }
  .state i { width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
  .blocked { margin: 14px 0 0; padding: 12px 14px; border: 1px solid var(--line); border-left: 3px solid var(--warn); border-radius: 8px; font-size: 0.875rem; line-height: 1.6; }
  fieldset { margin: 16px 0 0; padding: 4px 12px; border: 1px solid var(--line); border-radius: 8px; min-width: 0; }
  legend { padding: 0 5px; color: var(--muted); font-size: 0.875rem; font-weight: 600; }
  .check { display: flex; align-items: flex-start; gap: 10px; min-height: 44px; padding: 10px 0; border-top: 1px solid var(--line); font-size: 0.875rem; cursor: pointer; }
  .check:first-of-type { border-top: 0; }
  .check input { flex: none; width: 18px; height: 18px; margin: 2px 0 0; accent-color: var(--accent); }
  .check span { display: grid; gap: 2px; min-width: 0; }
  .check small, .devices small { color: var(--muted); font-size: 0.875rem; line-height: 1.45; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; }
  .said { min-height: 1.4em; margin: 12px 0 0; font-size: 0.875rem; line-height: 1.5; color: var(--muted); }
  .said:empty { display: none; }
  .bad { color: var(--bad); }
  .devices { list-style: none; margin: 8px 0 0; padding: 0; display: grid; }
  .devices li { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 0; border-top: 1px solid var(--line); font-size: 0.875rem; }
  .devices li:first-child { border-top: 0; }
  .devices span { display: grid; gap: 2px; min-width: 0; }
  @media (max-width: 560px) { .card { padding: 16px; } .actions .btn { flex: 1 1 auto; } }
</style>
