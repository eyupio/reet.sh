import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const captures=process.env.REET_UI_SCREENSHOTS||'../work/gaffer/widget-screenshots'
await mkdir(captures,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(), pid='a'.repeat(32), otherPid='b'.repeat(32)
const provider=(id,name,model,isDefault=false)=>({id,revision:1,name,kind:'openai_compatible',baseUrl:'https://example.test/v1',model,reasoningEffort:'',enabled:true,isDefault,localOnly:false,allowStatus:false,allowSessions:false,allowCode:false,hasKey:true,createdAt:now,updatedAt:now})
const providers=[provider(pid,'My OpenCode','glm-5',true),provider(otherPid,'Workshop Ollama','qwen3:14b')]
const session=(id,title,structured=true)=>({id,agentId:'a1',title,structured,model:'worker-model',tool:'opencode',state:'running',createdAt:now-60000,updatedAt:now,exitCode:null,dir:'/work'})
const sessions=[session('session-gaffer-alpha','First session'),session('session-gaffer-bravo','Second session'),session('session-gaffer-plain','Plain terminal',false)]
const caps={protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpHistory:true,acpLiveControls:true,acpStartOptions:true}
const machine={agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:caps}
function fixture({seed=false,noProviders=false}={}){
 const box={conversations:new Map(),posts:[],creates:[],patches:[],deletes:[],cancel:[],models:[],providerReads:0,failModels:false,failSend:false,dropAck:false,lateScope:null,releaseScope:null}
 let next=1
 function create(scope='',providerId=pid,model=''){
  const view={conversation:{id:(next++).toString(16).padStart(32,'0'),sessionId:scope,providerId,model,revision:1,createdAt:now,updatedAt:now},turns:[]};box.conversations.set(scope,view);return view
 }
 function find(id){return [...box.conversations.values()].find(v=>v.conversation.id===id)}
 const settings={providers:noProviders?[]:providers,presets:[],available:true,canUseLocal:true,maxProviders:12}
 const routes={
  '/api/v2/gaffer/providers':()=>{box.providerReads++;return settings},
  '/api/v2/gaffer/providers/models':request=>{box.models.push(request.postDataJSON());return box.failModels?{status:502,json:{error:'Model discovery is unavailable.'}}:{models:['model-beta','model-gamma']}},
  '/api/v2/gaffer/conversations':async request=>{
   if(request.method()==='POST'){const input=request.postDataJSON();box.creates.push(input);return box.conversations.get(input.sessionId)||create(input.sessionId,input.providerId,input.model)}
   if(new URL(request.url()).searchParams.get('all')==='true')return {conversations:[...box.conversations.values()].map(v=>v.conversation)}
   const scope=new URL(request.url()).searchParams.get('sessionId')||''
   if(scope===box.lateScope)await new Promise(resolve=>{box.releaseScope=resolve})
   return box.conversations.get(scope)||{status:404,json:{error:'Conversation unavailable'}}
  },
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions},
  '/api/v2/terminal/machines':{machines:[machine]},
  '/api/v2/machines/health':{serverNow:now,machines:[{...machine,state:'connected',connection:{connected:true,reportFresh:true,contactAt:now,reportAt:now,missedProbes:0,reconnects:0,path:'direct'}}]},
  '/api/v2/yolo':{allowed:false},
  '/api/v2/projects':{projects:[]},
 }
 for(const s of sessions){
  routes[`/api/v2/terminal/sessions/${s.id}/seen`]={}
  routes[`/api/v2/terminal/sessions/${s.id}/skills`]={skills:[],machineSeen:now,host:'Workshop'}
  routes[`/api/v2/terminal/sessions/${s.id}/chat/controls`]={controls:{working:false,waiting:false,compacting:false,model:'worker-model',options:[]}}
  routes[`/api/v2/terminal/sessions/${s.id}/chat/queue`]={queue:[]}
 }
 if(seed){const view=create();view.conversation.revision=3;view.turns.push({id:'c'.repeat(32),conversationId:view.conversation.id,requestRevision:1,prompt:'Help me pressure-test the plan before I start.',answer:'Reet, let’s give it a proper look.\n\nStart with the smallest change you can verify. What result would tell you it worked, and which assumption needs testing first?',state:'completed',providerId:pid,providerName:'My OpenCode',model:'glm-5',inputTokens:82,outputTokens:54,usageReported:true,cut:false,startedAt:now-12000,updatedAt:now,finishedAt:now})}
 return {box,routes,create,find,settings}
}
async function open(options={}){
 const f=fixture(options),w=await openWorkspace(browser,{width:options.width||1440,height:options.height||1000,theme:options.theme||'light',touch:(options.width||1440)<650,routes:f.routes,status:options.status})
 await w.page.route('**/api/v2/gaffer/conversations/**',async route=>{
  const path=new URL(route.request().url()).pathname, parts=path.split('/'),id=parts[5],view=f.find(id),r=route.request()
  if(!view)return route.fulfill({status:404,json:{error:'Conversation unavailable'}})
  if(parts[6]==='turns'&&parts.length===7){
   const input=r.postDataJSON();f.box.posts.push({...input,conversation:id})
   if(f.box.failSend){f.box.failSend=false;return route.abort('failed')}
   let turn=view.turns.find(t=>t.id===input.id)
   if(!turn){turn={id:input.id,conversationId:id,requestRevision:input.revision,prompt:input.text,answer:'',state:'running',providerId:view.conversation.providerId,providerName:providers.find(p=>p.id===view.conversation.providerId).name,model:view.conversation.model||providers.find(p=>p.id===view.conversation.providerId).model,inputTokens:0,outputTokens:0,usageReported:false,cut:false,startedAt:now,updatedAt:now,finishedAt:0};view.turns.push(turn);view.conversation.revision++}
   if(f.box.dropAck){f.box.dropAck=false;return route.abort('failed')}
   return route.fulfill({status:202,json:turn})
  }
  if(parts[8]==='cancel'){f.box.cancel.push(parts[7]);return route.fulfill({json:{requested:true}})}
  if(r.method()==='PATCH'){const input=r.postDataJSON();f.box.patches.push(input);view.conversation={...view.conversation,providerId:input.providerId,model:input.model,revision:view.conversation.revision+1};return route.fulfill({json:view})}
  if(r.method()==='DELETE'){f.box.deletes.push(id);f.box.conversations.delete(view.conversation.sessionId);return route.fulfill({json:{removed:true}})}
  return route.fulfill({json:view})
 })
 if(!options.native)await w.page.addInitScript(()=>{
  class Stream {
   constructor(url){this.url=String(url);this.listeners={};this.closed=false;(window.__gafferStreams??=[]).push(this);if(this.url.includes('/gaffer/'))window.__gafferStream=this;else setTimeout(()=>{this.emit('reset',{truncated:false});this.emit('chat',{seq:1,kind:'agent',messageId:'worker',text:'PRIVATE WORKER TRANSCRIPT'});this.emit('chat',{seq:2,kind:'status',state:'idle'})},0)}
   addEventListener(type,callback){(this.listeners[type]??=[]).push(callback)}
   emit(type,data){if(!this.closed)for(const callback of this.listeners[type]??[])callback({data:JSON.stringify(data)})}
   close(){this.closed=true}
  }
  window.EventSource=Stream
 })
 return {...w,...f}
}
const panel=page=>page.getByRole('dialog',{name:'Gaffer',exact:true})
async function show(w,path='/'){
 await w.page.goto(base+path);await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click();await expect(panel(w.page)).toBeVisible();await expect(panel(w.page).getByText('Finding our place…')).toHaveCount(0)
}
async function finish(w,state='completed',answer='Reet, here’s a thought worth checking.'){
 const view=[...w.box.conversations.values()].find(v=>v.turns.some(t=>t.state==='running'))
 const turn=view.turns.find(t=>t.state==='running');Object.assign(turn,{state,answer,updatedAt:now+1000,finishedAt:now+1000,error:state==='cancelled'?'You cancelled this Gaffer answer.':undefined});view.conversation.revision++
 await w.page.evaluate(t=>window.__gafferStream.emit('turn',t),turn)
 await expect(panel(w.page).getByText(answer,{exact:true})).toBeVisible()
}
try{
 const w=await open();await w.page.goto(base+'/')
 expect(w.box.posts).toHaveLength(0);expect(w.box.creates).toHaveLength(0);expect(w.box.providerReads).toBe(0)
 await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
 await expect(panel(w.page).getByRole('heading',{name:'Reet. What’s on your mind?'})).toBeVisible()
 // A desktop companion stays modeless so the workspace can still be used.
 expect(await w.page.evaluate(()=>document.querySelector('.gaffer-chat').matches(':modal'))).toBe(false)
 await panel(w.page).getByLabel('Message Gaffer').fill('Think this through with me')
 await panel(w.page).getByRole('button',{name:'Send message',exact:true}).click()
 await expect(panel(w.page).getByRole('button',{name:'Cancel answer',exact:true})).toBeVisible()
 expect(w.box.posts).toHaveLength(1)
 await panel(w.page).getByRole('button',{name:'Close Gaffer',exact:true}).click()
 expect(w.box.cancel).toHaveLength(0)
 await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
 await expect(panel(w.page).getByRole('button',{name:'Cancel answer',exact:true})).toBeVisible();expect(w.box.posts).toHaveLength(1)
 await finish(w)
 await expect(panel(w.page).getByRole('button',{name:'Send message',exact:true})).toBeDisabled()
 await panel(w.page).getByRole('button',{name:'Model',exact:true}).click()
 await panel(w.page).getByLabel('Gaffer provider',{exact:true}).selectOption(otherPid)
 await panel(w.page).getByRole('button',{name:'Fetch models',exact:true}).click()
 await expect(panel(w.page).getByLabel('Gaffer model',{exact:true}).getByRole('option',{name:'model-beta',exact:true})).toHaveCount(1)
 await panel(w.page).getByLabel('Gaffer model',{exact:true}).selectOption('model-beta')
 await panel(w.page).getByRole('button',{name:'Apply choice',exact:true}).click()
 await expect(panel(w.page).getByText('Choice of brains saved.',{exact:true})).toBeVisible()
 expect(w.box.patches[0].model).toBe('model-beta');expect(w.box.models[0].id).toBe(otherPid)
 await panel(w.page).getByLabel('Message Gaffer').fill('Another question')
 await panel(w.page).getByRole('button',{name:'Send message',exact:true}).click()
 await panel(w.page).getByRole('button',{name:'Cancel answer',exact:true}).click()
 await expect.poll(()=>w.box.cancel.length).toBe(1);await finish(w,'cancelled','A partial thought before cancellation.')
 await panel(w.page).getByRole('button',{name:'Remove conversation',exact:true}).click()
 await w.page.getByRole('button',{name:'Remove conversation',exact:true}).last().click()
 await expect(panel(w.page).getByText('Conversation removed. Fresh brew, fresh start.')).toBeVisible()
 expect(w.errors).toEqual([]);await w.context.close()

 const uncertain=await open();uncertain.box.failSend=true;await show(uncertain)
 await panel(uncertain.page).getByLabel('Message Gaffer').fill('Private draft stays in memory')
 await panel(uncertain.page).getByRole('button',{name:'Send message',exact:true}).click()
 await expect(panel(uncertain.page).getByText('Message delivery is unconfirmed.')).toBeVisible()
 await expect(panel(uncertain.page).getByLabel('Message Gaffer')).toHaveValue('Private draft stays in memory')
 const stored=await uncertain.page.evaluate(()=>Object.entries(sessionStorage).filter(([k])=>k.startsWith('reet-gaffer-pending:')).map(([,v])=>v))
 expect(stored.join('')).not.toContain('Private draft')
 await panel(uncertain.page).getByRole('button',{name:'Send same message safely'}).click()
 await expect(panel(uncertain.page).getByRole('button',{name:'Cancel answer',exact:true})).toBeVisible()
 expect(uncertain.box.posts).toHaveLength(2);expect(uncertain.box.posts[1].id).toBe(uncertain.box.posts[0].id)
 await finish(uncertain);expect(uncertain.errors).toEqual([]);await uncertain.context.close()

 const lost=await open();lost.box.dropAck=true;await show(lost)
 await panel(lost.page).getByLabel('Message Gaffer').fill('Already accepted by the controller')
 await panel(lost.page).getByRole('button',{name:'Send message',exact:true}).click()
 await expect(panel(lost.page).getByText('Message delivery is unconfirmed.')).toBeVisible()
 await lost.page.reload();await lost.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
 await expect(panel(lost.page).getByRole('button',{name:'Cancel answer',exact:true})).toBeVisible()
 await expect(panel(lost.page).getByText('Message delivery is unconfirmed.')).toHaveCount(0)
 expect(lost.box.posts).toHaveLength(1);await finish(lost);expect(lost.errors).toEqual([]);await lost.context.close()

 const scoped=await open();scoped.create(sessions[0].id);scoped.create(sessions[1].id)
 await show(scoped,`/sessions?agent=a1&session=${sessions[0].id}`)
 await expect(panel(scoped.page).getByLabel('Conversation',{exact:true})).toHaveValue('session')
 await expect(panel(scoped.page).getByText('Working agent: OpenCode · worker-model',{exact:true})).toBeVisible()
 await panel(scoped.page).getByLabel('Message Gaffer').fill('Draft for the first session')
 await panel(scoped.page).getByLabel('Conversation',{exact:true}).selectOption('general')
 await expect(panel(scoped.page).getByLabel('Message Gaffer')).toHaveValue('')
 await panel(scoped.page).getByLabel('Message Gaffer').fill('General draft')
 await panel(scoped.page).getByLabel('Conversation',{exact:true}).selectOption('session')
 await expect(panel(scoped.page).getByLabel('Message Gaffer')).toHaveValue('Draft for the first session')
 await expect(panel(scoped.page)).not.toContainText('PRIVATE WORKER TRANSCRIPT')
 // A late response from session A must not replace session B, even while the panel stays open.
 scoped.box.lateScope=sessions[0].id
 await panel(scoped.page).getByLabel('Conversation',{exact:true}).selectOption('general')
 await panel(scoped.page).getByLabel('Conversation',{exact:true}).selectOption('session')
 await expect.poll(()=>typeof scoped.box.releaseScope).toBe('function')
 await scoped.page.getByRole('button',{name:/^Second session on/}).click()
 await expect(panel(scoped.page).locator('.scope-title')).toHaveText('Second session')
 await expect(panel(scoped.page).getByLabel('Message Gaffer')).toHaveValue('')
 await panel(scoped.page).getByLabel('Message Gaffer').fill('Second session draft')
 scoped.box.releaseScope();scoped.box.lateScope=null
 await expect(panel(scoped.page).getByLabel('Message Gaffer')).toHaveValue('Second session draft')
 await scoped.page.getByRole('button',{name:/^First session on/}).click()
 await expect(panel(scoped.page).locator('.scope-title')).toHaveText('First session')
 await expect(panel(scoped.page).getByLabel('Message Gaffer')).toHaveValue('Draft for the first session')
 await panel(scoped.page).getByRole('button',{name:'Close Gaffer',exact:true}).click()
 await scoped.page.goto(`${base}/sessions?agent=a1&session=${sessions[2].id}`)
 await scoped.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
 await expect(panel(scoped.page).getByRole('heading',{name:'This session uses a terminal.'})).toBeVisible()
 expect(scoped.errors).toEqual([]);await scoped.context.close()

 const history=await open({seed:true});history.create('archived-session-buddy');await show(history)
 await panel(history.page).getByRole('button',{name:'Saved chats',exact:true}).click()
 const saved=history.page.getByRole('dialog',{name:'Saved Gaffer chats',exact:true})
 await expect(saved.getByText('Session companion · on-buddy',{exact:true})).toBeVisible()
 await saved.getByRole('button',{name:'Remove session on-buddy chat',exact:true}).click()
 await history.page.getByRole('button',{name:'Remove saved chat',exact:true}).click()
 await expect(saved.getByText('Session companion · on-buddy',{exact:true})).toHaveCount(0)
 await saved.getByRole('button',{name:'Close saved chats',exact:true}).click()
 expect(history.box.deletes).toHaveLength(1);expect(history.errors).toEqual([]);await history.context.close()

 const no=await open({noProviders:true});await show(no)
 await expect(panel(no.page).getByRole('link',{name:'Set up Gaffer',exact:true})).toBeVisible()
 await expect(panel(no.page).getByRole('button',{name:'Send message',exact:true})).toBeDisabled();await no.context.close()

 // Exercise the browser's real EventSource over the HTTP SSE envelope as well.
 const native=await open({native:true})
 await native.page.route('**/api/v2/gaffer/conversations/**/events',async route=>{
  const view=[...native.box.conversations.values()][0],turn=view.turns[0]
  Object.assign(turn,{answer:'Native streamed answer.',state:'completed',updatedAt:now+1,finishedAt:now+1});view.conversation.revision++
  await route.fulfill({headers:{'Content-Type':'text/event-stream','Cache-Control':'no-store'},body:`event: turn\ndata: ${JSON.stringify(turn)}\n\n`})
 })
 await show(native);await panel(native.page).getByLabel('Message Gaffer').fill('Check native streaming')
 await panel(native.page).getByRole('button',{name:'Send message',exact:true}).click()
 await expect(panel(native.page).getByText('Native streamed answer.',{exact:true})).toBeVisible()
 expect(native.box.posts).toHaveLength(1);expect(native.errors).toEqual([]);await native.context.close()

 for(const width of [1440,390,320])for(const theme of ['light','dark']){
  const w=await open({seed:true,width,theme,height:width<650?844:1000});await show(w)
  await expect(panel(w.page).getByText('Reet, let’s give it a proper look.',{exact:false})).toBeVisible()
  expect(await w.page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  const bounds=await panel(w.page).boundingBox();expect(bounds.x).toBeGreaterThanOrEqual(0);expect(bounds.x+bounds.width).toBeLessThanOrEqual(width+1)
  if(width<650){expect(await w.page.evaluate(()=>document.querySelector('.gaffer-chat').matches(':modal'))).toBe(true);expect((await panel(w.page).getByRole('button',{name:'Send message',exact:true}).boundingBox()).height).toBeGreaterThanOrEqual(44)}
  await w.page.screenshot({path:`${captures}/gaffer-chat-${width}-${theme}.png`})
  await panel(w.page).getByLabel('Message Gaffer').fill('☕'.repeat(2667))
  await expect(panel(w.page).getByText('8,001 / 8,000 bytes',{exact:true})).toBeVisible()
  await expect(panel(w.page).getByRole('button',{name:'Send message',exact:true})).toBeDisabled()
  await w.page.keyboard.press('Escape');await expect(panel(w.page)).toHaveCount(0)
  await expect(w.page.getByRole('button',{name:'Ask Gaffer',exact:true})).toBeFocused()
  expect(w.errors).toEqual([]);await w.context.close()
 }
 const short=await open({width:390,height:430,seed:true});await show(short)
 await expect(panel(short.page)).toHaveAttribute('data-shell-short','')
 await panel(short.page).getByLabel('Message Gaffer').fill('A short-screen question')
 const sendBox=await panel(short.page).getByRole('button',{name:'Send message',exact:true}).boundingBox()
 expect(sendBox.y+sendBox.height).toBeLessThanOrEqual(430)
 expect(short.errors).toEqual([]);await short.context.close()
 for(const status of [{user:{id:2,name:'Reader',email:'reader@example.test',role:'viewer'}},{mode:'none',user:null},{mode:'accounts',user:null}]){
  const w=await open({status});await w.page.goto(base+'/');await expect(w.page.getByRole('button',{name:'Ask Gaffer',exact:true})).toHaveCount(0);await w.context.close()
 }
}finally{await browser.close()}
console.log('Gaffer companion: CRUD, streaming, cancellation, exact receipt retry, scope isolation, availability, keyboard and theme/phone matrix passed.')
