import { chromium,expect } from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',captures=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-ui-gaffer-context'
await mkdir(captures,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),cid='c'.repeat(32),sid='context-session-alpha'
function fixture(){
 const p={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:true,allowSessions:true,allowCode:false,hasKey:true,createdAt:now,updatedAt:now}
 const session={id:sid,agentId:'a1',title:'Review the release',tool:'opencode',model:'worker-model',structured:true,state:'running',createdAt:now-60000,updatedAt:now,exitCode:null,dir:'/work'}
 const caps={protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true,acpHistory:true}
 const box={grants:new Map(),saves:[],previews:[],messages:[],workerWrites:0,failSave:false}
 const key=(kind,scope)=>`${kind}:${scope}`
 const grant=(kind,scope)=>box.grants.get(key(kind,scope))||{sessionId:scope,revision:0,status:false,history:false,text:false,updatedAt:0}
 function state(kind,scope){const g=grant(kind,scope);return {grant:g,allowed:{status:true,history:!!scope,text:!!scope&&(kind!=='gaffer'||p.allowCode)},needsReview:kind==='gaffer'&&(g.status||g.history||g.text)&&g.providerRevision!==p.revision,providerId:kind==='gaffer'?p.id:undefined,providerName:kind==='gaffer'?p.name:undefined,providerRevision:kind==='gaffer'?p.revision:undefined,model:kind==='gaffer'?p.model:undefined}}
 const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],context:state('gaffer',sid)}
 function snapshot(scope,flags){return {version:1,sessionId:scope,readAt:now,source:'controller_retained_not_live',shared:flags,notRead:['raw_tool_arguments_and_output','reasoning','approval_prompts'],...(scope?{session:{id:sid,...(flags.status?{state:'running',worker:'opencode',updatedAt:now-12000}:{}),...(flags.text?{title:'WORKER PRIVATE TITLE'}:{})},...(flags.history?{history:{firstRetainedSeq:5,lastRetainedSeq:9,retainedCount:5,unavailable:false,truncated:true,events:[{seq:9,at:now-5000,kind:'agent',...(flags.text?{text:'UNTRUSTED <img src=x onerror="window.__contextXss=true">'}:{})}]}}:{})}:{fleet:{machines:3,sessions:{running:2,ended:1},lastReportedAt:now-12000}})}}
 function settings(kind,scope){return {context:state(kind,scope),...(kind!=='gaffer'?{sessions:[{id:sid,title:session.title}],scopes:[...box.grants.entries()].filter(([k,g])=>k.startsWith(kind+':')&&(g.status||g.history||g.text)).map(([,g])=>g)}:{})}}
 function contextRoute(kind,scope,preview=false){return request=>{
  if(preview){const flags=request.postDataJSON();box.previews.push({kind,scope,flags});return {context:snapshot(scope,flags)}}
  if(request.method()==='PUT'){
   const input=request.postDataJSON();box.saves.push({kind,scope,input})
   if(box.failSave){box.failSave=false;return {status:409,json:{error:'Sharing changed in another tab. Reload it.'}}}
   if(input.revision!==grant(kind,scope).revision)return {status:409,json:{error:'Stale sharing revision.'}}
   if(kind==='gaffer'&&(input.status||input.history||input.text))expect(input.providerId).toBe(pid)
   box.grants.set(key(kind,scope),{...input,sessionId:scope,revision:input.revision+1,updatedAt:now,...(kind==='gaffer'?{providerId:p.id,providerRevision:p.revision,model:p.model}:{})})
   if(kind==='gaffer'){view.conversation.revision++;view.context=state(kind,scope)}
  }
  return settings(kind,scope)
 }}
 const routes={
  '/api/v2/gaffer/providers':{providers:[p],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>({...view,context:state('gaffer',sid)}),
  [`/api/v2/gaffer/conversations/${cid}/context`]:contextRoute('gaffer',sid),
  [`/api/v2/gaffer/conversations/${cid}/context/preview`]:contextRoute('gaffer',sid,true),
  [`/api/v2/gaffer/conversations/${cid}/turns`]:request=>{const input=request.postDataJSON();box.messages.push(input);const g=grant('gaffer',sid);const turn={id:input.id,conversationId:cid,requestRevision:input.revision,prompt:input.text,answer:'The retained events show progress; the latest PR checks were not read.',state:'completed',providerId:pid,providerName:p.name,model:p.model,inputTokens:0,outputTokens:0,usageReported:false,cut:false,startedAt:now,updatedAt:now,finishedAt:now,context:{version:1,...g,grantRevision:g.revision,readAt:now,lastSeq:9,truncated:true,digest:'d'.repeat(64)}};view.turns.push(turn);view.conversation.revision+=2;return {status:202,json:turn}},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:caps}]},
  '/api/v2/projects':{projects:[]},
  [`/api/v2/terminal/sessions/${sid}/seen`]:{},
  [`/api/v2/terminal/sessions/${sid}/chat/controls`]:{controls:{working:false,waiting:false,model:'worker-model',options:[]}},
  [`/api/v2/terminal/sessions/${sid}/chat/queue`]:request=>{if(request.method()!=='GET')box.workerWrites++;return {queue:[]}},
  '/api/v2/mcp':{enabled:true,browserEnabled:true,browserScope:'read',canWrite:true,endpoint:'https://example.test/mcp',connections:[{id:7,name:'Desktop client',kind:'bearer',scope:'read',enabled:true,createdAt:now,expiresAt:now+86400000,lastUsedAt:0}]},
 }
 for(const kind of ['mcp','browser']){const prefix=kind==='mcp'?'/api/v2/mcp/connections/7/context':'/api/v2/mcp/browser/context';routes[prefix]=request=>contextRoute(kind,new URL(request.url()).searchParams.get('sessionId')||'')(request);routes[prefix+'/preview']=request=>contextRoute(kind,new URL(request.url()).searchParams.get('sessionId')||'',true)(request)}
 return {box,p,view,routes}
}
try{
 for(const theme of ['light','dark'])for(const width of [1440,390,320]){
  const f=fixture(),w=await openWorkspace(browser,{width,height:900,theme,touch:width<650,routes:f.routes})
  await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
  try{
   await w.page.goto(`${base}/sessions?agent=a1&session=${sid}`)
   await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
   const chat=w.page.getByRole('dialog',{name:'Gaffer',exact:true})
   const access=async()=>{await chat.getByRole('button',{name:'Chat options',exact:true}).click();const options=w.page.getByRole('dialog',{name:'Chat options',exact:true});await options.getByText('Advanced session controls',{exact:true}).click();return options.getByRole('button',{name:'Context sharing',exact:true})}
   let accessButton=await access()
   await expect(accessButton).toHaveAttribute('title','Context off · typed messages only')
   await expect(accessButton).toHaveAccessibleDescription('Context off · typed messages only')
   await accessButton.click()
   const dialog=w.page.getByRole('dialog',{name:'What Gaffer can read',exact:true})
   await expect(dialog.getByText('Workshop Gaffer',{exact:true})).toBeVisible()
   await expect(dialog.getByLabel('Share status',{exact:true})).not.toBeChecked()
   await expect(dialog.getByLabel('Share message text and code',{exact:true})).toBeDisabled()
   await dialog.getByLabel('Share status',{exact:true}).check();await dialog.getByLabel('Share event summaries',{exact:true}).check()
   await expect(dialog.getByRole('button',{name:'Enable selected sharing',exact:true})).toBeDisabled()
   await dialog.getByRole('button',{name:'Preview shared data',exact:true}).click()
   await expect(dialog.getByRole('region',{name:'Shared context preview'})).toBeVisible()
   expect(await dialog.innerText()).not.toContain('WORKER PRIVATE TITLE')
   if(width!==320)await dialog.screenshot({path:`${captures}/gaffer-context-${width}-${theme}.png`})
   await dialog.getByRole('button',{name:'Enable selected sharing',exact:true}).click()
   await expect(dialog).toHaveCount(0);accessButton=await access();await expect(accessButton).toHaveAttribute('title','Selected context shared');await w.page.keyboard.press('Escape')
   await chat.getByRole('textbox',{name:'Message Gaffer',exact:true}).fill('What has happened so far?')
   await chat.getByRole('button',{name:'Send message',exact:true}).click()
   await chat.getByText('Reply details',{exact:true}).click()
   await expect(chat.getByText(/Retained context · read/)).toBeVisible()
   expect(f.box.messages[0].revision).toBe(2)
   expect(f.box.workerWrites).toBe(0)
   accessButton=await access();await accessButton.click()
   await dialog.getByRole('button',{name:'Stop sharing',exact:true}).click()
   await expect(dialog).toHaveCount(0);accessButton=await access();await expect(accessButton).toHaveAttribute('title','Context off · typed messages only');await w.page.keyboard.press('Escape')
   // Each MCP client and browser subject has separate, explicit grants.
   await chat.getByRole('button',{name:'Close Gaffer',exact:true}).click()
   await w.page.goto(`${base}/account#mcp`)
   await w.page.getByRole('button',{name:'Context access',exact:true}).click()
   const mcp=w.page.getByRole('dialog',{name:'Context shared with Desktop client',exact:true})
   await mcp.getByRole('combobox',{name:'Context scope',exact:true}).selectOption(sid)
   await mcp.getByLabel('Share event summaries',{exact:true}).check();await mcp.getByLabel('Share message text and code',{exact:true}).check()
   await mcp.getByRole('button',{name:'Preview shared data',exact:true}).click()
   await expect(mcp.getByText('WORKER PRIVATE TITLE',{exact:true})).toBeVisible()
   expect(await w.page.evaluate(()=>window.__contextXss)).toBeUndefined()
   await expect(mcp.locator('img[src="x"]')).toHaveCount(0)
   await mcp.getByRole('button',{name:'Enable selected sharing',exact:true}).click()
   await expect(mcp.getByText('Sharing saved. Reet, no guessing what goes where.')).toBeVisible()
   expect(f.box.grants.get(`gaffer:${sid}`).text).toBe(false)
   expect(f.box.grants.get(`mcp:${sid}`).text).toBe(true)
   await w.page.keyboard.press('Escape');await expect(mcp).toHaveCount(0)
   expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('Gaffer context: explicit preview/grants, provider policy, revision refresh, receipts, revocation, separate MCP subjects, escaped text and six responsive/theme views passed.')
}finally{await browser.close()}
