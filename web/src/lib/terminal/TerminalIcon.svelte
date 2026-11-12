<script lang="ts">
 import { terminalIconPaths, type TerminalIconName } from './icons'
 let {name}:{name:TerminalIconName}=$props()
</script>
<svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round"><path d={terminalIconPaths[name]}/></svg>
