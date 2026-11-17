import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// Home's one list of what is waiting: Factory pipelines and sessions together, oldest first, each with its action. On a
// wide screen the Factory rows and the session rows line up (chip, stages and buttons each in a fixed column); on a tablet
// and a phone each is a card with no sideways scroll, and the Projects button narrows the list.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-home-waiting'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const MIN=60000,now=Date.now()
const sid='factory-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32),oid='e'.repeat(32)
// The approval pipeline's session holds a permission: it is a session card too, and must show once, as the pipeline.
const psid='factory-session-permit',pcid='f'.repeat(32)
const sessionsKey='reet.home.sections.1'
function fixture(){
 const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
 const base={agentId:'a1',tool:'claude',model:'m',structured:true,createdAt:now-3*3600000,exitCode:null,dir:'/work'}
 const sessions=[
  {...base,id:sid,title:'Export API · code',tool:'opencode',model:'working-model',state:'running',updatedAt:now,projectId:'p1'},
  {...base,id:psid,title:'Stages when collapsed · code',state:'running',updatedAt:now,projectId:'p1',attention:'waiting',attentionSource:'chat',changedSinceSeen:true,bellAt:now-30*MIN,pendingPermission:{permissionId:'perm-pipe00000001',title:'Run a command',command:'npm run check',path:'',truncated:false,queued:0}},
  // A session that asked a question, one that finished and has not been seen, and one that failed.
  {...base,id:'ask-session',title:'Portal sign-in copy',state:'running',updatedAt:now-10*MIN,projectId:'p2',attention:'waiting',attentionSource:'chat',changedSinceSeen:true,bellAt:now-10*MIN,pendingQuestion:{questionId:'q1',message:'Which wording for the expired-link page?',truncated:false,queued:0}},
  {...base,id:'done-session',title:'Rename tunlx hosts',state:'running',updatedAt:now-15*MIN,projectId:'p2',attention:'finished',attentionSource:'chat',changedSinceSeen:true},
  {...base,id:'failed-session',title:'Speed up the runner pool',state:'exited',exitCode:2,updatedAt:now-20*MIN,projectId:'p2',attention:'failed',attentionSource:'terminal',changedSinceSeen:true},
 ]
 const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},box={moved:[],posted:[],sessions}
 const goal={...pins,id:'1'.repeat(32),conversationId:cid,sessionId:sid,revision:3,text:'Outcome: x',state:'active',note:'Gaffer brought a question back.',maxReviews:12,usedReviews:3,expiresAt:now+20*3600000,createdAt:now-3600000,updatedAt:now-40*MIN,nextAt:now,lastSeq:42,summary:'Tests pass locally.',inputTokens:0,outputTokens:0,usageReported:true,advice:{choice:'move',why:'The button and its tests are committed.'}}
 const row={goal,session:{id:sid,agentId:'a1',title:sessions[0].title,state:'running'},available:true,attention:true,reason:'Gaffer brought a question back.'}
 const permitGoal={...goal,id:'2'.repeat(32),conversationId:pcid,sessionId:psid,note:'The agent is waiting for your approval.',updatedAt:now-30*MIN,waitingOnWorker:true,advice:undefined}
 const permitRow={goal:permitGoal,session:{id:psid,agentId:'a1',title:sessions[1].title,state:'running'},available:true,attention:true,reason:'The agent is waiting for your approval.'}
 const stage=(kind,state,s=sid,c=cid)=>({kind,tool:'opencode',model:'',state,requestId:'r',...(state==='running'?{sessionId:s,conversationId:c}:{})})
 const outcome={id:oid,revision:5,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
  {key:'a',title:'Export API',prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:sid,conversationId:cid,updatedAt:now,stage:0,stages:[stage('code','running'),stage('review','pending'),stage('merge','pending')]},
  {key:'b',title:'Export button',prompt:'p',state:'following',sessionState:'running',note:'The review stage is under way.',sessionId:'s-b',conversationId:'b'.repeat(32),updatedAt:now,stage:1,stages:[stage('code','done'),stage('review','running','s-b','b'.repeat(32)),stage('merge','pending')]},
  {key:'c',title:'Export docs',prompt:'p',state:'ready_to_merge',sessionState:'running',note:'Ready to merge. Nothing merges until you approve it.',sessionId:'s-c',conversationId:'d'.repeat(32),updatedAt:now-50*MIN,stage:2,stages:[stage('code','done'),stage('review','done'),stage('merge','pending')]},
  {key:'d',title:'Stages when collapsed',prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:psid,conversationId:pcid,updatedAt:now,stage:0,stages:[stage('code','running',psid,pcid),stage('review','pending'),stage('pr','pending')]}]}
 const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],messages:[]}
 const goalView=()=>({goal,pins,allowed:true,setupAllowed:true,providerName:'Workshop Gaffer',conversationRevision:1,requiresProviderAccess:false})
 const project=(id,name,dir,position)=>({id,host:'a1',machine:'Workshop',dir,name,isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position,version:1,createdAt:now,updatedAt:now})
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>view,
  '/api/v2/gaffer/outcomes':()=>({outcomes:[outcome]}),
  '/api/v2/gaffer/goals':()=>({goals:[row,permitRow],readAt:Date.now()}),
  [`/api/v2/gaffer/conversations/${cid}/goal`]:()=>goalView(),
  // Moving Export API on: its code stage is done, review is in hand, and Gaffer is no longer waiting on the person.
  [`/api/v2/gaffer/outcomes/${oid}/sessions/a/advance`]:request=>{box.moved.push(request.postDataJSON());const piece=outcome.sessions[0];piece.stages[0].state='done';piece.stage=1;piece.stages[1]={...piece.stages[1],state:'running',sessionId:sid,conversationId:cid};piece.note='The review stage is under way.';row.attention=false;delete row.reason;delete goal.advice;outcome.revision++;return outcome},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':()=>({sessions:box.sessions}),
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true,inspect:true}}]},
  '/api/v2/projects':{projects:[]},
  '/api/v2/folder-projects':{projects:[project('p1','billing','/work/billing',0),project('p2','portal','/work/portal',1)],savedWaiting:0},
  '/api/v2/settings/yolo':{allowed:false},
 }
 return{routes,outcome,box,row}
}
// Sessions that need the person in other ways, so the rightmost button of each kind of row is measured.
function withMore(f){
 const base={agentId:'a1',tool:'claude',model:'m',structured:true,createdAt:now,exitCode:null,dir:'/work'}
 f.box.sessions.push(
  {...base,id:'approve-session',title:'Release notes',state:'running',updatedAt:now,attention:'waiting',attentionSource:'chat',changedSinceSeen:true,bellAt:now-90000,pendingPermission:{permissionId:'p1',title:'Run a command',command:'npm run build',path:'',truncated:false,queued:0}},
  {...base,id:'review-session',title:'Billing tidy-up',state:'exited',exitCode:0,updatedAt:now-120000,attention:'finished',attentionSource:'terminal',changedSinceSeen:true,worktreePath:'/work/billing-wt',worktreeRepo:'/work/billing',worktreeBranch:'tidy'},
 )
}
// Nothing waits anywhere: no outcomes and no session that needs the person.
function quiet(f){
 f.routes['/api/v2/gaffer/outcomes']=()=>({outcomes:[]})
 f.routes['/api/v2/gaffer/goals']=()=>({goals:[],readAt:Date.now()})
 f.box.sessions=f.box.sessions.filter(s=>s.id===sid).map(s=>({...s,projectId:'p1'}))
}
async function open(width,theme='light',change=()=>{},height){
 const f=fixture();change(f)
 const w=await openWorkspace(browser,{width,height:height||(width<500?844:width<1000?1112:900),theme,touch:width<1000,routes:f.routes})
 await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
 await w.page.goto(`${base}/`)
 return {...w,f}
}
const section=(page,name)=>page.getByRole('region',{name,exact:true})
// A section's header is its one button, named by the section's name first. (Your machines is no region of its own: the
// strip inside it is the region.)
const head=(page,name)=>page.getByRole('main').getByRole('button',{name:new RegExp(`^${name} `)}).first()
const entries=page=>section(page,'Waiting for you').locator('.waiting-list > li')
const right=async locator=>{const box=await locator.boundingBox();return box.x+box.width}
const noSideways=page=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)
try{
 // The list: five entries, oldest first, its header saying what kinds wait; the finished session is not one of them.
 {
  const w=await open(1440),page=w.page
  try{
   const waiting=section(page,'Waiting for you')
   await expect(entries(page)).toHaveCount(5)
   await expect(entries(page).locator('h3')).toHaveText(['Export docs','Export API','Stages when collapsed','Speed up the runner pool','Portal sign-in copy'])
   await expect(head(page,'Waiting for you')).toHaveAttribute('aria-expanded','true')
   await expect(head(page,'Waiting for you')).toContainText('· 5')
   await expect(head(page,'Waiting for you')).toContainText('1 decision, 1 approval, 1 merge, 1 question, 1 failed · oldest 50 min')
   // A plainly finished session is not waiting; Continue working says it has not been seen.
   await expect(waiting).not.toContainText('Rename tunlx hosts')
   // The permission pipeline is listed once, as the pipeline, with Allow and Deny.
   await expect(waiting.getByText('Stages when collapsed',{exact:true})).toHaveCount(1)
   await expect(waiting.getByText('Stages when collapsed · code',{exact:true})).toHaveCount(0)
   const permit=entries(page).filter({hasText:'Stages when collapsed'})
   await expect(permit.getByRole('button',{name:'Allow once'})).toBeVisible()
   await expect(permit.getByRole('button',{name:'Deny'})).toBeVisible()
   await expect(permit).toContainText('npm run check')
   // Every Factory item names its stages: the dots carry the caption.
   for(const title of ['Export docs','Export API','Stages when collapsed'])await expect(entries(page).filter({hasText:title}).locator('.caption')).toBeVisible()
   await expect(entries(page).filter({hasText:'Export API'}).locator('.caption')).toHaveText('Code · 1 of 3 · next Review')
   // The bell and the title count the section.
   await expect.poll(()=>page.title()).toBe('(5) Home · reet.sh')
   await expect(page.getByRole('button',{name:'Needs you: 5',exact:true})).toBeVisible()
   // The head says it in one line.
   await expect(page.locator('.workspace-heading')).toContainText('5 waiting for you · 1 pipeline working · 4 sessions open')
   // Details goes to the pipeline on the Factory page.
   // Details is a real link to the pipeline on the Factory page.
   const details=entries(page).filter({hasText:'Export docs'}).getByRole('link',{name:'Details and other actions for Export docs'})
   await expect(details).toHaveAttribute('href',`/factory?pipeline=${oid}.c`)
   // Factory, working without you: closed the first time, naming the working pipeline; opened, it stays open after a reload.
   const working=head(page,'Factory, working without you')
   await expect(working).toHaveAttribute('aria-expanded','false')
   await expect(working).toContainText('Export button')
   await working.click()
   await expect(working).toHaveAttribute('aria-expanded','true')
   const line=section(page,'Factory, working without you').getByRole('link',{name:/Export button/})
   await expect(line).toHaveAttribute('href',`/factory?pipeline=${oid}.b`)
   await expect(section(page,'Factory, working without you').getByRole('link',{name:'Open the Factory',exact:true})).toHaveAttribute('href','/factory')
   await page.reload()
   await expect(head(page,'Factory, working without you')).toHaveAttribute('aria-expanded','true')
   expect(JSON.parse(await page.evaluate(k=>localStorage.getItem(k),sessionsKey))).toMatchObject({waiting:true,working:true})
   // Your machines is closed the first time.
   await expect(head(page,'Your machines')).toHaveAttribute('aria-expanded','false')
   // Both views still switch, and the choice survives a reload; Continue working marks the finished session not seen yet.
   const views=page.getByRole('group',{name:'Home view',exact:true})
   await views.getByRole('button',{name:'Continue working',exact:true}).click()
   const resume=page.getByRole('region',{name:'Continue working',exact:true})
   await expect(resume.locator('li',{hasText:'Rename tunlx hosts'})).toContainText('not seen yet')
   await page.screenshot({path:`${shots}/home-waiting-1440-continue.png`,fullPage:true})
   await page.reload()
   await expect(views.getByRole('button',{name:'Continue working',exact:true})).toHaveAttribute('aria-pressed','true')
   await views.getByRole('button',{name:'Planning board',exact:true}).click()
   await expect(page.locator('.session-card',{hasText:'Rename tunlx hosts'}).locator('.unseen')).toHaveText('Not seen yet')
   await page.reload()
   await expect(views.getByRole('button',{name:'Planning board',exact:true})).toHaveAttribute('aria-pressed','true')
   await details.click()
   await expect(page).toHaveURL(new RegExp(`/factory\\?pipeline=${oid}\\.c$`))
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 // Acting from Home: the decision's filled button opens the Factory page's confirmation, and confirming moves it on once.
 {
  const w=await open(1440),page=w.page
  try{
   const decision=entries(page).filter({hasText:'Export API'})
   await decision.getByRole('button',{name:/^Move to next stage/}).click()
   const confirm=page.getByRole('dialog',{name:'Move “Export API” on to review?'})
   await expect(confirm).toBeVisible()
   await confirm.getByRole('button',{name:'Move on',exact:true}).click()
   await expect.poll(()=>w.f.box.moved.length).toBe(1)
   await expect(entries(page)).toHaveCount(4)
   await expect(section(page,'Waiting for you')).not.toContainText('Export API')
   await expect.poll(()=>page.title()).toBe('(4) Home · reet.sh')
   expect(w.f.box.moved).toHaveLength(1)
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 // Nothing waiting: one line, and no Factory section.
 {
  const w=await open(1440,'light',quiet),page=w.page
  try{
   await expect(section(page,'Waiting for you')).toContainText('Nothing is waiting for you.')
   await expect(head(page,'Waiting for you')).toHaveAttribute('aria-expanded','true')
   await expect(section(page,'Factory, working without you')).toHaveCount(0)
   await expect(page.locator('.workspace-heading')).toContainText('0 waiting for you · 1 session open')
   await expect.poll(()=>page.title()).toBe('Home · reet.sh')
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 // On a wide screen the rows line up: buttons share one right edge across Factory and session rows of every kind.
 for(const theme of ['light','dark']){
  const w=await open(1440,theme,withMore),page=w.page
  try{
   const details=page.getByRole('link',{name:/^Details and other actions for /})
   const answer=page.getByRole('link',{name:'Answer in chat',exact:true})
   await expect(answer).toBeVisible();await expect(details.first()).toBeVisible()
   const edges=[await right(details.first()),await right(answer)]
   for(const name of ['Release notes','Billing tidy-up']){
    const article=entries(page).filter({hasText:name}).locator('article')
    await expect(article).toBeVisible()
    edges.push(await right(article.locator('.buttons .btn').last()))
   }
   for(const edge of edges)expect(Math.abs(edge-edges[0])).toBeLessThanOrEqual(1)
   // The bell lists what it counts: every entry, the pipelines with a link to their detail.
   await page.getByRole('button',{name:'Needs you: 7',exact:true}).click()
   const pop=page.getByRole('banner').getByRole('region',{name:'Needs you'})
   await expect(pop.locator('.item')).toHaveCount(7)
   await expect(pop.getByRole('link',{name:'Open in the Factory',exact:true})).toHaveCount(3)
   await expect(pop.locator('.item',{hasText:'Export docs'}).getByRole('link',{name:'Open in the Factory',exact:true})).toHaveAttribute('href',`/factory?pipeline=${oid}.c`)
   await expect(pop.locator('.item',{hasText:'Stages when collapsed'}).getByRole('button',{name:'Allow once'})).toBeVisible()
   await page.screenshot({path:`${shots}/home-bell-1440-${theme}.png`})
   await page.keyboard.press('Escape')
   // Every row is as wide as the list; the chips and the stages each start at one left edge.
   const rows=await page.evaluate(()=>[...document.querySelectorAll('.waiting-list > li article')].map(a=>a.getBoundingClientRect().right))
   expect(rows.length).toBe(7)
   expect(Math.max(...rows)-Math.min(...rows)).toBeLessThanOrEqual(1)
   const chips=await page.evaluate(()=>[...document.querySelectorAll('.waiting-list > li .chipcell')].map(c=>Math.round(c.getBoundingClientRect().left)))
   expect(new Set(chips).size).toBe(1)
   const stages=await page.evaluate(()=>[...document.querySelectorAll('.waiting-list > li .stagecell')].map(c=>Math.round(c.getBoundingClientRect().left)))
   expect(stages.length).toBe(3);expect(new Set(stages).size).toBe(1)
   expect(await noSideways(page)).toBe(true)
   await page.screenshot({path:`${shots}/home-waiting-1440-${theme}.png`,fullPage:true})
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 // Tablet and phone: each item is a card, the Projects button narrows the list, and nothing scrolls sideways.
 for(const width of [834,390,320]){
  const w=await open(width,'light'),page=w.page
  try{
   const answer=page.getByRole('link',{name:'Answer in chat',exact:true})
   await expect(answer).toBeVisible()
   await expect(entries(page)).toHaveCount(5)
   const borders=await page.evaluate(()=>[...document.querySelectorAll('.waiting-list > li article')].map(a=>[a.classList.contains('card'),getComputedStyle(a).borderTopWidth]))
   expect(borders.length).toBe(5);for(const [card,border] of borders){expect(card).toBe(true);expect(border).toBe('1px')}
   for(const title of ['Export docs','Export API','Stages when collapsed'])await expect(entries(page).filter({hasText:title}).locator('.stages')).toBeVisible()
   expect(await noSideways(page)).toBe(true)
   expect(await answer.evaluate(el=>el.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
   await page.screenshot({path:`${shots}/home-waiting-${width}.png`,fullPage:true})
   if(width===390){
    await page.getByRole('button',{name:'Needs you: 5',exact:true}).click()
    const pop=page.getByRole('banner').getByRole('region',{name:'Needs you'})
    await expect(pop.locator('.item')).toHaveCount(5)
    await expect(pop.getByRole('link',{name:'Open in the Factory',exact:true})).toHaveCount(3)
    expect(await noSideways(page)).toBe(true)
    await page.screenshot({path:`${shots}/home-bell-390.png`})
    await page.keyboard.press('Escape')
   }
   // Closed, each header still says what is inside, on its own line on a phone.
   await head(page,'Waiting for you').click()
   await expect(head(page,'Waiting for you')).toHaveAttribute('aria-expanded','false')
   await expect(head(page,'Waiting for you')).toContainText('1 decision, 1 approval, 1 merge, 1 question, 1 failed · oldest 50 min')
   expect(await noSideways(page)).toBe(true)
   await page.screenshot({path:`${shots}/home-waiting-${width}-closed.png`})
   await head(page,'Waiting for you').click()
   const picker=page.getByRole('button',{name:'Projects: all',exact:true})
   expect((await picker.boundingBox()).height).toBeGreaterThanOrEqual(44)
   await picker.click()
   await page.getByRole('dialog',{name:'Projects'}).getByRole('button',{name:/^portal/}).click()
   await expect(page.getByRole('button',{name:'Projects: portal',exact:true})).toBeVisible()
   await expect(entries(page)).toHaveCount(2)
   await expect(entries(page).locator('h3')).toHaveText(['Speed up the runner pool','Portal sign-in copy'])
   await expect(section(page,'Factory, working without you')).toHaveCount(0)
   expect(await noSideways(page)).toBe(true)
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 // The floating Gaffer button covers no button or link of any waiting item (Open, Details, Allow once, Deny, Mark seen,
 // Answer in chat, Approve the merge), at the top of the page and with the end of the list scrolled into view.
 // At 1180 the rows are wide and the gaps between them are narrower than the button, so a second pass adds eight more
 // failed sessions (ten rows, each with its buttons at the right) and the button is checked at every scroll position.
 const crowded=f=>{withMore(f);for(let i=0;i<8;i++)f.box.sessions.push({agentId:'a1',tool:'claude',model:'m',structured:true,createdAt:now-3*3600000,exitCode:2,dir:'/work',id:`crowd-${i}`,title:`Lost run ${i}`,state:'exited',updatedAt:now-(30+i)*MIN,projectId:'p2',attention:'failed',attentionSource:'terminal',changedSinceSeen:true})}
 for(const [width,height,many] of [[1440],[1180,800],[1180,800,true],[1250,800,true],[834],[390]]){
  const w=await open(width,'light',many?crowded:withMore,height),page=w.page
  try{
   await expect(entries(page)).toHaveCount(many?10:7)
   const settled=()=>expect.poll(()=>page.evaluate(()=>new Promise(done=>{const el=document.querySelector('.gaffer-launcher');if(!el)return done(false);const a=el.getBoundingClientRect();requestAnimationFrame(()=>requestAnimationFrame(()=>{const b=el.getBoundingClientRect();done(el.style.getPropertyValue('--launcher-clearance')!==''&&a.top===b.top&&a.left===b.left)}))}))).toBe(true)
   const under=()=>page.evaluate(()=>{
    const r=document.querySelector('.gaffer-launcher').getBoundingClientRect(),hit=[]
    const controls=[...document.querySelectorAll('.waiting-list button,.waiting-list a[href]')]
    if(!controls.length)return {hit:['no controls were checked'],onScreen:false}
    for(const node of controls){
     const b=node.getBoundingClientRect()
     if(b.width&&b.height&&b.left<r.right&&b.right>r.left&&b.top<r.bottom&&b.bottom>r.top)hit.push((node.getAttribute('aria-label')||node.textContent).trim().replace(/\s+/g,' ').slice(0,40))
    }
    return {hit,onScreen:r.top>=0&&r.bottom<=innerHeight}
   })
   await page.evaluate(()=>scrollTo(0,0));await settled()
   expect(await under()).toEqual({hit:[],onScreen:true})
   await page.screenshot({path:`${shots}/home-launcher-${width}${many?'-crowded':''}-top.png`})
   // Scrolled part-way through ten rows spaced closer than the button is tall there is no gap anywhere in view, and then
   // the button may cover only a sliver of a button's corner, never its label (a quarter of it at most).
   if(many)for(let y=70;y<=2100;y+=70){
    await page.evaluate(y=>scrollTo(0,y),y);await settled()
    const worst=await page.evaluate(()=>{
     const r=document.querySelector('.gaffer-launcher').getBoundingClientRect();let most=0
     for(const node of document.querySelectorAll('.waiting-list button,.waiting-list a[href]')){
      const b=node.getBoundingClientRect(),w=Math.min(b.right,r.right)-Math.max(b.left,r.left),h=Math.min(b.bottom,r.bottom)-Math.max(b.top,r.top)
      if(w>0&&h>0)most=Math.max(most,w*h/(b.width*b.height))
     }
     return {most,onScreen:r.top>=0&&r.bottom<=innerHeight}
    })
    expect(worst.onScreen,`scrolled to ${y}`).toBe(true);expect(worst.most,`scrolled to ${y}`).toBeLessThanOrEqual(.25)
   }
   await page.evaluate(()=>scrollTo(0,0));await settled()
   const last=page.locator('.waiting-list > li').last()
   await last.scrollIntoViewIfNeeded();await settled()
   expect(await under()).toEqual({hit:[],onScreen:true})
   await page.screenshot({path:`${shots}/home-launcher-${width}${many?'-crowded':''}-list-end.png`})
   await page.evaluate(()=>scrollTo(0,document.scrollingElement.scrollHeight));await settled()
   expect(await under()).toEqual({hit:[],onScreen:true})
   await page.screenshot({path:`${shots}/home-launcher-${width}${many?'-crowded':''}-end.png`})
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 // A new project starts the list at its first ten again.
 {
  const many=f=>{for(let i=0;i<12;i++)f.box.sessions.push({agentId:'a1',tool:'claude',model:'m',structured:true,createdAt:now-3*3600000,exitCode:2,dir:'/work',id:`many-${i}`,title:`Lost run ${i}`,state:'exited',updatedAt:now-(30+i)*MIN,projectId:'p2',attention:'failed',attentionSource:'terminal',changedSinceSeen:true})}
  const w=await open(834,'light',many),page=w.page
  try{
   await expect(entries(page)).toHaveCount(10)
   await section(page,'Waiting for you').getByRole('button',{name:'Show 7 more',exact:true}).click()
   await expect(entries(page)).toHaveCount(17)
   await page.getByRole('button',{name:'Projects: all',exact:true}).click()
   await page.getByRole('dialog',{name:'Projects'}).getByRole('button',{name:/^portal/}).click()
   await expect(entries(page)).toHaveCount(10)
   await expect(section(page,'Waiting for you').getByRole('button',{name:'Show 4 more',exact:true})).toBeVisible()
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('Home waiting: five entries oldest first with the summary, a pipeline’s session once with Allow and Deny, the finished session left to Continue working and the board as not seen yet, the title and bell at 5, Move to next stage confirmed and posted once, Factory working without you closed then remembered, both views kept, nothing waiting in one line, rows lined up at 1440 light and dark, cards narrowed by Projects at 834, 390 and 320, Details a link, the bell listing what it counts with links to the Factory, paging back to ten on a new project, ok')
}finally{await browser.close()}
