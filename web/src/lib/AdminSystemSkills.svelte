<script lang="ts">
  import { onMount } from 'svelte'
  import { terminalRequest } from './terminal/api'
  import type { BuddySkill } from './skillBuddy'
  type SystemSkill = BuddySkill & { stage: string; stages?: string[]; keywords?: string; attribution?: string; disabled?: boolean }
  type Answer = { skills: SystemSkill[]; defaults: SystemSkill[]; version: number }
  let answer = $state<Answer | null>(null), chosen = $state<SystemSkill | null>(null), error = $state(''), note = $state(''), busy = $state(false), query = $state('')
  const stages = ['plan','design','code','review','test','pr','babysit','merge']
  const shown = $derived((answer?.skills ?? []).filter(s => `${s.name} ${s.description} ${s.keywords}`.toLowerCase().includes(query.toLowerCase())))
  async function load() { busy = true; error = ''; try { answer = await (await terminalRequest('/api/v2/admin/system-skills')).json() } catch(e) { error = e instanceof Error ? e.message : 'System skills could not be read.' } finally { busy = false } }
  function edit(skill?: SystemSkill) { chosen = skill ? { ...skill } : { id: 'builtin:', name:'',description:'',source:'builtin',revision:'',reason:'',stage:'code',instructions:'',keywords:'',attribution:'',disabled:false }; error = ''; note = '' }
  async function save(reset = false) {
    if (!chosen || !answer) return
    busy = true; error = ''; note = ''
    try { answer = await (await terminalRequest('/api/v2/admin/system-skills', { method:'PUT', body:JSON.stringify({skill:chosen,version:answer.version,reset}) })).json(); chosen = null; note = reset ? 'Repository default restored. Existing work keeps the revision it selected.' : 'System skill saved. New choices use this revision; existing work keeps its selected guidance.' }
    catch(e) { error = e instanceof Error ? e.message : 'The skill could not be saved.' }
    finally { busy = false }
  }
  onMount(() => { void load() })
</script>

