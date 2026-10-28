import { chromium, expect } from '@playwright/test'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const SITE=new URL(base).origin
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const context=await browser.newContext({viewport:{width:1360,height:1000},permissions:['clipboard-read','clipboard-write']})
const page=await context.newPage()
const DAY=86400000,errors=[],writes=[],requests=[],unmocked=[]
page.on('pageerror',e=>errors.push(e.message))
const foreign=[]
page.on('request',r=>{const u=r.url();if(!u.startsWith(base)&&!u.startsWith('data:')&&!u.startsWith('blob:'))foreign.push(u)})
const deferred=()=>{let resolve;const promise=new Promise(r=>resolve=r);return{promise,resolve}}

// Server state. Scan times only ever grow, as the controller's do, so "a newer scan" is always a later stamp.
let aliasesOf={}   // saved aliases per canonical host, as the server keeps them after a rename or merge
let authMode='accounts',viewer=false,seq=0,emptied=false,syncStatus=0,syncHold=null,tokenMode='ok'
const stamp=()=>Date.now()+(++seq)
const scope=(host,over={})=>({id:`scope-${host}`,host,name:host,kind:'agent',agent:'agents',project:'',complete:true,scannedAt:stamp(),lastSeen:stamp(),skills:[],skillCount:1,content:{state:'observed',skills:[{name:'demo',complete:true,files:3,bytes:92,digest:'d'.repeat(64)}]},os:'linux',arch:'amd64',version:'dev',...over})
const connector=(id,host,paused=false)=>({id,host,version:'dev',paused,autoUpdate:false,lastSeen:Date.now()})
let scopes=[scope('workstation')],agents=[connector('agent-ws','workstation')]
const machinesOf=()=>{const by=new Map();for(const s of scopes)by.set(s.host,[...(by.get(s.host)??[]),s]);return[...by].map(([host,groups])=>({host,groups,skillCount:0,...(aliasesOf[host]?{aliases:aliasesOf[host]}:{})}))}
const reset=()=>{aliasesOf={};scopes=[scope('workstation')];agents=[connector('agent-ws','workstation')];syncStatus=0;syncHold=null;tokenMode='ok'}
const TOO_LONG='That name is too long. Use at most 200 bytes; accented letters and emoji take more than one.'
const SECRET='rst_SECRET_TOKEN_VALUE_9f3a'
const BUNDLE='d'.repeat(32),REV='1'.repeat(32)

await page.route('**/api/**',async route=>{
 const req=route.request(),path=new URL(req.url()).pathname,method=req.method(),body=method==='GET'?null:req.postDataJSON()
 requests.push({method,path})
 let data={}
 if(path==='/api/v2/auth/status')data=authMode==='accounts'?{mode:'accounts',setupRequired:false,bootstrapRequired:false,user:{id:1,name:'Operator',email:'operator@example.test',role:viewer?'viewer':'owner'}}:{mode:authMode,setupRequired:false,bootstrapRequired:false}
 else if(path==='/api/v2/profile')data={profile:{theme:'system',shelfView:'list'}}
 else if(path==='/api/skills')data={skills:[],counts:{all:0,used:0}}
 else if(path==='/api/environments')data={environments:scopes,machines:machinesOf()}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/sync'){
  if(syncHold)await syncHold.promise
  if(syncStatus===401)return route.fulfill({status:401,json:{error:'missing or wrong token'}})
  if(syncStatus)return route.fulfill({status:syncStatus,json:{error:'The machines could not be read right now.'}})
  data=emptied?{environments:[],machines:[],agents:[],targets:[],jobs:[]}:{environments:scopes,machines:machinesOf(),agents,targets:[],jobs:[]}
 }
 else if(path==='/api/v2/machines/health')data={serverNow:Date.now(),machines:[]}
 else if(path==='/api/v2/setup/profiles')data={profiles:[]}
 else if(path==='/api/v2/terminal/machines')data={machines:[]}
 // Machines reads the server's terminal status for a writer in accounts mode: the first-terminal checklist moved here from
 // Control when Control was folded into Home and Machines (R4, PR H). A viewer and the legacy modes never read it (checked
 // at the end of this file).
 else if(path==='/api/v2/terminal/status')data={configured:false,available:false,reason:'disabled'}
 else if(path==='/api/v2/folder-projects')data={projects:[],savedWaiting:0}
 else if(path==='/api/v2/terminal/sessions')data={sessions:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'v1.2.3',commit:'fixture'}
 else if(path==='/api/v2/skill-bundles')data={bundles:[{id:BUNDLE,name:'demo',head:REV,createdAt:Date.now()}]}
 else if(path===`/api/v2/skill-bundles/${BUNDLE}/revisions`)data={revisions:[{id:REV,createdAt:Date.now(),manifestDigest:'1'.repeat(64),origin:{kind:'user'}}]}
 else if(path==='/api/v2/skill-content-targets'&&method==='GET')data={targets:[]}
 else if(path==='/api/v2/skill-content-previews'&&method==='GET')data={previews:[],capabilities:[]}
 else if(path==='/api/v2/skill-content-receipts'&&method==='GET')data={receipts:[]}
 else if(path==='/api/v2/tokens'&&method==='POST'){
  writes.push({method,path,body})
  if(tokenMode==='drop'){tokenMode='ok';return route.abort('failed')}
  if(tokenMode==='refuse')return route.fulfill({status:500,json:{error:'Tokens cannot be made right now.'}})
  data={id:1,name:body.name,token:SECRET,createdAt:Date.now()}
 }
 else{unmocked.push(`${method} ${path}`);return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})}
 await route.fulfill({json:data})
})

