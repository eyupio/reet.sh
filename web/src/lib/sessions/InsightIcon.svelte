<script lang="ts">
  let { name }: { name: 'overview' | 'changes' | 'files' | 'github' | 'terminal' | 'check' | 'conflict' | 'clock' | 'file' | 'refresh' | 'arrow' } = $props()
  const paths = {
    overview: 'M3 3h6v6H3zM13 3h8v4h-8zM3 13h6v8H3zM13 11h8v10h-8z',
    changes: 'M6 3v12a3 3 0 0 0 3 3h6M6 7h9a3 3 0 0 1 3 3v8M3 3h6M15 18l3 3 3-3',
    files: 'M3 6h6l2-3h5l2 3h3v14H3z',
    github: 'M6 5a2 2 0 1 0 0 .01M6 7v10M6 17a2 2 0 1 0 0 .01M18 17a2 2 0 1 0 0 .01M18 15V8a3 3 0 0 0-3-3h-2M15 2l-3 3 3 3',
    terminal: 'M3 4h18v16H3zM6 8l4 4-4 4M13 16h5',
    check: 'M4 12l5 5L20 6',
    conflict: 'M12 3 2 21h20L12 3zM12 9v5M12 17v.1',
    clock: 'M12 8v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0',
    file: 'M5 2h9l5 5v15H5zM14 2v6h5M8 13h8M8 17h6',
    refresh: 'M20 7v5h-5M4 17v-5h5M5 7a8 8 0 0 1 13-2l2 3M4 16l2 3a8 8 0 0 0 13-2',
    arrow: 'M7 17 17 7M7 7h10v10',
  }
</script>
<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d={paths[name]} /></svg>
