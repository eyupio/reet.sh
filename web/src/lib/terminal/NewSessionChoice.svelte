<script lang="ts">
  import WorkspaceDialog from './WorkspaceDialog.svelte'

  // What to start: an agent session, a plain terminal, or an outcome for Gaffer's Factory. The Sessions workspace asks
  // with this dialog. It starts nothing itself: the page does what the choice says.
  export type NewSessionKind = 'agent' | 'terminal' | 'factory'
  let { project = '', machine = '', factory = false, onchoose, onclose }: {
    /** The project the session starts in; without one the choice is for the machine. */
    project?: string
    machine?: string
    /** Whether this account has the Factory. */
    factory?: boolean
    onchoose: (kind: NewSessionKind) => void
    onclose: () => void
  } = $props()
</script>

<WorkspaceDialog title={project ? `New session in ${project}` : 'New session'} {onclose}>
  <p class="copy">Choose what to start{#if project}{' '}in <strong>{project}</strong> on <strong>{machine || 'this machine'}</strong>{:else if machine}{' '}on <strong>{machine}</strong>{/if}.</p>
  <div class="choice">
    <button type="button" class="option" aria-label="Agent session" onclick={() => onchoose('agent')}><strong>Agent session</strong><span>Give an agent a brief and let it work in a folder.</span></button>
    <button type="button" class="option" aria-label="Terminal session" onclick={() => onchoose('terminal')}><strong>Terminal session</strong><span>A shell on the machine, with no agent.</span></button>
    {#if factory}<button type="button" class="option" aria-label="Factory outcome" onclick={() => onchoose('factory')}><strong>Factory outcome</strong><span>Hand Gaffer an outcome. It plans the pipelines, you approve, and it runs each one to the finish.</span></button>{/if}
  </div>
</WorkspaceDialog>

<style>
  .copy{margin:0 0 12px;color:var(--muted);line-height:1.5;overflow-wrap:anywhere}.copy strong{color:var(--ink)}
  .choice{display:grid;gap:8px}.option{display:grid;gap:4px;min-height:44px;text-align:left;padding:12px 16px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--ink);font:inherit;cursor:pointer}.option:hover,.option:focus-visible{border-color:var(--accent)}.option strong{font-size:.9rem}.option span{font-size:0.875rem;color:var(--muted);line-height:1.5}
</style>
