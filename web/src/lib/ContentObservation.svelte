<script lang="ts">
  import type { ContentInventory } from './api'
  let { content, onimport, onpin }: { content?: ContentInventory; onimport?: (name: string) => void; onpin?: (name: string, opener: HTMLElement) => void } = $props()
  const labels: Record<string, string> = { observed: 'Content observed', incomplete: 'Content incomplete', stale: 'Content report out of date', paused: 'Content reporting paused' }
</script>
{#if !content || content.state === 'not_reported'}
  <small>Content not reported</small>
{:else}
  <details class="content-observation">
    <summary>{labels[content.state] ?? 'Content status unknown'} · {content.skills.length} {content.skills.length === 1 ? 'skill' : 'skills'}</summary>
    <p>Read-only fingerprints. Matching names or sources do not prove matching files. No skill files have been uploaded.</p>
    {#if content.receivedAt}<p>Received {new Date(content.receivedAt).toLocaleString()}</p>{/if}
    <ul>
      {#each content.skills as skill (skill.name)}
        <li><strong>{skill.name}</strong> · {skill.complete ? `${skill.files} files · ${skill.bytes} bytes` : 'Incomplete; contents cannot be compared'}
          {#if skill.complete && skill.digest}<code>{skill.digest}</code>{/if}
          {#if onimport || onpin}
            <span class="skill-actions">
              {#if onimport}<button type="button" onclick={() => onimport?.(skill.name)} aria-label={`Import ${skill.name} privately`}>Import privately</button>{/if}
              {#if onpin}<button type="button" onclick={(event) => onpin?.(skill.name, event.currentTarget)} aria-label={`Pin a library revision for ${skill.name}`}>Pin a library revision</button>{/if}
            </span>
          {/if}
          {#if skill.issues?.length}<span>{skill.issues.join(', ').replaceAll('_', ' ')}</span>{/if}
        </li>
      {/each}
    </ul>
  </details>
{/if}
<style>
  small, .content-observation { font-size: .75rem; color: var(--muted); }
  summary { cursor: pointer; }
  p { margin: .4rem 0; }
  ul { padding-left: 1.2rem; }
  code { display: block; overflow-wrap: anywhere; font-size: .68rem; }
  .skill-actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
</style>
