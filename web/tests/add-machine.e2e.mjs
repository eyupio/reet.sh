import { chromium, expect } from '@playwright/test'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const SITE=new URL(base).origin
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const context=await browser.newContext({viewport:{width:1360,height:1000},permissions:['clipboard-read','clipboard-write']})
const page=await context.newPage()
const DAY=86400000,errors=[],writes=[],requests=[],unmocked=[]
page.on('pageerror',e=>errors.push(e.message))
const deferred=()=>{let resolve;const promise=new Promise(r=>resolve=r);return{promise,resolve}}

// Server state. Scan times only ever grow, as the controller's do, so "a newer scan" is always a later stamp.
let aliasesOf={}   // saved aliases per canonical host, as the server keeps them after a rename or merge
let authMode='accounts',viewer=false,seq=0,syncStatus=0,syncHold=null,tokenMode='ok'
const stamp=()=>Date.now()+(++seq)
const scope=(host,over={})=>({id:`scope-${host}`,host,name:host,kind:'agent',agent:'agents',project:'',complete:true,scannedAt:stamp(),lastSeen:stamp(),skills:[],skillCount:0,content:{state:'observed',skills:[]},os:'linux',arch:'amd64',version:'dev',...over})
const connector=(id,host,paused=false)=>({id,host,version:'dev',paused,autoUpdate:false,lastSeen:Date.now()})
let scopes=[scope('workstation')],agents=[connector('agent-ws','workstation')]
const machinesOf=()=>{const by=new Map();for(const s of scopes)by.set(s.host,[...(by.get(s.host)??[]),s]);return[...by].map(([host,groups])=>({host,groups,skillCount:0,...(aliasesOf[host]?{aliases:aliasesOf[host]}:{})}))}
const reset=()=>{aliasesOf={};scopes=[scope('workstation')];agents=[connector('agent-ws','workstation')];syncStatus=0;syncHold=null;tokenMode='ok'}
const TOO_LONG='That name is too long. Use at most 200 bytes; accented letters and emoji take more than one.'
const SECRET='rst_SECRET_TOKEN_VALUE_9f3a'

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
  data={environments:scopes,machines:machinesOf(),agents,targets:[],jobs:[]}
 }
 else if(path==='/api/v2/machines/health')data={serverNow:Date.now(),machines:[]}
 else if(path==='/api/v2/setup/profiles')data={profiles:[]}
 else if(path==='/api/v2/terminal/machines')data={machines:[]}
 else if(path==='/api/v2/terminal/sessions')data={sessions:[]}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'v1.2.3',commit:'fixture'}
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
 use:name=>dialog.getByRole('button',{name:`Use ${name}`,exact:true}),
 anyUse:dialog.getByRole('button',{name:/^Use /}),
})
const words=async dialog=>{const text=await dialog.innerText();expect(text).not.toMatch(/verified|healthy/i);return text}
// Setting the value the way a paste would, which can carry what typing cannot.
const paste=async(input,value)=>{await input.evaluate((el,v)=>{el.focus();el.value=v;el.dispatchEvent(new Event('input',{bubbles:true}))},value)}
const open=async()=>{
 await addButton().evaluate(el=>el.click())   // no focus moves, as on Safari and Firefox on a Mac
 const dialog=dialogOf();await expect(dialog).toBeVisible();return dialog
}
const closeDialog=async dialog=>{await dialog.getByRole('button',{name:'Close',exact:true}).click();await expect(dialog).toHaveCount(0)}

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
 // With machines listed, the entry sits beside the existing add action and the header one is gone.
 await expect(addButton()).toHaveCount(1)
 await expect(page.locator('.fleet-controls').getByRole('button',{name:'Add a machine',exact:true})).toBeVisible()
 await expect(page.locator('.fleet-controls').getByRole('button',{name:'Connect a machine',exact:true})).toBeVisible()

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

 // Layout: the dialog fits at 1360, 390 and 320px in light and dark, with no sideways scroll.
 for(const scheme of ['light','dark'])for(const [width,height] of [[1360,1000],[390,800],[320,640]]){
  await page.emulateMedia({colorScheme:scheme})
  await page.setViewportSize({width,height})
  const d=await open(),pp=parts(d)
  await pp.name.fill("a-rather-long-machine-name-that-has-no-spaces-so-it-must-wrap-inside-the-block-and-never-the-page")
  await expect(pp.commands).toBeVisible()
  await layout()
  const fits=await d.evaluate(el=>{const r=el.getBoundingClientRect();const body=el.querySelector('.body');return{inside:r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight,noSideways:el.scrollWidth<=el.clientWidth&&body.scrollWidth<=body.clientWidth}})
  expect(fits).toEqual({inside:true,noSideways:true})
  await closeDialog(d)
 }
 await page.emulateMedia({colorScheme:'light'});await page.setViewportSize({width:1360,height:1000})

 // Nothing was written to browser storage.
 expect(await storageCalls()).toBe(callsBefore)
 expect(await storage()).toBe(storageBefore)

 // A viewer and a legacy sign-in mode see no entry at all.
 viewer=true;await page.reload()
 await expect(card('workstation')).toBeVisible()
 await expect(addButton()).toHaveCount(0)
 viewer=false;authMode='none';await page.reload()
 await expect(card('workstation')).toBeVisible()
 await expect(addButton()).toHaveCount(0)
 expect(unmocked).toEqual([])
 expect(errors).toEqual([])
 console.log('Add a machine UI passed: entry beside the existing action and absent for viewers and legacy sign-in, the header entry before machines load with no state claimed for an unloaded list, no writes on open, name rules shown after touch, the block equal to the builders and free of the token, token refusal and dropped reply, copy of the block alone, Waiting/Connected/First scan/Ready in words, an aged scan judged by the clock now, back-off and recovery, already-here sticky until a new connector, step-by-step hand-over, close and Escape stopping the watch with focus returned, hidden pause and resume, the watch cap and Check again, Use landing with highlight, scroll and focus, sign-out hand-over, layouts and themes, and no browser storage.')
}finally{await browser.close()}
