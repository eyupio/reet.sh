<script lang="ts">
  import Skeleton from './Skeleton.svelte'
  import { onMount } from 'svelte'
  import { getCommunity, addSkill, type ListSkill, type SharedList } from './api'
  import {
    communityCountLabel, filterCommunityLists, filterCommunitySkills, type CommunityTab,
  } from './community'
  import { elapsed } from './time'
  import CollectionPanel from './CollectionPanel.svelte'
  import PageHeader from './PageHeader.svelte'
  import CopyButton from './CopyButton.svelte'
  import { installCommand } from './install'
  import { t } from './copy'

  let { canSave = false }: { canSave?: boolean } = $props()

  let saving = $state('')
  let saved = $state<string[]>([])
  let notice = $state('')
  let actionError = $state('')
  let lists = $state<SharedList[]>([])
  let skills = $state<ListSkill[]>([])
  let query = $state('')
  let tab = $state<CommunityTab>('lists')
  let loading = $state(true)
  let error = $state('')

  const searchedLists = $derived(filterCommunityLists(lists, query))
  const searchedSkills = $derived(filterCommunitySkills(skills, query))
  const countLabel = $derived(communityCountLabel(
    tab === 'lists' ? searchedLists.length : searchedSkills.length,
    tab === 'lists' ? lists.length : skills.length,
    tab,
  ))
  const countText = $derived(loading ? 'Reading the community…' : error ? 'Community unavailable' : countLabel)

  async function save(skill: ListSkill) {
    if (saving) return
    saving = skill.id
    actionError = ''
    try {
      await addSkill(skill.id, { bookmarked: true })
      saved = [...saved, skill.id]
      notice = `Saved ${skill.name} to your personal shelf.`
    } catch (e) {
      actionError = e instanceof Error ? e.message : String(e)
    } finally {
      saving = ''
    }
  }

  async function load() {
    loading = true
    error = ''
    try {
      const r = await getCommunity()
      lists = r.lists
      skills = r.skills
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  function updated(ts: number): string {
    const age = elapsed(ts)
    return age === 'just now' ? 'Updated just now' : `Updated ${age} ago`
  }

  onMount(() => { void load() })
</script>

<PageHeader eyebrow="Public community" title="Have a nosy." lede="Publicly published lists and their skills. Explore without an account; your personal shelf, teams and selected-people lists stay separate." />

{#if actionError}<p class="notice bad" role="alert">{actionError}</p>{/if}
{#if notice}<p class="notice good" role="status">{notice}</p>{/if}

<CollectionPanel label="Community" count={countText}>
  {#snippet toolbar()}
    <div class="community-toolbar">
      <label class="search">Search community
        <input type="search" bind:value={query} placeholder="Search skills or lists" />
      </label>
      <div class="tabs" role="group" aria-label="Show community">
        <button type="button" aria-pressed={tab === 'lists'} onclick={() => (tab = 'lists')}>Lists<small>{searchedLists.length}</small></button>
        <button type="button" aria-pressed={tab === 'skills'} onclick={() => (tab = 'skills')}>Skills<small>{searchedSkills.length}</small></button>
      </div>
    </div>
  {/snippet}

  {#if loading}
    <Skeleton label="Reading the community…" shape="rows" count={5} />
  {:else if error}
    <div class="state error" role="alert">
      <p>{error}</p>
      <button type="button" class="btn" onclick={load}>Try again</button>
    </div>
  {:else if tab === 'lists'}
    {#if searchedLists.length === 0}
      <div class="state">
        <h2>{query ? t('empty.community.title') : 'The community starts with a shared find.'}</h2>
        <p>{query ? 'Try a different search.' : 'Create a list and deliberately choose Public community to publish it. Existing private lists stay private.'}</p>
        <div class="state-actions">
          {#if query}<button type="button" class="btn" onclick={() => (query = '')}>Clear search</button>{/if}
          <a class="btn" href="/shared">Create or manage your lists</a>
        </div>
      </div>
    {:else}
      <div class="records">
        {#each searchedLists as list (list.id)}
          <a class="record list-record" href={`/l/${list.slug}`}>
            <div class="description">
              <h2>{list.name}</h2>
              <p>{list.description || 'A community skill collection.'}</p>
            </div>
            <div class="meta">
              <div class="meta-row"><span>{list.skillCount} {list.skillCount === 1 ? 'skill' : 'skills'}</span></div>
              <div class="meta-row"><time datetime={new Date(list.updatedAt).toISOString()} title={new Date(list.updatedAt).toLocaleString()}>{updated(list.updatedAt)}</time></div>
            </div>
            <span class="go" aria-hidden="true"></span>
          </a>
        {/each}
      </div>
    {/if}
  {:else if searchedSkills.length === 0}
    <div class="state">
      <h2>{query ? t('empty.community.title') : 'No skills from public lists yet.'}</h2>
      <p>{query ? 'Try a different search.' : 'Skills appear here when someone publishes a list containing them.'}</p>
      {#if query}<div class="state-actions"><button type="button" class="btn" onclick={() => (query = '')}>Clear search</button></div>{/if}
    </div>
  {:else}
    <div class="records">
      {#each searchedSkills as skill (skill.id)}
        <div class="record skill-record">
          <div class="description">
            <h2>{skill.name}</h2>
            <p class="source">{skill.source || 'Unknown source'}</p>
            <p>{skill.description}</p>
          </div>
          <div class="actions">
            {#if canSave}
              <button type="button" class="btn" disabled={!!saving || saved.includes(skill.id)} onclick={() => save(skill)}>{saved.includes(skill.id) ? 'Saved to your shelf' : saving === skill.id ? 'Saving…' : 'Save to personal shelf'}</button>
            {:else}<a class="btn" href="/skills">Open your shelf to save</a>{/if}
            {#if installCommand(skill)}<CopyButton text={installCommand(skill)!} label="Copy install command" />{/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</CollectionPanel>

<p class="hint">This community contains lists explicitly published on this reet site. It does not expose private collections or search other reet servers.</p>
<style>
  .community-toolbar { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 12px 16px; }
  .search { flex: 1 1 260px; display: grid; gap: 6px; color: var(--muted); font-size: 0.8rem; }
  .search input { width: 100%; }
  .tabs { display: inline-flex; gap: 2px; padding: 3px; border: 1px solid var(--line); border-radius: 9px; background: var(--bg); }
  .tabs button { display: inline-flex; align-items: center; gap: 6px; min-height: 34px; padding: 4px 10px; border: 0; border-radius: 6px; background: transparent; color: var(--muted); font: inherit; font-size: 0.8rem; cursor: pointer; }
  .tabs button:hover { color: var(--ink); }
  .tabs button[aria-pressed='true'] { background: var(--tint); color: var(--ink); font-weight: 600; }
  .tabs small { font-size: 0.75rem; font-variant-numeric: tabular-nums; }

  .records { display: grid; }
  .record { color: var(--ink); text-decoration: none; min-width: 0; }
  .list-record { display: grid; grid-template-columns: minmax(0, 1fr) clamp(180px, 24vw, 260px) 8px; align-items: center; gap: 16px; padding: 16px 2px; border-top: 1px solid var(--line-soft); }
  .list-record:first-child { border-top: 0; }
  .list-record:hover h2 { text-decoration: underline; text-underline-offset: 2px; }
  .skill-record { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 16px; padding: 16px 2px; border-top: 1px solid var(--line-soft); }
  .skill-record:first-child { border-top: 0; }
  .description { min-width: 0; }
  h2 { margin: 0 0 4px; font: 500 1.2rem/1.3 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; }
  .description p { margin: 0; color: var(--muted); font-size: 0.85rem; overflow-wrap: anywhere; }
  .description .source { margin-bottom: 3px; color: var(--muted); font: 0.75rem var(--mono); }
  .meta { display: grid; width: 100%; justify-items: end; gap: 5px; }
  .meta-row { display: flex; align-items: center; flex-wrap: wrap; justify-content: flex-end; gap: 5px 14px; color: var(--muted); font-size: 0.75rem; text-align: right; }
  .meta-row > * { flex: none; }
  .go { width: 7px; height: 7px; border-top: 2px solid var(--muted); border-right: 2px solid var(--muted); transform: rotate(45deg); }
  .actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
  .actions :global(.btn) { min-height: 38px; padding: 6px 12px; font-size: 0.8rem; }

  .state { padding: 26px 0 18px; max-width: 60ch; }
  .state h2 { font-size: 1.3rem; }
  .state p { margin: 6px 0 0; color: var(--muted); }
  .state-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
  .error p { margin: 0 0 12px; color: var(--ink); }

  .hint { margin: 12px 2px 0; color: var(--muted); font-size: 0.8rem; line-height: 1.5; }

  @media (prefers-reduced-motion: no-preference) {
    .tabs button { transition: background-color 0.15s, border-color 0.15s, color 0.15s; }
  }
  @media (max-width: 650px) {
    .community-toolbar { gap: 10px; }
    .search { flex-basis: 100%; }
    .list-record, .skill-record { grid-template-columns: minmax(0, 1fr); gap: 10px; padding: 14px 0; }
    .meta { justify-items: start; }
    .meta-row { justify-content: flex-start; text-align: left; }
    .go { display: none; }
  }
  @media (pointer: coarse) { .tabs button, .actions :global(.btn) { min-height: 44px; } }
</style>
