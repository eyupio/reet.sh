// Session workspace: real rendering and navigation, synthetic machine and chat evidence.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium',args:['--no-sandbox']})
const caps = {protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['opencode'],acp:true,acpHistory:true,acpApproval:true,inspect:true}
const session = {id:'session-insights-001',agentId:'a1',title:'Improve session workspace',state:'running',createdAt:Date.now()-3600000,updatedAt:Date.now(),activityAt:Date.now()-20000,exitCode:null,tool:'opencode',model:'provider/model',projectId:'p1',dir:'/home/dev/reet',structured:true,attention:'working',attentionSource:'chat'}
const machine = {agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:caps}
const snapshot = () => ({checkedAt:Date.now(),repo:true,branch:'feature/session-workspace',ahead:2,behind:1,upstream:true,changed:3,conflicts:1,countsKnown:true,files:[{path:'src/session.ts',status:' M',added:42,removed:12},{path:'src/conflict.ts',status:'UU',added:3,removed:1},{path:'new.md',status:'??',added:0,removed:0}]})
const pull = () => ({...snapshot(),pull:{number:42,title:'Improve the session workspace',url:'https://github.com/example/project/pull/42',state:'OPEN',draft:false,review:'CHANGES_REQUESTED',mergeable:'CONFLICTING',added:45,removed:13,checks:[{name:'Browser checks',state:'FAILURE',url:'https://github.com/example/project/actions/runs/1'},{name:'Build',state:'IN_PROGRESS'},{name:'Unit tests',state:'SUCCESS'}]}})
async function open(width=1600,theme='dark', supported=true, hold=null) {
 const calls=[], box={fail:false, answer:null}
 const routes={
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/machines':{machines:[{...machine,capabilities:{...caps,inspect:supported}}]},
  '/api/v2/machines/health':()=>({serverNow:Date.now(),machines:[{...machine,state:'connected',connection:{connected:true,reportFresh:true,reportAt:Date.now(),contactAt:Date.now(),missedProbes:0,reconnects:0,path:'direct'}}]}),
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/folder-projects':{projects:[{id:'p1',name:'reet',host:'a1',machine:'studio',dir:session.dir,isRepo:true,baseBranch:'main',worktreeDefault:'inherit',colour:'green',defaultTool:'opencode',defaultModel:'',hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1}],savedWaiting:0},
  '/api/v2/projects':{projects:[]},
  [`/api/v2/terminal/sessions/${session.id}/seen`]:{},
  [`/api/v2/terminal/sessions/${session.id}/skills`]:{skills:[],machineSeen:Date.now(),host:'studio'},
  '/api/v2/yolo':{allowed:false},
  [`/api/v2/terminal/sessions/${session.id}/inspect`]:async request=>{
   const q=new URL(request.url()).searchParams,view=q.get('view'),path=q.get('path');calls.push({view,path})
   if(hold&&view==='status')await hold
   if(box.answer)return box.answer(view,path)
   if(box.fail)return {status:503,json:{error:'Machine unavailable'}}
   if(view==='status')return snapshot()
   if(view==='github')return pull()
   if(view==='files')return {checkedAt:Date.now(),files:path?[{path:'src/session.ts',added:0,removed:0}]:[{path:'src',directory:true,added:0,removed:0},{path:'README.md',added:0,removed:0}],ahead:0,behind:0,changed:0,conflicts:0}
   return {checkedAt:Date.now(),text:view==='diff'?'@@ -1 +1 @@\n-old value\n+new value':'export const status = "working"',ahead:0,behind:0,changed:0,conflicts:0}
  },
 }
 const opened=await openWorkspace(browser,{width,height:1000,touch:width<800,theme,routes})
 await opened.page.addInitScript(()=>{
  class Stream {
   constructor(){this.listeners={};window.__insightStream=this;setTimeout(()=>{this.onopen?.({});this.emit('reset',{truncated:false});this.emit('chat',{seq:1,kind:'agent',messageId:'reply',text:'The workspace is connected. I am checking the browser layout.'});this.emit('chat',{seq:2,kind:'tool',toolCallId:'done',title:'Read session component',toolKind:'read',status:'completed'});this.emit('chat',{seq:3,kind:'tool',toolCallId:'current',title:'Run browser checks',toolKind:'execute',status:'in_progress',command:'npm run check'});this.emit('chat',{seq:4,kind:'status',state:'working'})},0)}
   addEventListener(type,fn){(this.listeners[type]??=[]).push(fn)}
   emit(type,data){for(const fn of this.listeners[type]??[])fn({data:JSON.stringify(data)})}
   close(){}
  }
  window.EventSource=Stream
  sessionStorage.setItem('reet-session-visit:session-insights-001',JSON.stringify({seq:1,at:Date.now()-600000}))
 })
 await opened.page.goto(`${base}/sessions?agent=a1&session=${session.id}`,{timeout:120000})
 const panel=opened.page.getByRole('complementary',{name:'Session context'})
 if(width<1100)await opened.page.getByRole('button',{name:'Details',exact:true}).click()
 try { await expect(panel).toBeVisible({timeout:60000}) } catch(e) { console.error('Runtime errors:',opened.errors,'Unexpected routes:',opened.state.unexpected); throw e }
 return {...opened,calls,box,panel}
}
async function audit(page,panel) {
 if(!process.env.REET_AXE_PATH)return
 if(!await page.evaluate(()=>!!window.axe))await page.addScriptTag({path:process.env.REET_AXE_PATH})
 const violations=await panel.evaluate(async el=>(await window.axe.run(el,{runOnly:{type:'tag',values:['wcag2a','wcag2aa','wcag21a','wcag21aa','wcag22aa']}})).violations.map(v=>({id:v.id,impact:v.impact,nodes:v.nodes.map(n=>({target:n.target,reason:n.failureSummary}))})))
 expect(violations).toEqual([])
}
try {
 for(const theme of ['light','dark']){
  const {page,context,panel,calls,box,errors}=await open(1600,theme)
  await expect(panel.getByText('Since your last visit')).toBeVisible()
  await expect(panel.getByText('3 new events',{exact:false})).toBeVisible()
  await expect(panel.getByText('Latest agent update')).toBeVisible()
  await expect(panel.getByText('Run browser checks',{exact:true}).first()).toBeVisible()
  await expect(panel.getByText('3 changed files',{exact:true})).toBeVisible()
  await audit(page,panel)
  // Arrow keys move and activate tabs. No extra chat connection is opened for the panel.
  await panel.getByRole('tab',{name:'Overview',exact:true}).focus();await page.keyboard.press('ArrowRight')
  await expect(panel.getByRole('tab',{name:'Changes',exact:false})).toHaveAttribute('aria-selected','true')
  await expect(panel.getByText('1 file with merge conflicts')).toBeVisible()
  await panel.getByRole('button',{name:/session.ts/}).click()
  await expect(panel.locator('.preview')).toContainText('+new value')
  await expect(panel.locator('.addition')).toHaveCount(1)
  await panel.getByRole('button',{name:'File',exact:true}).click()
  await expect(panel.locator('.preview')).toContainText('export const status')
  await panel.getByRole('tab',{name:'Files',exact:true}).click()
  await panel.getByRole('button',{name:/src Folder/}).click()
  await expect(panel.locator('.folder-head')).toContainText('src')
  await panel.getByRole('button',{name:/session.ts/}).click()
  await expect(panel.locator('.preview')).toContainText('export const status')
  await panel.getByRole('tab',{name:'GitHub',exact:true}).click()
  await expect(panel.getByText('Resolve merge conflicts')).toBeVisible()
  await audit(page,panel)
  await expect(panel.getByText('Browser checks',{exact:true})).toBeVisible()
  await expect(panel.getByText('Unit tests',{exact:true})).not.toBeVisible()
  await panel.getByText('1 passed or skipped',{exact:true}).click()
  await expect(panel.getByText('Unit tests',{exact:true})).toBeVisible()
  box.fail=true;await panel.getByRole('button',{name:'Refresh session workspace'}).click()
  await expect(panel.getByText(/Machine unavailable/)).toBeVisible()
  await expect(panel.getByText('Resolve merge conflicts')).toBeVisible()
  await expect(panel.locator('footer')).toContainText('Last known')
  box.fail=false
  await panel.getByRole('tab',{name:'Changes',exact:false}).click()
  await panel.getByRole('textbox',{name:'Filter changed files'}).fill('not-found')
  await expect(panel.getByText('No files match this filter.')).toBeVisible()
  await expect(page.locator('.session-sidebar .signals')).toContainText('42')
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await panel.getByRole('tab',{name:'GitHub',exact:true}).click();await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/session-github-${theme}.png`});await panel.getByRole('tab',{name:'Overview',exact:true}).click();await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/session-overview-${theme}.png`})}
  expect(errors).toEqual([]);expect(calls.some(c=>c.view==='file'&&c.path==='src/session.ts')).toBe(true)
  await context.close()
 }
 for(const width of [375,768]){
  const {page,context,panel,errors}=await open(width,'dark')
  await expect(panel.getByRole('button',{name:'Close details'})).toBeFocused()
  await audit(page,panel)
  await panel.locator('.session-details > summary').focus();await page.keyboard.press('Tab')
  await expect(panel.getByRole('checkbox',{name:'Live updates'})).toBeFocused()
  await panel.getByRole('tab',{name:'Changes',exact:false}).click()
  await expect(panel.getByText('1 file with merge conflicts')).toBeVisible()
  await panel.getByRole('textbox',{name:'Filter changed files'}).focus()
  await page.keyboard.press('Tab')
  await expect(panel.getByRole('button',{name:/session.ts|conflict.ts/}).first()).toBeFocused()
  await panel.getByRole('checkbox',{name:'Live updates'}).focus()
  await page.keyboard.press('Tab');await expect(panel.getByRole('button',{name:'Session',exact:true})).toBeFocused()
  await page.keyboard.press('Tab');await expect(panel.getByRole('button',{name:'Close details'})).toBeFocused()
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  await page.keyboard.press('Escape');await expect(panel).not.toBeVisible();await expect(page.getByRole('button',{name:'Details',exact:true})).toBeFocused()
  expect(errors).toEqual([]);await context.close()
 }
 // The overview keeps every section where it is while the current task changes: only the text inside the live slots changes.
 for(const width of [1600,375]){
  const steady=await open(width,'dark'),{page:sp,panel:sw}=steady
  await expect(sw.getByText('3 changed files',{exact:true})).toBeVisible()
  await expect(sw.getByText('Latest agent update')).toBeVisible()
  const pinned=()=>sw.evaluate(el=>{const top=n=>Math.round(n.getBoundingClientRect().top),h=name=>[...el.querySelectorAll('.panel-body h4')].find(n=>n.textContent.trim().toLowerCase()===name);return{changes:top(h('workspace changes')),where:top(h("where it's working")),details:top(el.querySelector('.session-details > summary'))}})
  const settle=()=>sp.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  const baseline=await pinned()
  const longTitle='Edit web/src/lib/sessions/ContextPanel.svelte so the overview keeps every section where it is while the current task changes'
  const longAsk='Allow the agent to run a command that rewrites several generated files across the whole repository before the build starts?'
  const paragraphs=Array.from({length:14},(_,i)=>`Paragraph ${i+1} of a long agent update that wraps over several lines in a narrow panel.`).join('\n\n')
  const tool=(id,title,status,extra={})=>({kind:'tool',toolCallId:id,title,toolKind:'edit',status,...extra})
  const steps=[
   ['a short task starts',[tool('s1','Run git status','in_progress',{command:'git status'})]],
   ['a long task title',[tool('s1','Run git status','completed'),tool('s2',longTitle,'in_progress',{path:'web/src/lib/sessions/ContextPanel.svelte'})]],
   ['a long agent update',[{kind:'agent',messageId:'long',text:paragraphs}]],
   ['three tasks at once',[tool('s3','Run tests','in_progress',{command:'npm test'}),tool('s4','Read a file','in_progress',{path:'a.ts'}),tool('s5','Search the repository','in_progress',{command:'rg value'})]],
   ['a task fails',[tool('s3','Run tests','failed')]],
   ['nothing is running',[tool('s2','Edit','completed'),tool('s4','Read','completed'),tool('s5','Search','completed')]],
   ['the agent asks permission',[{kind:'permission',permissionId:'ask1',title:longAsk,toolKind:'execute'}]],
   ['the permission is answered',[{kind:'permission_resolved',permissionId:'ask1',allow:true}]],
   ['the chat reports an error',[{kind:'status',state:'error',detail:longAsk+' '+longAsk}]],
   ['work resumes',[{kind:'status',state:'working'},tool('s6','Run browser checks','in_progress',{command:'npm run check'})]],
  ]
  let seq=20
  for(const [name,events] of steps){
   for(const event of events)await sp.evaluate(e=>window.__insightStream.emit('chat',e),{seq:seq++,...event})
   await settle()
   expect(await pinned(),`${width}px: sections moved after ${name}`).toEqual(baseline)
  }
  // The updates did reach the panel: the headline carries the live task, and nothing wrapped to push the page sideways.
  await expect(sw.locator('.session-summary h3')).toHaveText('Run browser checks')
  expect(await sp.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(steady.errors).toEqual([]);await steady.context.close()
 }
 // The first workspace read replaces a short "Checking…" line with the changes card, and the section after it stays put.
 {
  let release;const held=new Promise(done=>{release=done})
  const first=await open(1600,'dark',true,held)
  const whereTop=()=>first.panel.evaluate(el=>Math.round([...el.querySelectorAll('.panel-body h4')].find(n=>n.textContent.trim().toLowerCase()==="where it's working").getBoundingClientRect().top))
  await expect(first.panel.getByText('Checking the workspace…')).toBeVisible()
  const loading=await whereTop();release()
  await expect(first.panel.getByText('3 changed files',{exact:true})).toBeVisible()
  expect(await whereTop(),'sections moved when the workspace status arrived').toBe(loading)
  expect(first.errors).toEqual([]);await first.context.close()
 }
 const returning=await open()
 await expect(returning.panel.getByText('3 changed files',{exact:true})).toBeVisible()
 await returning.page.clock.install()
 const beforeHidden=returning.calls.length
 await returning.page.evaluate(()=>{window.__testHidden=true;Object.defineProperty(document,'hidden',{configurable:true,get:()=>window.__testHidden});document.dispatchEvent(new Event('visibilitychange'))})
 await returning.page.clock.fastForward(65000)
 expect(returning.calls.length).toBe(beforeHidden)
 await returning.page.evaluate(()=>{window.__insightStream.emit('chat',{seq:5,kind:'agent',messageId:'away',text:'The checks finished while you were away.'});window.__insightStream.emit('chat',{seq:6,kind:'status',state:'idle'})})
 await returning.page.evaluate(()=>{window.__testHidden=false;document.dispatchEvent(new Event('visibilitychange'))})
 await expect(returning.panel.getByText('2 new events',{exact:false})).toBeVisible()
 await expect(returning.panel.getByText('The checks finished while you were away.')).toBeVisible()
 await expect.poll(()=>returning.calls.length).toBeGreaterThan(beforeHidden)
 await returning.panel.getByRole('checkbox',{name:'Live updates'}).uncheck()
 const beforePaused=returning.calls.length;await returning.page.clock.fastForward(130000);expect(returning.calls.length).toBe(beforePaused)
 expect(returning.errors).toEqual([]);await returning.context.close()
 const old=await open(1600,'light',false);await old.panel.getByRole('tab',{name:'Files',exact:true}).click();await expect(old.panel.getByText(/Update reet on this machine to use read-only file browsing/)).toBeVisible();expect(old.calls).toEqual([]);expect(old.errors).toEqual([]);await old.context.close()
 console.log('Session workspace: tabs, return summary, changes, previews, folders, CI, stale data, sidebar signals and phone/tablet focus passed.')
}finally{await browser.close()}
