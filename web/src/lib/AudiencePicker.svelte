<script lang="ts">
  import { onMount } from 'svelte'
  import { getGroups, type Audience, type Group } from './api'
  let { visibility = $bindable('personal'), audience = $bindable<Audience>({ groupRole: 'viewer', people: [], excluded: [] }), linkEnabled = true, legacyAllowed = false }: { visibility: string; audience: Audience; linkEnabled?: boolean; legacyAllowed?: boolean } = $props()
  let groups = $state<Group[]>([]), error = $state('')
  const defaultRole = 'viewer'
  let peopleText = $state(''), excludedText = $state('')
  $effect(() => { const text = audience.people.map(p => p.email).join('\n'); if (emails(peopleText).join('\n') !== text) peopleText = text; const excluded = audience.excluded.join('\n'); if (emails(excludedText).join('\n') !== excluded) excludedText = excluded })
  const group = $derived(groups.find(g => g.id === audience.groupId))
  const options = $derived([
    { value: 'personal', title: 'Personal', description: 'Only you. Keep your own collection separate.' },
    { value: 'private', title: 'Selected people', description: 'Invite named people as viewers or editors.' },
    { value: 'team', title: 'Team or organisation', description: 'Follow group membership, add people, and exclude individuals.' },
    { value: 'public', title: 'Public community', description: 'Anyone can discover and read the skills and list notes, without signing in.' },
    ...(linkEnabled ? [{ value: 'link', title: 'Unlisted link', description: 'Anyone with the link can read it. Hidden from community discovery.' }] : []),
    ...(legacyAllowed ? [{ value: 'members', title: 'Legacy instance audience', description: 'Preserves access for signed-in accounts on this server. This is not public.' }] : []),
  ])
  onMount(() => { void getGroups().then(r => groups = r.groups).catch(() => error = 'Could not load your groups. Retry before choosing a team audience.') })
  function choose(value: string) {
    visibility = value
    audience = { ...audience, groupId: value === 'team' ? audience.groupId : 0, people: value === 'personal' ? [] : audience.people, excluded: ['private', 'team'].includes(value) ? audience.excluded : [] }
  }
  function emails(value: string) { return [...new Set(value.split(/[,;\n]/).map(s => s.trim()).filter(Boolean))] }
  function peopleInput(value: string) {
    peopleText = value
    audience = { ...audience, people: emails(value).map(email => ({ email, role: audience.people.find(p => p.email === email)?.role ?? defaultRole })) }
  }
</script>

