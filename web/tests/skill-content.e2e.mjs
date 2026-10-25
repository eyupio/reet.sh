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
let pinsBroken=false,pinsHold=null,dropNextSave=false,failSave404=false,connectorsPaused=false
const hex=c=>c.repeat(32)
const B={demo:hex('d'),alpha:hex('a'),zeta:hex('e'),long:hex('f'),twinA:hex('7'),twinB:hex('8')}
const R={d1:hex('1'),d2:hex('2'),d3:hex('3'),a1:hex('4'),z1:hex('5'),l1:hex('6')}
const LONG_NAME='a-very-long-private-skill-name-that-has-no-spaces-so-it-must-wrap-or-scroll-inside-its-container-and-never-the-page'
// The library lists bundles by name then id, and revisions oldest first.
// Two library skills share the name twin, so only they may show the start of an id.
const bundles=[{id:B.long,name:LONG_NAME,head:R.l1,createdAt:now-DAY},{id:B.alpha,name:'alpha',head:R.a1,createdAt:now-DAY},{id:B.demo,name:'demo',head:R.d3,createdAt:now-DAY},{id:B.twinA,name:'twin',head:hex('9'),createdAt:now-DAY},{id:B.twinB,name:'twin',head:hex('9'),createdAt:now-DAY},{id:B.zeta,name:'zeta',head:R.z1,createdAt:now-DAY}]
const T={d1:now-3*DAY,d2:now-2*DAY,d3:now-DAY}
const revisions={
 [B.demo]:[{id:R.d1,createdAt:T.d1,manifestDigest:'1'.repeat(64),origin:{kind:'import'}},{id:R.d2,parent:R.d1,createdAt:T.d2,manifestDigest:'2'.repeat(64),origin:{kind:'user'}},{id:R.d3,parent:R.d2,createdAt:T.d3,manifestDigest:'3'.repeat(64),origin:{kind:'user'}}],
 [B.alpha]:[{id:R.a1,createdAt:now-DAY,manifestDigest:'4'.repeat(64),origin:{kind:'import'}}],
 [B.zeta]:[{id:R.z1,createdAt:now-DAY,manifestDigest:'5'.repeat(64),origin:{kind:'user'}}],
 [B.long]:[{id:R.l1,createdAt:now-DAY,manifestDigest:'6'.repeat(64),origin:{kind:'user'}}],
}
// A pin that belongs to a different scope: it must never appear in this scope's dialog.
let pins=[{id:'pin-elsewhere',environmentId:'scope-stale',agentId:'agent-0',name:'demo',bundleId:B.demo,revisionId:R.d1,manifestDigest:'1'.repeat(64),mode:'additive',version:'ver-seed',updatedAt:now,verification:'not_verified'}]
const mine=()=>pins.find(p=>p.environmentId==='scope-one'&&p.name==='demo')
const serverEdit=change=>{pins=pins.map(p=>p===mine()?{...p,...change}:p)}
const CONFLICT={error:'The content target changed. Refresh and review before saving again.'}

