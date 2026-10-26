import { chromium, expect } from '@playwright/test'
// Projects: the list, the Project page and its dialogs, against synthetic API responses that follow the server's rules
// (If-Match on change and delete, one run per request id, run members that move from pending to started or failed).
// Every request the page sends is recorded, so what was sent (and that nothing was sent) is checked against the words.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const context=await browser.newContext({viewport:{width:1360,height:900}})
const page=await context.newPage()
const errors=[],unmocked=[],requests=[]
page.on('pageerror',e=>errors.push(e.message))
const deferred=()=>{let resolve;const promise=new Promise(r=>resolve=r);return{promise,resolve}}

// ── Server state. ──
const A='connector-laptop000000001',B='connector-oldbox00000001',C='connector-spare000000001',D='connector-nowtbox0000001',GONE='connector-gone0000000001'
const caps=over=>({protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,worktree:true,tools:['claude','codex'],...over})
const tmachines=[
 {agentId:A,host:'dev-laptop',ready:true,serverEnabled:true,capabilities:caps()},
 // An older connector: no launch requests, so no agents (and no worktrees).
 {agentId:B,host:'old-box',ready:true,serverEnabled:true,capabilities:caps({launch:undefined,worktree:undefined})},
 {agentId:C,host:'spare-box',ready:false,serverEnabled:true,capabilities:caps()},
 // Starts agents, but its connector predates worktrees.
 {agentId:D,host:'nowt-box',ready:true,serverEnabled:true,capabilities:caps({worktree:undefined})},
]
const now=Date.now()
const scope=(host,project,id)=>({id,host,name:host,kind:'agent',agent:'claude',project,complete:true,scannedAt:now,lastSeen:now,skills:[],skillCount:1,os:'linux',arch:'amd64',version:'dev'})
const scopes=[scope('dev-laptop','/home/dev/work/site','s1'),scope('dev-laptop','/home/dev/work/api','s2'),scope('old-box','/srv/old','s3')]
const machinesOf=()=>{const by=new Map();for(const s of scopes)by.set(s.host,[...(by.get(s.host)??[]),s]);return[...by].map(([host,groups])=>({host,groups,skillCount:1}))}
const agents=tmachines.map(m=>({id:m.agentId,host:m.host,version:'dev',paused:false,autoUpdate:false,lastSeen:now}))
const PROMPT='Read the README and tidy the tests'
const LONG='x'.repeat(110)
// The profile follows the system scheme, so emulating dark really applies the dark theme (a saved 'light' would pin it).
let authMode='accounts',viewer=false,signedOut=false,seq=0,eventSeq=0,profileTheme='system'
// How the sessions read answers: 'ok', 'fail', or a held promise.
let sessionsMode='ok',machinesMode='ok'
let projects=[],runs=[],sessions=[],events=[]
const statusAnswers=[]
// One-off answers for a request, checked first: {match(method,path), answer(route,body)}; 'pass' falls through.
const once=[]
const session=(id,agentId,state='running')=>({id,agentId,title:'Claude Code',state,createdAt:now,updatedAt:now,exitCode:null})
const view=p=>{const{sharedPrompt,...rest}=p;return rest}
const audit=(action,detail)=>events.unshift({id:++eventSeq,at:Date.now(),source:'ui',action,detail:JSON.stringify(detail)})
const project=over=>({id:'proj'+(++seq),name:'Project',defaultTool:'claude',defaultModel:'',sharedPrompt:'',hasSharedPrompt:false,useWorktrees:true,members:[],createdAt:now,updatedAt:now,version:1,...over})
const member=(host,dir,position,over={})=>({host,dir,tool:'',model:'',position,...over})

await page.route('**/api/**',async route=>{
 const req=route.request(),path=new URL(req.url()).pathname,method=req.method(),body=req.postData()
 requests.push({method,path,body,headers:req.headers()})
 for(let i=0;i<once.length;i++)if(once[i].match(method,path)){const h=once.splice(i,1)[0];if(await h.answer(route,body)!=='pass')return;break}
 const json=(data,status=200)=>route.fulfill({status,json:data})
 const role=viewer?'viewer':'owner'
 if(path==='/api/v2/auth/status')return json(authMode!=='accounts'?{mode:authMode,setupRequired:false,bootstrapRequired:false}:{mode:'accounts',setupRequired:false,bootstrapRequired:false,user:signedOut?null:{id:1,name:'Operator',email:'operator@example.test',role}})
 if(path==='/api/v2/profile')return json({user:{id:1,name:'Operator',email:'operator@example.test',role},profile:{theme:profileTheme,shelfView:'list'},sessionCount:1,accounts:authMode==='accounts'})
 if(path==='/api/skills')return json({skills:[],counts:{all:0,used:0}})
 if(path==='/api/environments')return json({environments:scopes,machines:machinesOf()})
 if(path==='/api/tags')return json({tags:[]})
 if(path==='/api/v2/lists')return json({lists:[],linkEnabled:false})
 if(path==='/api/v2/sync')return json({environments:scopes,machines:machinesOf(),agents,targets:[],jobs:[]})
 if(path==='/api/v2/machines/health')return json({serverNow:Date.now(),machines:[]})
 if(path==='/api/v2/notifications/count')return json({unread:0})
 if(path==='/api/v2/setup/profiles')return json({profiles:[]})
 if(path==='/api/version')return json({version:'v1.2.3',commit:'fixture'})
 if(path==='/api/events')return json({events})
 if(path==='/api/v2/terminal/status')return json({available:true,configured:true,reason:''})
 if(path==='/api/v2/terminal/machines'){
  // A held read answers when released; the page may have given up on it by then.
  const mode=machinesMode
  if(mode&&mode.promise){await mode.promise;return json({machines:tmachines}).catch(()=>{})}
  return json({machines:tmachines})
 }
 if(path==='/api/v2/terminal/sessions'&&method==='GET'){
  const mode=sessionsMode
  // A held read answers when released; the page may have given up on it by then.
  if(mode&&mode.promise){await mode.promise;return json({sessions}).catch(()=>{})}
  if(mode==='fail')return json({error:'terminal broker is restarting'},503)
  return json({sessions})
 }
 if(path==='/api/v2/projects'&&method==='GET')return json({projects:projects.map(view)})
 if(path==='/api/v2/projects'&&method==='POST'){
  const b=JSON.parse(body)
  if(projects.some(p=>p.name.toLowerCase()===b.name.toLowerCase()))return json({error:'A Project with that name is already saved.',code:'name_taken',field:'name'},409)
  const p=project({name:b.name,defaultTool:b.defaultTool,defaultModel:b.defaultModel??'',sharedPrompt:b.sharedPrompt??'',hasSharedPrompt:!!b.sharedPrompt,useWorktrees:b.useWorktrees??true,members:(b.members??[]).map((m,i)=>({...m,position:i}))})
  projects.push(p)
  audit('project-create',{project:p.id,name:p.name,members:p.members.length})
  return json(view(p),201)
 }
 let m=path.match(/^\/api\/v2\/projects\/([^/]+)$/)
 if(m){
  const p=projects.find(x=>x.id===m[1])
  if(!p)return json({error:"That Project isn't on this account.",code:'not_found'},404)
  if(method==='GET')return json(p)
  // As the server: no If-Match is no precondition, so a stale change is only caught when the version is sent.
  const want=req.headers()['if-match']
  if(want!==undefined&&want!==`"${p.version}"`)return json({error:'Someone changed this Project. Reload it and try again.',code:'project_changed'},409)
  if(method==='PATCH'){
   const b=JSON.parse(body),previousName=p.name
   Object.assign(p,b,{version:p.version+1,updatedAt:Date.now()})
   p.hasSharedPrompt=p.sharedPrompt!==''
   if(b.members)p.members=b.members.map((x,i)=>({...x,position:i}))
   audit('project-update',{project:p.id,name:p.name,...(previousName!==p.name?{previousName}:{}),members:p.members.length})
   return json(view(p))
  }
  if(method==='DELETE'){projects=projects.filter(x=>x!==p);audit('project-delete',{project:p.id,name:p.name,members:p.members.length});return json({ok:true,mayHaveWorktrees:true,message:'Project deleted. Its runs are kept. Worktrees those runs made, if still there, stay on their machines.'})}
 }
 m=path.match(/^\/api\/v2\/projects\/([^/]+)\/runs$/)
 if(m&&method==='GET')return json({runs:runs.filter(r=>r.projectId===m[1])})
 if(m&&method==='POST'){
  // One run per request id: the same id answers with the run it already started, whatever the version is now.
  const b=JSON.parse(body),existing=runs.find(r=>r.requestId===b.requestId)
  if(existing)return json(existing,200)
  // As the server: a run starts only from the version the sheet was made from (no If-Match is no precondition).
  const p=projects.find(x=>x.id===m[1]),want=req.headers()['if-match']
  if(p&&want!==undefined&&want!==`"${p.version}"`)return json({error:'Someone changed this Project. Reload it and try again.',code:'project_changed'},409)
  return json(createRun(m[1],b),201)
 }
 m=path.match(/^\/api\/v2\/project-runs\/([^/]+)$/)
 if(m){const r=runs.find(x=>x.id===m[1]);return r?json(r):json({error:'That run is not on this account.',code:'run_not_found'},404)}
 m=path.match(/^\/api\/v2\/project-runs\/([^/]+)\/members\/(\d+)\/retry$/)
 if(m){
  const r=runs.find(x=>x.id===m[1]),mm=r.members.find(x=>x.position===Number(m[2]))
  if(mm.state!=='failed')return json({error:'Only a repository that failed to start can be tried again.',code:'not_failed'},409)
  Object.assign(mm,{state:'pending',code:'',sentence:'',attempt:mm.attempt+1});settle(r)
  audit('project-run-retry',{project:r.projectId,run:r.id,position:mm.position,attempt:mm.attempt})
  return json(r)
 }
 if(path==='/api/v2/terminal/worktrees/status'){const a=statusAnswers.shift();if(!a){unmocked.push('an unexpected worktree status read');return json({error:'unexpected'},500)}if(a.__status){const{__status,...refusal}=a;return json(refusal,__status)}return json(a)}
 if(path==='/api/v2/terminal/worktrees/remove'){
  const b=JSON.parse(body)
  for(const r of runs)for(const mm of r.members)if(mm.worktreeRun===b.run)mm.state='removed'
  return json({ok:true,path:'/home/dev/.reet/worktrees/site/site/tok0',branch:'reet/site-tok0'})
 }
 unmocked.push(`${method} ${path}`)
 return json({error:`Unmocked ${path}`},404)
})
function createRun(projectId,b){
 const p=projects.find(x=>x.id===projectId)
 const r={id:'run'+(++seq),requestId:b.requestId,projectId:p.id,projectName:p.name,state:'running',startedAt:Date.now(),useWorktrees:p.useWorktrees,worktreeProject:'site',members:b.positions.map(pos=>{const mm=p.members.find(x=>x.position===pos);const tool=mm.tool||p.defaultTool;return{position:pos,host:mm.host,dir:mm.dir,tool,model:mm.model||(tool===p.defaultTool?p.defaultModel:''),state:'pending',code:'',sentence:'',sessionId:'',worktreePath:'',worktreeRepo:'',worktreeBranch:'',worktreeRun:'',worktreeProject:'',sharesCheckout:!p.useWorktrees,attempt:1}})}
 runs.unshift(r)
 audit('project-run',{project:p.id,run:r.id,members:r.members.length})
 return r
}
// A run's state from its members, as the server sets it.
function settle(r){const s=r.members.map(x=>x.state);r.state=s.includes('pending')?'running':!s.includes('failed')?'complete':s.some(x=>x==='started'||x==='removed')?'partial':'failed'}
function startMember(r,position){
 const mm=r.members.find(x=>x.position===position),repo=mm.dir.split('/').pop()
 Object.assign(mm,{state:'started',sessionId:`sess-${position}-${mm.attempt}`,worktreePath:`/home/dev/.reet/worktrees/site/${repo}/tok${position}`,worktreeRepo:repo,worktreeBranch:`reet/site-tok${position}`,worktreeRun:'tok'+position,worktreeProject:'site'})
 sessions.push(session(mm.sessionId,mm.host))
 settle(r)
}
function failMember(r,position){Object.assign(r.members.find(x=>x.position===position),{state:'failed',code:'repo_busy',sentence:'That repository is in the middle of a merge, rebase or similar. Finish or abort it first.'});settle(r)}