// Every write to browser storage is recorded from before the page loads, so use of the dialog can be shown to make none.
await page.addInitScript(()=>{
 window.__storageCalls=[]
 for(const op of ['setItem','removeItem','clear']){const original=Storage.prototype[op];Storage.prototype[op]=function(...args){window.__storageCalls.push({area:this===window.localStorage?'local':this===window.sessionStorage?'session':'other',op,key:args[0]});return original.apply(this,args)}}
 const scrolled=[];window.__scrolled=scrolled
 const scroll=Element.prototype.scrollIntoView;Element.prototype.scrollIntoView=function(...args){scrolled.push(this.getAttribute('aria-labelledby')||this.id||this.tagName);return scroll.apply(this,args)}
})
// The dialog's poll timers (3, 6, 12 and 20 seconds) are counted while they are pending, with the delay each asked for, and
// fire after 50 ms so the file runs in seconds. The clock can be moved on (the watch cap, a scan that has aged).
await page.addInitScript(()=>{
 window.__poll={pending:new Set(),fired:0,scheduled:0,delays:[],max:0}
 const set=window.setTimeout.bind(window),clear=window.clearTimeout.bind(window)
 window.setTimeout=(fn,delay,...args)=>{
  if(![3000,6000,12000,20000].includes(delay))return set(fn,delay,...args)
  window.__poll.scheduled++;window.__poll.delays.push(delay)
  const id=set(()=>{window.__poll.pending.delete(id);window.__poll.fired++;fn(...args)},50)
  window.__poll.pending.add(id);window.__poll.max=Math.max(window.__poll.max,window.__poll.pending.size);return id
 }
 window.clearTimeout=id=>{window.__poll.pending.delete(id);clear(id)}
 const real=Date.now.bind(Date);window.__skew=0;Date.now=()=>real()+window.__skew
})
// The three-second continue is held by the fixture instead of run: it is tagged, so only it is taken, and fired by hand.
await page.addInitScript(()=>{
 window.__auto={pending:new Map(),all:[],delays:[]}
 const set=window.setTimeout,clear=window.clearTimeout
 const wrapped=window.setTimeout
 window.setTimeout=(fn,delay,...args)=>{
  if(args[0]!=='reet-auto-continue')return wrapped(fn,delay,...args)
  const id=set.call(window,()=>{},2147483647);window.__auto.pending.set(id,fn);window.__auto.all.push(fn);window.__auto.delays.push(delay);return id
 }
 const wrappedClear=window.clearTimeout
 window.clearTimeout=id=>{window.__auto.pending.delete(id);wrappedClear(id)}
})
// When each dialog was shown and when text first reached a status region inside it, so a note can be shown to arrive after the
// region exists (a region that is filled as it appears may not be announced). Also the behaviour each scroll asked for.
await page.addInitScript(()=>{
 window.__live={shown:[],text:[]};window.__scrollBehaviour=[]
 const show=HTMLDialogElement.prototype.showModal
 HTMLDialogElement.prototype.showModal=function(...args){const result=show.apply(this,args);window.__live.shown.push({title:this.querySelector('h2')?.textContent??'',at:performance.now()});return result}
 new MutationObserver(records=>{for(const r of records){const node=r.target.nodeType===3?r.target.parentElement:r.target;const el=node?.closest?.('.live[role="status"]');if(el&&el.textContent.trim())window.__live.text.push({text:el.textContent.trim(),at:performance.now()})}}).observe(document,{subtree:true,childList:true,characterData:true})
 const scroll=Element.prototype.scrollIntoView;Element.prototype.scrollIntoView=function(...args){window.__scrollBehaviour.push(args[0]?.behavior??'');return scroll.apply(this,args)}
})
const autoPending=()=>page.evaluate(()=>window.__auto.pending.size)
const autoDelays=()=>page.evaluate(()=>[...window.__auto.delays])
const fireAuto=()=>page.evaluate(()=>{const run=[...window.__auto.pending.values()];window.__auto.pending.clear();run.forEach(fn=>fn('reet-auto-continue'))})
const fireStale=()=>page.evaluate(()=>window.__auto.all.forEach(fn=>fn('reet-auto-continue')))
const pendingPolls=()=>page.evaluate(()=>window.__poll.pending.size)
const scheduled=()=>page.evaluate(()=>window.__poll.scheduled)
const fired=()=>page.evaluate(()=>window.__poll.fired)
const mostPending=()=>page.evaluate(()=>window.__poll.max)
const resetMost=()=>page.evaluate(()=>{window.__poll.max=window.__poll.pending.size})
const delays=()=>page.evaluate(()=>[...window.__poll.delays])
const syncReads=()=>requests.filter(r=>r.method==='GET'&&r.path==='/api/v2/sync').length
const afterRequests=()=>page.evaluate(()=>fetch('/api/version').then(r=>r.status))
// Nothing is polling: after a few more turns of the same timers none are pending and none were asked for.
const quiet=async()=>{
 await page.evaluate(()=>new Promise(done=>{let n=0;const turn=()=>++n<4?setTimeout(turn,3000):done();setTimeout(turn,3000)}))
 await afterRequests()
 const before=await scheduled()
 await page.evaluate(()=>new Promise(done=>{let n=0;const turn=()=>++n<4?setTimeout(turn,3000):done();setTimeout(turn,3000)}))
 await afterRequests()
 expect(await scheduled()).toBe(before+4)
 expect(await pendingPolls()).toBe(0)
 // And no read is in flight or made: the page's own five-second read may land in the window, a watch would make one per turn.
 const reads=syncReads()
 await page.evaluate(()=>new Promise(done=>{let n=0;const turn=()=>++n<4?setTimeout(turn,3000):done();setTimeout(turn,3000)}))
 // The one deliberate wait of real time in this file ("nothing happens" cannot be shown any other way): the turns above run on
 // quickened timers, so a read at a real pace could land after them.
 await page.waitForTimeout(1000)
 await afterRequests()
 expect(syncReads()-reads).toBeLessThanOrEqual(1)
}
const setHidden=hidden=>page.evaluate(h=>{for(const key of ['hidden','visibilityState'])Object.defineProperty(document,key,{configurable:true,get:()=>key==='hidden'?h:(h?'hidden':'visible')});document.dispatchEvent(new Event('visibilitychange'))},hidden)
const skew=ms=>page.evaluate(m=>{window.__skew=m},ms)
const layout=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
const storage=()=>page.evaluate(async()=>JSON.stringify({local:{...localStorage},session:{...sessionStorage},idb:(await indexedDB.databases()).map(d=>d.name).sort()}))
const storageCalls=()=>page.evaluate(()=>window.__storageCalls.length)
const card=host=>page.locator('article').filter({has:page.getByRole('heading',{name:host,exact:true})})
const q=s=>"'"+s.replaceAll("'","'\\''")+"'"
// The block written out in full, from the three existing builders' text, so it is checked against the words and not against the code.
const block=name=>[
 'curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh\nexport PATH="$HOME/.local/bin:$PATH"\nreet --version',
 `reet configure --url ${q(SITE)} --version latest`,
 `reet agent enroll --server ${q(SITE)} --env ${q(name)}\nreet agent run`,
].join('\n\n')

// Whitespace counts: the blank lines between the three parts are part of what is pasted.
const commandsAre=async(p,name)=>{await expect.poll(()=>p.commands.textContent()).toBe(block(name))}
const dialogOf=()=>page.getByRole('dialog',{name:'Add a machine',exact:true})
const addButton=()=>page.getByRole('button',{name:'Add a machine',exact:true})
// The dialog's parts, found by what a person sees.
const parts=dialog=>({
 name:dialog.getByLabel('Machine name',{exact:true}),
 commands:dialog.locator('pre.commands'),
 status:dialog.locator('.join-status'),
 use:name=>dialog.getByRole('button',{name:`Done — show ${name}`,exact:true}),
 anyUse:dialog.getByRole('button',{name:/^Done — show /}),
})
const words=async dialog=>{const text=await dialog.innerText();expect(text).not.toMatch(/verified|healthy/i);return text}
// Setting the value the way a paste would, which can carry what typing cannot.
const paste=async(input,value)=>{await input.evaluate((el,v)=>{el.focus();el.value=v;el.dispatchEvent(new Event('input',{bubbles:true}))},value)}
const open=async()=>{
 await addButton().evaluate(el=>el.click())   // no focus moves, as on Safari and Firefox on a Mac
 const dialog=dialogOf();await expect(dialog).toBeVisible();return dialog
}
const closeDialog=async dialog=>{await dialog.locator('header').getByRole('button',{name:'Close',exact:true}).click();await expect(dialog).toHaveCount(0)}

