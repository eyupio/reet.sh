<script lang="ts">
  import Markdown from './Markdown.svelte'
  let { text }: {text:string} = $props()
</script>

<div class="gaffer-prose"><Markdown {text} /></div>
<style>
 .gaffer-prose { font-size:15px;line-height:1.7;overflow-wrap:anywhere;min-width:0;--md-gap:14px; }
</style>
