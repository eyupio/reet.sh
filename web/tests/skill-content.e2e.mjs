import { chromium, expect } from '@playwright/test'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const page=await browser.newPage({viewport:{width:1360,height:1000}})
const now=Date.now(),DAY=86400000,errors=[],writes=[],requests=[],unmocked=[]
page.on('pageerror',e=>errors.push(e.message))
const abandoned=[]
page.on('requestfailed',r=>{if(/skill-content-previews\/chk/.test(r.url()))abandoned.push(r.url())})
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

// Ownership. The server records an adoption only for a captured check with no reasons whose review hash is the one sent, for a
// candidate revision, in a slot with no record; the check it uses is consumed (superseded, receipt_changed). A repeated key is
// answered with the first record. Releasing needs the record's current version (internal/store/store_content_receipts.go).
const SHARED_409='This ownership record is unavailable, blocked, changed or already exists. Preview again, and release existing ownership before adopting again.'
let receiptsDb=[],receiptKeys=new Map(),receiptSeq=0,receiptsBroken=false,adoptHold=null,adoptRefuse=false,dropAdopt=null,releaseHold=null
let breakdownHold={},adoptFail=0,releaseRefuse=false,previewGetFail=0,itemMissing='',receiptsFail=0,listFail=0
const breakdownReads=[]
const bdFile=(path,status,bytes=10,executable=false)=>({path,bytes,digest:'a'.repeat(64),executable,status})
const BIG_PATH=`deep/${'ünïcödé-segment-without-any-break-'.repeat(8)}last.md`
const breakdowns={
 // One path that differs between the machine and the library arrives twice, as the server sends it.
 [R.d1]:()=>({revisionId:R.d1,files:[bdFile('SKILL.md','claimed',120),bdFile('extra.md','library_only'),bdFile('notes/mine.txt','stays_yours',12),bdFile('scripts/run.sh','stays_yours',41),bdFile('scripts/run.sh','library_only',40,true)],matched:1,observedOnly:2,revisionOnly:2}),
 [R.d2]:()=>({revisionId:R.d2,files:[bdFile('SKILL.md','claimed',120),bdFile('notes/mine.txt','claimed',12),bdFile('scripts/run.sh','claimed',40,true)],matched:3,observedOnly:0,revisionOnly:0}),
 [R.d3]:()=>({revisionId:R.d3,files:[bdFile('SKILL.md','claimed',120),...Array.from({length:254},(_,i)=>bdFile(`local/file-${String(i).padStart(3,'0')}.txt`,'stays_yours',100+i)),bdFile(BIG_PATH,'stays_yours',9)],matched:1,observedOnly:255,revisionOnly:0}),
}

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
 // Machines reads the server's terminal status for a writer in accounts mode: the first-terminal checklist moved here from
 // Control when Control was folded into Home and Machines (R4, PR H). A viewer and the legacy modes never read it (checked
 // at the end of this file).
 else if(path==='/api/v2/terminal/status')data={configured:false,available:false,reason:'disabled'}
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
  if(listFail)return route.fulfill({status:listFail,json:{error:'missing or wrong token'}})
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
  if(itemMissing===id)return route.fulfill({status:404,json:{error:'content preview unavailable'}})
  if(previewGetFail)return route.fulfill({status:previewGetFail,json:{error:previewGetFail===401?'missing or wrong token':'The checks could not be read right now.'}})
  if(previewGetHold)await previewGetHold.promise
  const p=previewsDb.find(x=>x.id===id);if(!p)return route.fulfill({status:404,json:{error:'content preview unavailable'}})
  // The browser may have abandoned this request while it was held.
  return route.fulfill({json:p}).catch(()=>{})
 }
 else if(path.match(/^\/api\/v2\/skill-content-previews\/[^/]+$/)&&method==='DELETE'){
  writes.push({method,path,body})
  const id=path.split('/').pop();previewsDb=previewsDb.filter(x=>x.id!==id);data={ok:true}
 }
 else if(path==='/api/v2/skill-content-receipts'&&method==='GET'){
  if(receiptsBroken||receiptsFail>0){if(receiptsFail>0)receiptsFail--;return route.fulfill({status:500,json:{error:'The ownership records could not be read right now.'}})}
  data={receipts:receiptsDb}
 }
 else if(path==='/api/v2/skill-content-receipts'&&method==='POST'){
  writes.push({method,path,body})
  if(dropAdopt==='before'){dropAdopt=null;return route.abort('failed')}
  if(adoptFail)return route.fulfill({status:adoptFail,json:{error:'Private skill unavailable.'}})
  if(adoptHold)await adoptHold.promise
  const replayed=receiptsDb.find(r=>receiptKeys.get(r.id)===body.requestKey)
  const p=previewsDb.find(x=>x.id===body.previewId)
  if(replayed&&p&&replayed.evidence.previewId===body.previewId)data=replayed
  else{
   const ok=!adoptRefuse&&p&&p.state==='captured'&&!(p.reasons??[]).length&&p.reviewHash===body.reviewHash&&p.adoption?.candidates.some(c=>c.revisionId===body.adoptAsRevisionId)&&!receiptsDb.some(r=>r.name===p.name&&r.environmentId===p.environmentId)&&!replayed
   if(!ok)return route.fulfill({status:409,json:{error:SHARED_409}})
   const claimed=breakdowns[body.adoptAsRevisionId]().files.filter(f=>f.status==='claimed').length
   const r={id:`rcp${String(++receiptSeq).padStart(4,'0')}`,environmentId:p.environmentId,agentId:p.agentId,name:p.name,bundleId:p.bundleId,revisionId:body.adoptAsRevisionId,managedDigest:'m'.repeat(64),fileCount:claimed,provenance:'adopted',verification:'controller_attested',evidence:{previewId:p.id,reviewHash:p.reviewHash,captureHash:'c'.repeat(64),observedDigest:'o'.repeat(64),pinId:p.targetId,pinVersion:p.targetVersion,capturedAt:Date.now(),adoptedAt:Date.now(),adoptedBy:1},version:`rv-${++receiptSeq}`,createdAt:Date.now(),updatedAt:Date.now()}
   receiptsDb.push(r);receiptKeys.set(r.id,body.requestKey)
   Object.assign(p,{state:'superseded',reasons:['receipt_changed'],updatedAt:Date.now()})
   data=r
   if(dropAdopt==='after'){dropAdopt=null;return route.abort('failed')}
  }
 }
 else if(path.match(/^\/api\/v2\/skill-content-receipts\/[^/]+$/)&&method==='DELETE'){
  writes.push({method,path,body})
  if(releaseHold)await releaseHold.promise
  if(releaseRefuse)return route.fulfill({status:409,json:{error:SHARED_409}})
  const r=receiptsDb.find(x=>x.id===decodeURIComponent(path.split('/').pop()))
  if(!r)return route.fulfill({status:404,json:{error:'Ownership record or preview unavailable.'}})
  if(r.version!==body?.expectedVersion)return route.fulfill({status:409,json:{error:SHARED_409}})
  receiptsDb=receiptsDb.filter(x=>x!==r);data={removed:true}
 }
 else if(path.match(/^\/api\/v2\/skill-content-previews\/[^/]+\/adoption\/[^/]+$/)&&method==='GET'){
  const parts=path.split('/'),id=parts[4],rev=parts[6]
  breakdownReads.push({id,rev})
  if(breakdownHold[rev])await breakdownHold[rev].promise
  const p=previewsDb.find(x=>x.id===id)
  if(!p||p.state!=='captured'||(p.reasons??[]).length||!breakdowns[rev])return route.fulfill({status:409,json:{error:SHARED_409}})
  return route.fulfill({json:breakdowns[rev]()}).catch(()=>{})
 }
 else if(path==='/api/v2/folder-projects')data={projects:[],savedWaiting:0}
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
 window.__poll={pending:new Set(),fired:0,scheduled:0,delays:new Set()}
 const set=window.setTimeout.bind(window),clear=window.clearTimeout.bind(window)
 // The page asks for 3 or 10 seconds; the delay it asked for is recorded and the timer fires after 50 ms, so the file runs in seconds.
 window.setTimeout=(fn,delay,...args)=>{
  if(delay!==3000&&delay!==10000)return set(fn,delay,...args)
  window.__poll.scheduled++;window.__poll.delays.add(delay)
  const id=set(()=>{window.__poll.pending.delete(id);window.__poll.fired++;fn(...args)},50)
  window.__poll.pending.add(id);return id
 }
 window.clearTimeout=id=>{window.__poll.pending.delete(id);clear(id)}
 const real=Date.now.bind(Date);window.__skew=0;Date.now=()=>real()+window.__skew
})
const pendingPolls=()=>page.evaluate(()=>window.__poll.pending.size)
// Nothing is polling: after a few more turns of the same timers there are none pending and none were asked for. The turns are
// four timers of the kind the page polls with (they fire at the same speed), counted and taken off the total.
const quiet=async()=>{
 await page.evaluate(()=>new Promise(done=>{let n=0;const turn=()=>++n<4?setTimeout(turn,3000):done();setTimeout(turn,3000)}))
 await afterRequests()
 const before=await page.evaluate(()=>window.__poll.scheduled)
 await page.evaluate(()=>new Promise(done=>{let n=0;const turn=()=>++n<4?setTimeout(turn,3000):done();setTimeout(turn,3000)}))
 await afterRequests()
 expect(await page.evaluate(()=>window.__poll.scheduled)).toBe(before+4)
 expect(await pendingPolls()).toBe(0)
}
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
// A read that was already under way when the tab was hidden has landed: it would otherwise be stamped with the moved clock.
const settleReads=async()=>{await afterRequests();await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>setTimeout(r,0))))}
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

 // The checks cannot be read when the dialog opens: the pins and the library still work, a plain note offers a retry, and
 // checking is switched off with that reason until the retry succeeds.
 previewsBroken=true
 await scopeButton('workstation',SHARED).evaluate(el=>el.click())
 await expect(dialog).toBeVisible()
 const note=dialog.getByRole('status').filter({hasText:"Checks aren't available right now."})
 await expect(note).toBeVisible()
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 await expect(pinsList.getByRole('button',{name:'Change pin for demo'})).toBeVisible()
 const savesBefore=writes.length
 await pinsList.getByRole('button',{name:'Change pin for demo'}).click()
 await revision.selectOption(R.d2)
 await save.click()
 await expect(dialog.getByText(/^Sorted\./)).toContainText(L.d2)
 expect(writes).toHaveLength(savesBefore+1)
 await expect(note).toBeVisible()
 // Nothing can be said about the last check while the checks cannot be read.
 await expect(pinsList).toContainText('Last check: Unavailable')
 await expect(pinsList).not.toContainText('Not checked yet')
 await pinsList.getByRole('button',{name:'Open check for demo'}).click()
 await expect(dialog.getByRole('button',{name:'Check this machine',exact:true})).toBeDisabled()
 const whyId=await dialog.getByRole('button',{name:'Check this machine',exact:true}).getAttribute('aria-describedby')
 await expect(page.locator(`#${whyId}`)).toContainText("Checks aren't available right now.")
 await dialog.getByRole('button',{name:'Back to pins',exact:true}).click()
 previewsBroken=false
 await note.getByRole('button',{name:'Try again'}).click()
 await expect(note).toHaveCount(0)
 await expect(pinsList).toContainText('Last check: Not checked yet')
 // The same dialog can check now, without closing it first.
 await pinsList.getByRole('button',{name:'Open check for demo'}).click()
 await expect(dialog.getByRole('button',{name:'Check this machine',exact:true})).toBeEnabled()
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 // The checks come back by themselves: while the Check step is open and the connector has not reported in, the list is read
 // now and then, and the note and the disabled button go once it can be read.
 previewsBroken=true
 await scopeButton('workstation',SHARED).evaluate(el=>el.click())
 await expect(note).toBeVisible()
 await pinsList.getByRole('button',{name:'Open check for demo'}).click()
 await expect(dialog.getByRole('button',{name:'Check this machine',exact:true})).toBeDisabled()
 previewsBroken=false
 await expect(note).toHaveCount(0,{timeout:15000})
 await expect(dialog.getByRole('button',{name:'Check this machine',exact:true})).toBeEnabled()
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)

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
 await expect(page.locator(`#${await checkBtn.getAttribute('aria-describedby')}`)).toContainText("hasn't checked in recently")
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
 await expect.poll(pendingPolls).toBe(1)
 // Queued, then capturing: the progress changes in the same polite live region.
 setLatest({state:'capturing'})
 await expect(live).toContainText('Reading the folder on this machine',{timeout:SECOND})
 await expect.poll(pendingPolls).toBe(1)

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
 await expect.poll(pendingPolls).toBe(1)
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
 await expect(dialog).toContainText('The connector reported a problem reading this folder.')
 await expect(dialog).not.toContainText('scope')
 await expect(dialog).toContainText("The folder couldn't be read completely.")
 await expect(dialog).not.toContainText('Up to date')
 // A folder that could not be read is never Up to date, even when the plan has nothing to do.
 keys.push((await askAgain()).requestKey)
 setLatest({state:'captured',capturedAt:Date.now(),reviewHash:'h'.repeat(64),capture:captureOf('unknown',{issues:[{path:'',code:'skill_unavailable'}]}),plan:planOf()})
 await expect(heading("Couldn't tell")).toBeVisible({timeout:SECOND})
 await expect(dialog.getByText('Up to date')).toHaveCount(0)
 // Without a plan nothing is said about who manages which file.
 keys.push((await askAgain()).requestKey)
 setLatest({state:'captured',capturedAt:Date.now(),reviewHash:'h'.repeat(64),capture:captureOf('present',{manifest:manifestOf([file('SKILL.md',120)])})})
 await expect(heading('Found')).toBeVisible({timeout:SECOND})
 await expect(files.getByRole('row',{name:/SKILL\.md/})).toContainText('Not known')
 await expect(dialog.getByText('Not managed by reet')).toHaveCount(0)
 await expect(dialog.getByText('Up to date')).toHaveCount(0)
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
  if(!notFinal)await quiet()
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
 await expect.poll(pendingPolls).toBe(0)
 await afterRequests()
 const readsBefore=reads(queued)
 await quiet()
 expect(reads(queued)).toBe(readsBefore)
 setLatest({state:'capturing'})
 await setHidden(false)
 await expect(live).toContainText('Reading the folder on this machine',{timeout:SECOND})
 expect(reads(queued)).toBeGreaterThan(readsBefore)
 await expect.poll(pendingPolls).toBe(1)

 // Closing the dialog stops polling: nothing is pending and nothing reads afterwards.
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 await afterRequests()
 const readsAtClose=reads(queued)
 await quiet()
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
 await settleReads()
 await page.evaluate(()=>{window.__skew=60000})
 await expect(dialog.getByText('Updating this result')).toBeVisible({timeout:SECOND})
 await expect(heading('Found')).toHaveCount(0)
 await page.evaluate(()=>{window.__skew=0})
 await setHidden(false)
 await expect(heading('Found')).toBeVisible({timeout:SECOND})

 // Going back to the pins stops the watcher: a read that is still under way is abandoned, nothing polls any more, and the
 // answer to it, which arrives late, changes nothing and starts nothing.
 const watched=latest().id
 const readsBeforeHold=reads(watched)
 previewGetHold=deferred()
 await expect.poll(()=>reads(watched)).toBeGreaterThan(readsBeforeHold)
 const abandonedBefore=abandoned.length
 await dialog.getByRole('button',{name:'Back to pins',exact:true}).click()
 await expect(pinsList).toBeVisible()
 await expect.poll(()=>abandoned.length).toBeGreaterThan(abandonedBefore)
 setLatest({state:'cancelled'})
 previewGetHold.resolve();previewGetHold=null
 await afterRequests()
 const readsAfterBack=reads(watched)
 await quiet()
 expect(reads(watched)).toBe(readsAfterBack)
 await expect(pinsList).toContainText('Last check: Checked')
 await expect(pinsList).not.toContainText('Last check: Stopped')
 // Opening the check again reads it, because what is held is not final.
 await pinsList.getByRole('button',{name:'Open check for demo'}).click()
 await expect(live).toContainText('This check was stopped.',{timeout:SECOND})
 expect(reads(watched)).toBeGreaterThan(readsAfterBack)
 await quiet()

 // Stop and dismiss use the server's cancel and dismiss, and the dialog says what was done.
 keys.push((await askAgain()).requestKey)
 await dialog.getByRole('button',{name:'Stop this check'}).click()
 await expect(live).toContainText('This check was stopped.')
 await expect(checkHeading).toBeFocused()
 const stopped=latest().id
 expect(writes.at(-1)).toEqual({method:'POST',path:`${POSTS}/${stopped}/cancel`,body:{}})
 const dismiss=dialog.getByRole('button',{name:'Dismiss this result'})
 await dismiss.click()
 await expect.poll(()=>previewsDb.some(p=>p.id===stopped)).toBe(false)
 await expect(checkHeading).toBeFocused()
 expect(writes.at(-1)).toEqual({method:'DELETE',path:`${POSTS}/${stopped}`,body:null})
 // The one before it is shown next; dismissing them all leaves nothing to show.
 while(previewsDb.length){const n=previewsDb.length;await dismiss.click();await expect.poll(()=>previewsDb.length).toBe(n-1)}
 await expect(dialog.getByText('Nothing has been checked yet.')).toBeVisible()
 await expect(checkBtn).toBeEnabled()
 await quiet()

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
 await expect(dialog.getByRole('alert')).toHaveText('This machine is already checking demo.')
 const resume=dialog.getByRole('button',{name:'Resume the check for demo',exact:true})
 await expect(resume).toBeVisible()
 await resume.click()
 await expect(checkHeading).toBeFocused()
 await expect(live).toContainText('Waiting for the machine')
 await expect(resume).toHaveCount(0)
 expect(checkPosts()).toHaveLength(sentBusy+2)
 setLatest(found())
 await expect(heading('Found')).toBeVisible({timeout:SECOND})

 // A check made for an older version of the pin is not trusted on the server's word: it reads as out of date, names the
 // revision it was made for, shows no result, and offers Check again, though the server still says captured with no reasons.
 setLatest({state:'captured',targetVersion:'ver-before-the-pin-moved',revisionId:R.d3,reasons:[],...found()})
 await expect(dialog.getByText('The pin changed after this check was made.')).toBeVisible({timeout:SECOND})
 await expect(dialog.getByText('This check was made for revision')).toBeVisible()
 await expect(heading('Found')).toHaveCount(0)
 await expect(dialog.getByRole('table')).toHaveCount(0)
 await expect(again).toBeVisible()
 await quiet()

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
 // Plain text tapped in a long result: the clock asking again each second takes neither focus nor the scroll position.
 await page.setViewportSize({width:390,height:600})
 {
  const body=dialog.locator('.body')
  await body.evaluate(el=>{el.scrollTop=400})
  const scrolled=await body.evaluate(el=>el.scrollTop)
  expect(scrolled).toBeGreaterThan(100)
  await page.evaluate(()=>document.activeElement?.blur())
  const box=await body.boundingBox()
  await page.mouse.click(box.x+4,box.y+box.height-4)
  expect(await page.evaluate(()=>document.activeElement?.tagName)).not.toBe('H3')
  await page.waitForTimeout(2600)
  expect(await body.evaluate(el=>el.scrollTop)).toBe(scrolled)
  expect(await page.evaluate(()=>document.activeElement?.tagName)).not.toBe('H3')
 }
 await page.setViewportSize({width:1360,height:1000})
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 await quiet()

 // The list can keep a check that the item read refuses: it is read once more at most, not over and over, and the person is told.
 {
  const missing=latest().id,before=reads(missing)
  itemMissing=missing
  await scopeButton('workstation',SHARED).evaluate(el=>el.click())
  await expect(dialog).toBeVisible()
  await pinsList.getByRole('button',{name:'Open check for demo'}).click()
  await expect(dialog.getByRole('alert')).toHaveText('Something changed. Check again.')
  await expect(dialog.getByText('Reading the latest check')).toHaveCount(0)
  await expect(dialog.getByRole('button',{name:/^Check (this machine|again)$/})).toBeVisible()
  await page.waitForTimeout(1500)
  expect(reads(missing)-before).toBeLessThanOrEqual(2)
  itemMissing=''
  // A pin row says "Out of date" for a check made for an older version of the pin, as the Check step does.
  await dialog.getByRole('button',{name:'Back to pins',exact:true}).click()
  await dialog.getByRole('button',{name:'Done'}).click()
  await expect(dialog).toHaveCount(0)
  setLatest({state:'captured',reasons:[],targetVersion:'ver-before-the-pin-moved'})
  await scopeButton('workstation',SHARED).evaluate(el=>el.click())
  await expect(dialog).toBeVisible()
  await expect(pinsList).toContainText('Last check: Out of date')
  await expect(pinsList).not.toContainText('Last check: Checked')
  await dialog.getByRole('button',{name:'Done'}).click()
  await expect(dialog).toHaveCount(0)
 }
 await quiet()

 // ---- The Adopt and Release steps ----
 const RECEIPTS='/api/v2/skill-content-receipts'
 const receiptPosts=()=>writes.filter(w=>w.method==='POST'&&w.path===RECEIPTS)
 const receiptDeletes=()=>writes.filter(w=>w.method==='DELETE'&&w.path.startsWith(`${RECEIPTS}/`))
 // The review hash is opaque to the page: this one has capitals and spaces around it, so any trimming, parsing or rebuilding shows.
 const HASH=' 0F'+'ab'.repeat(31)+'Z '
 const cand=(revisionId,matched,observedOnly,revisionOnly)=>({revisionId,matched,observedOnly,revisionOnly})
 const adoptCheck=(over={})=>({state:'captured',capturedAt:Date.now(),reviewHash:HASH,captureHash:'c'.repeat(64),capture:captureOf('present',{manifest:manifestOf([file('SKILL.md',120),file('notes/mine.txt',12),file('scripts/run.sh',41,true)])}),plan:planOf({ready:false,conflicts:[{path:'',code:'adoption_required'}]}),adoption:{candidates:[cand(R.d1,1,2,2),cand(R.d2,3,0,0),cand(R.d3,1,255,0)]},...over})
 const recordBtn=dialog.getByRole('button',{name:'Record ownership',exact:true})
 const recordHeading=dialog.getByRole('heading',{name:'Record ownership',exact:true})
 const radios=dialog.getByRole('radio')
 const choices=dialog.locator('label.choice')
 const bdTable=dialog.getByRole('table',{name:'What recording ownership would do to each file'})
 const asking=page.getByRole('dialog',{name:'Record ownership of demo?'})
 const confirmRecord=asking.getByRole('button',{name:'Confirm and record',exact:true})
 const records=dialog.locator('section.records')
 const releaseAsk=page.getByRole('dialog',{name:'Release ownership of demo?'})
 const alertText=async()=>{const t=await dialog.getByRole('alert').innerText();noIds(t);expect(t).not.toMatch(/\b(40\d|50\d)\b|Conflict|SKILL\.md|already exists|release existing ownership/i);return t}
 // A check made now, then replaced by what the machine "reports".
 const adoptable=async(over={})=>{const n=checkPosts().length;await again.click();await expect.poll(()=>checkPosts().length).toBe(n+1);setLatest(adoptCheck(over));await expect(recordHeading).toBeVisible({timeout:SECOND});return latest().id}
 // The check that follows a recording is waiting for the machine until the machine reports; it reports an owned folder.
 const settled=async()=>{await expect.poll(()=>latest().state).toBe('queued');setLatest(found({plan:planOf({preconditions:{baseline:{manifest:OWNED}}})}));await expect(dialog.getByText('Up to date.',{exact:false})).toBeVisible({timeout:SECOND})}
 // The tab is hidden for the two clicks so no poll can land between them, then shown again.
 const attempt=async()=>{await setHidden(true);await recordBtn.click();await confirmRecord.click();await setHidden(false)}
 const choose=async i=>{await radios.nth(i).check();await expect(bdTable).toBeVisible()}

 // The records cannot be read when the dialog opens: the pins and the checks still work, a plain note offers a retry.
 previewsDb=[];receiptsBroken=true
 await scopeButton('workstation',SHARED).evaluate(el=>el.click())
 await expect(dialog).toBeVisible()
 await expect(records).toContainText("Ownership records couldn't be read right now.")
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 await pinsList.getByRole('button',{name:'Open check for demo'}).click()
 await expect(checkBtn).toBeEnabled()
 await dialog.getByRole('button',{name:'Back to pins',exact:true}).click()
 receiptsBroken=false
 await records.getByRole('button',{name:'Try again'}).click()
 await expect(records).toHaveCount(0)
 await pinsList.getByRole('button',{name:'Open check for demo'}).click()
 await expect(dialog.getByText('Nothing has been checked yet.')).toBeVisible()
 await checkBtn.click()
 await expect(live).toContainText('Waiting for the machine')

 // A folder nothing owns yet, with three revisions that could be recorded: none is chosen for the person, nothing is read about
 // a revision until one is chosen, and Record ownership cannot be pressed.
 setLatest(adoptCheck())
 await expect(recordHeading).toBeVisible({timeout:SECOND})
 await expect(radios).toHaveCount(3)
 await expect(dialog.locator('section.adopt')).toContainText('Recording ownership says which of these files are identical to a revision in your library. It changes nothing on the machine.')
 await expect(dialog.locator('section.adopt')).not.toContainText('published')
 for(let i=0;i<3;i++)await expect(radios.nth(i)).not.toBeChecked()
 await expect(choices.nth(0)).toContainText(L.d1)
 await expect(choices.nth(2)).toContainText(NEWEST)
 await expect(choices.nth(1)).toContainText('3 files match')
 await expect(dialog).toContainText('Choose a revision to see which files would be recorded.')
 await expect(recordBtn).toBeDisabled()
 expect(breakdownReads).toEqual([])
 await recordBtn.evaluate(el=>{el.disabled=false;el.click()})
 await expect(asking).toHaveCount(0)
 await recordBtn.evaluate(el=>{el.disabled=true})
 noIds(await dialog.locator('section.adopt').innerText())

 // A slow answer for an earlier choice must not overwrite a later one, and Record ownership waits for the table.
 breakdownHold={[R.d1]:deferred()}
 await radios.nth(0).check()
 await expect(dialog.getByText('Reading the file list for this revision')).toBeVisible()
 await expect(recordBtn).toBeDisabled()
 await radios.nth(1).check()
 await expect(bdTable.getByRole('row',{name:/scripts\/run\.sh/})).toContainText('Claimed')
 const answeredOld=page.waitForResponse(r=>r.url().includes(`/adoption/${R.d1}`)).catch(()=>{})
 breakdownHold[R.d1].resolve();breakdownHold={}
 await answeredOld
 await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>setTimeout(r,0))))
 await expect(bdTable.getByRole('row',{name:/extra\.md/})).toHaveCount(0)
 await expect(radios.nth(1)).toBeChecked()
 await expect(recordBtn).toBeEnabled()

 // The table: one row per path, a path that differs between the machine and the library is one row, and nothing claims to
 // know why it differs.
 await choose(0)
 await expect(bdTable.locator('tbody tr')).toHaveCount(4)
 await expect(bdTable.getByRole('row',{name:/SKILL\.md/})).toContainText('Claimed')
 await expect(bdTable.getByRole('row',{name:/notes\/mine\.txt/})).toContainText('Stays yours')
 await expect(bdTable.getByRole('row',{name:/extra\.md/})).toContainText('Only in the library')
 await expect(bdTable.getByRole('row',{name:/scripts\/run\.sh/})).toHaveCount(1)
 await expect(bdTable.getByRole('row',{name:/scripts\/run\.sh/})).toContainText('Differs from this revision')
 await expect(dialog.locator('section.adopt')).toContainText('1 file would be recorded as owned by reet.')
 await expect(dialog).not.toContainText(/edited|modified on this machine/i)
 await expect(dialog.getByRole('region',{name:/Files this revision would claim/})).toBeVisible()

 // 256 files and a very long unicode path stay inside their own scroll area at every width.
 await choose(2)
 await expect(bdTable.locator('tbody tr')).toHaveCount(256)
 await expect(bdTable.getByRole('row',{name:/last\.md/})).toHaveCount(1)
 for(const [width,height] of [[1360,1000],[390,844],[320,640]]){
  await page.setViewportSize({width,height})
  await layout();expect(await fits()).toBe(true)
  expect(await dialog.getByRole('region',{name:/Files this revision would claim/}).evaluate(el=>el.scrollWidth<=el.clientWidth+1)).toBe(true)
  await expect(dialog.getByRole('button',{name:'Done'})).toBeInViewport()
 }
 await page.setViewportSize({width:1360,height:1000})

 // The confirmation says what is being recorded, in the exact words, and that nothing on the machine changes. Cancel sends nothing.
 await choose(1)
 await recordBtn.click()
 await expect(asking).toBeVisible()
 await expect(asking).toContainText('Recorded by reet from a recent check, not verified on the machine.')
 await expect(asking).toContainText('Nothing on the machine is changed.')
 await expect(asking).toContainText('Files that are not listed as claimed stay yours.')
 await expect(asking).toContainText('release this record')
 await expect(asking).toContainText(L.d2)
 noIds(await asking.innerText())
 await asking.getByRole('button',{name:'Cancel'}).click()
 await expect(asking).toHaveCount(0)
 expect(receiptPosts()).toEqual([])
 await expect(recordBtn).toBeFocused()

 // The check is still captured but its review hash is another one by the time it is confirmed: what the person saw is not what
 // would be recorded, so nothing is sent and the records are read again.
 await recordBtn.click()
 await expect(asking).toBeVisible()
 await setHidden(true);await settleReads()
 const recordReads=()=>requests.filter(r=>r.method==='GET'&&r.path===RECEIPTS).length,readsBeforeHash=recordReads()
 setLatest({reviewHash:'1'+HASH})
 await confirmRecord.click()
 await expect(dialog.getByRole('alert')).toHaveText('Something changed. Check again.')
 await afterRequests()
 expect(receiptPosts()).toEqual([])
 expect(recordReads()).toBeGreaterThan(readsBeforeHash)
 await setHidden(false)
 // The table is read again for the new hash, and Record ownership waits for it.
 breakdownHold={[R.d2]:deferred()}
 setLatest({reviewHash:'2'+HASH})
 await expect(recordBtn).toBeDisabled()
 await expect(dialog.getByText('Reading the file list for this revision')).toBeVisible()
 breakdownHold[R.d2].resolve();breakdownHold={}
 await expect(recordBtn).toBeEnabled()

 // The check goes out of date between opening the confirmation and confirming it (the tab is hidden, so nothing re-reads on its
 // own): the check is read once more, nothing is sent, and the screen says so.
 await recordBtn.click()
 await expect(asking).toBeVisible()
 await setHidden(true);await settleReads()
 setLatest({reasons:['inventory_changed']})
 await confirmRecord.click()
 await expect(dialog.getByRole('alert')).toHaveText('Something changed. Check again.')
 await expect(dialog.getByText('The machine reported a newer scan after this check was made.')).toBeVisible()
 await expect(recordBtn).toHaveCount(0)
 await afterRequests()
 expect(receiptPosts()).toEqual([])
 await setHidden(false)

 // Anything that is not a current, adoptable capture offers nothing: not even a disabled button.
 const notOffered=async(over,why)=>{
  const n=checkPosts().length;await again.click();await expect.poll(()=>checkPosts().length).toBe(n+1)
  setLatest(adoptCheck(over))
  await expect(dialog.getByText(why,{exact:false}).first()).toBeVisible({timeout:SECOND})
  await expect(again).toBeVisible()
  await expect(recordBtn).toHaveCount(0);await expect(recordHeading).toHaveCount(0);await expect(radios).toHaveCount(0)
 }
 await notOffered({state:'stale',reasons:['stale']},'more than five minutes old')
 await notOffered({state:'superseded',reasons:['receipt_changed']},'The ownership record for this skill changed')
 await notOffered({reasons:['pin_changed']},'The pin changed after this check was made.')
 await notOffered({plan:planOf({ready:false,conflicts:[{path:'',code:'adoption_required'},{path:'x.md',code:'unmanaged_path'}]})},"reet doesn't own it")
 await notOffered({capture:captureOf('present',{rootOwnership:'shared',manifest:manifestOf([file('SKILL.md',120)])})},'Another agent scope')
 await notOffered({capture:captureOf('absent'),plan:planOf({actions:[{path:'SKILL.md',kind:'create',after:file('SKILL.md',130)}]})},'Not there yet')
 await notOffered({reviewHash:undefined},'Files already there are not recorded as owned by reet yet.')
 // A captured read the browser has not renewed for thirty seconds is only "refreshing": the step goes until it is read again.
 {
  const n=checkPosts().length;await again.click();await expect.poll(()=>checkPosts().length).toBe(n+1)
  setLatest(adoptCheck());await expect(recordHeading).toBeVisible({timeout:SECOND})
  await setHidden(true);await settleReads();await page.evaluate(()=>{window.__skew=60000})
  await expect(dialog.getByText('Updating this result')).toBeVisible({timeout:SECOND})
  await expect(recordBtn).toHaveCount(0);await expect(recordHeading).toHaveCount(0)
  await page.evaluate(()=>{window.__skew=0});await setHidden(false)
  await expect(recordHeading).toBeVisible({timeout:SECOND})
  // It comes back with nothing chosen.
  for(let i=0;i<3;i++)await expect(radios.nth(i)).not.toBeChecked()
 }
 // A confirmation that is open when the check lapses closes with a plain sentence rather than vanishing.
 {
  await choose(1)
  await recordBtn.click()
  await expect(asking).toBeVisible()
  await setHidden(true);await settleReads();await page.evaluate(()=>{window.__skew=60000})
  await expect(dialog.getByRole('alert')).toHaveText('That check is out of date. Check again.',{timeout:SECOND})
  await expect(asking).toHaveCount(0)
  await page.evaluate(()=>{window.__skew=0});await setHidden(false)
  await expect(recordHeading).toBeVisible({timeout:SECOND})
 }
 // A server that has gone off the check is heard while the step is open: its verdict arrives on the next read and the step goes.
 setLatest({state:'superseded',reasons:['inventory_changed']})
 await expect(recordHeading).toHaveCount(0,{timeout:SECOND})
 await expect(recordBtn).toHaveCount(0)

 // One revision that matches is chosen for the person, but its table is still read first and Record ownership waits for it.
 breakdownHold={[R.d2]:deferred()}
 {
  const n=checkPosts().length;await again.click();await expect.poll(()=>checkPosts().length).toBe(n+1)
  setLatest(adoptCheck({adoption:{candidates:[cand(R.d2,3,0,0)]}}))
  await expect(recordHeading).toBeVisible({timeout:SECOND})
  await expect(radios).toHaveCount(1)
  await expect(radios.nth(0)).toBeChecked()
  await expect(dialog.getByText('Reading the file list for this revision')).toBeVisible()
  await expect(recordBtn).toBeDisabled()
  breakdownHold[R.d2].resolve();breakdownHold={}
  await expect(bdTable).toBeVisible()
  await expect(recordBtn).toBeEnabled()
 }
 // No revision matches: nothing to record, and no way to try.
 await adoptable({adoption:{candidates:[]}})
 await expect(dialog.getByText('No revision of this library skill matches the files in this folder')).toBeVisible()
 await expect(radios).toHaveCount(0)
 await expect(recordBtn).toBeDisabled()

 // The scope stops qualifying while the step is open (a connector is paused elsewhere): recording is switched off with the reason,
 // and a confirmation that was already open sends nothing and says why.
 await adoptable()
 await choose(1)
 await expect(recordBtn).toBeEnabled()
 await recordBtn.click()
 await expect(asking).toBeVisible()
 connectorsPaused=true
 await expect(recordBtn).toBeDisabled({timeout:15000})
 await expect(dialog.locator('section.adopt')).toContainText('The connector on this machine is paused')
 {
  const sentBefore=receiptPosts().length
  await confirmRecord.click()
  await expect(dialog.getByRole('alert')).toContainText('The connector on this machine is paused')
  await afterRequests()
  expect(receiptPosts()).toHaveLength(sentBefore)
 }
 connectorsPaused=false
 await expect(recordBtn).toBeEnabled({timeout:15000})

 // Reads of the watched check that fail are tried again more and more slowly, and keep a Try again (a sign-out is tested at the end).
 {
  const watched=latest().id
  previewGetFail=500
  await expect(dialog.getByRole('alert')).toContainText('The checks could not be read right now.',{timeout:SECOND})
  await page.waitForTimeout(400)
  const settledReads=reads(watched)
  await page.waitForTimeout(2000)
  expect(reads(watched)-settledReads).toBeLessThanOrEqual(1)
  previewGetFail=0
  await dialog.getByRole('alert').getByRole('button',{name:'Try again'}).click()
  await expect(dialog.getByRole('alert')).toHaveCount(0)
  previewGetFail=0
 }

 // Confirming after something else has become busy says so, sends nothing, and the message goes once the other action is done.
 await adoptable()
 await choose(1)
 await recordBtn.click()
 await expect(asking).toBeVisible()
 pinsHold=deferred()
 await again.evaluate(el=>el.click())
 {
  const sentBusy=receiptPosts().length
  await confirmRecord.click()
  await expect(dialog.getByRole('alert')).toHaveText('Another action is still under way. Try again in a moment.')
  expect(receiptPosts()).toHaveLength(sentBusy)
 }
 pinsHold.resolve();pinsHold=null
 await expect.poll(()=>latest().state).toBe('queued')
 setLatest(adoptCheck())
 await expect(recordHeading).toBeVisible({timeout:SECOND})
 await expect(dialog.getByRole('alert')).toHaveCount(0)

 // ---- Recording ownership ----
 // One click on Confirm sends one request with exactly the four fields, the review hash as the server gave it, even when the
 // button is pressed twice while the request is under way. The check is read again first (the tab is hidden so only that read can be it).
 let checkId=await adoptable()
 await choose(1)
 await recordBtn.click()
 await expect(asking).toBeVisible()
 adoptHold=deferred()
 await setHidden(true)
 let mark=requests.length
 await confirmRecord.evaluate(el=>{el.click();el.click()})
 await expect.poll(()=>receiptPosts().length).toBe(1)
 const fresh=requests.slice(mark),postAt=fresh.findIndex(r=>r.method==='POST'&&r.path===RECEIPTS)
 expect(fresh.slice(0,postAt).some(r=>r.method==='GET'&&r.path===`${POSTS}/${checkId}`)).toBe(true)
 await afterRequests()
 expect(receiptPosts()).toHaveLength(1)
 const sent=receiptPosts()[0].body
 expect(Object.keys(sent).sort()).toEqual(['adoptAsRevisionId','previewId','requestKey','reviewHash'])
 expect(sent.requestKey).toMatch(/^[A-Za-z0-9_-]{16,80}$/)
 expect(sent.previewId).toBe(checkId);expect(sent.reviewHash).toBe(HASH);expect(sent.adoptAsRevisionId).toBe(R.d2)
 // While it is under way nothing else can be started.
 await expect(records.getByRole('button',{name:/Release ownership/})).toHaveCount(0)
 await setHidden(false)
 const checksBefore=checkPosts().length
 adoptHold.resolve();adoptHold=null
 await expect(records.getByRole('listitem')).toHaveCount(1)
 expect(receiptPosts()).toHaveLength(1)
 const item=records.getByRole('listitem').first()
 await expect(item).toContainText('demo')
 await expect(item).toContainText(L.d2)
 await expect(item).toContainText('3 files')
 await expect(item).toContainText('Recorded by reet from a recent check, not verified on the machine.')
 noIds(await records.innerText())
 const said=dialog.locator('.live')
 await expect(said).toContainText('Sorted.')
 await expect(said).toContainText('Ownership of 3 files in demo is recorded.')
 await expect(said).toContainText('Nothing on the machine was changed.')
 expect(((await said.innerText()).replace('Recorded by reet from a recent check, not verified on the machine.','')).match(/verified/i)).toBeNull()
 noIds(await said.innerText())
 expect(await said.getAttribute('aria-live')).toBe('polite')
 // A new check is started for the person, the one that was used is not offered again, and focus has somewhere to be.
 await expect.poll(()=>checkPosts().length).toBe(checksBefore+1)
 await expect(recordBtn).toHaveCount(0)
 await expect(checkHeading).toBeFocused()
 // The next check shows the result as an owned folder: up to date, or the read-only change list.
 setLatest(found({plan:planOf({preconditions:{baseline:{manifest:OWNED}}})}))
 await expect(dialog.getByText('Up to date.',{exact:false})).toBeVisible({timeout:SECOND})
 await expect(recordHeading).toHaveCount(0)
 {
  const n=checkPosts().length;await again.click();await expect.poll(()=>checkPosts().length).toBe(n+1)
  setLatest(found())
  await expect(dialog.getByRole('table',{name:'What a later apply would change'})).toBeVisible({timeout:SECOND})
  await expect(dialog).toContainText("Applying changes isn't available yet.")
 }

 // Release asks first and says it removes the record only; cancel sends nothing.
 const releaseBtn=records.getByRole('button',{name:/^Release ownership/})
 await releaseBtn.click()
 await expect(releaseAsk).toBeVisible()
 await expect(releaseAsk).toContainText('removes the ownership record only')
 await expect(releaseAsk).toContainText('Nothing on the machine is changed')
 await expect(releaseAsk).toContainText('the files stay')
 await releaseAsk.getByRole('button',{name:'Cancel'}).click()
 expect(receiptDeletes()).toEqual([])
 await expect(releaseBtn).toBeFocused()
 // A refusal with the version unchanged says nothing was released, not "check again".
 releaseRefuse=true
 await releaseBtn.click()
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(dialog.getByRole('alert')).toHaveText('Nothing was released. Try again.')
 releaseRefuse=false
 expect(receiptDeletes()).toHaveLength(1)
 // Confirming after something else has become busy says so, rather than doing nothing.
 await releaseBtn.click()
 await expect(releaseAsk).toBeVisible()
 pinsHold=deferred()
 await again.evaluate(el=>el.click())
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(dialog.getByRole('alert')).toHaveText('Another action is still under way. Try again in a moment.')
 expect(receiptDeletes()).toHaveLength(1)
 pinsHold.resolve();pinsHold=null
 await expect.poll(()=>latest().state).toBe('queued')
 setLatest(found({plan:planOf({preconditions:{baseline:{manifest:OWNED}}})}))
 await expect(dialog.getByText('Up to date.',{exact:false})).toBeVisible({timeout:SECOND})
 // A version that moved on is refused: the records are read again and the screen says what changed, without the server's words.
 const heldRecord=receiptsDb[0],oldVersion=heldRecord.version
 heldRecord.version='rv-elsewhere'
 await releaseBtn.click()
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(dialog.getByRole('alert')).toContainText('The ownership record for demo changed elsewhere. Nothing was released.')
 await alertText()
 expect(receiptDeletes()).toHaveLength(2)
 expect(receiptDeletes()[1].body).toEqual({expectedVersion:oldVersion})
 await expect(item).toContainText('demo')
 // Read again, the next release carries the version that is current now, and says what was done.
 await releaseBtn.click()
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(records).toHaveCount(0)
 expect(receiptDeletes()).toHaveLength(3)
 expect(receiptDeletes()[2]).toEqual({method:'DELETE',path:`${RECEIPTS}/${heldRecord.id}`,body:{expectedVersion:'rv-elsewhere'}})
 await expect(said).toContainText('Ownership released for demo.')
 await expect(said).toContainText('nothing on the machine changed')
 await expect(dialog.getByRole('heading',{name:/^Private skills on/})).toBeFocused()
 await expect(dialog.getByRole('alert')).toHaveCount(0)

 // A reply that never arrives but the record was made: it is found among the records and no second request is sent.
 checkId=await adoptable()
 await choose(1)
 dropAdopt='after'
 let postsBefore=receiptPosts().length
 const checksAtLost=checkPosts().length
 await attempt()
 await expect(records.getByRole('listitem')).toHaveCount(1)
 expect(receiptPosts()).toHaveLength(postsBefore+1)
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 await expect(said).toContainText('Ownership of 3 files in demo is recorded.')
 await expect.poll(()=>checkPosts().length).toBe(checksAtLost+1)
 setLatest(found({plan:planOf({preconditions:{baseline:{manifest:OWNED}}})}))
 await expect(dialog.getByText('Up to date.',{exact:false})).toBeVisible({timeout:SECOND})
 await records.getByRole('button',{name:/^Release ownership/}).click()
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(records).toHaveCount(0)

 // The reply is lost AND the records cannot be read: the record that exists is not listed yet, nothing is sent twice, and it appears
 // by itself once the records can be read.
 checkId=await adoptable()
 await choose(1)
 dropAdopt='after';receiptsFail=2
 postsBefore=receiptPosts().length
 await attempt()
 await expect(dialog.getByRole('alert')).toContainText("Couldn't reach reet")
 expect(receiptPosts()).toHaveLength(postsBefore+1)
 await expect(records).toHaveCount(0)
 await expect(records.getByRole('listitem')).toHaveCount(1,{timeout:SECOND})
 // The message about the lost reply gives way to one that says what happened.
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 await expect(dialog.locator('.live')).toContainText('That ownership was recorded.')
 expect(receiptPosts()).toHaveLength(postsBefore+1)
 await records.getByRole('button',{name:/^Release ownership/}).click()
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(records).toHaveCount(0)

 // The records stay unreadable: after about six tries (and none while the tab is hidden) the person is told it could not be confirmed.
 checkId=await adoptable()
 await choose(1)
 dropAdopt='after';receiptsBroken=true
 postsBefore=receiptPosts().length
 await setHidden(true)
 await recordBtn.click();await confirmRecord.click()
 await expect(dialog.getByRole('alert')).toContainText("Couldn't reach reet")
 await settleReads()
 {
  const n=requests.filter(r=>r.method==='GET'&&r.path===RECEIPTS).length
  await page.waitForTimeout(700)
  expect(requests.filter(r=>r.method==='GET'&&r.path===RECEIPTS).length).toBe(n)
 }
 await setHidden(false)
 await expect(dialog.getByRole('alert')).toHaveText("Couldn't confirm whether that was recorded. Check the list below, or try again.",{timeout:SECOND})
 expect(receiptPosts()).toHaveLength(postsBefore+1)
 {
  const n=requests.filter(r=>r.method==='GET'&&r.path===RECEIPTS).length
  await page.waitForTimeout(600)
  expect(requests.filter(r=>r.method==='GET'&&r.path===RECEIPTS).length).toBe(n)
 }
 receiptsBroken=false;receiptsDb=[]

 // A reply that never arrives and the records cannot be read either: the message is plain, the key is kept, and the next press
 // sends the same request, which the server answers once.
 checkId=await adoptable()
 await choose(1)
 dropAdopt='before';receiptsBroken=true
 postsBefore=receiptPosts().length
 await attempt()
 await expect(dialog.getByRole('alert')).toContainText("Couldn't reach reet")
 expect(receiptPosts()).toHaveLength(postsBefore+1)
 const lostAdoptKey=receiptPosts()[postsBefore].body.requestKey
 receiptsBroken=false
 await expect(recordBtn).toBeEnabled()
 await attempt()
 await expect(records.getByRole('listitem')).toHaveCount(1)
 expect(receiptPosts()).toHaveLength(postsBefore+2)
 expect(receiptPosts()[postsBefore+1].body.requestKey).toBe(lostAdoptKey)
 expect(receiptsDb).toHaveLength(1)
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 await settled()
 await records.getByRole('button',{name:/^Release ownership/}).click()
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(records).toHaveCount(0)

 // At the account's limit of 1,024 records the refusal says so.
 checkId=await adoptable()
 await choose(1)
 receiptsDb=Array.from({length:1024},(_,i)=>({id:`bulk${i}`,environmentId:'scope-stale',agentId:'agent-0',name:`bulk-${i}`,bundleId:B.demo,revisionId:R.d1,managedDigest:'m'.repeat(64),fileCount:1,provenance:'adopted',verification:'controller_attested',evidence:{previewId:`x${i}`,reviewHash:'y',captureHash:'c',observedDigest:'o',pinId:'p',pinVersion:'v',capturedAt:1,adoptedAt:1,adoptedBy:1},version:`rv-bulk-${i}`,createdAt:1,updatedAt:1}))
 adoptRefuse=true
 await attempt()
 await expect(dialog.getByRole('alert')).toContainText('This account already holds the most ownership records it can (1,024).')
 expect(await alertText()).not.toMatch(/check again/i)
 receiptsDb=[];adoptRefuse=false

 // The server refuses (a 409 whose text is shared by many causes): the screen says what happened in its own words, the dialog stays
 // usable, and a new deliberate attempt is a new request key.
 checkId=await adoptable()
 await choose(1)
 adoptRefuse=true
 postsBefore=receiptPosts().length
 await attempt()
 // Nothing explains the refusal and the check read again is as it was: it does not send the person off to check again.
 await expect(dialog.getByRole('alert')).toContainText("Nothing was recorded. reet couldn't record ownership this time. Try again; if it keeps happening, release a record you no longer need.")
 expect(await alertText()).not.toMatch(/check again/i)
 expect(receiptPosts()).toHaveLength(postsBefore+1)
 await expect(again).toBeEnabled()
 await expect(recordBtn).toBeEnabled()
 adoptRefuse=false
 await attempt()
 await expect(records.getByRole('listitem')).toHaveCount(1)
 expect(receiptPosts()).toHaveLength(postsBefore+2)
 expect(receiptPosts()[postsBefore+1].body.requestKey).not.toBe(receiptPosts()[postsBefore].body.requestKey)
 await expect(dialog.getByRole('alert')).toHaveCount(0)
 await settled()
 await records.getByRole('button',{name:/^Release ownership/}).click()
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(records).toHaveCount(0)

 // Somebody else recorded the skill meanwhile: the refusal says the folder already has a record, and the record is listed.
 checkId=await adoptable()
 await choose(1)
 receiptsDb=[{id:'rcptaken',environmentId:'scope-one',agentId:'agent-0',name:'demo',bundleId:B.demo,revisionId:R.d1,managedDigest:'m'.repeat(64),fileCount:2,provenance:'adopted',verification:'controller_attested',evidence:{previewId:'x',reviewHash:'y',captureHash:'c',observedDigest:'o',pinId:'p',pinVersion:'v',capturedAt:1,adoptedAt:1,adoptedBy:1},version:'rv-taken',createdAt:Date.now(),updatedAt:Date.now()}]
 await attempt()
 await expect(dialog.getByRole('alert')).toContainText('demo already has an ownership record in this folder.')
 await alertText()
 await expect(records.getByRole('listitem')).toHaveCount(1)
 await records.getByRole('button',{name:/^Release ownership/}).click()
 await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(records).toHaveCount(0)
 // A viewer's refusal and an unavailable check have their own words, and neither shows the server's.
 checkId=await adoptable()
 await choose(1)
 adoptFail=403
 await attempt()
 await expect(dialog.getByRole('alert')).toHaveText("Your account can only view, so it can't record ownership.")
 adoptFail=404
 await attempt()
 await expect(dialog.getByRole('alert')).toContainText('Nothing was recorded.')
 expect(await alertText()).not.toMatch(/unavailable/i)
 adoptFail=0

 // A record that can no longer be read is released and nothing more is said about it; a record for another scope is not shown.
 receiptsDb=[{id:'rcpolder',environmentId:'scope-one',agentId:'agent-0',name:'older',bundleId:B.demo,revisionId:hex('9'),managedDigest:'m'.repeat(64),fileCount:1,provenance:'adopted',verification:'controller_attested',evidence:{previewId:'x',reviewHash:'y',captureHash:'c',observedDigest:'o',pinId:'p',pinVersion:'v',capturedAt:1,adoptedAt:1,adoptedBy:1},version:'rv-older',createdAt:Date.now(),updatedAt:Date.now()},{id:'rcpdamaged',environmentId:'scope-one',agentId:'agent-0',name:'ghost',bundleId:'',revisionId:'',managedDigest:'',fileCount:0,provenance:'',verification:'',evidence:{},version:'rv-damaged',createdAt:0,updatedAt:0,damaged:true},{id:'rcpelsewhere',environmentId:'scope-stale',agentId:'agent-0',name:'other-scope',bundleId:B.demo,revisionId:R.d1,managedDigest:'m'.repeat(64),fileCount:2,provenance:'adopted',verification:'controller_attested',evidence:{previewId:'x',reviewHash:'y',captureHash:'c',observedDigest:'o',pinId:'p',pinVersion:'v',capturedAt:1,adoptedAt:1,adoptedBy:1},version:'rv-else',createdAt:1,updatedAt:1}]
 await dialog.getByRole('button',{name:'Back to pins',exact:true}).click()
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 await scopeButton('workstation',SHARED).evaluate(el=>el.click())
 await expect(records.getByRole('listitem')).toHaveCount(2)
 // A record whose revision the library no longer lists is "an older revision", never a fragment of an id.
 const older=records.getByRole('listitem').filter({hasText:'older'}).first()
 await expect(older).toContainText('An older revision · 1 file')
 await expect(older).not.toContainText(/Revision an/i)
 noIds(await older.innerText())
 const ghost=records.getByRole('listitem').filter({hasText:'ghost'}).first()
 await expect(ghost).toContainText('ghost')
 await expect(ghost).toContainText("This record can't be read. It can still be released.")
 for(const word of [/file/i,/Recorded/,/revision/i,/verified/i])await expect(ghost).not.toContainText(word)
 await expect(ghost.getByRole('button',{name:/^Release ownership/})).toBeVisible()
 await expect(records).not.toContainText('other-scope')
 await ghost.getByRole('button',{name:/^Release ownership/}).click()
 await page.getByRole('dialog',{name:'Release ownership of ghost?'}).getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(records.getByRole('listitem')).toHaveCount(1)
 await older.getByRole('button',{name:/^Release ownership/}).click()
 await page.getByRole('dialog',{name:'Release ownership of older?'}).getByRole('button',{name:'Release ownership',exact:true}).click()
 await expect(records).toHaveCount(0)
 expect(receiptDeletes().find(d=>d.path.endsWith('rcpdamaged'))).toEqual({method:'DELETE',path:`${RECEIPTS}/rcpdamaged`,body:{expectedVersion:'rv-damaged'}})
 await dialog.getByRole('button',{name:'Done'}).click()
 await expect(dialog).toHaveCount(0)
 await quiet()

 // ---- Layout, focus and motion across sizes and themes ----
 // Every step and both confirmations at four widths in light and dark: the page never scrolls sideways, the dialog stays inside the
 // window with its header and footer in view, and each primary action can be reached (scrolled to inside the dialog's own body and
 // not covered by anything). Focus goes back to what opened the dialog, or to a control that is still there, after each confirmation;
 // Escape closes only the top dialog and writes nothing; with reduced motion nothing in the dialogs animates or transitions.
 const SIZES=[[1360,1000],[768,1024],[390,844],[320,640]],SCHEMES=['light','dark']
 let home=[1360,1000,'light']
 const setView=async(width,height,scheme)=>{await page.setViewportSize({width,height});await page.emulateMedia({colorScheme:scheme})}
 const reachable=async target=>{
  await target.scrollIntoViewIfNeeded()
  await expect(target).toBeInViewport()
  expect(await target.evaluate(el=>{const r=el.getBoundingClientRect(),hit=document.elementFromPoint(r.left+r.width/2,r.top+r.height/2);return !!hit&&(hit===el||el.contains(hit))})).toBe(true)
 }
 // top: the confirmation is the dialog on top, so it is the one that must sit inside the window.
 const holds=async(targets,{top=false}={})=>{
  await layout()
  if(top)expect(await page.evaluate(()=>{const d=[...document.querySelectorAll('dialog[open]')].at(-1),r=d.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth+.5&&r.top>=0&&r.bottom<=innerHeight+.5&&d.scrollWidth<=d.clientWidth+1})).toBe(true)
  else{
   expect(await fits()).toBe(true)
   await expect(dialog.getByRole('button',{name:'Close private skills'})).toBeInViewport()
   await expect(dialog.getByRole('button',{name:'Done'})).toBeInViewport()
  }
  for(const target of targets)await reachable(target)
  expect(await page.evaluate(()=>[...document.querySelectorAll('dialog[open] [role=region]')].every(el=>el.scrollWidth<=el.clientWidth+1))).toBe(true)
 }
 const sweep=async(targets,opts)=>{
  for(const [width,height] of SIZES)for(const scheme of SCHEMES){await setView(width,height,scheme);await holds(targets,opts)}
  await setView(...home)
 }
 // What is animating or could transition inside any open dialog. Computed styles are read, so the answer does not depend on timing.
 const motion=()=>page.evaluate(()=>{
  const running=document.getAnimations().filter(a=>a.playState==='running'&&a.effect?.target?.closest?.('dialog')).length
  const styled=[...document.querySelectorAll('dialog[open]')].flatMap(d=>[d,...d.querySelectorAll('*')]).filter(el=>{const s=getComputedStyle(el);return s.animationName!=='none'||s.transitionDuration.split(',').some(x=>parseFloat(x)>0)}).length
  return {running,styled}
 })
 const matrixPin=(name,bundleId,revisionId)=>({id:`pin-matrix-${name.length}`,environmentId:'scope-one',agentId:'agent-0',name,bundleId,revisionId,manifestDigest:'0'.repeat(64),mode:'additive',version:`ver-matrix-${name.length}`,updatedAt:now,verification:'not_verified'})
 const matrixRecords=()=>[{id:'rcpolder',environmentId:'scope-one',agentId:'agent-0',name:'older',bundleId:B.demo,revisionId:hex('9'),managedDigest:'m'.repeat(64),fileCount:1,provenance:'adopted',verification:'controller_attested',evidence:{previewId:'x',reviewHash:'y',captureHash:'c',observedDigest:'o',pinId:'p',pinVersion:'v',capturedAt:1,adoptedAt:1,adoptedBy:1},version:'rv-older',createdAt:Date.now(),updatedAt:Date.now()},{id:'rcpdamaged',environmentId:'scope-one',agentId:'agent-0',name:'ghost',bundleId:'',revisionId:'',managedDigest:'',fileCount:0,provenance:'',verification:'',evidence:{},version:'rv-damaged',createdAt:0,updatedAt:0,damaged:true}]
 const bulk=Array.from({length:60},(_,i)=>file(`references/${'a-very-long-unbroken-file-name-'.repeat(3)}${i}-ünïcödé.md`,100+i))

 // One walk through the dialog: open, every step, both confirmations (cancelled with Escape, cancelled with the button, then
 // confirmed), the records list with a damaged record, and the two ways to close. full sweeps all eight size and theme pairs at
 // every state; otherwise the state is held to the size and theme it is run at.
 const lifecycle=async(label,[width,height,scheme],{full=false,reduce=false}={})=>{
  home=[width,height,scheme]
  await setView(width,height,scheme)
  await page.emulateMedia({reducedMotion:reduce?'reduce':'no-preference'})
  authMode='accounts';viewer=false;connectorsPaused=false;capReady=true;previewsBroken=false;pinsBroken=false;receiptsBroken=false
  adoptFail=0;adoptRefuse=false;dropAdopt=null;dropCreate=null;releaseRefuse=false;releaseHold=null;adoptHold=null;breakdownHold={};hold=null;pinsHold=null;previewGetHold=null
  pins=[...pins.filter(p=>p.id==='pin-elsewhere'),matrixPin('demo',B.demo,R.d3),matrixPin(LONG_NAME,B.long,R.l1)]
  previewsDb=[];receiptsDb=matrixRecords();receiptKeys=new Map()
  const stays=async()=>{if(reduce)expect(await motion()).toEqual({running:0,styled:0})}
  const state=async(targets,opts)=>{await (full?sweep:holds)(targets,opts);await stays()}
  const sent=()=>writes.length

  await expand('workstation')
  await opener.click()
  await expect(dialog).toBeVisible()
  await expect(pinsList.getByRole('listitem')).toHaveCount(2)
  const rows=records.getByRole('listitem')
  await expect(rows).toHaveCount(2)
  await expect(rows.filter({hasText:'ghost'})).toContainText("This record can't be read. It can still be released.")
  await state([dialog.getByRole('button',{name:'Save pin'}),rows.first().getByRole('button',{name:/^Release ownership/}),rows.last().getByRole('button',{name:/^Release ownership/})])
  // Not vacuous: without the preference the buttons do transition, so the reduced run below proves something.
  if(!reduce)expect((await motion()).styled).toBeGreaterThan(0)

  // Check step: nothing checked yet, waiting for the machine, then a found folder with a long list of files.
  await pinsList.getByRole('button',{name:'Open check for demo'}).click()
  await expect(checkBtn).toBeEnabled()
  await expect(checkHeading).toBeFocused()
  await state([checkBtn])
  const made=checkPosts().length
  await checkBtn.click()
  await expect.poll(()=>checkPosts().length).toBe(made+1)
  await expect(live).toContainText('Waiting for the machine')
  await state([dialog.getByRole('button',{name:'Stop this check',exact:true})])
  setLatest(found({capture:captureOf('present',{manifest:manifestOf([file('SKILL.md',120),...bulk])})}))
  await expect(heading('Found')).toBeVisible({timeout:SECOND})
  await state([again,dialog.getByRole('button',{name:'Dismiss this result'})])

  // Adopt step: nothing chosen, then 256 files with a very long path, then the confirmation.
  await adoptable()
  await state([recordBtn])
  await choose(2)
  await expect(bdTable.locator('tbody tr')).toHaveCount(256)
  await expect(recordBtn).toBeEnabled()
  await state([recordBtn])
  await recordBtn.click()
  await expect(asking).toBeVisible()
  await state([asking.getByRole('button',{name:'Cancel'}),confirmRecord],{top:true})
  if(!reduce)expect((await motion()).styled).toBeGreaterThan(0)
  // Escape closes the confirmation only: the dialog stays, focus returns to the button that asked, and nothing was written.
  let before=sent()
  await page.keyboard.press('Escape')
  await expect(asking).toHaveCount(0)
  await expect(dialog).toBeVisible()
  await expect(recordBtn).toBeFocused()
  expect(sent()).toBe(before)
  await recordBtn.click()
  await expect(asking).toBeVisible()
  await asking.getByRole('button',{name:'Cancel'}).click()
  await expect(asking).toHaveCount(0)
  await expect(recordBtn).toBeFocused()
  expect(sent()).toBe(before)
  // Confirming records once, and focus goes to the Check step's heading because the button that asked has gone.
  const recorded=receiptPosts().length
  await recordBtn.click()
  await confirmRecord.click()
  await expect.poll(()=>receiptPosts().length).toBe(recorded+1)
  await expect(recordBtn).toHaveCount(0)
  await expect(checkHeading).toBeFocused()
  await settled()
  await expect(rows).toHaveCount(3)
  await state([again,rows.filter({has:page.getByText('demo',{exact:true})}).getByRole('button',{name:'Release ownership of demo'})])

  // Release: the same three ways out of the confirmation. Releasing leaves focus on the records heading, which is still there.
  const releaseDemo=rows.filter({has:page.getByText('demo',{exact:true})}).getByRole('button',{name:'Release ownership of demo'})
  await releaseDemo.click()
  await expect(releaseAsk).toBeVisible()
  await state([releaseAsk.getByRole('button',{name:'Cancel'}),releaseAsk.getByRole('button',{name:'Release ownership',exact:true})],{top:true})
  before=sent()
  await page.keyboard.press('Escape')
  await expect(releaseAsk).toHaveCount(0)
  await expect(dialog).toBeVisible()
  await expect(releaseDemo).toBeFocused()
  await releaseDemo.click()
  await releaseAsk.getByRole('button',{name:'Cancel'}).click()
  await expect(releaseDemo).toBeFocused()
  expect(sent()).toBe(before)
  await releaseDemo.click()
  await releaseAsk.getByRole('button',{name:'Release ownership',exact:true}).click()
  await expect(rows).toHaveCount(2)
  await expect(records.getByRole('heading',{name:'Ownership records in this folder'})).toBeFocused()
  await state([again])

  // Closing: Escape writes nothing and hands focus back to the opener; so does Done on the next opening.
  before=sent()
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(opener).toBeFocused()
  expect(sent()).toBe(before)
  await opener.click()
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button',{name:'Done'}).click()
  await expect(dialog).toHaveCount(0)
  await expect(opener).toBeFocused()
  await quiet()
  await stays()
 }
 await lifecycle('desktop, light',[1360,1000,'light'],{full:true})
 await lifecycle('tablet, dark',[768,1024,'dark'])
 await lifecycle('phone, light',[390,844,'light'])
 await lifecycle('narrow phone, dark, reduced motion',[320,640,'dark'],{reduce:true})
 await page.emulateMedia({reducedMotion:'no-preference'})
 await setView(1360,1000,'light')

 // Pins, checks and ownership never touch browser storage: not one write, from the moment the dialog first opened, and the
 // stored state is what it was. Nothing ever asked for revision bytes or any other read of a skill's files.
 expect((await page.evaluate(()=>window.__storageCalls)).slice(storageCallsAtOpen)).toEqual([])
 expect(await storage()).toBe(before)
 expect(requests.filter(r=>/\/revisions\/[^/]/.test(r.path))).toEqual([])
 // The breakdown is only ever read (GET, metadata), and no check can exist before a pin does: the checks are first read after the first pin was saved.
 expect(requests.filter(r=>/\/adoption\//.test(r.path)).every(r=>r.method==='GET')).toBe(true)
 expect(breakdownReads.length).toBeGreaterThan(0)
 const firstPinSave=requests.findIndex(r=>r.method==='POST'&&r.path==='/api/v2/skill-content-targets'),firstCheckRead=requests.findIndex(r=>r.method==='GET'&&r.path==='/api/v2/skill-content-previews')
 expect(firstPinSave).toBeGreaterThan(-1)
 expect(firstCheckRead).toBeGreaterThan(firstPinSave)

 // Polling asked for three seconds while a check could change and ten once it was captured, and for nothing else.
 expect(await page.evaluate(()=>[...window.__poll.delays].sort((x,y)=>x-y))).toEqual([3000,10000])

 // A sign-in that ends mid-flow: every read and timer stops, the plain sentence is said, and the page's own sign-in card takes over.
 // (1) a poll of the watched check gets a 401.
 const openCheckFresh=async()=>{
  await page.reload()
  await expect(card('workstation')).toBeVisible();await expand('workstation')
  await scopeButton('workstation',SHARED).evaluate(el=>el.click())
  await expect(dialog).toBeVisible()
  await pinsList.getByRole('button',{name:'Open check for demo'}).click()
  const n=checkPosts().length
  await dialog.getByRole('button',{name:/^Check (this machine|again)$/}).click()
  await expect.poll(()=>checkPosts().length).toBe(n+1)
  setLatest(found())
  await expect(heading('Found')).toBeVisible({timeout:SECOND})
  return latest().id
 }
 {
  const watched=await openCheckFresh()
  previewGetFail=401
  await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible({timeout:SECOND})
  const after=reads(watched)
  await page.waitForTimeout(1200)
  expect(reads(watched)).toBe(after)
  previewGetFail=0
 }
 // (2) the capabilities read (running because the connector has not reported in) gets a 401: nothing re-arms it.
 {
  capReady=false
  await page.reload()
  await expect(card('workstation')).toBeVisible();await expand('workstation')
  await scopeButton('workstation',SHARED).evaluate(el=>el.click())
  await expect(dialog).toBeVisible()
  await pinsList.getByRole('button',{name:'Open check for demo'}).click()
  await expect(dialog).toContainText("hasn't checked in recently")
  listFail=401
  await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible({timeout:SECOND})
  const listReads=()=>requests.filter(r=>r.method==='GET'&&r.path===POSTS).length
  const listed=listReads()
  await page.waitForTimeout(1200)
  expect(listReads()).toBe(listed)
  listFail=0;capReady=true
 }

 // A viewer and a legacy sign-in mode see no Private skills actions at all (and a legacy mode keeps the existing imports button).
 const beforeReaders=requests.length
 viewer=true;await page.reload()
 await expect(card('workstation')).toBeVisible();await expand('workstation')
 await scopeRow('workstation',SHARED).getByText('Content observed',{exact:false}).click()
 await expect(page.getByRole('button',{name:/^Private skills/})).toHaveCount(0)
 await expect(page.getByRole('button',{name:/^Pin a library revision/})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Import a machine skill…',exact:true})).toHaveCount(0)
 viewer=false;authMode='none';await page.reload()
 await expect(card('workstation')).toBeVisible();await expand('workstation')
 await scopeRow('workstation',SHARED).getByText('Content observed',{exact:false}).click()
 await expect(page.getByRole('button',{name:'Import a machine skill…',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:/^Private skills/})).toHaveCount(0)
 await expect(page.getByRole('button',{name:/^Pin a library revision/})).toHaveCount(0)
 // Neither reads anything of terminals (R16): no terminal endpoint is called for a viewer or a legacy sign-in.
 expect(requests.slice(beforeReaders).filter(r=>r.path.startsWith('/api/v2/terminal/')).map(r=>`${r.method} ${r.path}`)).toEqual([])
 expect(unmocked).toEqual([])
 expect(errors).toEqual([])
 console.log('Skill content UI passed: entry points and reasons, read-only open and retry, create/change/remove with versions, conflicts, lost replies and 404s, busy guard, connector lapse, newest revision, labels not ids, stale-response fencing, focus return, storage writes, responsive layouts and viewer/legacy isolation; checks: readiness, request body and key reuse, queued to captured polling, every presence and ownership result, stale/superseded/expired/failed/cancelled/unknown states, pin re-read, lost replies and busy resume, hidden pause, close stops polling, summary rows read in full, aged reads, stop and dismiss; adopt and release: candidates with none chosen, one preselected, breakdown table with grouped differences at 256 files and 320px, stale-race fenced confirmation, exact request body and hash, double click, reply reconciled through the records, key reuse and renewal, plain refusals, release with versions, damaged records.')
}finally{await browser.close()}
