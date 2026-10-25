// Single sign-on in the interface: the button and the password form on the sign-in page, the plain sentence for each
// way a sign-in can fail, linking and unlinking on the account page, and the admin panel. The API is answered from
// fixtures, so nothing here needs an identity provider.
import { chromium, expect } from '@playwright/test'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)
const sso = (extra={}) => ({enabled:true,label:'Sign in with Acme',passwordLogin:true,...extra})
const fitsWidth = page => page.evaluate(()=>document.scrollingElement.scrollWidth<=innerWidth+1)

try {
 // With single sign-on off the sign-in page is the page it always was.
 if (run('off')) {
  const {context,page,errors} = await open({width:1440,status:{user:null}})
  await page.goto(`${base}/signin`)
  await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible()
  await expect(page.getByRole('link',{name:/single sign-on|Sign in with/i})).toHaveCount(0)
  await expect(page.locator('#signin-password')).toBeVisible()
  await expect(page.getByText('Sign in with a password',{exact:true})).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // With it on, a button leads to the provider and the password form stays while passwords are allowed.
 for (const [width,scheme] of run('button') ? [[1440,'light'],[390,'light'],[320,'dark'],[390,'dark']] : []) {
  const {context,page,errors} = await open({width,status:{user:null,sso:sso()}})
  await page.emulateMedia({colorScheme:scheme})
  await page.goto(`${base}/signin`)
  const button = page.getByRole('link',{name:'Sign in with Acme'})
  await expect(button).toBeVisible()
  await expect(button).toHaveAttribute('href','/api/v2/auth/oidc/start')
  await expect(page.locator('#signin-password')).toBeVisible()
  expect(await fitsWidth(page), `sign-in page fits ${width}px (${scheme})`).toBe(true)
  const box = await button.boundingBox()
  expect(box.x).toBeGreaterThanOrEqual(0)
  expect(box.x + box.width).toBeLessThanOrEqual(width)
  expect(box.height, 'the button is a touch-sized target').toBeGreaterThanOrEqual(40)
  expect(errors).toEqual([])
  await context.close()
 }

 // Where a deep link brought someone to sign in, the button resumes there.
 if (run('resume')) {
  const {context,page} = await open({width:1440,status:{user:null,sso:sso()}})
  await page.goto(`${base}/lists?tab=liked`)
  await expect(page.getByRole('link',{name:'Sign in with Acme'})).toHaveAttribute('href','/api/v2/auth/oidc/start?next=%2Flists%3Ftab%3Dliked')
  await context.close()
 }

 // Where passwords are refused for everyone but the owner the form is folded away, and still reachable.
 if (run('folded')) {
  const {context,page,errors} = await open({width:390,status:{user:null,sso:sso({passwordLogin:false})}})
  await page.goto(`${base}/signin`)
  await expect(page.getByRole('link',{name:'Sign in with Acme'})).toBeVisible()
  await expect(page.locator('#signin-password')).toBeHidden()
  await page.getByText('Sign in with a password',{exact:true}).click()
  await expect(page.locator('#signin-password')).toBeVisible()
  await expect(page.getByText('Only the owner can sign in with a password here.')).toBeVisible()
  expect(await fitsWidth(page)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }

 // A sign-in that could not finish says why in words, once, and does not leave the code in the address.
 if (run('error')) {
  const {context,page} = await open({width:390,status:{user:null,sso:sso()}})
  await page.goto(`${base}/?sso_error=no_account`)
  await expect(page.getByRole('alert').filter({hasText:'There is no account for you here'})).toBeVisible()
  expect(new URL(page.url()).search).toBe('')
  await page.reload()
  await expect(page.getByRole('alert').filter({hasText:'There is no account for you here'})).toHaveCount(0)
  await context.close()
  // A code the interface does not know gets one general sentence, and anything that is not a plain code gets nothing:
  // the address is never printed on the page.
  const unknown = await open({width:390,status:{user:null,sso:sso()}})
  await unknown.page.goto(`${base}/?sso_error=a_new_code`)
  await expect(unknown.page.getByRole('alert').filter({hasText:'could not finish'})).toBeVisible()
  await expect(unknown.page.getByText('a_new_code')).toHaveCount(0)
  await unknown.context.close()
  const hostile = await open({width:390,status:{user:null,sso:sso()}})
  await hostile.page.goto(`${base}/signin?sso_error=${encodeURIComponent('<b>boom</b>')}`)
  await expect(hostile.page.getByRole('alert')).toHaveCount(0)
  await expect(hostile.page.getByText('boom')).toHaveCount(0)
  await hostile.context.close()
 }

 // A signed-in person whose link failed is told on the page they land on.
 if (run('signedin-error')) {
  const {context,page} = await open({width:1440,status:{sso:sso()}})
  await page.goto(`${base}/?sso_error=linked_elsewhere`)
  await expect(page.getByText('That identity is already linked to another account.')).toBeVisible()
  expect(new URL(page.url()).search).toBe('')
  await context.close()
 }

 // The account page links and unlinks an identity, and says why an unlink was refused.
 if (run('account')) {
  let identity = {enabled:true,label:'Sign in with Acme',linked:false,hasPassword:true}
  const deletes = []
  const routes = {'/api/v2/auth/oidc/identity':req=>{
   if(req.method()==='DELETE'){deletes.push(1);return identity.refuse?{status:409,json:{error:'this would leave no way to sign in: set a password first, or ask an admin'}}:(identity={...identity,linked:false},{json:{ok:true}})}
   return {json:identity}
  }}
  const {context,page,errors} = await open({width:1440,status:{sso:sso()},routes})
  await page.goto(`${base}/account/sso`)
  await expect(page.getByRole('heading',{name:'Single sign-on',exact:true})).toBeVisible()
  await expect(page.getByRole('link',{name:'Link your identity'})).toHaveAttribute('href','/api/v2/auth/oidc/start?intent=link&next=%2Faccount%2Fsso')
  await expect(page.getByRole('button',{name:'Unlink'})).toHaveCount(0)
  identity = {...identity,linked:true,linkedAt:Date.UTC(2026,9,25),canUnlink:true}
  await page.reload()
  await expect(page.getByText(/Linked since/)).toBeVisible()
  await expect(page.getByRole('link',{name:'Link your identity'})).toHaveCount(0)
  await page.getByRole('button',{name:'Unlink'}).click()
  await page.getByRole('button',{name:'Unlink',exact:true}).last().click()
  await expect(page.getByRole('link',{name:'Link your identity'})).toBeVisible()
  expect(deletes.length).toBe(1)
  identity = {...identity,linked:true,canUnlink:false,refuse:true,hasPassword:false}
  await page.reload()
  await expect(page.getByText('Set a password before unlinking')).toBeVisible()
  await expect(page.getByRole('button',{name:'Unlink'})).toBeDisabled()
  expect(errors).toEqual([])
  await context.close()
 }

 // With single sign-on off the account page has no section for it.
 if (run('account-off')) {
  const {context,page} = await open({width:1440,routes:{'/api/v2/auth/oidc/identity':{enabled:false}}})
  await page.goto(`${base}/account/profile`)
  await expect(page.getByRole('link',{name:'Single sign-on'})).toHaveCount(0)
  await context.close()
 }

 // The admin page shows how it is set up, read-only, and never a secret.
 if (run('admin')) {
  const status = {enabled:true,issuer:'https://id.example.test',redirectUrl:'https://reet.example.test/api/v2/auth/oidc/callback',label:'Sign in with Acme',passwordLogin:false,autoCreate:true,linkByEmail:false,defaultRole:'viewer',groupsClaim:'groups',adminGroups:['ops'],memberGroups:[],viewerGroups:[],linked:3,reachable:false,problem:'dial tcp: connection refused'}
  const adminStatus = {version:'test',commit:'x',db:{path:'/data',size:1},counts:{all:0,used:0,liked:0,bookmarked:0},stages:{use:0,try:0,shelf:0},environments:[],events:[],skillsSh:true,metaLastOK:'',metaLastError:'',topics:{lastSynced:'',lastError:''},settings:{refreshEnabled:true,topicsEnabled:true,linkListsEnabled:false,rollupIndividual:true,pruneServerEnvironmentsDays:0,instanceName:''}}
  const routes = {'/api/admin/status':adminStatus,'/api/v2/admin/users':{users:[]},'/api/v2/admin/sso':status}
  const {context,page,errors} = await open({width:1440,routes})
  await page.goto(`${base}/admin/users`)
  const panel = page.locator('section[aria-labelledby="sso-title"]')
  await expect(panel.getByRole('heading',{name:'Single sign-on'})).toBeVisible()
  await expect(panel.getByText('Not reachable: dial tcp: connection refused')).toBeVisible()
  await expect(panel.getByText('https://id.example.test')).toBeVisible()
  await expect(panel.getByText('Owner only')).toBeVisible()
  await expect(panel.getByText(/secret/i)).toHaveCount(0)
  await expect(panel.locator('input, textarea, select, button')).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }
 console.log('Single sign-on: button, folded password form, plain errors, link and unlink, admin panel passed.')
} finally {await browser.close()}
