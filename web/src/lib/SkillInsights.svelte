<script lang="ts">
  import { onMount } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import { buddyTotals, receiptName, skillInsights, sourceName, type BuddyRecord } from './skillBuddy'
  let sessionUsage = $state<Record<string,{inputTokens:number;outputTokens:number;cachedTokens:number;reported:boolean}>>({})
  let records = $state<BuddyRecord[]>([]), loading = $state(true), error = $state(''), filter = $state('all')
  const linkedUsage = $derived(Object.values(sessionUsage).filter(u=>u.reported))
  const linkedTokens = $derived(linkedUsage.reduce((n,u)=>n+u.inputTokens+u.outputTokens+u.cachedTokens,0))
  const totals = $derived(buddyTotals(records))
  const shown = $derived(records.filter(r => filter === 'all' || (filter === 'factory' ? r.scope.startsWith('factory:') : r.scope.startsWith('session:'))))
  const ranked = $derived.by(() => {
    const map = new Map<string, { name: string; source: string; count: number }>()
    for (const r of shown) if (['drafted', 'selected', 'prepared'].includes(r.skill.state ?? '')) {
      const row = map.get(r.skill.id) ?? { name: r.skill.name, source: r.skill.source, count: 0 }; row.count++; map.set(r.skill.id, row)
    }
    return [...map.values()].sort((a, b) => b.count - a.count).slice(0, 6)
  })
  async function load() { loading = true; error = ''; try { const out = await skillInsights(AbortSignal.timeout(15000)); records = out.records; sessionUsage = out.sessionUsage ?? {} } catch (e) { error = e instanceof Error ? e.message : 'Insights could not be read.' } finally { loading = false } }
  onMount(() => { void load() })
  const href = (r: BuddyRecord) => r.sessionId ? `/sessions?session=${encodeURIComponent(r.sessionId)}` : '/factory'
</script>

