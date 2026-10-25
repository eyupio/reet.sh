import { chromium, expect } from '@playwright/test'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const page=await browser.newPage({viewport:{width:1360,height:1000}})
const now=Date.now(),DAY=86400000,errors=[],writes=[],requests=[],unmocked=[]
page.on('pageerror',e=>errors.push(e.message))
const deferred=()=>{let resolve;const promise=new Promise(r=>resolve=r);return{promise,resolve}}

// Server state. Pins are keyed by scope and library-skill name, versions are opaque and change on every real change,
// exactly as the real store does (internal/store/store_content_targets.go).
let authMode='accounts',viewer=false,hold=null,alphaHold=null,zetaBroken=true,sequence=0
const hex=c=>c.repeat(32)
const B={demo:hex('d'),alpha:hex('a'),zeta:hex('e'),long:hex('f')}
const R={d1:hex('1'),d2:hex('2'),d3:hex('3'),a1:hex('4'),z1:hex('5'),l1:hex('6')}
const LONG_NAME='a-very-long-private-skill-name-that-has-no-spaces-so-it-must-wrap-or-scroll-inside-its-container-and-never-the-page'
// The library lists bundles by name then id, and revisions oldest first.
const bundles=[{id:B.long,name:LONG_NAME,head:R.l1,createdAt:now-DAY},{id:B.alpha,name:'alpha',head:R.a1,createdAt:now-DAY},{id:B.demo,name:'demo',head:R.d3,createdAt:now-DAY},{id:B.zeta,name:'zeta',head:R.z1,createdAt:now-DAY}]
const revisions={
 [B.demo]:[{id:R.d1,createdAt:now-3*DAY,manifestDigest:'1'.repeat(64),origin:{kind:'import'}},{id:R.d2,parent:R.d1,createdAt:now-2*DAY,manifestDigest:'2'.repeat(64),origin:{kind:'user'}},{id:R.d3,parent:R.d2,createdAt:now-DAY,manifestDigest:'3'.repeat(64),origin:{kind:'user'}}],
 [B.alpha]:[{id:R.a1,createdAt:now-DAY,manifestDigest:'4'.repeat(64),origin:{kind:'import'}}],
 [B.zeta]:[{id:R.z1,createdAt:now-DAY,manifestDigest:'5'.repeat(64),origin:{kind:'user'}}],
 [B.long]:[{id:R.l1,createdAt:now-DAY,manifestDigest:'6'.repeat(64),origin:{kind:'user'}}],
}
// A pin that belongs to a different scope: it must never appear in this scope's dialog.
let pins=[{id:'pin-elsewhere',environmentId:'scope-stale',agentId:'agent-0',name:'demo',bundleId:B.demo,revisionId:R.d1,manifestDigest:'1'.repeat(64),mode:'additive',version:'ver-seed',updatedAt:now,verification:'not_verified'}]
const mine=()=>pins.find(p=>p.environmentId==='scope-one'&&p.name==='demo')
const serverEdit=change=>{pins=pins.map(p=>p===mine()?{...p,...change}:p)}
const CONFLICT={error:'The content target changed. Refresh and review before saving again.'}

// Machines. One host has four scopes: supported, scan-only Codex, a stale scan and (with a second, paused connector
// listed first) a running connector. The others have a paused connector and no connector at all.
const content={state:'observed',skills:[{name:'demo',complete:true,files:3,bytes:92,digest:'d'.repeat(64)},{name:'other',complete:true,files:1,bytes:10,digest:'e'.repeat(64)}]}
const scope=(id,host,agent,over={})=>({id,host,name:host,kind:'agent',agent,project:'',complete:true,scannedAt:now,lastSeen:now,skills:[],skillCount:2,content,os:'linux',arch:'amd64',version:'dev',...over})
const one=scope('scope-one','workstation','agents'),codex=scope('scope-codex','workstation','codex'),stale=scope('scope-stale','workstation','claude',{scannedAt:now-15*DAY})
const sleeper=scope('scope-paused','sleeper','cursor'),orphan=scope('scope-orphan','orphan','agents')
const envs=[one,codex,stale,sleeper,orphan]
const machines=[{host:'workstation',groups:[one,codex,stale],skillCount:2},{host:'sleeper',groups:[sleeper],skillCount:2},{host:'orphan',groups:[orphan],skillCount:2}]
const agent=(id,host,paused)=>({id,host,version:'dev',paused,autoUpdate:false,lastSeen:now})
const snapshot={environments:envs,machines,agents:[agent('agent-paused-twin','WORKSTATION',true),agent('agent-0','workstation',false),agent('agent-1','sleeper',true)],targets:[],jobs:[]}

