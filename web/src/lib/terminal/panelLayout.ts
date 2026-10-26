// How the terminal workspace's sessions panel sits beside the terminal: docked
// (a grid column, never over the terminal), a drawer (phones, and a tablet whose
// person turned Keep open off), or a 64 px mini rail. No browser needed, so the
// rules are unit tested.

export type PanelMode = 'full' | 'mini' | 'closed'
export type SavedLayout = { mode: PanelMode | undefined; width: number | undefined; pinned: boolean }

export const MINI_WIDTH = 64
export const PANEL_MIN = 240, PANEL_MAX = 480, PANEL_DEFAULT = 280
/** A drag that asks for less than this becomes the mini rail. */
export const SNAP_TO_MINI = 160
/** The panel never takes more than this share of the workspace, so the terminal keeps at least 60 %. */
export const PANEL_SHARE = 0.4
/** Keyboard step for the separator's arrow keys. */
export const KEY_STEP = 16
/** A screen whose shorter side (or a window) is under this is a phone: the panel is a drawer and never docks. */
export const DOCK_FROM = 600
/** From this width the full panel always docks; between DOCK_FROM and here it docks while Keep open is on (the default). */
export const ALWAYS_DOCKED = 960
/** A first visit under ALWAYS_DOCKED starts the panel at the minimum, so a tablet's terminal keeps the most columns. */
export const TABLET_DEFAULT = 240

const MODES: readonly PanelMode[] = ['full', 'mini', 'closed']

/**
 * A phone: a window narrower than 600 px, or a device whose screen's shorter side is under 600 px (the test Android
 * uses for tablets), so a phone held sideways stays a phone while a small tablet with tall browser chrome does not.
 * The terminal's own sizing rules still use LANDSCAPE_PHONE in sizing.ts; only the panel decides by the screen.
 */
export function isPhone(width: number, screenShortSide: number): boolean {
  return width < DOCK_FROM || screenShortSide < DOCK_FROM
}

/** The device screen's shorter side, or the window's when the screen is not reported. */
export function screenShortSide(screen: { width?: number; height?: number } | undefined, innerWidth: number, innerHeight: number): number {
  const width = screen?.width || innerWidth, height = screen?.height || innerHeight
  return Math.min(width, height)
}

/** Whether the full panel is a docked column (true) or today's drawer over the terminal (false). */
export function panelDocks(width: number, pinned: boolean, phone = false): boolean {
  return !phone && width >= DOCK_FROM && (pinned || width >= ALWAYS_DOCKED)
}

/**
 * The panel a page load starts with. Phones start closed (the drawer opens on request). A tablet with Keep open turned
 * off starts closed too, as the drawer would cover the terminal; a mini rail covers nothing, so it comes back anywhere.
 */
export function panelMode(width: number, saved: PanelMode | undefined, pinned: boolean, phone = false): PanelMode {
  if (phone || width < DOCK_FROM) return 'closed'
  const mode = saved ?? 'full'
  if (mode === 'full' && !panelDocks(width, pinned)) return 'closed'
  return mode
}

/** The widest the docked panel may be in a workspace this wide; an unmeasured (0) workspace only has the 480 px limit. */
export function widestPanel(containerWidth: number): number {
  return containerWidth > 0 ? Math.min(PANEL_MAX, Math.floor(containerWidth * PANEL_SHARE)) : PANEL_MAX
}

/**
 * The panel width to draw: 240 to 480 px, and never more than 40 % of the workspace, so the terminal keeps at least
 * 60 % (in a workspace under 600 px the 60 % wins over the 240 px minimum).
 */
export function clampPanelWidth(requested: number, containerWidth: number): number {
  const wanted = Number.isFinite(requested) ? Math.round(requested) : PANEL_DEFAULT
  return Math.min(widestPanel(containerWidth), Math.max(PANEL_MIN, wanted))
}

