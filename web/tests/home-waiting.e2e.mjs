import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// Home's waiting items. Run once Home mounts them (Task 4 of the Home rework): on a wide screen the Factory rows and the
// session rows line up (chip, stages and buttons each in a fixed column), on a phone each is a card with no sideways scroll.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-home-waiting'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const sid='factory-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32),oid='e'.repeat(32),now=Date.now()
function fixture(){
 const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
 const session={id:sid,agentId:'a1',title:'Export API · code',tool:'opencode',model:'working-model',structured:true,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
 const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},box={moved:[],posted:[]}
 const goal={...pins,id:'1'.repeat(32),conversationId:cid,sessionId:sid,revision:3,text:'Outcome: x',state:'active',note:'Gaffer brought a question back.',maxReviews:12,usedReviews:3,expiresAt:now+20*3600000,createdAt:now-3600000,updatedAt:now,nextAt:now,lastSeq:42,summary:'Tests pass locally.',inputTokens:0,outputTokens:0,usageReported:true,advice:{choice:'move',why:'The review stage will catch the rest.'}}
 const row={goal,session:{id:sid,agentId:'a1',title:session.title,state:session.state},available:true,attention:true,reason:'Gaffer brought a question back.'}
 const stage=(kind,state,s=sid,c=cid)=>({kind,tool:'opencode',model:'',state,requestId:'r',...(state==='running'?{sessionId:s,conversationId:c}:{})})
 const outcome={id:oid,revision:5,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
  {key:'a',title:'Export API',prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:sid,conversationId:cid,updatedAt:now,stage:0,stages:[stage('code','running'),stage('review','pending'),stage('merge','pending')]},
  {key:'b',title:'Export button',prompt:'p',state:'following',sessionState:'running',note:'The review stage is under way.',sessionId:'s-b',conversationId:'b'.repeat(32),updatedAt:now,stage:1,stages:[stage('code','done'),stage('review','running','s-b','b'.repeat(32)),stage('merge','pending')]},
  {key:'c',title:'Export docs',prompt:'p',state:'ready_to_merge',sessionState:'running',note:'Ready to merge. Nothing merges until you approve it.',sessionId:'s-c',conversationId:'d'.repeat(32),updatedAt:now,stage:2,stages:[stage('code','done'),stage('review','done'),stage('merge','pending')]}]}
 const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],messages:[]}
 const goalView=()=>({goal,pins,allowed:true,setupAllowed:true,providerName:'Workshop Gaffer',conversationRevision:1,requiresProviderAccess:false})
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>view,
  '/api/v2/gaffer/outcomes':()=>({outcomes:[outcome]}),
  '/api/v2/gaffer/goals':()=>({goals:[row],readAt:now}),
  [`/api/v2/gaffer/conversations/${cid}/goal`]:()=>goalView(),
  [`/api/v2/gaffer/outcomes/${oid}/automation`]:request=>{const input=request.postDataJSON();box.posted.push(input);outcome.automation=input.on;return outcome},
  // Moving Export API on: its code stage is done, review is in hand, and Gaffer is no longer waiting on the person.
  [`/api/v2/gaffer/outcomes/${oid}/sessions/a/advance`]:request=>{box.moved.push(request.postDataJSON());const piece=outcome.sessions[0];piece.stages[0].state='done';piece.stage=1;piece.stages[1]={...piece.stages[1],state:'running',sessionId:sid,conversationId:cid};piece.note='The review stage is under way.';row.attention=false;delete row.reason;delete goal.advice;outcome.revision++;return outcome},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true,inspect:true}}]},
  '/api/v2/projects':{projects:[]},
  '/api/v2/folder-projects':{projects:[{id:'p1',host:'a1',machine:'Workshop',dir:'/work/billing',name:'billing',isRepo:true,baseBranch:'',worktreeDefault:'',colour:'',position:0,version:1,createdAt:now,updatedAt:now}],savedWaiting:0},
  '/api/v2/settings/yolo':{allowed:false},
 }
 return{routes,outcome,box,row}
}
// Sessions that need the person in other ways, so the rightmost button of each kind of row is measured.
function withMore(f){
 const base={agentId:'a1',tool:'claude',model:'m',structured:true,createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
 const approving={...base,id:'approve-session',title:'Release notes',state:'running',bellAt:now-90000,pendingPermission:{permissionId:'p1',title:'Run a command',command:'npm run build',path:'',truncated:false,queued:0}}
 const finished={...base,id:'review-session',title:'Billing tidy-up',state:'exited',exitCode:0,bellAt:now-120000,worktreePath:'/work/billing-wt',worktreeRepo:'/work/billing',worktreeBranch:'tidy'}
 const before=f.routes['/api/v2/terminal/sessions']
 f.routes['/api/v2/terminal/sessions']=()=>({sessions:[...before().sessions,approving,finished]})
}
// A second session that asked the person a question, so Home has a session row beside the Factory rows.
function withQuestion(f){
 const asking={id:'ask-session',agentId:'a1',title:'Portal sign-in copy',tool:'claude',model:'m',structured:true,state:'running',createdAt:now,updatedAt:now,bellAt:now-60000,exitCode:null,dir:'/work',pendingQuestion:{questionId:'q1',message:'Which wording for the expired-link page?',truncated:false,queued:0}}
 const sessions=f.routes['/api/v2/terminal/sessions']
 f.routes['/api/v2/terminal/sessions']=()=>({sessions:[...sessions.sessions,asking]})
}
async function open(width,theme){
 const f=fixture();withQuestion(f);withMore(f)
 const w=await openWorkspace(browser,{width,height:width<500?844:1000,theme,touch:width<1000,routes:f.routes})
 await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
 await w.page.goto(`${base}/`)
 return w
}
const right=async locator=>(await locator.boundingBox()).x+(await locator.boundingBox()).width
try{
 for(const theme of ['light','dark']){
  const w=await open(1440,theme),page=w.page
  try{
   const details=page.getByRole('button',{name:/^Details and other actions for /})
   const answer=page.getByRole('link',{name:'Answer in chat',exact:true})
   await expect(answer).toBeVisible();await expect(details.first()).toBeVisible()
   // The buttons' right edges are equal across a Factory row and each kind of session row, within 1 px: the Factory
   // row's Details, the question row's Answer in chat, and the rightmost button of the approval and review rows.
   const edges=[await right(details.first()),await right(answer)]
   for(const name of ['Release notes','Billing tidy-up']){
    const article=page.locator('article',{hasText:name}).first()
    await expect(article).toBeVisible()
    edges.push(await right(article.locator('.btn').last()))
   }
   const rowRight=await page.evaluate(()=>[...document.querySelectorAll('article')].map(a=>a.getBoundingClientRect().right))
   for(const edge of edges)expect(Math.abs(edge-edges[0])).toBeLessThanOrEqual(1)
   expect(Math.max(...rowRight)-Math.min(...rowRight)).toBeLessThanOrEqual(1)
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   await page.screenshot({path:`${shots}/home-waiting-1440-${theme}.png`,fullPage:true})
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 for(const width of [390,320]){
  const w=await open(width,'light'),page=w.page
  try{
   const answer=page.getByRole('link',{name:'Answer in chat',exact:true})
   await expect(answer).toBeVisible()
   // Each item is a card: its content stacks, so the title sits above the buttons.
   const cards=await page.evaluate(()=>[...document.querySelectorAll('article')].filter(a=>a.querySelector('.btn')).map(a=>getComputedStyle(a).borderTopWidth))
   expect(cards.length).toBeGreaterThan(0);for(const border of cards)expect(border).toBe('1px')
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   expect(await answer.evaluate(el=>el.getBoundingClientRect().height)).toBeGreaterThanOrEqual(44)
   await page.screenshot({path:`${shots}/home-waiting-${width}.png`,fullPage:true})
   expect(w.state.unexpected).toEqual([]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('Home waiting items: rows lined up at 1440 and cards at 390 and 320, ok')
}finally{await browser.close()}
