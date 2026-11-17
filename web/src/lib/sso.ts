// Everything the interface decides about single sign-on lives here, so the sign-in page, the account page and the
// admin page agree and one test covers the words.

export type SsoStatus = { enabled?: boolean; label?: string; passwordLogin?: boolean }

export type SsoIdentity = { enabled?: boolean; label?: string; linked?: boolean; hasPassword?: boolean; linkedAt?: number; lastLoginAt?: number; canUnlink?: boolean }

export type AdminSso = {
  enabled?: boolean
  issuer?: string
  redirectUrl?: string
  label?: string
  /** The button text as stored; empty means the default words. `label` is the text in use. */
  labelSetting?: string
  passwordLogin?: boolean
  autoCreate?: boolean
  linkByEmail?: boolean
  defaultRole?: string
  nameClaim?: string
  emailClaim?: string
  groupsClaim?: string
  scopes?: string[]
  adminGroups?: string[]
  memberGroups?: string[]
  viewerGroups?: string[]
  linked?: number
  reachable?: boolean
  problem?: string
}

const defaultLabel = 'Sign in with single sign-on'

// signInChoices says what the sign-in page offers. With single sign-on off the answer is the page as it always was.
// With it on the button comes first. Where passwords are refused for everyone but the owner the form is folded away
// rather than removed, because the owner can still sign in with one and has nowhere else to do it.
export function signInChoices(sso?: SsoStatus): { button: boolean; label: string; passwordForm: 'open' | 'folded' } {
  if (!sso?.enabled) return { button: false, label: '', passwordForm: 'open' }
  const label = (sso.label ?? '').trim() || defaultLabel
  return { button: true, label, passwordForm: sso.passwordLogin === false ? 'folded' : 'open' }
}

const messages: Record<string, string> = {
  denied: 'Sign-in was cancelled or refused by your identity provider. Try again when you are ready.',
  expired: 'That sign-in took too long or was already used. Start it again.',
  mismatch: 'That sign-in was finished in a different browser from the one that began it. Start it again here.',
  failed: 'Your identity provider answered in a way this instance could not trust. Try again, or ask an admin to check the server log.',
  unavailable: 'Your identity provider could not be reached. Try again shortly.',
  not_ready: 'This instance has no owner yet. Finish setting it up first.',
  no_account: 'There is no account for you here, and accounts are not created automatically. Ask an admin for an invitation.',
  exists: 'An account with your email address already exists and is not linked to your identity. Ask an admin, or sign in with your password and link it from Account.',
  not_linkable: 'That email address belongs to an owner or admin, which single sign-on will not take over. Sign in with your password.',
  email_missing: 'Your identity provider did not send an email address, so no account could be matched.',
  email_unverified: 'Your identity provider does not vouch for your email address, so no account could be matched.',
  disabled: 'This account is disabled. Ask an admin.',
  already_linked: 'This account is already linked to an identity. Unlink it first if you want to link another.',
  linked_elsewhere: 'That identity is already linked to another account.',
  signin_required: 'Linking needs you to stay signed in here. Sign in and try again.',
  rate_limited: 'Too many sign-ins are starting. Try again in a minute.',
  busy: 'Too many sign-ins are waiting. Try again in a minute.',
}

// ssoErrorMessage turns a code from the server into a sentence. A code it does not know gets one general sentence,
// never the code itself, so nothing in the address is ever printed on the page.
export function ssoErrorMessage(code: string): string {
  if (!code) return ''
  return messages[code] ?? 'Single sign-on could not finish. Try again, or sign in another way.'
}

// ssoErrorFromSearch reads ?sso_error=CODE and accepts only a plain code.
export function ssoErrorFromSearch(search: string): string {
  const code = new URLSearchParams(search).get('sso_error') ?? ''
  return /^[a-z_]{1,40}$/.test(code) ? code : ''
}

// withoutSsoError is the address with the code taken off, so a reload does not show the message again.
export function withoutSsoError(url: string): string {
  const u = new URL(url, 'http://local.invalid')
  u.searchParams.delete('sso_error')
  return u.pathname + u.search + u.hash
}

const startPath = '/api/v2/auth/oidc/start'

export function ssoStartHref(next: string): string {
  return next ? `${startPath}?next=${encodeURIComponent(next)}` : startPath
}

export function ssoLinkHref(next = '/account/sso'): string {
  return `${startPath}?intent=link&next=${encodeURIComponent(next)}`
}

// ssoNext is where a sign-in should resume: the page the person was on, unless that is the home page or an entry page,
// where the server's own default is the right place.
export function ssoNext(pathname: string, search: string): string {
  if (['/', '/signin', '/signup', '/welcome', '/setup'].includes(pathname)) return ''
  return pathname + search
}

export function linkedLine(id: SsoIdentity, fmt: (ms: number) => string): string {
  if (!id.linked) return ''
  let line = `Linked since ${fmt(id.linkedAt ?? 0)}.`
  if (id.lastLoginAt) line += ` Last used to sign in ${fmt(id.lastLoginAt)}.`
  return line
}

function list(v?: string[]): string {
  return v && v.length ? v.join(', ') : 'None'
}