/** Where a drag of the separator lands: below 160 px the mini rail, otherwise the full panel at a clamped width. */
export function dragWidth(requested: number, containerWidth: number): { mode: 'mini'; width: null } | { mode: 'full'; width: number } {
  return requested < SNAP_TO_MINI ? { mode: 'mini', width: null } : { mode: 'full', width: clampPanelWidth(requested, containerWidth) }
}

/**
 * The separator's keys. Left/Right move 16 px; Left at the minimum shrinks to the mini rail and Right from the rail opens
 * the panel at its minimum. Home is the smallest (the rail), End the widest. The width the panel returns to is kept.
 */
export function keyWidth(key: string, mode: PanelMode, width: number, containerWidth: number): { mode: PanelMode; width: number } | null {
  const current = clampPanelWidth(width, containerWidth)
  if (key === 'Home') return { mode: 'mini', width }
  if (key === 'End') return { mode: 'full', width: widestPanel(containerWidth) }
  if (key === 'ArrowRight') return mode === 'mini' ? { mode: 'full', width: clampPanelWidth(PANEL_MIN, containerWidth) } : { mode: 'full', width: clampPanelWidth(current + KEY_STEP, containerWidth) }
  if (key === 'ArrowLeft') {
    if (mode === 'mini') return { mode, width }
    return current <= PANEL_MIN ? { mode: 'mini', width: current } : { mode: 'full', width: clampPanelWidth(current - KEY_STEP, containerWidth) }
  }
  return null
}

/** The width a first visit starts at: 240 px under 960 px, 280 px from there. */
export function defaultPanelWidth(width: number): number {
  return width < ALWAYS_DOCKED ? TABLET_DEFAULT : PANEL_DEFAULT
}

/** The layout a first visit has; it is never written, so storage only ever holds a person's own choice. */
export function defaultLayout(width: number) {
  return layoutRecord('full', defaultPanelWidth(width), true)
}

/**
 * A change of mode. Expanding the mini rail on a tablet or desktop turns Keep open on, so the panel docks where the rail
 * was instead of opening as a drawer over the terminal (and a drag that started on the rail keeps its separator).
 */
export function changeMode(from: PanelMode, to: PanelMode, pinned: boolean, phone: boolean): { mode: PanelMode; pinned: boolean } {
  return { mode: to, pinned: pinned || (!phone && from === 'mini' && to === 'full') }
}

/** The panel's chevron: full → mini → closed → full. A phone has no rail, so its chevron only closes and opens. */
export function chevronNext(mode: PanelMode, phone: boolean): PanelMode {
  if (phone) return mode === 'closed' ? 'full' : 'closed'
  return mode === 'full' ? 'mini' : mode === 'mini' ? 'closed' : 'full'
}

/**
 * Reads the saved layout. Older pages saved only {open, width} (width 220 to 480): open maps to full or closed and the
 * width is brought into today's range. A missing Keep open counts as on. Anything unreadable falls back to the
 * defaults; a missing width is left to the screen (defaultPanelWidth).
 */
export function readLayout(raw: string | null | undefined): SavedLayout {
  const layout: SavedLayout = { mode: undefined, width: undefined, pinned: true }
  let saved: unknown
  try { saved = JSON.parse(raw || '{}') } catch { return layout }
  if (!saved || typeof saved !== 'object' || Array.isArray(saved)) return layout
  const record = saved as Record<string, unknown>
  if (MODES.includes(record.mode as PanelMode)) layout.mode = record.mode as PanelMode
  else if (typeof record.open === 'boolean') layout.mode = record.open ? 'full' : 'closed'
  if (typeof record.width === 'number' && Number.isFinite(record.width)) layout.width = Math.round(Math.min(PANEL_MAX, Math.max(PANEL_MIN, record.width)))
  if (typeof record.pinned === 'boolean') layout.pinned = record.pinned
  return layout
}

/** What is saved: the mode, the width, whether it is pinned, and `open` so an older page still reads it. */
export function layoutRecord(mode: PanelMode, width: number, pinned: boolean) {
  return { open: mode === 'full', mode, width: Math.round(Math.min(PANEL_MAX, Math.max(PANEL_MIN, width))), pinned }
}
