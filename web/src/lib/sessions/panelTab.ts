// The view chosen in the session workspace's panel. It is a browser preference,
// not session state: switching to another session keeps the same view open.
// Kept free of browser globals so node:test can run it.

export const PANEL_TAB_KEY = 'reet.workspace.context-tab'
export const PANEL_TABS = ['overview', 'changes', 'files', 'github'] as const
export type PanelTab = (typeof PANEL_TABS)[number]

/** The saved view, or Overview when nothing valid has been chosen. */
export function readPanelTab(raw: string | null | undefined): PanelTab {
  return raw === 'changes' || raw === 'files' || raw === 'github' ? raw : 'overview'
}
