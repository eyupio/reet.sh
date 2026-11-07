import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const context=await browser.newContext({viewport:{width:1360,height:900},permissions:['clipboard-read','clipboard-write']})
const page=await context.newPage(),errors=[],inputs=[],tickets=[]
page.on('pageerror',error=>errors.push(error.message))
const now=Date.now(),agentA='connector-laptop000000001',agentB='connector-builder00000001',sessionA='session-build00000000001',sessionB='session-deploy0000000001'
let denied=false
// Plain terminals carry attention as the server lists it: quiet and seen needs nothing.
const quiet={attention:'quiet',attentionSource:'terminal',changedSinceSeen:false}
const sessions=[{id:sessionA,agentId:agentA,title:'Build workspace',state:'running',createdAt:now,updatedAt:now,exitCode:null,...quiet},{id:sessionB,agentId:agentB,title:'Deployment checks',state:'running',createdAt:now,updatedAt:now,exitCode:null,...quiet}]
// A user-ended plain terminal as the server reports it: ended with no exit code (the machine kills the shell, see
// internal/terminalhost/manager.go), no attention word for an ended session (internal/store/store_attention.go), and
// updated_at past the seen time. It must raise no card.
const ended=item=>{item.state='ended';item.exitCode=null;delete item.attention;item.changedSinceSeen=true;item.updatedAt=Date.now()}
const seen=[]
const machines=[{agentId:agentA,host:'dev-laptop',ready:true,serverEnabled:true,capabilities:{osUser:'developer',terminalEnabled:true,terminalControl:true,readiness:'ready'}},{agentId:agentB,host:'build-server',ready:true,serverEnabled:true,capabilities:{osUser:'builder',terminalEnabled:true,terminalControl:true,readiness:'ready'}}]
const operations=[],receipts=new Map(),mutations=[],archived=new Set(),archiveRequests=[]
let failArchive=false
let sequence=0,failFirstCreate=true,claimed=false,listReads=0,pendingList=null,pendingMutation=null
const makeSession=(agentId,title)=>({id:`new-session-${String(++sequence).padStart(24,'0')}`,agentId,title,state:'running',createdAt:now,updatedAt:now,exitCode:null,...quiet})
machines.push({agentId:'connector-offline00000001',host:'offline workstation',ready:false,serverEnabled:true,capabilities:{osUser:'offline-user',terminalEnabled:true,terminalControl:true,readiness:'ready'}})
function hold(which){let entered,release;const gate={entered:new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(new Error(`No ${which} request reached the gate`)),10000);entered=()=>{clearTimeout(timer);resolve()}}),wait:new Promise(resolve=>release=resolve),release:()=>release(),mark:()=>entered(),reads:listReads};if(which==='list')pendingList=gate;else pendingMutation=gate;return gate}
const holdNextList=()=>hold('list'),holdNextMutation=()=>hold('mutation')
await page.route('**/api/**',async route=>{
 const request=route.request(),path=new URL(request.url()).pathname,method=request.method()
 let data={}
 if(path==='/api/v2/auth/status')data={mode:'accounts',user:{id:1,name:'Lifecycle tester',email:'test@example.test',role:'owner'}}
 else if(path==='/api/v2/profile')data={profile:{theme:'light',shelfView:'list',cardsExpanded:false}}
 else if(path==='/api/skills')data={skills:[],counts:{all:0,used:0,liked:0,bookmarked:0}}
 else if(path==='/api/environments')data={environments:[],machines:[]}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/sync')data={environments:[],machines:[],targets:[],agents:[],jobs:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'fixture',commit:'fixture'}
 else if(path==='/api/v2/machines/health')data={serverNow:Date.now(),machines:[],history:[]}
 else if(path==='/api/v2/terminal/machines')data={machines}
 else if(path==='/api/v2/terminal/status')data={available:true,configured:true,reason:''}
 else if(path==='/api/v2/terminal/sessions'){
  if(denied)return route.fulfill({status:403,json:{error:'Account terminal access revoked'}})
  if(method==='POST'){
   const operation=request.postDataJSON();operations.push(operation)
   let item=receipts.get(operation.requestId)
   if(!item){item=makeSession(operation.agentId,operation.title);receipts.set(operation.requestId,item)}
   if(failFirstCreate){failFirstCreate=false;return route.fulfill({status:503,json:{error:'Creation response interrupted'}})}
   if(!sessions.some(s=>s.id===item.id))sessions.push(item)
   return route.fulfill({json:item})
  }
  listReads++
  const snapshot=structuredClone(sessions.filter(item=>!archived.has(item.id))),gate=pendingList;pendingList=null
  if(gate){gate.mark();await gate.wait}
  data={sessions:snapshot}
 }
 else if(path.endsWith('/attachments')){
  const id=path.split('/').at(-2),body=request.postDataJSON();tickets.push({id,takeover:body.takeover})
  if(claimed&&!body.takeover)return route.fulfill({status:409,json:{error:'Session attached elsewhere'}})
  claimed=false;data={ticket:'fixture-ticket00000000000'}
 }
 else if(path.endsWith('/archive')){
  const id=path.split('/').at(-2),item=sessions.find(item=>item.id===id);archiveRequests.push(id)
  if(!item||!['end_requested','ended','exited','lost'].includes(item.state))return route.fulfill({status:409,json:{error:'Request an end before removing the session'}})
  if(failArchive)return route.fulfill({status:503,json:{error:'History removal unavailable'}})
  const gate=pendingMutation;pendingMutation=null;if(gate){gate.mark();await gate.wait}
  archived.add(id);data={ok:true}
 }
 else if(path.endsWith('/seen')){const id=path.split('/').at(-2),item=sessions.find(s=>s.id===id);seen.push(id);if(item)item.changedSinceSeen=false;data={seenAt:Date.now()}}
 else if(method==='PATCH'||path.endsWith('/end')){
  const id=path.split('/').at(path.endsWith('/end')?-2:-1),body=request.postDataJSON(),item=sessions.find(s=>s.id===id)
  const gate=pendingMutation;pendingMutation=null;if(gate){gate.mark();await gate.wait}
  if(method==='PATCH'){item.title=body.title;mutations.push({kind:'rename',id,...body})}
  else{if(machines.find(m=>m.agentId===item.agentId).ready)ended(item);else item.state='end_requested';mutations.push({kind:'end',id,...body})}
  data={ok:true}
 }
 else if(path.endsWith('/history'))return route.fulfill({body:'Read-only history'})
 else return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})
 await route.fulfill({json:data})
})
await page.routeWebSocket('**/api/v2/terminal/socket',socket=>{
 socket.onMessage(message=>{if(typeof message==='string'){if(JSON.parse(message).ticket){socket.send('{"type":"ready"}');socket.send(Buffer.from('Existing shell\r\nready> '))}}else{inputs.push(message.toString());socket.send(message)}})
})
// The 15-second polls still run; this also lets the test run them now, so an attention count is read after a fresh list.
await page.addInitScript(()=>{const set=window.setInterval.bind(window),clear=window.clearInterval.bind(window),polls=new Map();window.setInterval=(f,d,...a)=>{const id=set(f,d,...a);if(d===15000)polls.set(id,()=>f(...a));return id};window.clearInterval=id=>{polls.delete(id);clear(id)};window.__poll=()=>{for(const f of [...polls.values()])f()}})
// The attention count every surface shows, read after the bell's and the workspace's lists are read again: the tab title's
// prefix, the bell, and the workspace sidebar's badge when the sidebar is on screen.
async function needs(n){
 const before=listReads;await page.evaluate(()=>window.__poll())
 await expect.poll(()=>listReads).toBeGreaterThan(before)
 await page.waitForTimeout(300)
 await expect.poll(()=>page.title()).toMatch(n?new RegExp(`^\\(${n}\\) `):/^[^(]/)
 const bell=page.getByRole('button',{name:/^Needs you: /})
 if(await bell.count())await expect(bell).toHaveAttribute('aria-label',`Needs you: ${n}`)
 const side=page.getByRole('complementary',{name:'Session navigation'})
 if(await side.count()){const badge=side.locator('header .need-badge');if(n)await expect(badge).toHaveText(String(n));else await expect(badge).toHaveCount(0)}
}
const pane=page.locator('.pane')
const selected=()=>new URL(page.url()).searchParams.get('session')
async function live(){await expect(pane).toHaveAttribute('data-connection-state','live')}
async function chooseMachine(id){const host=machines.find(m=>m.agentId===id)?.host||id;await page.getByRole('button',{name:'Find machine',exact:true}).click();await page.getByLabel('Machine or shell user').fill(host);await page.getByRole('navigation',{name:'Matching machines'}).getByRole('button',{name:new RegExp(`^${host.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')}`)}).click()}
async function create(name){await page.getByRole('button',{name:'New session',exact:true}).click();await page.getByRole('button',{name:'Terminal session',exact:true}).click();await page.getByLabel('New session name').fill(name);await page.getByRole('button',{name:'New terminal',exact:true}).click()}
async function refresh(){const toggle=page.locator('.workspace-actions').getByRole('button',{name:/^(Sessions|Show sessions)/});if(await toggle.getAttribute('aria-expanded')!=='true')await toggle.click();await page.getByRole('button',{name:'Refresh status',exact:true}).click()}
async function manage(name){await page.locator('.session-menu summary').click();await page.getByRole('button',{name,exact:true}).click()}
try{
 await page.goto(`${base}/sessions?agent=${agentA}&session=${sessionA}`);await live()
 await needs(0)
 // An accepted creation with an ambiguous response stays bound to A after working on B.
 await create('Uncertain A')
 await expect(page.getByRole('dialog')).toContainText('Creation response interrupted')
 const originalOperation=operations[0]
 await page.getByRole('button',{name:'Close New terminal session'}).click()
 await chooseMachine(agentB);await live()
 await create('Fresh B');await live();await expect(pane).toContainText('Fresh B')
 expect(operations[1].agentId).toBe(agentB)
 expect(operations[1].requestId).not.toBe(originalOperation.requestId)
 await chooseMachine(agentA);await live()
 await page.getByRole('button',{name:'New session',exact:true}).click()
 await expect(page.getByLabel('New session name')).toHaveValue('Uncertain A')
 await expect(page.getByLabel('New session name')).toBeDisabled()
 await page.getByRole('button',{name:'Retry creation',exact:true}).click()
 await live();await expect(pane).toContainText('Uncertain A')
 expect(operations[2]).toEqual(originalOperation)
 expect(receipts.size).toBe(2)
 const uncertain=selected()
 // A response captured before a successful write cannot erase its resulting session.
 const gate=holdNextList()
 await refresh();await gate.entered
 await create('During delayed poll');await live();await expect(pane).toContainText('During delayed poll')
 const newest=selected()
 gate.release()
 await expect.poll(()=>listReads).toBeGreaterThan(gate.reads+1)
 await expect(pane).toContainText('During delayed poll')
 expect(selected()).toBe(newest)
 // Search keeps unavailable machines discoverable; selecting never creates a shell.
 const writesBeforeSearch=operations.length
 await page.getByRole('button',{name:'Find machine',exact:true}).click()
 await expect(page.getByLabel('Machine or shell user')).toBeFocused()
 await page.getByLabel('Machine or shell user').fill('builder')
 const matches=page.getByRole('navigation',{name:'Matching machines'})
 await expect(matches.getByRole('button')).toHaveCount(1)
 await matches.getByRole('button',{name:/build-server/}).click();await live()
 expect(operations.length).toBe(writesBeforeSearch)
 await page.setViewportSize({width:390,height:844})
 await expect(page.getByRole('complementary',{name:'Session navigation'})).toHaveCount(0)
 await expect.poll(async()=>{const b=await page.locator('.session-menu summary').boundingBox();return Math.round(b.x+b.width)}).toBeLessThanOrEqual(390)
 await expect(pane.locator('.host')).toBeVisible();await expect(pane.locator('.host')).toContainText('builder@build-server')
 if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/lifecycle-phone.png`})}
 await page.setViewportSize({width:1360,height:900})
 await page.getByRole('button',{name:'Find machine',exact:true}).click()
 await page.getByLabel('Machine or shell user').fill('offline')
 await expect(matches).toContainText('reconnecting')
 await page.getByRole('button',{name:'Close Find a machine'}).click()
 // Rename response arrives after switching machines; target and selection stay independent.
 await chooseMachine(agentA);await live()
 const renamedId=selected(),renameGate=holdNextMutation()
 await manage('Rename');await page.getByRole('dialog').getByLabel('Session name').fill('Renamed A')
 await page.getByRole('button',{name:'Save name',exact:true}).click();await renameGate.entered
 await chooseMachine(agentB);await live()
 renameGate.release()
 await expect.poll(()=>mutations.length).toBe(1)
 expect(mutations[0].id).toBe(renamedId)
 await expect(pane).toContainText('Fresh B')
 await chooseMachine(agentA);await live()
 await expect(pane).toContainText('Renamed A')
 // Offline end stays pending. Retry reuses its ID; another shell is unaffected.
 machines[0].ready=false;await refresh()
 await manage('End session');await expect(page.getByRole('dialog')).toContainText('Renamed A')
 await page.getByRole('dialog').getByRole('button',{name:'End session',exact:true}).click()
 await expect(page.getByText(/End requested. Waiting for this machine/)).toBeVisible()
 // A pending end holds the view and counts nothing.
 await needs(0)
 await page.locator('.session-menu summary').click()
 await page.getByRole('button',{name:'Remove from history',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('end request stays queued')
 await expect(page.getByRole('dialog')).toContainText('still counts towards its limit')
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 await manage('Retry end request');await page.getByRole('dialog').getByRole('button',{name:'End session',exact:true}).click()
 await expect.poll(()=>mutations.filter(m=>m.kind==='end').length).toBe(2)
 const ends=mutations.filter(m=>m.kind==='end');expect(ends[0]).toEqual(ends[1]);expect(ends[0].id).toBe(renamedId)
 await chooseMachine(agentB);await live()
 expect(sessions.find(item=>item.id===sessionB).state).toBe('running')
 machines[0].ready=true;ended(sessions.find(item=>item.id===renamedId))
 // Closed-history removal works while execution is disabled and preserves a newer selection.
 const builderBeforeArchive=selected()
 await refresh();expect(selected()).toBe(builderBeforeArchive)
 // The person's own end of a plain terminal, now confirmed, raises no Failed or Finished card and no count.
 await needs(0)
 // Explicitly reopening history may show an ended session; a delayed end must not steal the newer selection.
 await page.goto(`${base}/sessions?agent=${agentA}&session=${renamedId}`)
 await expect(pane).toHaveAttribute('data-connection-state','ended')
 await needs(0)
 machines[0].serverEnabled=false;machines[0].ready=false;await refresh()
 await manage('Remove from history')
 await expect(page.getByRole('dialog')).toContainText('Files on the machine are kept')
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 const closedRow=page.getByRole('navigation',{name:'Terminal sessions'}).getByRole('button',{name:/^Renamed A/})
 await expect(closedRow).toBeVisible();expect(archiveRequests).toHaveLength(0)
 failArchive=true
 await manage('Remove from history');await page.getByRole('dialog').getByRole('button',{name:'Remove from history',exact:true}).click()
 await expect(page.getByRole('alert')).toContainText('History removal unavailable')
 await expect(closedRow).toBeVisible()
 failArchive=false
 const archiveGate=holdNextMutation()
 await manage('Remove from history');await page.getByRole('dialog').getByRole('button',{name:'Remove from history',exact:true}).click();await archiveGate.entered
 await chooseMachine(agentB);await live()
 const selectedBeforeArchiveRelease=selected()
 archiveGate.release()
 await expect.poll(()=>archived.has(renamedId)).toBe(true)
 expect(archiveRequests).toEqual([renamedId,renamedId])
 expect(selected()).toBe(selectedBeforeArchiveRelease);await live()
 machines[0].serverEnabled=true;machines[0].ready=true
 await page.reload();await live()
 await chooseMachine(agentA);await live()
 await expect(closedRow).toHaveCount(0)
 await page.locator('.session-menu summary').click()
 await expect(page.getByRole('button',{name:'Remove from history',exact:true})).toHaveCount(0)
 await page.locator('.session-menu summary').click()
 await chooseMachine(agentB);await live()
 // A stuck offline end can be removed without another stop or a replacement shell.
 const pending=makeSession(agentA,'Stuck offline shell');pending.state='end_requested';sessions.push(pending)
 machines[0].ready=false;machines[0].serverEnabled=false
 const beforePendingRemoval=operations.length,beforePendingEnd=mutations.filter(m=>m.kind==='end').length
 await page.goto(`${base}/sessions?agent=${agentA}&session=${pending.id}`)
 await expect(pane).toHaveAttribute('data-connection-state','ending')
 await needs(0)
 await page.getByRole('button',{name:'Session actions',exact:true}).click()
 await page.getByRole('dialog',{name:'Session actions',exact:true}).getByRole('button',{name:'Remove from history…',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('without confirming that it has stopped')
 await page.getByRole('dialog').getByRole('button',{name:'Remove from history',exact:true}).click()
 await expect.poll(()=>archived.has(pending.id)).toBe(true)
 await page.reload()
 await expect(page.getByRole('navigation',{name:'Terminal sessions'}).getByRole('button',{name:/^Stuck offline shell/})).toHaveCount(0)
 expect(pending.state).toBe('end_requested')
 expect(operations.length).toBe(beforePendingRemoval)
 expect(mutations.filter(m=>m.kind==='end').length).toBe(beforePendingEnd)
 machines[0].ready=true;machines[0].serverEnabled=true
 await chooseMachine(agentB);await live()
 // A busy lease requires an explicit transfer and preserves the session ID.
 claimed=true;await page.reload()
 await expect(pane).toHaveAttribute('data-connection-state','taken_over')
 const takeoverSession=selected(),beforeTakeover=operations.length
 await page.getByRole('button',{name:'Continue here',exact:true}).click();await live()
 expect(tickets.at(-1)).toEqual({id:takeoverSession,takeover:true})
 expect(operations.length).toBe(beforeTakeover)
 // A lost shell has recovery guidance and never creates a replacement.
 const lostId=selected(),beforeLost=operations.length
 // As the server lists it: a lost shell is a Connection lost card until the person has seen it.
 Object.assign(sessions.find(item=>item.id===lostId),{state:'lost',attention:'failed',changedSinceSeen:true,updatedAt:Date.now()})
 await needs(1)
 await page.reload()
 await expect(pane).toHaveAttribute('data-connection-state','lost')
 // Opening it marks it seen, and the card clears.
 await expect.poll(()=>seen).toContain(lostId)
 await needs(0)
 await expect(pane).toContainText('No replacement was created')
 expect(operations.length).toBe(beforeLost)
 // Quota blocks an extra creation, but existing sessions remain usable.
 await chooseMachine(agentA)
 for(let i=0;i<8;i++)sessions.push(makeSession(agentA,`Quota ${i}`))
 await refresh()
 await page.getByRole('button',{name:'New session',exact:true}).click()
 await page.getByRole('button',{name:'Terminal session',exact:true}).click()
 await expect(page.getByRole('dialog')).toContainText('Session limit reached')
 await expect(page.getByRole('button',{name:'New terminal',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'Close New terminal session'}).click()
 // Revocation removes the attachment and private inventory, with explicit guidance.
 denied=true;await refresh()
 await expect(page.getByRole('heading',{name:'Terminal access unavailable'})).toBeVisible()
 await expect(pane).toHaveCount(0)
 expect(errors).toEqual([])
 console.log('Terminal lifecycle passed: machine search/context, per-machine immutable retries, stale-poll fencing, rename targeting, pending end/retry, explicit takeover, quota, revocation and persistent closed-history removal without affecting another selected shell; attention counts stayed 0 across pending and confirmed ends and history reopening, and a lost shell counted once until seen.')
}catch(error){if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/lifecycle-failure.png`,fullPage:true})}throw error}finally{await browser.close()}