// Every write to browser storage, recorded from before the page loads and kept across page loads.
const storageWrites=[]
await page.exposeFunction('__recordStorage',(op,key,value)=>{storageWrites.push(`${op} ${key} ${value}`)})
await page.addInitScript(()=>{
 for(const op of ['setItem','removeItem','clear']){const original=Storage.prototype[op];Storage.prototype[op]=function(...args){window.__recordStorage(op,String(args[0]??''),String(args[1]??''));return original.apply(this,args)}}
 for(const op of ['open','deleteDatabase']){const original=IDBFactory.prototype[op];IDBFactory.prototype[op]=function(...args){window.__recordStorage('idb-'+op,String(args[0]??''),'');return original.apply(this,args)}}
 window.__urls=[]
 for(const op of ['pushState','replaceState']){const original=history[op].bind(history);history[op]=(state,title,url)=>{window.__urls.push(String(url??''));return original(state,title,url)}}
})

const sent=(method,re)=>requests.filter(r=>r.method===method&&re.test(r.path))
const projectCalls=list=>list.filter(r=>r.path.startsWith('/api/v2/projects')||r.path.startsWith('/api/v2/project-runs')||r.path.startsWith('/api/v2/terminal/worktrees'))
const UUID=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const settleUI=()=>page.evaluate(()=>fetch('/api/version').then(r=>r.status))
const wake=()=>page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')))
// No sideways scroll on the page, and nothing wider than an open dialog.
async function noScroll(where){
 const w=await page.evaluate(()=>({page:document.documentElement.scrollWidth<=innerWidth,dialogs:[...document.querySelectorAll('dialog[open]')].map(d=>d.scrollWidth<=d.clientWidth&&d.getBoundingClientRect().right<=innerWidth)}))
 expect(w,`${where}: scrolls sideways`).toEqual({page:true,dialogs:w.dialogs.map(()=>true)})
}
const sheet=()=>page.getByRole('dialog',{name:'Start all',exact:true})
const startAll=()=>page.getByRole('button',{name:'Start all',exact:true})
const removeButtons=()=>page.getByRole('button',{name:/^Remove worktree/})

