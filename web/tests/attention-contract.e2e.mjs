// The attention contract in the browser: one rule for Home's Waiting for you, the bell, the tab title and the workspace sidebar.
// A blocked chat stays counted after it is opened, a seen terminal bell does not count and an opened bell clears, an answer
// refreshes every surface at once (not on the next poll), and a failed list read keeps the last good count.
// Synthetic API answers; nothing is started on a machine.
import {chromium,expect} from '@playwright/test'
import {openWorkspace} from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const MIN=60_000,HOUR=60*MIN,now=Date.now()
const session=(id,title,over={})=>({id,agentId:'a1',title,state:'running',tool:'opencode',model:'fixture-model',dir:'/work',createdAt:now-2*HOUR,updatedAt:now-10*MIN,exitCode:null,...over})
// Already opened (changedSinceSeen false) but still holding an approval: it counts until it is answered.
const chat=()=>session('contract-chat-0001','Fix the build',{structured:true,attention:'waiting',attentionSource:'chat',bellAt:now-5*MIN,changedSinceSeen:false,pendingPermission:{permissionId:'perm-contract-0001',title:'go test',command:'go test ./...'}})
const failed=()=>session('contract-fail-0001','Docs pass',{state:'exited',exitCode:2,attention:'failed',attentionSource:'terminal',updatedAt:now-HOUR,changedSinceSeen:true})
// Unseen, but outside the 24-hour window.
const stale=()=>session('contract-done-0001','Old release notes',{state:'exited',exitCode:0,attention:'finished',attentionSource:'terminal',updatedAt:now-25*HOUR,changedSinceSeen:true})
// A terminal bell the person has already seen: nothing to resolve, so it does not count.
const bell=()=>session('contract-bell-0001','Tail the logs',{attention:'waiting',attentionSource:'terminal',bellAt:now-10*MIN,changedSinceSeen:false})
// A busy shell: running, never counted, but the server calls it unseen again whenever the terminal moves.
const busy=()=>session('contract-busy-0001','Build watcher',{attention:'working',attentionSource:'terminal',changedSinceSeen:false})
const machine={agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'dev',terminalControl:true,terminalEnabled:true,readiness:'ready',acp:true,acpHistory:true,acpLiveControls:true}}

