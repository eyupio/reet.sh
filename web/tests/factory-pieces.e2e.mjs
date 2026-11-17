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
}finally{await browser.close()}