<fieldset class="audience">
  <legend>Who can access this list?</legend>
  <div class="choices">
    {#each options as option}
      <label class="choice"><input type="radio" name="audience" value={option.value} checked={visibility === option.value} onchange={() => choose(option.value)} /><span><strong>{option.title}</strong><small>{option.description}</small></span></label>
    {/each}
  </div>
  {#if visibility === 'team'}
    <label class="field">Team or organisation <select aria-label="Team or organisation" value={audience.groupId ?? 0} onchange={e => audience = { ...audience, groupId: Number(e.currentTarget.value) }} required><option value="0">Choose a group</option>{#each groups as g}<option value={g.id}>{g.kind === 'organisation' ? 'Organisation' : 'Team'} · {g.name}{g.parentId ? ` · ${groups.find(p => p.id === g.parentId)?.name || 'Organisation'}` : ''}</option>{/each}</select></label>
    <label class="field">Group members can <select aria-label="Group access role" value={audience.groupRole || 'viewer'} onchange={e => audience = { ...audience, groupRole: e.currentTarget.value }}><option value="viewer">View only</option><option value="editor">Edit skills and notes</option></select></label>
    <p class="hint">New group members gain access automatically; people who leave lose inherited access. Teams inside an organisation have their own explicit membership.</p>
    {#if !groups.length}<p class="hint"><a href="/spaces">Create or join a team / organisation</a> first.</p>{/if}
    {#if error}<p role="alert">{error}</p>{/if}
  {/if}
  {#if visibility !== 'personal'}
    <details class="exceptions" open={visibility === 'private' || audience.people.length > 0 || audience.excluded.length > 0}><summary>{visibility === 'private' ? 'Selected people & exclusions' : visibility === 'team' ? 'People & exceptions (optional)' : 'Collaborators (optional)'}</summary>
    <label class="field">{visibility === 'private' ? 'People to include' : 'Additional collaborators'} <textarea aria-label="People to include" value={peopleText} oninput={e => peopleInput(e.currentTarget.value)} placeholder="alex@example.com&#10;sam@example.com" rows="2"></textarea></label>
    <p class="hint">One existing account email per line. {visibility === 'public' || visibility === 'link' || visibility === 'members' ? 'This controls collaboration; it does not limit who can read this audience.' : 'Only these people and the selected group can read the list.'}</p>

    {#each audience.people as person, i}<label class="person"><span>{person.email}</span><select aria-label={`Role for ${person.email}`} value={person.role} onchange={e => audience = { ...audience, people: audience.people.map((p, j) => j === i ? { ...p, role: e.currentTarget.value } : p) }}><option value="viewer">Viewer</option><option value="editor">Editor</option></select></label>{/each}
  {#if visibility === 'team' || visibility === 'private'}
    <label class="field">Except these people <textarea aria-label="People to exclude" value={excludedText} oninput={e => { excludedText = e.currentTarget.value; audience = { ...audience, excluded: emails(excludedText) } }} placeholder="Optional: one account email per line" rows="2"></textarea></label>
    <p class="hint">Exclusions override group membership and individual invitations. Public and unlisted audiences cannot exclude readers.</p>
  {/if}
    </details>
  {/if}
  <div class="preview" aria-live="polite"><strong>Audience preview</strong><p>{visibility === 'personal' ? 'Personal — your list is private to you.' : visibility === 'public' ? 'Public — the list, skills and list notes will be discoverable by anyone.' : visibility === 'team' ? `${group?.name || 'Choose a group'} · ${audience.people.length} additional people · ${audience.excluded.length} exclusions` : visibility === 'private' ? `${audience.people.length} selected people · ${audience.excluded.length} exclusions` : visibility === 'link' ? 'Unlisted — anyone with its link can read it.' : 'Legacy — signed-in accounts on this instance can read it.'}</p><small>Instance administrators retain administrative access. Personal shelf notes, tokens and machine inventories are never published with a list.</small></div>
</fieldset>

<style>
 .audience { margin:20px 0 0; padding:0; border:0; min-width:0; } legend { font-weight:650; font-size:.85rem; margin-bottom:10px; }
 .choices { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:7px; } .choice { display:grid; grid-template-columns:18px 1fr; gap:10px; padding:11px; border:1px solid var(--line); border-radius:9px; cursor:pointer; } .choice:has(input:checked) { border-color:var(--accent); background:var(--tint); } .choice input { accent-color:var(--accent); margin-top:4px; } .choice span { display:grid; gap:3px; } strong { font-size:.85rem; } small,.hint { color:var(--muted); font-size:.77rem; line-height:1.5; } .field { display:grid; gap:6px; font-size:.8rem; font-weight:600; margin-top:12px; } textarea,select { width:100%; min-width:0; font-weight:400; } .person { display:flex; gap:8px; align-items:center; margin-top:8px; } .person span { flex:1; font-size:.8rem; overflow-wrap:anywhere; } .person select { width:100px; }
 .preview { padding:12px; margin-top:16px; border:1px solid var(--line); border-radius:9px; background:var(--bg); } .preview p { margin:6px 0; font-size:.8rem; overflow-wrap:anywhere; }
.exceptions { margin-top:16px; padding:12px; border:1px solid var(--line); border-radius:9px; } summary { cursor:pointer; font-size:.83rem; font-weight:600; }
 @media(max-width:480px) { .choices { grid-template-columns:1fr; } }
</style>
