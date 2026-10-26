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
let authMode='accounts',viewer=false,signedOut=false,seq=0,eventSeq=0
// How the sessions read answers: 'ok', 'fail', or a held promise.
let sessionsMode='ok'
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
 if(path==='/api/v2/profile')return json({user:{id:1,name:'Operator',email:'operator@example.test',role},profile:{theme:'light',shelfView:'list'},sessionCount:1,accounts:authMode==='accounts'})
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
 if(path==='/api/v2/terminal/machines')return json({machines:tmachines})
 if(path==='/api/v2/terminal/sessions'&&method==='GET'){
  const mode=sessionsMode
  if(mode&&mode.promise)await mode.promise
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
  // One run per request id: the same id answers with the run it already started.
  const b=JSON.parse(body),existing=runs.find(r=>r.requestId===b.requestId)
  return existing?json(existing,200):json(createRun(m[1],b),201)
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
 if(path==='/api/v2/terminal/worktrees/status'){const a=statusAnswers.shift();if(!a){unmocked.push('an unexpected worktree status read');return json({error:'unexpected'},500)}return json(a)}
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
 await expect(page.getByRole('navigation',{name:'Main navigation'}).getByRole('link',{name:'Projects'}).first()).toHaveAttribute('aria-current','page')

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
 const starts=sent('POST',/^\/api\/v2\/projects\/proj1\/runs$/).map(r=>JSON.parse(r.body))
 expect(starts).toHaveLength(2)
 expect(starts[0].requestId).toMatch(UUID)
 expect(starts[1]).toEqual(starts[0])
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
 await expect(page.getByText("Couldn't read your sessions just now, so Start all can't check the session limits. Try again in a moment.")).toBeVisible()
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
 expect(sent('DELETE',/proj1$/).map(r=>r.headers['if-match'])).toEqual(['"7"'])
 expect(projects).toContain(site)
 await dd.getByRole('button',{name:'Reload the Project'}).click()
 await expect(dd).toBeHidden()
 await expect(page.getByText('Read the Project again.',{exact:false})).toBeVisible()
 await page.getByRole('button',{name:'Delete',exact:true}).click()
 await dd.getByRole('button',{name:'Delete Project'}).click()
 await expect(page.getByRole('heading',{name:'Project deleted.'})).toBeVisible()
 await expect(page.getByText('Project deleted. Its runs are kept. Worktrees those runs made, if still there, stay on their machines.')).toBeVisible()
 expect(sent('DELETE',/proj1$/).map(r=>r.headers['if-match'])).toEqual(['"7"','"9"'])
 expect(projects).not.toContain(site)

 // ── A Project without worktrees warns that sessions share the checkout; a new run gets a new request id; 401 hands over to sign-in. ──
 const plain=project({name:'Plain',useWorktrees:false,members:[member(A,'/home/dev/work/site',0)]})
 projects.push(plain)
 await page.goto(`${base}/projects/${plain.id}`)
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
 console.log(`Projects UI passed: create checked before sending with one save per press while Escape and Close wait, the shared prompt kept across two changes (never sent unchanged), If-Match on every change and delete with 409 project_changed and Reload for both, Start all listing each reason a repository won't start (not enrolled with Add a machine, not ready, no launch, agent not installed, no worktrees, 8 per machine and 32 per account counting what it plans), a lost start resent with the same request id and positions and a new id for a new run, a run through pending, started, failed, Retry (attempt 2) and removed with polling stopping at rest, Remove worktree never offered while a session runs or the sessions are unknown, the status read first, blocked by changes and commits on no branch, Check again, the confirmation stating the counts the remove repeats and a session that runs again stopping it, delete keeping runs, the no-worktree warning, 401 handed to sign-in, the Activity lines, nothing in storage or addresses, no Projects for viewers, open or token sign-in and no Projects calls from them, and no sideways scroll at 320px in light and dark. Unmocked: ${[...new Set(unmocked)].join(', ')||'none'}.`)
}finally{await browser.close()}
