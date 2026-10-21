<script lang="ts">
  import { onMount } from 'svelte'
  import { answerMCPConsent, getMCPConsent, type MCPConsent } from './api'
  const query = location.search.slice(1)
  let info = $state<MCPConsent | null>(null)
  let scope = $state<'read' | 'write'>('read')
  let error = $state('')
  let busy = $state(false)
  async function load() {
    try { info = await getMCPConsent(query); scope = info.canWrite ? info.scope : 'read'; error = '' }
    catch (e) { error = e instanceof Error ? e.message : 'This connection request is no longer available.' }
  }
  onMount(() => { void load() })
  async function answer(approve: boolean) {
    busy = true; error = ''
    try { const result = await answerMCPConsent(query, approve, scope); location.assign(result.redirect) }
    catch (e) { error = e instanceof Error ? e.message : 'Could not complete the connection.'; busy = false }
  }
</script>

<section class="consent" aria-labelledby="consent-title">
  <p class="eyebrow">Connect your shelf</p>
  <h1 id="consent-title">{info ? `Allow ${info.clientName} to connect?` : 'Review this connection'}</h1>
  {#if error}<div class="error" role="alert"><p>{error}</p><button type="button" class="btn" onclick={load} disabled={busy}>Try again</button></div>{/if}
  {#if !info && !error}<p class="muted" role="status">Checking the client and requested permissions…</p>{/if}
  {#if info}
    <p class="lede">You’re signed in as <strong>{info.account}</strong>. This connection uses your account and the lists you can access.</p>
    <div class="permissions">
      <p class="label">What you allow</p>
      <label class="choice"><input type="radio" name="scope" bind:group={scope} value="read" /><span><strong>Read your shelf</strong><small>Skills, personal notes, tags, machine inventories, sync plans and accessible lists.</small></span></label>
      {#if info.scope === 'write' && info.canWrite}<label class="choice"><input type="radio" name="scope" bind:group={scope} value="write" /><span><strong>Read and manage your shelf</strong><small>Also add, edit or remove skills and personal notes, change tags, and create or add skills to personal lists.</small></span></label>{/if}
    </div>
    <div class="details"><p><strong>Your control stays here.</strong> This does not grant access to account settings, passwords, API credentials, backups or administration. It does not install or execute skills.</p><p>Authorization lasts 30 days. Pause or revoke it at any time in Account → MCP access.</p>{#if !info.enabled}<p>Approving turns MCP access on for your account. Existing connections resume unless you paused them individually.</p>{/if}</div>
    <p class="callback">Return to <strong>{info.callbackHost}</strong> after approval. Client names are provided by the connecting app; approve only a connection you started.</p>
    <div class="actions"><button type="button" class="btn" disabled={busy} onclick={() => answer(false)}>Cancel</button><button type="button" class="btn primary" disabled={busy} onclick={() => answer(true)}>{busy ? 'Connecting…' : scope === 'write' ? 'Allow read & manage' : 'Allow read access'}</button></div>
  {/if}
</section>

<style>
  .consent { max-width:640px; margin:clamp(24px,6vw,64px) auto; padding:clamp(22px,4vw,40px); border:1px solid var(--line); background:var(--card); border-radius:16px; }
  h1 { font:500 clamp(1.8rem,4vw,2.4rem)/1.15 var(--serif); letter-spacing:-.035em; margin:12px 0 18px; overflow-wrap:anywhere; }
  p { line-height:1.65; }
  .lede,.muted,.details,.callback { color:var(--muted); }
  .permissions { margin:26px 0; }
  .label { font-size:.72rem; text-transform:uppercase; letter-spacing:.08em; color:var(--muted); }
  .choice { display:flex; align-items:flex-start; gap:12px; padding:16px; border:1px solid var(--line); border-radius:10px; margin-top:10px; cursor:pointer; }
  .choice:has(input:checked) { border-color:var(--accent); background:var(--tint); }
  input { flex:none; width:16px; height:16px; margin-top:3px; accent-color:var(--accent); }
  small { display:block; color:var(--muted); font-size:.8rem; line-height:1.6; margin-top:5px; }
  .details,.callback { font-size:.8rem; }
  .callback { padding-top:16px; border-top:1px solid var(--line); overflow-wrap:anywhere; }
  .actions { margin-top:28px; display:flex; justify-content:flex-end; gap:10px; flex-wrap:wrap; }
  .error { border:1px solid var(--bad); padding:14px; border-radius:8px; }
  @media(max-width:480px) { .actions { flex-direction:column-reverse; } .actions button { width:100%; } }
</style>