// Checks. The server keeps full previews; the list returns summaries (no capture, plan or review hash), as the real one does.
// A create is idempotent by requestKey, one check per connector is active at a time, and the list says whether the connector
// has recently negotiated (internal/store/store_content_previews.go).
let previewsDb=[],capReady=true,dropCreate=null,previewsBroken=false,previewGetHold=null,previewSeq=0
const previewReads=[]
const BUSY={error:'content preview changed, expired, busy or unavailable'}
const summary=p=>{const{capture,plan,reviewHash,captureHash,adoption,...rest}=p;return rest}
const mkPreview=(pin,requestKey)=>({id:`chk${String(++previewSeq).padStart(4,'0')}`,requestKey,targetId:pin.id,targetVersion:pin.version,environmentId:pin.environmentId,agentId:pin.agentId,agent:'agents',project:'',name:pin.name,bundleId:pin.bundleId,revisionId:pin.revisionId,manifestDigest:pin.manifestDigest,mode:pin.mode,instanceId:'instance-1',inventoryRevision:'4611686018427387904',state:'queued',reasons:[],createdAt:Date.now()+previewSeq,updatedAt:Date.now(),expiresAt:Date.now()+600000,authority:'preview_only'})
const latest=()=>previewsDb.at(-1)
const setLatest=over=>{Object.assign(latest(),over,{updatedAt:Date.now()})}
const file=(path,bytes,executable=false)=>({path,bytes,digest:'a'.repeat(64),executable})
const manifestOf=files=>({schemaVersion:1,algorithm:'sha256',modeSemantics:'executable-bit',complete:true,files,totalBytes:files.reduce((n,f)=>n+f.bytes,0)})
const captureOf=(presence,over={})=>({schemaVersion:1,adapter:'agents',rootId:'r'.repeat(16),rootOwnership:'exclusive',presence,complete:presence==='present',issues:[],...over})
const planOf=(over={})=>({version:1,preconditions:{},ready:true,actions:[],conflicts:[],preserved:[],hash:'p'.repeat(64),...over})
const OWNED=manifestOf([file('SKILL.md',120)])

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
 else if(path==='/api/v2/sync')data={...snapshot,agents:snapshot.agents.map(a=>connectorsPaused&&a.host.toLowerCase()==='workstation'?{...a,paused:true}:a)}
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
 else if(path==='/api/v2/skill-content-targets'&&method==='GET'){
  if(pinsHold)await pinsHold.promise
  if(pinsBroken)return route.fulfill({status:500,json:{error:'The pins could not be read right now.'}})
  data={targets:pins}
 }
 else if(path==='/api/v2/skill-content-targets'&&method==='POST'){
  writes.push({method,path,body})
  if(hold)await hold.promise
  if(failSave404)return route.fulfill({status:404,json:{error:'Private skill unavailable.'}})
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
 else if(path==='/api/v2/skill-content-previews'&&method==='GET'){
  if(previewsBroken)return route.fulfill({status:500,json:{error:'The checks could not be read right now.'}})
  data={previews:previewsDb.map(summary),capabilities:[{agentId:'agent-0',ready:capReady,observedAt:now}]}
 }
 else if(path==='/api/v2/skill-content-previews'&&method==='POST'){
  writes.push({method,path,body})
  const replay=previewsDb.find(p=>p.requestKey===body.requestKey)
  if(replay)data=replay
  else{
   if(dropCreate==='before'){dropCreate=null;return route.abort('failed')}
   const pin=pins.find(p=>p.id===body.targetId)
   if(!pin||pin.version!==body.expectedTargetVersion||!capReady||previewsDb.some(p=>['queued','capturing'].includes(p.state)))return route.fulfill({status:409,json:BUSY})
   const created=mkPreview(pin,body.requestKey);previewsDb.push(created);data=created
   if(dropCreate==='after'){dropCreate=null;return route.abort('failed')}
  }
 }
 else if(path.match(/^\/api\/v2\/skill-content-previews\/[^/]+\/cancel$/)&&method==='POST'){
  writes.push({method,path,body})
  const p=previewsDb.find(x=>x.id===path.split('/')[4]);if(!p)return route.fulfill({status:404,json:{error:'content preview unavailable'}})
  Object.assign(p,{state:'cancelled',updatedAt:Date.now()});data=p
 }
 else if(path.match(/^\/api\/v2\/skill-content-previews\/[^/]+$/)&&method==='GET'){
  const id=path.split('/').pop();previewReads.push(id)
  if(previewGetHold)await previewGetHold.promise
  const p=previewsDb.find(x=>x.id===id);if(!p)return route.fulfill({status:404,json:{error:'content preview unavailable'}})
  data=p
 }
 else if(path.match(/^\/api\/v2\/skill-content-previews\/[^/]+$/)&&method==='DELETE'){
  writes.push({method,path,body})
  const id=path.split('/').pop();previewsDb=previewsDb.filter(x=>x.id!==id);data={ok:true}
 }
 else{unmocked.push(`${method} ${path}`);return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})}
 // A reply that never arrives: the server has kept the write, the browser sees the connection fail.
 if(dropNextSave&&method==='POST'&&path==='/api/v2/skill-content-targets'){dropNextSave=false;return route.abort('failed')}
 await route.fulfill({json:data})
})

