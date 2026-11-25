<script lang="ts">
  import type { Pathway } from '../pathways'

  // A stand-in for the pathway editor, which comes next: it holds the editor's place and props so the routes work now.
  let { id, start, onclose }: { id: string | null; start: 'blank' | 'standard'; onclose: () => void; onsaved: (p: Pathway) => void } = $props()

  function back(event: MouseEvent) {
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.button !== 0) return
    event.preventDefault()
    onclose()
  }
</script>

<section class="editor" aria-labelledby="pathway-editor-title" data-pathway={id ?? ''} data-start={start}>
  <a href="/factory/pathways" onclick={back}><span aria-hidden="true">‹&nbsp;</span>Pathways</a>
  <h1 id="pathway-editor-title">Pathway editor</h1>
</section>

<style>
  .editor { max-width: 920px; margin: 0 auto; padding: 36px 40px; }
  a { display: inline-flex; align-items: center; min-height: 44px; color: var(--muted); }
  h1 { margin: 4px 0 0; font: 600 1.9rem/1.15 var(--serif); letter-spacing: -0.02em; }
  @media (max-width: 560px) { .editor { padding: 20px 16px; } }
</style>
