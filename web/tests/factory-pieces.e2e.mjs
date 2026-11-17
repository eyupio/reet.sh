import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// The Factory page's pieces, one labelled block each: the stage pipeline, a waiting item, the pipeline detail, and the
// page that puts them together (the list beside the detail on a wide screen, the detail as its own screen on a narrow one).
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-factory-pieces'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const sid='factory-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32),oid='e'.repeat(32),now=Date.now()
// Three pipelines of one outcome: Export API waits on a decision Gaffer brought back (its advice is to move on), Export
// button is working by itself, and Export docs waits for its merge to be approved.
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
async function open(width,{theme='light',height,hash='',tweak,scale}={}){
 const f=fixture();tweak?.(f)
 const w=await openWorkspace(browser,{width,height:height??(width<500?844:1000),theme,touch:width<1000,routes:f.routes,scale})
 await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
 await w.page.goto(`${base}/factory${hash}`)
 return{f,w,page:w.page}
}
const sideways=page=>page.evaluate(()=>document.documentElement.scrollWidth)
try{
 // The stage pipeline: three stages, the first in hand, and no sideways scroll on a phone.
 for(const [width,theme] of [[1440,'light'],[390,'dark']]){
  const {w,page}=await open(width,{theme})
  try{
   const stages=page.getByRole('list',{name:'Stages of Export API',exact:true})
   await expect(stages).toBeVisible()
   await expect(stages.getByRole('listitem')).toHaveCount(3)
   await expect(stages.getByRole('listitem').first()).toHaveAttribute('aria-current','step')
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/stage-pipeline-${width}-${theme}.png`})
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('ok - stage pipeline')
 // A waiting item: its filled action and the way to the detail; a working one has no filled button.
 {
  const {w,page}=await open(1000,{height:900})
  try{
   const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
   await expect(waiting.getByRole('button',{name:/^Move to next stage/})).toBeVisible()
   await expect(waiting.getByRole('button',{name:'Details and other actions for Export API',exact:true})).toBeVisible()
   await page.getByRole('button',{name:/^Working/}).click()
   const working=page.getByRole('region',{name:'Working',exact:true})
   await expect(working.locator(`#pipeline-${oid}-b`)).toContainText('Export button')
   await expect(working.locator('.btn.primary')).toHaveCount(0)
  }finally{await w.context.close()}
  console.log('ok - waiting item')
 }
 // The pipeline detail: one bar of what can be done, the settings in a line, who decides, and the facts of a picked stage.
 {
  const {f,w,page}=await open(1440)
  try{
   // On a wide screen the list's item is one button that opens its pipeline beside the list.
   const item=page.getByRole('region',{name:'Waiting for you',exact:true}).getByRole('button',{name:'Decision: Export API',exact:true})
   await item.click();await expect(item).toHaveAttribute('aria-pressed','true')
   const bar=page.getByRole('group',{name:'What do you want to do?',exact:true})
   await expect(bar.getByRole('button').first()).toHaveAccessibleName(/^Move to next stage/)
   await expect(bar.getByRole('button',{name:'Tell the agent…',exact:true})).toHaveCount(1)
   await expect(bar.getByRole('button',{name:/^Move to next stage/})).toHaveCount(1)
   await expect(bar.getByRole('button',{name:'Goal and limits',exact:true})).toHaveCount(0)
   await page.getByText('Usage and budget',{exact:true}).click()
   await expect(page.getByRole('button',{name:'Goal and limits',exact:true})).toBeVisible()
   await expect(page.getByText('You decide · Ask me · merge waits for you',{exact:true})).toBeVisible()
   await page.getByText('Settings for this outcome',{exact:true}).click()
   await page.getByRole('radio',{name:'Gaffer decides',exact:true}).check()
   await expect.poll(()=>f.box.posted).toEqual([{revision:5,on:true}])
   await page.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('button',{name:/Review/}).click()
   await expect(page.getByRole('region',{name:'Review stage',exact:true})).toBeVisible()
   await expect(page.getByRole('region',{name:'Review stage',exact:true})).toContainText('will do this stage')
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
  console.log('ok - pipeline detail')
 }
 // The page on a wide screen: the list and the detail side by side, the first waiting pipeline open on arrival.
 for(const [width,theme] of [[1280,'light'],[1440,'dark']]){
  const {f,w,page}=await open(width,{theme})
  try{
   const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
   await expect(waiting).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
   await expect(waiting.getByRole('button',{name:'Decision: Export API',exact:true})).toHaveAttribute('aria-pressed','true')
   // One line of counts replaces the tiles.
   await expect(page.getByText('2 waiting for you · 1 working · 0 finished',{exact:true})).toBeVisible()
   await expect(page.getByRole('list',{name:'Factory at a glance',exact:true})).toHaveCount(0)
   // Working starts closed and still says what is in it; Finished or stopped and Other sessions are closed too.
   const workingHead=page.getByRole('button',{name:/^Working/})
   await expect(workingHead).toHaveAttribute('aria-expanded','false');await expect(workingHead).toContainText('Export button')
   await expect(page.getByRole('button',{name:/^Finished or stopped/})).toHaveAttribute('aria-expanded','false')
   await expect(page.getByRole('button',{name:/^Other sessions/})).toHaveAttribute('aria-expanded','false')
   // The detail's bar stays at the foot of the visible detail.
   const pane=page.locator('.pane'),barBox=await page.getByRole('group',{name:'What do you want to do?',exact:true}).boundingBox(),paneBox=await pane.boundingBox()
   expect(barBox.y+barBox.height).toBeLessThanOrEqual(paneBox.y+paneBox.height+1);expect(barBox.y+barBox.height).toBeGreaterThan(paneBox.y+paneBox.height-40)
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/factory-page-${width}-${theme}.png`})
   if(width===1280){
    // Choosing another item opens it, and the address names it so a reload reopens it.
    await waiting.getByRole('button',{name:'Ready to merge: Export docs',exact:true}).click()
    await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
    await expect(page).toHaveURL(new RegExp(`[?&]pipeline=${oid}\\.c(&|$)`))
    await page.reload()
    await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
    await waiting.getByRole('button',{name:'Decision: Export API',exact:true}).click()
    // Taking the decision moves the pipeline to its next stage and out of Waiting for you; the detail stays on it.
    await page.getByRole('group',{name:'What do you want to do?',exact:true}).getByRole('button',{name:/^Move to next stage/}).click()
    await page.getByRole('dialog').getByRole('button',{name:'Move on',exact:true}).click()
    await expect.poll(()=>f.box.moved).toEqual([{confirmed:true,stage:'code'}])
    await expect(waiting.getByRole('button',{name:'Decision: Export API',exact:true})).toHaveCount(0)
    await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
    await expect(page.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('listitem').nth(1)).toHaveAttribute('aria-current','step')
    // A section the person opened stays open after a reload.
    await workingHead.click();await expect(workingHead).toHaveAttribute('aria-expanded','true')
    await page.reload()
    await expect(page.getByRole('button',{name:/^Working/})).toHaveAttribute('aria-expanded','true')
    await expect(page.getByRole('region',{name:'Working',exact:true}).getByRole('button',{name:'Running: Export API',exact:true})).toBeVisible()
   }
   expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 // A link to a pipeline (Home's, or an old bookmark) opens that pipeline.
 for(const width of [1280,834]){
  const {w,page}=await open(width,{hash:`#pipeline-${oid}-b`})
  try{
   const title=page.getByRole('heading',{name:'Export button',level:2,exact:true})
   await expect(title).toBeVisible();await expect(title).toBeFocused()
   if(width<1100)await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toHaveCount(0)
   // Its section opens, though Working starts closed, so the chosen item is in view in the list.
   else{await expect(page.getByRole('button',{name:/^Working/})).toHaveAttribute('aria-expanded','true');await expect(page.getByRole('button',{name:'Running: Export button',exact:true})).toHaveAttribute('aria-pressed','true')}
  }finally{await w.context.close()}
 }
 console.log('ok - the page on a wide screen')
 // The page on a tablet and a phone: the list is the page, and Details opens one pipeline as a screen of its own.
 for(const [width,theme] of [[834,'light'],[390,'dark']]){
  const {w,page}=await open(width,{theme})
  try{
   const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
   await expect(waiting).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toHaveCount(0)
   await expect(page.getByRole('group',{name:'What do you want to do?',exact:true})).toHaveCount(0)
   // From 600 px the cards name each stage; below it they are circles with a caption.
   const stages=waiting.getByRole('list',{name:'Stages of Export API',exact:true})
   if(width>=600)await expect(stages.locator('.name')).toHaveText(['Code','Review','Merge'])
   else await expect(waiting.getByText('Code in hand · the first stage · next Review',{exact:true})).toBeVisible()
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/factory-list-${width}-${theme}.png`,fullPage:true})
   await waiting.getByRole('button',{name:'Details and other actions for Export API',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
   // The focus moves to the detail's title, not to the top of the document.
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeFocused()
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toHaveCount(0)
   await expect(page).toHaveURL(new RegExp(`[?&]pipeline=${oid}\\.a(&|$)`))
   const nav=page.getByRole('navigation',{name:'Waiting items',exact:true})
   await expect(nav).toContainText('1 of 2 waiting for you')
   // The bar sits at the foot of the screen while the detail scrolls.
   const bar=page.getByRole('group',{name:'What do you want to do?',exact:true}),box=await bar.boundingBox(),inner=await page.evaluate(()=>innerHeight)
   expect(box.y+box.height).toBeLessThanOrEqual(inner+1)
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/factory-detail-${width}-${theme}.png`})
   // Previous and next step through what is waiting, in order.
   await nav.getByRole('button',{name:'Next waiting item',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
   await expect(nav).toContainText('2 of 2 waiting for you');await expect(nav.getByRole('button',{name:'Next waiting item',exact:true})).toBeDisabled()
   await nav.getByRole('button',{name:'Previous waiting item',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
   // Back to the list, and the address no longer names a pipeline.
   await nav.getByRole('button',{name:'‹ Factory',exact:true}).click()
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toHaveCount(0)
   expect(new URL(page.url()).searchParams.get('pipeline')).toBeNull()
   // The focus is back on the Details button of the item that was open.
   await expect(waiting.getByRole('button',{name:'Details and other actions for Export API',exact:true})).toBeFocused()
   // The browser's Back closes a detail opened from the list, and Forward opens it again.
   await waiting.getByRole('button',{name:'Details and other actions for Export docs',exact:true}).click()
   await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
   await page.goBack()
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toHaveCount(0)
   expect(new URL(page.url()).pathname).toBe('/factory');expect(new URL(page.url()).searchParams.get('pipeline')).toBeNull()
   await page.goForward()
   await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
   await page.getByRole('button',{name:'‹ Factory',exact:true}).click()
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
   expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('ok - the page on a tablet and a phone')

 // The Projects picker: a second project with one running pipeline, so the counts and the narrowing have something to say.
 const twoProjects=f=>{
  const stage=(kind,state)=>({kind,tool:'opencode',model:'',state,requestId:'r'})
  const other={id:'f'.repeat(32),revision:1,objective:'Tidy the rail.',state:'settled',note:'',agentId:'a1',projectId:'p2',projectName:'reet',dir:'/work/reet',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
   {key:'z',title:'Rail tidy',prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:'s-z',conversationId:'9'.repeat(32),updatedAt:now,stage:0,stages:[stage('code','running'),stage('merge','pending')]}]}
  f.routes['/api/v2/gaffer/outcomes']=()=>({outcomes:[f.outcome,other]})
  const folder=f.routes['/api/v2/folder-projects'];f.routes['/api/v2/folder-projects']={projects:[folder.projects[0],{...folder.projects[0],id:'p2',dir:'/work/reet',name:'reet',position:1}],savedWaiting:0}
 }
 for(const width of [834,390]){
  const {w,page}=await open(width,{tweak:twoProjects})
  try{
   const pick=page.getByRole('button',{name:'Projects: all',exact:true})
   await expect(pick).toBeVisible()
   await expect(page.getByRole('main').getByText('Export API',{exact:true}).first()).toBeVisible()
   await page.screenshot({path:`${shots}/picker-closed-${width}.png`})
   await pick.click()
   const dialog=page.getByRole('dialog')
   await expect(dialog).toBeVisible()
   await expect(dialog.getByRole('button',{name:/^All projects/})).toHaveAttribute('aria-pressed','true')
   await expect(dialog.getByRole('button',{name:/^billing/})).toContainText('2 waiting · 1 working')
   await expect(dialog.getByRole('button',{name:/^reet/})).toContainText('0 waiting · 1 working')
   await page.screenshot({path:`${shots}/picker-open-${width}.png`})
   await dialog.getByRole('button',{name:/^billing/}).click()
   await expect(dialog).toHaveCount(0)
   await expect(page.getByRole('button',{name:'Projects: billing',exact:true})).toBeVisible()
   await expect(page.getByText('Rail tidy',{exact:true})).toHaveCount(0)
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
   await page.getByRole('button',{name:'Projects: billing',exact:true}).click()
   await expect(page.getByRole('dialog').getByRole('button',{name:/^billing/})).toHaveAttribute('aria-pressed','true')
   await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0)
   await page.getByRole('button',{name:'Projects: billing',exact:true}).click()
   await page.getByRole('dialog').getByRole('button',{name:/^All projects/}).click()
   await expect(page.getByRole('button',{name:'Projects: all',exact:true})).toBeVisible()
   expect(await sideways(page)).toBeLessThanOrEqual(width)
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('ok - projects picker on a tablet and a phone')
 // The wide Projects panel says how many of each project's pipelines wait and work, and has no picker button.
 {
  const {w,page}=await open(1440,{tweak:twoProjects})
  try{
   await expect(page.getByRole('button',{name:/^Projects: /})).toHaveCount(0)
   await expect(page.getByRole('button',{name:/^billing.*2 waiting · 1 working/})).toBeVisible()
   await expect(page.getByRole('button',{name:/^reet.*0 waiting · 1 working/})).toBeVisible()
   await expect(page.locator('aside.rail')).toContainText('2 waiting · 1 working')
   await page.screenshot({path:`${shots}/projects-panel-1440.png`})
  }finally{await w.context.close()}
  console.log('ok - projects panel counts')
 }
 // A phone with five pipelines of eight stages: no sideways scroll, big touch targets, and the caption under the circles.
 {
  const eight=['plan','design','code','review','test','pr','babysit','merge']
  const {w,page}=await open(390,{tweak:f=>{
   f.outcome.sessions=['a','b','c','d','e'].map((key,i)=>({key,title:`Pipeline ${i+1}`,prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:`s-${key}`,conversationId:String(i+1).repeat(32),updatedAt:now,stage:2,stages:eight.map((kind,n)=>({kind,tool:'opencode',model:'',state:n<2?'done':n===2?'running':'pending',requestId:'r'}))}))
  }})
  try{
   await page.getByRole('button',{name:/^Working/}).click()
   const card=page.locator(`#pipeline-${oid}-a`)
   await expect(card).toContainText('Code in hand · Plan, Design done · next Review')
   const heights=await card.getByRole('button').evaluateAll(list=>list.map(b=>b.getBoundingClientRect().height))
   expect(heights.length).toBeGreaterThan(0);for(const h of heights)expect(h).toBeGreaterThanOrEqual(44)
   await page.screenshot({path:`${shots}/five-pipelines-390.png`})
   expect(await sideways(page)).toBeLessThanOrEqual(390)
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
  console.log('ok - five pipelines of eight stages on a phone')
 }
 // Each check below reports by itself, so one that fails does not hide the next; the run fails at the end if any did.
 const failures=[]
 const step=async(name,run)=>{try{await run();console.log(`ok - ${name}`)}catch(e){failures.push(name);console.log(`not ok - ${name}\n   ${String(e?.message??e).split('\n').filter(Boolean).slice(0,8).join('\n   ')}`)}}
 const eight=['plan','design','code','review','test','pr','babysit','merge']
 const stagesOf=(kinds,at,s=sid,c=cid)=>kinds.map((kind,n)=>({kind,tool:'opencode',model:'',state:n<at?'done':n===at?'running':'pending',requestId:'r',...(n===at?{sessionId:s,conversationId:c}:{})}))
 // The agent of Export API holds a tool approval: its item is an Approval, answered with Allow once or Deny.
 const approval=f=>{
  f.box.answers=[];f.row.goal.waitingOnWorker=true;delete f.row.goal.advice
  f.routes['/api/v2/terminal/sessions'].sessions[0].pendingPermission={permissionId:'perm-1',title:'Run a command',command:'npm run check',queued:0}
  f.routes[`/api/v2/terminal/sessions/${sid}/chat/permissions/perm-1`]=request=>{f.box.answers.push(request.postDataJSON());return {}}
 }
 // The floating Gaffer button, once it has settled where it rests, and what it would cover there: the topmost thing under
 // each of a grid of points across it that is text, a control, an item or the stage card.
 const settled=async page=>{
  await expect(page.locator('.gaffer-launcher')).toBeVisible()
  await expect.poll(()=>page.evaluate(()=>new Promise(done=>{const el=document.querySelector('.gaffer-launcher');if(!el)return done(false);const a=el.getBoundingClientRect();requestAnimationFrame(()=>requestAnimationFrame(()=>{const b=el.getBoundingClientRect();done(el.style.getPropertyValue('--launcher-clearance')!==''&&a.top===b.top&&a.left===b.left)}))}))).toBe(true)
 }
 const covered=page=>page.evaluate(()=>{
  const el=document.querySelector('.gaffer-launcher'),r=el.getBoundingClientRect(),hit=new Set()
  const busy='button,a,summary,input,label,select,textarea,p,h1,h2,h3,dt,dd,code,li,.item,.stage,.waits,.rows,.caption,.name,.ask,.counts,.choices'
  for(let i=0;i<=6;i++)for(let j=0;j<=6;j++){
   const under=document.elementsFromPoint(r.left+1+(r.width-2)*i/6,r.top+1+(r.height-2)*j/6).find(node=>!el.contains(node))
   const what=under?.closest(busy)
   if(what)hit.add(`${what.tagName.toLowerCase()}${what.className?`.${String(what.className).trim().split(/\s+/).join('.')}`:''}: ${what.textContent.trim().replace(/\s+/g,' ').slice(0,40)}`)
  }
  return [...hit]
 })

 // F1: an approval is answered from the narrow card and from the detail on every width, and posts what Home posts.
 for(const width of [1280,834]){
  await step(`approval answered with Allow once at ${width} px`,async()=>{
   const {f,w,page}=await open(width,{tweak:approval})
   try{
    if(width<1100){
     const card=page.locator(`#pipeline-${oid}-a`)
     await expect(card).toContainText('Approval')
     await expect(card.getByRole('button',{name:'Allow once',exact:true})).toBeVisible()
     await expect(card.getByRole('button',{name:'Deny',exact:true})).toBeVisible()
     await card.getByRole('button',{name:'Details and other actions for Export API',exact:true}).click()
    }
    await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
    const bar=page.getByRole('group',{name:'What do you want to do?',exact:true})
    await expect(bar).toContainText('npm run check')
    await expect(bar.getByRole('button',{name:'Deny',exact:true})).toBeVisible()
    await bar.getByRole('button',{name:'Allow once',exact:true}).click()
    await expect.poll(()=>f.box.answers).toEqual([{allow:true}])
    expect(w.errors).toEqual([]);expect(w.state.unexpected).toEqual([])
   }finally{await w.context.close()}
  })
 }
 // F4: the card says Gaffer's recommendation in the detail's words and marks the filled button; without advice it is as before.
 await step('the waiting card says what Gaffer recommends',async()=>{
  const {w,page}=await open(834)
  try{
   const card=page.locator(`#pipeline-${oid}-a`)
   await expect(card.locator('.why')).toHaveText('Gaffer recommends: Move to next stage. The review stage will catch the rest.')
   await expect(card.locator('.btn.primary')).toHaveAccessibleName('Move to next stage Recommended')
   const docs=page.locator(`#pipeline-${oid}-c`)
   await expect(docs.locator('.why')).toHaveText('Ready to merge. Nothing merges until you approve it.')
   await expect(docs.locator('.btn.primary')).toHaveAccessibleName('Approve the merge')
   await docs.getByRole('button',{name:/^Details/}).click()
   await expect(page.getByRole('group',{name:'What do you want to do?',exact:true}).locator('.btn.primary')).toHaveAccessibleName(/^Approve the merge… Recommended$/)
  }finally{await w.context.close()}
 })
 // F2: an archived outcome's pipelines offer nothing but Restore: no bar, a card with only Details, a read-only report and settings.
 await step('an archived outcome offers no action',async()=>{
  const {w,page}=await open(834,{tweak:f=>{
   f.outcome.archivedAt=now;f.outcome.state='stopped'
   f.outcome.report={text:'Two pipelines did not finish.',at:now,proposals:[{title:'Retry the importer',prompt:'p',why:'It failed to start.'}],inputTokens:1,outputTokens:1,usageReported:true}
   Object.assign(f.outcome.sessions[0],{state:'failed',note:'Not started: the agent is missing.'})
   Object.assign(f.outcome.sessions[1],{state:'unfollowed',note:'Stopped following.'})
   Object.assign(f.outcome.sessions[2],{state:'done',note:'Every stage is done.'})
  }})
  try{
   await page.getByRole('button',{name:'Show archived (1)',exact:true}).click()
   const head=page.getByRole('button',{name:/^Finished or stopped/});if(await head.getAttribute('aria-expanded')!=='true')await head.click()
   for(const key of ['a','b','c']){
    const names=await page.locator(`#pipeline-${oid}-${key}`).getByRole('button').evaluateAll(list=>list.map(b=>b.getAttribute('aria-label')||b.textContent.trim()))
    expect(names.length).toBe(1);expect(names[0]).toMatch(/^Details and other actions for /)
   }
   for(const [key,title] of [['a','Export API'],['b','Export button']]){
    await page.locator(`#pipeline-${oid}-${key}`).getByRole('button',{name:/^Details/}).click()
    await expect(page.getByRole('heading',{name:title,level:2,exact:true})).toBeVisible()
    await expect(page.getByRole('group',{name:'What do you want to do?',exact:true})).toHaveCount(0)
    for(const row of ['Gaffer’s report','Usage and budget','Settings for this outcome'])await page.locator('.detail summary').filter({hasText:row}).click()
    const rows=page.locator('.detail .rows')
    await expect(rows).toContainText('Two pipelines did not finish.');await expect(rows).toContainText('Retry the importer')
    await expect(rows.getByRole('checkbox')).toHaveCount(0);await expect(rows.getByRole('radio')).toHaveCount(0)
    await expect(rows.getByRole('button')).toHaveCount(0)
    await page.getByRole('button',{name:'More for this outcome',exact:true}).click()
    await expect(page.locator('.detail .menu ul button')).toHaveText(['Restore'])
    await page.getByRole('button',{name:'‹ Factory',exact:true}).click()
   }
   expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 })
 await step('a pipeline no longer followed has no merge switch',async()=>{
  const {w,page}=await open(1440,{tweak:f=>{Object.assign(f.outcome.sessions[1],{state:'unfollowed',note:'Stopped following.'})}})
  try{
   await page.getByRole('button',{name:/^Finished or stopped/}).click()
   await page.getByRole('button',{name:'Stopped: Export button',exact:true}).click()
   await page.locator('.detail summary').filter({hasText:'Settings for this outcome'}).click()
   await expect(page.locator('.detail .rows')).toContainText('Waits for you')
   await expect(page.getByRole('button',{name:/merge by itself|wait for you before merging/})).toHaveCount(0)
  }finally{await w.context.close()}
 })
 // F2: a read that fails after one that worked keeps the list and says so.
 await step('a failed read keeps the last list and says so',async()=>{
  // Reads fail from the moment the page has shown its list (the bell reads the outcomes too, so a count of reads would not do).
  const read={failing:false}
  const {w,page}=await open(1280,{tweak:f=>{const answer=f.routes['/api/v2/gaffer/outcomes'];f.routes['/api/v2/gaffer/outcomes']=request=>read.failing?{status:503,json:{error:'The controller is restarting.'}}:answer(request)}})
  try{
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
   read.failing=true
   await expect(page.getByRole('alert').filter({hasText:'Showing the last successful read.'})).toBeVisible({timeout:15000})
   await expect(page.getByRole('region',{name:'Waiting for you',exact:true}).getByRole('button',{name:'Decision: Export API',exact:true})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
  }finally{await w.context.close()}
 })
 // F3a: eight named stages on a phone: the detail never scrolls the page sideways and every stage is a full-size target.
 await step('eight labelled stages in a phone detail',async()=>{
  const {w,page}=await open(390,{tweak:f=>{Object.assign(f.outcome.sessions[0],{stage:5,stages:stagesOf(eight,5)})}})
  try{
   await page.locator(`#pipeline-${oid}-a`).getByRole('button',{name:/^Details/}).click()
   const list=page.getByRole('list',{name:'Choose a stage of Export API',exact:true})
   await expect(list.getByRole('button')).toHaveCount(8)
   expect(await sideways(page)).toBeLessThanOrEqual(390)
   const sizes=await list.getByRole('button').evaluateAll(l=>l.map(b=>{const r=b.getBoundingClientRect();return [r.width,r.height]}))
   for(const [wide,high] of sizes){expect(wide).toBeGreaterThanOrEqual(44);expect(high).toBeGreaterThanOrEqual(44)}
   // Each one-word name stays whole on one line: no stage is named "Desig n".
   const lines=await list.locator('.name').evaluateAll(l=>l.map(n=>Math.round(n.getBoundingClientRect().height/parseFloat(getComputedStyle(n).lineHeight))))
   expect(lines).toEqual([1,1,1,1,1,1,1,1])
   // The stage in hand is in view inside the line's own box.
   const seen=await list.evaluate(ol=>{const box=ol.parentElement.getBoundingClientRect(),here=ol.querySelector('[aria-current]').getBoundingClientRect();return here.left>=box.left-1&&here.right<=box.right+1})
   expect(seen).toBe(true)
   for(const [index,name] of [[0,'Plan'],[7,'Merge']]){await list.getByRole('button').nth(index).click();await expect(page.getByRole('region',{name:`${name} stage`,exact:true})).toBeVisible()}
   expect(await sideways(page)).toBeLessThanOrEqual(390)
   await page.screenshot({path:`${shots}/eight-stages-detail-390.png`})
   expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 })
 // F3b: a pipeline with no stages lists, opens and offers its action, with no stage line.
 for(const width of [1280,834]){
  await step(`a pipeline with no stages at ${width} px`,async()=>{
   const {w,page}=await open(width,{tweak:f=>{const piece=f.outcome.sessions[2];delete piece.stages;delete piece.stage;Object.assign(piece,{state:'failed',note:'Not started: that agent is not installed.'})}})
   try{
    const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
    if(width<1100){
     const card=page.locator(`#pipeline-${oid}-c`)
     await expect(card.getByRole('button',{name:'Try again',exact:true})).toBeVisible()
     await card.getByRole('button',{name:/^Details/}).click()
    }else await waiting.getByRole('button',{name:'Failed: Export docs',exact:true}).click()
    await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
    await expect(page.getByRole('list',{name:'Choose a stage of Export docs',exact:true})).toHaveCount(0)
    await expect(page.getByRole('group',{name:'What do you want to do?',exact:true}).getByRole('button',{name:/^Try again/})).toBeVisible()
    expect(w.errors).toEqual([])
   }finally{await w.context.close()}
  })
 }
 // F3c: a stage split in four parts with long names: the names wrap and no two circles overlap.
 for(const width of [834,390]){
  await step(`four long part names at ${width} px`,async()=>{
   const parts=['Database migration and backfill','Billing export endpoint and its tests','Download button in the account page','Documentation for the export format']
   const {w,page}=await open(width,{tweak:f=>{const stages=[{kind:'plan',tool:'opencode',model:'',state:'done',requestId:'r'},...parts.map((part,n)=>({kind:'code',part,tool:'opencode',model:'',state:n===0?'running':'pending',requestId:'r',...(n===0?{sessionId:sid,conversationId:cid}:{})})),{kind:'review',tool:'opencode',model:'',state:'pending',requestId:'r'}];Object.assign(f.outcome.sessions[0],{stage:1,stages})}})
   try{
    await page.locator(`#pipeline-${oid}-a`).getByRole('button',{name:/^Details/}).click()
    const list=page.getByRole('list',{name:'Choose a stage of Export API',exact:true})
    await expect(list.locator('.name')).toHaveCount(6)
    const boxes=await list.evaluate(ol=>({dots:[...ol.querySelectorAll('.dot')].map(d=>{const r=d.getBoundingClientRect();return [r.left,r.right,r.top,r.bottom]}),names:[...ol.querySelectorAll('.name')].map(n=>{const r=n.getBoundingClientRect(),li=n.closest('li').getBoundingClientRect();return {lines:Math.round(r.height/parseFloat(getComputedStyle(n).lineHeight)),inside:r.left>=li.left-1&&r.right<=li.right+1}})}))
    for(let i=0;i<boxes.dots.length;i++)for(let j=i+1;j<boxes.dots.length;j++){const [a,b]=[boxes.dots[i],boxes.dots[j]];expect(a[1]<=b[0]||b[1]<=a[0]||a[3]<=b[2]||b[3]<=a[2]).toBe(true)}
    expect(boxes.names.every(n=>n.inside)).toBe(true);expect(boxes.names.some(n=>n.lines>1)).toBe(true)
    // They wrap between words, never inside one.
    const split=await list.evaluate(ol=>[...ol.querySelectorAll('.name')].flatMap(n=>{const text=n.firstChild,out=[];let at=0;for(const word of text.textContent.split(' ')){const range=document.createRange();range.setStart(text,at);range.setEnd(text,at+word.length);if(new Set([...range.getClientRects()].map(r=>Math.round(r.top))).size>1)out.push(word);at+=word.length+1}return out}))
    expect(split).toEqual([])
    expect(await sideways(page)).toBeLessThanOrEqual(width)
    await page.screenshot({path:`${shots}/split-stage-detail-${width}.png`})
   }finally{await w.context.close()}
  })
 }
 // F3d: the YOLO confirmation and the merge-on-YOLO confirmation open from the Settings row and post what they did before.
 await step('YOLO and merge confirmations from the Settings row',async()=>{
  const {f,w,page}=await open(1440,{tweak:f=>{
   f.box.yolo=[];f.box.modes=[]
   f.routes[`/api/v2/gaffer/outcomes/${oid}/yolo`]=request=>{const input=request.postDataJSON();f.box.yolo.push(input);f.outcome.yolo=input.on;f.outcome.revision++;return {outcome:f.outcome,notSwitched:[]}}
   f.routes[`/api/v2/gaffer/outcomes/${oid}/sessions/a/automerge`]=request=>{const input=request.postDataJSON();f.box.modes.push(input);f.outcome.sessions[0].autoMerge=input.on;f.outcome.sessions[0].yoloMergeOk=!!input.yoloConfirmed;f.outcome.revision++;return f.outcome}
  }})
  try{
   await page.locator('.detail summary').filter({hasText:'Settings for this outcome'}).click()
   await page.getByRole('button',{name:'Put this outcome on YOLO',exact:true}).click()
   const yolo=page.getByRole('dialog',{name:'Put this outcome on YOLO?',exact:true})
   await expect(yolo).toBeVisible();expect(f.box.yolo).toEqual([])
   await yolo.getByRole('button',{name:'Switch to YOLO',exact:true}).click()
   await expect.poll(()=>f.box.yolo).toEqual([{revision:5,on:true,confirmed:true}])
   await page.getByRole('button',{name:'Let Export API merge by itself',exact:true}).click()
   const merge=page.getByRole('dialog',{name:'Let “Export API” merge by itself on YOLO?',exact:true})
   await expect(merge).toBeVisible();expect(f.box.modes).toEqual([])
   await merge.getByRole('button',{name:'Let it merge by itself',exact:true}).click()
   await expect.poll(()=>f.box.modes).toEqual([{on:true,yoloConfirmed:true}])
   expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 })
 // X1: an address naming a pipeline of an archived outcome opens it, with archived outcomes shown.
 for(const width of [1280,834]){
  await step(`?pipeline= of an archived outcome at ${width} px`,async()=>{
   const {w,page}=await open(width,{hash:`?pipeline=${oid}.c`,tweak:f=>{f.outcome.archivedAt=now}})
   try{
    await expect(page.getByRole('heading',{name:'Export docs',level:2,exact:true})).toBeVisible()
    await expect(page.locator('section.detail')).toContainText('archived')
    if(width<1100)await page.getByRole('button',{name:'‹ Factory',exact:true}).click()
    await expect(page.getByRole('button',{name:'Hide archived',exact:true})).toHaveAttribute('aria-pressed','true')
   }finally{await w.context.close()}
  })
 }
 // X2: History is read again each time its row is opened.
 await step('History is read again on each opening',async()=>{
  const event=(id,stage)=>({id,at:now-id*1000,source:'gaffer',action:'gaffer-stage-started',detail:JSON.stringify({outcome:oid,pipeline:'a',stage,reason:'Under way.'})})
  const {f,w,page}=await open(1440,{tweak:f=>{f.events=[event(2,'code')];f.routes[`/api/v2/gaffer/outcomes/${oid}/history`]=()=>({events:f.events})}})
  try{
   const row=page.locator('.detail details.row').filter({has:page.locator('summary .label').getByText('History',{exact:true})})
   await row.locator('summary').click()
   await expect(row.locator('.history li')).toHaveCount(1)
   await row.locator('summary').click()
   f.events=[event(1,'review'),...f.events]
   await row.locator('summary').click()
   await expect(row.locator('.history li')).toHaveCount(2)
   await expect(row.locator('summary')).toContainText('2 events')
  }finally{await w.context.close()}
 })
 // A1: four choices stay on one line at 1280 and wider.
 for(const width of [1280,1440]){
  await step(`the bar's four choices on one line at ${width} px`,async()=>{
   const {w,page}=await open(width)
   try{
    const buttons=page.getByRole('group',{name:'What do you want to do?',exact:true}).getByRole('button')
    await expect(buttons).toHaveCount(4)
    const tops=await buttons.evaluateAll(l=>l.map(b=>Math.round(b.getBoundingClientRect().top)))
    expect(new Set(tops).size).toBe(1)
   }finally{await w.context.close()}
  })
 }
 // A2, A3: the compact item's dots run edge to edge under its title; the detail's eyebrow has one full stop at most.
 await step('compact dots edge to edge, and the eyebrow without a doubled stop',async()=>{
  const {w,page}=await open(1440)
  try{
   const edges=await page.getByRole('button',{name:'Decision: Export API',exact:true}).evaluate(el=>{const s=getComputedStyle(el),box=el.getBoundingClientRect(),dots=[...el.querySelectorAll('.dot')].map(d=>d.getBoundingClientRect());return {left:box.left+parseFloat(s.paddingLeft)+parseFloat(s.borderLeftWidth),right:box.right-parseFloat(s.paddingRight)-parseFloat(s.borderRightWidth),first:dots[0].left,last:dots.at(-1).right}})
   expect(Math.abs(edges.first-edges.left)).toBeLessThanOrEqual(1);expect(Math.abs(edges.last-edges.right)).toBeLessThanOrEqual(1)
   await expect(page.locator('.detail .head .eyebrow')).toHaveText('Ship CSV export for billing · pipeline 1 of 3')
  }finally{await w.context.close()}
 })
 // A4: when Gaffer advises opening the session, the bar's first choice is that, filled.
 await step('advice to open the session fills it in the bar',async()=>{
  const {w,page}=await open(1440,{tweak:f=>{f.row.goal.advice={choice:'open',why:'The agent wrote its question in the chat.'}}})
  try{
   const bar=page.getByRole('group',{name:'What do you want to do?',exact:true})
   const first=bar.locator('.choices .btn').first()
   await expect(first).toHaveClass(/primary/);await expect(first).toHaveAccessibleName(/^Open the session/)
   await expect(first).toHaveAttribute('href',`/sessions?agent=a1&session=${sid}`)
  }finally{await w.context.close()}
 })
 // A5: the outcome menu closes on Escape and on a click outside it, and the focus goes back to its button.
 await step('the outcome menu closes on Escape and outside',async()=>{
  const {w,page}=await open(1440)
  try{
   const more=page.getByRole('button',{name:'More for this outcome',exact:true}),menu=page.locator('.detail details.menu')
   await more.click();await expect(menu).toHaveAttribute('open','')
   await page.keyboard.press('Escape');await expect(menu).not.toHaveAttribute('open','');await expect(more).toBeFocused()
   await more.click();await expect(menu).toHaveAttribute('open','')
   await page.getByRole('heading',{name:'Factory',level:1,exact:true}).click();await expect(menu).not.toHaveAttribute('open','');await expect(more).toBeFocused()
  }finally{await w.context.close()}
 })
 // A6, A7: a card's title is a heading, and a failed stage is not the stage in hand.
 await step('card headings, and a failed stage is not current',async()=>{
  const {w,page}=await open(834,{tweak:f=>{Object.assign(f.outcome.sessions[1],{state:'failed',note:'Not started: the agent is missing.'})}})
  try{
   await expect(page.getByRole('heading',{name:'Export API',level:3,exact:true})).toBeVisible()
   const stages=page.getByRole('list',{name:'Stages of Export button',exact:true}).getByRole('listitem')
   await expect(stages.nth(1)).toHaveClass(/failed/)
   expect(await stages.evaluateAll(l=>l.filter(li=>li.hasAttribute('aria-current')).length)).toBe(0)
  }finally{await w.context.close()}
 })
 // A8: a group chosen on the wide rail is named on the Projects button once the window is narrowed.
 await step('a group scope is named on the narrow Projects button',async()=>{
  const {w,page}=await open(1440,{tweak:f=>{f.routes['/api/v2/folder-projects'].projects[0].groupId='g1';f.routes['/api/v2/project-groups']={groups:[{id:'g1',name:'Workshop group',colour:'',position:0,createdAt:now,updatedAt:now,version:1}]}}})
  try{
   await page.locator('aside.rail .group-name').filter({hasText:'Workshop group'}).click()
   await expect(page.getByText('Showing Workshop group.')).toBeVisible()
   await page.setViewportSize({width:834,height:1000})
   await expect(page.getByRole('button',{name:'Projects: Workshop group',exact:true})).toBeVisible()
  }finally{await w.context.close()}
 })
 // A9: archiving from the narrow detail's menu returns to the list, clears the address and focuses the list's first item.
 await step('archiving from the narrow detail returns to the list',async()=>{
  const {f,w,page}=await open(834,{tweak:f=>{
   twoProjects(f)
   for(const piece of f.outcome.sessions){piece.state='done';piece.note='Every stage is done.';piece.stages=piece.stages.map(stage=>({...stage,state:'done'}));piece.stage=2}
   f.routes[`/api/v2/gaffer/outcomes/${oid}/archive`]=()=>{f.outcome.archivedAt=now;f.outcome.revision++;return f.outcome}
  }})
  try{
   await page.getByRole('button',{name:/^Working/}).click()
   await page.getByRole('button',{name:/^Finished or stopped/}).click()
   await page.locator(`#pipeline-${oid}-a`).getByRole('button',{name:/^Details/}).click()
   await expect(page).toHaveURL(/[?&]pipeline=/)
   await page.getByRole('button',{name:'More for this outcome',exact:true}).click()
   await page.getByRole('button',{name:'Archive',exact:true}).click()
   await expect(page.getByRole('region',{name:'Working',exact:true})).toBeVisible()
   expect(new URL(page.url()).searchParams.get('pipeline')).toBeNull()
   await expect(page.getByRole('button',{name:'Details and other actions for Rail tidy',exact:true})).toBeFocused()
  }finally{await w.context.close()}
 })
 // A10: on a phone every button of the first waiting card, and the Projects button, is a full-size target.
 await step('touch sizes on a phone',async()=>{
  const {w,page}=await open(390)
  try{
   const card=page.getByRole('region',{name:'Waiting for you',exact:true}).locator('.item').first()
   await expect(card.getByRole('button',{name:/^Details/})).toBeVisible()
   const heights=await card.getByRole('button').evaluateAll(l=>l.map(b=>b.getBoundingClientRect().height))
   expect(heights.length).toBeGreaterThanOrEqual(2);for(const h of heights)expect(h).toBeGreaterThanOrEqual(44)
   expect((await page.getByRole('button',{name:'Projects: all',exact:true}).boundingBox()).height).toBeGreaterThanOrEqual(44)
  }finally{await w.context.close()}
 })
 // F7: the floating Gaffer button covers no text, control, item or stage card where the captures are taken, and on a
 // narrow detail the bar rests on the foot of the screen (on the tab bar of a phone) with no band beneath it.
 for(const [width,height] of [[1440,1000],[834,1112],[390,844]]){
  await step(`the floating button covers nothing at ${width} px`,async()=>{
   const {w,page}=await open(width,{height})
   try{
    if(width<1100){
     await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
     await settled(page);expect(await covered(page)).toEqual([])
     await page.locator(`#pipeline-${oid}-a`).getByRole('button',{name:/^Details/}).click()
    }
    const bar=page.getByRole('group',{name:'What do you want to do?',exact:true})
    await expect(bar).toBeVisible();await settled(page)
    expect(await covered(page)).toEqual([])
    if(width<1100){
     const foot=()=>page.evaluate(()=>{const bar=document.querySelector('.detail .bar').getBoundingClientRect(),tabs=document.querySelector('nav.bottom-tabs');const floor=tabs&&tabs.getClientRects().length?tabs.getBoundingClientRect().top:innerHeight;return Math.round(floor-bar.bottom)})
     expect(Math.abs(await foot())).toBeLessThanOrEqual(1)
     await page.evaluate(()=>scrollTo(0,document.scrollingElement.scrollHeight));await settled(page)
     expect(Math.abs(await foot())).toBeLessThanOrEqual(1);expect(await covered(page)).toEqual([])
    }
    await page.screenshot({path:`${shots}/launcher-${width}.png`})
   }finally{await w.context.close()}
  })
 }
 // The five captures kept in docs/images/ui, at twice the pixel density and as the person sees the screen (not full page,
 // so the fixed bars sit where they are). Saved only when REET_UI_SCREENSHOTS names a folder.
 if(process.env.REET_UI_SCREENSHOTS){
  for(const [width,theme] of [[1440,'light'],[1440,'dark']]){
   const {w,page}=await open(width,{theme,scale:2})
   try{
    await expect(page.getByRole('heading',{name:'Export API',level:2,exact:true})).toBeVisible()
    await expect(page.getByRole('group',{name:'What do you want to do?',exact:true})).toBeVisible()
    await settled(page)
    await page.screenshot({path:`${shots}/factory-rework-${width}-${theme}.png`,fullPage:false})
   }finally{await w.context.close()}
  }
  {
   const {w,page}=await open(834,{scale:2,height:1112})
   try{
    const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
    await expect(waiting).toBeVisible()
    await settled(page)
    await page.screenshot({path:`${shots}/factory-rework-834-light-list.png`,fullPage:false})
    await waiting.getByRole('button',{name:'Details and other actions for Export API',exact:true}).click()
    await expect(page.getByRole('group',{name:'What do you want to do?',exact:true})).toBeVisible()
    await settled(page)
    await page.screenshot({path:`${shots}/factory-rework-834-light-detail.png`,fullPage:false})
   }finally{await w.context.close()}
  }
  {
   const {w,page}=await open(390,{theme:'dark',scale:2})
   try{
    await expect(page.getByRole('region',{name:'Waiting for you',exact:true})).toBeVisible()
    await settled(page)
    await page.screenshot({path:`${shots}/factory-rework-390-dark.png`,fullPage:false})
   }finally{await w.context.close()}
  }
  console.log('ok - captures saved')
 }
 if(failures.length)throw new Error(`${failures.length} check(s) failed: ${failures.join('; ')}`)
}finally{await browser.close()}
