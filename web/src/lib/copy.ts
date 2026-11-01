/**
 * Reet UI copy.
 *
 * Every entry keeps a plain, standard string as the source of truth. A `reet`
 * variant is only used on surfaces where personality cannot cost the user
 * clarity. Critical actions, status labels, navigation, form labels and CLI
 * syntax always resolve to `standard`, even when dialect is switched on.
 *
 * `support` is a companion line (subtext, tooltip, status detail) that can sit
 * beside a standard string on any surface, including critical ones.
 *
 * See docs/yorkshire-copy.md for the full usage matrix.
 */

export type CopySurface =
  | 'critical'
  | 'navigation'
  | 'form'
  | 'status'
  | 'success'
  | 'empty'
  | 'onboarding'
  | 'microcopy'
  | 'marketing'
  | 'command'

export type CopyEntry = {
  standard: string
  reet?: string
  support?: string
  surface: CopySurface
}

export const copy = {
  // Navigation, auth and destructive actions. Never dialect: a user must be
  // able to find these without decoding local language.
  'nav.home': { standard: 'Home', surface: 'navigation' },
  'nav.sessions': { standard: 'Sessions', surface: 'navigation' },
  'nav.machines': { standard: 'Machines', surface: 'navigation' },
  'nav.skills': { standard: 'Skills', surface: 'navigation' },
  'nav.activity': { standard: 'Activity', surface: 'navigation' },
  'nav.account': { standard: 'Account', surface: 'navigation' },
  'nav.settings': { standard: 'Settings', surface: 'navigation' },
  'nav.admin': { standard: 'Admin', surface: 'navigation' },
  'nav.more': { standard: 'More', surface: 'navigation' },
  'nav.signOut': { standard: 'Sign out', surface: 'navigation' },
  'nav.signIn': { standard: 'Sign in', surface: 'navigation' },
  'nav.createAccount': { standard: 'Create an account', surface: 'navigation' },

  'action.delete': { standard: 'Delete', surface: 'critical' },
  'action.remove': { standard: 'Remove', surface: 'critical' },
  'action.revoke': { standard: 'Revoke access', surface: 'critical' },
  'action.terminate': { standard: 'Terminate session', surface: 'critical' },
  'action.forget': { standard: 'Forget machine', surface: 'critical' },
  'action.restore': { standard: 'Restore', surface: 'critical' },
  'action.save': { standard: 'Save changes', surface: 'form' },
  'action.cancel': { standard: 'Cancel', surface: 'navigation' },
  'action.confirm': { standard: 'Confirm', surface: 'critical' },
  'action.retry': { standard: 'Try again', surface: 'critical' },
  'action.copy': { standard: 'Copy', surface: 'navigation' },
  'action.copied': { standard: 'Copied', surface: 'navigation' },
  'action.close': { standard: 'Close', surface: 'navigation' },
  'action.refresh': { standard: 'Refresh', surface: 'navigation' },

  'security.deleteWarning': { standard: 'This cannot be undone.', surface: 'critical' },
  'security.revokeWarning': {
    standard: 'This revokes access immediately. The machine will need to be connected again.',
    surface: 'critical',
  },
  'security.secretOnce': {
    standard: 'Copy this token now. It will not be shown again.',
    surface: 'critical',
  },
  'security.permissionRequired': { standard: 'Permission required', surface: 'critical' },

  // Form labels stay familiar. Validation keeps a precise standard sentence and
  // may add a warm, non-essential support line.
  'form.required': {
    standard: 'This field is required.',
    support: 'Give us summat to work with.',
    surface: 'form',
  },
  'form.invalidEmail': { standard: 'Enter a valid email address.', surface: 'form' },
  'form.invalidUrl': { standard: 'Enter a complete, valid URL.', surface: 'form' },
  'form.unsavedChanges': { standard: 'You have unsaved changes.', surface: 'form' },
  'form.saveFailed': { standard: 'Could not save your changes. Try again.', surface: 'critical' },

  // Success. Safe to replace the whole sentence: the action is already done and
  // the surrounding UI carries the state.
  'success.saved': { standard: 'Saved.', reet: 'Sorted. Saved.', surface: 'success' },
  'success.updated': { standard: 'Updated.', reet: 'Reet, updated.', surface: 'success' },
  'success.copied': {
    standard: 'Copied to clipboard.',
    reet: 'Reet, copied to clipboard.',
    surface: 'success',
  },
  'success.created': { standard: 'Created.', reet: "Reet, that's made.", surface: 'success' },
  'success.deleted': { standard: 'Deleted.', reet: 'Gone. That one is deleted.', surface: 'success' },
  'success.connected': {
    standard: 'Machine connected.',
    reet: "Reet, that machine's connected.",
    surface: 'success',
  },
  'success.synced': {
    standard: 'Everything is in sync.',
    reet: "All reet — everything's in sync.",
    surface: 'success',
  },
  'success.deployed': {
    standard: 'Deployed successfully.',
    reet: "Reet good — that's deployed.",
    surface: 'success',
  },
  'success.invited': {
    standard: 'Invitation sent.',
    reet: "Reet, invite's away.",
    surface: 'success',
  },
  'success.taskComplete': {
    standard: 'Task complete.',
    reet: "Reet, job's a good 'un.",
    surface: 'success',
  },

  // Empty states. The strongest dialect lives here, always paired with a plain
  // next step so the screen still tells the user what to do.
  'empty.generic.title': { standard: 'Nothing here yet.', reet: 'Nowt here yet.', surface: 'empty' },
  'empty.generic.body': {
    standard: 'Add something to get started.',
    reet: "Get crackin' and add summat.",
    surface: 'empty',
  },
  'empty.search.title': {
    standard: 'No results match your search.',
    reet: 'Nowt matches just yet.',
    surface: 'empty',
  },
  'empty.search.body': {
    standard: 'Try another search or clear your filters.',
    reet: 'Have another go, or clear your filters.',
    surface: 'empty',
  },
  'empty.machines.title': {
    standard: 'No machines connected yet.',
    reet: 'Nowt connected yet.',
    surface: 'empty',
  },
  'empty.machines.body': {
    standard: 'Connect a machine with the reet CLI to see it here.',
    reet: "Get a machine connected with the reet CLI and it'll show up here.",
    surface: 'empty',
  },
  'empty.sessions.title': { standard: 'No sessions yet.', reet: 'Nowt running yet.', surface: 'empty' },
  'empty.sessions.body': {
    standard: 'Start a session from Home.',
    reet: "Start one from Home when you're ready.",
    surface: 'empty',
  },
  'empty.skills.title': { standard: 'No skills yet.', reet: 'Nowt on the shelf yet.', surface: 'empty' },
  'empty.skills.body': {
    standard: 'Add your first skill to get started.',
    reet: 'Whack your first skill on the shelf.',
    surface: 'empty',
  },
  'empty.notifications.title': {
    standard: 'Nothing needs you.',
    reet: 'Nowt needs you.',
    surface: 'empty',
  },
  'empty.lists.title': { standard: 'No lists yet.', reet: 'Nowt in the drawer yet.', surface: 'empty' },
  'empty.community.title': {
    standard: 'Nothing matches yet.',
    reet: 'Nowt matches just yet.',
    surface: 'empty',
  },

  'notFound.eyebrow': { standard: 'Not found', surface: 'empty' },
  'notFound.title': { standard: 'Page not found', surface: 'empty' },
  'notFound.lede': {
    standard:
      'That address does not lead anywhere on this instance. It may have moved, or the link may be mistyped.',
    reet: 'Nowt lives at that address on this instance. It may have moved, or the link may be mistyped.',
    surface: 'empty',
  },
  'notFound.back': { standard: 'Back to Home', surface: 'navigation' },

  // Onboarding and first-run. Personality sets the tone; the numbered steps and
  // primary controls around it stay standard.
  'onboarding.greeting': { standard: 'Welcome.', reet: 'Ey up.', surface: 'onboarding' },
  'onboarding.welcomeBack': {
    standard: 'Welcome back.',
    reet: 'Ey up. Good to see you again.',
    surface: 'onboarding',
  },
  'onboarding.firstRun.title': {
    standard: "Let's get started.",
    reet: "Reet, let's get crackin'.",
    surface: 'onboarding',
  },
  'onboarding.firstRun.body': {
    standard: 'Connect your first machine to get started.',
    reet: "Connect your first machine and we'll get you sorted.",
    surface: 'onboarding',
  },
  'onboarding.emptyFleet': {
    standard: 'Your fleet is empty.',
    reet: 'Nowt in the fleet yet.',
    surface: 'onboarding',
  },
  'onboarding.stepDone': {
    standard: 'Step complete.',
    reet: "Reet, job's a good 'un.",
    surface: 'onboarding',
  },
  'onboarding.ready': {
    standard: 'You are ready to go.',
    reet: "All reet — you're ready to go.",
    surface: 'onboarding',
  },
  'onboarding.noFaff': {
    standard: 'No unnecessary complexity.',
    reet: 'No faff, just code.',
    surface: 'onboarding',
  },

  // System status. The label is always standard; dialect may appear as the
  // tooltip or detail line, never as the only signal of health.
  'status.operational': {
    standard: 'Operational',
    support: "All reet — everything's ticking over.",
    surface: 'status',
  },
  'status.degraded': {
    standard: 'Degraded',
    support: "Summat's not reet. Some parts are slower or failing.",
    surface: 'status',
  },
  'status.partialOutage': {
    standard: 'Partial outage',
    support: "Summat's not reet for some machines.",
    surface: 'status',
  },
  'status.majorOutage': {
    standard: 'Major outage',
    support: 'Nowt is responding right now.',
    surface: 'status',
  },
  'status.down': { standard: 'Down', support: 'Nowt is responding right now.', surface: 'status' },
  'status.maintenance': {
    standard: 'Maintenance',
    support: 'Planned work. Back shortly.',
    surface: 'status',
  },
  'status.unknown': {
    standard: 'Unknown',
    support: 'We have not heard from it yet.',
    surface: 'status',
  },
  'status.checking': { standard: 'Checking…', support: 'Hold your horses.', surface: 'status' },
  'status.paused': {
    standard: 'Paused',
    support: 'Hold your horses — this one is paused.',
    surface: 'status',
  },
  'status.throttled': {
    standard: 'Rate limited',
    support: 'Hold your horses — too many requests just now.',
    surface: 'status',
  },
  'status.offline': {
    standard: 'Offline',
    support: 'Nowt heard from this machine lately.',
    surface: 'status',
  },
  'status.synced': {
    standard: 'Everything in sync',
    support: 'All reet.',
    surface: 'status',
  },
  'status.syncing': {
    standard: 'Syncing…',
    support: 'Hold your horses — changes are on their way.',
    surface: 'status',
  },
  'status.healthy': {
    standard: 'Healthy and operational',
    support: 'Reet as rain.',
    surface: 'status',
  },

  // Errors. Keep the standard sentence and HTTP meaning intact; the dialect is
  // optional supporting text, never the alert itself.
  'error.generic.title': {
    standard: 'Something went wrong',
    support: "Summat's not reet. The detail below is the reliable bit.",
    surface: 'critical',
  },
  'error.generic.body': {
    standard: 'Try again. If it keeps happening, check the server logs.',
    support: 'No faff: the error code and request ID are the useful bits.',
    surface: 'critical',
  },
  'error.network': {
    standard: 'Could not reach the server. Check your connection and try again.',
    support: "We're not reet sure the request got there.",
    surface: 'critical',
  },
  'error.unauthorized': {
    standard: 'Your session has expired. Sign in again to continue.',
    surface: 'critical',
  },
  'error.forbidden': {
    standard: 'You do not have permission to do that.',
    support: 'Ask an owner or admin on this instance.',
    surface: 'critical',
  },
  'error.notFound': {
    standard: 'That item could not be found.',
    support: 'It may have been removed, or the link may be wrong.',
    surface: 'critical',
  },
  'error.conflict': {
    standard: 'That change conflicts with a newer version.',
    support: 'Refresh, then make your change again.',
    surface: 'critical',
  },
  'error.validation': {
    standard: 'Check the highlighted fields and try again.',
    support: 'Nowt was saved until the form is valid.',
    surface: 'critical',
  },
  'error.rateLimited': {
    standard: 'Too many requests. Wait a moment and try again.',
    support: 'Hold your horses — then have another go.',
    surface: 'critical',
  },
  'error.server': {
    standard: 'The server could not complete that request.',
    support: 'Hold your horses and try again in a moment.',
    surface: 'critical',
  },
  'error.timeout': {
    standard: 'The request timed out.',
    support: 'Hold your horses — the machine may still be working.',
    surface: 'critical',
  },
  'error.offline': {
    standard: 'This machine is offline.',
    support: 'Reconnect it, then try again.',
    surface: 'critical',
  },
  'error.copyFailed': {
    standard: 'Could not copy to the clipboard.',
    support: "Use your browser's copy command instead.",
    surface: 'critical',
  },

  // Small, low-risk moments: tooltips, badges and transient helper text.
  'microcopy.greeting': { standard: 'Hello', reet: 'Ey up', surface: 'microcopy' },
  'microcopy.good': { standard: 'Good', reet: 'Reet good', surface: 'microcopy' },
  'microcopy.veryGood': { standard: 'Very good', reet: 'Reet grand', surface: 'microcopy' },
  'microcopy.exactlyRight': { standard: 'Exactly right', reet: 'Too reet', surface: 'microcopy' },
  'microcopy.fast': { standard: 'Very fast', reet: 'Reet quick', surface: 'microcopy' },
  'microcopy.reliable': { standard: 'Very reliable', reet: 'Reet solid', surface: 'microcopy' },
  'microcopy.happy': { standard: 'Happy with that', reet: 'Chuffed with that', surface: 'microcopy' },
  'microcopy.friction': {
    standard: 'No unnecessary complexity',
    reet: 'No faff, just code',
    surface: 'microcopy',
  },
  'microcopy.processing': { standard: 'Processing…', reet: 'Hold your horses…', surface: 'microcopy' },
  'microcopy.allGood': { standard: 'All good', reet: 'All reet', surface: 'microcopy' },
  'microcopy.done': { standard: 'Done', reet: "Job's a good 'un", surface: 'microcopy' },
  'microcopy.badge': {
    standard: 'New notification',
    reet: 'Ey up — summat new',
    surface: 'microcopy',
  },

  // CLI output. Command syntax, exit codes and diagnostics stay exact; only the
  // human-facing completion or waiting line may carry the accent.
  'cli.installSuccess': {
    standard: 'reet installed successfully.',
    reet: 'Reet, installed.',
    surface: 'success',
  },
  'cli.scanSuccess': {
    standard: 'Scan complete.',
    reet: "Reet, scan's done.",
    surface: 'success',
  },
  'cli.syncSuccess': {
    standard: 'Sync complete.',
    reet: "All reet — sync's done.",
    surface: 'success',
  },
  'cli.waiting': {
    standard: 'Waiting for the machine…',
    reet: 'Hold your horses — waiting for the machine…',
    surface: 'status',
  },
  'cli.error': { standard: 'Error:', surface: 'command' },
  'cli.hint': { standard: 'Run `reet --help` for usage.', surface: 'command' },
  'cli.lowLatency': {
    standard: 'Low latency',
    reet: 'Reet quick',
    support: 'Round-trip time is below the healthy threshold.',
    surface: 'microcopy',
  },
  'cli.stableConnection': {
    standard: 'Stable connection',
    reet: 'Reet solid',
    support: 'No missed probes in the current window.',
    surface: 'microcopy',
  },
  'cli.noResults': { standard: 'No results.', reet: 'Nowt found.', surface: 'empty' },

  // Landing and marketing. This is the loudest place for voice, but CTAs that
  // begin authentication keep their standard labels above.
  'hero.eyebrow': {
    standard: 'Your AI control room.',
    reet: 'Ey up. Your AI control room.',
    surface: 'marketing',
  },
  'hero.title': {
    standard: 'One command on any machine. Your whole AI setup shows up.',
    reet: 'One command on any machine. Your whole AI setup shows up.',
    surface: 'marketing',
  },
  'hero.cta': {
    standard: 'Get started',
    reet: "Reet, let's get crackin'",
    surface: 'marketing',
  },
  'hero.secondaryCta': { standard: 'See how it works', reet: 'Have a nosy', surface: 'marketing' },
  'marketing.control': {
    standard: 'Proper control, on your own kit',
    reet: 'Proper control, on your own kit',
    surface: 'marketing',
  },
  'marketing.toolkit': {
    standard: 'A very good agent toolkit.',
    reet: 'A reet good agent toolkit.',
    surface: 'marketing',
  },
} as const satisfies Record<string, CopyEntry>