await page.route('**/api/**',async route=>{
 const req=route.request(),path=new URL(req.url()).pathname,method=req.method(),body=method==='GET'?null:req.postDataJSON()
 requests.push({method,path})
 let data={}
 if(path==='/api/v2/auth/status')data=authMode==='accounts'?{mode:'accounts',setupRequired:false,bootstrapRequired:false,user:{id:1,name:'Operator',email:'operator@example.test',role:viewer?'viewer':'owner'}}:{mode:authMode,setupRequired:false,bootstrapRequired:false}
 else if(path==='/api/v2/profile')data={profile:{theme:'system',shelfView:'list'}}
 else if(path==='/api/skills')data={skills:[],counts:{all:0,used:0}}
 else if(path==='/api/environments')data={environments:envs,machines}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/sync')data=snapshot
 else if(path==='/api/v2/machines/health')data={serverNow:now,machines:[]}
 else if(path==='/api/v2/setup/profiles')data={profiles:[]}
 else if(path==='/api/v2/terminal/machines')data={machines:[]}
 else if(path==='/api/v2/terminal/sessions')data={sessions:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'dev',commit:'fixture'}
 else if(path==='/api/v2/skill-bundles')data={bundles}
 // The revision read that returns file bytes. Nothing may ever ask for it.
 else if(path.match(/^\/api\/v2\/skill-bundles\/[^/]+\/revisions\/[^/]+/))return route.fulfill({status:500,json:{error:'bytes route called'}})
 else if(path.match(/^\/api\/v2\/skill-bundles\/[^/]+\/revisions$/)){
  const id=path.split('/')[4]
  if(id===B.alpha&&alphaHold)await alphaHold.promise
  if(id===B.zeta&&zetaBroken)return route.fulfill({status:500,json:{error:'The library is unavailable right now.'}})
  data={revisions:revisions[id]??[]}
 }
 else if(path==='/api/v2/skill-content-targets'&&method==='GET')data={targets:pins}
 else if(path==='/api/v2/skill-content-targets'&&method==='POST'){
  writes.push({method,path,body})
  if(hold)await hold.promise
  const bundle=bundles.find(b=>b.id===body.bundleId),current=pins.find(p=>p.environmentId===body.environmentId&&p.name===bundle?.name)
  if(current&&current.agentId===body.agentId&&current.bundleId===body.bundleId&&current.revisionId===body.revisionId&&current.mode===body.mode)data=current
  else if((current&&current.version!==body.expectedVersion)||(!current&&body.expectedVersion!==''))return route.fulfill({status:409,json:CONFLICT})
  else{
   const next={id:current?.id??`pin-${++sequence}`,environmentId:body.environmentId,agentId:body.agentId,name:bundle.name,bundleId:body.bundleId,revisionId:body.revisionId,manifestDigest:'0'.repeat(64),mode:body.mode,version:`ver-${++sequence}`,updatedAt:now,verification:'not_verified'}
   pins=current?pins.map(p=>p===current?next:p):[...pins,next];data=next
  }
 }
 else if(path.match(/^\/api\/v2\/skill-content-targets\/[^/]+$/)&&method==='DELETE'){
  writes.push({method,path,body})
  const pin=pins.find(p=>p.id===decodeURIComponent(path.split('/').pop()))
  if(!pin)return route.fulfill({status:404,json:{error:'Skill content target unavailable.'}})
  if(pin.version!==body.expectedVersion)return route.fulfill({status:409,json:CONFLICT})
  pins=pins.filter(p=>p!==pin);data={removed:true}
 }
 else{unmocked.push(`${method} ${path}`);return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})}
 await route.fulfill({json:data})
})