<PageHeader group="Workspace" title="Skill insights" lede="The know-how behind your work. See what helped, where it was chosen, and what reet can actually verify."/>
<div class="insights">
  <section class="hero"><div><p class="eyebrow">Gaffer’s notebook</p><h2>{totals.chosen ? `${totals.chosen} skill choices across your work` : 'Start with the next piece of work'}</h2><p>{totals.chosen ? 'Follow a choice from its recommendation to the session where it was prepared.' : 'Open an agent session or select a Factory stage. Gaffer suggests relevant skills there, and records the choices you make.'}</p></div><a class="btn quiet" href="/factory">Open Factory</a></section>
  <div class="stats" aria-label="Skill activity">
    {#each [{n:totals.suggested,label:'Suggestions recorded'},{n:totals.chosen,label:'Chosen for work'},{n:totals.prepared,label:'Stage prompts prepared'},{n:totals.sessions,label:'Linked sessions'}] as stat}<article><strong>{stat.n}</strong><span>{stat.label}</span></article>{/each}
  </div>
  <div class="toolbar"><div role="group" aria-label="Work type">{#each [{id:'all',label:'All work'},{id:'session',label:'Agent sessions'},{id:'factory',label:'Factory stages'}] as tab}<button class:active={filter===tab.id} aria-pressed={filter===tab.id} onclick={()=>filter=tab.id}>{tab.label}</button>{/each}</div><button class="btn quiet" disabled={loading} onclick={() => void load()}>{loading ? 'Reading…' : 'Refresh'}</button></div>
  {#if error}<p class="notice bad" role="alert">{error}</p>{/if}
  <div class="columns">
    <section class="panel"><h2>Skills you reach for</h2><p class="sub">Choices made in the work shown here.</p>
      {#each ranked as row}<div class="rank"><div><strong>{row.name}</strong><small>{sourceName(row.source)}</small></div><span>{row.count}</span><meter min="0" max={ranked[0]?.count || 1} value={row.count} aria-label={`${row.name}: ${row.count} choices`}></meter></div>{:else}<p class="empty">No choices yet. Reet will suggest relevant skills as you work.</p><a href="/sessions">Open agent sessions →</a>{/each}
    </section>
    <section class="panel evidence"><p class="eyebrow">Evidence, with context</p><h2>What these receipts establish</h2><p>Choosing guidance records your intent. A prepared stage prompt records that reet included it. Neither proves the agent followed it.</p><p class="usage-number">{linkedUsage.length ? `${linkedTokens.toLocaleString()} reported tokens across ${linkedUsage.length} linked sessions` : 'No usage reported by linked sessions yet'}</p><p>These are whole-session totals, including work before a skill was chosen. Token counts and check results belong to the whole session. Reet does not claim a skill saved time, money or tokens without a comparable baseline.</p><a href="/usage">See agent usage →</a></section>
  </div>
  <section class="panel"><div class="activity-head"><h2>Recent skill activity</h2><span>Up to 500 receipts · kept for 90 days</span></div>
    <div class="activity">{#each shown.slice(0,50) as rec (`${rec.scope}:${rec.skill.id}`)}<a class="receipt" href={href(rec)}><span class="dot" class:chosen={rec.skill.state !== 'suggested' && rec.skill.state !== 'dismissed'}></span><div><strong>{rec.skill.name}</strong><small>{sourceName(rec.skill.source)} · {rec.stage || 'Agent session'} · revision {rec.skill.revision || 'not loaded'}</small></div><span class="receipt-state">{receiptName(rec.skill.state)}<small>{new Date(rec.at).toLocaleString()}</small></span></a>{:else}<p class="empty">{loading ? 'Reading your skill activity…' : 'Nothing recorded for this view yet.'}</p>{/each}</div>
  </section>
</div>

<style>
  /* UX polish informed by Unslop, Damir Sokolovsky (@heydamir), CC BY 4.0: https://github.com/heydamir/unslop */
  .stats strong{font-variant-numeric:tabular-nums;font-weight:500}.hero h2{font-weight:500}.receipt:hover{background:var(--bg)}.receipt:focus-visible{outline:2px solid var(--accent);outline-offset:4px}.rank strong{font-weight:500}.usage-number{font-variant-numeric:tabular-nums;color:var(--ink)!important}
  .insights{max-width:1180px;margin:0 auto 40px;display:grid;gap:20px}.hero{display:flex;justify-content:space-between;align-items:center;gap:20px;padding:28px;border:1px solid var(--line);border-radius:16px;background:var(--card)}.hero h2{font-size:clamp(24px,3vw,32px);margin:8px 0 12px}.hero p{max-width:650px;color:var(--muted);margin:0}.eyebrow{font-size:14px;color:var(--accent)!important}.stats{display:grid;grid-template-columns:repeat(4,1fr);gap:12px}.stats article,.panel{border:1px solid var(--line);border-radius:12px;background:var(--card);padding:20px}.stats strong{font-size:32px;display:block}.stats span,.sub,small{color:var(--muted);font-size:14px}.toolbar{display:flex;justify-content:space-between;align-items:center;gap:10px}.toolbar [role=group]{display:flex;gap:4px;flex-wrap:wrap}.toolbar [role=group] button{border:1px solid transparent;border-radius:8px;padding:8px 12px;background:transparent;color:var(--muted);font:inherit;cursor:pointer}.toolbar button.active{border-color:var(--line);background:var(--card);color:var(--ink)}.columns{display:grid;grid-template-columns:2fr 1fr;gap:20px}.panel h2{font-size:16px;margin:0 0 8px}.panel p{line-height:1.65}.sub{margin-top:0}.rank{display:grid;grid-template-columns:1fr auto;gap:8px;margin:24px 0}.rank strong{font-size:14px}.rank small,.receipt small{display:block;margin-top:4px}meter{grid-column:1/-1;width:100%;height:8px;accent-color:var(--accent)}.evidence{background:var(--card)}.evidence p{font-size:14px;color:var(--muted)}.empty{padding:20px 0;color:var(--muted);font-size:14px}.activity-head{display:flex;justify-content:space-between;gap:10px;flex-wrap:wrap}.activity-head span{font-size:14px;color:var(--muted)}.receipt{display:flex;align-items:center;gap:12px;text-decoration:none;color:var(--ink);padding:16px 0;border-top:1px solid var(--line)}.receipt:first-child{margin-top:16px}.receipt strong{font-size:14px}.receipt-state{margin-left:auto;font-size:14px;text-align:right}.dot{width:8px;height:8px;border-radius:50%;background:var(--muted);flex-shrink:0}.dot.chosen{background:var(--accent)}a{color:var(--accent)}@media(max-width:650px){.hero{padding:20px}.stats{grid-template-columns:repeat(2,1fr)}.columns{grid-template-columns:1fr}.receipt{flex-wrap:wrap}.receipt-state{margin-left:20px;text-align:left}.insights{gap:16px}.stats article,.panel{padding:16px}}
</style>
