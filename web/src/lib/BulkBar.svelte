<script lang="ts">
  let { count, tagOptions, busy, onclear, ontag, onstage, onlike, onremove }: {
    count: number
    tagOptions: string[]
    busy: boolean
    onclear: () => void
    ontag: (name: string) => void
    onstage: (stage: 'use' | 'try' | 'shelf') => void
    onlike: () => void
    onremove: () => void
  } = $props()

  let stage = $state<'use' | 'try' | 'shelf'>('use')
  let tagOpen = $state(false)
  let tagDraft = $state('')
  let tagInput = $state<HTMLInputElement>()

  function toggleTag() {
    tagOpen = !tagOpen
  }

  function addTag(e: SubmitEvent) {
    e.preventDefault()
    const name = tagDraft.trim()
    if (!name) return
    ontag(name)
    tagDraft = ''
    tagOpen = false
  }

  $effect(() => {
    if (tagOpen) tagInput?.focus()
  })
</script>

<section class="bulk" aria-label="Bulk actions" aria-live="polite">
  <strong>{count === 1 ? '1 selected' : `${count} selected`}</strong>
  <div class="actions">
    {#if tagOpen}
      <form onsubmit={addTag}>
        <label class="vh" for="bulk-tag">Tag selected skills</label>
        <input id="bulk-tag" bind:this={tagInput} list="bulk-tag-options" bind:value={tagDraft} placeholder="Tag name" autocomplete="off" />
        <datalist id="bulk-tag-options">
          {#each tagOptions as name}<option value={name}></option>{/each}
        </datalist>
        <button type="submit" class="btn primary" disabled={busy || !tagDraft.trim()}>Add tag</button>
        <button type="button" class="btn quiet" onclick={() => (tagOpen = false)}>Cancel</button>
      </form>
    {:else}
      <button type="button" class="btn quiet" onclick={toggleTag} disabled={count === 0 || busy}>Tag</button>
    {/if}
    <span class="sep" aria-hidden="true"></span>
    <label class="vh" for="bulk-stage">Move to stage</label>
    <select id="bulk-stage" bind:value={stage} disabled={count === 0 || busy}>
      <option value="use">In use</option>
      <option value="try">To test</option>
      <option value="shelf">On the shelf</option>
    </select>
    <button type="button" class="btn quiet" onclick={() => onstage(stage)} disabled={count === 0 || busy}>Move</button>
    <button type="button" class="btn quiet" onclick={onlike} disabled={count === 0 || busy}>Like</button>
    <button type="button" class="btn quiet danger" onclick={onremove} disabled={count === 0 || busy}>Remove</button>
    <button type="button" class="btn quiet" onclick={onclear}>Done</button>
  </div>
</section>

<style>
  .bulk {
    position: fixed; left: 50%; bottom: 20px; z-index: 15; transform: translate(-50%, 0);
    width: max-content; max-width: min(760px, calc(100vw - 32px));
    display: flex; align-items: center; gap: 12px; flex-wrap: wrap;
    padding: 10px 14px; border: 1px solid var(--line); border-left: 3px solid var(--accent);
    border-radius: 12px; background: var(--card); box-shadow: 0 18px 50px -20px rgb(0 0 0 / 0.4);
  }
  strong { font-size: 0.9rem; }
  .actions { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; }
  .actions form { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .actions input { width: 12rem; }
  .sep { width: 1px; height: 20px; background: var(--line); margin: 0 4px; }
  .actions select { padding: 7px 10px; }
  @media (max-width: 560px) {
    .bulk { bottom: 12px; width: calc(100vw - 24px); max-width: none; padding: 10px 12px; }
    .actions { width: 100%; }
    .actions input { flex: 1; width: auto; }
  }
  @media (prefers-reduced-motion: no-preference) {
    .bulk { transition: opacity 180ms var(--ease-out), transform 180ms var(--ease-out); }
    @starting-style { .bulk { opacity: 0; transform: translate(-50%, 8px); } }
  }
</style>
