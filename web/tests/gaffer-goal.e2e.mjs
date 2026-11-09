import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-goals'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const sid='goal-session-alpha',cid='c'.repeat(32),pid='a'.repeat(32),now=Date.now()
function fixture(){
 const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
 const session={id:sid,agentId:'a1',title:'Prepare the release',tool:'opencode',model:'working-model',structured:true,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
 const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},box={allowed:false,starts:[],controls:[],workerWrites:0}
 const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[],messages:[]}
 const goalView=()=>({goal:view.goal??null,pins,allowed:!!view.goal&&box.allowed,setupAllowed:box.allowed,providerName:'Workshop Gaffer',conversationRevision:view.conversation.revision,requiresProviderAccess:!view.goal})
 const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':()=>view,
  [`/api/v2/gaffer/conversations/${cid}/goal`]:()=>goalView(),
  [`/api/v2/gaffer/conversations/${cid}/goal/setup`]:request=>{const in_=request.postDataJSON();expect(in_.confirmed).toBe(true);expect(in_.conversationRevision).toBe(view.conversation.revision);if(in_.action==='resume'){box.controls.push(in_);view.goal.state='active';view.goal.revision++;return goalView()}if(in_.action==='start'){expect(in_.revision).toBe(view.goal?.revision??0);for(const [key,value]of Object.entries(pins))expect(in_[key]).toBe(value);box.starts.push(in_);view.goal={...pins,id:'b'.repeat(32),conversationId:cid,sessionId:sid,revision:(view.goal?.revision??0)+1,text:in_.text,state:'active',note:'Waiting for new worker progress. No model request was made.',maxReviews:in_.maxReviews,usedReviews:0,createdAt:now,updatedAt:now,expiresAt:now+in_.hours*3600000,nextAt:now,lastSeq:-1,inputTokens:0,outputTokens:0,usageReported:true}}return goalView()},
  [`/api/v2/gaffer/conversations/${cid}/goal/control`]:request=>{const in_=request.postDataJSON();expect(in_.revision).toBe(view.goal.revision);if(in_.action==='resume')expect(in_.confirmed).toBe(true);box.controls.push(in_);view.goal.state={pause:'paused',resume:'active',complete:'completed',stop:'stopped'}[in_.action];view.goal.revision++;view.goal.note=in_.action==='complete'?'You marked this goal complete.':'Goal updated.';return goalView()},
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true}}]},
  '/api/v2/projects':{projects:[]},
  [`/api/v2/terminal/sessions/${sid}/seen`]:{},
  [`/api/v2/terminal/sessions/${sid}/chat/controls`]:{controls:{working:false,waiting:false,model:'working-model',options:[]}},
  [`/api/v2/terminal/sessions/${sid}/chat/queue`]:request=>{if(request.method()==='POST')box.workerWrites++;return{queue:[]}},
 }
 return{routes,view,box}
}
try{
 for(const theme of ['light','dark'])for(const width of [1440,800,390,320]){
  const f=fixture(),w=await openWorkspace(browser,{width,height:1000,theme,touch:width<1000,routes:f.routes})
  await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
  try{
   await w.page.goto(`${base}/sessions?agent=a1&session=${sid}`);await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
   const chat=w.page.getByRole('dialog',{name:'Gaffer',exact:true}),goal=w.page.getByRole('dialog',{name:'Look after this session',exact:true})
   await expect(chat.getByRole('button',{name:'Look after this session',exact:true})).toBeVisible()
   const openGoal=async()=>{await chat.getByRole('button',{name:'Chat options',exact:true}).click();await w.page.getByRole('dialog',{name:'Chat options',exact:true}).getByRole('button',{name:'Look after this session',exact:true}).click()}
   await openGoal();await expect(goal.getByRole('button',{name:'Start follow-up',exact:true})).toBeDisabled();expect(f.box.starts).toHaveLength(0)
   await w.page.keyboard.press('Escape');await expect(chat.getByRole('button',{name:'Chat options',exact:true})).toBeFocused()
   f.box.allowed=true;await openGoal();await expect(goal.getByLabel('Review budget',{exact:true})).toHaveValue('12');await expect(goal.getByLabel('Follow for (hours)',{exact:true})).toHaveValue('24')
   await goal.getByRole('button',{name:'Use a prompt',exact:true}).click()
   await expect(w.page.getByRole('listbox',{name:'Goal prompt',exact:true})).toBeVisible()
   await expect(w.page.getByText('reet prompts',{exact:true})).toBeVisible()
   await w.page.getByRole('option',{name:/^Ship the pull request/}).click()
   await expect(goal.getByLabel('Agreed goal',{exact:true})).toHaveValue(/Ensure a pull request is raised/)
   await goal.getByRole('button',{name:'Save as prompt',exact:true}).click()
   await expect(goal.getByText('Saved to your prompts.',{exact:true})).toBeVisible()
   await goal.getByRole('button',{name:'Use a prompt',exact:true}).click()
   await expect(w.page.getByText('Your prompts',{exact:true})).toBeVisible()
   await expect(w.page.getByRole('option',{name:/^Ensure a pull request is raised/})).toHaveCount(1)
   await w.page.getByRole('button',{name:/^Remove “Ensure a pull request is raised/}).click()
   await expect(w.page.getByRole('option',{name:/^Ensure a pull request is raised/})).toHaveCount(0)
   await w.page.keyboard.press('Escape')
   await goal.getByLabel('Agreed goal',{exact:true}).fill('🧠'.repeat(600));await expect(goal.getByRole('button',{name:'Start follow-up',exact:true})).toBeDisabled();await expect(goal.getByText('Keep the goal within 2,000 UTF-8 bytes.',{exact:true})).toBeVisible()
   await goal.getByLabel('Agreed goal',{exact:true}).fill('Ensure a PR is raised and its checks and conflicts are fixed. Bring decisions back to me. Do not merge.');await goal.getByRole('button',{name:'Start follow-up',exact:true}).click()
   await expect(goal.getByRole('heading',{name:'Looking after this session',exact:true})).toBeVisible();expect(f.box.starts).toHaveLength(1);expect(f.box.workerWrites).toBe(0)
   await w.page.keyboard.press('Escape');await chat.getByRole('button',{name:'Close Gaffer',exact:true}).click();await w.page.reload();await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
   await expect(chat.getByRole('button',{name:'Gaffer following · 0/12 reviews',exact:true})).toBeVisible();expect(f.box.starts).toHaveLength(1)
   f.view.goal.usedReviews=1;f.view.goal.note='Waiting for new worker progress.';f.view.goal.lastSeq=42;f.view.goal.lastObservedAt=now;f.view.goal.lastReviewedAt=now+1000;f.view.goal.summary='A PR is raised. A check still needs a fix. <img src=x onerror="window.__goalXss=true">';f.view.goal.usageReported=false
   f.view.turns.push({goalId:f.view.goal.id,goalRevision:1,workerGrantRevision:1,id:'d'.repeat(32),conversationId:cid,requestRevision:1,prompt:'Review new worker progress against my saved goal.',answer:'A PR is raised. A check still needs a fix.',state:'completed',providerId:pid,providerName:'Workshop Gaffer',model:'gaffer-model',usageReported:false,cut:false,startedAt:now,updatedAt:now,finishedAt:now+1000});f.view.messages.push({id:'d'.repeat(32),turnId:'d'.repeat(32),sessionId:sid,queueId:'e'.repeat(32),text:'Fix the failing check and report back. Do not merge.',state:'sent',revision:3,grantRevision:1,createdAt:now});f.view.goal.pendingId='d'.repeat(32);f.view.conversation.revision=3
   await expect(chat.getByText('Goal review',{exact:true})).toBeVisible({timeout:12000});await chat.getByRole('button',{name:'Gaffer following · 1/12 reviews',exact:true}).click()
   await expect(goal.getByRole('heading',{name:'Latest Gaffer review',exact:true})).toBeVisible();expect(await w.page.evaluate(()=>window.__goalXss)).toBeUndefined();await expect(goal.locator('img[src="x"]')).toHaveCount(0)
   expect(await goal.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
   if(width===1440||width===390)await goal.screenshot({path:`${shots}/gaffer-goal-${width}-${theme}.png`})
   await goal.getByRole('button',{name:'Pause follow-up',exact:true}).click();await expect(goal.getByRole('heading',{name:'Paused',exact:true})).toBeVisible()
   await goal.getByRole('button',{name:'Resume after review',exact:true}).click();const resume=w.page.getByRole('dialog',{name:'Resume after checking the working conversation?',exact:true});expect(f.box.controls).toHaveLength(1);await resume.getByRole('button',{name:'Resume follow-up',exact:true}).click();await expect(goal.getByRole('heading',{name:'Looking after this session',exact:true})).toBeVisible();expect(f.view.goal.usedReviews).toBe(1)
   await goal.getByRole('button',{name:'Mark goal complete',exact:true}).click();await w.page.getByRole('dialog',{name:'Mark this goal complete?',exact:true}).getByRole('button',{name:'Mark complete',exact:true}).click();await expect(goal.getByRole('heading',{name:'You marked this complete',exact:true})).toBeVisible();expect(f.box.controls.map(x=>x.action)).toEqual(['pause','resume','complete']);expect(f.box.workerWrites).toBe(0)
   await w.page.keyboard.press('Escape');await expect(chat.getByRole('button',{name:'Chat options',exact:true})).toBeFocused();expect(w.errors).toEqual([])
  }finally{await w.context.close()}
 }
 console.log('Gaffer goals: explicit grants and budgets, UTF-8 bounds, durable reopen, automatic review labels, escaped evidence, pause/resume/complete confirmations, focus and eight theme/width views passed.')
}finally{await browser.close()}
