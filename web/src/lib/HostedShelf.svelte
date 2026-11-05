<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import { getHosting, saveHostedSkill, type HostedSkill, type SyncState } from './api'
  import type { InstallationReport } from './machines'
  import { writtenSkillWhere } from './skillSync'

  let { canWrite, onchange, reports = [], snapshot }: {
    canWrite: boolean
    onchange?: () => void
    /** Observed machine reports, for the same skill ids the shelf cards use. */
    reports?: InstallationReport[]
    /** Saved sync selections, read separately from what a machine last reported. */
    snapshot?: SyncState
  } = $props()
  let skills = $state<HostedSkill[]>([]), username = $state(''), loading = $state(true), error = $state(''), saving = $state(''), notice = $state('')
  let query = $state(''), showAll = $state(false)
  const drafts = $derived(skills.filter(skill => !skill.published))
  const visible = $derived(drafts.filter(skill => `${skill.slug} ${skill.description}`.toLowerCase().includes(query.toLowerCase())))
  function hostedId(skill: HostedSkill) { return `hosted/${location.host.toLowerCase()}/${username}/${skill.slug}` }
  // The id a draft has as a sync candidate (`hosted/<host>/<user>/<slug>`), so Sync can pre-select it.
  function syncHref(skill: HostedSkill) { return `/sync?skill=${encodeURIComponent(hostedId(skill))}` }
  function whereLine(skill: HostedSkill): string { return writtenSkillWhere(hostedId(skill), snapshot, reports) }
  async function publish(skill: HostedSkill) {
    if (saving) return
    saving = skill.slug; error = ''; notice = ''
    try {
      // Read the latest content before changing visibility, preserving edits
      // made from the editor since this page loaded.
      const current = (await getHosting()).skills.find(item => item.slug === skill.slug)
      if (!current) throw new Error('This skill was removed. Refresh Skills to see the latest list.')
      const saved = await saveHostedSkill({ ...current, published: true })
      skills = skills.map(item => item.slug === saved.slug ? saved : item)
      notice = `${saved.slug} is published. It is now on your shelf.`
      onchange?.()
    } catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { saving = '' }
  }
  onMount(() => { void getHosting().then(data => { skills = data.skills; username = data.username }).catch(e => { error = e instanceof Error ? e.message : String(e) }).finally(() => loading = false) })
</script>

<section class="hosted" aria-labelledby="hosted-shelf-title">
  <header><div><h2 id="hosted-shelf-title">Your drafts <span>{drafts.length}</span></h2><p>Private until published. Published skills appear on the shelf.</p></div>{#if canWrite}<div class="actions"><a class="btn" href="/hosting?import=1">Import skills</a><a class="btn" href="/hosting?new=1">Create a skill</a></div>{/if}</header>
  {#if loading}<Skeleton label="Loading your skills…" shape="cards" count={3} />
  {:else if !drafts.length && !error}<p class="empty">No drafts. Published skills are on the shelf. <a href="/hosting?new=1">Write a skill</a> or <a href="/hosting?import=1">import a folder</a>.</p>{/if}
  {#if drafts.length > 5}<label class="search">Find a skill<input type="search" bind:value={query} placeholder="Search your skills" /></label>{/if}
  <div class="hosted-list">
    {#each showAll || query ? visible : visible.slice(0, 3) as skill (skill.slug)}
      <article>
        <div class="identity"><a class="name" href={`/hosting?edit=${encodeURIComponent(skill.slug)}`}>{skill.slug}</a><p>{skill.description || 'Add a description in the editor.'}</p><p class="where" title="Sync plans and the last complete machine scans are separate. Neither proves the files are there.">{whereLine(skill)}</p></div>
        <div class="sharing">
          <label><span>Private draft</span><input type="checkbox" role="switch" checked={skill.published} aria-label={`Make ${skill.slug} public`} disabled={!canWrite || !!saving} onchange={() => void publish(skill)} /></label>
          <small>Only you can access this draft.</small>
        </div>
        <div class="actions">
          <a class="btn quiet" href={`/hosting?edit=${encodeURIComponent(skill.slug)}`}>Edit</a>
          <a class="btn quiet" href={syncHref(skill)} title="Drafts sync privately from this reet.sh; nothing is published.">Sync to machines…</a>
          <button class="btn" disabled={!canWrite || !!saving} onclick={() => void publish(skill)}>{saving === skill.slug ? 'Publishing…' : 'Publish & share'}</button>
        </div>
      </article>
    {/each}
  </div>
  {#if visible.length > 3 && !query}<button class="btn quiet" onclick={() => showAll = !showAll}>{showAll ? 'Show fewer' : `Show all ${visible.length} skills`}</button>{/if}
  {#if notice}<p class="feedback" role="status">{notice}</p>{/if}
  {#if error}<p class="feedback error" role="alert">{error}</p>{/if}
</section>

<style>
.hosted{border:1px solid var(--line);border-radius:12px;margin:24px 0;padding:18px;min-width:0}header{display:flex;justify-content:space-between;align-items:center;gap:16px;flex-wrap:wrap}h2{font:500 1.3rem/1.3 var(--serif);margin:0}h2 span{font:0.875rem var(--mono);color:var(--muted);margin-left:6px}p{color:var(--muted);font-size:0.875rem;line-height:1.5;margin:6px 0}a{color:var(--accent-text)}.actions{display:flex;flex-wrap:wrap;align-items:center;gap:8px}.hosted-list article{display:flex;align-items:center;gap:18px;flex-wrap:wrap;padding:16px 0;border-top:1px solid var(--line);margin-top:12px}.identity{flex:1 1 200px;min-width:0}.name{font-weight:650;font-size:.92rem;overflow-wrap:anywhere}.identity p{max-width:55ch}.identity .where{color:var(--muted);font-size:0.875rem;margin:3px 0 0}.sharing{flex:0 1 220px}.sharing label{display:flex;justify-content:space-between;gap:16px;align-items:center;font-size:0.875rem;font-weight:600}.sharing small{display:block;color:var(--muted);font-size:0.875rem;margin-top:4px}input[role=switch]{appearance:none;position:relative;width:36px;height:22px;border:1px solid var(--line);border-radius:20px;background:var(--line);cursor:pointer;flex-shrink:0}input[role=switch]:checked{background:var(--accent-text);border-color:var(--accent-text)}input[role=switch]:after{content:"";position:absolute;top:3px;left:3px;width:14px;height:14px;border-radius:50%;background:var(--card)}input[role=switch]:checked:after{left:17px}.search{display:grid;gap:6px;font-size:0.875rem;margin-top:18px}.empty{margin-top:14px}.feedback{padding:10px;border:1px solid var(--good);border-radius:8px}.feedback.error{border-color:var(--bad)}@media(max-width:600px){.sharing{flex:1 1 100%}.hosted-list article>.actions{width:100%}header>.actions{width:100%}}
</style>