// adminRows is the read-only panel on the admin page. The server never sends the secret and this never asks for it.
export function adminRows(s: AdminSso): { label: string; value: string }[] {
  return [
    { label: 'Provider', value: s.reachable ? 'Reachable' : `Not reachable${s.problem ? `: ${s.problem}` : ''}` },
    { label: 'Issuer', value: s.issuer ?? '' },
    { label: 'Redirect address', value: s.redirectUrl ?? '' },
    { label: 'Button', value: s.label ?? s.labelSetting ?? '' },
    { label: 'Password sign-in', value: s.passwordLogin === false ? 'Owner only' : 'Everyone' },
    { label: 'New people', value: s.autoCreate ? 'Created automatically' : 'Need an account first' },
    { label: 'Linking by email', value: s.linkByEmail ? 'On, for verified addresses' : 'Off' },
    { label: 'Default role', value: s.defaultRole ?? '' },
    { label: 'Name claim', value: s.nameClaim || 'name' },
    { label: 'Email claim', value: s.emailClaim || 'email' },
    { label: 'Groups claim', value: s.groupsClaim || 'groups' },
    { label: 'Scopes asked for', value: s.scopes && s.scopes.length ? s.scopes.join(' ') : 'openid' },
    { label: 'Admin groups', value: list(s.adminGroups) },
    { label: 'Member groups', value: list(s.memberGroups) },
    { label: 'Viewer groups', value: list(s.viewerGroups) },
    { label: 'Linked people', value: String(s.linked ?? 0) },
  ]
}

// The owner's form. The server takes the whole set at once, so the form keeps every field as text or a flag and turns
// them into the body in one place, where a test can see it.

export type SsoForm = {
  label: string
  passwordLogin: boolean
  autoCreate: boolean
  linkByEmail: boolean
  defaultRole: 'member' | 'viewer'
  nameClaim: string
  emailClaim: string
  groupsClaim: string
  scopes: string
  adminGroups: string
  memberGroups: string
  viewerGroups: string
}

export type SsoSettings = {
  label: string
  passwordLogin: boolean
  autoCreate: boolean
  linkByEmail: boolean
  defaultRole: string
  nameClaim: string
  emailClaim: string
  groupsClaim: string
  scopes: string[]
  adminGroups: string[]
  memberGroups: string[]
  viewerGroups: string[]
}

/** Entries are separated by commas, spaces or new lines, as the environment variables always were. */
export function parseList(text: string): string[] {
  return text.split(/[\s,]+/).filter(Boolean)
}

export function ssoFormFrom(s: AdminSso): SsoForm {
  return {
    label: s.labelSetting ?? '',
    passwordLogin: s.passwordLogin !== false,
    autoCreate: !!s.autoCreate,
    linkByEmail: !!s.linkByEmail,
    defaultRole: s.defaultRole === 'member' ? 'member' : 'viewer',
    nameClaim: s.nameClaim ?? '',
    emailClaim: s.emailClaim ?? '',
    groupsClaim: s.groupsClaim ?? '',
    scopes: (s.scopes ?? ['openid']).join(' '),
    adminGroups: (s.adminGroups ?? []).join(', '),
    memberGroups: (s.memberGroups ?? []).join(', '),
    viewerGroups: (s.viewerGroups ?? []).join(', '),
  }
}

/** The whole set, every field present, as the server insists. */
export function ssoBodyFrom(f: SsoForm): SsoSettings {
  return {
    label: f.label.trim(),
    passwordLogin: f.passwordLogin,
    autoCreate: f.autoCreate,
    linkByEmail: f.linkByEmail,
    defaultRole: f.defaultRole,
    nameClaim: f.nameClaim.trim(),
    emailClaim: f.emailClaim.trim(),
    groupsClaim: f.groupsClaim.trim(),
    scopes: parseList(f.scopes),
    adminGroups: parseList(f.adminGroups),
    memberGroups: parseList(f.memberGroups),
    viewerGroups: parseList(f.viewerGroups),
  }
}

export const ssoFieldNames = ['label', 'passwordLogin', 'autoCreate', 'linkByEmail', 'defaultRole', 'nameClaim', 'emailClaim', 'groupsClaim', 'scopes', 'adminGroups', 'memberGroups', 'viewerGroups'] as const
export type SsoField = (typeof ssoFieldNames)[number]

/** One plain sentence for each switch. */
export const ssoHelp: Record<SsoField, string> = {
  label: 'The words on the sign-in button. Leave empty for the default.',
  passwordLogin: 'Let everyone sign in with a password as well. Turned off, only the owner can use one, so the owner always has a way in.',
  autoCreate: 'Make an account the first time someone the identity provider vouches for signs in. Off, they need an account here first.',
  linkByEmail: 'Match a first sign-in to the existing account with the same email address. It never links an owner or an administrator, and it relies on the identity provider saying the address is verified.',
  defaultRole: 'The role a new person gets when no group below matches.',
  nameClaim: 'Which claim carries the person’s name. Empty means name.',
  emailClaim: 'Which claim carries the person’s email address. Empty means email.',
  groupsClaim: 'Which claim carries the person’s groups. Empty means groups.',
  scopes: 'What reet.sh asks the provider for, separated by spaces. It must include openid.',
  adminGroups: 'Groups that give the administrator role, separated by commas. The highest match wins.',
  memberGroups: 'Groups that give the member role, separated by commas.',
  viewerGroups: 'Groups that give the viewer role, separated by commas.',
}

/** The field a refusal belongs to, when it is one of the twelve; anything else is shown on the form as a whole. */
export function ssoRefusalField(field: string): SsoField | '' {
  return (ssoFieldNames as readonly string[]).includes(field) ? (field as SsoField) : ''
}