// Every write to browser storage is recorded from before the page loads, so use of the dialog can be shown to make none.
await page.addInitScript(()=>{
 window.__storageCalls=[]
 for(const op of ['setItem','removeItem','clear']){const original=Storage.prototype[op];Storage.prototype[op]=function(...args){window.__storageCalls.push({area:this===window.localStorage?'local':this===window.sessionStorage?'session':'other',op,key:args[0]});return original.apply(this,args)}}
})
// The page's poll timers (3 and 10 seconds) are counted while they are pending, and the clock can be moved on, so a test can
// wait for a timer to have fired or a read to have aged without sleeping.
await page.addInitScript(()=>{
 window.__poll={pending:new Set(),fired:0}
 const set=window.setTimeout.bind(window),clear=window.clearTimeout.bind(window)
 window.setTimeout=(fn,delay,...args)=>{
  if(delay!==3000&&delay!==10000)return set(fn,delay,...args)
  const id=set(()=>{window.__poll.pending.delete(id);window.__poll.fired++;fn(...args)},delay)
  window.__poll.pending.add(id);return id
 }
 window.clearTimeout=id=>{window.__poll.pending.delete(id);clear(id)}
 const real=Date.now.bind(Date);window.__skew=0;Date.now=()=>real()+window.__skew
})
const pendingPolls=()=>page.evaluate(()=>window.__poll.pending.size)
const setHidden=hidden=>page.evaluate(h=>{for(const key of ['hidden','visibilityState'])Object.defineProperty(document,key,{configurable:true,get:()=>key==='hidden'?h:(h?'hidden':'visible')});document.dispatchEvent(new Event('visibilitychange'))},hidden)
const layout=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
const storage=()=>page.evaluate(async()=>JSON.stringify({local:{...localStorage},session:{...sessionStorage},idb:(await indexedDB.databases()).map(d=>d.name).sort()}))
const card=host=>page.locator('article').filter({has:page.getByRole('heading',{name:host,exact:true})})
const scopeRow=(host,label)=>card(host).locator('li').filter({has:page.getByText(label,{exact:true})}).first()
const scopeButton=(host,label)=>scopeRow(host,label).getByRole('button',{name:/^Private skills/})
const expand=async host=>{const details=card(host).locator('details.agent-list');if(!await details.evaluate(el=>el.open))await details.locator(':scope > summary').click()}
const post=expectedVersion=>({environmentId:'scope-one',agentId:'agent-0',bundleId:B.demo,revisionId:R.d3,mode:'additive',expectedVersion})
const SHARED='Shared agents / Codex'
const stamp=ms=>page.evaluate(t=>new Date(t).toLocaleString(),ms)
// A request made now reaches the route after any made before it, so once it has, an earlier one would be in the log.
const afterRequests=()=>page.evaluate(()=>fetch('/api/version').then(r=>r.status))
const noIds=(...texts)=>{for(const text of texts)expect(text).not.toMatch(/[0-9a-f]{8}/)}

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

 const L={d1:await stamp(T.d1),d2:await stamp(T.d2),d3:await stamp(T.d3)}
 const NEWEST=`${L.d3} · Newest`

 // Opening the dialog from the scope reads and never writes. The click is made without moving focus, as Safari and Firefox
 // on a Mac do, so the dialog must hand focus back to what was pressed and not to what happened to be focused.
 // The first read of the pins fails: the failure shows and nothing else can be used.
 pinsBroken=true
 const opener=scopeButton('workstation',SHARED)
 await opener.evaluate(el=>el.click())
 const dialog=page.getByRole('dialog',{name:'Private skills on workstation'})
 await expect(dialog).toBeVisible()
 const storageCallsAtOpen=await page.evaluate(()=>window.__storageCalls.length)
 const library=dialog.getByLabel('Library skill',{exact:true}),revision=dialog.getByLabel('Revision',{exact:true}),save=dialog.getByRole('button',{name:'Save pin',exact:true})
 await expect(dialog.getByRole('alert')).toContainText('The pins could not be read right now.')
 await expect(save).toHaveCount(0)
 // Trying again shows that it is working, and once it succeeds the old failure is gone and the dialog can be used.
 pinsBroken=false;pinsHold=deferred()
 await dialog.getByRole('button',{name:'Try again'}).click()
 await expect(dialog.getByText('Loading pins and your library')).toBeVisible()
 await expect(dialog.getByRole('alert')).toHaveCount(1)
 pinsHold.resolve();pinsHold=null
 await expect(dialog.getByText('Nothing is pinned to this folder yet.')).toBeVisible()
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 await expect(dialog.getByRole('button',{name:'Try again'})).toHaveCount(0)
 await expect(dialog).not.toContainText(L.d1)
 await expect(dialog).toContainText('installs nothing and writes nothing to the machine')
 expect(writes).toEqual([])
 await expect(save).toBeDisabled()
 await expect(library).toHaveValue('')
 // Library skills are named by their name; only the two that share one carry the start of an id.
 const names=await library.evaluate(s=>[...s.options].map(o=>o.textContent.trim()))
 expect(names).toEqual(expect.arrayContaining(['demo','alpha','zeta',LONG_NAME,`twin · ${'7'.repeat(8)}`,`twin · ${'8'.repeat(8)}`]))
 expect(names.filter(n=>/[0-9a-f]{8}/.test(n))).toEqual([`twin · ${'7'.repeat(8)}`,`twin · ${'8'.repeat(8)}`])

 // Create: the newest revision is preselected, older ones stay available, and the default mode adds and updates.
 await library.selectOption(B.demo)
 await expect(revision).toHaveValue(R.d3)
 expect(await revision.evaluate(s=>[...s.options].map(o=>o.value))).toEqual([R.d3,R.d2,R.d1])
 // Revisions are named by when they were made, newest first and marked, with no id on show.
 expect(await revision.evaluate(s=>[...s.options].map(o=>o.textContent.trim()))).toEqual([NEWEST,L.d2,L.d1])
 await expect(dialog.getByRole('radio',{name:'Add and update'})).toBeChecked()
 await expect(dialog.getByRole('radio',{name:/^Also remove files reet owns that a newer revision drops/})).not.toBeChecked()
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText('demo')
 await expect(dialog.getByText(/^Sorted\./)).toContainText(L.d3)
 expect(writes).toEqual([{method:'POST',path:'/api/v2/skill-content-targets',body:post('')}])
 const pinsList=dialog.getByRole('list',{name:'Pinned in this folder'})
 await expect(pinsList.getByRole('listitem')).toHaveCount(1)
 await expect(pinsList).toContainText(NEWEST)
 await expect(pinsList).toContainText('Add and update')
 await expect(pinsList).not.toContainText('Not the newest')
 await expect(pinsList).not.toContainText(L.d1)
 noIds(await pinsList.innerText(),await dialog.locator('.live').innerText())

 // Change: the pin's current version goes with the write, here for a rollback with another mode.
 const first=mine().version
 await pinsList.getByRole('button',{name:'Change pin for demo'}).click()
 await expect(library).toHaveValue(B.demo)
 await expect(revision).toHaveValue(R.d3)
 await revision.selectOption(R.d2)
 await dialog.getByRole('radio',{name:/^Also remove files reet owns that a newer revision drops/}).check()
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText(L.d2)
 expect(writes).toHaveLength(2)
 expect(writes[1]).toEqual({method:'POST',path:'/api/v2/skill-content-targets',body:{...post(first),revisionId:R.d2,mode:'managed'}})
 await expect(pinsList).toContainText(L.d2)
 await expect(pinsList).toContainText('Also remove files reet owns that a newer revision drops')
 await expect(pinsList).toContainText('Not the newest in the library')
 noIds(await pinsList.innerText(),await dialog.locator('.live').innerText())

 // Conflict: someone else changed the pin. The write is refused, the pin is read again and the screen says what changed.
 const known=mine().version
 serverEdit({revisionId:R.d1,mode:'additive',version:'ver-elsewhere'})
 await pinsList.getByRole('button',{name:'Change pin for demo'}).click()
 await revision.selectOption(R.d3)
 await save.click()
 await expect(dialog.getByRole('alert')).toContainText('changed')
 await expect(dialog.getByRole('alert')).toContainText(L.d1)
 await expect(dialog.getByRole('alert')).toContainText('Add and update')
 await expect(dialog.getByRole('alert')).toContainText('Nothing was saved')
 noIds(await dialog.getByRole('alert').innerText())
 await expect(pinsList).toContainText(L.d1)
 expect(writes).toHaveLength(3)
 expect(writes[2].body).toEqual({...post(known),revisionId:R.d3,mode:'managed'})
 // After reading what changed, saving again sends the version that is current now, not the one that was refused.
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText(L.d3)
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
 await afterRequests()
 expect(writes).toHaveLength(5)
 expect(writes[4].body).toEqual({...post(current),revisionId:R.d2,mode:'managed'})
 hold.resolve();hold=null
 await expect(dialog.getByText(/^Sorted\./)).toContainText(L.d2)
 expect(writes).toHaveLength(5)

 // Remove asks first, says it removes intent only, then sends the current version.
 await pinsList.getByRole('button',{name:'Remove pin for demo'}).click()
 const confirm=page.getByRole('dialog',{name:'Remove the pin for demo?'})
 await expect(confirm).toBeVisible()
 await expect(confirm).toContainText('removes intent only')
 await expect(confirm).toContainText('changes nothing on the machine')
 await expect(confirm).toContainText('any ownership record for it stays as it is')
 await expect(confirm).not.toContainText(/release/i)
 await confirm.getByRole('button',{name:'Cancel'}).click()
 await expect(confirm).toHaveCount(0)
 expect(writes).toHaveLength(5)
 const removing=mine()
 await pinsList.getByRole('button',{name:'Remove pin for demo'}).click()
 await confirm.getByRole('button',{name:'Remove pin',exact:true}).click()
 await expect(dialog.getByText('Nothing is pinned to this folder yet.')).toBeVisible()
 await expect(dialog.getByRole('status').filter({hasText:'intent only'})).toBeVisible()
 await expect(dialog.getByRole('status').filter({hasText:'any ownership record for it stays as it is'})).toBeVisible()
 expect(writes).toHaveLength(6)
 expect(writes[5]).toEqual({method:'DELETE',path:`/api/v2/skill-content-targets/${removing.id}`,body:{expectedVersion:removing.version}})
 expect(pins.some(p=>p.id===removing.id)).toBe(false)

 // A save whose reply never arrives: the server kept it, so the pins are read again quietly (the plain message stays), and
 // the next save carries the version the server holds instead of claiming the skill was pinned elsewhere.
 await library.selectOption('');await library.selectOption(B.demo)
 await expect(revision).toHaveValue(R.d3)
 dropNextSave=true
 await save.click()
 await expect(dialog.getByRole('alert')).toContainText("Couldn't reach reet")
 expect(writes).toHaveLength(7)
 expect(writes[6].body).toEqual(post(''))
 await expect(pinsList.getByRole('listitem')).toHaveCount(1)
 await expect(pinsList).toContainText(NEWEST)
 await expect(save).toBeDisabled()
 const landed=mine().version
 await revision.selectOption(R.d2)
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText(L.d2)
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 expect(writes).toHaveLength(8)
 expect(writes[7].body).toEqual({...post(landed),revisionId:R.d2})

 // A removal that finds the pin changed is refused, and the pin stays.
 const created=mine().version
 serverEdit({version:'ver-moved',revisionId:R.d1})
 await pinsList.getByRole('button',{name:'Remove pin for demo'}).click()
 await confirm.getByRole('button',{name:'Remove pin',exact:true}).click()
 await expect(dialog.getByRole('alert')).toContainText('changed elsewhere')
 await expect(dialog.getByRole('alert')).toContainText(L.d1)
 await expect(dialog.getByRole('alert')).toContainText('Nothing was removed')
 await expect(pinsList.getByRole('listitem')).toHaveCount(1)
 expect(writes).toHaveLength(9)
 expect(writes[8].body).toEqual({expectedVersion:created})
 expect(mine()).toBeTruthy()

 // A save that finds the library skill gone says so in plain words (the server only says "unavailable"), and sends once.
 await pinsList.getByRole('button',{name:'Change pin for demo'}).click()
 await revision.selectOption(R.d3)
 failSave404=true
 await save.click()
 await expect(dialog.getByRole('alert')).toContainText("The library skill or revision you chose isn't there any more, so nothing was saved.")
 expect(writes).toHaveLength(10)
 failSave404=false
 // A removal that finds the pin already gone says so, and the list catches up.
 pins=pins.filter(p=>p!==mine())
 await pinsList.getByRole('button',{name:'Remove pin for demo'}).click()
 await confirm.getByRole('button',{name:'Remove pin',exact:true}).click()
 await expect(dialog.getByRole('alert')).toContainText('already removed elsewhere, so there was nothing to remove')
 await expect(dialog.getByText('Nothing is pinned to this folder yet.')).toBeVisible()
 expect(writes).toHaveLength(11)

 // Closing returns focus to the button that opened the dialog, though it was never focused by the click.
 await dialog.getByRole('button',{name:'Close private skills'}).click()
 await expect(dialog).toHaveCount(0)
 await expect(opener).toBeFocused()

 // From a reported skill row: the matching library skill sorts first and is chosen for you; the newest revision follows.
 // A pin made through another connector is not "the current pin": saving it re-points it at this machine's running connector.
 pins=[...pins,{id:'pin-old',environmentId:'scope-one',agentId:'agent-old',name:'demo',bundleId:B.demo,revisionId:R.d3,manifestDigest:'3'.repeat(64),mode:'additive',version:'ver-old',updatedAt:now,verification:'not_verified'}]
 const rowPin=scopeRow('workstation',SHARED).getByRole('button',{name:'Pin a library revision for demo'})
 await rowPin.evaluate(el=>el.click())
 await expect(dialog).toBeVisible()
 expect(await library.evaluate(s=>[...s.options].map(o=>o.value).filter(Boolean))).toEqual([B.demo,B.long,B.alpha,B.twinA,B.twinB,B.zeta])
 await expect(library).toHaveValue(B.demo)
 await expect(revision).toHaveValue(R.d3)
 await expect(pinsList).toContainText('demo')
 await expect(pinsList).toContainText(NEWEST)
 noIds(await pinsList.innerText())
 await expect(dialog).toContainText('It was made through a different connector')
 await expect(dialog).not.toContainText('This is the current pin')
 await expect(save).toBeEnabled()
 expect(writes).toHaveLength(11)
 // When the scope stops qualifying while the dialog is open (the page's own refresh sees the connector paused), saving is
 // switched off with the reason beside it, a click forced past that sends nothing, and removing a pin still works.
 connectorsPaused=true
 await expect(save).toBeDisabled({timeout:15000})
 await expect(dialog).toContainText("Pins can't be saved right now. The connector on this machine is paused.")
 await expect(pinsList.getByRole('button',{name:'Remove pin for demo'})).toBeEnabled()
 await save.evaluate(el=>{el.disabled=false;el.click()})
 await afterRequests()
 expect(writes).toHaveLength(11)
 connectorsPaused=false
 await expect(dialog).not.toContainText("Pins can't be saved right now",{timeout:15000})
 await expect(save).toBeEnabled()
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText(NEWEST)
 expect(writes).toHaveLength(12)
 expect(writes[11].body).toEqual(post('ver-old'))
 await expect(dialog).toContainText('This is the current pin for demo.')
 await expect(save).toBeDisabled()
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 // A reported skill that matches nothing in the library chooses nothing. This one is opened without moving focus too, and
 // is the one whose button hands focus back at the end (the scope lapsed above, which replaced the buttons of the row before it).
 const rowOther=scopeRow('workstation',SHARED).getByRole('button',{name:'Pin a library revision for other'})
 await rowOther.evaluate(el=>el.click())
 await expect(library).toHaveValue('')
 await expect(save).toBeDisabled()

 // A slow answer for an earlier choice must not overwrite a later one.
 alphaHold=deferred()
 await library.selectOption(B.alpha)
 await expect(dialog.getByText('Loading revisions')).toBeVisible()
 await expect(save).toBeDisabled()
 await library.selectOption(B.demo)
 await expect(revision).toHaveValue(R.d3)
 const answered=page.waitForResponse(r=>r.url().includes(`/skill-bundles/${B.alpha}/revisions`))
 alphaHold.resolve();alphaHold=null
 await answered
 // Give the page the turn it needs to act on the answer it has just received.
 await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>setTimeout(r,0))))
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
 expect(writes).toHaveLength(13)
 expect(writes[12].body).toEqual({environmentId:'scope-one',agentId:'agent-0',bundleId:B.long,revisionId:R.l1,mode:'additive',expectedVersion:''})
 noIds(await pinsList.innerText(),await dialog.locator('.live').innerText())
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
 await expect(dialog).toHaveCount(0)
 await expect(rowOther).toBeFocused()

 // ---- The Check step ----
 const POSTS='/api/v2/skill-content-previews'
 const checkPosts=()=>writes.filter(w=>w.method==='POST'&&w.path===POSTS)
 const reads=id=>previewReads.filter(r=>r===id).length
 const checkHeading=dialog.getByRole('heading',{name:'Check this machine',exact:true})
 const checkBtn=dialog.getByRole('button',{name:'Check this machine',exact:true})
 const again=dialog.getByRole('button',{name:'Check again',exact:true})
 const live=dialog.locator('.check-live')
 const heading=name=>dialog.getByRole('heading',{name,exact:true})
 const SECOND=15000
 const askAgain=async()=>{const n=checkPosts().length;await again.click();await expect.poll(()=>checkPosts().length).toBe(n+1);return checkPosts()[n].body}
 const keys=[]

 // The connector has not recently negotiated: the check is switched off with the reason, a click forced past that sends
 // nothing, and the button comes back by itself once the connector reports in.
 capReady=false
 await scopeButton('workstation',SHARED).evaluate(el=>el.click())
 await expect(dialog).toBeVisible()
 await expect(pinsList).toContainText('Last check: Not checked yet')
 await pinsList.getByRole('button',{name:'Open check for demo'}).click()
 await expect(checkHeading).toBeVisible()
 await expect(dialog).toContainText('reads file names, sizes and digests only')
 await expect(checkBtn).toBeDisabled()
 await expect(dialog).toContainText("hasn't checked in recently")
 await checkBtn.evaluate(el=>{el.disabled=false;el.click()})
 await afterRequests()
 expect(checkPosts()).toEqual([])
 capReady=true
 await expect(dialog).not.toContainText("hasn't checked in recently",{timeout:SECOND})
 await expect(checkBtn).toBeEnabled()

 // One click sends one request with exactly the three fields; a second click while it is under way sends nothing.
 const demo=mine()
 await checkBtn.evaluate(el=>{el.click();el.click()})
 await expect(live).toContainText('Waiting for the machine')
 await afterRequests()
 expect(checkPosts()).toHaveLength(1)
 const firstBody=checkPosts()[0].body
 expect(Object.keys(firstBody).sort()).toEqual(['expectedTargetVersion','requestKey','targetId'])
 expect(firstBody.requestKey).toMatch(/^[A-Za-z0-9_-]{16,80}$/)
 expect(firstBody.targetId).toBe(demo.id);expect(firstBody.expectedTargetVersion).toBe(demo.version)
 keys.push(firstBody.requestKey)
 await expect(live).toBeVisible()
 expect(await live.getAttribute('aria-live')).toBe('polite')
 expect(await pendingPolls()).toBe(1)
 // Queued, then capturing: the progress changes in the same polite live region.
 setLatest({state:'capturing'})
 await expect(live).toContainText('Reading the folder on this machine',{timeout:SECOND})
 expect(await pendingPolls()).toBe(1)

 // Captured, folder found, exclusive root: what was found (managed and not) and the plan in words, read-only.
 const found=(over={})=>({state:'captured',capturedAt:Date.now(),reviewHash:'h'.repeat(64),captureHash:'c'.repeat(64),capture:captureOf('present',{manifest:manifestOf([file('SKILL.md',120),file('notes/mine.txt',12)])}),plan:planOf({preconditions:{baseline:{manifest:OWNED}},actions:[{path:'SKILL.md',kind:'update',before:file('SKILL.md',120),after:file('SKILL.md',130)}],preserved:[{path:'notes/mine.txt',reason:'local_only'}]}),...over})
 setLatest(found())
 await expect(heading('Found')).toBeVisible({timeout:SECOND})
 const files=dialog.getByRole('table',{name:'Files found in the folder'})
 await expect(files.getByRole('row',{name:/SKILL\.md/})).toContainText('Managed by reet')
 await expect(files.getByRole('row',{name:/notes\/mine\.txt/})).toContainText('Not managed by reet')
 await expect(dialog.getByRole('table',{name:'What a later apply would change'}).getByRole('row',{name:/SKILL\.md/})).toContainText('Would update')
 await expect(dialog).toContainText("Applying changes isn't available yet.")
 await expect(dialog).toContainText("Stays yours: this file isn't in the pinned revision")
 await expect(dialog).not.toContainText('Another agent scope')
 await expect(dialog.getByRole('button',{name:/apply|install|adopt|record ownership/i})).toHaveCount(0)
 await expect(dialog).not.toContainText(/verified/i)
 expect(await pendingPolls()).toBe(1)
 // A found folder that the plan says is up to date.
 keys.push((await askAgain()).requestKey)
 setLatest(found({plan:planOf({preconditions:{baseline:{manifest:OWNED}}})}))
 await expect(dialog.getByText('Up to date.',{exact:false})).toBeVisible({timeout:SECOND})
 await expect(dialog.getByText("Applying changes isn't available yet.")).toHaveCount(0)
 // Not there yet: the create actions are shown read-only.
 keys.push((await askAgain()).requestKey)
 setLatest({state:'captured',capturedAt:Date.now(),reviewHash:'h'.repeat(64),capture:captureOf('absent'),plan:planOf({actions:[{path:'SKILL.md',kind:'create',after:file('SKILL.md',130)},{path:'scripts/run.sh',kind:'create',after:file('scripts/run.sh',40,true)}]})})
 await expect(heading('Not there yet')).toBeVisible({timeout:SECOND})
 await expect(dialog.getByRole('table',{name:'What a later apply would change'}).getByRole('row',{name:/scripts\/run\.sh/})).toContainText('Would create')
 await expect(dialog).toContainText("Applying changes isn't available yet.")
 await expect(dialog.getByRole('table',{name:'Files found in the folder'})).toHaveCount(0)
 await expect(dialog.getByRole('button',{name:/apply|install|create /i})).toHaveCount(0)
 // Couldn't tell: the issues are listed.
 keys.push((await askAgain()).requestKey)
 setLatest({state:'captured',capturedAt:Date.now(),reviewHash:'h'.repeat(64),capture:captureOf('unknown',{issues:[{path:'',code:'scope_root_missing'}]}),plan:planOf({ready:false,conflicts:[{path:'',code:'observation_unknown'}]})})
 await expect(heading("Couldn't tell")).toBeVisible({timeout:SECOND})
 await expect(dialog).toContainText('scope root missing')
 await expect(dialog).toContainText("The folder couldn't be read completely.")
 await expect(dialog).not.toContainText('Up to date')
 // A root another agent scope reaches: the folder warning once, and no actions at all.
 keys.push((await askAgain()).requestKey)
 setLatest({state:'captured',capturedAt:Date.now(),reviewHash:'h'.repeat(64),capture:captureOf('present',{rootOwnership:'shared',manifest:manifestOf([file('SKILL.md',120)])}),plan:planOf({ready:false,conflicts:[{path:'',code:'root_shared'}]})})
 await expect(heading('Found')).toBeVisible({timeout:SECOND})
 await expect(dialog.getByText('Another agent scope, or a link, reaches this same folder, so reet will not touch it.')).toHaveCount(1)
 await expect(dialog.getByText("Applying changes isn't available yet.")).toHaveCount(0)
 await expect(dialog.getByRole('table')).toHaveCount(0)
 // The same for a root whose ownership could not be told.
 keys.push((await askAgain()).requestKey)
 setLatest({state:'captured',capturedAt:Date.now(),reviewHash:'h'.repeat(64),capture:captureOf('present',{rootOwnership:'unknown'}),plan:planOf({ready:false,conflicts:[{path:'',code:'root_unknown'}]})})
 await expect(dialog.getByText("Couldn't tell whether another agent scope reaches this folder, so reet will not touch it.")).toHaveCount(1,{timeout:SECOND})
 await expect(dialog.getByRole('table')).toHaveCount(0)

 // Anything that is not a current capture shows its reason and Check again, never a verdict from its plan.
 const withPlan={capture:found().capture,plan:found().plan,reviewHash:'h'.repeat(64)}
 const ended=async(over,expectText,notFinal=false)=>{
  keys.push((await askAgain()).requestKey)
  setLatest(over)
  for(const text of expectText)await expect(dialog.getByText(text,{exact:false}).first()).toBeVisible({timeout:SECOND})
  await expect(again).toBeVisible()
  for(const verdict of [heading('Found'),heading('Not there yet'),dialog.getByText('Up to date'),dialog.getByText("Applying changes isn't available yet."),dialog.getByRole('table')])await expect(verdict).toHaveCount(0)
  if(!notFinal)expect(await pendingPolls()).toBe(0)
 }
 await ended({state:'stale',reasons:['stale'],...withPlan},['This check is out of date.','more than five minutes old'])
 await ended({state:'superseded',reasons:['pin_changed','receipt_changed'],...withPlan},['The pin changed after this check was made.','The ownership record for this skill changed after this check was made.'])
 await ended({state:'captured',reasons:['inventory_changed'],...withPlan},['The machine reported a newer scan after this check was made.'],true)
 await ended({state:'expired',reasons:['expired']},['This check ran out of time.'])
 await ended({state:'failed',error:'capture_failed'},["That check didn't finish.","The machine couldn't read this skill folder."])
 await ended({state:'failed',error:'plan_unavailable'},["couldn't be worked out"])
 await ended({state:'cancelled'},['This check was stopped.'])
 await ended({state:'frobnicated'},['Something changed. Check again.'])
 expect(new Set(keys).size).toBe(keys.length)

 // The pin is read again before a check is made: one that moved elsewhere is not checked, and the screen says so.
 const posts=checkPosts().length
 serverEdit({version:'ver-moved-on',revisionId:R.d1})
 await again.click()
 await expect(dialog.getByRole('alert')).toContainText('changed')
 await afterRequests()
 expect(checkPosts()).toHaveLength(posts)
 await again.click()
 await expect.poll(()=>checkPosts().length).toBe(posts+1)
 expect(checkPosts()[posts].body.expectedTargetVersion).toBe('ver-moved-on')
 keys.push(checkPosts()[posts].body.requestKey)
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 await expect(live).toContainText('Waiting for the machine')

 // Polling pauses while the tab is hidden: a timer that fires then reads nothing, and coming back reads at once.
 const queued=latest().id
 await setHidden(true)
 await afterRequests()
 const readsBefore=reads(queued),firedBefore=await page.evaluate(()=>window.__poll.fired)
 await page.waitForFunction(n=>window.__poll.fired>n,firedBefore,{timeout:SECOND})
 await afterRequests()
 expect(reads(queued)).toBe(readsBefore)
 expect(await pendingPolls()).toBe(0)
 setLatest({state:'capturing'})
 await setHidden(false)
 await expect(live).toContainText('Reading the folder on this machine',{timeout:SECOND})
 expect(reads(queued)).toBe(readsBefore+1)
 expect(await pendingPolls()).toBe(1)

 // Closing the dialog stops polling: nothing is pending and nothing reads afterwards.
 const readsAtClose=reads(queued)
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 expect(await pendingPolls()).toBe(0)
 await afterRequests()
 expect(reads(queued)).toBe(readsAtClose)

 // A captured row in the list has no capture, so it is not shown until the check has been read in full.
 setLatest(found())
 previewGetHold=deferred()
 await scopeButton('workstation',SHARED).evaluate(el=>el.click())
 await expect(dialog).toBeVisible()
 await expect(pinsList).toContainText('Last check: Checked')
 await pinsList.getByRole('button',{name:'Open check for demo'}).click()
 await expect(dialog.getByText('Reading the latest check')).toBeVisible()
 await expect(heading('Found')).toHaveCount(0)
 previewGetHold.resolve();previewGetHold=null
 await expect(heading('Found')).toBeVisible()
 await expect(files).toBeVisible()

 // A captured read the browser has not renewed for thirty seconds is not shown as current. The clock is moved on while the
 // tab is hidden (so nothing reads again), then put back and the read renewed.
 await setHidden(true)
 await page.evaluate(()=>{window.__skew=60000})
 await expect(dialog.getByText('Updating this result')).toBeVisible({timeout:SECOND})
 await expect(heading('Found')).toHaveCount(0)
 await page.evaluate(()=>{window.__skew=0})
 await setHidden(false)
 await expect(heading('Found')).toBeVisible({timeout:SECOND})

 // Stop and dismiss use the server's cancel and dismiss, and the dialog says what was done.
 keys.push((await askAgain()).requestKey)
 await dialog.getByRole('button',{name:'Stop this check'}).click()
 await expect(live).toContainText('This check was stopped.')
 const stopped=latest().id
 expect(writes.at(-1)).toEqual({method:'POST',path:`${POSTS}/${stopped}/cancel`,body:{}})
 const dismiss=dialog.getByRole('button',{name:'Dismiss this result'})
 await dismiss.click()
 await expect.poll(()=>previewsDb.some(p=>p.id===stopped)).toBe(false)
 expect(writes.at(-1)).toEqual({method:'DELETE',path:`${POSTS}/${stopped}`,body:null})
 await expect(live).not.toContainText('This check was stopped.')
 // The one before it is shown next; dismissing them all leaves nothing to show.
 while(previewsDb.length){const n=previewsDb.length;await dismiss.click();await expect.poll(()=>previewsDb.length).toBe(n-1)}
 await expect(dialog.getByText('Nothing has been checked yet.')).toBeVisible()
 await expect(checkBtn).toBeEnabled()
 expect(await pendingPolls()).toBe(0)

 // A reply that never arrives, the check was made, and the list can be read: the check is found and no second one is sent.
 dropCreate='after'
 const beforeLost=checkPosts().length
 await checkBtn.click()
 await expect(live).toContainText('Waiting for the machine')
 expect(checkPosts()).toHaveLength(beforeLost+1)
 expect(previewsDb.filter(p=>p.requestKey===checkPosts()[beforeLost].body.requestKey)).toHaveLength(1)
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 keys.push(checkPosts()[beforeLost].body.requestKey)
 setLatest(found({state:'failed',capture:undefined,plan:undefined,error:'capture_failed'}))
 await expect(again).toBeVisible({timeout:SECOND})
 // A reply that never arrives and the list cannot be read either: the same key is sent again and the server's own
 // check is returned, so there is still only one.
 dropCreate='after';previewsBroken=true
 const sentLost=checkPosts().length,dbBefore=previewsDb.length
 await again.click()
 await expect(dialog.getByRole('alert')).toContainText("Couldn't reach reet")
 expect(checkPosts()).toHaveLength(sentLost+1)
 const lostKey=checkPosts()[sentLost].body.requestKey
 previewsBroken=false
 await again.click()
 await expect.poll(()=>checkPosts().length).toBe(sentLost+2)
 expect(checkPosts()[sentLost+1].body.requestKey).toBe(lostKey)
 expect(previewsDb).toHaveLength(dbBefore+1)
 await expect(live).toContainText('Waiting for the machine')
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 keys.push(lostKey)
 setLatest({state:'cancelled'})
 await expect(again).toBeVisible({timeout:SECOND})
 // A reply that never arrives and a real busy conflict: the same key is sent again, the machine answers busy, and the other
 // check can be resumed (not adopted silently).
 dropCreate='before';previewsBroken=true
 const sentBusy=checkPosts().length
 await again.click()
 await expect(dialog.getByRole('alert')).toContainText("Couldn't reach reet")
 const busyKey=checkPosts()[sentBusy].body.requestKey
 previewsBroken=false
 const elsewhere=mkPreview(mine(),'made-elsewhere-0123456789');previewsDb.push(elsewhere)
 await again.click()
 await expect.poll(()=>checkPosts().length).toBe(sentBusy+2)
 expect(checkPosts()[sentBusy+1].body.requestKey).toBe(busyKey)
 await expect(dialog.getByRole('alert')).toContainText('busy')
 const resume=dialog.getByRole('button',{name:'Resume that check',exact:true})
 await expect(resume).toBeVisible()
 await resume.click()
 await expect(live).toContainText('Waiting for the machine')
 await expect(resume).toHaveCount(0)
 expect(checkPosts()).toHaveLength(sentBusy+2)
 setLatest(found())
 await expect(heading('Found')).toBeVisible({timeout:SECOND})

 // Layout: the result fits at every width, light and dark, with a long list of files inside its own scroll area.
 const many=Array.from({length:60},(_,i)=>file(`references/${'a-very-long-unbroken-file-name-'.repeat(3)}${i}-ünïcödé.md`,100+i))
 await again.click()
 await expect.poll(()=>checkPosts().length).toBe(sentBusy+3)
 setLatest(found({capture:captureOf('present',{manifest:manifestOf([file('SKILL.md',120),...many])})}))
 await expect(heading('Found')).toBeVisible({timeout:SECOND})
 for(const [width,height] of [[1360,1000],[390,844],[320,640]])for(const scheme of ['dark','light']){
  await page.setViewportSize({width,height});await page.emulateMedia({colorScheme:scheme})
  await layout();expect(await fits()).toBe(true)
  await expect(dialog.getByRole('button',{name:'Close private skills'})).toBeInViewport()
  await expect(dialog.getByRole('button',{name:'Done'})).toBeInViewport()
 }
 await page.setViewportSize({width:1360,height:1000});await page.emulateMedia({colorScheme:'light'})
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 expect(await pendingPolls()).toBe(0)

 // Pins, checks and ownership never touch browser storage: not one write, from the moment the dialog first opened, and the
 // stored state is what it was. Nothing ever asked for revision bytes or any other read of a skill's files.
 expect((await page.evaluate(()=>window.__storageCalls)).slice(storageCallsAtOpen)).toEqual([])
 expect(await storage()).toBe(before)
 expect(requests.filter(r=>/\/revisions\/[^/]/.test(r.path))).toEqual([])
 // Receipts belong to a later step, and no check can exist before a pin does: the checks are first read after the first pin was saved.
 expect(requests.filter(r=>/skill-content-receipts/.test(r.path))).toEqual([])
 const firstPinSave=requests.findIndex(r=>r.method==='POST'&&r.path==='/api/v2/skill-content-targets'),firstCheckRead=requests.findIndex(r=>r.method==='GET'&&r.path==='/api/v2/skill-content-previews')
 expect(firstPinSave).toBeGreaterThan(-1)
 expect(firstCheckRead).toBeGreaterThan(firstPinSave)

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
 console.log('Skill content UI passed: entry points and reasons, read-only open and retry, create/change/remove with versions, conflicts, lost replies and 404s, busy guard, connector lapse, newest revision, labels not ids, stale-response fencing, focus return, storage writes, responsive layouts and viewer/legacy isolation; checks: readiness, request body and key reuse, queued to captured polling, every presence and ownership result, stale/superseded/expired/failed/cancelled/unknown states, pin re-read, lost replies and busy resume, hidden pause, close stops polling, summary rows read in full, aged reads, stop and dismiss.')
}finally{await browser.close()}
