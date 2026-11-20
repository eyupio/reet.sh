// The first Skills screen has one obvious thing to do, and hosting says what it is before it asks for a permanent username.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill, skillRow, skillsPageOf } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // An empty library: adding a skill leads. Write a skill is secondary, there is nothing to start syncing (machines follow
 // the library), and the empty list says what to do.
 if (run('empty')) {
  for (const width of [1440,375]) {
   const {context,page,errors} = await open({width})
   await page.goto(`${base}/skills`)
   await expect(page.getByText('Nowt active. Paste a skill link above to get cracking.',{exact:true})).toBeVisible()
   await expect(page.getByLabel('Add a skill',{exact:true})).toBeVisible()
   await expect(page.getByRole('link',{name:'Sync all machines'}), `no Sync all machines at ${width}px`).toHaveCount(0)
   await expect(page.getByRole('button',{name:'Sync all machines'}), `no Sync all machines at ${width}px`).toHaveCount(0)
   // The line about the machines opens Sync, which reads them as this line does.
   await expect(page.locator('main .sync-chip')).toHaveText('No machines yet')
   await expect(page.getByRole('link',{name:'No machines yet'})).toHaveAttribute('href','/sync')
   const write = page.locator('main').getByRole('link',{name:'Write a skill',exact:true})
   await expect(write).toHaveAttribute('href','/hosting')
   await expect(write).not.toHaveClass(/primary/)
   await expect(page.getByRole('link',{name:'Host your own skill'})).toHaveCount(0)
   const filled = await page.locator('main .btn.primary').allInnerTexts()
   expect(filled.map(t=>t.trim()), `filled buttons at ${width}px`).toEqual(['Add'])
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // Once there is something in the library it is listed as Active, the header says where the machines stand, and writing
 // stays secondary.
 if (run('populated')) {
  const {context,page} = await open({width:1440,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.locator('[data-skill="demo/toolkit/code-review"]')).toBeVisible()
  await expect(page.getByRole('tab',{name:/^Active/})).toContainText('1')
  // The line about the machines opens Sync.
  await expect(page.locator('.sync-chip')).toHaveAttribute('href','/sync')
  await expect(page.locator('main').getByRole('link',{name:'Write a skill',exact:true})).not.toHaveClass(/primary/)
  await context.close()
 }

 // The library tools sit under the Skills list, whether it is empty or not: restore and import stay reachable, and the
 // bookmarklets are offered there once. Their words match what they do now: an import makes skills Active, and nothing
 // removes a skill "and ignores it" any more.
 if (run('tools')) {
  for (const skills of [[], [skill('code-review')]]) {
   const {context,page,errors} = await open({width:1440,skills})
   // The ignore list is gone (its routes answer 410), so nothing here asks for it or toasts its refusal.
   const ignoreAsks = []
   page.on('request', r => { if (/\/api\/(ignored|skill\/ignore)/.test(new URL(r.url()).pathname)) ignoreAsks.push(r.url()) })
   await page.goto(`${base}/skills`)
   await expect(page.getByRole('heading',{name:'Library tools'})).toBeVisible()
   await page.getByRole('button',{name:/Show bookmarklets and tools/}).click()
   await expect(page.getByRole('heading',{name:'Import what you have installed'})).toBeVisible()
   await page.waitForLoadState('networkidle')
   await expect(page.locator('.toast.bad')).toHaveCount(0)
   expect(ignoreAsks, 'the Skills page asked for the removed ignore list').toEqual([])
   await expect(page.getByRole('button',{name:'Import',exact:true})).toBeVisible()
   await expect(page.locator('#tools-panel')).not.toContainText(/in use|to test/i)
   // The bookmarklets add a skills.sh page to the library, or like it; there are no stages to put it in.
   await expect(page.locator('#tools-panel .links a')).toHaveText(['♡ Like on reet', '+ Add to reet'])
   await expect(page.locator('#tools-panel').getByRole('link', {name:'+ Add to reet'})).toHaveAttribute('href', /\/quick\?action=save&/)
   await expect(page.getByRole('heading',{name:'Ignored skills'})).toHaveCount(0)
   await expect(page.getByRole('button',{name:'Allow again'})).toHaveCount(0)
   await expect(page.getByRole('button',{name:'Restore from JSON'})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Keep a good find'})).toHaveCount(1)
   await expect(page.getByRole('button',{name:'Manage tags'})).toBeVisible()
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // An import makes new skills Active and leaves the ones already in the library as they are, and says which is which.
 if (run('import')) {
  let sent = null
  const {context,page,errors} = await open({width:1440,skills:[skill('code-review')],routes:{'/api/import': req => { sent = req.postDataJSON(); return {imported:3, created:2, ignored:0, pruned:0, errors:[]} }}})
  await page.goto(`${base}/skills`)
  await page.getByRole('button',{name:/Show bookmarklets and tools/}).click()
  await expect(page.locator('#tools-panel')).toContainText('New skills become Active and go on every machine; ones already in your library keep their state.')
  await page.getByLabel('Skills to import').fill('code-review\nanthropics/skills/pdf\nfind-skills')
  await page.getByRole('button',{name:'Import',exact:true}).click()
  await expect(page.getByRole('status').filter({hasText:'Imported 2 new as Active; 1 already in your library kept its state'})).toBeVisible()
  expect(sent).toEqual({text:'code-review\nanthropics/skills/pdf\nfind-skills'})
  expect(errors).toEqual([])
  await context.close()
 }

 // Markdown and CSV follow what the Skills page shows: its tab, search, Liked and tag.
 if (run('export')) {
  const rows = [skillRow('seo-sxo',{status:'archived',liked:true,tags:[{id:4,name:'seo'}]}), skillRow('code-review')]
  let asked = null
  const {context,page,errors} = await open({width:1440,routes:{
   '/api/v2/skills': req => skillsPageOf(rows, new URL(req.url()).searchParams.get('status') || 'active'),
   '/api/export': req => { asked = Object.fromEntries(new URL(req.url()).searchParams); return {exported:true} },
  }})
  await page.goto(`${base}/skills`)
  await page.getByRole('tab',{name:/^Archived/}).click()
  await page.getByRole('searchbox',{name:'Search skills'}).fill('seo')
  await page.getByRole('button',{name:'Liked'}).click()
  await page.getByRole('combobox',{name:'Tag'}).selectOption('seo')
  await expect(page.locator('[data-skill="item-seo-sxo"]')).toBeVisible()
  await page.getByRole('button',{name:/Show bookmarklets and tools/}).click()
  await page.getByLabel('Format').selectOption('csv')
  await page.getByRole('button',{name:'Download'}).click()
  await expect.poll(() => asked).toEqual({format:'csv', tab:'liked', q:'seo', sort:'name', stage:'archived', tag:'4'})
  expect(errors).toEqual([])
  await context.close()
 }

 // While the list loads it shows placeholder rows rather than a bare "Loading…" line, and they give way to the skills.
 if (run('loading')) {
  let release
  const gate = new Promise(resolve=>{release=resolve})
  const {context,page,errors} = await open({width:1440,skills:[skill('code-review')],gates:{'/api/v2/skills':gate}})
  await page.goto(`${base}/skills`)
  const loading = page.getByRole('status',{name:'Loading your skills'})
  await expect(loading).toBeVisible()
  await expect(loading).toHaveAttribute('aria-busy','true')
  await expect(loading.locator('[data-skeleton]')).toHaveCount(4)
  await expect(page.getByText('Loading…',{exact:true})).toHaveCount(0)
  release()
  await expect(page.getByRole('button',{name:'code-review',exact:true})).toBeVisible()
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
 // A skill you wrote is in the library without publishing: the Skills page lists it as yours, Active like any other, and
 // the Sync page lists it beside shelf skills, named as a private draft.
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
  const library = [skillRow('code-review',{id:'item-code-review',shelfId:shelfSkill.id}), skillRow('reet-incident-notes',{shelfId:'',originKind:'written',origin:'',yours:true,description:'A private draft.'})]
  const routes = {
   '/api/v2/sync/candidates': req => { const collection=new URL(req.url()).searchParams.get('collection');return collection==='library'?{skills:[shelfSkill,draft,nowDraft,broken]}:collection==='hosted'?{skills:[draft,nowDraft,broken]}:{skills:[shelfSkill,stale]} },
   '/api/v2/skills': req => skillsPageOf(library, new URL(req.url()).searchParams.get('status') || 'active'),
  }
  const {context,page,errors} = await open({width:1440,skills:[shelfSkill],hosting,sync,routes})
  await page.goto(`${base}/skills`)
  const written = page.locator('[data-skill="item-reet-incident-notes"]')
  await expect(written).toContainText('written by you')
  await expect(written.locator('.health')).toContainText('On all 3 machines')
  await expect(page.locator('[data-skill="item-code-review"]')).toBeVisible()
  await page.goto(`${base}/sync/advanced?skill=${encodeURIComponent(draftId)}`)
  await expect(page.getByRole('heading',{name:'Your skills, everywhere.'})).toBeVisible()
  await page.locator('.library-content > summary').filter({hasText:'In your library'}).click()
  const choice = page.locator('.library-content li', {hasText:'reet-incident-notes'})
  await expect(choice).toContainText('Private · only your machines')
  await expect(page.locator('.library-content li', {hasText:'code-review'})).toBeVisible()
  const unpublished = page.locator('.library-content li', {hasText:'reet-standup'})
  await expect(unpublished).toHaveCount(1)
  await expect(unpublished).toContainText('Private · only your machines')
  // The page's one status sentence counts what is left out; the reason is in the note under it.
  await expect(page.getByRole('region',{name:'Your skills, everywhere.'}).getByRole('status')).toContainText('1 skill is left out for now.')
  await expect(page.locator('.left-out')).toContainText('private skill storage is full')
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
  await expect(page.locator('main').getByText(`npx skills@1.7.4 add ${base}/your-username/skill-name`)).toBeVisible()
  const [a,b] = [await gives.boundingBox(), await claim.boundingBox()]
  expect(a.y, 'the explanation sits above the username form').toBeLessThan(b.y)
  await expect(page.getByText(/This username is permanent/)).toBeVisible()
  await context.close()
 }
 console.log('First skills: add leads on an empty library, the tools sit under the list, a skeleton while it loads, a written skill is a row like any other, hosting explains itself before the username.')
} finally {await browser.close()}
