import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// A Factory pipeline's decision is shown in the session doing its stage in hand, as the same clickable choices the
// Factory card has, with the recommended one first. A session that is not the stage in hand shows none.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-factory-decision'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const sid='factory-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32),oid='e'.repeat(32),now=Date.now()
function fixture(){
 const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
 const session={id:sid,agentId:'a1',title:'Export API · code',tool:'opencode',model:'working-model',structured:true,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
 const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},box={controls:[],moved:[],told:[],yolo:[]}
 const goal={...pins,id:'1'.repeat(32),conversationId:cid,sessionId:sid,revision:3,text:'Outcome: x',state:'needs_user',note:'Gaffer needs your decision. Read its latest review.',maxReviews:12,usedReviews:3,expiresAt:now+20*3600000,createdAt:now-3600000,updatedAt:now,nextAt:now,lastSeq:42,summary:'The endpoint and its tests are committed.',inputTokens:0,outputTokens:0,usageReported:true,
  advice:{choice:'move',why:'The endpoint and its tests are committed; the review stage will catch the rest.'}}
 const stage=(kind,state)=>({kind,tool:'opencode',model:'',state,requestId:'r',...(state==='running'?{sessionId:sid,conversationId:cid}:{})})
 const outcome={id:oid,revision:5,objective:'Ship CSV export for billing.',state:'settled',note:'',agentId:'a1',projectId:'p1',projectName:'billing',dir:'/work/billing',tool:'opencode',providerId:pid,maxReviews:12,hours:24,createdAt:now-7200000,updatedAt:now,sessions:[
  {key:'a',title:'Export API',prompt:'p',state:'following',sessionState:'running',note:'The code stage is under way.',sessionId:sid,conversationId:cid,updatedAt:now,stage:0,stages:[stage('code','running'),stage('review','pending'),stage('merge','pending')]}]}
 const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],messages:[]}
 const goalView=()=>({goal,pins,allowed:true,setupAllowed:true,providerName:'Workshop Gaffer',conversationRevision:1,requiresProviderAccess:false})
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>view,
  '/api/v2/gaffer/outcomes':()=>({outcomes:[outcome]}),
  '/api/v2/gaffer/goals':()=>({goals:[{goal,session:{id:sid,agentId:'a1',title:session.title,state:session.state},available:true,attention:goal.state!=='active',reason:goal.state!=='active'?'Gaffer brought a question back to you.':''}],readAt:now}),
  [`/api/v2/gaffer/conversations/${cid}/goal`]:()=>goalView(),
  [`/api/v2/gaffer/conversations/${cid}/goal/control`]:request=>{const input=request.postDataJSON();box.controls.push(input);goal.state='active';goal.revision++;delete goal.advice;return goalView()},
  [`/api/v2/gaffer/outcomes/${oid}/sessions/a/advance`]:request=>{box.moved.push(request.postDataJSON());const piece=outcome.sessions[0];piece.stages[0].state='done';piece.stage=1;piece.stages[1].state='running';piece.sessionId='next-session';outcome.revision++;return outcome},
  [`/api/v2/gaffer/outcomes/${oid}/yolo`]:request=>{const body=request.postDataJSON();box.yolo.push(body);outcome.yolo=body.on;outcome.revision++;return {outcome,notSwitched:body.on?['Export API']:[]}},
  [`/api/v2/gaffer/outcomes/${oid}/sessions/a/tell`]:request=>{box.told.push(request.postDataJSON().text);return {message:{id:'m1',state:'queued'}}},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true,inspect:true}}]},
  '/api/v2/projects':{projects:[]},
  '/api/v2/folder-projects':{projects:[],savedWaiting:0},
  '/api/v2/settings/yolo':{allowed:false},
  [`/api/v2/terminal/sessions/${sid}/skills`]:{skills:[],machineSeen:Date.now(),host:'Workshop'},
  [`/api/v2/terminal/sessions/${sid}/inspect`]:{status:503,json:{error:'Machine unavailable'}},
  [`/api/v2/terminal/sessions/${sid}/seen`]:{},
  [`/api/v2/terminal/sessions/${sid}/chat/controls`]:{controls:{working:false,waiting:false,model:'working-model',options:[]}},
  [`/api/v2/terminal/sessions/${sid}/chat/queue`]:{queue:[]},
 }
 return{routes,box,goal,outcome}
}
try{
 for(const [width,theme] of [[1440,'light'],[390,'dark']]){
  const f=fixture(),w=await openWorkspace(browser,{width,height:width<500?844:1000,theme,touch:width<1000,routes:f.routes})
  await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
  try{
   const page=w.page
   await page.goto(`${base}/sessions?agent=a1&session=${sid}`)
   await page.locator('.session-screen header.head').waitFor({state:'visible',timeout:60000})
   const decision=page.getByRole('region',{name:'Decision for Export API',exact:true})
   await expect(decision).toBeVisible()
   await expect(decision).toContainText('Code stage');await expect(decision).toContainText('Gaffer brought a question back to you.')
   // Gaffer's recommendation leads, marked, with its reason. There is nothing to open: this is the session.
   await expect(decision).toContainText('Gaffer recommends: Move to next stage. The endpoint and its tests are committed; the review stage will catch the rest.')
   const choices=decision.getByRole('group',{name:'Choices for Export API',exact:true}).getByRole('button')
   await expect(choices).toHaveText(['Move to next stage Recommended','Have Gaffer look again','Tell the agent…'])
   await expect(decision.getByRole('link',{name:'Export API',exact:true})).toHaveAttribute('href',`/factory#pipeline-${oid}-a`)
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=document.documentElement.clientWidth)).toBe(true)
   await page.screenshot({path:`${shots}/factory-decision-${width}-${theme}.png`})
   // Telling the agent is done here, in the person's own words.
   await decision.getByRole('button',{name:'Tell the agent…',exact:true}).click()
   const note=page.getByRole('dialog',{name:'Tell the agent of “Export API”',exact:true})
   await expect(note.getByRole('textbox',{name:'Instruction',exact:true})).toHaveValue('')
   await note.getByRole('textbox',{name:'Instruction',exact:true}).fill('Add the missing error case.')
   await note.getByRole('button',{name:'Send to the agent',exact:true}).click()
   await expect(page.getByRole('status').filter({hasText:'instruction'})).toHaveText('Your instruction is with the agent for its next turn.');expect(f.box.told).toEqual(['Add the missing error case.'])
   // Moving on asks first, then names the stage the person was shown. The decision then leaves this session with the stage.
   await decision.getByRole('button',{name:'Move to next stage Recommended',exact:true}).click()
   await page.getByRole('dialog',{name:'Move “Export API” on?',exact:true}).getByRole('button',{name:'Move on',exact:true}).click()
   await expect.poll(()=>f.box.moved).toEqual([{confirmed:true,stage:'code'}])
   await expect(decision).toHaveCount(0)
   expect(w.state.unexpected).toEqual([])
  }finally{await w.context.close()}
 }
 // Having Gaffer look again resumes the stage's goal under its current permissions, and the decision goes.
 const f=fixture(),w=await openWorkspace(browser,{width:1280,height:900,routes:f.routes})
 await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
 try{
  await w.page.goto(`${base}/sessions?agent=a1&session=${sid}`)
  const decision=w.page.getByRole('region',{name:'Decision for Export API',exact:true})
  await decision.getByRole('button',{name:'Have Gaffer look again',exact:true}).click()
  await expect.poll(()=>f.box.controls).toEqual([{revision:3,action:'resume',pins:{providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},confirmed:true}])
  await expect(w.page.getByRole('status').filter({hasText:'reviewing'})).toHaveText('Gaffer is reviewing this stage again.')
  await expect(decision).toHaveCount(0)
 }finally{await w.context.close()}
 // A started outcome can be put on YOLO from its card, after a confirmation, and taken off again.
 const y=fixture(),yw=await openWorkspace(browser,{width:1280,height:900,routes:y.routes})
 await yw.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
 try{
  const page=yw.page
  await page.goto(`${base}/factory`)
  await page.getByRole('button',{name:'Put this outcome on YOLO',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:'Put this outcome on YOLO?'})
  await expect(dialog).toContainText('act without asking you')
  expect(y.box.yolo).toEqual([])
  await dialog.getByRole('button',{name:'Switch to YOLO',exact:true}).click()
  await expect.poll(()=>y.box.yolo).toEqual([{revision:5,on:true,confirmed:true}])
  await expect(page.getByRole('status').filter({hasText:'on YOLO'})).toContainText('The session in hand for “Export API” could not be switched')
  await page.getByRole('button',{name:'Make this outcome’s agents ask me',exact:true}).click()
  await expect.poll(()=>y.box.yolo.at(-1)).toEqual({revision:6,on:false})
  await page.screenshot({path:`${shots}/factory-yolo-switch-1280.png`})
 }finally{await yw.context.close()}
 console.log('factory yolo switch: ok')
 console.log('factory decision in a session: ok')
}finally{await browser.close()}
