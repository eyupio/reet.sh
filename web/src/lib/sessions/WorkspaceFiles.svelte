<script lang="ts">
  import { fileWord, type SessionInsight, type InsightFile } from '../sessionInsights'
  import InsightIcon from './InsightIcon.svelte'
  let { data, changes = false, onopen }: { data: SessionInsight; changes?: boolean; onopen: (file: InsightFile) => void } = $props()
  let filter = $state('')
  const files = $derived((data.files ?? []).filter(f => f.path.toLowerCase().includes(filter.toLowerCase())))
  const added = $derived((data.files ?? []).reduce((n, f) => n + f.added, 0))
  const removed = $derived((data.files ?? []).reduce((n, f) => n + f.removed, 0))
</script>
{#if changes}
  <div class="totals"><span><b>{data.changed}{data.changedMore ? "+" : ""}</b> changed files</span>{#if data.countsKnown}<span class="good">+{added}</span><span class="bad">−{removed}</span>{/if}</div>
  {#if data.conflicts}<p class="conflicts"><InsightIcon name="conflict" />{data.conflicts} file{data.conflicts === 1 ? '' : 's'} with merge conflicts</p>{/if}
  <p class="hint">Staged and unstaged changes against HEAD. {data.countsKnown ? 'Untracked files have no line counts.' : 'Line counts are unavailable.'}{data.more ? ' Totals cover listed files only.' : ''}</p>
{/if}
<label class="filter"><InsightIcon name="overview" /><input aria-label={changes ? 'Filter changed files' : 'Filter files in this folder'} bind:value={filter} placeholder="Filter files…" /></label>
<div class="files">
  {#each files as file (file.path)}
    {@const conflict = fileWord(file) === 'Conflict'}
    <button class="file" class:conflict onclick={() => onopen(file)} title={file.path}>
      <InsightIcon name={conflict ? 'conflict' : file.directory ? 'files' : 'file'} />
      <span class="path"><strong>{file.path.split('/').at(-1)}</strong><small>{changes ? `${fileWord(file)} · ${file.path}` : file.directory ? 'Folder' : file.path}</small></span>
      {#if changes}<span class="stats">{#if file.binary}<small>binary</small>{:else if file.status === '??'}<small>new</small>{:else if !data.countsKnown}<small>—</small>{:else}<span><i class="good">+{file.added}</i> <i class="bad">−{file.removed}</i></span><span class="bar" aria-hidden="true"><i style={`width:${file.added + file.removed ? file.added / (file.added + file.removed) * 100 : 0}%`}></i></span>{/if}</span>{/if}
    </button>
  {/each}
</div>
{#if !files.length}<p class="empty">{filter ? 'No files match this filter.' : changes ? 'No uncommitted changes.' : 'This folder is empty.'}</p>{/if}
{#if data.more}<p class="hint">This listing is limited. Narrow the folder or use the terminal for the full view.</p>{/if}
<style>
  .totals{display:flex;align-items:center;gap:10px;font:12px var(--mono);margin-bottom:8px}.totals>span:first-child{flex:1;color:var(--ink-soft);font-family:var(--sans)}.totals b{font-size:20px;color:var(--ink)}.good{color:var(--good)}.bad{color:var(--bad)}.hint{color:var(--muted);font-size:11px;line-height:1.6;margin:8px 0 14px}.conflicts{display:flex;align-items:center;gap:7px;color:var(--bad);font-size:12px;padding:10px;border:1px solid color-mix(in srgb,var(--bad) 35%,transparent);border-radius:8px;background:color-mix(in srgb,var(--bad) 7%,transparent)}.filter{display:flex;align-items:center;gap:8px;border:1px solid var(--line);border-radius:7px;padding:0 9px;margin:10px 0;color:var(--muted)}input{border:0;background:none;min-width:0;width:100%;min-height:34px;padding:5px 0;font-size:12px;box-shadow:none}.file{display:flex;align-items:center;gap:9px;width:100%;min-height:52px;padding:9px 3px;border:0;border-bottom:1px solid var(--line-soft);border-radius:4px;background:none;color:var(--muted);text-align:left;cursor:pointer}.file:hover{background:var(--tint);color:var(--ink)}.file.conflict{color:var(--bad)}.file:focus-visible{outline:2px solid var(--accent);outline-offset:0}.path{min-width:0;flex:1}.path strong{display:block;font:500 12px var(--mono);color:var(--ink);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.path small{display:block;font:10px var(--sans);margin-top:5px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.stats{display:grid;justify-items:end;gap:6px;font:10px var(--mono);flex:none}.stats i{font-style:normal}.bar{display:block;width:42px;height:4px;overflow:hidden;border-radius:2px;background:var(--bad)}.bar i{display:block;height:100%;background:var(--good)}.empty{padding:18px 0;color:var(--muted);font-size:12px}
  @media(pointer:coarse){input{min-height:44px;font-size:16px}}
</style>