export type CopyKey = keyof typeof copy

export type CopyOptions = {
  /** Set false to force the standard string, e.g. for tests or an A/B run. */
  dialect?: boolean
  vars?: Record<string, string | number>
}

const IMMERSIVE_SURFACES: ReadonlySet<CopySurface> = new Set<CopySurface>([
  'success',
  'empty',
  'onboarding',
  'microcopy',
  'marketing',
])

const TOKEN = /\{(\w+)\}/g

function fill(template: string, vars?: Record<string, string | number>): string {
  if (!vars) return template
  return template.replace(TOKEN, (match, key: string) =>
    Object.hasOwn(vars, key) ? String(vars[key]) : match,
  )
}

/**
 * Resolve a copy key. Dialect is applied only when the entry's surface is
 * immersive and a `reet` variant exists. Critical, navigation, form, status
 * and command surfaces always return the standard string.
 */
export function t(key: CopyKey, options: CopyOptions = {}): string {
  const entry: CopyEntry = copy[key]
  const useDialect =
    options.dialect !== false && IMMERSIVE_SURFACES.has(entry.surface) && entry.reet !== undefined
  return fill(useDialect ? entry.reet! : entry.standard, options.vars)
}

/**
 * The companion line for a key. Safe to show next to a standard label or error
 * on any surface; empty when the entry has none.
 */
export function support(key: CopyKey): string {
  const entry: CopyEntry = copy[key]
  return entry.support ?? ''
}

/** True when `t(key)` may return a Yorkshire variant. */
export function hasDialect(key: CopyKey): boolean {
  const entry: CopyEntry = copy[key]
  return IMMERSIVE_SURFACES.has(entry.surface) && entry.reet !== undefined
}
