<script lang="ts">
 let {name}:{name:'search'|'copy'|'screen'|'paste'|'commands'|'appearance'|'history'|'expand'|'interrupt'|'theatre'|'select'|'image'}=$props()
 const paths={theatre:'M1 3h14v10H1zM4 6h8v4H4z',search:'M11 11l4 4M12 7a5 5 0 1 1-10 0 5 5 0 0 1 10 0',copy:'M6 5h8v10H6zM3 11H2V1h8v1',screen:'M1 2.5h14v9H1zM5 14.5h6M8 11.5v3',paste:'M5 3H3v12h10V3h-2M5 1h6v4H5z',commands:'M5 4L1 8l4 4M11 4l4 4-4 4M9 2L7 14',appearance:'M3 2v12M8 2v12M13 2v12M1 5h4M6 11h4M11 6h4',history:'M2 3v5h5M2 8a6 6 0 1 1 2 5M8 4v5l3 1',expand:'M1 6V1h5M10 1h5v5M15 10v5h-5M6 15H1v-5',interrupt:'M4 4h8v8H4z',select:'M6 2h4M8 2v12M6 14h4',image:'M2 3h12v10H2zM2 11l3.5-3.5 2.5 2.5 1.5-1.5L14 12M10.5 6.5h.01'}
</script>
<svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round"><path d={paths[name]}/></svg>
