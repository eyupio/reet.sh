import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
// Compile the pure single sign-on helper without a browser or an extra test dependency.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { signInChoices, ssoErrorMessage, ssoErrorFromSearch, withoutSsoError, ssoStartHref, ssoLinkHref, ssoNext, linkedLine, adminRows } = await import(`data:text/javascript;base64,${Buffer.from(compile('./sso.ts')).toString('base64')}`)

test('with single sign-on off the sign-in page is exactly what it was', () => {
  for (const sso of [undefined, {}, { enabled: false }, { enabled: false, label: 'Sign in', passwordLogin: false }]) {
    assert.deepEqual(signInChoices(sso), { button: false, label: '', passwordForm: 'open' })
  }
})

test('with it on the button leads, and the password form stays open while passwords are allowed', () => {
  assert.deepEqual(signInChoices({ enabled: true, label: 'Sign in with Acme', passwordLogin: true }), { button: true, label: 'Sign in with Acme', passwordForm: 'open' })
})

test('with passwords refused for everyone but the owner the form is folded away, not removed', () => {
  assert.deepEqual(signInChoices({ enabled: true, label: 'Sign in with Acme', passwordLogin: false }), { button: true, label: 'Sign in with Acme', passwordForm: 'folded' })
})

test('a missing or blank label falls back to the default words', () => {
  assert.equal(signInChoices({ enabled: true, passwordLogin: true }).label, 'Sign in with single sign-on')
  assert.equal(signInChoices({ enabled: true, label: '   ', passwordLogin: true }).label, 'Sign in with single sign-on')
})

test('every code the server can send reads as a sentence that says what to do', () => {
  const codes = ['denied', 'expired', 'mismatch', 'failed', 'unavailable', 'not_ready', 'no_account', 'exists', 'not_linkable', 'email_missing', 'email_unverified', 'disabled', 'already_linked', 'linked_elsewhere', 'signin_required', 'rate_limited', 'busy']
  const seen = new Set()
  for (const code of codes) {
    const m = ssoErrorMessage(code)
    assert.match(m, /[a-z]{3}.*\.$/, code)
    assert.ok(!m.includes('_'), `${code} leaks its code: ${m}`)
    seen.add(m)
  }
  assert.equal(seen.size, codes.length, 'each code has its own sentence')
})

test('an unknown or empty code is not echoed back', () => {
  assert.equal(ssoErrorMessage(''), '')
  const m = ssoErrorMessage('<script>alert(1)</script>')
  assert.ok(!m.includes('<') && !m.includes('script'))
  assert.match(m, /could not finish/i)
})

test('the code is read from the address, and only a plain code', () => {
  assert.equal(ssoErrorFromSearch('?sso_error=expired'), 'expired')
  assert.equal(ssoErrorFromSearch('?a=1&sso_error=no_account&b=2'), 'no_account')
  assert.equal(ssoErrorFromSearch(''), '')
  assert.equal(ssoErrorFromSearch('?sso_error=' + encodeURIComponent('<b>x</b>')), '')
  assert.equal(ssoErrorFromSearch('?sso_error=' + 'a'.repeat(80)), '')
})

test('the code is taken off the address once it has been shown', () => {
  assert.equal(withoutSsoError('/?sso_error=expired'), '/')
  assert.equal(withoutSsoError('/lists?tab=liked&sso_error=expired#x'), '/lists?tab=liked#x')
  assert.equal(withoutSsoError('/lists?tab=liked'), '/lists?tab=liked')
})

test('the button starts a sign-in that resumes where the person was', () => {
  assert.equal(ssoStartHref(''), '/api/v2/auth/oidc/start')
  assert.equal(ssoStartHref('/lists?tab=liked'), '/api/v2/auth/oidc/start?next=%2Flists%3Ftab%3Dliked')
  assert.equal(ssoNext('/', ''), '')
  assert.equal(ssoNext('/signin', ''), '')
  assert.equal(ssoNext('/signup', ''), '')
  assert.equal(ssoNext('/welcome', ''), '')
  assert.equal(ssoNext('/account/mcp/authorize', '?client_id=x&state=a%20b'), '/account/mcp/authorize?client_id=x&state=a%20b')
})

test('linking comes back to the account page', () => {
  assert.equal(ssoLinkHref(), '/api/v2/auth/oidc/start?intent=link&next=%2Faccount%2Fsso')
})

test('a linked account says since when, and a last sign-in only if there was one', () => {
  const fmt = ms => `day-${ms}`
  assert.equal(linkedLine({ linked: true, linkedAt: 5 }, fmt), 'Linked since day-5.')
  assert.equal(linkedLine({ linked: true, linkedAt: 5, lastLoginAt: 9 }, fmt), 'Linked since day-5. Last used to sign in day-9.')
  assert.equal(linkedLine({ linked: false }, fmt), '')
})

test('the admin panel reads the status without ever naming a secret', () => {
  const rows = adminRows({ enabled: true, issuer: 'https://id.example', redirectUrl: 'https://reet.example/cb', label: 'Acme', passwordLogin: false, autoCreate: true, linkByEmail: false, defaultRole: 'viewer', groupsClaim: 'groups', adminGroups: ['ops'], memberGroups: [], viewerGroups: [], linked: 3, reachable: false, problem: 'dial tcp: refused' })
  const text = JSON.stringify(rows)
  assert.ok(text.includes('https://id.example') && text.includes('ops') && text.includes('refused'))
  assert.ok(!/secret/i.test(text))
  const byLabel = Object.fromEntries(rows.map(r => [r.label, r.value]))
  assert.equal(byLabel['Provider'], 'Not reachable: dial tcp: refused')
  assert.equal(byLabel['Password sign-in'], 'Owner only')
  assert.equal(byLabel['New people'], 'Created automatically')
  assert.equal(byLabel['Linking by email'], 'Off')
  assert.equal(byLabel['Linked people'], '3')
  assert.equal(byLabel['Admin groups'], 'ops')
  assert.equal(byLabel['Member groups'], 'None')
})
