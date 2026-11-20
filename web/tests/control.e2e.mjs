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
const skill=id=>({id:`demo/toolkit/${id}`,name:id,slug:id,url:`https://www.skills.sh/demo/toolkit/${id}`,source:'demo/toolkit',sourceType:'github',description:'Example skill for UI verification.',notes:'',installs:0,installsWeek:0,trend:[],liked:false,bookmarked:false,used:true,createdAt:now,updatedAt:now})
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
 else if(path==='/api/v2/skills'&&request.method()==='GET'){const status=new URL(request.url()).searchParams.get('status')||'active';data={skills:status==='active'?skills.map(s=>({id:s.id,shelfId:s.id,name:s.name,description:s.description,status:'active',originKind:'imported',origin:s.url,yours:false,published:false,hasCopy:true,liked:false,notes:'',tags:[],lists:[],health:{state:'ok',pending:0,total:2,hosts:[],machines:[]}})):[],counts:{active:skills.length,archived:0},newOnMachines:[],machines:2,notice:null}}
 else if(path==='/api/environments')data={environments,machines:snapshot.machines}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/library')data={schemaVersion:1,items:[]}
 else if(path==='/api/v2/sync'){
  if(failState)return route.fulfill({status:503,json:{error:'Inventory unavailable'}})
  data=snapshot
 }
 // The shared library's setup sits on Advanced sync now, and reads copies in progress.
 else if(path==='/api/v2/skill-imports')data={requests:[],capabilities:[]}
 else if(path==='/api/v2/sync/skills')data={machines:snapshot.agents.map(a=>({agentId:a.id,host:a.host,paused:!!a.paused,online:true,protocol:2,state:a.paused?'paused':'ok',failed:[],waiting:[],lastSeen:now})),activity:[],work:{failed:0,installing:0,removing:0,updating:0},active:skills.length}
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
 else if(path==='/api/v2/sync/skills')data={active:0,work:{failed:0,installing:0,removing:0,updating:0},activity:[],machines:[]}
 else if(path==='/api/v2/machines/health')data={serverNow:now,machines:[]}
 else if(path==='/api/v2/setup/profiles')data={profiles:[]}
 else if(path==='/api/v2/terminal/machines'){if(failState)return route.fulfill({status:503,json:{error:'Inventory unavailable'}});data={machines:snapshot.agents.map(item=>({agentId:item.id,host:item.host,ready:terminalReady,serverEnabled:true,capabilities:{terminalControl:true,osUser:'developer',terminalEnabled:true,readiness:'ready',launch:true,agentsV2:true,tools:['claude','codex'],acpAgents:[{tool:'claude',adapter:'claude-agent-acp',state:'missing',minimumVersion:'0.99.1'},{tool:'codex',adapter:'codex-acp',state:'missing',minimumVersion:'1.13.1'}]}}))}}
 else if(path==='/api/v2/terminal/sessions')data={sessions:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'test',commit:'fixture'}
 else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 await route.fulfill({json:data})
})
const noOverflow=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
const openSettings=async card=>{await card.locator('summary[aria-label^="More options"]').click();await page.getByRole('menuitem',{name:'Settings…',exact:true}).click()}
try {
 // Control is folded into Home and Machines (R4): its old address goes to Home.
 await page.goto(`${base}/control`)
 await expect(page).toHaveURL(`${base}/`)
 if(process.env.REET_UI_SCREENSHOTS)await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true})
 await page.goto(`${base}/machines`)
 await page.getByText('Agents on this machine',{exact:true}).first().click()
 await page.getByText('Agents on this machine',{exact:true}).last().click()
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
 await expect(serverUpdates.getByRole('button',{name:/^Retry update to/})).toBeVisible()
 // A right-click anywhere on a machine card opens the same options menu as its ⋮ button.
 await laptop.click({button:'right'})
 await expect(page.getByRole('menuitem',{name:'Choose skills & locations'})).toHaveCount(0)
 await expect(page.getByRole('menuitem',{name:'Settings…',exact:true})).toBeVisible()
 await page.keyboard.press('Escape')
 await expect(page.getByRole('menuitem',{name:'Settings…',exact:true})).toBeHidden()
 await expect(laptop.locator('summary[aria-label^="More options"]')).toBeFocused()
 // Policies live in the machine's settings, not beside the update button.
 await expect(laptop.getByRole('button',{name:'Manage'})).toHaveCount(0)
 if(process.env.REET_UI_SCREENSHOTS)await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machines-desktop.png`,fullPage:true})
 await openSettings(laptop)
 const settings=page.getByRole('dialog',{name:'dev-laptop'})
 await expect(settings.getByRole('switch',{name:'Automatic updates'})).toBeChecked()
 await expect(settings.getByRole('button',{name:'Forget Claude Code…'})).toBeVisible()
 await expect(laptop.getByText('Forget a report…')).toHaveCount(0)
 if(process.env.REET_UI_SCREENSHOTS)await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machine-settings.png`})
 await settings.getByRole('switch',{name:'Automatic updates'}).click()
 await expect.poll(()=>writes.at(-1)).toEqual({path:'/api/v2/sync/agents',body:{id:'a',action:'auto-update-off'}})
 await expect(settings.getByRole('switch',{name:'Automatic updates'})).not.toBeChecked()
 await expect(settings).toContainText('v1.4.2')
 // Forgetting an install location lives in Settings too, and hands over to the usual confirmation.
 await settings.getByRole('button',{name:'Forget Claude Code…'}).click()
 await expect(page.getByRole('dialog',{name:'Forget Claude Code on dev-laptop?'})).toBeVisible()
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await openSettings(laptop)
 await settings.getByRole('button',{name:'Forget machine…'}).click()
 // The settings close first, so the machine must be read before the confirmation opens.
 await expect(page.getByRole('dialog',{name:'Forget dev-laptop?'})).toBeVisible()
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await openSettings(laptop)
 await settings.getByRole('button',{name:'Switch channel…'}).click()
 const review=page.getByRole('dialog',{name:'Update reet on dev-laptop'})
 await expect(review.getByRole('radio',{name:/Development|development/})).toBeChecked()
 await review.getByRole('button',{name:'Update to the development build'}).click()
 await expect.poll(()=>writes.at(-1)).toEqual({path:'/api/v2/sync/agents/update',body:{id:'a',version:'dev'}})
 await expect(page.getByRole('status').filter({hasText:'Switch to the development build queued for dev-laptop'})).toBeVisible()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 writes.length=0
 await noOverflow()
 // Retired comparison links lead to current status, without plan selectors.
 await page.goto(`${base}/sync/advanced?machine=dev-laptop`)
 await expect(page).toHaveURL(`${base}/sync`)
 await expect(page.getByLabel('Sync mode',{exact:true})).toHaveCount(0)
 // A scan is not proof of a live terminal connection. The wizard verifies both separately.
 terminalReady=false
 await page.goto(`${base}/machines#connect-machine`)
 await page.getByLabel('Machine name',{exact:true}).fill('dev-laptop')
 await page.getByRole('button',{name:'Continue to authentication',exact:true}).click()
 await page.getByRole('button',{name:'Continue to terminal connection',exact:true}).click()
 await expect(page.getByRole('heading',{name:'Bring this machine into your workspace.'})).toBeVisible()
 await expect(page.getByText('Waiting for terminal readiness',{exact:true})).toBeVisible()
 await expect(page.getByRole('link',{name:'Open terminals for dev-laptop',exact:true})).toHaveCount(0)
 terminalReady=true
 await page.getByRole('button',{name:'Check terminal connection',exact:true}).click()
 await expect(page.getByRole('link',{name:'Open terminals for dev-laptop',exact:true})).toHaveAttribute('href','/sessions?agent=a')
 // The watch keeps its own beat: a machine that becomes ready after the
 // operator stops pressing buttons appears without another click.
 terminalReady=false
 await page.getByRole('button',{name:'Check terminal connection',exact:true}).click()
 await expect(page.getByText('Waiting for terminal readiness',{exact:true})).toBeVisible()
 terminalReady=true
 await expect(page.getByRole('link',{name:'Open terminals for dev-laptop',exact:true})).toBeVisible({timeout:10000})
 for(const width of [390,768,1440]){
  await page.setViewportSize({width,height:900})
  await page.goto(`${base}/machines`)
  await page.evaluate(()=>document.documentElement.dataset.theme='dark')
  await openSettings(page.getByRole('article',{name:'dev-laptop'}))
  await expect(page.getByRole('dialog',{name:'dev-laptop'})).toBeVisible()
  await noOverflow()
  // The machine reports the Gaffer adapters for Claude Code and Codex missing: each hint is there, with its command, and wholly inside the dialog.
  const hints=page.getByRole('dialog',{name:'dev-laptop'}).locator('.chat-adapter')
  await expect(hints).toHaveCount(2)
  await expect(hints.first()).toContainText('npm install --global @agentclientprotocol/claude-agent-acp@0.99.1')
  await expect(hints.last()).toContainText('npm install --global @agentclientprotocol/codex-acp@1.13.1')
  for(const adapterHint of [hints.first(),hints.last()]){
   await expect(adapterHint).toBeVisible()
   const inside=await adapterHint.locator('code').evaluate(el=>{const r=el.getBoundingClientRect(),d=el.closest('dialog').getBoundingClientRect();return r.left>=d.left-1&&r.right<=d.right+1})
   expect(inside).toBe(true)
  }
  if(width===390&&process.env.REET_UI_SCREENSHOTS)await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/machine-settings-phone-dark.png`})
  await page.getByRole('button',{name:'Close settings'}).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.goto(`${base}/sync`)
  await expect(page.locator('.scope-option')).toHaveCount(0)
  await noOverflow()
 }
 expect(errors).toEqual([])
 console.log('Control UI passed: updates, settings, connection, terminal readiness and responsive layouts.')
} finally { await browser.close() }
