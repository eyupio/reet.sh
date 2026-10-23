<script lang="ts">
  import type { MachineHealth } from './api'
  import { age, bytes, fresh, missing, percent, uptime, value } from './health'
  let { health, serverNow }: { health?: MachineHealth; serverNow: number } = $props()
  const connection = $derived(health?.connection)
  const report = $derived(connection?.report)
  const isFresh = $derived(fresh(health, serverNow))
  const cpu = $derived(value(report?.cpuPercent))
  const memory = $derived(percent(report?.memory.totalBytes, report?.memory.availableBytes))
  const rtt = $derived(connection?.connected && connection.rttMillis != null ? connection.rttMillis : null)
</script>

<section class="health" class:stale={!isFresh} aria-label="Machine health">
  <div class="health-title"><span class="dot" class:connected={connection?.connected}></span><strong>{connection?.connected ? 'Private connection' : 'Awaiting connection'}</strong><span>{report?.scope === 'container' ? 'Container scope · ' : ''}{isFresh ? `Updated ${age(connection?.reportAt, serverNow)}` : report ? `Last known · ${age(connection?.reportAt, serverNow)}` : 'No resource samples yet'}</span></div>
  <dl>
    <div title="Controller-to-host application round trip over the private connection"><dt>Host RTT</dt><dd>{rtt == null ? '—' : rtt < 1 ? '<1' : Math.round(rtt)}<small>{rtt == null ? '' : ' ms'}</small></dd></div>
    <div title={cpu == null ? missing(report?.cpuPercent) : isFresh ? 'Host-wide CPU utilisation' : 'Last observed CPU, currently stale'}><dt>{report?.scope === 'container' ? 'Container CPU' : 'Host CPU'}</dt><dd>{cpu == null ? '—' : cpu.toFixed(1)}<small>{cpu == null ? '' : '%'}</small></dd></div>
    <div title={memory == null ? missing(report?.memory.totalBytes) : `${bytes(value(report?.memory.availableBytes))} available`}><dt>Memory used</dt><dd>{memory == null ? '—' : Math.round(memory)}<small>{memory == null ? '' : '%'}</small></dd></div>
    <div title="Space available to the agent user on the home filesystem"><dt>Disk available</dt><dd class="compact">{bytes(value(report?.disk.availableBytes))}</dd></div>
    <div title="Host uptime at the last resource observation, not agent uptime"><dt>Host uptime</dt><dd class="compact">{uptime(value(report?.hostUptimeSeconds))}</dd></div>
  </dl>
</section>

<style>
  .health { border:1px solid var(--line);border-radius:12px;background:var(--card);padding:16px 20px;min-width:0 }
  .health-title {display:flex;gap:8px;align-items:center;font-size:.75rem;flex-wrap:wrap;color:var(--muted)} .health-title strong{color:var(--ink);font-weight:600}.health-title>span:last-child{margin-left:auto}
  .dot{width:7px;height:7px;border-radius:50%;background:var(--muted)}.dot.connected{background:var(--good);box-shadow:0 0 0 3px color-mix(in srgb,var(--good) 12%,transparent)}
  dl{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:12px;margin:16px 0 0}dl>div{min-width:0;border-right:1px solid var(--line);padding-right:12px}dl>div:last-child{border:0}dt{font-size:.7rem;color:var(--muted);margin-bottom:6px}dd{font:500 1.5rem/1.2 var(--mono,monospace);margin:0;font-variant-numeric:tabular-nums}dd.compact{font-size:1.15rem}dd small{font-size:.75rem;color:var(--muted)}.stale dd{color:var(--muted)}
  @media(max-width:650px){.health{padding:14px}dl{grid-template-columns:repeat(3,minmax(0,1fr));gap:16px 10px}.health-title>span:last-child{margin-left:0;width:100%}dl>div{border:0;padding:0}dd{font-size:1.2rem}dd.compact{font-size:.95rem}}
</style>