const layout=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
const storage=()=>page.evaluate(async()=>JSON.stringify({local:{...localStorage},session:{...sessionStorage},idb:(await indexedDB.databases()).map(d=>d.name).sort()}))
const card=host=>page.locator('article').filter({has:page.getByRole('heading',{name:host,exact:true})})
const scopeRow=(host,label)=>card(host).locator('li').filter({has:page.getByText(label,{exact:true})}).first()
const scopeButton=(host,label)=>scopeRow(host,label).getByRole('button',{name:/^Private skills/})
const expand=async host=>{const details=card(host).locator('details.agent-list');if(!await details.evaluate(el=>el.open))await details.locator(':scope > summary').click()}
const post=expectedVersion=>({environmentId:'scope-one',agentId:'agent-0',bundleId:B.demo,revisionId:R.d3,mode:'additive',expectedVersion})
const SHARED='Shared agents / Codex'

try{
 await page.goto(`${base}/machines`)
 await expect(card('workstation')).toBeVisible()
 const before=await storage()
 for(const host of ['workstation','sleeper','orphan'])await expand(host)

 // Entry points: offered for a supported scope, disabled with a plain reason when the reason can be fixed, and absent for a viewer.
 await expect(scopeButton('workstation',SHARED)).toBeEnabled()
 await expect(scopeButton('workstation','Legacy Codex (scan only)')).toBeDisabled()
 await expect(scopeRow('workstation','Legacy Codex (scan only)')).toContainText("This agent is scan-only, so private skills can't be pinned to it.")
 await expect(scopeButton('workstation','Claude Code')).toBeDisabled()
 await expect(scopeRow('workstation','Claude Code')).toContainText("hasn't reported a complete scan of this folder in the last 14 days")
 await expect(scopeButton('sleeper','Cursor')).toBeDisabled()
 await expect(scopeRow('sleeper','Cursor')).toContainText('The connector on this machine is paused')
 await expect(scopeButton('orphan',SHARED)).toBeDisabled()
 await expect(scopeRow('orphan',SHARED)).toContainText('No connector is enrolled on this machine')
 // A reported skill row gets its own button beside Import privately, only where the scope can take a pin.
 await scopeRow('workstation',SHARED).getByText('Content observed',{exact:false}).click()
 await expect(scopeRow('workstation',SHARED).getByRole('button',{name:'Import demo privately'})).toBeVisible()
 await expect(scopeRow('workstation',SHARED).getByRole('button',{name:'Pin a library revision for demo'})).toBeVisible()
 await expect(scopeRow('workstation',SHARED).getByRole('button',{name:'Pin a library revision for other'})).toBeVisible()
 for(const [host,label] of [['workstation','Legacy Codex (scan only)'],['workstation','Claude Code'],['sleeper','Cursor'],['orphan',SHARED]]){
  await scopeRow(host,label).getByText('Content observed',{exact:false}).click()
  await expect(scopeRow(host,label).getByRole('button',{name:/^Pin a library revision/})).toHaveCount(0)
 }
 expect(writes).toEqual([])

 // Opening the dialog from the scope reads and never writes. Only this scope's pins are shown.
 const opener=scopeButton('workstation',SHARED)
 await opener.click()
 const dialog=page.getByRole('dialog',{name:'Private skills on workstation'})
 await expect(dialog).toBeVisible()
 await expect(dialog.getByText('Nothing is pinned to this folder yet.')).toBeVisible()
 await expect(dialog).not.toContainText('11111111')
 await expect(dialog).toContainText('installs nothing and writes nothing to the machine')
 expect(writes).toEqual([])
 const library=dialog.getByLabel('Library skill',{exact:true}),revision=dialog.getByLabel('Revision',{exact:true}),save=dialog.getByRole('button',{name:'Save pin',exact:true})
 await expect(save).toBeDisabled()
 await expect(library).toHaveValue('')

 // Create: the newest revision is preselected, older ones stay available, and the default mode adds and updates.
 await library.selectOption(B.demo)
 await expect(revision).toHaveValue(R.d3)
 expect(await revision.evaluate(s=>[...s.options].map(o=>o.value))).toEqual([R.d3,R.d2,R.d1])
 await expect(dialog.getByRole('radio',{name:'Add and update'})).toBeChecked()
 await expect(dialog.getByRole('radio',{name:/^Also remove files reet owns that a newer revision drops/})).not.toBeChecked()
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText('demo')
 expect(writes).toEqual([{method:'POST',path:'/api/v2/skill-content-targets',body:post('')}])
 const pinsList=dialog.getByRole('list',{name:'Pinned in this folder'})
 await expect(pinsList.getByRole('listitem')).toHaveCount(1)
 await expect(pinsList).toContainText('33333333')
 await expect(pinsList).toContainText('Add and update')
 await expect(pinsList).toContainText('Newest in the library')
 await expect(pinsList).not.toContainText('11111111')

 // Change: the pin's current version goes with the write, here for a rollback with another mode.
 const first=mine().version
 await pinsList.getByRole('button',{name:'Change pin for demo'}).click()
 await expect(library).toHaveValue(B.demo)
 await expect(revision).toHaveValue(R.d3)
 await revision.selectOption(R.d2)
 await dialog.getByRole('radio',{name:/^Also remove files reet owns that a newer revision drops/}).check()
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText('22222222')
 expect(writes).toHaveLength(2)
 expect(writes[1]).toEqual({method:'POST',path:'/api/v2/skill-content-targets',body:{...post(first),revisionId:R.d2,mode:'managed'}})
 await expect(pinsList).toContainText('22222222')
 await expect(pinsList).toContainText('Also remove files reet owns that a newer revision drops')
 await expect(pinsList).toContainText('Not the newest in the library')

 // Conflict: someone else changed the pin. The write is refused, the pin is read again and the screen says what changed.
 const known=mine().version
 serverEdit({revisionId:R.d1,mode:'additive',version:'ver-elsewhere'})
 await pinsList.getByRole('button',{name:'Change pin for demo'}).click()
 await revision.selectOption(R.d3)
 await save.click()
 await expect(dialog.getByRole('alert')).toContainText('changed')
 await expect(dialog.getByRole('alert')).toContainText('11111111')
 await expect(dialog.getByRole('alert')).toContainText('Add and update')
 await expect(dialog.getByRole('alert')).toContainText('Nothing was saved')
 await expect(pinsList).toContainText('11111111')
 expect(writes).toHaveLength(3)
 expect(writes[2].body).toEqual({...post(known),revisionId:R.d3,mode:'managed'})
 // After reading what changed, saving again sends the version that is current now, not the one that was refused.
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText('33333333')
 expect(writes).toHaveLength(4)
 expect(writes[3].body).toEqual({...post('ver-elsewhere'),mode:'managed'})

 // A second click while a save is in flight sends nothing: a real double click, then a click forced past the disabled button.
 const current=mine().version
 await pinsList.getByRole('button',{name:'Change pin for demo'}).click()
 await revision.selectOption(R.d2)
 hold=deferred()
 await save.dblclick()
 await expect.poll(()=>writes.length).toBe(5)
 await expect(save).toBeDisabled()
 await expect(pinsList.getByRole('button',{name:'Remove pin for demo'})).toBeDisabled()
 await save.evaluate(el=>{el.disabled=false;el.click()})
 await page.waitForTimeout(250)
 expect(writes).toHaveLength(5)
 expect(writes[4].body).toEqual({...post(current),revisionId:R.d2,mode:'managed'})
 hold.resolve();hold=null
 await expect(dialog.getByText(/^Sorted\./)).toContainText('22222222')
 expect(writes).toHaveLength(5)

 // Remove asks first, says it removes intent only, then sends the current version.
 await pinsList.getByRole('button',{name:'Remove pin for demo'}).click()
 const confirm=page.getByRole('dialog',{name:'Remove the pin for demo?'})
 await expect(confirm).toBeVisible()
 await expect(confirm).toContainText('intent')
 await expect(confirm).toContainText('ownership record')
 await expect(confirm).toContainText('stays in place')
 await confirm.getByRole('button',{name:'Cancel'}).click()
 await expect(confirm).toHaveCount(0)
 expect(writes).toHaveLength(5)
 const removing=mine()
 await pinsList.getByRole('button',{name:'Remove pin for demo'}).click()
 await confirm.getByRole('button',{name:'Remove pin',exact:true}).click()
 await expect(dialog.getByText('Nothing is pinned to this folder yet.')).toBeVisible()
 await expect(dialog.getByRole('status').filter({hasText:'intent only'})).toBeVisible()
 expect(writes).toHaveLength(6)
 expect(writes[5]).toEqual({method:'DELETE',path:`/api/v2/skill-content-targets/${removing.id}`,body:{expectedVersion:removing.version}})
 expect(pins.some(p=>p.id===removing.id)).toBe(false)

 // A removal that finds the pin changed is refused, and the pin stays.
 await library.selectOption(B.demo)
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toBeVisible()
 expect(writes).toHaveLength(7)
 expect(writes[6].body).toEqual(post(''))
 const created=mine().version
 serverEdit({version:'ver-moved',revisionId:R.d2})
 await pinsList.getByRole('button',{name:'Remove pin for demo'}).click()
 await confirm.getByRole('button',{name:'Remove pin',exact:true}).click()
 await expect(dialog.getByRole('alert')).toContainText('changed elsewhere')
 await expect(dialog.getByRole('alert')).toContainText('22222222')
 await expect(dialog.getByRole('alert')).toContainText('Nothing was removed')
 await expect(pinsList.getByRole('listitem')).toHaveCount(1)
 expect(writes).toHaveLength(8)
 expect(writes[7].body).toEqual({expectedVersion:created})
 expect(mine()).toBeTruthy()

 // Closing returns focus to the button that opened the dialog.
 await dialog.getByRole('button',{name:'Close private skills'}).click()
 await expect(dialog).toHaveCount(0)
 await expect(opener).toBeFocused()

 // From a reported skill row: the matching library skill sorts first and is chosen for you; the newest revision follows.
 await scopeRow('workstation',SHARED).getByRole('button',{name:'Pin a library revision for demo'}).click()
 await expect(dialog).toBeVisible()
 expect(await library.evaluate(s=>[...s.options].map(o=>o.value).filter(Boolean))).toEqual([B.demo,B.long,B.alpha,B.zeta])
 await expect(library).toHaveValue(B.demo)
 await expect(revision).toHaveValue(R.d3)
 await expect(pinsList).toContainText('demo')
 expect(writes).toHaveLength(8)
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 // A reported skill that matches nothing in the library chooses nothing.
 await scopeRow('workstation',SHARED).getByRole('button',{name:'Pin a library revision for other'}).click()
 await expect(library).toHaveValue('')
 await expect(save).toBeDisabled()

 // A slow answer for an earlier choice must not overwrite a later one.
 alphaHold=deferred()
 await library.selectOption(B.alpha)
 await expect(dialog.getByText('Loading revisions')).toBeVisible()
 await expect(save).toBeDisabled()
 await library.selectOption(B.demo)
 await expect(revision).toHaveValue(R.d3)
 alphaHold.resolve();alphaHold=null
 await page.waitForTimeout(250)
 await expect(revision).toHaveValue(R.d3)
 expect(await revision.evaluate(s=>[...s.options].map(o=>o.value))).toEqual([R.d3,R.d2,R.d1])
 // A failed revision read says so, cannot be saved, and can be tried again.
 await library.selectOption(B.zeta)
 await expect(dialog.getByRole('alert')).toContainText('The library is unavailable right now.')
 await expect(save).toBeDisabled()
 zetaBroken=false
 await dialog.getByRole('button',{name:'Try again'}).click()
 await expect(revision).toHaveValue(R.z1)
 await expect(save).toBeEnabled()
 // A long, unbroken name is pinned too, for the layout checks below.
 await library.selectOption(B.long)
 await expect(revision).toHaveValue(R.l1)
 await save.click()
 await expect(pinsList.getByRole('listitem')).toHaveCount(2)
 expect(writes).toHaveLength(9)
 expect(writes[8].body).toEqual({environmentId:'scope-one',agentId:'agent-0',bundleId:B.long,revisionId:R.l1,mode:'additive',expectedVersion:''})
 await pinsList.getByRole('button',{name:'Change pin for demo'}).click()

 // Layout: no horizontal page scroll, the dialog stays inside the window with its header and footer in view, light and dark.
 const fits=()=>dialog.evaluate(el=>{const r=el.getBoundingClientRect(),body=el.querySelector('.body');return r.left>=0&&r.right<=innerWidth+.5&&r.top>=0&&r.bottom<=innerHeight+.5&&body.scrollWidth<=body.clientWidth+1})
 for(const [width,height] of [[1360,1000],[390,844],[320,640]])for(const scheme of ['dark','light']){
  await page.setViewportSize({width,height});await page.emulateMedia({colorScheme:scheme})
  await layout();expect(await fits()).toBe(true)
  await expect(dialog.getByRole('button',{name:'Close private skills'})).toBeInViewport()
  await expect(dialog.getByRole('button',{name:'Done'})).toBeInViewport()
  if(process.env.REET_CONTENT_SCREENSHOT_DIR){await page.waitForTimeout(400);await page.screenshot({path:`${process.env.REET_CONTENT_SCREENSHOT_DIR}/pin-${width}-${scheme}.png`})}
 }
 await page.setViewportSize({width:1360,height:1000});await page.emulateMedia({colorScheme:'light'})
 await dialog.getByRole('button',{name:'Done'}).click()

 // Pins, checks and ownership never touch browser storage, and nothing ever asked for revision bytes or any other read of a skill's files.
 expect(await storage()).toBe(before)
 expect(requests.filter(r=>/\/revisions\/[^/]/.test(r.path))).toEqual([])
 expect(requests.filter(r=>r.method==='GET'&&/skill-content-(previews|receipts)/.test(r.path))).toEqual([])

 // A viewer and a legacy sign-in mode see no Private skills actions at all (and a legacy mode keeps the existing imports button).
 viewer=true;await page.reload()
 await expect(card('workstation')).toBeVisible();await expand('workstation')
 await scopeRow('workstation',SHARED).getByText('Content observed',{exact:false}).click()
 await expect(page.getByRole('button',{name:/^Private skills/})).toHaveCount(0)
 await expect(page.getByRole('button',{name:/^Pin a library revision/})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Private skill imports',exact:true})).toHaveCount(0)
 viewer=false;authMode='none';await page.reload()
 await expect(card('workstation')).toBeVisible();await expand('workstation')
 await scopeRow('workstation',SHARED).getByText('Content observed',{exact:false}).click()
 await expect(page.getByRole('button',{name:'Private skill imports',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:/^Private skills/})).toHaveCount(0)
 await expect(page.getByRole('button',{name:/^Pin a library revision/})).toHaveCount(0)
 expect(unmocked).toEqual([])
 expect(errors).toEqual([])
 console.log('Skill content UI passed: entry points and reasons, read-only open, create/change/remove with versions, conflicts, busy guard, newest revision, stale-response fencing, focus return, storage, responsive layouts and viewer/legacy isolation.')
}finally{await browser.close()}
