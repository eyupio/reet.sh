import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// The Factory page's smaller pieces, one labelled block each. Later tasks append their checks below.
// Not yet run or in CI: the stage pipeline has nowhere to render until the page mounts it.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-factory-pieces'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const sid='factory-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32),oid='e'.repeat(32),now=Date.now()
function fixture(){
 const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
 const session={id:sid,agentId:'a1',title:'Export API · code',tool:'opencode',model:'working-model',structured:true,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
 const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1}
 const goal={...pins,id:'1'.repeat(32),conversationId:cid,sessionId:sid,revision:3,text:'Outcome: x',state:'active',note:'',maxReviews:12,usedReviews:3,expiresAt:now+20*3600000,createdAt:now-3600000,updatedAt:now,nextAt:now,lastSeq:42,summary:'',inputTokens:0,outputTokens:0,usageReported:true}
 const stage=(kind,state)=>({kind,tool:'opencode',model:'',state,requestId:'r',...(state==='running'?{sessionId:sid,conversationId:cid}:{})})
 const outcome={id:oid,revision:5,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
  {key:'a',title:'Export API',prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:sid,conversationId:cid,updatedAt:now,stage:0,stages:[stage('code','running'),stage('review','pending'),stage('merge','pending')]}]}
 const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],messages:[]}
 const goalView=()=>({goal,pins,allowed:true,setupAllowed:true,providerName:'Workshop Gaffer',conversationRevision:1,requiresProviderAccess:false})
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>view,
  '/api/v2/gaffer/outcomes':()=>({outcomes:[outcome]}),
  '/api/v2/gaffer/goals':()=>({goals:[{goal,session:{id:sid,agentId:'a1',title:session.title,state:session.state},available:true,attention:false,reason:''}],readAt:now}),
  [`/api/v2/gaffer/conversations/${cid}/goal`]:()=>goalView(),
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true,inspect:true}}]},
  '/api/v2/projects':{projects:[]},
  '/api/v2/folder-projects':{projects:[],savedWaiting:0},
  '/api/v2/settings/yolo':{allowed:false},
 }
 return{routes,outcome}
}
try{
 // The stage pipeline: three stages, the first in hand, and no sideways scroll on a phone.
 for(const [width,theme] of [[1440,'light'],[390,'dark']]){
  const f=fixture(),w=await openWorkspace(browser,{width,height:width<500?844:1000,theme,touch:width<1000,routes:f.routes})
  await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
  try{
   const page=w.page
   await page.goto(`${base}/factory`)
   const stages=page.getByRole('list',{name:'Stages of Export API',exact:true})
   await expect(stages).toBeVisible()
   await expect(stages.getByRole('listitem')).toHaveCount(3)
   await expect(stages.getByRole('listitem').first()).toHaveAttribute('aria-current','step')
   expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
   await page.screenshot({path:`${shots}/stage-pipeline-${width}-${theme}.png`})
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('ok - stage pipeline')
 // A waiting item: its filled action and the way to the detail. Not run until the page mounts WaitingItem (Task 6).
 {
  const f=fixture(),w=await openWorkspace(browser,{width:1000,height:900,theme:'light',touch:false,routes:f.routes})
  try{
   const page=w.page
   await page.goto(`${base}/factory`)
   const waiting=page.getByRole('region',{name:'Waiting for you',exact:true})
   await expect(waiting.getByRole('button',{name:'Move to next stage',exact:true})).toBeVisible()
   await expect(waiting.getByRole('button',{name:/^Details and other actions/})).toBeVisible()
   const working=page.getByRole('region',{name:'Working',exact:true})
   await expect(working.locator('.btn.primary')).toHaveCount(0)
  }finally{await w.context.close()}
  console.log('ok - waiting item')
 }
 // The pipeline detail: one bar of what can be done, the settings in a line, who decides, and the facts of a picked stage.
 // Not run until the page mounts PipelineDetail (Task 6).
 {
  const f=fixture(),posted=[]
  const advised={...f.routes['/api/v2/gaffer/goals']().goals[0]}
  advised.attention=true;advised.reason='Gaffer brought a question back.';advised.goal={...advised.goal,note:'Gaffer brought a question back.',summary:'Tests pass locally.',advice:{choice:'move',why:'The review stage will catch the rest.'}}
  const routes={...f.routes,
   '/api/v2/gaffer/goals':()=>({goals:[advised],readAt:now}),
   [`/api/v2/gaffer/outcomes/${oid}/automation`]:request=>{const input=request.postDataJSON();posted.push(input);f.outcome.automation=input.on;return f.outcome},
  }
  const w=await openWorkspace(browser,{width:1440,height:1000,theme:'light',touch:false,routes})
  await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
  try{
   const page=w.page
   await page.goto(`${base}/factory`)
   await page.getByRole('region',{name:'Waiting for you',exact:true}).getByRole('button',{name:/^Details and other actions/}).click()
   const bar=page.getByRole('group',{name:'What do you want to do?',exact:true})
   await expect(bar.getByRole('button').first()).toHaveAccessibleName(/^Move to next stage/)
   await expect(bar.getByRole('button',{name:'Tell the agent…',exact:true})).toHaveCount(1)
   await expect(bar.getByRole('button',{name:/^Move to next stage/})).toHaveCount(1)
   await expect(page.getByText('You decide · Ask me · merge waits for you',{exact:true})).toBeVisible()
   await page.getByText('Settings for this outcome',{exact:true}).click()
   await page.getByRole('radio',{name:'Gaffer decides',exact:true}).check()
   await expect.poll(()=>posted).toEqual([{revision:5,on:true}])
   await page.getByRole('list',{name:'Choose a stage of Export API',exact:true}).getByRole('button',{name:/Review/}).click()
   await expect(page.getByRole('region',{name:'Review stage',exact:true})).toBeVisible()
   await expect(page.getByRole('region',{name:'Review stage',exact:true})).toContainText('will do this stage')
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
  console.log('ok - pipeline detail')
 }
}finally{await browser.close()}
