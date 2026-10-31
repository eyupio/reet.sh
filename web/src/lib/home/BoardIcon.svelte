<script lang="ts">let { name, size = 16 }: { name: string; size?: number } = $props()</script>
<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  {#if name === 'board'}<rect x="3" y="4" width="18" height="16" rx="3"/><path d="M9 4v16M15 4v16"/>
  {:else if name === 'backlog'}<path d="M5 4h14v16H5zM9 8h6M9 12h6M9 16h3"/>
  {:else if name === 'active'}<path d="m9 5 11 7-11 7z"/>
  {:else if name === 'finished'}<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/>
  {:else if name === 'search'}<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>
  {:else if name === 'filter'}<path d="M4 7h16M4 17h16"/><circle cx="8" cy="7" r="2" fill="var(--card)"/><circle cx="16" cy="17" r="2" fill="var(--card)"/>
  {:else if name === 'machine'}<rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/>
  {:else if name === 'folder'}<path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>
  {:else if name === 'image'}<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8" cy="8" r="1.5"/><path d="m21 16-6-6-9 11"/>
  {:else if name === 'plus'}<path d="M12 5v14M5 12h14"/>
  {:else if name === 'arrow'}<path d="M5 12h14m-5-5 5 5-5 5"/>
  {:else if name === 'more'}<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>
  {:else if name === 'clock'}<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>
  {:else if name === 'close'}<path d="m6 6 12 12M6 18 18 6"/>
  {/if}
</svg>
