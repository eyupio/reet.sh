<script lang="ts">
  import { onMount } from 'svelte'
  import { getGroups, createGroup, getTeamNotifications, saveTeamNotifications, setGroupMember, updateGroup, type Group, type TeamNotifyView } from './api'
  import PageHeader from './PageHeader.svelte'
  import ConfirmDialog from './ConfirmDialog.svelte'
  let { canWrite }: { canWrite: boolean } = $props()
  let groups = $state<Group[]>([]), name = $state(''), kind = $state('team'), parentId = $state(0), busy = $state(false), loading = $state(true), error = $state(''), notice = $state('')
  let emails = $state<Record<number, string>>({}), roles = $state<Record<number, string>>({})
  let renames = $state<Record<number, string>>({}), deleting = $state<Group | null>(null)
  let notify = $state<Record<number, TeamNotifyView>>({}), extraText = $state<Record<number, string>>({})
  let savingNotify = $state(0), notifyNotice = $state('')
  async function rename(g: Group, remove = false) { busy = true; error = ''; deleting = null; try { groups = (await updateGroup(g.id, renames[g.id] || g.name, remove)).groups; notice = remove ? 'Group deleted. Lists remain private to their owners and individually invited people.' : 'Group renamed.' } catch (e) { fail(e) } finally { busy = false } }
  let removing = $state<{ group: Group; email: string } | null>(null)
  async function load() { try { groups = (await getGroups()).groups; await loadNotify(groups) } catch (e) { fail(e) } finally { loading = false } }
  async function loadNotify(gs: Group[]) {
    for (const g of gs) {
      if (!['owner', 'manager'].includes(g.role)) continue
      try {
        const v = await getTeamNotifications(g.id)
        notify[g.id] = v
        extraText[g.id] = v.notify.extraEmails.join('\n')
      } catch {
        /* A group that cannot be read here simply has no notification form. */
      }
    }
  }
  function fail(e: unknown) { error = e instanceof Error ? e.message : String(e) }
  onMount(() => { void load() })
  async function create(e: SubmitEvent) { e.preventDefault(); busy = true; error = ''; try { groups = (await createGroup({ name, kind, parentId: kind === 'team' ? parentId : 0 })).groups; name = ''; await loadNotify(groups); notice = 'Created. Add people below, then use this group as a list audience.' } catch (e) { fail(e) } finally { busy = false } }
  async function member(g: Group, email: string, role: string, remove = false) { busy = true; error = ''; removing = null; try { groups = (await setGroupMember(g.id, email, role, remove)).groups; emails[g.id] = ''; await loadNotify(groups); notice = remove ? 'Membership removed. Inherited list access has been revoked; separate personal invitations still apply.' : 'Membership saved. Access to lists shared with this group now follows its membership.' } catch (e) { fail(e) } finally { busy = false } }
  function toggleEvent(n: TeamNotifyView, key: string, on: boolean) {
    const chosen = new Set(n.notify.events)
    if (on) chosen.add(key); else chosen.delete(key)
    n.notify.events = n.events.filter((e) => chosen.has(e.key)).map((e) => e.key)
  }
  async function saveNotify(g: Group) {
    const n = notify[g.id]
    if (!n) return
    savingNotify = g.id; error = ''
    try {
      const v = await saveTeamNotifications(g.id, {
        enabled: n.notify.enabled,
        events: n.notify.events,
        recipients: n.notify.recipients,
        extraEmails: (extraText[g.id] || '').split(/[\s,;]+/).filter(Boolean),
      })
      notify[g.id] = v
      extraText[g.id] = v.notify.extraEmails.join('\n')
      notifyNotice = 'Notifications saved.'
    } catch (e) { fail(e) } finally { savingNotify = 0 }
  }
