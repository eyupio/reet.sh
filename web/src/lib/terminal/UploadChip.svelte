<script lang="ts">
  import { formatImageSize } from './upload'
  let { name, size, oncancel }: { name: string; size: number; oncancel: () => void } = $props()
</script>

<div class="upload-chip" role="status">
  <div class="upload-chip-copy">
    <span class="upload-chip-name" title={name}>Uploading {name}</span>
    <span class="upload-chip-size">{formatImageSize(size)}</span>
  </div>
  <progress aria-label={`Uploading ${name}`}></progress>
  <button type="button" onclick={oncancel} aria-label={`Cancel upload of ${name}`}>Cancel</button>
</div>

<style>
  .upload-chip { display:flex; align-items:center; gap:8px; padding:6px 8px; border:1px solid var(--line); border-radius:8px; background:var(--card); box-shadow:var(--shadow); font-size:.7rem; color:var(--ink); pointer-events:auto }
  .upload-chip-copy { display:flex; flex-direction:column; min-width:0; flex:1 }
  .upload-chip-name { max-width:180px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap }
  .upload-chip-size { color:var(--muted); font-size:.65rem }
  .upload-chip progress { width:56px; height:4px; accent-color:var(--accent) }
  .upload-chip button { font:inherit; font-size:.65rem; min-height:28px; padding:3px 7px; border:1px solid var(--line); border-radius:6px; background:var(--bg); color:var(--ink); cursor:pointer }
  @media(pointer:coarse) { .upload-chip button { min-width:40px; min-height:40px } }
</style>
