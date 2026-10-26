// Terminal sizing rules that do not need a browser: the font default, the size
// sent to the host, resize hygiene, and how a phone keyboard is detected.

export type Size = { cols: number; rows: number }

export const FONT_MIN = 10, FONT_MAX = 22, DESKTOP_FONT = 14, PHONE_FONT = 11
// Below this the agent re-lays out for a grid it cannot use, and a full-screen
// app may truncate what it later has to repaint. The protocol allows 2×1.
export const MIN_COLS = 20, MIN_ROWS = 8, MAX_COLS = 500, MAX_ROWS = 300

/** The stored preference wins when it is in range; otherwise phones start smaller so agent layouts fit. */
export function initialFontSize(stored: string | null | undefined, phone: boolean): number {
  const size = stored ? Number(stored) : NaN
  return size >= FONT_MIN && size <= FONT_MAX ? size : phone ? PHONE_FONT : DESKTOP_FONT
}

export function hostSize(cols: number, rows: number): Size {
  return { cols: Math.min(MAX_COLS, Math.max(MIN_COLS, cols)), rows: Math.min(MAX_ROWS, Math.max(MIN_ROWS, rows)) }
}

const same = (a: Size | null, b: Size | null) => !!a && !!b && a.cols === b.cols && a.rows === b.rows

type Timer = ReturnType<typeof setTimeout>
export type ResizeSenderOptions = { delay?: number; gap?: number; setTimer?: (fn: () => void, ms: number) => Timer; clearTimer?: (timer: Timer) => void }

/**
 * Sends the host its terminal size: only after a burst of layout changes has
 * settled, never a size the host already has, and, on request, a one-row nudge
 * that makes a full-screen app repaint (tmux forwards a real size change).
 * `send` returns false when the connection could not deliver the frame.
 */
export class ResizeSender {
  private sent: Size | null = null
  private wanted: Size | null = null
  private timer: Timer | null = null
  private nudging = false
  private readonly send: (size: Size) => boolean
  private readonly delay: number
  private readonly gap: number
  private readonly setTimer: (fn: () => void, ms: number) => Timer
  private readonly clearTimer: (timer: Timer) => void

  constructor(send: (size: Size) => boolean, options: ResizeSenderOptions = {}) {
    this.send = send
    this.delay = options.delay ?? 150
    this.gap = options.gap ?? 300
    this.setTimer = options.setTimer ?? ((fn, ms) => setTimeout(fn, ms))
    this.clearTimer = options.clearTimer ?? (timer => clearTimeout(timer))
  }
  /** The host already has this size, e.g. from the attachment ticket. */
  known(size: Size) { this.cancel(); this.sent = { ...size }; this.wanted = { ...size } }
  request(size: Size) {
    this.wanted = { ...size }
    // A nudge in progress already ends on the wanted size, after its full gap.
    if (this.nudging) return
    this.cancel(); this.timer = this.setTimer(() => this.flush(), this.delay)
  }
  nudge() {
    const target = this.wanted ?? this.sent
    if (!target) return
    this.cancel()
    const step = { cols: target.cols, rows: target.rows > 1 ? target.rows - 1 : target.rows + 1 }
    if (!this.send(step)) return
    this.sent = step
    this.nudging = true
    this.timer = this.setTimer(() => this.flush(), this.gap)
  }
  dispose() { this.cancel() }
  private flush() {
    this.timer = null; this.nudging = false
    if (!this.wanted || same(this.wanted, this.sent)) return
    if (this.send({ ...this.wanted })) this.sent = { ...this.wanted }
  }
  private cancel() { if (this.timer !== null) this.clearTimer(this.timer); this.timer = null; this.nudging = false }
}

export type ViewportReading = { innerWidth: number; innerHeight: number; visual?: { height: number; offsetTop: number; scale: number } | null }
export type UsableViewport = { height: number; top: number; shrink: number }

// A layout viewport this much shorter than its tallest at the same width means
// the browser resized the page for the keyboard (interactive-widget=resizes-content).
const LAYOUT_RESIZE = 0.15

/**
 * The height the terminal can use. Normally the visual viewport (iOS, and
 * Android when the keyboard overlays the page). When the browser resized the
 * layout viewport itself, innerHeight is the visible page; a visual viewport
 * that reports less (one browser subtracts a bottom toolbar it has already
 * hidden) would otherwise leave a blank band above the keyboard.
 */
export class ViewportTracker {
  private width = -1
  private tallestInner = 0
  private tallestUsable = 0
  read(reading: ViewportReading): UsableViewport {
    if (reading.innerWidth !== this.width) { this.width = reading.innerWidth; this.tallestInner = 0; this.tallestUsable = 0 }
    this.tallestInner = Math.max(this.tallestInner, reading.innerHeight)
    const layoutResized = this.tallestInner - reading.innerHeight > this.tallestInner * LAYOUT_RESIZE
    const visual = reading.visual
    const usable = !visual || layoutResized ? { height: reading.innerHeight, top: 0 } : { height: visual.height, top: visual.offsetTop }
    this.tallestUsable = Math.max(this.tallestUsable, usable.height)
    return { ...usable, shrink: this.tallestUsable ? 1 - usable.height / this.tallestUsable : 0 }
  }
}

/** An on-screen keyboard is open over the terminal: a narrow touch screen lost a quarter of its height while typing in the terminal, without zooming. */
export function keyboardLikelyOpen(shrink: number, context: { coarse: boolean; narrow: boolean; editing: boolean; scale: number }): boolean {
  return context.coarse && context.narrow && context.editing && context.scale <= 1.05 && shrink >= 0.25
}

/** Which ends of a sideways-scrolling row hide controls. */
export function overflowEdges(scrollLeft: number, scrollWidth: number, clientWidth: number): { start: boolean; end: boolean } {
  return { start: scrollLeft > 1, end: scrollLeft + clientWidth < scrollWidth - 1 }
}

/** The current browser viewport, read for ViewportTracker. */
export function browserViewport(): ViewportReading {
  const visual = window.visualViewport
  return { innerWidth, innerHeight, visual: visual ? { height: visual.height, offsetTop: visual.offsetTop, scale: visual.scale } : null }
}
// One tracker per page, so the workspace and a maximised pane agree on the tallest height seen.
export const pageViewport = new ViewportTracker()