async function fixture(){
 const box={sessions:[chat(),failed(),stale(),bell(),busy()],busy:false,stuck:'',failNext:0,failed:0,hold:null,reads:0,seen:[],answers:[]}
 const routes={
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/machines':{machines:[machine]},
  '/api/v2/terminal/sessions':async()=>{
   box.reads++
   if(box.failNext>0){box.failNext--;box.failed++;return{status:500,json:{error:'down'}}}
   if(box.busy)box.sessions.find(x=>x.id==='contract-busy-0001').changedSinceSeen=true
   const then={sessions:structuredClone(box.sessions)}
   if(box.hold){const wait=box.hold;box.hold=null;await wait}
   return then
  },
 }
 for(const s of box.sessions){
  const path=`/api/v2/terminal/sessions/${s.id}`
  // As the server does it: seen records the time; a waiting session with nothing to answer stops waiting, an approval stays.
  routes[path+'/seen']=()=>{box.seen.push(s.id);const item=box.sessions.find(x=>x.id===s.id);if(box.stuck===s.id)return{seenAt:Date.now()};item.changedSinceSeen=false;if(item.attention==='waiting'&&!item.pendingPermission)item.attention='working';return{seenAt:Date.now()}}
  routes[path+'/chat/controls']={controls:{working:false,waiting:true,model:'fixture-model',options:[]}}
  routes[path+'/chat/queue']={queue:[]};routes[path+'/skills']={skills:[]}
 }
 // Answering the approval: the server clears the pending item and the chat goes back to work.
 routes['/api/v2/terminal/sessions/contract-chat-0001/chat/permissions/perm-contract-0001']=request=>{
  box.answers.push(request.postDataJSON());const item=box.sessions.find(x=>x.id==='contract-chat-0001')
  delete item.pendingPermission;item.attention='working';return{answered:true}
 }
 const w=await openWorkspace(browser,{width:1440,height:950,terminalStatus:{configured:true,available:true},terminalMachines:[machine],routes})
 // Every 15-second poll (the bell's, Home's, the Sessions list's and the workspace's) runs only when the test says so, so a
 // count that changes after an answer can only have come from the answer itself.
 await w.page.addInitScript(()=>{
  const set=window.setInterval.bind(window),clear=window.clearInterval.bind(window),polls=new Map()
  window.setInterval=(f,d,...a)=>{if(d===15000){const id=set(()=>{},2**30);polls.set(id,()=>f(...a));return id}return set(f,d,...a)}
  window.clearInterval=id=>{polls.delete(id);clear(id)}
  window.__poll=()=>{for(const f of [...polls.values()])f()}
  window.EventSource=class{constructor(){setTimeout(()=>this.onopen?.({}),0)}addEventListener(){}close(){}}
 })
 return{...w,box}
}
const bellButton=(page,n)=>page.getByRole('button',{name:`Needs you: ${n}`,exact:true})
const poll=page=>page.evaluate(()=>window.__poll())
async function surfaces(page,n,title){
 await expect(bellButton(page,n)).toBeVisible()
 if(n>0)await expect(bellButton(page,n).locator('.badge')).toHaveText(String(n))
 await expect.poll(()=>page.title()).toBe(n>0?`(${n}) ${title} · reet.sh`:`${title} · reet.sh`)
}
// Home's Waiting for you: its header's count and its items, one per thing that waits.
async function homeCounts(page,n){
 const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
 await expect(waiting.locator('.head .count')).toHaveText(`· ${n}`)
 await expect(waiting.locator('.waiting-list > li')).toHaveCount(n)
 await surfaces(page,n,'Home')
}
const sidebar=page=>page.getByRole('complementary',{name:'Session navigation'})
async function showSidebar(page){if(await sidebar(page).count()===0)await page.locator('.workspace-actions').getByRole('button',{name:/^(Sessions|Show sessions)/}).click()}
async function sidebarCount(page,n){
 const badge=sidebar(page).locator('header .need-badge')
 await expect(badge).toHaveText(String(n))
 await expect(badge).toHaveAttribute('aria-label',n===1?'1 needs you':`${n} need you`)
}

