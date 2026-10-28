import { chromium, expect } from '@playwright/test'
// Start an agent: the dialog from the Terminals workspace and from a machine card, against synthetic API responses. Every
// create request the page sends is recorded, so what was sent (and that nothing was sent) is checked against the words.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const context=await browser.newContext({viewport:{width:1360,height:900}})
const page=await context.newPage()
const errors=[],posts=[],unmocked=[],urls=[]
page.on('pageerror',e=>errors.push(e.message))
page.on('framenavigated',f=>{if(f===page.mainFrame())urls.push(f.url())})
const deferred=()=>{let resolve;const promise=new Promise(r=>resolve=r);return{promise,resolve}}

// Server state.
const A='connector-laptop000000001',B='connector-oldbox00000001',C='connector-spare000000001'
const HOME_A='/home/dev',SITE='/home/dev/work/site'
let authMode='accounts',viewer=false,machinesFail=false,theme='light',machineReads=0
const tmachines=[
 {agentId:A,host:'dev-laptop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['claude','codex','gemini']}},
 // An older connector: no launch capability. The tools list is there only to show that it is not enough on its own.
 {agentId:B,host:'old-box',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'old',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',tools:['claude']}},
 {agentId:C,host:'spare-box',ready:false,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'spare',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['claude']}},
]
const now=Date.now()
const scope=(host,project,id)=>({id,host,name:host,kind:'agent',agent:'claude',project,complete:true,scannedAt:now,lastSeen:now,skills:[],skillCount:1,os:'linux',arch:'amd64',version:'dev'})
// Folders the machines reported: one project twice (with a trailing slash), one relative path that is never offered, and another machine's.
const scopes=[scope('dev-laptop',SITE,'s1'),scope('dev-laptop',SITE+'/','s2'),scope('dev-laptop','work/relative','s3'),scope('dev-laptop','','s4'),scope('old-box','/srv/old','s5'),scope('spare-box','/srv/spare','s6')]
const machinesOf=()=>{const by=new Map();for(const s of scopes)by.set(s.host,[...(by.get(s.host)??[]),s]);return[...by].map(([host,groups])=>({host,groups,skillCount:1}))}
const agents=[A,B,C].map((id,i)=>({id,host:tmachines[i].host,version:'dev',paused:false,autoUpdate:false,lastSeen:Date.now()}))
let sessions=[]
// How the next create is answered: a queue of answers, each 'ok', 'drop', a held promise, or {status, json}.
const answers=[]
const launchEvent={id:7,at:now,source:'ui',action:'terminal-launch',detail:JSON.stringify({machine:A,host:'dev-laptop',tool:'claude',model:'opus',dir:SITE})}

await page.route('**/api/**',async route=>{
 const req=route.request(),path=new URL(req.url()).pathname,method=req.method()
 let data={}
 if(path==='/api/v2/auth/status')data=authMode==='accounts'?{mode:'accounts',setupRequired:false,bootstrapRequired:false,user:{id:1,name:'Operator',email:'operator@example.test',role:viewer?'viewer':'owner'}}:{mode:authMode,setupRequired:false,bootstrapRequired:false}
 else if(path==='/api/v2/profile')data={user:{id:1,name:'Operator',email:'operator@example.test',role:viewer?'viewer':'owner'},profile:{theme,shelfView:'list'},sessionCount:1,accounts:authMode==='accounts'}
 else if(path==='/api/skills')data={skills:[],counts:{all:0,used:0}}
 else if(path==='/api/environments')data={environments:scopes,machines:machinesOf()}
 else if(path==='/api/tags')data={tags:[]}
 else if(path==='/api/v2/lists')data={lists:[],linkEnabled:false}
 else if(path==='/api/v2/sync')data={environments:scopes,machines:machinesOf(),agents,targets:[],jobs:[]}
 else if(path==='/api/v2/machines/health')data={serverNow:Date.now(),machines:[]}
 else if(path==='/api/v2/setup/profiles')data={profiles:[]}
 else if(path==='/api/v2/terminal/machines'){machineReads++;if(machinesFail)return route.fulfill({status:503,json:{error:'terminal machine is reconnecting; retry after it reconnects'}});data={machines:tmachines}}
 else if(path==='/api/v2/terminal/status')data={available:true,configured:true,reason:''}
 else if(path==='/api/v2/notifications/count')data={unread:0}
 else if(path==='/api/version')data={version:'v1.2.3',commit:'fixture'}
 else if(path==='/api/events')data={events:[launchEvent]}
 else if(path==='/api/v2/terminal/sessions'&&method==='GET')data={sessions}
 else if(path==='/api/v2/terminal/sessions'&&method==='POST'){
  const body=req.postData();posts.push(body)
  const answer=answers.shift()??'ok'
  if(answer&&answer.promise){await answer.promise;return reply(route,body)}
  if(answer==='drop')return route.abort('failed')
  // A 200 whose session the machine did not start (the broker marks it lost, as when local consent was switched off just before).
  if(answer&&answer.state){const b=JSON.parse(body);return route.fulfill({json:{id:`session-${b.requestId.replaceAll('-','').slice(0,20)}`,agentId:b.agentId,title:b.title,state:answer.state,createdAt:Date.now(),updatedAt:Date.now(),exitCode:null}})}
  if(typeof answer==='object')return route.fulfill({status:answer.status,json:answer.json})
  return reply(route,body)
 }
 else if(path.endsWith('/attachments'))data={ticket:'fixture-ticket00000000000'}
 else if(path.endsWith('/history'))return route.fulfill({body:''})
 else if(path.endsWith('/command-history'))data={entries:[],sources:[],truncated:false,skipped:0}
 else{unmocked.push(`${method} ${path}`);return route.fulfill({status:404,json:{error:`Unmocked ${path}`}})}
 await route.fulfill({json:data})
})
function reply(route,raw){
 const body=JSON.parse(raw)
 const s={id:`session-${body.requestId.replaceAll('-','').slice(0,20)}`,agentId:body.agentId,title:body.title,state:'running',createdAt:Date.now(),updatedAt:Date.now(),exitCode:null}
 if(!sessions.some(x=>x.id===s.id))sessions=[...sessions,s]
 return route.fulfill({json:s})
}
await page.routeWebSocket('**/api/v2/terminal/socket',socket=>{socket.onMessage(m=>{if(typeof m==='string'&&JSON.parse(m).ticket)socket.send('{"type":"ready"}')})})
// Every write to browser storage and every URL the page sets for itself is recorded from before the page loads, and kept by
// the fixture across page loads (a later write cannot hide an earlier one).
const storageWrites=[]
await page.exposeFunction('__recordStorage',(op,key,value)=>{storageWrites.push(`${op} ${key} ${value}`)})
await page.addInitScript(()=>{
 window.__storageCalls=[]
 for(const op of ['setItem','removeItem','clear']){const original=Storage.prototype[op];Storage.prototype[op]=function(...args){window.__storageCalls.push({op,key:String(args[0]??''),value:String(args[1]??'')});window.__recordStorage(op,String(args[0]??''),String(args[1]??''));return original.apply(this,args)}}
 // IndexedDB: opening or deleting a database and every write to a store, so an existing database cannot be written unseen.
 for(const op of ['open','deleteDatabase']){const original=IDBFactory.prototype[op];IDBFactory.prototype[op]=function(...args){window.__recordStorage('idb-'+op,String(args[0]??''),'');return original.apply(this,args)}}
 for(const op of ['put','add','delete','clear']){const original=IDBObjectStore.prototype[op];IDBObjectStore.prototype[op]=function(...args){let value='';try{value=JSON.stringify(args)}catch{value=String(args[0])}window.__recordStorage('idb-'+op,this.name,value);return original.apply(this,args)}}
 window.__urls=[]
 for(const op of ['pushState','replaceState']){const original=history[op].bind(history);history[op]=(state,title,url)=>{window.__urls.push(String(url??''));return original(state,title,url)}}
})

const dialog=()=>page.getByRole('dialog',{name:'Start an agent',exact:true})
const parts=d=>({
 machine:d.getByRole('combobox',{name:'Machine',exact:true}),
 model:d.getByRole('combobox',{name:/^Model/}),
 other:d.getByRole('textbox',{name:'Model name',exact:true}),
 folder:d.getByRole('combobox',{name:'Folder',exact:true}),
 path:d.getByRole('textbox',{name:'Folder path',exact:true}),
 prompt:d.getByRole('textbox',{name:/^Opening prompt/}),
 tool:name=>d.getByRole('radio',{name,exact:true}),
 status:d.getByRole('status'),
 sentence:d.locator('.sentence'),
 count:d.locator('.count'),
 start:d.getByRole('button',{name:'Start',exact:true}),
 retry:d.getByRole('button',{name:'Try the same request again',exact:true}),
 starting:d.getByRole('button',{name:'Starting…',exact:true}),
 close:d.locator('header').getByRole('button',{name:'Close',exact:true}),
 notListed:d.getByRole('button',{name:'Not listed? Add a machine',exact:true}),
})
const sidebarEntry=()=>page.locator('#terminal-sessions').getByRole('button',{name:'New agent session',exact:true})
const card=host=>page.locator('article').filter({has:page.getByRole('heading',{name:host,exact:true})})
const cardEntry=host=>card(host).getByRole('button',{name:'Start an agent',exact:true})
// A round trip through the page, so every handler that a key or click started has finished before the next check.
const settle=()=>page.evaluate(()=>fetch('/api/version').then(r=>r.status))
// The form submitted without its button, as a second press would: proves the dialog's own guard, not just a disabled button.
const submitAgain=d=>d.locator('form').evaluate(f=>f.requestSubmit())
const body=i=>JSON.parse(posts.at(i))
const UUID=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const LOST="We didn't hear back, so the agent may have started. Check your sessions before starting another. If you close this and start again, a second agent may start."
const LONG='a'.repeat(1998)+'é'
const secrets=['fix the failing build please',LONG.slice(0,40),'opus-4.1','gpt-5-codex',SITE,'/home/dev/typed']
const texts=async d=>{const t=await d.innerText();expect(t).not.toMatch(/verified|healthy/i);return t}
// While a request is in flight or its outcome is unknown, nothing that would change the request can be touched.
const locked=async p=>{
 for(const field of [p.machine,p.model,p.folder,p.prompt,p.tool('Claude Code'),p.tool('Codex'),p.tool('Gemini CLI')])await expect(field).toBeDisabled()
 for(const field of [p.other,p.path])if(await field.count())await expect(field).toBeDisabled()
}
const openFromSidebar=async()=>{await sidebarEntry().click();const d=dialog();await expect(d).toBeVisible();return d}

try{
 // ── From the Terminals workspace: the machine you came from is preselected and focused. ──
 await page.goto(`${base}/sessions?agent=${A}`)
 await expect(sidebarEntry()).toBeEnabled()
 let d=await openFromSidebar(),p=parts(d)
 await expect(p.machine).toHaveValue(A)
 await expect(p.machine).toBeFocused()
 await texts(d)
 // Agents: what the machine reports, the rest listed and disabled with the reason.
 await expect(p.tool('Claude Code')).toBeChecked()
 await expect(p.tool('Codex')).toBeEnabled()
 await expect(p.tool('Gemini CLI')).toBeEnabled()
 await expect(p.tool('OpenCode')).toBeDisabled()
 await expect(p.tool('OpenCode')).toHaveAccessibleDescription('Not installed on this machine.')
 await expect(p.tool('Claude Code')).toHaveAccessibleDescription('')
 // The reason is text to read, not a disabled control: it is not faded (an axe scan found it below contrast when it was).
 expect(await d.getByText('Not installed on this machine.',{exact:true}).evaluate(el=>{let o=1;for(let n=el;n&&n.tagName!=='DIALOG';n=n.parentElement)o*=Number(getComputedStyle(n).opacity);return o})).toBe(1)
 // Machines: every machine listed; the one not ready cannot be chosen.
 await expect(p.machine.locator('option')).toHaveText(['dev-laptop','old-box','spare-box · not ready'])
 await expect(p.machine.locator(`option[value="${C}"]`)).toBeDisabled()
 // Model suggestions per agent, Agent default first and Other last.
 await expect(p.model.locator('option')).toHaveText(['Agent default','sonnet','opus','haiku','Other…'])
 await p.tool('Codex').check()
 await expect(p.model.locator('option')).toHaveText(['Agent default','gpt-5-codex','Other…'])
 await p.tool('Gemini CLI').check()
 await expect(p.model.locator('option')).toHaveText(['Agent default','gemini-2.5-pro','gemini-2.5-flash','Other…'])
 await p.tool('Claude Code').check()
 // Folders: home first, each reported project once, never a relative path or another machine's.
 await expect(p.folder.locator('option')).toHaveText(['Home folder',SITE,'Type a path…'])
 // The line that says what will happen, without and then with a prompt.
 await expect(p.sentence).toHaveText('Starts Claude Code in the home folder on dev-laptop. Nothing runs until you press Start.')
 await p.folder.selectOption(SITE)
 await expect(p.sentence).toHaveText(`Starts Claude Code in ${SITE} on dev-laptop. Nothing runs until you press Start.`)

 // Other: checked against the character rule, and a missing name is asked for only once Start is pressed.
 await p.model.selectOption({label:'Other…'})
 await expect(d).not.toContainText('Type a model name')
 await p.start.click()
 await expect(p.status).toHaveText('Type a model name, or choose Agent default.')
 await expect(p.other).toBeFocused()
 await expect(p.other).toHaveAttribute('maxlength','100')
 for(const bad of ['bad model','-opus','opus;rm']){
  await p.other.fill(bad)
  await expect(d.getByText("That model name isn't one reet can pass on. Use letters, digits and . _ : / @ + -, up to 100 characters, starting with a letter or digit.",{exact:true})).toBeVisible()
 }
 expect(posts).toHaveLength(0)
 await p.other.fill('  opus-4.1  ')
 await expect(d.getByText("That model name isn't one reet can pass on.",{exact:false})).toHaveCount(0)
 // Editing the field takes the attempt's message out of the status line: it never outlives the mistake it named.
 await expect(p.status).toHaveText('')

 // Prompt rules, with the byte counter.
 await expect(p.count).toHaveText("0 of 2,000 bytes. Secrets don't belong here.")
 await p.prompt.fill('é')
 await expect(p.count).toHaveText("2 of 2,000 bytes. Secrets don't belong here.")
 await p.prompt.fill('-fix the build')
 await expect(d.getByText('Start the prompt with a word, not a dash.',{exact:true})).toBeVisible()
 await submitAgain(d)
 await expect(p.status).toHaveText('Start the prompt with a word, not a dash.')
 await expect(p.prompt).toBeFocused()
 await p.prompt.fill(LONG+'b')
 await expect(p.count).toHaveText("2,001 of 2,000 bytes. Secrets don't belong here.")
 await expect(d.getByText('That prompt is too long. Use at most 2000 bytes; accented letters and emoji take more than one.',{exact:true})).toBeVisible()
 await p.prompt.fill(LONG)
 await expect(p.count).toHaveText("2,000 of 2,000 bytes. Secrets don't belong here.")
 await expect(p.status).toHaveText('')
 await expect(d.getByText('That prompt is too long.',{exact:false})).toHaveCount(0)
 // One word could be read as a subcommand by claude and codex; gemini takes the prompt as a flag value, so one word is fine.
 await p.prompt.fill('purge')
 await expect(d.getByText('Write the opening prompt as at least two words.',{exact:true})).toBeVisible()
 await p.tool('Codex').check()
 await expect(d.getByText('Write the opening prompt as at least two words.',{exact:true})).toBeVisible()
 await p.tool('Gemini CLI').check()
 await expect(d.getByText('Write the opening prompt as at least two words.',{exact:true})).toHaveCount(0)
 await expect(p.prompt).not.toHaveAttribute('aria-invalid','true')
 // A different agent started the model and folder afresh; going back does not bring them back.
 await expect(p.model).toHaveValue('')
 await expect(p.folder).toHaveValue('')
 await p.tool('Claude Code').check()
 await expect(p.model).toHaveValue('')
 await expect(p.folder).toHaveValue('')

 // Type a path: absolute, checked as typed and when Start is pressed (focus goes to it).
 await p.folder.selectOption({label:'Type a path…'})
 await p.prompt.fill('')
 await p.start.click()
 await expect(p.status).toHaveText('Use a full path that starts with /.')
 await expect(p.path).toBeFocused()
 await p.path.fill('/home/dev/typed')
 await expect(p.status).toHaveText('')
 // The next attempt replaces the line too.
 await p.path.fill('')
 await p.start.click()
 await expect(p.status).toHaveText('Use a full path that starts with /.')
 await p.prompt.fill('-x marks the spot')
 await p.path.fill('/home/dev/typed')
 await p.start.click()
 await expect(p.status).toHaveText('Start the prompt with a word, not a dash.')
 await p.prompt.fill('')
 await expect(p.status).toHaveText('')
 await p.path.fill('work/site')
 await expect(d.locator('p.reason').filter({hasText:/^Use a full path that starts with \/\.$/})).toHaveCount(1)
 await expect(p.path).toHaveAttribute('aria-invalid','true')
 await p.path.fill('/home/dev/‮work')
 await expect(d.getByText('Use a folder path without control or invisible characters.',{exact:true})).toBeVisible()
 await p.path.fill('/'+'a'.repeat(4096))
 await expect(d.getByText('That folder path is too long. Use at most 4096 bytes; accented letters and emoji take more than one.',{exact:true})).toBeVisible()
 expect(posts).toHaveLength(0)

 // ── A full start: exactly the fields chosen, a request id, and the session opens with its title. ──
 await p.folder.selectOption(SITE)
 await p.model.selectOption({label:'Other…'})
 await p.other.fill('  opus-4.1  ')
 await p.prompt.fill('fix the failing build please')
 await expect(p.sentence).toHaveText(`Starts Claude Code in ${SITE} on dev-laptop. Nothing runs until you press Start. The prompt is kept with the session record and is visible to other users of that machine in its process list.`)
 await p.start.click()
 await expect(d).toHaveCount(0)
 expect(posts).toHaveLength(1)
 const first=body(0)
 expect(first.requestId).toMatch(UUID)
 expect(first).toEqual({agentId:A,requestId:first.requestId,title:'Claude Code · site',cols:100,rows:30,tool:'claude',model:'opus-4.1',dir:SITE,prompt:'fix the failing build please'})
 const opened=`session-${first.requestId.replaceAll('-','').slice(0,20)}`
 await expect(page).toHaveURL(new RegExp(`[?&]session=${opened}(&|$)`))
 await expect(page.locator('#terminal-sessions').locator('.session-row[aria-current="true"]')).toContainText('Claude Code · site')
 await expect(page.locator('.pane')).toBeVisible()

 // Opened again, the prompt is gone (it lived only in the closed dialog). Escape closes and focus returns to the opener.
 d=await openFromSidebar();p=parts(d)
 await expect(p.prompt).toHaveValue('')
 await expect(p.model).toHaveValue('')
 await p.prompt.fill('fix the failing build please')
 await page.keyboard.press('Escape')
 await expect(d).toHaveCount(0)
 await expect(sidebarEntry()).toBeFocused()

 // A plain new terminal is unchanged: none of the launch keys.
 await page.locator('#terminal-sessions').getByRole('button',{name:'New terminal on dev-laptop',exact:true}).first().click()
 await expect.poll(()=>posts.length).toBe(2)
 const plain=body(1)
 expect(Object.keys(plain)).toEqual(['agentId','requestId','title','cols','rows'])
 expect(posts[1]).toBe(JSON.stringify({agentId:A,requestId:plain.requestId,title:'Shell',cols:100,rows:30}))
 await expect(page).toHaveURL(new RegExp(`session=session-${plain.requestId.replaceAll('-','').slice(0,20)}`))

 // ── A held first Start: every field locked, Close and Escape wait, a second submit sends nothing. ──
 d=await openFromSidebar();p=parts(d)
 await p.model.selectOption({label:'Other…'})
 await p.other.fill('sonnet-4')
 await p.folder.selectOption({label:'Type a path…'})
 await p.path.fill('/home/dev/held')
 await p.prompt.fill('wait for me please')
 const firstHold=deferred();answers.push(firstHold)
 const sentFirst=posts.length
 await p.start.click()
 await expect(p.starting).toBeDisabled()
 await locked(p)
 await expect(p.close).toBeDisabled()
 await expect(p.status).toHaveText('Starting Claude Code on dev-laptop… this can take a few seconds.')
 await submitAgain(d)
 await page.keyboard.press('Escape')
 await settle()
 await expect(d).toBeVisible()
 expect(posts.length).toBe(sentFirst+1)
 firstHold.resolve()
 await expect(d).toHaveCount(0)
 expect(body(-1)).toEqual({agentId:A,requestId:body(-1).requestId,title:'Claude Code · held',cols:100,rows:30,tool:'claude',model:'sonnet-4',dir:'/home/dev/held',prompt:'wait for me please'})

 // ── A lost reply: plain words, Retry focused, the same request id and body; nothing changes while it is in flight. ──
 d=await openFromSidebar();p=parts(d)
 await p.tool('Codex').check()
 await p.model.selectOption('gpt-5-codex')
 await p.prompt.fill('tidy the readme')
 answers.push('drop')
 await p.start.click()
 await expect(p.status).toHaveText(LOST)
 await expect(p.retry).toBeFocused()
 await locked(p)
 // The machine list refreshes while the outcome is unknown and Codex is no longer reported: nothing shown or sent changes.
 const heldSentence=await p.sentence.textContent()
 expect(heldSentence).toContain('Starts Codex in the home folder on dev-laptop.')
 const readsBefore=machineReads
 tmachines[0].capabilities.tools=['claude','gemini']
 await page.evaluate(()=>window.dispatchEvent(new Event('online')))
 await expect.poll(()=>machineReads).toBeGreaterThan(readsBefore)
 await settle()
 await expect(p.sentence).toHaveText(heldSentence)
 await expect(p.tool('Codex')).toBeChecked()
 await expect(p.model).toHaveValue('gpt-5-codex')
 await expect(p.prompt).toHaveValue('tidy the readme')
 await locked(p)
 // With every field disabled, the scrolling part can still be reached from the keyboard to read the choices.
 await expect(d.locator('.body')).toHaveAttribute('tabindex','0')
 await expect(d.locator('.body')).toHaveAccessibleName('Your choices')
 const lost=posts.at(-1)
 // A 5xx is no answer either.
 answers.push({status:502,json:{error:'Bad gateway'}})
 await p.retry.click()
 await expect(p.status).toHaveText(LOST)
 await expect(p.retry).toBeFocused()
 // Held in flight: Close and Escape wait, a second submit sends nothing.
 const hold=deferred();answers.push(hold)
 const sent=posts.length
 await p.retry.click()
 await expect(p.starting).toBeDisabled()
 await expect(p.close).toBeDisabled()
 await locked(p)
 await expect(p.status).toHaveText('Starting Codex on dev-laptop… this can take a few seconds.')
 await submitAgain(d);await submitAgain(d)
 await page.keyboard.press('Escape')
 await settle()
 await expect(d).toBeVisible()
 expect(posts.length).toBe(sent+1)
 hold.resolve()
 await expect(d).toHaveCount(0)
 expect(posts.slice(-3)).toEqual([lost,lost,lost])
 tmachines[0].capabilities.tools=['claude','codex','gemini']
 expect(JSON.parse(lost)).toEqual({agentId:A,requestId:JSON.parse(lost).requestId,title:'Codex',cols:100,rows:30,tool:'codex',model:'gpt-5-codex',prompt:'tidy the readme'})

 // ── Definite refusals: the exact sentence, the field concerned focused, and the next Start is a new request. ──
 d=await openFromSidebar();p=parts(d)
 await p.folder.selectOption({label:'Type a path…'})
 await p.path.fill('/home/dev/typed')
 await p.prompt.fill('look around here')
 const refusals=[
  {answer:{status:422,json:{error:"That folder isn't available on that machine.",code:'folder_unavailable',message:"That folder isn't available on that machine.",field:'dir'}},text:"That folder isn't available on that machine.",focus:p.path},
  {answer:{status:422,json:{error:"That agent isn't installed on that machine.",code:'tool_not_installed',message:"That agent isn't installed on that machine.",field:'tool'}},text:"That agent isn't installed on that machine.",focus:p.tool('Claude Code')},
  {answer:{status:422,json:{error:'Write the opening prompt as at least two words.',code:'prompt_unsupported',message:'Write the opening prompt as at least two words.',field:'prompt'}},text:'Write the opening prompt as at least two words.',focus:p.prompt},
  {answer:{status:409,json:{error:'Update reet on that machine to start agents.',code:'launch_unsupported',message:'Update reet on that machine to start agents.'}},text:'Update reet on that machine to start agents.',focus:p.machine},
  {answer:{status:409,json:{error:'terminal session limit reached'}},text:'The terminal session limit has been reached. End a session and try again.',focus:p.start},
  {answer:{status:409,json:{error:'terminal operation conflicts with current state'}},text:'That request changed. Start it again.',focus:p.start},
  {answer:{status:403,json:{error:'terminal access denied'}},text:'Something changed. Check again.',focus:p.start},
  {answer:{status:401,json:{error:'unauthorized'}},text:"You've been signed out. Sign in again to continue.",focus:p.start},
 ]
 const ids=[]
 for(const r of refusals){
  answers.push(r.answer)
  await p.start.click()
  await expect(p.status).toHaveText(r.text)
  await expect(r.focus).toBeFocused()
  await expect(p.start).toBeEnabled()
  await expect(p.prompt).toBeEnabled()
  ids.push(body(-1).requestId)
 }
 expect(new Set(ids).size).toBe(refusals.length)
 expect(body(-1)).toEqual({agentId:A,requestId:ids.at(-1),title:'Claude Code · typed',cols:100,rows:30,tool:'claude',dir:'/home/dev/typed',prompt:'look around here'})
 // A 200 for a session the machine did not start: a definite outcome, said plainly, no landing, and the next Start is a new request.
 const urlBefore=page.url()
 answers.push({state:'lost'})
 await p.start.click()
 await expect(p.status).toHaveText("That machine didn't start the agent. Check its terminal settings on that machine and try again.")
 await expect(d).toBeVisible()
 await expect(p.start).toBeFocused()
 await expect(p.prompt).toBeEnabled()
 expect(page.url()).toBe(urlBefore)
 const lostId=body(-1).requestId
 answers.push({status:409,json:{error:'terminal operation conflicts with current state'}})
 await p.start.click()
 await expect(p.status).toHaveText('That request changed. Start it again.')
 expect(body(-1).requestId).not.toBe(lostId)
 // A reconnecting machine (503) may still have been asked: it keeps the request, like a lost reply.
 answers.push({status:503,json:{error:'terminal machine is reconnecting; retry after it reconnects'}})
 await p.start.click()
 await expect(p.status).toHaveText('That machine is reconnecting. Try again in a moment. Check your sessions before starting another. If you close this and start again, a second agent may start.')
 await expect(p.retry).toBeFocused()
 const reconnecting=posts.at(-1)
 await p.retry.click()
 await expect(d).toHaveCount(0)
 expect(posts.at(-1)).toBe(reconnecting)

 // ── The three disabled states. ──
 d=await openFromSidebar();p=parts(d)
 // An older machine: told to update, nothing offered, and nothing sent even if the form is submitted.
 await p.machine.selectOption(B)
 await expect(p.machine).toHaveAccessibleDescription('Update reet on this machine to start agents.')
 await expect(d.locator('p.reason')).toHaveText(['Update reet on this machine to start agents.'])
 for(const name of ['Claude Code','Codex','OpenCode','Gemini CLI']){await expect(p.tool(name)).toBeDisabled();await expect(p.tool(name)).toHaveAccessibleDescription('Update reet on this machine to start agents.')}
 await expect(p.start).toBeDisabled()
 await expect(p.sentence).toHaveCount(0)
 const before=posts.length
 await submitAgain(d)
 await settle()
 expect(posts.length).toBe(before)
 await p.close.click()
 await expect(d).toHaveCount(0)
 await expect(sidebarEntry()).toBeFocused()
 // The machine you came from is not ready: said plainly, another is chosen.
 await page.goto(`${base}/sessions?agent=${C}`)
 d=await openFromSidebar();p=parts(d)
 await expect(p.machine).toHaveValue(A)
 await expect(p.machine).toHaveAccessibleDescription("spare-box isn't ready, so dev-laptop is selected.")
 await page.keyboard.press('Escape')
 // Nothing ready: the existing readiness sentence and no Start.
 for(const m of tmachines)m.ready=false
 await page.goto(`${base}/sessions?agent=${A}`)
 d=await openFromSidebar();p=parts(d)
 await expect(p.machine).toHaveValue(A)
 await expect(p.machine).toHaveAccessibleDescription('The machine is reconnecting. Existing shells will be kept.')
 await expect(p.start).toBeDisabled()
 await submitAgain(d)
 await settle()
 expect(posts.length).toBe(before)
 await page.keyboard.press('Escape')
 for(const m of tmachines)m.ready=m.agentId!==C

 // ── From a machine card. ──
 await page.goto(`${base}/machines`)
 await expect(cardEntry('dev-laptop')).toBeEnabled()
 await expect(cardEntry('old-box')).toBeEnabled()
 // Not ready: disabled, with the plain readiness sentence.
 await expect(cardEntry('spare-box')).toBeDisabled()
 await expect(cardEntry('spare-box')).toHaveAccessibleDescription('The machine is reconnecting. Existing shells will be kept.')
 await cardEntry('old-box').click()
 d=dialog();p=parts(d)
 await expect(p.machine).toHaveValue(B)
 await p.close.click()
 await expect(cardEntry('old-box')).toBeFocused()
 // Not listed? hands over to Add a machine, which gives focus back to the card's button.
 await cardEntry('dev-laptop').click()
 await parts(dialog()).notListed.click()
 const add=page.getByRole('dialog',{name:'Add a machine',exact:true})
 await expect(add).toBeVisible()
 await expect(dialog()).toHaveCount(0)
 await page.keyboard.press('Escape')
 await expect(cardEntry('dev-laptop')).toBeFocused()
 // The simplest start: only the tool is sent, and the Terminals page opens on the new session.
 await cardEntry('dev-laptop').click()
 d=dialog();p=parts(d)
 await expect(p.machine).toHaveValue(A)
 await p.tool('Gemini CLI').check()
 await p.start.click()
 await expect(page).toHaveURL(/\/sessions\?/)
 const simple=body(-1)
 expect(posts.at(-1)).toBe(JSON.stringify({agentId:A,requestId:simple.requestId,title:'Gemini CLI',cols:100,rows:30,tool:'gemini'}))
 await expect(page).toHaveURL(`${base}/sessions?agent=${A}&session=session-${simple.requestId.replaceAll('-','').slice(0,20)}`)
 // From Terminals, Not listed? goes to the Machines page with Add a machine open, and leaves no marker in the address.
 d=await openFromSidebar()
 await parts(d).notListed.click()
 await expect(add).toBeVisible()
 await expect(page).toHaveURL(`${base}/machines`)
 await page.keyboard.press('Escape')

 // From Machines, too, the dialog stays Starting… until the page has gone; if the next page never arrives (here a 204), it
 // lets go after a few seconds instead of leaving a modal with no way out.
 await page.goto(`${base}/machines`)
 const onlyTerminals=url=>new URL(url).pathname==='/sessions'
 await page.route(onlyTerminals,route=>route.fulfill({status:204,body:''}))
 await cardEntry('dev-laptop').click()
 d=dialog();p=parts(d)
 await expect(p.machine).toHaveValue(A)
 const postsBefore=posts.length
 await expect(p.start).toBeEnabled()
 // Pressed from inside: Playwright's own click waits for the navigation it starts, which a 204 never finishes.
 await page.evaluate(()=>document.querySelector('dialog[open] footer button[type=submit]').click())
 // Playwright's locators wait for a navigation a 204 never finishes, so the page is read from inside.
 const held=()=>page.evaluate(()=>{const x=document.querySelector('dialog[open]');return{url:location.pathname,open:!!x,text:x?.querySelector('footer button[type=submit]')?.textContent,close:x?.querySelector('[data-shell-close]')?.disabled}})
 await expect.poll(()=>posts.length).toBe(postsBefore+1)
 await page.waitForTimeout(1500)
 expect(await held()).toEqual({url:'/machines',open:true,text:'Starting…',close:true})
 await expect.poll(held,{timeout:9000}).toMatchObject({url:'/machines',open:false})
 expect(posts.length).toBe(postsBefore+1)
 // When it lets go it says what started, with the way to it, as Home does.
 const stalled=body(-1),stalledSession=`session-${stalled.requestId.replaceAll('-','').slice(0,20)}`
 const said=await page.evaluate(()=>{const n=[...document.querySelectorAll('.notice')].find(x=>x.textContent.startsWith('Started '));return{text:n?.textContent??null,href:n?.querySelector('a')?.getAttribute('href')??null}})
 expect(said).toEqual({text:`Started ${stalled.title} on dev-laptop. Open it`,href:`/sessions?agent=${A}&session=${stalledSession}`})
 await page.unroute(onlyTerminals)

 // A 401 from the card's dialog hands over to the sign-in card, as the page's other dialogs do.
 await page.goto(`${base}/machines`)
 await cardEntry('dev-laptop').click()
 d=dialog();p=parts(d)
 answers.push({status:401,json:{error:'unauthorized'}})
 await p.start.click()
 await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible()
 // An enrolled machine the terminal list does not know yet is said plainly, not as a choice to make.
 scopes.push(scope('fresh-box','/srv/fresh','s9'));agents.push({id:'connector-fresh00000001',host:'fresh-box',version:'dev',paused:false,autoUpdate:false,lastSeen:Date.now()})
 await page.goto(`${base}/machines`)
 await expect(cardEntry('fresh-box')).toBeDisabled()
 await expect(cardEntry('fresh-box')).toHaveAccessibleDescription("This machine hasn't connected for terminals yet.")
 scopes.pop();agents.pop()
 // A failed readiness read is said on the card until a read succeeds.
 machinesFail=true
 await page.goto(`${base}/machines`)
 await expect(card('dev-laptop')).toContainText("Couldn't check this machine just now. Try again in a moment.")
 await expect(cardEntry('dev-laptop')).toBeDisabled()
 await expect(cardEntry('dev-laptop')).toHaveAccessibleDescription("Couldn't check this machine just now. Try again in a moment.")
 machinesFail=false
 await page.evaluate(()=>window.dispatchEvent(new Event('online')))
 await expect(cardEntry('dev-laptop')).toBeEnabled()
 await expect(card('dev-laptop')).not.toContainText("Couldn't check")

 // ── Matrix: every state of the dialog at 1360, 768, 390 and 320px in light and dark. ──
 // One walk through the states; at each, the viewport and scheme are changed under the open dialog (no reload) and the same checks
 // are made: no sideways scroll of the page, the dialog inside the window with neither it, its body nor anything in it reaching
 // past its edges, and each primary action scrolled into view with a hit test at its centre landing on it. A machine with a long
 // unbroken name and a long project folder is added, so wrapping and truncation are tested with values that cannot break at a space.
 const COMBOS=[];for(const scheme of ['light','dark'])for(const [width,height] of [[1360,900],[768,900],[390,800],[320,640]])COMBOS.push({scheme,width,height,label:`${scheme} ${width}px`})
 // Two frames after the change, so the page's own media-query listeners (the workspace shuts its list below 960px) have run.
 const setCombo=async c=>{await page.emulateMedia({colorScheme:c.scheme});await page.setViewportSize({width:c.width,height:c.height});await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))))}
 const reachable=async locator=>{
  await locator.scrollIntoViewIfNeeded()
  return locator.evaluate(el=>{const r=el.getBoundingClientRect(),top=document.elementFromPoint(r.left+r.width/2,r.top+r.height/2);return r.width>0&&r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight&&(top===el||el.contains(top))||`${el.tagName}#${el.id} at ${Math.round(r.left)},${Math.round(r.top)}-${Math.round(r.right)},${Math.round(r.bottom)} in ${innerWidth}x${innerHeight}, under ${top?.tagName}.${top?.className}`})
 }
 const surfaces={}
 let sweeps=0
 // before runs after each size change: the Terminals sidebar is shut on a narrow screen once a session is chosen, as a person would find it.
 const sweep=async(state,primary,{before}={})=>{
  for(const c of COMBOS){
   await setCombo(c)
   if(before)await before()
   const where=`${state} at ${c.label}`
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${where}: the page scrolls sideways`).toBe(true)
   if(await dialog().count()){
    const fit=await dialog().evaluate(el=>{
     const r=el.getBoundingClientRect(),body=el.querySelector('.body')
     const spill=[...el.querySelectorAll('*')].filter(n=>{const b=n.getBoundingClientRect();return b.width>0&&b.height>0&&(b.left<r.left-0.5||b.right>r.right+0.5)}).map(n=>n.tagName+'.'+n.className)
     return{inside:r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight,dialog:el.scrollWidth<=el.clientWidth,body:body.scrollWidth<=body.clientWidth,spill,surface:getComputedStyle(el).backgroundColor}
    })
    expect({...fit,surface:undefined},`${where}: the dialog does not fit`).toEqual({inside:true,dialog:true,body:true,spill:[],surface:undefined})
    surfaces[c.scheme]=fit.surface
   }
   for(const locator of primary)expect(await reachable(locator),`${where}: an action is out of reach or covered`).toBe(true)
  }
  sweeps++
  await setCombo(COMBOS[0])
 }
 const LONG_HOST='build-server-in-the-back-office-with-a-very-long-hostname-01'
 const LONG_ID='connector-longname000001'
 const LONG_DIR='/home/builder/work/clients/a-very-long-client-name-with-no-spaces-at-all/and-an-even-longer-project-folder-name-that-must-wrap'
 tmachines.push({agentId:LONG_ID,host:LONG_HOST,ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'builder',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['claude']}})
 scopes.push(scope(LONG_HOST,LONG_DIR,'s7'))
 agents.push({id:LONG_ID,host:LONG_HOST,version:'dev',paused:false,autoUpdate:false,lastSeen:Date.now()})
 theme='system'
 const writesBefore=storageWrites.length
 // On a narrow screen the sessions list is behind Show sessions; the entry is then reached the way a person would reach it.
 // The workspace restores its saved layout as it loads, so the check is repeated until the list stays open.
 const showSidebar=async()=>{const toggle=page.locator('.workspace-actions [aria-controls="terminal-sessions"]');await expect(async()=>{if(await toggle.getAttribute('aria-expanded')!=='true')await toggle.click();await expect(sidebarEntry()).toBeVisible({timeout:1000})}).toPass()}
 const openAt=async agentId=>{await page.goto(`${base}/sessions?agent=${agentId}`);await showSidebar();return openFromSidebar()}

 // Closed: both entry points.
 await page.goto(`${base}/sessions?agent=${A}`)
 await expect(sidebarEntry()).toBeVisible()
 await sweep('closed, Terminals entry',[sidebarEntry()],{before:showSidebar})
 await page.goto(`${base}/machines`)
 await expect(cardEntry(LONG_HOST)).toBeEnabled()
 await sweep('closed, machine card entries',[cardEntry('dev-laptop'),cardEntry(LONG_HOST),cardEntry('spare-box')])

 // Open, as it first appears.
 d=await openAt(A);p=parts(d)
 await sweep('open default (OpenCode not installed)',[p.start,p.close,p.tool('OpenCode')])
 // Long names, from the card: the select shows what fits and the whole value is in the option and in the sentence, which wraps.
 await page.keyboard.press('Escape')
 await page.goto(`${base}/machines`)
 await cardEntry(LONG_HOST).click()
 d=dialog();p=parts(d)
 await expect(p.machine).toHaveValue(LONG_ID)
 await expect(p.machine.locator('option:checked')).toHaveText(LONG_HOST)
 await expect(p.tool('Codex')).toHaveAccessibleDescription('Not installed on this machine.')
 await p.folder.selectOption(LONG_DIR)
 await expect(p.folder.locator('option:checked')).toHaveText(LONG_DIR)
 await expect(p.sentence).toHaveText(`Starts Claude Code in ${LONG_DIR} on ${LONG_HOST}. Nothing runs until you press Start.`)
 await sweep('long machine and folder names, three agents not installed',[p.start,p.machine,p.folder])
 // The sentence with a prompt, which adds the process-list line.
 await p.prompt.fill('fix the failing build please')
 await expect(p.sentence).toContainText('visible to other users of that machine in its process list.')
 await sweep('sentence with a prompt',[p.start,p.sentence])
 await p.close.click()
 await expect(cardEntry(LONG_HOST)).toBeFocused()

 // The machine you came from is not ready; then nothing is ready.
 d=await openAt(C);p=parts(d)
 await expect(p.machine).toHaveAccessibleDescription("spare-box isn't ready, so dev-laptop is selected.")
 await sweep('a machine not ready',[p.start,p.machine])
 await page.keyboard.press('Escape')
 for(const m of tmachines)m.ready=false
 d=await openAt(A);p=parts(d)
 await expect(p.start).toBeDisabled()
 await sweep('no machine ready',[p.close,p.machine])
 await page.keyboard.press('Escape')
 for(const m of tmachines)m.ready=m.agentId!==C

 // An older machine.
 d=await openAt(A);p=parts(d)
 await p.machine.selectOption(B)
 await expect(d.locator('p.reason')).toHaveText(['Update reet on this machine to start agents.'])
 await sweep('old machine',[p.close,p.machine])
 // Other model with its message; a typed path with its message; the long prompt with its counter; the leading-dash rule.
 await p.machine.selectOption(A)
 await p.model.selectOption({label:'Other…'})
 await p.other.fill('bad model')
 await expect(d.getByText("That model name isn't one reet can pass on.",{exact:false})).toBeVisible()
 await sweep('Other model with its message',[p.other,p.start])
 await p.model.selectOption('')
 await p.folder.selectOption({label:'Type a path…'})
 await p.path.fill('work/'+'a-long-relative-folder-name-without-spaces'.repeat(3))
 await expect(d.locator('p.reason').filter({hasText:'Use a full path that starts with /.'})).toBeVisible()
 await sweep('typed path with its message',[p.path,p.start])
 await p.path.fill(LONG_DIR.replace('/home/builder','/home/dev'))
 await p.prompt.fill(LONG+'b')
 await expect(p.count).toHaveText("2,001 of 2,000 bytes. Secrets don't belong here.")
 await sweep('long prompt with its counter',[p.prompt,p.count,p.start])
 await p.prompt.fill('-fix the build')
 await p.start.click()
 await expect(p.status).toHaveText('Start the prompt with a word, not a dash.')
 await sweep('prompt rule in the status line',[p.prompt,p.start])
 await p.prompt.fill('fix the failing build please')

 // Busy, then the lost reply with its Retry, then the landing.
 const matrixHold=deferred();answers.push(matrixHold)
 await p.start.click()
 await expect(p.starting).toBeDisabled()
 await sweep('busy, Starting…',[p.starting])
 // The held request answers now; the next one (the lost reply) is dropped.
 matrixHold.resolve();answers.push('drop')
 await expect(d).toHaveCount(0)
 d=await openAt(A);p=parts(d)
 await p.prompt.fill('fix the failing build please')
 await p.start.click()
 await expect(p.status).toHaveText(LOST)
 await sweep('lost reply with Retry',[p.retry])
 const landed=`session-${body(-1).requestId.replaceAll('-','').slice(0,20)}`
 await p.retry.click()
 await expect(d).toHaveCount(0)
 await expect(page).toHaveURL(new RegExp(`[?&]session=${landed}(&|$)`))
 await expect(page.locator('.pane')).toBeVisible()
 await sweep('landing on the new session',[page.locator('.pane')])

 // Each kind of refusal, with its field focused.
 d=await openAt(LONG_ID);p=parts(d)
 await p.folder.selectOption(LONG_DIR)
 await p.prompt.fill('look around here')
 for(const r of [
  {name:'folder refused',answer:{status:422,json:{error:'x',code:'folder_unavailable',message:'x'}},focus:p.folder},
  {name:'agent not installed',answer:{status:422,json:{error:'x',code:'tool_not_installed',message:'x'}},focus:p.tool('Claude Code')},
  {name:'one-word prompt refused',answer:{status:422,json:{error:'x',code:'prompt_unsupported',message:'x'}},focus:p.prompt},
  {name:'update reet',answer:{status:409,json:{error:'x',code:'launch_unsupported',message:'x'}},focus:p.machine},
  {name:'session limit',answer:{status:409,json:{error:'terminal session limit reached'}},focus:p.start},
  {name:'request changed',answer:{status:409,json:{error:'terminal operation conflicts with current state'}},focus:p.start},
  {name:'access refused',answer:{status:403,json:{error:'terminal access denied'}},focus:p.start},
  {name:'terminals switched off on the server',answer:{status:403,json:{error:'terminal access is disabled by the server'}},focus:p.start},
 ]){
  answers.push(r.answer)
  await p.start.click()
  await expect(p.start).toBeEnabled()
  await expect(r.focus).toBeFocused()
  await sweep(`refusal: ${r.name}`,[r.focus,p.start,p.status])
 }
 answers.push({status:503,json:{error:'terminal machine is reconnecting; retry after it reconnects'}})
 await p.start.click()
 await expect(p.retry).toBeFocused()
 await sweep('refusal: reconnecting (kept for retry)',[p.retry,p.status])
 await p.retry.click()
 await expect(d).toHaveCount(0)
 expect(surfaces.light).toBeTruthy();expect(surfaces.dark).toBeTruthy();expect(surfaces.dark).not.toBe(surfaces.light)   // dark is really applied
 expect(sweeps).toBe(24)

 // Escape closes without a request and focus goes back to what opened the dialog, at every size and in both schemes.
 await page.goto(`${base}/sessions?agent=${A}`)
 for(const c of COMBOS){
  await setCombo(c)
  await showSidebar()
  const sent=posts.length
  d=await openFromSidebar();p=parts(d)
  await p.prompt.fill('fix the failing build please')
  await page.keyboard.press('Escape')
  await expect(d).toHaveCount(0)
  await expect(sidebarEntry(),c.label).toBeFocused()
  await settle()
  expect(posts.length,`Escape at ${c.label} sent a request`).toBe(sent)
 }
 await page.goto(`${base}/machines`)
 for(const c of COMBOS){
  await setCombo(c)
  const sent=posts.length
  await cardEntry(LONG_HOST).click()
  d=dialog();p=parts(d)
  await p.prompt.fill('fix the failing build please')
  await page.keyboard.press('Escape')
  await expect(d).toHaveCount(0)
  await expect(cardEntry(LONG_HOST),c.label).toBeFocused()
  await settle()
  expect(posts.length,`Escape at ${c.label} sent a request`).toBe(sent)
 }
 await setCombo(COMBOS[0])

 // Reduced motion: with the preference the open dialog has no animation and no transform transition; without it the buttons'
 // press transition is there, so the check is not empty.
 const moving=()=>dialog().evaluate(el=>{
  const all=[el,...el.querySelectorAll('*')]
  return{moving:all.filter(n=>{const cs=getComputedStyle(n),names=cs.transitionProperty.split(',').map(t=>t.trim()),times=cs.transitionDuration.split(',').map(t=>parseFloat(t)*(t.trim().endsWith('ms')?1:1000))
   return cs.animationName!=='none'||names.some((name,i)=>(name==='all'||name==='transform')&&times[i%times.length]>0)}).map(n=>n.tagName+'.'+n.className),
   running:document.getAnimations().filter(a=>a.effect&&el.contains(a.effect.target)&&a.playState==='running').length}
 })
 await cardEntry('dev-laptop').click()
 expect((await moving()).moving.length).toBeGreaterThan(0)
 await page.keyboard.press('Escape')
 await page.emulateMedia({reducedMotion:'reduce'})
 await cardEntry('dev-laptop').click()
 d=dialog();p=parts(d)
 await p.prompt.fill('fix the failing build please')
 await p.start.hover()
 expect(await moving()).toEqual({moving:[],running:0})
 await page.keyboard.press('Escape')
 await page.emulateMedia({reducedMotion:'no-preference',colorScheme:'light'})

 // The matrix wrote nothing to browser storage under any key not already written before it (the sign-in token's clean-up), and
 // nothing about a prompt, model or folder. Opening the sessions list the way a person does writes no layout either.
 const keyOf=w=>w.split(' ',2).join(' ')
 const knownKeys=new Set(storageWrites.slice(0,writesBefore).map(keyOf))
 const matrixWrites=storageWrites.slice(writesBefore)
 expect(matrixWrites.filter(w=>!knownKeys.has(keyOf(w)))).toEqual([])
 for(const secret of [LONG_DIR,'fix the failing build please','look around here'])expect(matrixWrites.join('\n').includes(secret)).toBe(false)
 theme='light'
 tmachines.pop();scopes.pop();agents.pop()

 // ── Nothing about prompts, models or folders in browser storage or any address. ──
 const kept=await page.evaluate(async()=>JSON.stringify({local:{...localStorage},session:{...sessionStorage},calls:window.__storageCalls,idb:(await indexedDB.databases()).map(x=>x.name)}))
 const addresses=[...urls,...await page.evaluate(()=>window.__urls)].join('\n')
 const written=storageWrites.join('\n')
 // The storage recorder works: a write made here is seen (the workspace writes its layout only when a person changes it).
 await page.evaluate(()=>{localStorage.setItem('recorder-probe','x');localStorage.removeItem('recorder-probe')})
 expect(storageWrites.slice(-2)).toEqual(['setItem recorder-probe x','removeItem recorder-probe '])
 for(const secret of [...secrets,'tidy the readme','look around here','wait for me please','sonnet-4','/home/dev/held','purge','-fix','"tool"'])for(const where of [kept,addresses,written])expect(where.includes(secret)).toBe(false)
 expect(JSON.parse(kept).idb).toEqual([])
 expect(storageWrites.filter(w=>w.startsWith('idb-'))).toEqual([])
 // The IndexedDB recorder works: a write made here is seen (and the probe database removed again).
 await page.evaluate(()=>new Promise((resolve,reject)=>{const r=indexedDB.open('recorder-probe');r.onupgradeneeded=()=>r.result.createObjectStore('s');r.onsuccess=()=>{const tx=r.result.transaction('s','readwrite');tx.objectStore('s').put('x','k');tx.oncomplete=()=>{r.result.close();indexedDB.deleteDatabase('recorder-probe').onsuccess=resolve};tx.onerror=reject};r.onerror=reject}))
 expect(storageWrites.filter(w=>w.startsWith('idb-'))).toEqual(['idb-open recorder-probe ','idb-put s ["x","k"]','idb-deleteDatabase recorder-probe '])

 // ── Activity reads the launch event, without any prompt. ──
 await page.goto(`${base}/account#activity`)
 await expect(page.getByText(`Started Claude Code with opus in ${SITE} on dev-laptop`,{exact:true})).toBeVisible()

 // ── A viewer and a legacy sign-in mode see neither entry. ──
 viewer=true
 await page.goto(`${base}/machines`)
 await expect(page.getByRole('heading',{name:'dev-laptop',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'Start an agent'})).toHaveCount(0)
 await page.goto(`${base}/sessions?agent=${A}`)
 // The Terminals address shows the machines inventory instead of the workspace: its own heading is there, the workspace's is not.
 await expect(page).toHaveTitle('Sessions · reet.sh')
 await expect(page.getByRole('heading',{name:'3 machines, one clear view.',exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'Sessions',exact:true,level:1})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'New agent session'})).toHaveCount(0)
 viewer=false;authMode='none'
 await page.goto(`${base}/machines`)
 await expect(page.getByRole('heading',{name:'dev-laptop',exact:true})).toBeVisible()
 await expect(page.getByRole('button',{name:'Start an agent'})).toHaveCount(0)
 await page.goto(`${base}/sessions?agent=${A}`)
 // The Terminals address shows the machines inventory instead of the workspace: its own heading is there, the workspace's is not.
 await expect(page).toHaveTitle('Sessions · reet.sh')
 await expect(page.getByRole('heading',{name:'3 machines, one clear view.',exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'Sessions',exact:true,level:1})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'New agent session'})).toHaveCount(0)

 expect(errors).toEqual([])
 console.log(`Start an agent UI passed: both entry points with the machine preselected and focus returned, agents offered from what the machine reports (not installed, update reet, not ready), model suggestions and Other with cleanModel sent, folder choices and typed paths, prompt rules and byte counter, the sentence with and without a prompt, exact create bodies (launch fields only when set, plain shells unchanged), the session opened with its title, a lost reply and a 5xx retried with the same id and body while Close and Escape wait and a second submit sends nothing, seven definite refusals with exact sentences, focus and a new id each, a 503 kept for retry, Not listed? from both pages, a failed readiness read said and cleared, nothing in storage or addresses, the Activity line, no entry for viewers or legacy sign-in, and a matrix of 24 states (both closed entry points, open, long machine and folder names, the process-list sentence, not ready, nothing ready, old machine, Other model, typed path, long prompt, prompt rule, Starting…, lost reply, landing and nine refusals) at 1360, 768, 390 and 320px in light and dark with no sideways scroll, nothing past the dialog's edges and each primary action reachable and uncovered, Escape from both entry points at every size returning focus without a request, a reduced-motion run and no new storage key. Unmocked: ${[...new Set(unmocked)].join(', ')||'none'}.`)
}finally{await browser.close()}
