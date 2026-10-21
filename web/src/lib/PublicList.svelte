<script lang="ts">
  import { onMount } from 'svelte'
  import PageHeader from './PageHeader.svelte'
  import { getPublicList, type ListDetail } from './api'
  import CopyButton from './CopyButton.svelte'
  import { copyText } from './clipboard'
  import { installCommand } from './install'
  import { fmt } from './shelf'

  let { slug }: { slug: string } = $props()

  let detail = $state<ListDetail | null>(null)
  let loading = $state(true)
  let error = $state('')
  let status = $state('')

  const commands = $derived((detail?.skills ?? []).map(installCommand).filter((x): x is string => Boolean(x)))

  onMount(() => {
    document.title = 'Shared list · reet.sh'
    void (async () => {
      try {
        detail = await getPublicList(slug)
        document.title = `${detail.name} · reet.sh`
      } catch (e) {
        error = e instanceof Error ? e.message : String(e)
      } finally {
        loading = false
      }
    })()
    return () => {
      document.title = 'reet.sh – your shelf for AI skills'
    }
  })

  async function copyAll() {
    if (!commands.length) return
    if (await copyText(commands.join('\n'))) status = `Copied ${commands.length} install ${commands.length === 1 ? 'command' : 'commands'}.`
    else error = 'Could not copy to the clipboard.'
  }
</script>

{#if loading}
  <p class="empty" role="status">Loading shared list…</p>
{:else if !detail}
  <section class="missing">
<PageHeader eyebrow="Shared list" title="That link is not available." lede={error || 'The list may be private, deleted, or link sharing may be turned off.'} />
    <p><a class="back" href="/">Go to reet.sh</a></p>
  </section>
{:else}
  <article class="public">
    <PageHeader eyebrow="Read-only shared list" title={detail.name} lede={detail.description}>
      {#snippet meta()}{#if detail}<span class="badge">{detail.skillCount} {detail.skillCount === 1 ? 'skill' : 'skills'} · {detail.memberCount} {detail.memberCount === 1 ? 'person' : 'people'}</span>{/if}{/snippet}
    </PageHeader>

    <div class="actions">
      <button type="button" class="btn primary" onclick={copyAll} disabled={!commands.length}>Copy all install commands</button>
      <a class="btn" href="/">Open your own shelf</a>
    </div>
    {#if status}<p class="status" role="status">{status}</p>{/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}

    <p class="privacy">Only the skills and list notes are shown. Member names, emails and personal notes stay private.</p>

    <ol class="entries">
      {#each detail.skills as skill (skill.id)}
        <li class="entry">
          <div class="entry-head">
            <div>
              <h2>
                {#if skill.url.startsWith('https://www.skills.sh/')}
                  <a href={skill.url} target="_blank" rel="noopener noreferrer">{skill.name}<span class="vh"> (opens skills.sh)</span></a>
                {:else}{skill.name}{/if}
              </h2>
              <p class="source"><code>{skill.source}</code>{#if skill.installs > 0}<span>· {fmt(skill.installs)} installs</span>{/if}</p>
            </div>
            {#if skill.useCount > 0}
              <span class="adoption" title="{skill.useCount} of {skill.memberCount} list members marked this in use">{skill.useCount} of {skill.memberCount} use this</span>
            {/if}
          </div>
          {#if skill.description}<p class="desc">{skill.description}</p>{/if}
          {#if skill.note}<p class="note">{skill.note}</p>{/if}
          {#if installCommand(skill)}
            <div class="command"><code>{installCommand(skill)}</code><CopyButton text={installCommand(skill)!} compact /></div>
          {/if}
        </li>
      {/each}
    </ol>
  </article>
{/if}

<style>
  .public, .missing { margin-top: 0; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 20px; }
  .actions .btn { display: inline-flex; align-items: center; justify-content: center; text-decoration: none; }
  .actions a.btn { color: var(--ink); }
  .status { margin: 14px 0 0; color: var(--good); font-size: 0.88rem; }
  .error { margin: 14px 0 0; color: var(--bad); font-size: 0.88rem; }
  .privacy { margin: 18px 0 0; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; background: color-mix(in srgb, var(--bg) 45%, var(--card)); color: var(--muted); font-size: 0.8rem; }
  .empty { text-align: center; color: var(--muted); font: italic 1.1rem/1.5 var(--serif); padding: 56px 0; }
  .back { color: var(--accent); font-weight: 600; text-underline-offset: 3px; }

  .entries { list-style: none; margin: 20px 0 0; padding: 0; display: grid; gap: 14px; }
  .entry { padding: 22px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--card); box-shadow: 0 18px 50px -48px rgb(0 0 0 / 0.4); }
  .entry-head { display: flex; gap: 12px; align-items: flex-start; justify-content: space-between; }
  .entry h2 { margin: 0; font: 500 1.4rem/1.25 var(--serif); letter-spacing: -0.01em; overflow-wrap: anywhere; }
  .entry h2 a { color: inherit; text-decoration: none; }
  .entry h2 a:hover { color: var(--accent); text-decoration: underline; text-underline-offset: 3px; }
  .source { display: flex; flex-wrap: wrap; gap: 6px; margin: 4px 0 0; color: var(--muted); font-size: 0.76rem; }
  .source code { overflow-wrap: anywhere; }
  .adoption { flex: none; padding: 3px 8px; border: 1px solid color-mix(in srgb, var(--good) 35%, var(--line)); border-radius: 999px; color: var(--good); background: color-mix(in srgb, var(--good) 7%, var(--card)); font-size: 0.7rem; font-weight: 650; white-space: nowrap; }
  .desc { margin: 8px 0 0; color: var(--muted); max-width: 64ch; font-size: 0.9rem; }
  .note { margin: 10px 0 0; padding: 6px 0 6px 12px; border-left: 2px solid var(--accent); font: italic 0.95rem/1.5 var(--serif); white-space: pre-wrap; }
  .command { display: flex; align-items: flex-start; gap: 8px; margin-top: 14px; padding: 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .command code { flex: 1; min-width: 0; font-size: 0.78rem; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
  @media (max-width: 560px) {
    .entry-head { flex-direction: column; }
    .adoption { white-space: normal; }
    .command { flex-direction: column; align-items: stretch; }
    .actions .btn { flex: 1 1 100%; }
    .command :global(.copy) { align-self: flex-start; }
  }
  @media (pointer: coarse) { .btn, .back { min-height: 44px; } }
</style>
