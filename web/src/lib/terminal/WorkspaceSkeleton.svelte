<script lang="ts">
  // The terminal workspace, before its data is here: the same card, toolbar, sessions panel, session screen and details
  // panel the real workspace draws, with their real widths. Every stage that waits for the workspace (the static boot
  // shell, App's route skeleton, Machines' chunk wait and the workspace's own first read) uses this shape, so a slow
  // load never moves between layouts.
  import { clampContextWidth, CONTEXT_DEFAULT, readContextLayout } from '../sessions/contextLayout'
  import { clampPanelWidth, defaultPanelWidth, isPhone, MINI_WIDTH, panelDocks, panelMode, readLayout, screenShortSide } from './panelLayout'

  let { label = 'Loading your sessions', context = true }: { label?: string; context?: boolean } = $props()

  function stored(key: string) {
    try { return localStorage.getItem(key) } catch { return null }
  }
  const width = innerWidth
  const phone = isPhone(width, screenShortSide(window.screen, width, innerHeight))
  const savedPanel = readLayout(stored('reet.workspace.panel'))
  const mode = panelMode(width, savedPanel.mode, savedPanel.pinned, phone)
  const panel: 'full' | 'mini' | 'closed' = mode === 'mini' ? 'mini' : !phone && mode === 'full' && panelDocks(width, savedPanel.pinned, phone) ? 'full' : 'closed'
  // The same room the real card has: the window, less main's padding, Gaffer's corner and the card's border.
  const gaffer = matchMedia('(min-width: 768px), (max-height: 500px) and (pointer: coarse) and (orientation: landscape)').matches
  const bodyWidth = Math.max(0, width - (width <= 700 ? 16 : 24) - (gaffer ? 88 : 0) - 2)
  const panelWidth = panel === 'mini' ? MINI_WIDTH : clampPanelWidth(savedPanel.width ?? defaultPanelWidth(width), Math.max(0, bodyWidth - 8))
  const areaWidth = Math.max(0, bodyWidth - (panel === 'closed' ? 0 : panelWidth + 8))
  // The details panel only docks in a wide session screen; the area below decides that with a container query, exactly as
  // SessionScreen's 1100 px rule does. Its saved width is clamped to the same 40 % the real panel uses.
  const savedContext = readContextLayout(stored('reet.workspace.context'))
  const contextOpen = $derived(context && (savedContext.open ?? true))
  const contextWidth = clampContextWidth(savedContext.width ?? CONTEXT_DEFAULT, Math.max(0, areaWidth - 8))
</script>

