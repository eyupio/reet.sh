<script lang="ts">
  import { onMount } from 'svelte'
  import { getHosting, saveHostedSkill, type HostedSkill } from './api'
  import CopyButton from './CopyButton.svelte'
  import { installPrompt } from './install'
  import type { Skill } from './api'

  let { canWrite, onchange }: { canWrite: boolean; onchange?: () => void } = $props()
  let skills = $state<HostedSkill[]>([]), username = $state(''), loading = $state(true), error = $state(''), saving = $state(''), notice = $state('')
  let query = $state(''), showAll = $state(false)
  const visible = $derived(skills.filter(skill => `${skill.slug} ${skill.description}`.toLowerCase().includes(query.toLowerCase())))
  function url(skill: HostedSkill) { return `${location.origin}/${username}/${skill.slug}` }
  function prompt(skill: HostedSkill) { return installPrompt({ sourceType: 'hosted', url: url(skill), slug: skill.slug } as Skill) ?? '' }
  async function publish(skill: HostedSkill, published: boolean) {
    if (saving) return
    saving = skill.slug; error = ''; notice = ''
    try {
      // Read the latest content before changing visibility, preserving edits
      // made from the editor since this page loaded.
      const current = (await getHosting()).skills.find(item => item.slug === skill.slug)
      if (!current) throw new Error('This skill was removed. Refresh Skills to see the latest list.')
      const saved = await saveHostedSkill({ ...current, published })
      skills = skills.map(item => item.slug === saved.slug ? saved : item)
      notice = published ? `${saved.slug} is public. Copy its link or prompt to share it.` : `${saved.slug} is now a private draft. Its public link is no longer available.`
      onchange?.()
    } catch (e) { error = e instanceof Error ? e.message : String(e) }
    finally { saving = '' }
  }
  onMount(() => { void getHosting().then(data => { skills = data.skills; username = data.username }).catch(e => { error = e instanceof Error ? e.message : String(e) }).finally(() => loading = false) })
</script>

<section class="hosted" aria-labelledby="hosted-shelf-title">
  <header><div><h2 id="hosted-shelf-title">Skills you create <span>{skills.length}</span></h2><p>Write, publish and share your own skills here.</p></div>{#if canWrite}<div class="actions"><a class="btn" href="/hosting?import=1">Import skills</a><a class="btn" href="/hosting?new=1">Create a skill</a></div>{/if}</header>
  {#if loading}<p role="status">Loading your hosted skills…</p>
  {:else if !skills.length && !error}<p class="empty">No hosted skills yet. <a href="/hosting?new=1">Create your first skill</a> or <a href="/hosting?import=1">import a skill folder</a>.</p>{/if}
  {#if skills.length > 5}<label class="search">Find a hosted skill<input type="search" bind:value={query} placeholder="Search hosted skills" /></label>{/if}
  <div class="hosted-list">
    {#each showAll || query ? visible : visible.slice(0, 3) as skill (skill.slug)}
      <article>
        <div class="identity"><a class="name" href={`/hosting?edit=${encodeURIComponent(skill.slug)}`}>{skill.slug}</a><p>{skill.description || 'Add a description in the editor.'}</p></div>
        <div class="sharing">
          <label><span class:public={skill.published}>{skill.published ? 'Public' : 'Private draft'}</span><input type="checkbox" role="switch" checked={skill.published} aria-label={`Make ${skill.slug} public`} disabled={!canWrite || !!saving} onchange={async event => { const input = event.currentTarget; await publish(skill, input.checked); input.checked = !!skills.find(item => item.slug === skill.slug)?.published }} /></label>
          <small>{skill.published ? 'Anyone with the link can use this skill.' : 'Only you can access this draft.'}</small>
        </div>
        <div class="actions">
          <a class="btn quiet" href={`/hosting?edit=${encodeURIComponent(skill.slug)}`}>Edit</a>
          {#if skill.published}<CopyButton text={url(skill)} label="Copy share link" ariaLabel={`Copy share link for ${skill.slug}`} compact /><CopyButton text={prompt(skill)} label="Copy prompt" ariaLabel={`Copy prompt for ${skill.slug}`} compact />{:else}<button class="btn" disabled={!canWrite || !!saving} onclick={() => void publish(skill, true)}>{saving === skill.slug ? 'Publishing…' : 'Publish & share'}</button>{/if}
        </div>
      </article>
    {/each}
  </div>
  {#if visible.length > 3 && !query}<button class="btn quiet" onclick={() => showAll = !showAll}>{showAll ? 'Show fewer' : `Show all ${visible.length} hosted skills`}</button>{/if}
  {#if notice}<p class="feedback" role="status">{notice}</p>{/if}
  {#if error}<p class="feedback error" role="alert">{error}</p>{/if}
</section>

<style>
.hosted{border:1px solid var(--line);border-radius:12px;margin:24px 0;padding:18px;min-width:0}header{display:flex;justify-content:space-between;align-items:center;gap:16px;flex-wrap:wrap}h2{font:500 1.3rem/1.3 var(--serif);margin:0}h2 span{font:.75rem var(--mono);color:var(--muted);margin-left:6px}p{color:var(--muted);font-size:.82rem;line-height:1.5;margin:6px 0}a{color:var(--accent-text)}.actions{display:flex;flex-wrap:wrap;align-items:center;gap:8px}.hosted-list article{display:flex;align-items:center;gap:18px;flex-wrap:wrap;padding:16px 0;border-top:1px solid var(--line);margin-top:12px}.identity{flex:1 1 200px;min-width:0}.name{font-weight:650;font-size:.92rem;overflow-wrap:anywhere}.identity p{max-width:55ch}.sharing{flex:0 1 220px}.sharing label{display:flex;justify-content:space-between;gap:16px;align-items:center;font-size:.8rem;font-weight:600}.sharing small{display:block;color:var(--muted);font-size:.72rem;margin-top:4px}.public{color:var(--good)}input[role=switch]{appearance:none;position:relative;width:36px;height:22px;border:1px solid var(--line);border-radius:20px;background:var(--line);cursor:pointer;flex-shrink:0}input[role=switch]:checked{background:var(--accent-text);border-color:var(--accent-text)}input[role=switch]:after{content:"";position:absolute;top:3px;left:3px;width:14px;height:14px;border-radius:50%;background:var(--card)}input[role=switch]:checked:after{left:17px}.search{display:grid;gap:6px;font-size:.8rem;margin-top:18px}.empty{margin-top:14px}.feedback{padding:10px;border:1px solid var(--good);border-radius:8px}.feedback.error{border-color:var(--bad)}@media(max-width:600px){.sharing{flex:1 1 100%}.hosted-list article>.actions{width:100%}header>.actions{width:100%}}
</style>