try{
 // Before the machines are read, the entry is already offered in the header, and nothing is claimed about a name.
 syncHold=deferred()
 await page.goto(`${base}/machines`)
 await expect(addButton()).toBeVisible()
 {
  const dialog=await open(),p=parts(dialog)
  await p.name.fill('workstation')
  await expect(p.status).toContainText('Machines are still loading.')
  await expect(p.anyUse).toHaveCount(0)
  await expect(p.status).toContainText('If they never load, the step-by-step setup below still works.')
  await expect(p.status).not.toContainText('Waiting for it')
  await expect(p.status).not.toContainText('Ready')
  // The commands wait too: run before the machines were read they could not be told from the machine that is already there.
  await expect(dialog.getByText('Machines are still loading. The commands appear once they have been read.')).toBeVisible()
  await expect(p.commands).toHaveCount(0)
  await expect(dialog.getByRole('button',{name:/^Copy/})).toHaveCount(0)
  syncHold.resolve();syncHold=null
  await expect(p.commands).toBeVisible()
  await commandsAre(p,'workstation')
  // Once they are read, the name that already belongs to a reporting machine is said to be in use, not Ready.
  await expect(p.status).toContainText('A machine called workstation is already here.')
  await closeDialog(dialog)
 }
 await page.reload()   // the name just typed is remembered for the page's lifetime; start clean
 await expect(card('workstation')).toBeVisible()
 const storageBefore=await storage(),callsBefore=await storageCalls()
 // With machines listed, there is still one connect action and no second, differently-worded one.
 await expect(addButton()).toHaveCount(1)
 await expect(page.getByRole('button',{name:'Connect a machine',exact:true})).toHaveCount(0)

 // Opening writes nothing. The name is empty: no reason yet, no commands, nothing watched.
 const opener=addButton()
 const dialog=await open(),p=parts(dialog)
 expect(writes).toEqual([])
 await expect(p.name).toBeFocused()
 await expect(p.name).toHaveValue('')
 await expect(dialog.getByText('Enter a name for the machine.')).toHaveCount(0)
 await expect(p.commands).toHaveCount(0)
 await expect(dialog.getByRole('button',{name:/^Copy/})).toHaveCount(0)
 await expect(p.status).toHaveAttribute('aria-live','polite')
 await expect(p.status).toHaveAttribute('role','status')
 await expect(dialog).toContainText('Nothing happens on that machine until you run this there. reet.sh never logs in to it.')
 await words(dialog)
 // The name is refused with a plain reason once the field has been touched, and the commands wait for a good one.
 await p.name.focus();await p.name.blur()
 await expect(dialog.getByText('Enter a name for the machine.')).toBeVisible()
 await expect(p.name).toHaveAttribute('aria-invalid','true')
 await paste(p.name,'tab\there')
 await expect(dialog.getByText('Use a name without line breaks or other control characters.')).toBeVisible()
 await paste(p.name,'next\u0085line')
 await expect(dialog.getByText('Use a name without line breaks or other control characters.')).toBeVisible()
 await p.name.fill('x'.repeat(201))
 await expect(dialog.getByText(TOO_LONG)).toBeVisible()
 await expect(p.commands).toHaveCount(0)
 await expect(dialog.getByRole('button',{name:/^Copy/})).toHaveCount(0)
 // The limit is the server's, in bytes: 51 emoji are 204 bytes and are refused here rather than waited for for ever; 50 are 200.
 await p.name.fill('😀'.repeat(51))
 await expect(dialog.getByText(TOO_LONG)).toBeVisible()
 await expect(p.commands).toHaveCount(0)
 await expect(dialog.getByRole('button',{name:/^Copy/})).toHaveCount(0)
 await expect(p.status).not.toContainText('Waiting for it')
 await p.name.fill('😀'.repeat(50))
 await expect(dialog.getByText(TOO_LONG)).toHaveCount(0)
 await commandsAre(p,'😀'.repeat(50))
 await p.name.fill('  studio  ')
 await expect(dialog.getByText(TOO_LONG)).toHaveCount(0)
 await expect(p.name).not.toHaveAttribute('aria-invalid','true')
 // The block is the three builders' text for the trimmed name, and holds no token.
 await commandsAre(p,'studio')
 await p.name.fill("o'neil studio")
 await commandsAre(p,"o'neil studio")
 await p.name.fill('studio')
 await commandsAre(p,'studio')
 await expect(dialog).toContainText('The machine will ask for a token')
 // The status starts at Waiting for it, in words, with the dot hidden from a screen reader.
 await expect(p.status).toContainText('Waiting for it')
 await expect(p.status.locator('.dot')).toHaveAttribute('aria-hidden','true')
 await words(dialog)
 expect(writes).toEqual([])

 // Giving it a key: a refusal in the server's own words, a dropped reply as a plain sentence, then the token. Each leaves the dialog usable.
 tokenMode='refuse'
 await dialog.getByRole('button',{name:'Create machine token'}).click()
 await expect(dialog.getByRole('alert')).toContainText('Tokens cannot be made right now.')
 await commandsAre(p,'studio')
 await expect(dialog).toContainText('The machine will ask for a token')
 tokenMode='drop'
 await dialog.getByRole('button',{name:'Create machine token'}).click()
 await expect(dialog.getByRole('alert')).not.toContainText('Tokens cannot be made right now.')
 await expect(dialog.getByRole('alert')).not.toContainText(/TypeError|Failed to fetch|NetworkError/i)
 await expect(dialog.getByRole('alert')).toContainText(/\S/)
 await p.name.fill('studio ');await p.name.fill('studio')   // still usable
 await commandsAre(p,'studio')
 await dialog.getByRole('button',{name:'Create machine token'}).click()
 await expect(dialog.getByText('Token created.')).toBeVisible()
 expect(writes.map(w=>w.path)).toEqual(['/api/v2/tokens','/api/v2/tokens','/api/v2/tokens'])
 // The token is shown only in its own place; the block does not hold it, and copying copies the block alone.
 await dialog.getByText('Show the one-time machine token').click()
 await expect(dialog.locator('details.one-time code')).toHaveText(SECRET)
 expect(await p.commands.innerText()).not.toContain(SECRET)
 expect(await page.evaluate(()=>location.href+document.cookie)).not.toContain(SECRET)
 await dialog.getByRole('button',{name:'Copy commands'}).click()
 await expect(dialog.getByText('Copied to clipboard')).toBeAttached()
 expect(await page.evaluate(()=>navigator.clipboard.readText())).toBe(block('studio'))
 await commandsAre(p,'studio')

 // The state walks Waiting -> Connected (paused, with its reason) -> Waiting for its first scan -> Ready, each in words.
 const polled=await scheduled()
 agents.push(connector('agent-studio','studio',true))
 await expect(p.status).toContainText('Connected')
 await expect(p.status).toContainText('Its connector is paused. Resume it on the Machines page.')
 await expect(p.status).not.toContainText('Ready')
 await expect(p.anyUse).toHaveCount(0)
 await words(dialog)
 agents=agents.map(a=>a.id==='agent-studio'?{...a,paused:false}:a)
 await expect(p.status).toContainText('Waiting for its first scan')
 await expect(p.anyUse).toHaveCount(0)
 expect(await scheduled()).toBeGreaterThan(polled)
 expect(await delays()).toContain(3000)
 scopes.push(scope('studio'))
 await expect(p.status).toContainText('Ready')
 await expect(p.status).toContainText('does not show the machine works as you expect')
 await expect(p.use('studio')).toBeVisible()
 await words(dialog)

 // A scan ages: the state is judged against the clock as it is when asked, not as it was when the dialog opened. (Moving the
 // clock on this far also passes the watch cap, which is why Check again is pressed before it moves back.)
 await skew(15*DAY)
 await expect(p.status).toContainText('Waiting for its first scan')
 await expect(p.anyUse).toHaveCount(0)
 await expect(p.status).toContainText('Still waiting. Check again.')
 await dialog.getByRole('button',{name:'Check again',exact:true}).click()
 await skew(0)
 await expect(p.use('studio')).toBeVisible()
 // Reading again is backed off when it fails and returns to the normal pace once it works.
 const slow=await delays()
 syncStatus=500
 await expect.poll(async()=>(await delays()).filter(d=>d===12000).length).toBeGreaterThan(0)
 expect((await delays()).filter(d=>d===6000).length).toBeGreaterThan(0)
 await expect(dialog).toBeVisible()
 await expect(p.use('studio')).toBeVisible()
 syncStatus=0
 const afterFailure=(await delays()).length
 await expect.poll(async()=>(await delays()).slice(afterFailure).includes(3000)).toBe(true)
 expect(slow.length).toBeGreaterThan(0)
 await page.reload()   // the page's own read has shown an error; start the next part clean
 await expect(card('studio')).toBeVisible()

 // Choosing another name starts the watch afresh. A name that belongs to a machine that is reporting is said to be in use,
 // is offered at once, and stays so while that machine keeps reporting (its scans and heartbeats move on).
 {
  const d=await open(),pp=parts(d)
  await pp.name.fill('workstation')
  await expect(pp.status).toContainText('A machine called workstation is already here.')
  await expect(pp.status).not.toContainText(/\bReady\b/)
  await expect(pp.use('workstation')).toBeVisible()
  const reads=syncReads()
  scopes=scopes.map(s=>s.host==='workstation'?{...s,scannedAt:stamp(),lastSeen:stamp()}:s)
  agents=agents.map(a=>a.host==='workstation'?{...a,lastSeen:Date.now()+1}:a)
  await expect.poll(()=>syncReads()).toBeGreaterThan(reads+2)
  await expect(pp.status).toContainText('A machine called workstation is already here.')
  await expect(pp.status).not.toContainText(/\bReady\b/)
  // A connector that was not there before the name was chosen is a new enrolment: the usual states apply.
  agents.push(connector('agent-ws-new','workstation',true))
  await expect(pp.status).toContainText('Connected')
  await expect(pp.status).not.toContainText('already here')
  await words(d)
  // The step-by-step setup opens for the same name.
  await d.getByRole('button',{name:'Prefer the step-by-step setup?'}).click()
  await expect(d).toHaveCount(0)
  await expect(page.locator('details#connect-machine')).toHaveJSProperty('open',true)
  await expect(page.locator('details#connect-machine').getByLabel('Machine name',{exact:true})).toHaveValue('workstation')
  await quiet()
 }
 agents=agents.filter(a=>a.id!=='agent-ws-new')

 // A saved alias (kept after a rename or merge) is the same machine: the enrolment reports under the canonical name.
 {
  aliasesOf={workstation:['old-workstation'],lathe:['lathe-old']}
  scopes.push(scope('lathe',{scannedAt:Date.now()-60000}))
  await page.reload()
  await expect(card('lathe')).toBeVisible()
  const d=await open(),pp=parts(d)
  await pp.name.fill('Old-Workstation')
  await expect(pp.status).toContainText('A machine called workstation is already here.')
  await expect(pp.status).toContainText('You typed Old-Workstation; reet.sh saved it under workstation.')
  await expect(pp.use('workstation')).toBeVisible()
  await commandsAre(pp,'Old-Workstation')   // the command keeps what was typed: the server resolves it
  await words(d)
  // An alias of a machine whose connector is not there yet waits under the canonical name, then walks to Ready.
  await pp.name.fill('lathe-old')
  await expect(pp.status).toContainText('Waiting for it')
  await expect(pp.status).toContainText('The machine will appear as lathe.')
  await expect(pp.anyUse).toHaveCount(0)
  await commandsAre(pp,'lathe-old')
  agents.push(connector('agent-lathe','lathe'))
  await expect(pp.status).toContainText('Waiting for its first scan')
  scopes=scopes.map(s=>s.host==='lathe'?{...s,scannedAt:stamp(),lastSeen:stamp()}:s)
  await expect(pp.status).toContainText('Ready')
  await expect(pp.use('lathe')).toBeVisible()
  await words(d)
  await pp.use('lathe').click()
  await expect(d).toHaveCount(0)
  await expect(card('lathe')).toHaveClass(/just-added/)
  await expect(card('lathe')).toContainText('Just added')
  await expect(card('workstation')).not.toHaveClass(/just-added/)
  expect(await page.evaluate(()=>window.__scrolled)).toContain('machine-lathe')
  await quiet()
  aliasesOf={};scopes=scopes.filter(s=>s.host!=='lathe');agents=agents.filter(a=>a.id!=='agent-lathe')
 }

 // An alias saved while the dialog is open is picked up once: the wait moves to the canonical name.
 {
  aliasesOf={};scopes.push(scope('press'))
  await page.reload()
  await expect(card('press')).toBeVisible()
  const d=await open(),pp=parts(d)
  await pp.name.fill('press-old')
  await expect(pp.status).toContainText('Waiting for it')
  await expect(pp.status).not.toContainText('will appear as')
  aliasesOf={press:['press-old']}
  await expect(pp.status).toContainText('The machine will appear as press.')
  await commandsAre(pp,'press-old')
  await closeDialog(d)
  aliasesOf={};scopes=scopes.filter(s=>s.host!=='press')
 }

 // Closing stops everything, returns focus to what opened it, and a tab left hidden does not poll.
 await page.reload()
 await expect(card('workstation')).toBeVisible()
 {
  const d=await open(),pp=parts(d)
  await pp.name.fill('annexe')
  await expect(pp.status).toContainText('Waiting for it')
  await expect.poll(()=>pendingPolls()).toBe(1)
  await page.keyboard.press('Escape')
  await expect(d).toHaveCount(0)
  await expect(addButton()).toBeFocused()
  await quiet()
  // Hidden: nothing is asked for; back in view, it reads at once and the watch carries on.
  const d2=await open(),p2=parts(d2)
  await expect(p2.name).toHaveValue('annexe')   // remembered in memory for this page's lifetime, nowhere else
  await expect.poll(()=>pendingPolls()).toBe(1)
  await setHidden(true)
  await quiet()
  const reads=syncReads(),asked=await scheduled()
  await setHidden(false)
  await expect.poll(()=>syncReads()).toBeGreaterThan(reads)
  await expect.poll(()=>scheduled()).toBeGreaterThan(asked)
  // The watch cap counts hidden time too, but coming back reads once before it is applied (the read is held, so it is seen).
  await setHidden(true);await skew(16*60*1000)
  const held=deferred();syncHold=held
  const beforeWake=syncReads()
  await setHidden(false)
  await expect.poll(()=>syncReads()).toBeGreaterThan(beforeWake)
  await expect(d2.getByText('Still waiting. Check again.')).toHaveCount(0)
  held.resolve();syncHold=null
  // The watch cap: after fifteen minutes of waiting it stops and says so, and Check again starts it again.
  await expect(p2.status).toContainText('Still waiting. Check again.')
  await expect(d2.getByRole('button',{name:'Check again',exact:true})).toBeVisible()
  await quiet()
  const stopped=syncReads()
  await d2.getByRole('button',{name:'Check again',exact:true}).click()
  await expect(d2.getByRole('button',{name:'Check again',exact:true})).toHaveCount(0)
  await expect.poll(()=>syncReads()).toBeGreaterThan(stopped)
  await skew(0)
  // One chain only: over several reads there is never more than one pending timer, and each read schedules one.
  await resetMost()
  const f0=await fired(),s0=await scheduled()
  await expect.poll(()=>fired()).toBeGreaterThanOrEqual(f0+6)
  expect(await mostPending()).toBe(1)
  expect((await scheduled())-s0).toBeLessThanOrEqual((await fired())-f0+1)
  await closeDialog(d2)
  await quiet()
 }

 // From Machines, Ready offers Use <name>: it closes, scrolls to and highlights the card, and focus returns to the opener.
 {
  const d=await open(),pp=parts(d)
  await pp.name.fill('studio')
  await expect(pp.status).toContainText('A machine called studio is already here.')
  await pp.name.fill('annexe')
  await expect(pp.status).toContainText('Waiting for it')
  agents.push(connector('agent-annexe','annexe'))
  scopes.push(scope('annexe'))
  await expect(pp.status).toContainText('Ready')
  await expect(pp.use('annexe')).toBeVisible()
  await pp.use('annexe').click()
  await expect(d).toHaveCount(0)
  await expect(card('annexe')).toBeVisible()
  await expect(card('annexe')).toHaveClass(/just-added/)
  await expect(card('annexe')).toContainText('Just added')
  await expect(card('workstation')).not.toHaveClass(/just-added/)
  expect(await page.evaluate(()=>window.__scrolled)).toContain('machine-annexe')
  await expect(addButton()).toBeFocused()
  await quiet()
 }

 // On a Machines page filtered to one machine (?machine=), landing a different new machine clears the filter first, so its card
 // exists to be shown; other parameters and the hash stay, and the machine already shown stays too.
 {
  reset()
  await page.goto(`${base}/machines?machine=workstation&keep=1#top`)
  await expect(card('workstation')).toBeVisible()
  await expect(card('annexe')).toHaveCount(0)
  await page.evaluate(()=>{window.__scrolled.length=0})
  const d=await open(),pp=parts(d)
  await pp.name.fill('annexe')
  await expect(pp.status).toContainText('Waiting for it')
  agents.push(connector('agent-annexe','annexe'))
  scopes.push(scope('annexe'))
  await expect(pp.status).toContainText('Ready')
  await pp.use('annexe').click()
  await expect(d).toHaveCount(0)
  await expect(card('annexe')).toBeVisible()
  await expect(card('annexe')).toHaveClass(/just-added/)
  await expect(card('workstation')).toBeVisible()
  await expect(card('workstation')).not.toHaveClass(/just-added/)
  await expect.poll(()=>page.url()).not.toContain('machine=')
  const url=new URL(page.url())
  expect(url.searchParams.get('keep')).toBe('1')
  expect(url.searchParams.has('machine')).toBe(false)
  expect(url.hash).toBe('#top')
  await expect(page.locator('.machine-focus')).toHaveCount(0)
  expect(await page.evaluate(()=>window.__scrolled)).toContain('machine-annexe')
  await expect.poll(()=>card('annexe').evaluate(el=>{const r=el.getBoundingClientRect();return r.top>=0&&r.bottom<=innerHeight})).toBe(true)
  await expect(addButton()).toBeFocused()
  await quiet()
 }
 // Landing the machine the page is already filtered to leaves the filter where it is.
 {
  reset()
  await page.goto(`${base}/machines?machine=annexe&keep=1`)
  await expect(card('workstation')).toBeVisible()
  await expect(page.getByText('No machine named annexe is connected.')).toBeVisible()
  const d=await open(),pp=parts(d)
  await pp.name.fill('annexe')
  await expect(pp.status).toContainText('Waiting for it')
  agents.push(connector('agent-annexe','annexe'))
  scopes.push(scope('annexe'))
  await expect(pp.status).toContainText('Ready')
  await pp.use('annexe').click()
  await expect(d).toHaveCount(0)
  await expect(card('annexe')).toBeVisible()
  await expect(card('annexe')).toHaveClass(/just-added/)
  await expect(card('workstation')).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('machine')).toBe('annexe')
  expect(new URL(page.url()).searchParams.get('keep')).toBe('1')
  await expect(page.locator('.machine-focus')).toBeVisible()
  await quiet()
 }
 await page.goto(`${base}/machines`)
 await expect(card('workstation')).toBeVisible()

 // A signed-out session stops the watch at once and hands over to the sign-in card; nothing is read again.
 {
  const d=await open(),pp=parts(d)
  await pp.name.fill('garage')
  await expect(pp.status).toContainText('Waiting for it')
  syncStatus=401
  await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible()
  const reads=syncReads()
  await quiet()
  expect(syncReads()).toBe(reads)
  syncStatus=0
 }
 await page.reload()
 await expect(card('workstation')).toBeVisible()


 // ---- From the Private skills dialog ----
 // The entry, the context it carries, and the landing: the new machine's matching folder opens with the skill chosen.
 scopes.push(scope('bench',{id:'scope-bench',agent:'claude'}));agents.push(connector('agent-bench','bench'))
 await page.reload()
 await expect(card('bench')).toBeVisible()
 const SHARED='Shared agents / Codex',NOT_LISTED='Not listed? Add a machine'
 const privateOf=host=>page.getByRole('dialog',{name:`Private skills on ${host}`,exact:true})
 const anyPrivate=()=>page.getByRole('dialog',{name:/^Private skills on/})
 const pinButton=host=>card(host).getByRole('button',{name:'Pin a library revision for demo'}).first()
 const expand=async host=>{for(const selector of ['details.agent-list','details.content-observation']){const details=card(host).locator(selector).first();if(!await details.evaluate(el=>el.open))await details.locator(':scope > summary').click()}}
 const fromPrivate=async host=>{
  await expand(host)
  await pinButton(host).evaluate(el=>el.click())   // no focus moves, as on Safari and Firefox on a Mac
  const d=privateOf(host);await expect(d).toBeVisible();await expect(d.getByLabel('Library skill',{exact:true})).toBeVisible();return d
 }
 const addFrom=async host=>{
  const pd=await fromPrivate(host)
  await pd.getByRole('button',{name:NOT_LISTED,exact:true}).click()
  await expect(anyPrivate()).toHaveCount(0)
  const d=dialogOf();await expect(d).toBeVisible();return{d,p:parts(d)}
 }
 const closePrivate=pd=>pd.getByRole('button',{name:'Close private skills'}).click()
 const auto=d=>d.getByText(/^Opening the Private skills dialog for demo in 3 seconds\.$/)
 const hold=d=>d.locator('.hold')
 const noRequestToAMachine=()=>expect(foreign).toEqual([])
 const writesBefore=writes.length
 {
  const pd=await fromPrivate('workstation')
  const entry=pd.getByRole('button',{name:NOT_LISTED,exact:true})
  await expect(entry).toBeVisible()
  expect(writes.length).toBe(writesBefore)
  // Pressing it closes the Private skills dialog and opens the add dialog; closing that returns to what was pressed.
  await entry.click()
  await expect(anyPrivate()).toHaveCount(0)
  const d=dialogOf();await expect(d).toBeVisible()
  await expect(parts(d).name).toBeFocused()
  await expect(auto(d)).toHaveCount(0)
  await closeDialog(d)
  await expect(pinButton('workstation')).toBeFocused()
  await expect(anyPrivate()).toHaveCount(0)
  expect(await autoPending()).toBe(0)
 }

 // A machine that has reported in but cannot take a pin leaves the person in place, with the reason, whatever the cause.
 // Nothing here ever opens the Private skills dialog or offers Use.
 {
  const {d,p:pp}=await addFrom('workstation')
  await pp.name.fill('rack')
  await expect(pp.status).toContainText('Waiting for it')
  await expect(hold(d)).toHaveCount(0)
  agents.push(connector('agent-rack','rack',true))
  await expect(pp.status).toContainText('Connected')
  await expect(hold(d)).toContainText("Private skills can't open on rack yet.")
  await expect(hold(d)).toContainText("That machine isn't listed yet.")
  await expect(hold(d).getByRole('button',{name:'Wait here',exact:true})).toBeVisible()
  await expect(hold(d).getByRole('button',{name:'Close',exact:true})).toBeVisible()
  await expect(pp.anyUse).toHaveCount(0)
  // A scan-only agent folder is not a landing.
  scopes.push(scope('rack',{id:'scope-rack-codex',agent:'codex'}))
  await expect(hold(d)).toContainText('Its agent folders are scan-only')
  // A paused connector is not a landing, even with a good scan.
  scopes.push(scope('rack',{id:'scope-rack-claude',agent:'claude'}))
  await expect(hold(d)).toContainText('The connector on this machine is paused.')
  await expect(pp.anyUse).toHaveCount(0)
  expect(await autoPending()).toBe(0)
  // Running, but the only supported scan is a stale one: still not a landing.
  scopes=scopes.map(s=>s.id==='scope-rack-claude'?{...s,scannedAt:Date.now()-15*DAY}:s)
  agents=agents.map(a=>a.id==='agent-rack'?{...a,paused:false}:a)
  await expect(pp.status).toContainText('Waiting for its first scan')
  await expect(hold(d)).toContainText('last 14 days')
  await expect(pp.anyUse).toHaveCount(0)
  await expect(anyPrivate()).toHaveCount(0)
  await words(d)
  // Wait here keeps watching and says so; Close and Wait here are both there while nothing qualifies.
  await hold(d).getByRole('button',{name:'Wait here',exact:true}).click()
  await expect(hold(d)).toContainText('Waiting here.')
  await expect(hold(d).getByRole('button',{name:'Wait here',exact:true})).toHaveCount(0)
  const reads=syncReads()
  await expect.poll(()=>syncReads()).toBeGreaterThan(reads+1)
  expect(await autoPending()).toBe(0)
  // The scan arrives: Ready, the hold goes, and the three-second continue is offered (and can be cancelled).
  scopes=scopes.map(s=>s.id==='scope-rack-claude'?{...s,scannedAt:stamp()}:s)
  await expect(pp.status).toContainText('Ready')
  await expect(hold(d)).toHaveCount(0)
  await expect(auto(d)).toBeVisible()
  expect(await autoPending()).toBe(1)
  expect((await autoDelays()).at(-1)).toBe(3000)
  await d.getByRole('button',{name:'Cancel',exact:true}).click()
  await expect(auto(d)).toHaveCount(0)
  expect(await autoPending()).toBe(0)
  await fireStale()
  await afterRequests()
  await expect(d).toBeVisible()
  await expect(anyPrivate()).toHaveCount(0)
  // Use opens the dialog on the new machine's folder, with the skill chosen and a note that says which folder.
  await pp.use('rack').click()
  await expect(d).toHaveCount(0)
  const pd=privateOf('rack');await expect(pd).toBeVisible()
  await expect(pd.locator('.eyebrow')).toContainText('Claude Code')
  await expect(pd.getByLabel('Library skill',{exact:true})).toHaveValue(BUNDLE)
  await expect(pd.getByRole('status').filter({hasText:'Opened on'})).toHaveText('Opened on Claude Code on rack.')
  await expect(pd.getByRole('heading',{name:/^Private skills on/})).toBeFocused()
  expect(writes.length).toBe(writesBefore)
  await closePrivate(pd)
  await expect(card('rack').getByRole('button',{name:'Private skills for Claude Code'})).toBeFocused()
 }
 // A paused connector with a good scan: Connected, and no landing.
 {
  const {d,p:pp}=await addFrom('workstation')
  await pp.name.fill('nap')
  agents.push(connector('agent-nap','nap',true));scopes.push(scope('nap',{id:'scope-nap',agent:'claude'}))
  await expect(pp.status).toContainText('Connected')
  await expect(hold(d)).toContainText('The connector on this machine is paused.')
  await expect(pp.anyUse).toHaveCount(0)
  await expect(anyPrivate()).toHaveCount(0)
  await closeDialog(d)
 }

 // The same agent as the folder the person came from wins, though another is listed first.
 {
  const {d,p:pp}=await addFrom('bench')
  await pp.name.fill('duo')
  agents.push(connector('agent-duo','duo'))
  scopes.push(scope('duo',{id:'scope-duo-cursor',agent:'cursor'}),scope('duo',{id:'scope-duo-claude',agent:'claude'}))
  await expect(pp.status).toContainText('Ready')
  await expect(auto(d)).toBeVisible()
  await fireAuto()
  await expect(d).toHaveCount(0)
  const pd=privateOf('duo');await expect(pd).toBeVisible()
  await expect(pd.locator('.eyebrow')).toContainText('Claude Code')
  await expect(pd.getByRole('status').filter({hasText:'Opened on'})).toHaveText('Opened on Claude Code on duo. 1 other folder also qualifies; open it from the Machines page.')
  await expect(pd.getByLabel('Library skill',{exact:true})).toHaveValue(BUNDLE)
  await closePrivate(pd)
 }
 // Several qualify and none matches the agent: the first supported one, and the note counts the rest.
 {
  const {d,p:pp}=await addFrom('workstation')
  await pp.name.fill('trio')
  agents.push(connector('agent-trio','trio'))
  scopes.push(scope('trio',{id:'scope-trio-claude',agent:'claude'}),scope('trio',{id:'scope-trio-cursor',agent:'cursor'}),scope('trio',{id:'scope-trio-agents',agent:'agents'}))
  await expect(pp.status).toContainText('Ready')
  expect(await autoPending()).toBe(1)
  await pp.use('trio').click()
  expect(await autoPending()).toBe(0)
  const pd=privateOf('trio');await expect(pd).toBeVisible()
  await expect(pd.locator('.eyebrow')).toContainText(SHARED)
  await expect(pd.getByRole('status').filter({hasText:'Opened on'})).toHaveText(`Opened on ${SHARED} on trio. 2 other folders also qualify; open them from the Machines page.`)
  await closePrivate(pd)
 }
 // A stale folder is never chosen, even when it is the agent the person came from.
 {
  const {d,p:pp}=await addFrom('bench')
  await pp.name.fill('mix')
  agents.push(connector('agent-mix','mix'))
  scopes.push(scope('mix',{id:'scope-mix-claude',agent:'claude',scannedAt:Date.now()-15*DAY}),scope('mix',{id:'scope-mix-agents',agent:'agents'}))
  await expect(pp.status).toContainText('Ready')
  await pp.use('mix').click()
  const pd=privateOf('mix');await expect(pd).toBeVisible()
  await expect(pd.locator('.eyebrow')).toContainText(SHARED)
  await expect(pd.getByRole('status').filter({hasText:'Opened on'})).toHaveText(`Opened on ${SHARED} on mix.`)
  await expect(pd.getByRole('alert')).toHaveCount(0)
  await closePrivate(pd)
 }
 // The landing is made once: when the fleet is drawn again (the list emptied, then back) it does not open again.
 {
  emptied=true
  await page.getByRole('button',{name:'Refresh status'}).click()
  await expect(card('mix')).toHaveCount(0)
  emptied=false
  await page.getByRole('button',{name:'Refresh status'}).click()
  await expect(card('mix')).toBeVisible()
  await afterRequests()
  await expect(anyPrivate()).toHaveCount(0)
 }
 // Closing the add dialog while the continue is counting down stops it: it cannot fire afterwards.
 {
  const {d,p:pp}=await addFrom('workstation')
  await pp.name.fill('late')
  agents.push(connector('agent-late','late'));scopes.push(scope('late',{id:'scope-late',agent:'claude'}))
  await expect(pp.status).toContainText('Ready')
  await expect(auto(d)).toBeVisible()
  expect(await autoPending()).toBe(1)
  await closeDialog(d)
  expect(await autoPending()).toBe(0)
  await fireStale()
  await afterRequests()
  await expect(anyPrivate()).toHaveCount(0)
  await expect(pinButton('workstation')).toBeFocused()
  await quiet()
 }
 await noRequestToAMachine()

 // The note reaches a status region that was already there: text arrives later than the dialog was shown, and the heading keeps focus.
 {
  const {d,p:pp}=await addFrom('workstation')
  await pp.name.fill('ledge')
  agents.push(connector('agent-ledge','ledge'));scopes.push(scope('ledge',{id:'scope-ledge',agent:'claude'}))
  await expect(pp.status).toContainText('Ready')
  await pp.use('ledge').click()
  const pd=privateOf('ledge');await expect(pd).toBeVisible()
  const region=pd.locator('.live[role="status"]')
  await expect(region).toHaveAttribute('aria-live','polite')
  await expect(region).toHaveText('Opened on Claude Code on ledge.')
  await expect(pd.getByRole('heading',{name:/^Private skills on/})).toBeFocused()
  const gap=await page.evaluate(()=>{const shown=[...window.__live.shown].reverse().find(s=>s.title.startsWith('Private skills on ledge'));const text=window.__live.text.find(t=>t.text.startsWith('Opened on')&&t.at>=shown.at);return text.at-shown.at})
  expect(gap).toBeGreaterThanOrEqual(20)
  await closePrivate(pd)
 }

 // The landing re-checks the folder at the moment it opens it: if it has stopped qualifying in the same flush, nothing opens and
 // the person is told. (The scan ages past 14 days between pressing Use and the page drawing the landing.)
 {
  const {d,p:pp}=await addFrom('workstation')
  await pp.name.fill('flux')
  agents.push(connector('agent-flux','flux'));scopes.push(scope('flux',{id:'scope-flux',agent:'claude'}))
  await expect(pp.status).toContainText('Ready')
  await pp.use('flux').evaluate(el=>{el.click();window.__skew=15*86400000})
  await expect(d).toHaveCount(0)
  await expect(page.getByRole('status').filter({hasText:"Private skills can't open on that folder right now."})).toBeVisible()
  await afterRequests()
  await expect(anyPrivate()).toHaveCount(0)
  await skew(0)
  await quiet()
  // The landing was handed back as used: drawing the fleet again does not open it.
  await page.getByRole('button',{name:'Refresh status'}).click()
  await afterRequests()
  await expect(anyPrivate()).toHaveCount(0)
 }

 // The folder's own button cannot be found (the page is filtered to another machine, so the new card is not drawn): the dialog
 // still opens, and closing it leaves focus on the Add a machine button, never on the page.
 {
  await page.goto(`${base}/machines?machine=workstation`)
  await expect(card('workstation')).toBeVisible()
  await expect(card('hinge')).toHaveCount(0)
  const {d,p:pp}=await addFrom('workstation')
  await pp.name.fill('hinge')
  agents.push(connector('agent-hinge','hinge'));scopes.push(scope('hinge',{id:'scope-hinge',agent:'claude'}))
  await expect(pp.status).toContainText('Ready')
  await pp.use('hinge').click()
  const pd=privateOf('hinge');await expect(pd).toBeVisible()
  await expect(card('hinge')).toHaveCount(0)
  await expect(pd.getByRole('heading',{name:/^Private skills on/})).toBeFocused()
  await closePrivate(pd)
  await expect(pd).toHaveCount(0)
  await expect(addButton()).toBeFocused()
  await page.goto(`${base}/machines`)
  await expect(card('workstation')).toBeVisible()
 }

 // ---- Matrix: every state of the dialog at 1360, 768, 390 and 320px in light and dark ----
 // One walk through the states; at each, the viewport and scheme are changed under the open dialog (no reload) and the same checks
 // are made: no sideways scroll of the page, the dialog or its body, the command block wrapping inside its container, and the
 // primary action inside the window and not covered by anything.
 const COMBOS=[];for(const scheme of ['light','dark'])for(const [width,height] of [[1360,1000],[768,900],[390,800],[320,640]])COMBOS.push({scheme,width,height,label:`${scheme} ${width}px`})
 const setCombo=async c=>{await page.emulateMedia({colorScheme:c.scheme});await page.setViewportSize({width:c.width,height:c.height})}
 const reachable=async locator=>{
  await locator.scrollIntoViewIfNeeded()
  return locator.evaluate(el=>{const r=el.getBoundingClientRect(),top=document.elementFromPoint(r.left+r.width/2,r.top+r.height/2);return r.width>0&&r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight&&(top===el||el.contains(top))})
 }
 const surfaces={}
 const sweep=async(state,dlg,primary)=>{
  for(const c of COMBOS){
   await setCombo(c)
   const where=`${state} at ${c.label}`
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${where}: the page scrolls sideways`).toBe(true)
   if(dlg){
    const fit=await dlg.evaluate(el=>{
     const r=el.getBoundingClientRect(),body=el.querySelector('.body'),pre=el.querySelector('pre.commands'),box=pre?.parentElement?.getBoundingClientRect()
     return{inside:r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight,dialog:el.scrollWidth<=el.clientWidth,body:body.scrollWidth<=body.clientWidth,
      block:!pre||(pre.scrollWidth<=pre.clientWidth&&pre.getBoundingClientRect().right<=box.right+0.5&&box.right<=r.right),surface:getComputedStyle(el).backgroundColor}
    })
    expect({...fit,surface:undefined},`${where}: the dialog does not fit`).toEqual({inside:true,dialog:true,body:true,block:true,surface:undefined})
    surfaces[c.scheme]=fit.surface
   }
   for(const locator of primary)expect(await reachable(locator),`${where}: an action is out of reach or covered`).toBe(true)
  }
 }
 const LONG='a-rather-long-machine-name-that-has-no-spaces-so-it-must-wrap-inside-the-block-and-never-the-page'
 const copyButton=d=>d.getByRole('button',{name:'Copy commands',exact:true})
 reset();emptied=false
 scopes.push(scope('bench',{id:'scope-bench',agent:'claude'}));agents.push(connector('agent-bench','bench'))
 syncHold=deferred()
 await page.goto(`${base}/machines`)
 await expect(addButton()).toBeVisible()
 let sweeps=0
 {
  // Loading: nothing is claimed about the name and the step-by-step way is still there.
  const d=await open(),pp=parts(d)
  await pp.name.fill(LONG)
  await expect(pp.status).toContainText('Machines are still loading.')
  await sweep('loading',d,[d.getByRole('button',{name:'Prefer the step-by-step setup?'}),d.getByRole('button',{name:'Close',exact:true})]);sweeps++
  syncHold.resolve();syncHold=null
  // The name's own message.
  await pp.name.fill('')
  await pp.name.focus();await pp.name.blur()
  await expect(d.getByText('Enter a name for the machine.')).toBeVisible()
  await sweep('name message',d,[pp.name]);sweeps++
  await pp.name.fill(LONG)
  await expect(pp.status).toContainText('Waiting for it')
  await commandsAre(pp,LONG)
  await sweep('waiting',d,[copyButton(d),pp.name]);sweeps++
  agents.push(connector('agent-long',LONG,true))
  await expect(pp.status).toContainText('Connected')
  await sweep('connected, paused',d,[copyButton(d)]);sweeps++
  agents=agents.map(a=>a.id==='agent-long'?{...a,paused:false}:a)
  await expect(pp.status).toContainText('Waiting for its first scan')
  await sweep('first scan',d,[copyButton(d)]);sweeps++
  scopes.push(scope(LONG))
  await expect(pp.status).toContainText('Ready')
  await sweep('ready',d,[pp.use(LONG),copyButton(d)]);sweeps++
  // Still waiting at the cap (the scan has aged past 14 days and fifteen minutes have gone by).
  await skew(15*DAY)
  await expect(pp.status).toContainText('Still waiting. Check again.')
  await sweep('still waiting',d,[d.getByRole('button',{name:'Check again',exact:true})]);sweeps++
  await skew(0)
  await d.getByRole('button',{name:'Check again',exact:true}).click()
  await expect(pp.use(LONG)).toBeVisible()
  await closeDialog(d)
  scopes=scopes.filter(s=>s.host!==LONG);agents=agents.filter(a=>a.id!=='agent-long')
 }
 {
  // Already here, with the note that says what it was saved under.
  aliasesOf={workstation:['old-workstation']}
  await page.reload()
  await expect(card('workstation')).toBeVisible()
  const d=await open(),pp=parts(d)
  await pp.name.fill('Old-Workstation')
  await expect(pp.status).toContainText('You typed Old-Workstation; reet.sh saved it under workstation.')
  await sweep('already here with an alias note',d,[pp.use('workstation')]);sweeps++
  await closeDialog(d)
  aliasesOf={}
 }
 {
  // From the Private skills dialog: Ready with the continue notice and its Cancel, then the landing note.
  await page.reload()
  await expect(card('bench')).toBeVisible()
  const {d,p:pp}=await addFrom('bench')
  await pp.name.fill(LONG)
  agents.push(connector('agent-long2',LONG));scopes.push(scope(LONG,{id:'scope-long2',agent:'claude'}))
  await expect(pp.status).toContainText('Ready')
  await expect(d.locator('.auto')).toBeVisible()
  await sweep('ready with the continue notice',d,[pp.use(LONG),d.getByRole('button',{name:'Cancel',exact:true})]);sweeps++
  await pp.use(LONG).click()
  const pd=privateOf(LONG);await expect(pd).toBeVisible()
  const note=pd.locator('.live[role="status"]')
  await expect(note).toHaveText(`Opened on Claude Code on ${LONG}.`)
  await sweep('Private skills landing note',pd,[pd.getByRole('button',{name:'Close private skills'}),note]);sweeps++
  await closePrivate(pd)
  scopes=scopes.filter(s=>s.host!==LONG);agents=agents.filter(a=>a.id!=='agent-long2')
 }
 expect(surfaces.light).toBeTruthy();expect(surfaces.dark).toBeTruthy();expect(surfaces.dark).not.toBe(surfaces.light)   // dark is really applied
 expect(sweeps).toBe(10)
 {
  // Signed out: the page hands over to the sign-in card, which fits at every size.
  await page.reload()
  await expect(card('workstation')).toBeVisible()
  const d=await open(),pp=parts(d)
  await pp.name.fill('garage')
  await expect(pp.status).toContainText('Waiting for it')
  syncStatus=401
  const heading=page.getByRole('heading',{name:'Sign in to your workspace'})
  await expect(heading).toBeVisible()
  await sweep('signed out',null,[heading,page.getByRole('button',{name:'Sign in',exact:true})])
  syncStatus=0
  await page.reload()
  await expect(card('workstation')).toBeVisible()
 }
 // Escape closes without any write request, and focus goes back to what opened the dialog, at every size and in both schemes.
 for(const c of COMBOS){
  await setCombo(c)
  const writesSent=()=>requests.filter(r=>r.method!=='GET').length
  const before=writesSent()
  const d=await open(),pp=parts(d)
  await expect(pp.name).toBeFocused()
  await pp.name.fill('escape-'+c.width)
  await expect(pp.status).toContainText('Waiting for it')
  await page.keyboard.press('Escape')
  await expect(d).toHaveCount(0)
  await expect(addButton(),c.label).toBeFocused()
  await afterRequests()
  expect(writesSent(),`Escape at ${c.label} sent a write`).toBe(before)
  // And from the Private skills dialog, which gets focus back on its folder button.
  const pd=await fromPrivate('workstation')
  await pd.getByRole('button',{name:NOT_LISTED,exact:true}).click()
  const ad=dialogOf();await expect(ad).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(ad).toHaveCount(0)
  await expect(pinButton('workstation'),c.label).toBeFocused()
  expect(writesSent()).toBe(before)
 }
 await setCombo(COMBOS[0])
 // Reduced motion: nothing in the dialog runs an animation or a movement transition, and the new machine's highlight stays
 // visible as a still ring, with the page scrolling to it at once.
 {
  // The same landing with normal motion scrolls smoothly, so the difference below is the preference and nothing else.
  {
   await page.evaluate(()=>{window.__scrollBehaviour.length=0})
   const d=await open(),pp=parts(d)
   await pp.name.fill('motion')
   agents.push(connector('agent-motion','motion'));scopes.push(scope('motion'))
   await expect(pp.status).toContainText('Ready')
   await pp.use('motion').click()
   await expect(card('motion')).toHaveClass(/just-added/)
   expect(await page.evaluate(()=>window.__scrollBehaviour)).toContain('smooth')
  }
  await page.emulateMedia({reducedMotion:'reduce'})
  await page.evaluate(()=>{window.__scrollBehaviour.length=0})
  const d=await open(),pp=parts(d)
  await pp.name.fill('stillness')
  agents.push(connector('agent-stillness','stillness'));scopes.push(scope('stillness'))
  await expect(pp.status).toContainText('Ready')
  await copyButton(d).click()
  await expect(d.getByText('Copied to clipboard')).toBeAttached()
  const motion=await d.evaluate(el=>{
   const all=[el,...el.querySelectorAll('*')]
   const moving=all.filter(n=>{const cs=getComputedStyle(n);const names=cs.transitionProperty.split(',').map(t=>t.trim()),times=cs.transitionDuration.split(',').map(t=>parseFloat(t)*(t.trim().endsWith('ms')?1:1000))
    return cs.animationName!=='none'||names.some((name,i)=>(name==='all'||name==='transform')&&(times[i%times.length]>0))})
   return{moving:moving.map(n=>n.tagName+'.'+n.className),running:document.getAnimations().filter(a=>a.effect&&el.contains(a.effect.target)&&a.playState==='running'&&(a instanceof CSSAnimation||(a instanceof CSSTransition&&a.transitionProperty!=='opacity'))).length}
  })
  expect(motion).toEqual({moving:[],running:0})
  await pp.use('stillness').click()
  await expect(d).toHaveCount(0)
  await expect(card('stillness')).toHaveClass(/just-added/)
  const ring=await card('stillness').evaluate(el=>{const cs=getComputedStyle(el);return{animation:cs.animationName,shadow:cs.boxShadow,border:cs.borderTopColor,running:el.getAnimations().length}})
  expect(ring.animation).toBe('none')
  expect(ring.running).toBe(0)
  expect(ring.shadow).not.toBe('none')
  expect(ring.border).not.toBe(await card('workstation').evaluate(el=>getComputedStyle(el).borderTopColor))
  await expect(card('stillness')).toBeVisible()
  expect(await page.evaluate(()=>window.__scrollBehaviour)).not.toContain('smooth')
  expect(await page.evaluate(()=>window.__scrollBehaviour)).toContain('auto')
  await page.emulateMedia({reducedMotion:'no-preference'})
  await quiet()
 }
 await page.emulateMedia({colorScheme:'light'});await page.setViewportSize({width:1360,height:1000})

 // Nothing was written to browser storage.
 expect(await storageCalls()).toBe(callsBefore)
 expect(await storage()).toBe(storageBefore)

 // A viewer and a legacy sign-in mode see no entry at all.
 const beforeReaders=requests.length
 viewer=true;await page.reload()
 await expect(card('workstation')).toBeVisible()
 await expect(addButton()).toHaveCount(0)
 await expect(page.getByRole('button',{name:/^Private skills/})).toHaveCount(0)
 await expect(page.getByRole('button',{name:NOT_LISTED,exact:true})).toHaveCount(0)
 viewer=false;authMode='none';await page.reload()
 await expect(card('workstation')).toBeVisible()
 await expect(addButton()).toHaveCount(0)
 await expect(page.getByRole('button',{name:/^Private skills/})).toHaveCount(0)
 await expect(page.getByRole('button',{name:NOT_LISTED,exact:true})).toHaveCount(0)
 await noRequestToAMachine()
 // Neither reads anything of terminals (R16): no terminal endpoint is called for a viewer or a legacy sign-in.
 expect(requests.slice(beforeReaders).filter(r=>r.path.startsWith('/api/v2/terminal/')).map(r=>`${r.method} ${r.path}`)).toEqual([])
 expect(unmocked).toEqual([])
 expect(errors).toEqual([])
 console.log('Add a machine UI passed: entry beside the existing action and absent for viewers and legacy sign-in, the header entry before machines load with no state claimed for an unloaded list, no writes on open, name rules shown after touch, the block equal to the builders and free of the token, token refusal and dropped reply, copy of the block alone, Waiting/Connected/First scan/Ready in words, an aged scan judged by the clock now, back-off and recovery, already-here sticky until a new connector, step-by-step hand-over, close and Escape stopping the watch with focus returned, hidden pause and resume, the watch cap and Check again, Use landing with highlight, scroll and focus, sign-out hand-over, the Not listed? entry in the Private skills dialog (writers only) closing it and opening the add dialog with focus returned, no landing for a paused connector, a stale scan or a scan-only agent (Wait here and Close, reason shown), the three-second continue cancelled, fired and stopped by Close, the landing on the preferred agent or the first eligible folder with a note naming it and counting the rest, never on a stale folder, once only across a redraw, no request to a machine, the landing re-checked at the moment it opens (a folder that stopped qualifying opens nothing and says so), focus falling back to Add a machine when the folder button is not drawn, the landing note placed in a status region that already exists, a matrix of ten dialog states (loading, name message, waiting, paused connector, first scan, ready, still waiting, already here with an alias note, ready with the continue notice, landing note) plus the sign-in card at 1360, 768, 390 and 320 in light and dark with no sideways scroll, the command block wrapping, the primary action reachable and uncovered, Escape closing without a write and returning focus, a reduced-motion run, and no browser storage.')
}finally{await browser.close()}