<div class="workspace-skeleton" class:mini={panel === 'mini'} class:context-off={!contextOpen}
  style={`--panel:${panelWidth}px;--context:${contextWidth}px`}
  role="status" aria-busy="true" aria-label={label}>
  <span class="vh">{label}</span>
  <div class="workspace" aria-hidden="true">
    <div class="toolbar">
      <div class="toolbar-left"><i class="back"></i><i class="name"></i><i class="find"></i></div>
      <div class="toolbar-actions"><i class="button small"></i><i class="button primary"></i><i class="button"></i></div>
    </div>
    <div class="body" class:no-panel={panel === 'closed'} class:mini={panel === 'mini'}>
      {#if panel !== 'closed'}
        <aside class="panel">
          <div class="panel-head"><i class="label"></i><span class="panel-tools"><i></i><i></i><i></i></span></div>
          <div class="panel-items">
            {#each [1, 2, 3, 4] as item (item)}
              <div class="session"><i class="dot"></i><div class="session-copy"><i class="line"></i><i class="line short"></i></div></div>
            {/each}
          </div>
          <div class="panel-foot"><i></i><i></i></div>
        </aside>
        <div class="separator"></div>
      {/if}
      <div class="area">
        <div class="area-grid">
          <div class="stage">
            <div class="identity">
              <i class="avatar"></i>
              <div class="identity-copy"><i class="heading"></i><i class="sub"></i></div>
              <i class="status"></i>
            </div>
            <div class="tools"><i class="tool wide"></i><i class="tool"></i><i class="tool"></i><i class="tool"></i></div>
            <div class="screen">
              <div class="screen-head"><i class="title"></i><i class="meta"></i><i class="screen-action"></i></div>
              <div class="screen-lines"><i class="w1"></i><i class="w2"></i><i class="w3"></i><i class="w1"></i></div>
              <div class="composer"></div>
            </div>
          </div>
          <div class="area-separator"></div>
          <div class="details">
            <div class="details-head"><i class="label"></i><i class="status"></i></div>
            <div class="details-tabs"><i></i><i></i><i></i><i></i></div>
            <div class="details-card"></div>
            <div class="details-card short"></div>
          </div>
        </div>
      </div>
    </div>
  </div>
</div>

<style>
  /* main.terminal-layout is not a resolved-height flex box, so 100% would collapse to the skeleton's content. The real
     workspace takes the window below the header and the main padding; match that. */
  .workspace-skeleton { display: flex; flex-direction: column; height: calc(100dvh - 73px); min-height: 220px; min-width: 0; }
  @media (max-width: 700px) { .workspace-skeleton { height: calc(100dvh - 63px); } }
  /* The workspace uses its full width; Gaffer floats over the corner, as beside the real one. */
  .workspace { display: flex; flex-direction: column; flex: 1; min-height: 0; overflow: hidden; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
  .toolbar { display: flex; align-items: center; gap: 12px; flex-shrink: 0; height: 55px; padding: 8px 12px; border-bottom: 1px solid var(--line); }
  .toolbar-left, .toolbar-actions { display: flex; align-items: center; gap: 8px; }
  .toolbar-actions { margin-left: auto; }
  i { display: block; background: var(--line); border-radius: 6px; }
  .back { width: 26px; height: 26px; border-radius: 7px; }
  .name { width: 106px; height: 18px; border-radius: 7px; }
  .find { width: 54px; height: 30px; border-radius: 8px; margin-left: 4px; }
  .button { width: 40px; height: 32px; border-radius: 8px; }
  .button.small { width: 34px; }
  .button.primary { width: 92px; }
  .body { display: grid; grid-template-columns: var(--panel) 8px minmax(0, 1fr); flex: 1; min-height: 0; position: relative; }
  .body.mini { grid-template-columns: 64px 8px minmax(0, 1fr); }
  .body.no-panel { grid-template-columns: minmax(0, 1fr); }
  .body.no-panel .separator { display: none; }
  .panel { display: flex; flex-direction: column; min-width: 0; min-height: 0; background: var(--card); }
  .panel-head { display: flex; align-items: center; gap: 8px; padding: 12px 14px; border-bottom: 1px solid var(--line); }
  .panel-head .label { width: 90px; height: 13px; }
  .panel-tools { display: flex; gap: 8px; margin-left: auto; }
  .panel-tools i { width: 16px; height: 16px; }
  .panel-items { display: grid; align-content: start; gap: 6px; padding: 10px; }
  .session { display: flex; align-items: center; gap: 10px; padding: 12px 10px; border: 1px solid transparent; border-radius: 9px; }
  .session:nth-child(2) { border-color: var(--line); background: var(--bg); }
  .session .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; }
  .session-copy { display: grid; gap: 8px; flex: 1; min-width: 0; }
  .session-copy .line { width: 86%; height: 11px; }
  .session-copy .line.short { width: 58%; height: 9px; }
  .panel-foot { display: grid; gap: 10px; margin-top: auto; padding: 14px; border-top: 1px solid var(--line); }
  .panel-foot i { width: 46%; height: 10px; }
  /* The mini rail keeps its controls and drops its words, as the real rail does. */
  .mini .panel-head { justify-content: center; padding: 12px 8px; }
  .mini .panel-head .label, .mini .panel-tools i:not(:first-child), .mini .session-copy, .mini .panel-foot { display: none; }
  .mini .session { justify-content: center; padding: 12px 8px; }
  .area { min-width: 0; min-height: 0; container-type: inline-size; }
  .area-grid { display: grid; grid-template-columns: minmax(0, 1fr); height: 100%; min-width: 0; min-height: 0; }
  .stage { display: flex; flex-direction: column; min-width: 0; min-height: 0; overflow: hidden; background: var(--bg); }
  .identity { display: flex; align-items: center; gap: 12px; min-height: 50px; padding: 8px 16px; }
  .avatar { width: 32px; height: 32px; border-radius: 9px; flex: none; }
  .identity-copy { display: grid; gap: 8px; flex: 1; min-width: 0; }
  .identity-copy .heading { width: min(280px, 52%); height: 20px; border-radius: 7px; }
  .identity-copy .sub { width: min(190px, 36%); height: 10px; }
  .status { width: 96px; height: 28px; border-radius: 999px; flex: none; }
  .tools { display: flex; align-items: center; gap: 8px; min-height: 52px; padding: 8px 16px; border-top: 1px solid var(--line); }
  .tool { width: 72px; height: 29px; border-radius: 8px; }
  .tool.wide { width: 104px; }
  .screen { display: flex; flex-direction: column; flex: 1; min-height: 0; margin: 0 10px 10px; border: 1px solid var(--line); border-radius: 9px; background: var(--card); overflow: hidden; }
  .screen-head { display: flex; align-items: center; gap: 12px; min-height: 44px; padding: 0 14px; border-bottom: 1px solid var(--line); }
  .screen-head .title { width: 140px; height: 12px; }
  .screen-head .meta { width: 92px; height: 9px; }
  .screen-head .screen-action { width: 74px; height: 28px; margin-left: auto; border-radius: 8px; }
  .screen-lines { display: grid; gap: 14px; align-content: start; flex: 1; padding: 22px 18px; }
  .screen-lines i { height: 10px; }
  .screen-lines .w1 { width: 44%; }
  .screen-lines .w2 { width: 68%; }
  .screen-lines .w3 { width: 56%; }
  .composer { display: none; }
  .area-separator { display: none; min-width: 0; }
  .details { display: none; flex-direction: column; min-width: 0; min-height: 0; border-left: 1px solid var(--line); background: var(--card); }
  .details-head { display: flex; align-items: center; gap: 10px; padding: 14px; border-bottom: 1px solid var(--line); }
  .details-head .label { width: 100px; height: 13px; }
  .details-head .status { width: 64px; height: 10px; margin-left: auto; border-radius: 4px; }
  .details-tabs { display: flex; gap: 12px; padding: 12px 14px; border-bottom: 1px solid var(--line); }
  .details-tabs i { width: 54px; height: 10px; }
  .details-card { margin: 14px; height: 96px; border: 1px solid var(--line); border-radius: 9px; background: var(--bg); }
  .details-card.short { height: 54px; }
  @container (min-width: 1100px) {
    .area-grid { grid-template-columns: minmax(0, 1fr) 8px var(--context); }
    .area-separator, .details { display: flex; }
  }
  .context-off .area-grid { grid-template-columns: minmax(0, 1fr); }
  .context-off .area-separator, .context-off .details { display: none; }
  @media (prefers-reduced-motion: no-preference) {
    .workspace-skeleton i { background: linear-gradient(90deg, var(--line) 0%, var(--line-soft) 50%, var(--line) 100%); background-size: 200% 100%; animation: workspace-shimmer 1.8s ease-in-out infinite; }
  }
  @keyframes workspace-shimmer { to { background-position: -200% 0; } }
  @media (prefers-reduced-motion: reduce) { .workspace-skeleton i { animation: none; } }
  @media (max-width: 700px) {
    .toolbar { padding: 6px 8px; gap: 6px; }
    .name { width: 68px; }
    .find, .button.small { display: none; }
    .button.primary { width: 62px; }
  }
  @media (max-width: 650px) {
    .workspace { border-radius: 8px; }
    .toolbar { padding: 5px 6px; }
    .name { width: 54px; height: 15px; }
    .button { height: 30px; }
    .body, .body.mini { grid-template-columns: minmax(0, 1fr); }
    .panel, .separator { display: none; }
    .identity { padding: 7px 10px; }
    .identity-copy .heading { width: 62%; }
    .identity-copy .sub, .status, .tools { display: none; }
    .screen { margin: 0 6px 6px; }
    .screen-head { padding: 0 10px; }
    .screen-head .title { width: 100px; }
  }
</style>