try{
 const w=await fixture(),page=w.page,box=w.box
 // Home: the blocked chat (already opened) and the failed session count; the 25-hour-old finish and the seen bell do not.
 await page.goto(base+'/')
 await homeCounts(page,2)
 const cards=page.getByRole('region',{name:'Waiting for you',exact:true}).locator('.waiting-list > li')
 // The blocked chat holds an approval: its item says so with its chip and offers Allow once.
 await expect(cards.filter({hasText:'Fix the build'}).locator('.chip')).toHaveText('Approval')
 await expect(cards.filter({hasText:'Fix the build'}).getByRole('button',{name:'Allow once'})).toBeVisible()
 await expect(cards.filter({hasText:'Docs pass'})).toContainText('Exited with an error')
 await expect(cards.filter({hasText:'Old release notes'})).toHaveCount(0)
 await expect(cards.filter({hasText:'Tail the logs'})).toHaveCount(0)

 // A new bell counts while unseen; opening it clears it again.
 box.sessions.find(s=>s.id==='contract-bell-0001').changedSinceSeen=true
 await poll(page)
 await homeCounts(page,3)
 // Open in a new tab: Home stays, so what Home itself does on Open is what is seen here.
 const stay=()=>page.evaluate(()=>window.addEventListener('click',e=>{if(e.target.closest?.('a.btn[href^="/sessions?"]'))e.preventDefault()},{once:true}))
 await stay();await cards.filter({hasText:'Tail the logs'}).getByRole('link',{name:'Open Tail the logs',exact:true}).click()
 await expect.poll(()=>box.seen).toContain('contract-bell-0001')
 await homeCounts(page,2)

 // Opening the blocked chat marks it seen, and it stays counted: not even for a moment does any count drop, while the
 // read that follows the seen mark is held.
 await page.evaluate(()=>{
  window.__counts=[]
  const record=()=>window.__counts.push([document.querySelector('section[aria-label="Waiting for you"] .head .count')?.textContent.replace('· ',''),document.title,document.querySelector('.bell-button')?.getAttribute('aria-label')].join('|'))
  new MutationObserver(record).observe(document,{subtree:true,childList:true,characterData:true,attributes:true,attributeFilter:['aria-label']})
  record()
 })
 let release;box.hold=new Promise(r=>release=r)
 const readsBefore=box.reads
 await stay();await cards.filter({hasText:'Fix the build'}).getByRole('link',{name:'Open Fix the build',exact:true}).click()
 await expect.poll(()=>box.seen).toContain('contract-chat-0001')
 await expect.poll(()=>box.reads).toBeGreaterThan(readsBefore)
 await page.waitForTimeout(300)
 await homeCounts(page,2)
 release()
 await page.waitForTimeout(300)
 await homeCounts(page,2)
 const counts=await page.evaluate(()=>window.__counts)
 expect(counts.length).toBeGreaterThan(0)
 expect(counts.filter(c=>c!=='2|(2) Home · reet.sh|Needs you: 2')).toEqual([])

 // A list read that fails keeps the last good count on Home.
 box.failNext=1;await poll(page)
 await expect.poll(()=>box.failed).toBe(1)
 await page.waitForTimeout(300)
 await homeCounts(page,2)

 // The same count on the Sessions list and on any other page.
 await page.goto(base+'/sessions')
 await expect(page.getByRole('heading',{name:'Sessions',level:1})).toBeVisible()
 await surfaces(page,2,'Sessions')
 await page.goto(base+'/skills')
 await surfaces(page,2,'Skills')

 // The workspace, on the blocked chat: opening it marks it seen and the sidebar badge, bell and title still say 2.
 await page.goto(`${base}/sessions?agent=a1&session=contract-chat-0001`)
 await expect(page.locator('.session-screen .title')).toHaveText('Fix the build')
 await showSidebar(page)
 await sidebarCount(page,2)
 await surfaces(page,2,'Fix the build · studio')
 await expect(sidebar(page).locator('li.needs')).toHaveCount(2)
 await expect(sidebar(page).locator('li.needs',{hasText:'Fix the build'})).toHaveCount(1)
 await expect(sidebar(page).locator('li.needs',{hasText:'Docs pass'})).toHaveCount(1)
 await expect(sidebar(page).locator('li.needs',{hasText:'Old release notes'})).toHaveCount(0)
 await expect.poll(()=>box.seen.filter(id=>id==='contract-chat-0001').length).toBeGreaterThan(1)
 await poll(page)
 await page.waitForTimeout(300)
 await sidebarCount(page,2)
 await surfaces(page,2,'Fix the build · studio')

 // Answer the approval from the bell: every surface drops to 1 at once. The polls never run here, so only the answer's
 // refresh can have done it.
 await bellButton(page,2).click()
 const panel=page.getByRole('banner').getByRole('region',{name:'Needs you'})
 await panel.locator('.item',{hasText:'Fix the build'}).getByRole('button',{name:'Allow once',exact:true}).click()
 await expect.poll(()=>box.answers).toEqual([{allow:true}])
 await expect(bellButton(page,1)).toBeVisible({timeout:2000})
 await expect(sidebar(page).locator('header .need-badge')).toHaveText('1',{timeout:2000})
 await expect.poll(()=>page.title(),{timeout:2000}).toBe('(1) Fix the build · studio · reet.sh')
 await sidebarCount(page,1)
 await page.keyboard.press('Escape')

 // A list read that fails keeps the last good count on the bell, the title and the sidebar.
 const failedBefore=box.failed
 box.failNext=2;await poll(page)
 await expect.poll(()=>box.failed).toBe(failedBefore+2)
 await page.waitForTimeout(300)
 await sidebarCount(page,1)
 await surfaces(page,1,'Fix the build · studio')
 await poll(page);await page.waitForTimeout(300)
 await sidebarCount(page,1)
 await surfaces(page,1,'Fix the build · studio')
 // An unseen terminal bell comes in (the count goes 1 to 2); clicking its row in the workspace sidebar opens it, which marks it
 // seen. The polls are still frozen, so the sidebar badge, the bell and the title can only drop to 1 because opening the
 // session told them.
 // Let any trailing refresh from the answer above run out first, so it cannot stand in for the signal under test.
 await page.waitForTimeout(1300)
 // (Opening it earlier on Home marked it seen and, as the server does, took it off waiting, so it rings again.)
 Object.assign(box.sessions.find(s=>s.id==='contract-bell-0001'),{changedSinceSeen:true,attention:'waiting',bellAt:Date.now()})
 await poll(page)
 await sidebarCount(page,2)
 await surfaces(page,2,'Fix the build · studio')
 const bellRow=sidebar(page).locator('li.needs',{hasText:'Tail the logs'})
 await expect(bellRow).toHaveCount(1)
 const seenBefore=box.seen.filter(id=>id==='contract-bell-0001').length
 await bellRow.locator('button.session-row').click()
 await expect(page.locator('.session-screen .title')).toHaveText('Tail the logs')
 await expect.poll(()=>box.seen.filter(id=>id==='contract-bell-0001').length).toBeGreaterThan(seenBefore)
 await expect(sidebar(page).locator('header .need-badge')).toHaveText('1',{timeout:2000})
 await expect(bellButton(page,1)).toBeVisible({timeout:2000})
 await expect.poll(()=>page.title(),{timeout:2000}).toBe('(1) Tail the logs · studio · reet.sh')
 await sidebarCount(page,1)
 // What is left is the failed session: it is the one marked row, and it has no row to answer.
 await expect(sidebar(page).locator('li.needs')).toHaveCount(1)
 await expect(sidebar(page).locator('li.needs',{hasText:'Docs pass'})).toHaveCount(1)
 // A busy shell is unseen again whenever its terminal moves, but it is never counted: opening it marks it seen once and
 // signals nothing, so the refresh does not re-mark it in a loop.
 box.busy=true
 await page.goto(`${base}/sessions?agent=a1&session=contract-busy-0001`)
 await expect(page.locator('.session-screen .title')).toHaveText('Build watcher')
 await expect.poll(()=>box.seen.filter(id=>id==='contract-busy-0001').length).toBeGreaterThan(0)
 const readsAtOpen=box.reads
 await page.waitForTimeout(3500)
 expect(box.seen.filter(id=>id==='contract-busy-0001').length).toBeLessThanOrEqual(2)
 expect(box.reads-readsAtOpen).toBeLessThanOrEqual(2)
 // A counted session the server keeps calling unseen (a bell that keeps ringing): opening it marks it seen on every
 // refresh but signals once, so the refreshes do not feed each other.
 box.busy=false;box.stuck='contract-bell-0001'
 Object.assign(box.sessions.find(s=>s.id==='contract-bell-0001'),{changedSinceSeen:true,attention:'waiting',bellAt:Date.now()})
 await page.goto(`${base}/sessions?agent=a1&session=contract-bell-0001`)
 await expect(page.locator('.session-screen .title')).toHaveText('Tail the logs')
 await expect.poll(()=>box.seen.filter(id=>id==='contract-bell-0001').length).toBeGreaterThan(0)
 const stuckReads=box.reads
 await page.waitForTimeout(3500)
 expect(box.reads-stuckReads).toBeLessThanOrEqual(8)
 expect(w.errors).toEqual([])
 await w.context.close()
 // One rule on every page, with a Factory decision as well as a failed session: Home, another page and the Sessions page
 // all count 2, the title says (2), and the bell lists exactly what it counts, the pipeline with a link to its detail.
 {
  const oid='e'.repeat(32),cid='c'.repeat(32),pid='a'.repeat(32)
  const pins={providerId:pid,providerRevision:1,model:'m',contextRevision:1,workerRevision:1}
  const goal={...pins,id:'1'.repeat(32),conversationId:cid,sessionId:'factory-work-0001',revision:1,text:'x',state:'needs_user',note:'Gaffer brought a question back.',maxReviews:12,usedReviews:1,expiresAt:now+HOUR,createdAt:now-HOUR,updatedAt:now-20*MIN}
  const outcome={id:oid,revision:1,objective:'Ship CSV export.',state:'settled',note:'',agentId:'a1',projectId:'',projectName:'',dir:'/work',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-HOUR,updatedAt:now,sessions:[
   {key:'a',title:'Export API',prompt:'p',state:'following',sessionState:'running',note:'',sessionId:'factory-work-0001',conversationId:cid,updatedAt:now,stage:0,stages:[{kind:'code',tool:'opencode',state:'running',sessionId:'factory-work-0001',conversationId:cid},{kind:'review',tool:'opencode',state:'pending'}]}]}
  const routes={
   '/api/v2/terminal/status':{configured:true,available:true,reason:''},
   '/api/v2/terminal/machines':{machines:[machine]},
   '/api/v2/terminal/sessions':{sessions:[failed(),session('factory-work-0001','Export API · code',{structured:true,attention:'working',attentionSource:'chat'})]},
   '/api/v2/gaffer/outcomes':{outcomes:[outcome]},
   '/api/v2/gaffer/goals':{goals:[{goal,session:{id:'factory-work-0001',agentId:'a1',title:'Export API · code',state:'running'},available:true,attention:true,reason:'Gaffer brought a question back.'}],readAt:now},
  }
  const f=await openWorkspace(browser,{width:1440,height:950,terminalStatus:{configured:true,available:true},terminalMachines:[machine],routes})
  try{
   const page=f.page
   for(const [path,title] of [['/','Home'],['/skills','Skills'],['/sessions','Sessions']]){
    await page.goto(base+path)
    await surfaces(page,2,title)
    await bellButton(page,2).click()
    const pop=page.getByRole('banner').getByRole('region',{name:'Needs you'})
    await expect(pop.locator('.item'),`${title}: the bell lists what it counts`).toHaveCount(2)
    await expect(pop.getByRole('link',{name:'Open Export API in the Factory',exact:true})).toHaveAttribute('href',`/factory?pipeline=${oid}.a`)
    await expect(pop.locator('.item',{hasText:'Docs pass'})).toHaveCount(1)
    await page.keyboard.press('Escape')
   }
   expect(f.errors).toEqual([])
  }finally{await f.context.close()}
 }
 // The Factory page and the bell keep each other current. Every poll (the bell's 15 s, the page's 5 s list and 15 s
 // sessions) runs only when the test says so, so what changes can only have come from the signal.
 {
  const oid='f'.repeat(32),cid='9'.repeat(32),pid='a'.repeat(32)
  const pins={providerId:pid,providerRevision:1,model:'m',contextRevision:1,workerRevision:1}
  const box={merges:[],answers:[],outcomeReads:0,goalAttention:true}
  const stage=(kind,state,extra={})=>({kind,tool:'opencode',state,...extra})
  const outcome={id:oid,revision:3,objective:'Ship CSV export.',state:'settled',note:'',agentId:'a1',projectId:'',projectName:'',dir:'/work',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-2*HOUR,updatedAt:now,sessions:[
   {key:'a',title:'Export docs',prompt:'p',state:'ready_to_merge',sessionState:'running',note:'Ready to merge.',sessionId:'factory-merge-0001',conversationId:'8'.repeat(32),updatedAt:now-50*MIN,stage:1,stages:[stage('code','done'),stage('merge','pending')]},
   {key:'b',title:'Export API',prompt:'p',state:'following',sessionState:'running',note:'',sessionId:'factory-perm-0001',conversationId:cid,updatedAt:now,stage:0,stages:[stage('code','running',{sessionId:'factory-perm-0001',conversationId:cid}),stage('review','pending')]}]}
  const goal=()=>({...pins,id:'1'.repeat(32),conversationId:cid,sessionId:'factory-perm-0001',revision:1,text:'x',state:'active',note:'The agent is waiting for your approval.',waitingOnWorker:box.goalAttention,maxReviews:12,usedReviews:1,expiresAt:now+HOUR,createdAt:now-HOUR,updatedAt:now-20*MIN})
  let permission={permissionId:'perm-factory-0001',title:'Run a command',command:'npm run check'}
  const routes={
   '/api/v2/terminal/status':{configured:true,available:true,reason:''},
   '/api/v2/terminal/machines':{machines:[machine]},
   '/api/v2/terminal/sessions':()=>({sessions:[session('factory-merge-0001','Export docs · merge',{structured:true,attention:'working',attentionSource:'chat'}),session('factory-perm-0001','Export API · code',{structured:true,attention:permission?'waiting':'working',attentionSource:'chat',changedSinceSeen:false,...(permission?{pendingPermission:permission}:{})})]}),
   '/api/v2/gaffer/outcomes':()=>{box.outcomeReads++;return{outcomes:[outcome]}},
   '/api/v2/gaffer/goals':()=>({goals:[{goal:goal(),session:{id:'factory-perm-0001',agentId:'a1',title:'Export API · code',state:'running'},available:true,attention:box.goalAttention,...(box.goalAttention?{reason:'The agent is waiting for your approval.'}:{})}],readAt:Date.now()}),
   [`/api/v2/gaffer/outcomes/${oid}/sessions/a/merge`]:request=>{box.merges.push(request.postDataJSON());Object.assign(outcome.sessions[0],{state:'following',mergeApproved:true});outcome.revision++;return outcome},
   '/api/v2/terminal/sessions/factory-perm-0001/chat/permissions/perm-factory-0001':request=>{box.answers.push(request.postDataJSON());permission=undefined;box.goalAttention=false;return{answered:true}},
  }
  const f=await openWorkspace(browser,{width:1440,height:950,terminalStatus:{configured:true,available:true},terminalMachines:[machine],routes})
  await f.page.addInitScript(()=>{
   const set=window.setInterval.bind(window),clear=window.clearInterval.bind(window),polls=new Map()
   window.setInterval=(fn,d,...a)=>{if(d===15000||d===15_000||d===5000){const id=set(()=>{},2**30);polls.set(id,()=>fn(...a));return id}return set(fn,d,...a)}
   window.clearInterval=id=>{polls.delete(id);clear(id)}
   window.EventSource=class{constructor(){setTimeout(()=>this.onopen?.({}),0)}addEventListener(){}close(){}}
  })
  try{
   const page=f.page
   await page.goto(base+'/factory')
   const main=page.getByRole('main')
   await expect(main.getByRole('button',{name:'Approval: Export API',exact:true})).toBeVisible()
   await expect(bellButton(page,2)).toBeVisible()
   await expect.poll(()=>page.title()).toMatch(/^\(2\) /)
   // Approving the merge on the page drops the bell and the title at once.
   await main.getByRole('button',{name:/^Approve the merge/}).first().click()
   const readsAfter=box.outcomeReads
   await page.getByRole('dialog',{name:'Merge “Export docs”?'}).getByRole('button',{name:'Approve merge',exact:true}).click()
   await expect.poll(()=>box.merges.length).toBe(1)
   await expect(bellButton(page,1)).toBeVisible({timeout:2000})
   await expect.poll(()=>page.title(),{timeout:2000}).toMatch(/^\(1\) /)
   // Its own signal does not make the page read twice: the action's read, the bell's, and once each when the signal settles.
   await page.waitForTimeout(1600)
   expect(box.outcomeReads-readsAfter).toBeLessThanOrEqual(4)
   // Answering the approval in the bell takes the page's Approval item away, without a poll.
   await bellButton(page,1).click()
   const panel=page.getByRole('banner').getByRole('region',{name:'Needs you'})
   await panel.locator('.item',{hasText:'Export API'}).getByRole('button',{name:'Allow once',exact:true}).click()
   await expect.poll(()=>box.answers.length).toBe(1)
   await expect(main.getByRole('button',{name:'Approval: Export API',exact:true})).toHaveCount(0,{timeout:2000})
   await expect(bellButton(page,0)).toBeVisible({timeout:2000})
   expect(f.errors).toEqual([])
  }finally{await f.context.close()}
 }
 console.log('Attention contract: Home header, Needs you, bell and title agree at 2 with the Sessions list, another page and the workspace sidebar; a seen bell does not count and an opened bell clears; an opened blocked chat stays counted without a flicker; answering from the bell and opening an unseen bell in the workspace drop every surface to 1 without a poll; a failed list read keeps the last count; on the Factory page an approved merge drops the bell and title at once, and an approval answered in the bell leaves the page without a poll.')
}finally{await browser.close()}
