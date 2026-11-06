import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',captures=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-worker'
await mkdir(captures,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),sid='worker-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32)
function fixture(){
 const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:true,allowCode:false,hasKey:true,createdAt:now,updatedAt:now}
 const session={id:sid,agentId:'a1',title:'Prepare the release',tool:'opencode',model:'working-model',structured:true,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
 const grant={sessionId:sid,enabled:false,revision:0},view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],messages:[]}
 const box={posts:[],saves:[],interrupts:[],workerCount:0}
 const worker=()=>({worker:{grant:{...grant},allowed:true,needsReview:false,providerId:pid,providerRevision:1,model:'gaffer-model'},messages:view.messages})
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>view,
  [`/api/v2/gaffer/conversations/${cid}/worker`]:request=>{if(request.method()==='PUT'){const input=request.postDataJSON();expect(input.revision).toBe(grant.revision);box.saves.push(input);grant.enabled=input.enabled;grant.revision++;view.conversation.revision++;if(!input.enabled)for(const m of view.messages)if(m.state==='queued'){m.state='removed';m.revision++}}return worker()},
  [`/api/v2/gaffer/conversations/${cid}/turns`]:request=>{
   const input=request.postDataJSON();expect(input.revision).toBe(view.conversation.revision);box.posts.push(input)
   const turn={id:input.id,conversationId:cid,requestRevision:input.revision,prompt:input.text,answer:input.relay?'The instruction is queued for the working model. Acceptance is not completion.':'Here is the status from this conversation.',state:'completed',providerId:pid,providerName:provider.name,model:provider.model,usageReported:false,cut:false,startedAt:now,updatedAt:now,finishedAt:now,...(input.relay?{workerGrantRevision:grant.revision}:{})}
   if(input.relay){expect(grant.enabled).toBe(true);box.workerCount++;const m={id:input.id,sessionId:sid,queueId:'q'.repeat(32),turnId:input.id,text:'Raise a PR, fix the checks and do not merge. <img src=x onerror="window.__workerXss=true">',state:'queued',note:'Waiting for the next worker turn.',revision:1,grantRevision:grant.revision,createdAt:now};view.messages.push(m)
    routes[`/api/v2/gaffer/conversations/${cid}/worker/messages/${input.id}/interrupt`]=request=>{const body=request.postDataJSON();expect(body.confirm).toBe(true);expect(body.revision).toBe(m.revision);box.interrupts.push(body);m.state='sent';m.revision++;return {message:m}}
   }
   view.turns.push(turn);view.conversation.revision+=2;return {status:202,json:turn}
  },
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true}}]},
  '/api/v2/projects':{projects:[]},
  [`/api/v2/terminal/sessions/${sid}/seen`]:{},
  [`/api/v2/terminal/sessions/${sid}/chat/controls`]:{controls:{working:false,waiting:false,model:'working-model',options:[]}},
  [`/api/v2/terminal/sessions/${sid}/chat/queue`]:{queue:[]},
 }
 return {routes,box,view,grant}
}
try{
 for(const theme of ['light','dark'])for(const width of [1440,800,390,320]){
  const f=fixture(),w=await openWorkspace(browser,{width,height:900,theme,touch:width<1000,routes:f.routes})
  await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
  try{
   await w.page.goto(`${base}/sessions?agent=a1&session=${sid}`);await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
   const chat=w.page.getByRole('dialog',{name:'Gaffer',exact:true}),dialog=w.page.getByRole('dialog',{name:'Worker messages',exact:true})
   await expect(chat.locator('.scope-title')).toHaveText('Prepare the release')
   await chat.getByRole('button',{name:'Chat options',exact:true}).click();await w.page.getByRole('dialog',{name:'Chat options',exact:true}).getByRole('button',{name:'Worker messages',exact:true}).click()
   await expect(dialog.getByLabel('Allow worker messages',{exact:true})).not.toBeChecked()
   await expect(dialog.getByRole('button',{name:'Enable one worker message for next request',exact:true})).toHaveCount(0)
   await dialog.getByLabel('Allow worker messages',{exact:true}).check();await dialog.getByRole('button',{name:'Save worker permission',exact:true}).click()
   await expect(dialog.getByRole('button',{name:'Enable one worker message for next request',exact:true})).toBeEnabled()
   expect(f.box.workerCount).toBe(0)
   await dialog.getByRole('button',{name:'Enable one worker message for next request',exact:true}).click()
   await expect(chat.getByText('One worker message enabled for this request.')).toBeVisible()
   await chat.getByRole('textbox',{name:'Message Gaffer',exact:true}).fill('Ask the working model to raise the PR and fix checks. Do not merge.')
   await chat.getByRole('button',{name:'Send message',exact:true}).click()
   await expect(chat.getByText('Worker instruction · queued',{exact:true})).toBeVisible();expect(f.box.posts[0].relay).toBe(true);expect(f.box.workerCount).toBe(1)
   await expect(chat.getByText('One worker message enabled for this request.')).toHaveCount(0)
   await chat.getByRole('textbox',{name:'Message Gaffer',exact:true}).fill('A normal status question')
   await chat.getByRole('button',{name:'Send message',exact:true}).click()
   await expect(chat.getByText('Here is the status from this conversation.',{exact:true})).toBeVisible();expect(f.box.posts[1].relay).toBeUndefined();expect(f.box.workerCount).toBe(1)
   await chat.getByRole('button',{name:'Chat options',exact:true}).click();await w.page.getByRole('dialog',{name:'Chat options',exact:true}).getByRole('button',{name:'Worker messages',exact:true}).click()
   await expect(dialog.getByText('Queued for next turn',{exact:true})).toBeVisible()
   expect(await w.page.evaluate(()=>window.__workerXss)).toBeUndefined();await expect(dialog.locator('img[src="x"]')).toHaveCount(0)
   expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
   if(width===1440||width===390)await dialog.screenshot({path:`${captures}/gaffer-worker-${width}-${theme}.png`})
   await dialog.getByRole('button',{name:'Stop worker and send',exact:true}).click()
   const confirm=w.page.getByRole('dialog',{name:'Stop the worker and send this instruction?',exact:true})
   expect(f.box.interrupts).toHaveLength(0);await confirm.getByRole('button',{name:'Stop worker and send',exact:true}).click()
   await expect(dialog.getByText('Accepted by worker',{exact:true})).toBeVisible();expect(f.box.interrupts).toHaveLength(1)
   await dialog.getByLabel('Allow worker messages',{exact:true}).uncheck();await dialog.getByRole('button',{name:'Save worker permission',exact:true}).click()
   await expect(dialog.getByRole('button',{name:'Enable one worker message for next request',exact:true})).toHaveCount(0)
   await w.page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await expect(chat.getByRole('button',{name:'Chat options',exact:true})).toBeFocused()
   expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 for(const width of [1440,390,320]){
  const grants=new Map(),saves=[],settings=(kind,request)=>{
   const session=new URL(request.url()).searchParams.get('sessionId')||'',key=`${kind}:${session}`
   let grant=grants.get(key)||{sessionId:session,enabled:false,revision:0}
   if(request.method()==='PUT'){const body=request.postDataJSON();expect(body.revision).toBe(grant.revision);grant={sessionId:session,enabled:body.enabled,revision:grant.revision+1};grants.set(key,grant);saves.push(key)}
   return {worker:{grant,allowed:!!session,needsReview:false},messages:[],sessions:[{id:sid,title:'Release session'}],scopes:[]}
  }
  const w=await openWorkspace(browser,{width,height:900,touch:width<1000,routes:{
   '/api/v2/mcp':{enabled:true,browserEnabled:true,browserScope:'write',canWrite:true,endpoint:'https://example.test/mcp',connections:[{id:1,name:'Desktop client',scope:'write',enabled:true,kind:'bearer',createdAt:now,expiresAt:now+86400000,lastUsedAt:0}]},
   '/api/v2/mcp/connections/1/worker':request=>settings('client',request),
   '/api/v2/mcp/browser/worker':request=>settings('browser',request),
  }})
  try{
   await w.page.goto(`${base}/account#mcp`);await w.page.getByRole('button',{name:'Worker access',exact:true}).click()
   const client=w.page.getByRole('dialog',{name:'Worker access for Desktop client',exact:true})
   await client.getByRole('combobox',{name:'Worker session',exact:true}).selectOption(sid)
   await expect(client.getByLabel('Allow worker messages',{exact:true})).not.toBeChecked();await client.getByLabel('Allow worker messages',{exact:true}).check();await client.getByRole('button',{name:'Save worker permission',exact:true}).click()
   await expect(client.getByText('Worker permission saved. No instruction has been sent.',{exact:true})).toBeVisible()
   expect(await client.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
   await w.page.keyboard.press('Escape');await w.page.getByRole('button',{name:'Browser worker access',exact:true}).click()
   const browserGrant=w.page.getByRole('dialog',{name:'Worker access for Browser WebMCP',exact:true})
   await browserGrant.getByRole('combobox',{name:'Worker session',exact:true}).selectOption(sid)
   await expect(browserGrant.getByLabel('Allow worker messages',{exact:true})).not.toBeChecked();expect(saves).toEqual([`client:${sid}`])
   await browserGrant.getByLabel('Allow worker messages',{exact:true}).check();await browserGrant.getByRole('button',{name:'Save worker permission',exact:true}).click()
   await expect(browserGrant.getByText('Worker permission saved. No instruction has been sent.',{exact:true})).toBeVisible();expect(saves).toEqual([`client:${sid}`,`browser:${sid}`]);expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('Gaffer worker messaging: explicit grant and per-request opt-in, fresh revision, one relay, ordinary chat stays read-only, escaped receipts, explicit interrupt, focus return, independent MCP/browser grants and responsive/theme views passed.')
}finally{await browser.close()}