</script>
<PageHeader eyebrow="Teams & organisations" title="A proper toolkit. Together." lede="Organisations group your teams; each has explicit membership. Share lists with a whole group, with additional people or individual exclusions when needed." />
<p class="hint">Groups organise access within this reet site. They are not separate hosted tenants or an SSO directory. Instance administrators retain administrative access.</p>
{#if error}<p class="notice error" role="alert">{error}</p>{/if}{#if notice}<p class="notice" role="status">{notice}</p>{/if}{#if notifyNotice}<p class="notice" role="status">{notifyNotice}</p>{/if}
{#if canWrite}<details class="create"><summary>Create a team or organisation</summary><form onsubmit={create}><label>Name <input aria-label="Group name" bind:value={name} maxlength="80" required placeholder="e.g. Platform engineering" /></label><label>Type <select aria-label="Group type" bind:value={kind}><option value="team">Team</option><option value="organisation">Organisation</option></select></label>{#if kind === 'team'}<label>Organisation <select aria-label="Parent organisation" bind:value={parentId}><option value={0}>Independent team</option>{#each groups.filter(g => g.kind === 'organisation' && ['owner', 'manager'].includes(g.role)) as g}<option value={g.id}>{g.name}</option>{/each}</select></label>{/if}<button class="btn primary" disabled={busy || !name.trim()}>Create group</button></form></details>{/if}
{#if loading}<p role="status">Loading groups…</p>{:else}<div class="groups">{#each groups as g (g.id)}<article><div class="heading"><div><span class="badge">{g.kind === 'organisation' ? 'Organisation' : 'Team'}{g.parentId ? ` · ${groups.find(p => p.id === g.parentId)?.name || 'Organisation'}` : ''}</span><h2>{g.name}</h2></div><span class="badge">{g.role === 'owner' ? 'You manage this' : g.role === 'manager' ? 'Manager' : 'Member'}</span></div>
  {#if canWrite && ['owner', 'manager'].includes(g.role)}<details><summary>Group settings</summary><form onsubmit={e => { e.preventDefault(); void rename(g) }}><label>Group name <input aria-label={`Rename ${g.name}`} value={renames[g.id] ?? g.name} oninput={e => renames[g.id] = e.currentTarget.value} maxlength="80" required /></label><button class="btn" disabled={busy}>Rename</button>{#if g.role === 'owner'}<button type="button" class="btn quiet" disabled={busy} onclick={() => deleting = g}>Delete group</button>{/if}</form></details><ul>{#each g.members ?? [] as m (m.id)}<li><div><strong>{m.name}</strong><small>{m.email} · {m.role}</small></div>{#if m.id !== g.ownerId && (g.role === 'owner' || m.role !== 'manager')}<div class="actions">{#if g.role === 'owner'}<select aria-label={`Group role for ${m.email}`} value={m.role} disabled={busy} onchange={e => member(g, m.email, e.currentTarget.value)}><option value="member">Member</option><option value="manager">Manager</option></select>{/if}<button type="button" class="btn quiet" disabled={busy} onclick={() => removing = { group: g, email: m.email }}>Remove</button></div>{/if}</li>{/each}</ul><form class="add" onsubmit={e => { e.preventDefault(); void member(g, emails[g.id] || '', roles[g.id] || 'member') }}><label>Account email <input aria-label={`Add person to ${g.name}`} type="email" bind:value={emails[g.id]} required placeholder="person@example.com" /></label><label>Role <select aria-label={`New role in ${g.name}`} value={roles[g.id] || 'member'} onchange={e => roles[g.id] = e.currentTarget.value}><option value="member">Member</option>{#if g.role === 'owner'}<option value="manager">Manager</option>{/if}</select></label><button class="btn" disabled={busy}>Add person</button></form>{:else}<p class="hint">Ask this group's manager to update its membership. Group-shared lists appear under Lists → Teams & organisations.</p>{/if}
  {#if canWrite && ['owner', 'manager'].includes(g.role)}
    <details class="notify"><summary>Notifications</summary>
      {#if notify[g.id]}
        {@const n = notify[g.id]}
        <p class="hint">What this team is emailed about. Every kind is on until a manager chooses otherwise. Messages are written to the instance outbox and sent through its mail server.</p>
        {#if !n.mailReady}
          <p class="notice">The instance owner has not set up a mail server yet. These choices are saved, but no team notices are sent until one is set.</p>
        {/if}
        <form onsubmit={(e) => { e.preventDefault(); void saveNotify(g) }}>
          <label class="check"><input type="checkbox" checked={n.notify.enabled} onchange={(e) => (n.notify.enabled = e.currentTarget.checked)} /> Email this team about changes</label>
          <fieldset class="events" disabled={!n.notify.enabled}>
            <legend>What to send</legend>
            {#each n.events as event (event.key)}
              <label class="check"><input type="checkbox" checked={n.notify.events.includes(event.key)} onchange={(e) => toggleEvent(n, event.key, e.currentTarget.checked)} /><span><strong>{event.label}</strong><small>{event.description}</small></span></label>
            {/each}
          </fieldset>
          <div class="notify-pair">
            <label>Send to
              <select value={n.notify.recipients} onchange={(e) => (n.notify.recipients = e.currentTarget.value === 'members' ? 'members' : 'managers')} disabled={!n.notify.enabled}>
                <option value="managers">The owner and managers</option>
                <option value="members">Every member</option>
              </select>
            </label>
            <label>Extra addresses
              <textarea rows="2" placeholder="team-inbox@example.com, one per line" value={extraText[g.id] ?? ''} oninput={(e) => (extraText[g.id] = e.currentTarget.value)} disabled={!n.notify.enabled}></textarea>
              <small>People without access also receive the notice. They are not given access to anything.</small>
            </label>
          </div>
          <button class="btn" type="submit" disabled={busy || savingNotify === g.id}>{savingNotify === g.id ? 'Saving…' : 'Save notifications'}</button>
        </form>
      {:else}
        <p class="hint">Loading this group's notification settings…</p>
      {/if}
    </details>
  {/if}
</article>{:else}<div class="empty"><h2>A place for every team.</h2><p>Create an organisation or an independent team, then add existing accounts. Group membership does not expose anyone's personal shelf or machine reports.</p></div>{/each}</div>{/if}
{#if removing}<ConfirmDialog title="Remove group membership?" body={`${removing.email} will lose access inherited from ${removing.group.name}. Direct invitations to individual lists remain.`} confirmLabel="Remove membership" oncancel={() => removing = null} onconfirm={() => { if (removing) void member(removing.group, removing.email, 'member', true) }} />{/if}
{#if deleting}<ConfirmDialog title={`Delete ${deleting.name}?`} body="Its lists and skills remain. Members lose inherited group access; individual list invitations still apply. Delete child teams before deleting their organisation." confirmLabel="Delete group" oncancel={() => deleting = null} onconfirm={() => { if (deleting) void rename(deleting, true) }} />{/if}
<style>
 .hint { color:var(--muted); font-size:.83rem; line-height:1.6; } .notice { margin:12px 0 0; padding:10px 12px; border:1px solid var(--line); border-radius:8px; background:var(--card); font-size:.82rem; } .notify { margin-top:16px; padding-top:4px; border-top:1px solid var(--line); } .notify form { display:grid; gap:12px; } .check { display:flex; align-items:flex-start; gap:8px; flex:none; font-size:.84rem; } .check input { width:18px; height:18px; min-height:0; margin-top:1px; flex:none; } .check span { display:grid; gap:2px; } .check small { font-weight:400; line-height:1.4; } fieldset.events { display:grid; grid-template-columns:1fr 1fr; gap:8px 16px; border:1px solid var(--line); border-radius:8px; padding:12px; } fieldset.events legend { padding:0 5px; color:var(--muted); font-size:.78rem; } .notify-pair { display:grid; grid-template-columns:1fr 1fr; gap:12px; } .notify-pair label { display:grid; gap:5px; color:var(--muted); font-size:.8rem; } .notify-pair textarea { width:100%; min-height:64px; } @media (max-width:640px) { fieldset.events,.notify-pair { grid-template-columns:1fr; } } .create,article,.empty { border:1px solid var(--line); border-radius:12px; padding:20px; background:var(--card); margin-top:20px; } summary { cursor:pointer; font-weight:600; } form { display:flex; flex-wrap:wrap; align-items:end; gap:12px; margin-top:16px; } label { display:grid; gap:6px; font-size:.8rem; min-width:0; flex:1 1 160px; } input,select { width:100%; min-width:0; } .heading,.actions { display:flex; gap:12px; align-items:center; justify-content:space-between; flex-wrap:wrap; } h2 { font:500 1.5rem/1.3 var(--serif); margin:6px 0; overflow-wrap:anywhere; } .badge,small { color:var(--muted); font-size:.75rem; } ul { padding:0; list-style:none; } li { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:10px; border-top:1px solid var(--line); padding:12px 0; } li strong,li small { display:block; overflow-wrap:anywhere; } .actions select { width:auto; } .notice { padding:12px; border:1px solid var(--good); border-radius:8px; } .error { border-color:var(--bad); }
</style>