try{
 // ── The empty list, and the navigation entry. ──
 await page.goto(`${base}/projects`)
 await expect(page).toHaveTitle('Projects · reet.sh')
 await expect(page.getByRole('heading',{name:'No Projects yet.'})).toBeVisible()
 // Projects sits under More in the account menu (R13) until PR B retires the page.
 await page.locator('summary[aria-label^="Account menu"]').click()
 await expect(page.locator('.panel').getByRole('group',{name:'More'}).getByRole('link',{name:'Projects'})).toHaveAttribute('aria-current','page')
 await page.keyboard.press('Escape')

 // ── Create: checked before sending, one save per press, Escape and Close wait while it is in flight. ──
 const newButton=page.getByRole('button',{name:'New project'})
 await newButton.click()
 const pd=page.getByRole('dialog',{name:'New project'})
 await expect(pd.getByRole('textbox',{name:'Name'})).toBeFocused()
 await pd.getByRole('button',{name:'Save Project'}).click()
 await expect(pd.getByRole('status')).toHaveText('Give the Project a name of 1 to 80 characters, without line breaks or control characters.')
 await pd.getByRole('textbox',{name:'Name'}).fill('Site')
 await pd.getByRole('button',{name:'Add a repository'}).click()
 await pd.getByRole('combobox',{name:'Folder',exact:true}).nth(0).selectOption('/home/dev/work/site')
 await pd.getByRole('button',{name:'Add a repository'}).click()
 await pd.getByRole('combobox',{name:'Folder',exact:true}).nth(1).selectOption('/home/dev/work/api')
 await pd.getByRole('combobox',{name:'Agent',exact:true}).nth(1).selectOption('codex')
 await pd.getByRole('button',{name:'Add a repository'}).click()
 await pd.getByRole('combobox',{name:'Machine',exact:true}).nth(2).selectOption(B)
 await pd.getByRole('combobox',{name:'Folder',exact:true}).nth(2).selectOption('/srv/old')
 // The same folder written another way is the same repository.
 await pd.getByRole('button',{name:'Add a repository'}).click()
 await pd.getByRole('combobox',{name:'Folder',exact:true}).nth(3).selectOption({label:'Type a path…'})
 await pd.getByRole('textbox',{name:'Folder path'}).fill('/home/dev/work//site/.')
 await pd.getByRole('textbox',{name:'Shared prompt'}).fill(PROMPT)
 await pd.getByRole('button',{name:'Save Project'}).click()
 await expect(pd.getByRole('status')).toHaveText('Each machine and folder can be in a Project only once.')
 expect(sent('POST',/^\/api\/v2\/projects$/)).toHaveLength(0)
 await pd.getByRole('button',{name:'Remove repository 4 from this Project'}).click()
 const held=deferred()
 once.push({match:(m,p)=>m==='POST'&&p==='/api/v2/projects',answer:async()=>{await held.promise;return'pass'}})
 await pd.getByRole('button',{name:'Save Project'}).click()
 await expect(pd.getByRole('button',{name:'Saving…'})).toBeDisabled()
 await page.keyboard.press('Escape')
 await expect(pd).toBeVisible()
 await expect(pd.getByRole('button',{name:'Close'})).toBeDisabled()
 await pd.locator('form').evaluate(f=>f.requestSubmit())
 held.resolve()
 await expect(pd).toBeHidden()
 await expect(page.getByRole('status').filter({hasText:'Grand, Site is saved.'})).toBeVisible()
 await expect(newButton).toBeFocused()
 const creates=sent('POST',/^\/api\/v2\/projects$/)
 expect(creates).toHaveLength(1)
 expect(creates[0].headers['if-match']).toBeUndefined()
 expect(JSON.parse(creates[0].body)).toEqual({name:'Site',defaultTool:'claude',defaultModel:'',useWorktrees:true,sharedPrompt:PROMPT,members:[{host:A,dir:'/home/dev/work/site',tool:'',model:''},{host:A,dir:'/home/dev/work/api',tool:'codex',model:''},{host:B,dir:'/srv/old',tool:'',model:''}]})
 await expect(page.getByRole('link',{name:'Site',exact:true})).toBeVisible()
 const site=projects[0]
 // A repository on a machine that has since left the account.
 site.members.push(member(GONE,'/srv/gone',3))

 // ── Change: If-Match with the version read; the shared prompt is kept across two saves. ──
 await page.getByRole('link',{name:'Site',exact:true}).click()
 await expect(page.getByRole('heading',{level:1,name:'Site'})).toBeVisible()
 const ed=page.getByRole('dialog',{name:'Change Site'})
 await page.getByRole('button',{name:'Change',exact:true}).click()
 await expect(ed.getByRole('textbox',{name:'Shared prompt'})).toHaveValue(PROMPT)
 await ed.getByRole('combobox',{name:'Default model'}).fill('opus')
 await ed.getByRole('button',{name:'Save changes'}).click()
 await expect(ed).toBeHidden()
 await expect(page.getByText('Saved. Runs already started keep what they used.')).toBeVisible()
 await page.getByRole('button',{name:'Change',exact:true}).click()
 await expect(ed.getByRole('textbox',{name:'Shared prompt'})).toHaveValue(PROMPT)
 await expect(ed.getByRole('textbox',{name:'Shared prompt'})).toBeEnabled()
 await ed.getByRole('combobox',{name:'Default model'}).fill('sonnet')
 await ed.getByRole('button',{name:'Save changes'}).click()
 await expect(ed).toBeHidden()
 let patches=sent('PATCH',/^\/api\/v2\/projects\/proj1$/)
 expect(patches.map(r=>r.headers['if-match'])).toEqual(['"1"','"2"'])
 // An unchanged prompt is never sent: the save reply leaves it out, and "" would clear it.
 for(const r of patches)expect(JSON.parse(r.body)).not.toHaveProperty('sharedPrompt')
 expect(site.sharedPrompt).toBe(PROMPT)
 expect(site.defaultModel).toBe('sonnet')
 await expect(page.getByText("Set. It is sent as each repository's opening prompt",{exact:false})).toBeVisible()
 // Someone else saves meanwhile: 409 project_changed, nothing changed, and Reload reads it again.
 site.version=7
 await page.getByRole('button',{name:'Change',exact:true}).click()
 await ed.getByRole('textbox',{name:'Name'}).fill('Site two')
 await ed.getByRole('button',{name:'Save changes'}).click()
 await expect(ed.getByRole('status')).toHaveText('Someone changed this Project. Reload it and try again.')
 await expect(ed.getByRole('button',{name:'Reload the Project'})).toBeFocused()
 expect(sent('PATCH',/proj1$/).at(-1).headers['if-match']).toBe('"3"')
 expect(site.name).toBe('Site')
 await ed.getByRole('button',{name:'Reload the Project'}).click()
 await expect(ed).toBeHidden()
 await expect(page.getByText('Read the Project again. Make your change once more if it is still needed.')).toBeVisible()
 await page.getByRole('button',{name:'Change',exact:true}).click()
 await expect(ed.getByRole('textbox',{name:'Name'})).toHaveValue('Site')
 await expect(ed.getByRole('textbox',{name:'Shared prompt'})).toHaveValue(PROMPT)
 await page.keyboard.press('Escape')
 await expect(ed).toBeHidden()
 expect(sent('PATCH',/proj1$/)).toHaveLength(3)

 // ── Start all from a sheet made before another tab saved the Project: nothing starts, the Project is read again. ──
 await startAll().click()
 await expect(sheet()).toBeVisible()
 site.version++
 await sheet().getByRole('button',{name:'Start',exact:true}).click()
 await expect(sheet()).toBeHidden()
 await expect(page.getByText('This Project changed since that sheet was made, so nothing was started. Look it over, then press Start all again.')).toBeVisible()
 await expect(startAll()).toBeFocused()
 expect(sent('POST',/^\/api\/v2\/projects\/proj1\/runs$/).map(r=>r.headers['if-match'])).toEqual([`"${site.version-1}"`])
 expect(runs).toHaveLength(0)

 // ── Start all: the sheet, then a lost reply resent with the same request id and positions. ──
 await startAll().click()
 await expect(sheet()).toBeVisible()
 await expect(sheet().getByText("Won't start: Update reet on that machine to start agents.",{exact:true})).toBeVisible()
 await expect(sheet().getByText("Won't start: That machine isn't enrolled on this account.",{exact:true})).toBeVisible()
 await expect(sheet().getByText('A machine no longer on this account',{exact:true})).toBeVisible()
 await expect(sheet().getByText('Claude Code · sonnet · new worktree and branch',{exact:true})).toBeVisible()
 // A repository on another agent does not take the default agent's model.
 await expect(sheet().getByText('Codex · Agent default · new worktree and branch',{exact:true})).toBeVisible()
 await expect(sheet().locator('.sentence')).toHaveText('Starts 2 agents, each in its own git worktree and branch under ~/.reet/worktrees on its machine. reet never removes a worktree by itself. Nothing runs until you press Start. The prompt is kept with the session record and is visible to other users of that machine in its process list.')
 const boxes=sheet().getByRole('checkbox')
 await expect(boxes).toHaveCount(4)
 for(const i of [0,1]){await expect(boxes.nth(i)).toBeChecked();await expect(boxes.nth(i)).toBeEnabled()}
 for(const i of [2,3]){await expect(boxes.nth(i)).not.toBeChecked();await expect(boxes.nth(i)).toBeDisabled()}
 await expect(sheet().getByRole('button',{name:'Add a machine'})).toHaveCount(1)
 // The run starts on the server, but the reply never arrives.
 once.push({match:(m,p)=>m==='POST'&&/\/runs$/.test(p),answer:(route,body)=>{createRun(new URL(route.request().url()).pathname.split('/')[4],JSON.parse(body));return route.abort('failed')}})
 await sheet().getByRole('button',{name:'Start',exact:true}).click()
 await expect(sheet().getByRole('status')).toHaveText("We didn't hear back, so the run may have started. Try the same request again: it can't start a second run.")
 // What was asked for can't change while its outcome is unknown.
 for(const i of [0,1])await expect(boxes.nth(i)).toBeDisabled()
 await sheet().getByRole('button',{name:'Try the same request again'}).click()
 await expect(sheet()).toBeHidden()
 const starts=sent('POST',/^\/api\/v2\/projects\/proj1\/runs$/).slice(1).map(r=>JSON.parse(r.body))
 expect(starts).toHaveLength(2)
 expect(starts[0].requestId).toMatch(UUID)
 expect(starts[1]).toEqual(starts[0])
 // Both the lost start and its resend name the version the sheet was made from (the reloaded one).
 expect(sent('POST',/^\/api\/v2\/projects\/proj1\/runs$/).slice(1).map(r=>r.headers['if-match'])).toEqual([`"${site.version}"`,`"${site.version}"`])
 expect(starts[0].positions).toEqual([0,1])
 expect(runs).toHaveLength(1)
 const run=runs[0]
 await expect(page.getByText("Right, that's under way: 2 repositories starting.",{exact:false})).toBeVisible()
 await expect(page.getByRole('heading',{name:'Runs'})).toBeFocused()

 // ── The run: pending, then started and failed, then Retry. ──
 const card=page.locator('details.run-card').first()
 await expect(card.locator('summary .state')).toHaveText('Starting')
 await expect(card.getByText('Waiting to start.')).toHaveCount(2)
 await expect(card.getByText('Starting: 0 of 2 started so far.',{exact:false})).toBeVisible()
 startMember(run,0);failMember(run,1)
 await expect(card.getByText('That repository is in the middle of a merge, rebase or similar. Finish or abort it first.')).toBeVisible({timeout:6000})
 await expect(card.getByText('1 of 2 started, 1 failed.')).toBeVisible()
 await expect(card.locator('summary .state')).toHaveText('Some failed')
 await expect(card.locator('li[data-state="started"]').getByText('Started.',{exact:true})).toBeVisible()
 await expect(card.getByRole('link',{name:'Open Claude Code on dev-laptop in /home/dev/work/site'})).toHaveAttribute('href',`/terminals?agent=${A}&session=sess-0-1`)
 // Nothing is pending, so the run is no longer read.
 const reads=()=>sent('GET',/^\/api\/v2\/project-runs\//).length
 const readsAtRest=reads()
 await page.waitForTimeout(4500)
 expect(reads()).toBe(readsAtRest)
 const retry=card.getByRole('button',{name:'Retry dev-laptop /home/dev/work/api'})
 await retry.click()
 await expect(card.locator('li[data-state="pending"]').getByText('Waiting to start.')).toBeVisible()
 await expect(card.getByText('Codex · Agent default · attempt 2')).toBeVisible()
 await expect(card.locator('.run > .status')).toHaveText('Trying dev-laptop /home/dev/work/api again. Its state shows below.')
 expect(sent('POST',/\/retry$/).map(r=>r.path)).toEqual([`/api/v2/project-runs/${run.id}/members/1/retry`])
 startMember(run,1)
 await expect(card.getByText('All 2 started.')).toBeVisible({timeout:6000})
 await expect(card.locator('summary .state')).toHaveText('All started')
 await expect(card.locator('.run > .status')).toBeHidden()

 // ── Remove worktree: never offered while a session runs or while the sessions are not known. ──
 await expect(removeButtons()).toHaveCount(0)
 await expect(card.getByText('End its session before removing its worktree.')).toHaveCount(2)
 sessions.find(s=>s.id==='sess-0-1').state='ended'
 sessionsMode='fail'
 await wake()
 await expect(card.getByText("Couldn't check whether its session has ended, so its worktree can't be removed from here just now. Reload the page to try again.")).toHaveCount(2)
 await expect(removeButtons()).toHaveCount(0)
 // Not read yet: still not offered.
 const sessionsHeld=deferred()
 sessionsMode=sessionsHeld
 await page.reload()
 await expect(page.getByText('All 2 started.')).toBeVisible()
 await expect(page.getByText('Checking whether its session has ended…')).toHaveCount(2)
 await expect(removeButtons()).toHaveCount(0)
 sessionsMode='ok';sessionsHeld.resolve()
 const removeButton=page.getByRole('button',{name:'Remove worktree for dev-laptop /home/dev/work/site'})
 await expect(removeButton).toBeVisible()
 // The other member's session still runs.
 await expect(removeButtons()).toHaveCount(1)
 await expect(page.getByText('End its session before removing its worktree.')).toHaveCount(1)

 // ── Remove worktree: the status first; blocked while there are changes; Check again; the confirmation states the counts. ──
 const wt={path:'/home/dev/.reet/worktrees/site/site/tok0',branch:'reet/site-tok0',changed:0,unpushed:1,paths:[],unreferenced:0,hidden:0,nested:false,incomplete:false}
 statusAnswers.push({...wt,changed:23,paths:Array.from({length:20},(_,i)=>`src/${LONG}/file${i}.ts`)},{...wt,unreferenced:2},{...wt})
 const worktreeCalls=()=>requests.filter(r=>r.path.startsWith('/api/v2/terminal/worktrees/'))
 await removeButton.click()
 const wd=page.getByRole('dialog',{name:'Remove worktree'})
 await expect(wd.getByText("This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again.")).toBeVisible()
 expect(worktreeCalls().map(r=>r.path)).toEqual(['/api/v2/terminal/worktrees/status'])
 expect(JSON.parse(worktreeCalls()[0].body)).toEqual({agentId:A,project:'site',repo:'site',run:'tok0'})
 await expect(wd.getByText('23 files with uncommitted changes')).toBeVisible()
 await expect(wd.getByText('and 3 more not listed.')).toBeVisible()
 await expect(wd.getByRole('button',{name:/^Remove with/})).toHaveCount(0)
 await page.setViewportSize({width:320,height:740})
 await noScroll('worktree dialog with changes at 320px')
 await page.setViewportSize({width:1360,height:900})
 await wd.getByRole('button',{name:'Check again'}).click()
 await expect(wd.getByText('Some commits in this worktree are not on any branch. Put them on a branch first, then check again.')).toBeVisible()
 await expect(wd.getByRole('button',{name:/^Remove with/})).toHaveCount(0)
 await wd.getByRole('button',{name:'Check again'}).click()
 await expect(wd.locator('.confirm')).toHaveText('Remove the worktree at /home/dev/.reet/worktrees/site/site/tok0? It has no uncommitted changes and 1 unpushed commit, which stays on the branch. Ignored files in it, such as .env or node_modules, are deleted with the folder. The branch reet/site-tok0 is kept.')
 await expect(wd.getByText('No uncommitted changes',{exact:true})).toBeVisible()
 expect(worktreeCalls().filter(r=>r.path.endsWith('/remove'))).toHaveLength(0)
 // The session is looked at again just before: one that runs again stops the remove.
 sessions.find(s=>s.id==='sess-0-1').state='running'
 await wd.getByRole('button',{name:'Remove with 1 unpushed commit'}).click()
 await expect(wd.getByRole('status')).toHaveText('Its session is still running, so nothing was removed. End its session first.')
 expect(worktreeCalls().filter(r=>r.path.endsWith('/remove'))).toHaveLength(0)
 sessions.find(s=>s.id==='sess-0-1').state='ended'
 await wd.getByRole('button',{name:'Remove with 1 unpushed commit'}).click()
 await expect(wd.getByRole('status')).toHaveText('Sorted. The worktree at /home/dev/.reet/worktrees/site/site/tok0 is removed. The branch reet/site-tok0 is kept.')
 const removes=worktreeCalls().filter(r=>r.path.endsWith('/remove'))
 expect(removes).toHaveLength(1)
 expect(JSON.parse(removes[0].body)).toEqual({agentId:A,project:'site',repo:'site',run:'tok0',changed:0,unpushed:1})
 await wd.getByRole('button',{name:'Done'}).click()
 await expect(wd).toBeHidden()
 await expect(page.getByText('Started. Its worktree has been removed.')).toBeVisible()
 await expect(page.locator('li[data-state="removed"]')).toHaveCount(1)
 await expect(removeButtons()).toHaveCount(0)
 await expect(page.getByText('All 2 started.')).toBeVisible()
 expect(statusAnswers).toHaveLength(0)

 // ── The sheet names each reason a repository won't start, and counts what it plans against the session limits. ──
 const every=project({name:'Everything',members:[
  member(A,'/home/dev/work/site',0),member(A,'/home/dev/work/api',1,{tool:'codex'}),member(GONE,'/srv/gone',2),member(C,'/srv/spare',3),
  member(B,'/srv/old',4),member(A,'/home/dev/work/docs',5,{tool:'gemini'}),member(D,'/srv/nowt',6)]})
 projects.push(every)
 const kept=sessions
 const reason={
  machine:'That machine already has 8 sessions, the most it can have. End one, then start again.',
  account:'This account already has 32 sessions, the most it can have. End one, then start again.',
  enrolled:"That machine isn't enrolled on this account.",ready:'The machine is reconnecting. Existing shells will be kept.',
  launch:'Update reet on that machine to start agents.',tool:"That agent isn't installed on that machine.",worktree:'Update reet on that machine to start agents with worktrees.'}
 const rows=()=>sheet().locator('li')
 async function expectRows(expected){
  await expect(rows()).toHaveCount(expected.length)
  for(const[i,why]of expected.entries()){
   const box=rows().nth(i).getByRole('checkbox')
   if(why===''){await expect(box).toBeChecked();await expect(box).toBeEnabled();await expect(rows().nth(i).locator('.why')).toHaveCount(0)}
   else{await expect(box).not.toBeChecked();await expect(box).toBeDisabled();await expect(rows().nth(i).locator('.why')).toHaveText(`Won't start: ${why}`)}
  }
 }
 // Seven open sessions on dev-laptop (an ended one does not count): the first repository there makes eight.
 sessions=[...Array.from({length:7},(_,i)=>session(`busy-${i}`,A)),session('done-1',A,'ended')]
 await page.goto(`${base}/projects/${every.id}`)
 await startAll().click()
 await expectRows(['',reason.machine,reason.enrolled,reason.ready,reason.launch,reason.tool,reason.worktree])
 await expect(sheet().getByRole('button',{name:'Add a machine'})).toHaveCount(1)
 await expect(rows().nth(2).getByRole('button',{name:'Add a machine'})).toHaveCount(1)
 await expect(sheet().locator('.sentence')).toHaveText('Starts 1 agent, each in its own git worktree and branch under ~/.reet/worktrees on its machine. reet never removes a worktree by itself. Nothing runs until you press Start.')
 await rows().nth(0).getByRole('checkbox').uncheck()
 await expect(sheet().locator('.sentence')).toHaveText('No repository can start. Nothing runs until you press Start.')
 await expect(sheet().getByRole('button',{name:'Start',exact:true})).toBeDisabled()
 await page.keyboard.press('Escape')
 await expect(sheet()).toBeHidden()
 await expect(startAll()).toBeFocused()
 // Thirty-one open sessions elsewhere on the account: the first repository makes thirty-two.
 sessions=Array.from({length:31},(_,i)=>session(`far-${i}`,`connector-far${String(i%5).padStart(12,'0')}`))
 await startAll().click()
 await expectRows(['',reason.account,reason.enrolled,reason.ready,reason.launch,reason.tool,reason.worktree])
 await page.setViewportSize({width:320,height:740})
 await noScroll('sheet with every reason at 320px')
 await page.setViewportSize({width:1360,height:900})
 await page.keyboard.press('Escape')
 // Starting needs the account's sessions: when they can't be read, the sheet does not open.
 sessionsMode='fail'
 await startAll().click()
 await expect(page.getByText("Couldn't check your machines and sessions just now, so Start all can't open. Try again in a moment.")).toBeVisible()
 await expect(sheet()).toHaveCount(0)
 sessionsMode='ok'
 sessions=kept
 expect(sent('POST',new RegExp(`/projects/${every.id}/runs$`))).toHaveLength(0)

 // ── No sideways scroll at 320px: the list, the Project page and its dialogs, with long names and paths. ──
 projects.push(project({name:'A long name '+'y'.repeat(60),members:[member(A,`/home/dev/${LONG}/${LONG}`,0)]}))
 await page.setViewportSize({width:320,height:740})
 for(const theme of ['light','dark']){
  await page.emulateMedia({colorScheme:theme})
  await page.goto(`${base}/projects`)
  await expect(page.getByRole('link',{name:'Site',exact:true})).toBeVisible()
  await noScroll(`Projects list at 320px, ${theme}`)
  await page.goto(`${base}/projects/${projects.at(-1).id}`)
  await expect(page.getByRole('heading',{level:1})).toContainText('A long name')
  await noScroll(`long Project page at 320px, ${theme}`)
  await page.goto(`${base}/projects/proj1`)
  await expect(page.getByText('All 2 started.')).toBeVisible()
  await noScroll(`Project page with a run at 320px, ${theme}`)
  await startAll().click()
  await expect(sheet()).toBeVisible()
  await noScroll(`Start all at 320px, ${theme}`)
  await page.keyboard.press('Escape')
  await page.getByRole('button',{name:'Change',exact:true}).click()
  await expect(ed).toBeVisible()
  await noScroll(`Change at 320px, ${theme}`)
  await page.keyboard.press('Escape')
  await page.getByRole('button',{name:'Delete',exact:true}).click()
  await expect(page.getByRole('dialog',{name:'Delete Site?'})).toBeVisible()
  await noScroll(`Delete at 320px, ${theme}`)
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog',{name:'Delete Site?'})).toBeHidden()
 }
 await page.emulateMedia({colorScheme:'light'})
 await page.setViewportSize({width:1360,height:900})

 // ── Delete: If-Match; a changed Project is not deleted unseen, and Reload reads it again. ──
 site.version=9
 const dd=page.getByRole('dialog',{name:'Delete Site?'})
 await page.getByRole('button',{name:'Delete',exact:true}).click()
 await expect(dd.getByText('Worktrees its runs made, if still there, stay on their machines.',{exact:false})).toBeVisible()
 await dd.getByRole('button',{name:'Delete Project'}).click()
 await expect(dd.getByRole('status')).toHaveText('Someone changed this Project. Reload it and try again.')
 expect(sent('DELETE',/proj1$/).map(r=>r.headers['if-match'])).toEqual(['"8"'])
 expect(projects).toContain(site)
 await dd.getByRole('button',{name:'Reload the Project'}).click()
 await expect(dd).toBeHidden()
 await expect(page.getByText('Read the Project again.',{exact:false})).toBeVisible()
 await page.getByRole('button',{name:'Delete',exact:true}).click()
 await dd.getByRole('button',{name:'Delete Project'}).click()
 await expect(page.getByRole('heading',{name:'Project deleted.'})).toBeVisible()
 await expect(page.getByText('Project deleted. Its runs are kept. Worktrees those runs made, if still there, stay on their machines.')).toBeVisible()
 expect(sent('DELETE',/proj1$/).map(r=>r.headers['if-match'])).toEqual(['"8"','"9"'])
 expect(projects).not.toContain(site)

 // ── A Project without worktrees warns that sessions share the checkout; a new run gets a new request id; 401 hands over to sign-in. ──
 const plain=project({name:'Plain',useWorktrees:false,members:[member(A,'/home/dev/work/site',0)]})
 projects.push(plain)
 // Machine reads that never answer hold nothing back: the list and the Project page show at once, naming machines by
 // their ids, and New project and Change say they can't offer the machines until a read answers.
 const heldMachines=deferred()
 machinesMode=heldMachines
 await page.goto(`${base}/projects`)
 await expect(page.getByRole('link',{name:'Plain',exact:true})).toBeVisible({timeout:5000})
 const plainCard=()=>page.locator('ul.projects li').filter({has:page.getByRole('link',{name:'Plain',exact:true})})
 await expect(plainCard().getByText(`1 repository on ${A}`,{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'New project',exact:true}).click()
 await expect(page.getByText("Couldn't read your machines just now, so a new Project can't offer them. Try again in a moment.")).toBeVisible({timeout:35000})
 await expect(page.getByRole('dialog',{name:'New project'})).toHaveCount(0)
 await page.goto(`${base}/projects/${plain.id}`)
 await expect(page.getByRole('heading',{level:1,name:'Plain'})).toBeVisible({timeout:5000})
 await expect(page.locator('.repos').getByText(A,{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Change',exact:true}).click()
 await expect(page.getByText("Couldn't read your machines just now, so the Project can't be changed from here. Try again in a moment.")).toBeVisible({timeout:35000})
 machinesMode='ok';heldMachines.resolve()
 await page.goto(`${base}/projects`)
 await expect(plainCard().getByText('1 repository on dev-laptop',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'New project',exact:true}).click()
 await expect(page.getByRole('dialog',{name:'New project'})).toBeVisible()
 await page.keyboard.press('Escape')
 // Sessions reads that never answer are given up after 15 seconds each: Start all says so and opens nothing, then works
 // once the reads answer again. Nothing is planned from an unknown list.
 const stalled=deferred()
 sessionsMode=stalled
 await page.goto(`${base}/projects/${plain.id}`)
 await startAll().click()
 await expect(page.getByText("Couldn't check your machines and sessions just now, so Start all can't open. Try again in a moment.")).toBeVisible({timeout:45000})
 await expect(sheet()).toHaveCount(0)
 await expect(startAll()).toBeEnabled()
 sessionsMode='ok';stalled.resolve()
 await startAll().click()
 await expect(sheet().locator('.warn')).toHaveText('This Project makes no worktrees. Each agent works in the saved folder itself, so any other session there shares that checkout and its files.')
 await expect(sheet().getByText('Claude Code · Agent default · in the saved folder',{exact:true})).toBeVisible()
 once.push({match:(m,p)=>m==='POST'&&/\/runs$/.test(p),answer:route=>{signedOut=true;return route.fulfill({status:401,json:{error:'unauthorized'}})}})
 await sheet().getByRole('button',{name:'Start',exact:true}).click()
 await expect(page.getByRole('heading',{name:/Sign in/}).first()).toBeVisible()
 await expect(sheet()).toHaveCount(0)
 const plainStart=JSON.parse(sent('POST',new RegExp(`/projects/${plain.id}/runs$`))[0].body)
 expect(plainStart.requestId).toMatch(UUID)
 expect(plainStart.requestId).not.toBe(starts[0].requestId)
 signedOut=false

 // ── Activity names each Project change, without the prompt. ──
 await page.goto(`${base}/account#activity`)
 const activity=page.locator('ul.events')
 for(const line of ['Created project Site with 3 repositories','Updated project Site','Deleted project Site','Started a Project run on 2 repositories','Tried repository 2 of a Project run again (attempt 2)'])await expect(activity.getByText(line,{exact:true}).first()).toBeVisible()
 await expect(activity.getByText('Updated project Site',{exact:true})).toHaveCount(2)
 expect(await page.locator('main').innerText()).not.toContain(PROMPT)

 // ── Nothing about Projects in browser storage or any address. ──
 const addresses=[...await page.evaluate(()=>window.__urls),page.url()].join('\n')
 const written=storageWrites.join('\n')
 for(const secret of [PROMPT,'Site','Everything','/home/dev/work','proj1',starts[0].requestId,'sonnet','tok0'])expect(written.includes(secret),`storage holds ${secret}`).toBe(false)
 for(const secret of [PROMPT,starts[0].requestId,'sonnet','/home/dev/work'])expect(addresses.includes(secret),`an address holds ${secret}`).toBe(false)
 expect(storageWrites.filter(w=>w.startsWith('idb-'))).toEqual([])
 expect(await page.evaluate(()=>JSON.stringify({...localStorage,...sessionStorage}))).not.toMatch(/project|prompt|run/i)

 // ── Matrix: every state at 1360, 768, 390 and 320px in light and dark. ──
 // One walk through the states; at each, the viewport and scheme are changed under the open page or dialog (no reload) and
 // the same checks are made: no sideways scroll of the page, an open dialog inside the window with neither it, its scrolling
 // part nor anything in it reaching past its edges, and each primary action scrolled into view with a hit test at its centre
 // landing on it. A machine with a long unbroken name, an 80-character unbroken Project name and long folders are used, so
 // wrapping is tested with values that cannot break at a space.
 const COMBOS=[];for(const scheme of ['light','dark'])for(const [width,height] of [[1360,900],[768,900],[390,800],[320,640]])COMBOS.push({scheme,width,height,label:`${scheme} ${width}px`})
 const setCombo=async c=>{await page.emulateMedia({colorScheme:c.scheme});await page.setViewportSize({width:c.width,height:c.height});await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))))}
 const reachable=async locator=>{
  await locator.scrollIntoViewIfNeeded()
  // A link that wraps is hit on its first line box: the centre of its whole box can fall between its lines. Scrolling can
  // leave a box a fraction of a pixel past the edge, so 1px is allowed.
  return locator.evaluate(el=>{const r=el.getBoundingClientRect(),hit=el.getClientRects()[0]??r,top=document.elementFromPoint(hit.left+hit.width/2,hit.top+hit.height/2);return r.width>0&&r.left>=-1&&r.right<=innerWidth+1&&r.top>=-1&&r.bottom<=innerHeight+1&&(top===el||el.contains(top))||`${el.tagName}.${el.className} "${el.textContent.trim().slice(0,40)}" at ${Math.round(r.left)},${Math.round(r.top)}-${Math.round(r.right)},${Math.round(r.bottom)} in ${innerWidth}x${innerHeight}, under ${top?.tagName}.${top?.className}`})
 }
 const openDialogs=()=>page.locator('dialog[open]')
 const surfaces={}
 const swept=[]
 async function sweep(state,primary){
  for(const c of COMBOS){
   await setCombo(c)
   const where=`${state} at ${c.label}`
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${where}: the page scrolls sideways`).toBe(true)
   if(await openDialogs().count()){
    const fit=await openDialogs().last().evaluate(el=>{
     const r=el.getBoundingClientRect(),body=el.querySelector('.body')
     const spill=[...el.querySelectorAll('*')].filter(n=>{const b=n.getBoundingClientRect();return b.width>0&&b.height>0&&(b.left<r.left-0.5||b.right>r.right+0.5)}).map(n=>n.tagName+'.'+n.className)
     return{inside:r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight,dialog:el.scrollWidth<=el.clientWidth,body:!body||body.scrollWidth<=body.clientWidth,spill,surface:getComputedStyle(el).backgroundColor}
    })
    expect({...fit,surface:undefined},`${where}: the dialog does not fit`).toEqual({inside:true,dialog:true,body:true,spill:[],surface:undefined})
    surfaces[`dialog ${c.scheme}`]=fit.surface
   }else surfaces[`page ${c.scheme}`]=await page.evaluate(()=>getComputedStyle(document.body).backgroundColor)
   for(const locator of primary)expect(await reachable(locator),`${where}: an action is out of reach or covered`).toBe(true)
  }
  swept.push(state)
  await setCombo(COMBOS[0])
 }
 const LONG_HOST='build-server-in-the-back-office-with-a-very-long-hostname-01',LONG_ID='connector-longname000001'
 const LONG_DIR='/home/builder/work/clients/a-very-long-client-name-with-no-spaces-at-all/and-an-even-longer-project-folder-name-that-must-wrap'
 const LONG_NAME='Matrix'+'n'.repeat(74)
 tmachines.push({agentId:LONG_ID,host:LONG_HOST,ready:true,serverEnabled:true,capabilities:caps()})
 scopes.push(scope(LONG_HOST,LONG_DIR,'s9'))
 agents.push({id:LONG_ID,host:LONG_HOST,version:'dev',paused:false,autoUpdate:false,lastSeen:now})
 const mx=project({name:LONG_NAME,defaultModel:'opus',members:[member(LONG_ID,LONG_DIR,0),member(A,'/home/dev/work/site',1),member(A,'/home/dev/work/api',2,{tool:'codex'}),member(GONE,'/srv/gone',3),member(C,'/srv/spare',4)]})
 const MX_SLUG='matrixnnnnnnnnnnnnnnnnnnnnnnnnnn-1a2b3c4d',MX_WT=`/home/builder/.reet/worktrees/${MX_SLUG}/and-an-even-longer-project-folder-name-that-must-wrap/0123456789abcdef01234567`
 const mxRun={id:'run-matrix',requestId:'matrix-request-000001',projectId:mx.id,projectName:mx.name,state:'running',startedAt:now,useWorktrees:true,worktreeProject:MX_SLUG,members:[
  {position:0,host:LONG_ID,dir:LONG_DIR,tool:'claude',model:'opus',state:'started',code:'',sentence:'',sessionId:'mx-0',worktreePath:MX_WT,worktreeRepo:'and-an-even-longer-project-folder-name-that-must-wrap',worktreeBranch:`reet/${MX_SLUG}-0123456789abcdef01234567`,worktreeRun:'0123456789abcdef01234567',worktreeProject:MX_SLUG,sharesCheckout:false,attempt:1},
  {position:1,host:A,dir:'/home/dev/work/site',tool:'claude',model:'opus',state:'started',code:'',sentence:'',sessionId:'mx-1',worktreePath:'/home/dev/.reet/worktrees/x/site/1',worktreeRepo:'site',worktreeBranch:'reet/x-1',worktreeRun:'11',worktreeProject:MX_SLUG,sharesCheckout:false,attempt:1},
  {position:2,host:A,dir:'/home/dev/work/api',tool:'codex',model:'',state:'failed',code:'git_failed',sentence:"Git couldn't make a worktree there.",sessionId:'',worktreePath:'',worktreeRepo:'',worktreeBranch:'',worktreeRun:'22',worktreeProject:MX_SLUG,sharesCheckout:false,attempt:3},
  {position:3,host:GONE,dir:'/srv/gone',tool:'claude',model:'opus',state:'removed',code:'',sentence:'',sessionId:'mx-3',worktreePath:'/home/dev/.reet/worktrees/x/gone/3',worktreeRepo:'gone',worktreeBranch:'reet/x-3',worktreeRun:'33',worktreeProject:MX_SLUG,sharesCheckout:false,attempt:1},
  {position:4,host:C,dir:'/srv/spare',tool:'claude',model:'opus',state:'pending',code:'',sentence:'',sessionId:'',worktreePath:'',worktreeRepo:'',worktreeBranch:'',worktreeRun:'44',worktreeProject:MX_SLUG,sharesCheckout:false,attempt:1}]}
 sessions.push(session('mx-0',LONG_ID,'ended'),session('mx-1',A,'running'))
 const writesBefore=storageWrites.length
 const newProject=()=>page.getByRole('button',{name:'New project',exact:true})
 const change=()=>page.getByRole('button',{name:'Change',exact:true})
 const del=()=>page.getByRole('button',{name:'Delete',exact:true})

 // The list: empty, then with a long name.
 const saved=projects;projects=[]
 await page.goto(`${base}/projects`)
 await expect(page.getByRole('heading',{name:'No Projects yet.'})).toBeVisible()
 await sweep('list, empty',[newProject()])
 projects=saved;projects.push(mx);runs.unshift(mxRun)
 await page.goto(`${base}/projects`)
 const mxLink=page.getByRole('link',{name:LONG_NAME,exact:true})
 await expect(mxLink).toBeVisible()
 await sweep('list, loaded, with an 80-character unbroken name',[newProject(),mxLink])

 // New project: as it opens, then two repositories on the long machine, a typed relative path and the message Save gives.
 await newProject().click()
 let d=page.getByRole('dialog',{name:'New project'})
 await expect(d.getByRole('textbox',{name:'Name'})).toBeFocused()
 await sweep('new project, empty',[d.getByRole('button',{name:'Save Project'}),d.getByRole('button',{name:'Close'}),d.getByRole('button',{name:'Add a repository'})])
 await d.getByRole('textbox',{name:'Name'}).fill('Matrix two')
 await d.getByRole('button',{name:'Add a repository'}).click()
 await d.getByRole('combobox',{name:'Machine',exact:true}).nth(0).selectOption(LONG_ID)
 await d.getByRole('combobox',{name:'Folder',exact:true}).nth(0).selectOption(LONG_DIR)
 await d.getByRole('button',{name:'Add a repository'}).click()
 await d.getByRole('combobox',{name:'Folder',exact:true}).nth(1).selectOption({label:'Type a path…'})
 const typed=d.getByRole('textbox',{name:'Folder path'})
 await typed.fill('work/'+'a-long-relative-folder-name-without-spaces'.repeat(3))
 await d.getByRole('button',{name:'Save Project'}).click()
 await expect(d.getByRole('status')).not.toHaveText('')
 await sweep('new project, long machine and folder, a typed path refused',[d.getByRole('button',{name:'Save Project'}),d.getByRole('status'),typed])
 await page.keyboard.press('Escape')
 await expect(d).toBeHidden()
 expect(sent('POST',/^\/api\/v2\/projects$/)).toHaveLength(1)

 // The Project page with a run whose repositories are started, failed, removed and pending, and one long one.
 await page.goto(`${base}/projects/${mx.id}`)
 await expect(page.getByRole('heading',{level:1})).toHaveText(LONG_NAME)
 const mxRemove=page.getByRole('button',{name:`Remove worktree for ${LONG_HOST} ${LONG_DIR}`})
 await expect(mxRemove).toBeVisible()
 for(const state of ['started','failed','removed','pending'])await expect(page.locator(`li[data-state="${state}"]`).first()).toBeVisible()
 const mxRetry=page.getByRole('button',{name:'Retry dev-laptop /home/dev/work/api'}),mxOpen=page.getByRole('link',{name:`Open Claude Code on ${LONG_HOST} in ${LONG_DIR}`})
 await sweep('Project page, a run with started, failed, removed and pending repositories',[startAll(),change(),del(),mxRetry,mxRemove,mxOpen])

 // Change: as it opens with five repositories (one on a machine that left), then 409 project_changed with Reload.
 await change().click()
 d=page.getByRole('dialog',{name:`Change ${LONG_NAME}`})
 await expect(d.getByRole('textbox',{name:'Name'})).toHaveValue(LONG_NAME)
 await sweep('change, five repositories with long names',[d.getByRole('button',{name:'Save changes'}),d.getByRole('button',{name:'Close'}),d.getByRole('button',{name:'Add a repository'})])
 mx.version++
 await d.getByRole('button',{name:'Save changes'}).click()
 await expect(d.getByRole('status')).toHaveText('Someone changed this Project. Reload it and try again.')
 await sweep('change, project_changed',[d.getByRole('button',{name:'Reload the Project'}),d.getByRole('button',{name:'Save changes'})])
 await d.getByRole('button',{name:'Reload the Project'}).click()
 await expect(d).toBeHidden()

 // Start all: every reason shown with Add a machine, then a lost reply, then a definite refusal.
 await startAll().click()
 await expect(sheet()).toBeVisible()
 await expect(sheet().getByRole('button',{name:'Add a machine'})).toBeVisible()
 await sweep('Start all, with reasons',[sheet().getByRole('button',{name:'Start',exact:true}),sheet().getByRole('button',{name:'Close'}),sheet().getByRole('button',{name:'Add a machine'})])
 once.push({match:(m,p)=>m==='POST'&&/\/runs$/.test(p),answer:route=>route.abort('failed')})
 await sheet().getByRole('button',{name:'Start',exact:true}).click()
 await expect(sheet().getByRole('status')).toHaveText("We didn't hear back, so the run may have started. Try the same request again: it can't start a second run.")
 await sweep('Start all, lost reply',[sheet().getByRole('button',{name:'Try the same request again'}),sheet().getByRole('status')])
 await page.keyboard.press('Escape')
 await expect(sheet()).toHaveCount(0)
 await startAll().click()
 once.push({match:(m,p)=>m==='POST'&&/\/runs$/.test(p),answer:route=>route.fulfill({status:409,json:{error:'This Project is still starting. Wait for it to finish, then start again.',code:'run_in_progress'}})})
 await sheet().getByRole('button',{name:'Start',exact:true}).click()
 await expect(sheet().getByRole('status')).toHaveText('This Project is still starting. Wait for it to finish, then start again.')
 await sweep('Start all, refused (run in progress)',[sheet().getByRole('button',{name:'Start',exact:true}),sheet().getByRole('status')])
 await page.keyboard.press('Escape')
 await expect(sheet()).toHaveCount(0)
 expect(runs.filter(r=>r.projectId===mx.id)).toEqual([mxRun])

 // Remove worktree: blocked by changes with long paths, the confirmation, and git refusing the check with its detail.
 const mxWt={path:MX_WT,branch:mxRun.members[0].worktreeBranch,changed:0,unpushed:2,paths:[],unreferenced:0,hidden:0,nested:false,incomplete:false}
 statusAnswers.push({...mxWt,changed:23,unreferenced:1,hidden:2,nested:true,incomplete:true,paths:Array.from({length:20},(_,i)=>`src/${LONG}/file${i}.ts`)})
 await mxRemove.click()
 d=page.getByRole('dialog',{name:'Remove worktree'})
 await expect(d.getByText('and 3 more not listed.')).toBeVisible()
 await sweep('remove worktree, blocked by every check',[d.getByRole('button',{name:'Check again'}),d.getByRole('button',{name:'Close'})])
 // The folder, counts and paths scroll on a small screen; the part that scrolls is reached by keyboard (an axe scan found
 // it was not: scrollable-region-focusable).
 await setCombo(COMBOS[3])
 await d.getByRole('button',{name:'Close'}).focus()
 await page.keyboard.press('Tab')
 const details=d.getByRole('region',{name:'Worktree details'})
 await expect(details).toBeFocused()
 // Its focus ring is drawn inside it: outside, the dialog would clip the sides and the footer cover the bottom.
 expect(await details.evaluate(el=>{const cs=getComputedStyle(el);return{style:cs.outlineStyle,width:parseFloat(cs.outlineWidth),offset:parseFloat(cs.outlineOffset)}})).toEqual({style:'solid',width:2,offset:-2})
 await setCombo(COMBOS[0])
 statusAnswers.push({...mxWt})
 await d.getByRole('button',{name:'Check again'}).click()
 await expect(d.locator('.confirm')).toBeVisible()
 // When everything fits, the middle part is no tab stop: Tab from Close goes on to the footer.
 await expect(d.locator('.body')).not.toHaveAttribute('tabindex')
 await expect(d.getByRole('region')).toHaveCount(0)
 await sweep('remove worktree, the confirmation',[d.getByRole('button',{name:'Remove with 2 unpushed commits'}),d.getByRole('button',{name:'Check again'})])
 statusAnswers.push({__status:422,error:"Git couldn't check that worktree.",code:'git_failed',detail:'fatal: '+LONG})
 await d.getByRole('button',{name:'Check again'}).click()
 await expect(d.getByText('Git said:')).toBeVisible()
 await sweep('remove worktree, git refused the check',[d.getByRole('button',{name:'Check again'}),d.getByRole('status')])
 await page.keyboard.press('Escape')
 await expect(d).toBeHidden()
 await expect(mxRemove).toBeFocused()

 // Delete: as it opens, then 409 project_changed with Reload.
 await del().click()
 d=page.getByRole('dialog',{name:`Delete ${LONG_NAME}?`})
 await sweep('delete',[d.getByRole('button',{name:'Delete Project'}),d.getByRole('button',{name:'Keep it'})])
 mx.version++
 await d.getByRole('button',{name:'Delete Project'}).click()
 await expect(d.getByRole('status')).toHaveText('Someone changed this Project. Reload it and try again.')
 await sweep('delete, project_changed',[d.getByRole('button',{name:'Reload the Project'}),d.getByRole('button',{name:'Delete Project'})])
 await d.getByRole('button',{name:'Reload the Project'}).click()
 await expect(d).toBeHidden()

 // Escape closes each dialog without a write and focus goes back to what opened it, at every size and in both schemes.
 // Each opener is pressed by a script click with nothing focused (as a click on Safari, or on Firefox on a Mac, focuses no
 // button), so the focus that comes back is the dialog's doing, not the browser's own return of focus on close.
 const scriptClick=locator=>locator.evaluate(el=>{document.activeElement?.blur();el.click()})
 const writes=()=>requests.filter(r=>r.method!=='GET'&&!r.path.endsWith('/worktrees/status')).length
 await page.goto(`${base}/projects`)
 for(const c of COMBOS){
  await setCombo(c)
  const before=writes()
  await scriptClick(newProject())
  d=page.getByRole('dialog',{name:'New project'})
  await d.getByRole('textbox',{name:'Name'}).fill('Not saved')
  await page.keyboard.press('Escape')
  await expect(d).toHaveCount(0)
  await expect(newProject(),c.label).toBeFocused()
  expect(writes(),`Escape at ${c.label} sent a write`).toBe(before)
 }
 await setCombo(COMBOS[0])
 await page.goto(`${base}/projects/${mx.id}`)
 await expect(mxRemove).toBeVisible()
 const escapes=[
  {opener:change,dialog:()=>page.getByRole('dialog',{name:`Change ${LONG_NAME}`})},
  {opener:startAll,dialog:sheet},
  {opener:del,dialog:()=>page.getByRole('dialog',{name:`Delete ${LONG_NAME}?`})},
  {opener:()=>mxRemove,dialog:()=>page.getByRole('dialog',{name:'Remove worktree'}),ready:()=>expect(page.locator('dialog .confirm')).toBeVisible(),before:()=>statusAnswers.push({...mxWt})},
 ]
 for(const c of COMBOS){
  await setCombo(c)
  for(const e of escapes){
   const before=writes()
   e.before?.()
   await scriptClick(e.opener())
   await expect(e.dialog()).toBeVisible()
   await e.ready?.()
   await page.keyboard.press('Escape')
   await expect(e.dialog()).toHaveCount(0)
   await expect(e.opener(),`${c.label}`).toBeFocused()
   expect(writes(),`Escape at ${c.label} sent a write`).toBe(before)
  }
 }
 await setCombo(COMBOS[0])
 expect(statusAnswers).toHaveLength(0)

 // Reduced motion: with the preference, nothing on the page or in any open dialog has an animation or a transform
 // transition, and none is running; without it the buttons' press transition is there, so the check is not empty.
 const moving=()=>page.evaluate(()=>{
  const roots=[...document.querySelectorAll('dialog[open]')];if(!roots.length)roots.push(document.body)
  const all=roots.flatMap(r=>[r,...r.querySelectorAll('*')])
  return{moving:all.filter(n=>{const cs=getComputedStyle(n),names=cs.transitionProperty.split(',').map(t=>t.trim()),times=cs.transitionDuration.split(',').map(t=>parseFloat(t)*(t.trim().endsWith('ms')?1:1000))
   return cs.animationName!=='none'||names.some((name,i)=>(name==='all'||name==='transform')&&times[i%times.length]>0)}).map(n=>n.tagName+'.'+n.className),
   running:document.getAnimations().filter(a=>a.playState==='running').length}
 })
 expect((await moving()).moving.length).toBeGreaterThan(0)
 await page.emulateMedia({reducedMotion:'reduce'})
 await page.reload()
 await expect(mxRemove).toBeVisible()
 expect(await moving(),'the Project page moves').toEqual({moving:[],running:0})
 for(const e of escapes){
  e.before?.()
  await e.opener().click()
  await expect(e.dialog()).toBeVisible()
  await e.ready?.()
  expect(await moving(),'a dialog moves').toEqual({moving:[],running:0})
  await page.keyboard.press('Escape')
  await expect(e.dialog()).toHaveCount(0)
 }
 await page.goto(`${base}/projects`)
 await newProject().click()
 expect(await moving(),'New project moves').toEqual({moving:[],running:0})
 await page.keyboard.press('Escape')
 await page.emulateMedia({reducedMotion:'no-preference'})

 // Pages that say something went wrong: the Project can't be read, isn't on the account, has been deleted; a viewer.
 once.push({match:(m,p)=>m==='GET'&&p===`/api/v2/projects/${mx.id}`,answer:route=>route.fulfill({status:503,json:{error:"reet couldn't do that just now. Try again.",code:'server_error'}})})
 await page.goto(`${base}/projects/${mx.id}`)
 await expect(page.getByRole('alert')).toBeVisible()
 await sweep('Project page, could not be read',[page.getByRole('alert')])
 await page.goto(`${base}/projects/proj-not-here`)
 await expect(page.getByRole('heading',{name:'Project not found.'})).toBeVisible()
 await sweep('Project page, not on this account',[page.getByRole('link',{name:'Back to Projects'})])
 viewer=true
 await page.goto(`${base}/projects`)
 await expect(page.getByText("This account is read-only, so it can't use Projects.",{exact:true})).toBeVisible()
 await sweep('read-only account',[page.getByText("This account is read-only, so it can't use Projects.",{exact:true})])
 viewer=false
 await page.goto(`${base}/projects/${mx.id}`)
 await del().click()
 await page.getByRole('dialog',{name:`Delete ${LONG_NAME}?`}).getByRole('button',{name:'Delete Project'}).click()
 await expect(page.getByRole('heading',{name:'Project deleted.'})).toBeVisible()
 await sweep('Project deleted',[page.getByRole('link',{name:'Back to Projects'})])

 expect(surfaces['dialog light']).toBeTruthy();expect(surfaces['dialog dark']).toBeTruthy();expect(surfaces['dialog dark']).not.toBe(surfaces['dialog light'])   // dark is really applied
 expect(surfaces['page light']).toBeTruthy();expect(surfaces['page dark']).toBeTruthy();expect(surfaces['page dark']).not.toBe(surfaces['page light'])
 expect(swept).toHaveLength(19)
 // The matrix wrote nothing to browser storage under a key not written before it, and nothing about a Project.
 const keyOf=w=>w.split(' ',2).join(' ')
 const knownKeys=new Set(storageWrites.slice(0,writesBefore).map(keyOf))
 const matrixWrites=storageWrites.slice(writesBefore)
 expect(matrixWrites.filter(w=>!knownKeys.has(keyOf(w)))).toEqual([])
 for(const secret of [LONG_NAME,LONG_DIR,'Matrix two',mx.id])expect(matrixWrites.join('\n').includes(secret)).toBe(false)

 // ── A viewer, an instance without accounts and a token sign-in see no Projects, and the page asks nothing about them. ──
 const nav=()=>page.getByRole('navigation',{name:'Main navigation'})
 async function seesNothing(notice){
  const before=requests.length
  for(const where of ['/projects',`/projects/${plain.id}`]){
   await page.goto(base+where)
   await expect(page.getByText(notice,{exact:true})).toBeVisible()
   await expect(page.getByRole('button',{name:/^(New project|Start all|Change|Delete)$/})).toHaveCount(0)
  }
  await page.goto(`${base}/machines`)
  await expect(nav().getByRole('link',{name:'Projects'})).toHaveCount(0)
  await settleUI()
  expect(projectCalls(requests.slice(before))).toEqual([])
 }
 viewer=true
 await seesNothing("This account is read-only, so it can't use Projects.")
 viewer=false;authMode='none'
 await seesNothing('Projects need an account signed in through the browser.')
 authMode='token'
 await page.evaluate(()=>localStorage.setItem('reet-token','fixture-token'))
 await seesNothing('Projects need an account signed in through the browser.')
 authMode='accounts'

 expect(errors).toEqual([])
 expect(unmocked.filter(u=>/project|worktree|terminal/.test(u))).toEqual([])
 console.log(`Projects UI passed: create checked before sending with one save per press while Escape and Close wait, the shared prompt kept across two changes (never sent unchanged), If-Match on every change and delete with 409 project_changed and Reload for both, Start all listing each reason a repository won't start (not enrolled with Add a machine, not ready, no launch, agent not installed, no worktrees, 8 per machine and 32 per account counting what it plans), a start from a sheet made before another tab saved refused (If-Match) with nothing started and the Project read again, a lost start resent with the same request id, positions and version and a new id for a new run, sessions reads that never answer given up after 15 seconds, machine reads that never answer holding neither the list nor the Project page (machines named by id until read), a run through pending, started, failed, Retry (attempt 2) and removed with polling stopping at rest, Remove worktree never offered while a session runs or the sessions are unknown, the status read first, blocked by changes and commits on no branch, Check again, the confirmation stating the counts the remove repeats and a session that runs again stopping it, delete keeping runs, the no-worktree warning, 401 handed to sign-in, the Activity lines, nothing in storage or addresses, no Projects for viewers, open or token sign-in and no Projects calls from them, no sideways scroll at 320px in light and dark, and a matrix of 19 states (the list empty and loaded, New project empty and refused, the Project page with started, failed, removed and pending repositories, Change and its project_changed, Start all with reasons, a lost reply and a refusal, Remove worktree blocked, confirming and refused by git, Delete and its project_changed, a Project that can't be read, isn't on the account or was deleted, and a read-only account) at 1360, 768, 390 and 320px in light and dark with no sideways scroll, nothing past a dialog's edges and each primary action reachable and uncovered, the worktree details reachable by keyboard while they scroll with the focus ring drawn inside, Escape from every dialog at every size returning focus without a write, a reduced-motion run and no new storage key. Unmocked: ${[...new Set(unmocked)].join(', ')||'none'}.`)
}finally{await browser.close()}
