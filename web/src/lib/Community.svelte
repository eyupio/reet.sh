<script lang="ts">
  import { onMount } from 'svelte'
  import { getCommunity, addSkill, type ListSkill, type SharedList } from './api'
  import PageHeader from './PageHeader.svelte'
  import CopyButton from './CopyButton.svelte'
  import { installCommand } from './install'
  import { t } from './copy'
  let { canSave = false }: { canSave?: boolean } = $props()
  let saving = $state(''), saved = $state<string[]>([]), notice = $state(''), actionError = $state('')
  async function save(skill: ListSkill) { if (saving) return; saving = skill.id; actionError = ''; try { await addSkill(skill.id, { bookmarked: true }); saved = [...saved, skill.id]; notice = `Saved ${skill.name} to your personal shelf.` } catch (e) { actionError = e instanceof Error ? e.message : String(e) } finally { saving = '' } }
  let lists = $state<SharedList[]>([]), skills = $state<ListSkill[]>([])
  let query = $state(''), tab = $state('lists'), loading = $state(true), error = $state('')
  const term = $derived(query.toLowerCase().trim())
  const shownLists = $derived(lists.filter(l => `${l.name} ${l.description}`.toLowerCase().includes(term)))
  const shownSkills = $derived(skills.filter(s => `${s.name} ${s.source} ${s.description}`.toLowerCase().includes(term)))
  async function load() { loading = true; error = ''; try { const r = await getCommunity(); lists = r.lists; skills = r.skills } catch (e) { error = e instanceof Error ? e.message : String(e) } finally { loading = false } }
  onMount(() => { void load() })
</script>
<PageHeader eyebrow="Public community" title="Have a nosy." lede="Publicly published lists and their skills. Explore without an account; your personal shelf, teams and selected-people lists stay separate." />
<div class="controls"><div class="tabs" role="group" aria-label="Community content"><button type="button" class="btn" aria-pressed={tab === 'lists'} onclick={() => tab = 'lists'}>Lists · {lists.length}</button><button type="button" class="btn" aria-pressed={tab === 'skills'} onclick={() => tab = 'skills'}>Skills · {skills.length}</button></div><label>Search community <input type="search" bind:value={query} placeholder="Search skills or lists" /></label></div>
<p class="hint">This community contains lists explicitly published on this reet site. It does not expose private collections or search other reet servers.</p>
{#if actionError}<p role="alert">{actionError}</p>{/if}
{#if notice}<p role="status">{notice}</p>{/if}
{#if loading}<p role="status">Loading community…</p>{:else if error}<p role="alert">{error}</p><button type="button" class="btn" onclick={load}>Retry</button>{:else if tab === 'lists'}
  <div class="cards">{#each shownLists as list (list.id)}<article><span class="badge">Public community</span><h2><a href={`/l/${list.slug}`}>{list.name}</a></h2><p>{list.description || 'A community skill collection.'}</p><footer><span>{list.skillCount} skills</span><a href={`/l/${list.slug}`}>Explore list →</a></footer></article>{:else}<div class="empty"><h2>{term ? t('empty.community.title') : 'The community starts with a shared find.'}</h2><p>{term ? 'Try a different search.' : 'Create a list and deliberately choose Public community to publish it. Existing private lists stay private.'}</p><a class="btn" href="/shared">Create or manage your lists</a></div>{/each}</div>
{:else}
  <div class="cards">{#each shownSkills as skill (skill.id)}<article><span class="badge">From a public list</span><h2>{skill.name}</h2><p class="source">{skill.source || 'Unknown source'}</p><p>{skill.description}</p><div class="skill-actions">{#if canSave}<button type="button" class="btn" disabled={!!saving || saved.includes(skill.id)} onclick={() => save(skill)}>{saved.includes(skill.id) ? 'Saved to your shelf' : saving === skill.id ? 'Saving…' : 'Save to personal shelf'}</button>{:else}<a href="/skills">Open your shelf to save</a>{/if}{#if installCommand(skill)}<CopyButton text={installCommand(skill)!} label="Copy install command" />{/if}</div></article>{:else}<p class="empty">{term ? t('empty.community.title') : 'Skills appear here when someone publishes a list containing them.'}</p>{/each}</div>
{/if}
<style>
 .skill-actions { display:flex; flex-wrap:wrap; gap:10px; align-items:center; font-size:.8rem; }
 .controls { display:flex; gap:16px; align-items:end; justify-content:space-between; flex-wrap:wrap; margin-top:24px; } .tabs { display:flex; gap:8px; } [aria-pressed=true] { background:var(--tint); border-color:var(--accent); } label { display:grid; gap:6px; font-size:.8rem; } .hint { color:var(--muted); font-size:.8rem; line-height:1.5; }
 .cards { display:grid; grid-template-columns:repeat(auto-fit,minmax(min(100%,280px),1fr)); gap:16px; margin:24px 0; } article,.empty { padding:22px; border:1px solid var(--line); border-radius:12px; background:var(--card); min-width:0; } h2 { font:500 1.4rem/1.3 var(--serif); overflow-wrap:anywhere; } h2 a { color:var(--ink); text-decoration:none; } p { color:var(--muted); font-size:.85rem; line-height:1.6; overflow-wrap:anywhere; } .badge { color:var(--accent-text); font-size:.72rem; } footer { display:flex; justify-content:space-between; gap:10px; font-size:.8rem; } a { color:var(--accent-text); } .source { font-family:var(--mono); font-size:.75rem; }
</style>
