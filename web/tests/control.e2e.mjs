// UI contract checks use synthetic inventories; store tests verify real queue/approval semantics.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium',args:['--no-sandbox']})
const context = await browser.newContext({viewport:{width:1440,height:1080}})
const page = await context.newPage()
const errors=[], writes=[]
page.on('pageerror',e=>errors.push(e.message))
const now=Date.now()
const skill=id=>({id:`demo/toolkit/${id}`,name:id,slug:id,source:'demo/toolkit',sourceType:'github',description:'Example skill for UI verification.',notes:'',installs:0,installsWeek:0,trend:[],liked:false,bookmarked:false,used:true,createdAt:now,updatedAt:now})
const skills=[skill('code-review'),skill('testing'),skill('deployment')]
const scope=(id,host,agent,items)=>({id,host,agent,name:host,skills:items,skillCount:items.length,complete:true,scannedAt:now,lastSeen:now,os:'linux',arch:'amd64',version:'test'})
const environments=[scope('laptop-shared','dev-laptop','agents',skills.slice(0,2)),scope('laptop-claude','dev-laptop','claude',skills.slice(0,1)),scope('build-shared','build-server','agents',skills)]
let snapshot={environments,machines:['dev-laptop','build-server'].map(host=>({host,groups:environments.filter(e=>e.host===host),skillCount:host==='dev-laptop'?2:3})),agents:[{id:'a',host:'dev-laptop',version:'test',lastSeen:now,paused:false},{id:'b',host:'build-server',version:'test',lastSeen:now,paused:false}],targets:environments.map(target=>({target,sourceId:'shelf',sourceName:'Central library',mode:'additive',desired:skills,missing:skills.filter(s=>!target.skills.some(a=>a.id===s.id)),extra:[],revision:`rev-${target.id}`,updatedAt:now})),jobs:[]}
let viewer=false, failQueue=false, failSave=false, failState=false, failSource=false
await page.route('**/api/**',async route=>{
 const request=route.request(), path=new URL(request.url()).pathname
 const body=request.method()==='GET'?null:request.postDataJSON()
 let data={}
 if(path==='/api/v2/auth/status')data={mode:'accounts',user:{id:1,name:'Demo operator',email:'demo@example.test',role:viewer?'viewer':'owner'}}
 else if(path==='/api/v2/profile')data={profile:{theme:'light',shelfView:'list',cardsExpanded:false}}
 else if(path==='/api/skills')data={skills,counts:{all:3,used:3,liked:0,bookmarked:0}}
 else if(path==='/api/environments')data={environments,machines:snapshot.machines}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/sync'){
  if(failState)return route.fulfill({status:503,json:{error:'Inventory unavailable'}})
  data=snapshot
 }
 else if(path==='/api/v2/sync/candidates'){if(failSource)return route.fulfill({status:503,json:{error:'Source unavailable'}});data={skills}}
 else if(path==='/api/v2/sync/desired'){
  writes.push({path,body})
  if(failSave&&body.targetId==='laptop-claude')return route.fulfill({status:409,json:{error:'Desired set changed'}})
  data=snapshot
 }
 else if(path==='/api/v2/sync/adopt'){writes.push({path,body});data={ok:true}}
 else if(path==='/api/v2/sync/jobs'){
  writes.push({path,body})
  if(failQueue&&body.agentId==='b')return route.fulfill({status:409,json:{error:'Machine busy'}})
  data=snapshot
 }
 else if(path==='/api/v2/machines/health')data={serverNow:now,machines:[]}
 else if(path==='/api/v2/setup/profiles')data={profiles:[]}
 else if(path==='/api/v2/terminal/machines')data={machines:[]}
 else if(path==='/api/v2/terminal/sessions')data={sessions:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'test',commit:'fixture'}
 else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 await route.fulfill({json:data})
})
const noOverflow=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
try {
 await page.goto(base)
 await expect(page.getByRole('heading',{name:'Your machines. One control centre.'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Sync all · preview'})).toBeEnabled()
 await noOverflow()
 if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/control-desktop.png`,fullPage:true})}
 await page.getByRole('button',{name:'Sync all · preview'}).click()
 await expect(page.getByRole('dialog')).toContainText('3 jobs will be queued')
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 expect(writes).toHaveLength(0)
 failQueue=true
 await page.getByRole('button',{name:'Sync all · preview'}).click()
 await page.getByRole('button',{name:'Queue jobs',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'2 of 3 jobs queued'})).toBeVisible()
 await expect(page.getByRole('alert')).toContainText('Machine busy')
 expect(writes.filter(w=>w.body.kind==='preview')).toHaveLength(3)
 expect(writes.some(w=>w.body.action==='approve')).toBe(false)
 failQueue=false
 await page.goto(`${base}/sync?machine=*`)
 await expect(page.getByLabel('Destination',{exact:true})).toHaveValue('*')
 await expect(page.locator('.scope-option')).toHaveCount(3)
 await page.getByLabel('Find skills',{exact:true}).fill('code-review')
 await page.getByRole('button',{name:'Select visible',exact:true}).click()
 failSave=true
 await page.getByRole('button',{name:'Save to 3 agent scopes',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('1 of 3 scopes saved')
 expect(writes.filter(w=>w.path.endsWith('/desired'))).toHaveLength(2)
 failSave=false
 await page.getByRole('button',{name:'Save to 3 agent scopes',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'Desired skills saved for 3 scopes across 2 machines'})).toBeVisible()
 expect(writes.filter(w=>w.path.endsWith('/desired')).slice(-3).map(w=>[w.body.targetId,w.body.skillIds])).toEqual(environments.map(e=>[e.id,['demo/toolkit/code-review']]))
 await page.getByLabel('Sync mode',{exact:true}).selectOption('bidirectional')
 const beforeCancel=writes.length
 await page.getByRole('button',{name:'Save to 3 agent scopes',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('removes extra skills')
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 expect(writes.length).toBe(beforeCancel)
 await page.getByLabel('Sync mode',{exact:true}).selectOption('additive')
 await page.getByText('Differences by destination',{exact:true}).click()
 await expect(page.getByText('The counts above combine unique skills', {exact:false})).toBeVisible()
 const incoming=page.locator('details').filter({has:page.locator('summary').filter({hasText:'Only on selected scopes (2)'})})
 await incoming.locator('summary').click()
 for(const checkbox of await incoming.getByRole('checkbox').all())await checkbox.check()
 await incoming.getByRole('button',{name:'Add selected to reet.sh',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'2 selected skills added to reet.sh'})).toBeVisible()
 expect(writes.filter(w=>w.path.endsWith('/adopt')).map(w=>[w.body.targetId,w.body.skillIds])).toEqual([['laptop-shared',['demo/toolkit/testing']],['build-shared',['demo/toolkit/deployment']]])
 await page.getByLabel('Compare from',{exact:true}).selectOption('build-shared')
 await page.getByRole('button',{name:'Select visible',exact:true}).click()
 await page.getByRole('button',{name:'Save to 3 agent scopes',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'Desired skills saved for 3 scopes across 2 machines'})).toBeVisible()
 expect(writes.filter(w=>w.path.endsWith('/desired')).slice(-3).every(w=>w.body.sourceId==='build-shared'&&w.body.skillIds.length===3)).toBe(true)
 await noOverflow()
 await page.goto(`${base}/skills`)
 await expect(page.getByRole('heading',{name:'Your central skill library.'})).toBeVisible()
 await expect(page.getByRole('link',{name:'Create a skill',exact:true})).toBeVisible()
 await page.goto(`${base}/terminals`)
 await expect(page.getByRole('heading',{name:'Your machine. Your shell.'})).toBeVisible()
 await page.goto(`${base}/machines?view=compare`)
 await expect(page.getByLabel('Destination',{exact:true})).toBeVisible()
 for(const width of [390,768,1440]){
  await page.setViewportSize({width,height:900})
  await page.goto(base)
  await expect(page.getByRole('button',{name:'Scan all',exact:true})).toBeVisible()
  await page.evaluate(()=>document.documentElement.dataset.theme='dark')
  await noOverflow()
  if(width===390&&process.env.REET_UI_SCREENSHOTS)await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/control-phone-dark.png`,fullPage:true})
  await page.goto(`${base}/sync?machine=*`)
  await expect(page.locator('.scope-option')).toHaveCount(3)
  await noOverflow()
 }
 failSource=true
 await page.goto(`${base}/sync?machine=*`)
 await expect(page.getByRole('alert')).toContainText('Source unavailable')
 await expect(page.getByRole('button',{name:'Save to 3 agent scopes',exact:true})).toBeDisabled()
 failSource=false
 viewer=true
 await page.goto(base)
 await expect(page.getByRole('heading',{name:'Your machines. One control centre.'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Scan all',exact:true})).toHaveCount(0)
 await expect(page.getByRole('link',{name:'Terminals',exact:true})).toHaveCount(0)
 viewer=false;failState=true
 await page.goto(base)
 await expect(page.getByRole('alert')).toContainText('Inventory unavailable')
 await expect(page.getByLabel('Fleet overview')).toHaveCount(0)
 expect(errors).toEqual([])
 console.log('Control UI passed: navigation, all-machine selection, partial save/queue failures, no automatic apply, light/dark responsive layouts, viewer controls and unavailable inventory.')
} finally { await browser.close() }
