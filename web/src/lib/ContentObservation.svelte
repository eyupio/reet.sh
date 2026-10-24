<script lang="ts">
  import type { ContentInventory } from './api'
  let { content }: { content?: ContentInventory } = $props()
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
</style>
