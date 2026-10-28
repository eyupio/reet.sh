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
environments[0].content={state:'observed',schemaVersion:1,receivedAt:now,skills:[{name:'code-review',complete:true,digest:'a'.repeat(64),files:2,bytes:123}]}
environments[1].content={state:'stale',schemaVersion:1,receivedAt:now-600000,skills:[{name:'code-review',complete:false,files:0,bytes:0,issues:['symlink']}]}
let snapshot={environments,machines:['dev-laptop','build-server'].map(host=>({host,groups:environments.filter(e=>e.host===host),skillCount:host==='dev-laptop'?2:3})),agents:[{id:'a',host:'dev-laptop',version:'test',lastSeen:now,paused:false,autoUpdate:true},{id:'b',host:'build-server',version:'test',lastSeen:now,paused:false,autoUpdate:false}],releases:{latest:{version:'v1.4.2'}},updateJobs:[{id:'u1',agentId:'a',targetId:'',kind:'update',status:'succeeded',revision:'latest',hash:'',plan:{},message:'Updated to test. The agent is reporting again.',createdAt:now-120000,updatedAt:now-60000,stages:[{key:'queued',label:'Queued for the machine',status:'done'},{key:'install',label:'Installing the new binary',status:'done'}]},{id:'u2',agentId:'b',targetId:'',kind:'update',status:'failed',revision:'latest',hash:'',plan:{},message:'The checksum did not match.',createdAt:now-120000,updatedAt:now-60000,stages:[{key:'queued',label:'Queued for the machine',status:'done'},{key:'verify',label:'Verifying the checksum and build',status:'failed',detail:'Checksum mismatch'}]}],targets:environments.map(target=>({target,sourceId:'shelf',sourceName:'Central library',mode:'additive',desired:skills,missing:skills.filter(s=>!target.skills.some(a=>a.id===s.id)),extra:[],revision:`rev-${target.id}`,updatedAt:now})),jobs:[]}
let terminalReady=true
let viewer=false, failQueue=false, failRetry=false, failSave=false, failState=false, failSource=false
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
 else if(path==='/api/v2/sync/agents'){writes.push({path,body});const agent=snapshot.agents.find(item=>item.id===body.id);if(agent&&body.action.startsWith('auto-update-'))agent.autoUpdate=body.action==='auto-update-on';if(agent&&(body.action==='pause'||body.action==='resume'))agent.paused=body.action==='pause';data=snapshot}
 else if(path==='/api/v2/sync/agents/update'){writes.push({path,body});data=snapshot}
 else if(path==='/api/v2/sync/jobs'){
  writes.push({path,body})
  if(failQueue&&body.agentId==='b')return route.fulfill({status:409,json:{error:'Machine busy'}})
  if(failRetry&&body.agentId==='b')return route.fulfill({status:409,json:{error:'Machine busy'}})
  data=snapshot
 }
 else if(path==='/api/v2/machines/health')data={serverNow:now,machines:[]}
 else if(path==='/api/v2/setup/profiles')data={profiles:[]}
 else if(path==='/api/v2/terminal/machines'){if(failState)return route.fulfill({status:503,json:{error:'Inventory unavailable'}});data={machines:snapshot.agents.map(item=>({agentId:item.id,host:item.host,ready:terminalReady,serverEnabled:true,capabilities:{terminalControl:true,osUser:'developer',terminalEnabled:true,readiness:'ready'}}))}}
 else if(path==='/api/v2/terminal/sessions')data={sessions:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'test',commit:'fixture'}
 else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 await route.fulfill({json:data})
})
const noOverflow=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
try {
 // Control is folded into Home and Machines (R4): its old address goes to Home.
 await page.goto(`${base}/control`)
 await expect(page).toHaveURL(`${base}/`)
 if(process.env.REET_UI_SCREENSHOTS)await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true})
 await page.goto(`${base}/machines`)
 await page.getByText('Agents on this machine',{exact:true}).first().click()
 await page.getByText('Agents on this machine',{exact:true}).last().click()
 await expect(page.getByText('Content not reported',{exact:true}).first()).toBeVisible()
 await expect(page.getByText('Content report out of date · 1 skill',{exact:true})).toBeVisible()
 await page.getByText('Content observed · 1 skill',{exact:true}).click()
 await expect(page.getByText('a'.repeat(64),{exact:true})).toBeVisible()
 await expect(page.getByText(/No skill files have been uploaded/).first()).toBeVisible()
 expect(writes).toEqual([])
 await noOverflow()
 // The update card speaks about the machine as it is now: a finished update is one line, not its steps.
 const laptop=page.getByRole('article',{name:'dev-laptop'}), laptopUpdates=laptop.getByRole('region',{name:'reet updates for dev-laptop'})
 await expect(laptopUpdates).toContainText('Up to date')
 await expect(laptopUpdates).toContainText('Updated to test')
 await expect(laptopUpdates).toContainText('Automatic updates are on')
 await expect(laptopUpdates.getByText('Queued for the machine')).toHaveCount(0)
 await expect(laptopUpdates.getByRole('button')).toHaveCount(0)
 await expect(laptop.getByRole('checkbox',{name:'Update automatically'})).toHaveCount(0)
 // A failure keeps its steps, folded, because the step it stopped at is where diagnosis starts.
 const server=page.getByRole('article',{name:'build-server'}), serverUpdates=server.getByRole('region',{name:'reet updates for build-server'})
 await expect(serverUpdates).toContainText('Update did not finish')
 await expect(serverUpdates.getByText('Verifying the checksum and build')).toBeHidden()
 await serverUpdates.getByText('Show the steps').click()
 await expect(serverUpdates.getByText('Verifying the checksum and build')).toBeVisible()
 await expect(serverUpdates.getByRole('button',{name:'Try again…'})).toBeVisible()
 // Policies live in the machine's settings, not beside the update button.
 await expect(laptop.getByRole('button',{name:'Manage'})).toHaveCount(0)
 if(process.env.REET_UI_SCREENSHOTS)await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machines-desktop.png`,fullPage:true})
 await laptop.getByRole('button',{name:'Settings'}).click()
 const settings=page.getByRole('dialog',{name:'dev-laptop'})
 await expect(settings.getByRole('switch',{name:'Automatic updates'})).toBeChecked()
 if(process.env.REET_UI_SCREENSHOTS)await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machine-settings.png`})
 await settings.getByRole('switch',{name:'Automatic updates'}).click()
 expect(writes.at(-1)).toEqual({path:'/api/v2/sync/agents',body:{id:'a',action:'auto-update-off'}})
 await expect(settings.getByRole('switch',{name:'Automatic updates'})).not.toBeChecked()
 await expect(settings).toContainText('v1.4.2')
 // Forgetting hands over to the usual confirmation; the settings close first, so the machine must be read before they do.
 await settings.getByRole('button',{name:'Forget machine…'}).click()
 await expect(page.getByRole('dialog',{name:'Forget dev-laptop?'})).toBeVisible()
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await laptop.getByRole('button',{name:'Settings'}).click()
 await settings.getByRole('button',{name:'Switch channel…'}).click()
 const review=page.getByRole('dialog',{name:'Update reet on dev-laptop'})
 await expect(review.getByRole('radio',{name:/Development|development/})).toBeChecked()
 await review.getByRole('button',{name:'Update to the development build'}).click()
 expect(writes.at(-1)).toEqual({path:'/api/v2/sync/agents/update',body:{id:'a',version:'dev'}})
 await expect(page.getByRole('status').filter({hasText:'Switch to the development build queued for dev-laptop'})).toBeVisible()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 writes.length=0
 await noOverflow()
 await page.goto(`${base}/sync`)
 // The compare panel opens on the machine's saved set, not a blank form that contradicts it.
 await expect(page.getByText('Editing saved set · revision rev-laptop-shared')).toBeVisible()
 await expect(page.getByText('3 of 3 skills selected')).toBeVisible()
 await expect(page.getByRole('button',{name:'Sync all · preview'})).toBeEnabled()
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
 // Nothing selected never offers a plain Save: an empty set is only cleared deliberately.
 await expect(page.getByText('0 of 3 skills selected')).toBeVisible()
 await expect(page.getByRole('button',{name:'Save desired set (no files change yet)'})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Clear this desired set'})).toBeVisible()
 await expect(page.locator('.scope-option')).toHaveCount(3)
 await page.getByLabel('Find skills',{exact:true}).fill('code-review')
 await page.getByRole('button',{name:'Select visible',exact:true}).click()
 failSave=true
 await page.getByRole('button',{name:'Save desired set (no files change yet)',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('1 of 3 scopes saved')
 expect(writes.filter(w=>w.path.endsWith('/desired'))).toHaveLength(2)
 failSave=false
 await page.getByRole('button',{name:'Save desired set (no files change yet)',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'Desired skills saved for 3 scopes across 2 machines'})).toBeVisible()
 expect(writes.filter(w=>w.path.endsWith('/desired')).slice(-3).map(w=>[w.body.targetId,w.body.skillIds])).toEqual(environments.map(e=>[e.id,['demo/toolkit/code-review']]))
 await page.getByLabel('Sync mode',{exact:true}).selectOption('bidirectional')
 const beforeCancel=writes.length
 await page.getByRole('button',{name:'Save desired set (no files change yet)',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('removes extra skills')
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 expect(writes.length).toBe(beforeCancel)
 await page.getByLabel('Sync mode',{exact:true}).selectOption('additive')
 await expect(page.getByText('Differences by destination',{exact:true})).toBeVisible()
 const incoming=page.locator('details').filter({has:page.locator('summary').filter({hasText:'Only on selected scopes (2)'})})
 await incoming.locator('summary').click()
 for(const checkbox of await incoming.getByRole('checkbox').all())await checkbox.check()
 await incoming.getByRole('button',{name:'Add selected to reet.sh',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'2 selected skills added to reet.sh'})).toBeVisible()
 expect(writes.filter(w=>w.path.endsWith('/adopt')).map(w=>[w.body.targetId,w.body.skillIds])).toEqual([['laptop-shared',['demo/toolkit/testing']],['build-shared',['demo/toolkit/deployment']]])
 await page.getByLabel('Source',{exact:true}).selectOption('build-shared')
 await page.getByRole('button',{name:'Select visible',exact:true}).click()
 await page.getByRole('button',{name:'Save desired set (no files change yet)',exact:true}).click()
 await expect(page.getByRole('status').filter({hasText:'Desired skills saved for 3 scopes across 2 machines'})).toBeVisible()
 expect(writes.filter(w=>w.path.endsWith('/desired')).slice(-3).every(w=>w.body.sourceId==='build-shared'&&w.body.skillIds.length===3)).toBe(true)
 await noOverflow()
 await page.goto(`${base}/skills`)
 await expect(page.getByRole('heading',{name:'Skills.',exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'Create a skill',exact:true})).toBeVisible()
 await page.goto(`${base}/terminals`)
 await expect(page.getByRole('heading',{name:'Sessions'})).toBeVisible()
 await page.goto(`${base}/machines?view=compare`)
 await expect(page.getByLabel('Destination',{exact:true})).toBeVisible()
 // A scan is not proof of a live terminal connection. The wizard verifies both separately.
 terminalReady=false
 await page.goto(`${base}/machines#connect-machine`)
 await page.getByLabel('Machine name',{exact:true}).fill('dev-laptop')
 await page.getByRole('button',{name:'Continue to authentication →',exact:true}).click()
 await page.getByRole('button',{name:'Continue to terminal connection →',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Bring this machine into your workspace.'})).toBeVisible()
 await expect(page.getByText('Waiting for terminal readiness',{exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'Open terminals for dev-laptop →',exact:true})).toHaveCount(0)
 terminalReady=true
 await page.getByRole('button',{name:'Check terminal connection',exact:true}).click()
 await expect(page.getByRole('link',{name:'Open terminals for dev-laptop →',exact:true})).toHaveAttribute('href','/sessions?agent=a')
 // The watch keeps its own beat: a machine that becomes ready after the
 // operator stops pressing buttons appears without another click.
 terminalReady=false
 await page.getByRole('button',{name:'Check terminal connection',exact:true}).click()
 await expect(page.getByText('Waiting for terminal readiness',{exact:true})).toBeVisible()
 terminalReady=true
 await expect(page.getByRole('link',{name:'Open terminals for dev-laptop →',exact:true})).toBeVisible({timeout:10000})
 for(const width of [390,768,1440]){
  await page.setViewportSize({width,height:900})
  await page.goto(`${base}/machines`)
  await page.evaluate(()=>document.documentElement.dataset.theme='dark')
  await page.getByRole('article',{name:'dev-laptop'}).getByRole('button',{name:'Settings'}).click()
  await expect(page.getByRole('dialog',{name:'dev-laptop'})).toBeVisible()
  await noOverflow()
  if(width===390&&process.env.REET_UI_SCREENSHOTS)await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machine-settings-phone-dark.png`})
  await page.getByRole('button',{name:'Close settings'}).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.goto(`${base}/sync?machine=*`)
  await expect(page.locator('.scope-option')).toHaveCount(3)
  await noOverflow()
 }
 // A failed job's retry answers on the card that was pressed, not only in the banner at the top.
 // The panel's message is the real agent refusal: opting into apply is a local command.
 const failedJob=(id,agentId,targetId)=>({id,agentId,targetId,kind:'apply',status:'failed',revision:'rev',hash:'h',plan:{},message:'remote apply disabled locally; run reet agent allow-apply on in a terminal on this machine to opt in',stage:'',stages:[],createdAt:now-120000,updatedAt:now-60000})
 writes.length=0
 snapshot={...snapshot,agents:snapshot.agents.map(agent=>({...agent,lastSeen:Date.now()})),jobs:[failedJob('r1','a','laptop-shared'),failedJob('r2','b','build-shared')]}
 await page.setViewportSize({width:390,height:844})
 await page.goto(`${base}/sync`)
 const devJob=page.locator('.activity-host').filter({hasText:'dev-laptop'}).locator('.activity-job').filter({hasText:'Apply failed'})
 const devRetry=devJob.getByRole('button',{name:/run it/})
 await devRetry.scrollIntoViewIfNeeded()
 await devRetry.click()
 const devQueued=devJob.getByRole('status').filter({hasText:'Fresh preview queued for dev-laptop'})
 await expect(devQueued).toBeVisible()
 await devQueued.scrollIntoViewIfNeeded()
 await expect(devQueued).toBeInViewport()
 expect(writes.at(-1)).toEqual({path:'/api/v2/sync/jobs',body:{agentId:'a',targetId:'laptop-shared',kind:'preview'}})
 await expect(devJob.getByRole('button',{name:/run it/})).toHaveCount(0)
 failRetry=true
 const serverJob=page.locator('.activity-host').filter({hasText:'build-server'}).locator('.activity-job').filter({hasText:'Apply failed'})
 await serverJob.locator('summary').click()
 const serverRetry=serverJob.getByRole('button',{name:/run it/})
 await serverRetry.scrollIntoViewIfNeeded()
 await serverRetry.click()
 const refused=serverJob.getByRole('alert').filter({hasText:'Machine busy'})
 await expect(refused).toBeVisible()
 await refused.scrollIntoViewIfNeeded()
 await expect(refused).toBeInViewport()
 await expect(serverJob.getByRole('button',{name:/run it/})).toBeVisible()
 failRetry=false
 snapshot={...snapshot,jobs:[]}
 writes.length=0
 await page.setViewportSize({width:1440,height:900})
 failSource=true
 await page.goto(`${base}/sync?machine=*`)
 await expect(page.getByRole('alert')).toContainText('Source unavailable')
 await expect(page.getByRole('button',{name:'Save desired set (no files change yet)',exact:true})).toBeDisabled()
 failSource=false
 viewer=true
 await page.goto(`${base}/machines`)
 await expect(page.getByRole('heading',{name:'Your machines, connected.'})).toBeVisible()
 await expect(page.getByRole('link',{name:'Sessions',exact:true})).toHaveCount(0)
 viewer=false
 expect(errors).toEqual([])
 console.log('Control UI passed: /control to Home, navigation, all-machine selection, partial save/queue failures, no automatic apply, light/dark responsive layouts, viewer controls.')
} finally { await browser.close() }