<p class="intro">Maintain the small, portable skills Gaffer can recommend to everyone on this instance. Repository defaults are built in; your edits stay here across upgrades. Personal and community libraries remain separate.</p>
{#if error}<p class="notice bad" role="alert">{error} <button class="btn quiet" onclick={() => void load()}>Reload catalogue</button></p>{/if}
{#if note}<p class="notice" role="status">{note}</p>{/if}
<div class="tools"><label>Find a system skill<input type="search" bind:value={query} placeholder="Search system skills"/></label><button class="btn" disabled={busy} onclick={() => edit()}>New system skill</button></div>
<div class="layout">
  <div class="catalogue">{#each shown as skill (skill.id)}<button class="entry" class:selected={chosen?.id===skill.id} onclick={() => edit(skill)}><span class="status" class:off={skill.disabled}></span><div><strong>{skill.name}</strong><p>{skill.description}</p><small>{skill.stage} · {skill.disabled ? 'Disabled' : 'Available'} · revision {skill.revision}</small></div><span aria-hidden="true">›</span></button>{:else}<p>{busy ? 'Reading system skills…' : 'No system skills match this search.'}</p>{/each}</div>
  <section class="editor" aria-label="System skill editor">
    {#if chosen}<form onsubmit={event => { event.preventDefault(); void save() }}>
      <h2>{answer?.skills.some(s => s.id===chosen?.id) ? 'Edit system skill' : 'New system skill'}</h2>
      <p class="muted">These instructions are available to all users. Do not include private project information or credentials.</p>
      <label>Stable ID<input bind:value={chosen.id} required pattern={'builtin:[a-z][a-z0-9-]{0,59}'} readonly={answer?.skills.some(s => s.id===chosen?.id)} placeholder="builtin:api-review"/></label>
      <label>Name<input bind:value={chosen.name} required maxlength="80"/></label>
      <label>When it helps<textarea bind:value={chosen.description} maxlength="600" rows="2"></textarea></label>
      <label>Factory stage<select bind:value={chosen.stage}>{#each stages as stage}<option value={stage}>{stage}</option>{/each}</select></label>
      <fieldset><legend>Also suggest during</legend><div class="stage-options">{#each stages.filter(s => s !== chosen?.stage) as stage}<label class="check"><input type="checkbox" checked={chosen.stages?.includes(stage) ?? false} onchange={event => { if(chosen) chosen.stages = event.currentTarget.checked ? [...(chosen.stages ?? []),stage] : (chosen.stages ?? []).filter(s => s !== stage) }}/>{stage}</label>{/each}</div></fieldset>
      <label>Match keywords<input bind:value={chosen.keywords} maxlength="600" placeholder="api endpoints contracts"/></label>
      <label>Delivery<select bind:value={chosen.source}><option value="builtin">Built-in guidance — no install needed</option><option value="curated">Curated upstream source — discovery only</option></select></label>
      {#if chosen.source === 'curated'}<label>Upstream source URL<input type="url" bind:value={chosen.url} required placeholder="https://github.com/author/repo/blob/main/skills/name/SKILL.md"/></label><p class="muted">Offered as a source to explore. Reet does not install it or claim its scripts and hooks are available.</p>
      {:else}<label>Instructions<textarea class="instructions" bind:value={chosen.instructions} required rows="8"></textarea></label>
      <p class="muted">{new TextEncoder().encode(chosen.instructions ?? '').length} / 1,000 bytes. Keep guidance focused so it fits alongside the user's task.</p>{/if}
      <label>Attribution or research source<input bind:value={chosen.attribution} maxlength="1000" placeholder="Author and source, if applicable"/></label>
      <label class="check"><input type="checkbox" bind:checked={chosen.disabled}/> Disable recommendations for this skill</label>
      <div class="actions"><button class="btn" type="submit" disabled={busy}>{busy ? 'Saving…' : 'Save skill'}</button><button class="btn quiet" type="button" onclick={() => chosen=null}>Cancel</button>
      {#if answer?.defaults.some(s => s.id===chosen?.id)}<button class="btn quiet" type="button" disabled={busy} onclick={() => void save(true)}>Restore repo default</button>{/if}</div>
    </form>{:else}<div class="welcome"><h2>System guidance for every stage</h2><p>Choose a skill to inspect its instructions, change its stage or keywords, or disable it. Selected instructions are pinned to the work that uses them.</p><p>Nothing here installs a plugin, runs a command or changes an agent's permissions.</p></div>{/if}
  </section>
</div>

<style>
  fieldset{border:0;padding:0;margin:0 0 16px}legend{font-size:14px;margin-bottom:8px}.stage-options{display:flex;flex-wrap:wrap;gap:8px 16px}.stage-options label{margin:0}
  .intro{max-width:850px;line-height:1.7;color:var(--muted)}.tools{display:flex;align-items:end;justify-content:space-between;gap:16px;margin:24px 0}.tools label{flex:1;max-width:440px}.layout{display:grid;grid-template-columns:minmax(260px,1fr) minmax(340px,1.3fr);gap:24px}.catalogue{display:grid;gap:8px;align-content:start}.entry{display:flex;text-align:left;gap:12px;padding:16px;width:100%;border:1px solid var(--line);border-radius:10px;background:var(--card);color:var(--ink);font:inherit;cursor:pointer}.entry.selected{border-color:var(--accent);background:color-mix(in srgb,var(--accent) 6%,var(--card))}.entry div{flex:1}.entry p{font-size:14px;color:var(--muted);margin:6px 0}.entry small,.muted{font-size:14px;color:var(--muted)}.status{width:7px;height:7px;background:var(--accent);border-radius:50%;margin-top:7px;flex-shrink:0}.status.off{background:var(--muted)}.editor{border:1px solid var(--line);border-radius:12px;background:var(--card);padding:24px;align-self:start;position:sticky;top:20px}label{display:flex;flex-direction:column;gap:6px;margin-bottom:16px;font-size:14px}input,textarea,select{width:100%;box-sizing:border-box;background:var(--bg);color:var(--ink);border:1px solid var(--line);border-radius:7px;padding:10px;font:inherit}.instructions{font:14px/1.65 var(--mono,monospace);resize:vertical}.check{min-height:44px;flex-direction:row;align-items:center}.check input{width:auto}.actions{display:flex;flex-wrap:wrap;gap:8px}h2{font-size:20px;margin-top:0}.welcome{padding:24px 0}.welcome p{color:var(--muted);line-height:1.7}@media(max-width:800px){.layout{grid-template-columns:1fr}.editor{position:static}.tools{align-items:stretch;flex-direction:column}.tools label{max-width:none}}
</style>
