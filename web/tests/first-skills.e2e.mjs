// The first Skills screen has one obvious thing to do, and hosting says what it is before it asks for a permanent username.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Empty shelf: adding a skill leads. Nothing to sync yet, creating is secondary, and the same link is not offered twice.
 if (run('empty')) {
  for (const width of [1440,375]) {
   const {context,page,errors} = await open({width})
   await page.goto(`${base}/skills`)
   await expect(page.getByRole('heading',{name:'Put the first skill on your shelf.'})).toBeVisible()
   await expect(page.getByRole('link',{name:'Sync all machines'}), `Library sync at ${width}px`).toHaveAttribute('href','/sync')
   const create = page.getByRole('link',{name:'Create a skill'})
   await expect(create).toBeVisible()
   await expect(create).not.toHaveClass(/primary/)
   await expect(page.getByRole('link',{name:'Host your own skill'})).toHaveCount(0)
   const filled = await page.locator('main .btn.primary').allInnerTexts()
   expect(filled.map(t=>t.trim()), `filled buttons at ${width}px`).toEqual(['Add to shelf','Add your first skill'])
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // Once there is something on the shelf, syncing is offered, and creating stays secondary.
 if (run('populated')) {
  const {context,page} = await open({width:1440,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('link',{name:'Sync all machines'})).toHaveAttribute('href','/sync')
  await expect(page.getByRole('link',{name:'Create a skill'})).not.toHaveClass(/primary/)
  await context.close()
 }

 // On an empty shelf the tools are one quiet button, not a section with its own heading, and restore and import stay reachable.
 // With skills on the shelf the section is unchanged.
 if (run('tools')) {
  const empty = await open({width:1440})
  await empty.page.goto(`${base}/skills`)
  await expect(empty.page.getByRole('heading',{name:'Put the first skill on your shelf.'})).toBeVisible()
  await expect(empty.page.getByRole('heading',{name:'Shelf tools'})).toHaveCount(0)
  await empty.page.getByRole('button',{name:'Import or restore a backup'}).click()
  await expect(empty.page.getByRole('heading',{name:'Import what you have installed'})).toBeVisible()
  await expect(empty.page.getByRole('button',{name:'Restore from JSON'})).toBeVisible()
  await expect(empty.page.getByRole('heading',{name:'Keep a good find'}), 'the start card already offers the bookmarklets').toHaveCount(0)
  await empty.context.close()

  const full = await open({width:1440,skills:[skill('code-review')]})
  await full.page.goto(`${base}/skills`)
  await expect(full.page.getByRole('heading',{name:'Shelf tools'})).toBeVisible()
  await expect(full.page.getByRole('button',{name:'Import or restore a backup'})).toHaveCount(0)
  await full.page.getByRole('button',{name:/Show bookmarklets and tools/}).click()
  await expect(full.page.getByRole('button',{name:'Restore from JSON'})).toBeVisible()
  await expect(full.page.getByRole('heading',{name:'Keep a good find'})).toBeVisible()
  await full.context.close()
 }

 // While the shelf loads it shows placeholder cards rather than a bare "Loading…" line, and they give way to the skills.
 if (run('loading')) {
  let release
  const gate = new Promise(resolve=>{release=resolve})
  const {context,page,errors} = await open({width:1440,skills:[skill('code-review')],gates:{'/api/skills':gate}})
  await page.goto(`${base}/skills`)
  const loading = page.getByRole('status',{name:'Loading your shelf'})
  await expect(loading).toBeVisible()
  await expect(loading).toHaveAttribute('aria-busy','true')
  await expect(loading.locator('[data-skeleton]')).toHaveCount(3)
  await expect(page.getByText('Loading…',{exact:true})).toHaveCount(0)
  release()
  await expect(page.getByRole('heading',{name:'code-review'})).toBeVisible()
  await expect(loading).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }

 // Hosting: what it gives you comes first, then the username form.
 // A published skill's editor ends with both ways to hand it on: the install command and the prompt the shelf card offers.
 if (run('hosting-share')) {
  const now = Date.now()
  const hosting = {username:'sam',prefix:'',skills:[{slug:'ui-refresh',description:'Published.',content:'---\nname: ui-refresh\ndescription: x\n---\nbody',files:{},published:true,createdAt:now,updatedAt:now}]}
  const {context,page,errors} = await open({width:1440,hosting})
  await page.goto(`${base}/hosting?edit=ui-refresh`)
  await expect(page.getByRole('heading',{name:'Share and install'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Copy install command'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Copy prompt for ui-refresh'})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }
 // A SKILL.md whose name differs from the skill's is explained beside the editor, fixed in one press, and never silently refused.
 if (run('hosting-name')) {
  const now = Date.now()
  const hosting = {username:'sam',prefix:'',skills:[{slug:'ui-refresh',description:'Draft.',content:'---\nname: ui-refresh\ndescription: x\n---\nbody',files:{},published:false,createdAt:now,updatedAt:now}]}
  const {context,page,errors} = await open({width:1440,hosting,routes:{'/api/v2/hosting/skills/ui-refresh':{status:400,json:{error:'the SKILL.md name must match the skill name in the URL'}}}})
  await page.goto(`${base}/hosting?edit=ui-refresh`)
  const content = page.locator('#hosted-content')
  await content.fill('---\nname: other-name\ndescription: x\n---\nbody')
  const warn = page.locator('.notice.warn',{hasText:'SKILL.md says'})
  await expect(warn).toBeVisible()
  await expect(warn).toContainText('other-name')
  await expect(warn).toContainText('ui-refresh')
  await page.locator('form .actions').getByRole('button',{name:'Save draft'}).click()
  const alert = page.getByRole('alert')
  await expect(alert).toContainText('Make them match to save')
  await page.getByRole('button',{name:'Match name'}).click()
  await expect(content).toHaveValue(/name: ui-refresh/)
  await expect(warn).toHaveCount(0)
  await expect(alert).toHaveCount(0)
  expect(errors).toEqual([])
  await context.close()
 }
 // A draft you wrote can be chosen on the Sync page without publishing: it is listed beside shelf skills, named as a
 // private draft, and the Skills page's created-skill row links straight to it pre-selected.
 if (run('sync-draft')) {
  const now = Date.now()
  const host = new URL(base).host
  const draftId = `hosted/${host}/sam/reet-incident-notes`
  const shelfSkill = skill('code-review')
  const draft = {id:draftId,name:'reet-incident-notes',slug:'reet-incident-notes',source:'private:'+'a'.repeat(32)+':'+'b'.repeat(32)+':'+'c'.repeat(64),sourceType:'private'}
  // A skill published then unpublished: the shelf still carries its public entry, the hosted list carries the draft that now serves it.
  const stale = {...skill('reet-standup'),id:`hosted/${host}/sam/reet-standup`,source:`${host}/sam`,sourceType:'hosted'}
  const nowDraft = {id:stale.id,name:'reet-standup',slug:'reet-standup',source:'private:'+'d'.repeat(32)+':'+'e'.repeat(32)+':'+'f'.repeat(64),sourceType:'private'}
  const broken = {id:'hosted/127.0.0.1:4173/sam/reet-broken',name:'reet-broken',slug:'reet-broken',source:'',sourceType:'local',problem:'reet-broken cannot be synced privately: private skill storage is full'}
  const scope = {id:'dev-laptop-agents',host:'dev-laptop',agent:'agents',name:'dev-laptop',skills:[],skillCount:0,complete:true,scannedAt:now,lastSeen:now,os:'linux',arch:'amd64',version:'test'}
  const sync = {environments:[scope],machines:[{host:'dev-laptop',groups:[scope],skillCount:0}],agents:[{id:'a1',host:'dev-laptop',version:'1.2.3',lastSeen:now,paused:false,autoUpdate:false}],targets:[{target:scope,sourceId:'shelf',sourceName:'Shelf',mode:'additive',desired:[draft],missing:[draft],extra:[],revision:'r1',updatedAt:now}],jobs:[],runs:[],updateJobs:[]}
  const hosting = {username:'sam',prefix:'reet-',skills:[{slug:'reet-incident-notes',description:'A private draft.',content:'---\nname: reet-incident-notes\ndescription: x\n---\nbody',files:{},published:false,createdAt:now,updatedAt:now}]}
  const routes = {'/api/v2/sync/candidates': req => { const collection=new URL(req.url()).searchParams.get('collection');return collection==='library'?{skills:[shelfSkill,draft,nowDraft,broken]}:collection==='hosted'?{skills:[draft,nowDraft,broken]}:{skills:[shelfSkill,stale]} }}
  const {context,page,errors} = await open({width:1440,skills:[shelfSkill],hosting,sync,routes})
  await page.goto(`${base}/skills`)
  await expect(page.locator('.hosted-list .where')).toContainText('Pending on dev-laptop')
  const row = page.locator('.hosted-list').getByRole('link',{name:'Sync to machines…'})
  await expect(row).toHaveAttribute('href',`/sync?skill=${encodeURIComponent(draftId)}`)
  await page.goto(`${base}/sync?skill=${encodeURIComponent(draftId)}`)
  await expect(page.getByRole('heading',{name:'Your skills, everywhere.'})).toBeVisible()
  await page.locator('.library-content > summary').filter({hasText:'In your library'}).click()
  const choice = page.locator('.library-content li', {hasText:'reet-incident-notes'})
  await expect(choice).toContainText('Private · only your machines')
  await expect(page.locator('.library-content li', {hasText:'code-review'})).toBeVisible()
  const unpublished = page.locator('.library-content li', {hasText:'reet-standup'})
  await expect(unpublished).toHaveCount(1)
  await expect(unpublished).toContainText('Private · only your machines')
  await expect(page.getByRole('status').filter({hasText:'private skill storage is full'})).toBeVisible()
  await expect(page.locator('.library-content li', {hasText:'reet-broken'})).toContainText('Left out for now')
  expect(errors).toEqual([])
  await context.close()
 }
 if (run('hosting')) {
  const {context,page} = await open({width:1440})
  await page.goto(`${base}/hosting`)
  const gives = page.getByRole('heading',{name:'What publishing gives you'})
  const claim = page.getByRole('heading',{name:'Choose your publishing username'})
  await expect(gives).toBeVisible()
  await expect(claim).toBeVisible()
  await expect(page.locator('main').getByText(`npx skills add ${base}/your-username/skill-name`)).toBeVisible()
  const [a,b] = [await gives.boundingBox(), await claim.boundingBox()]
  expect(a.y, 'the explanation sits above the username form').toBeLessThan(b.y)
  await expect(page.getByText(/This username is permanent/)).toBeVisible()
  await context.close()
 }
 console.log('First skills: add leads on an empty shelf, quiet tools on an empty shelf, a skeleton while it loads, sync appears with content, hosting explains itself before the username.')
} finally {await browser.close()}
