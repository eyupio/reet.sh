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
// The opening prompt as the server keeps it: the list carries its first 300 characters, the prompt route the whole text.
const FULL_PROMPT = 'Rebuild the session overview so the opening prompt is readable at a glance.\n\n' + 'Keep the quote short and the status header where it is. '.repeat(6) + '\n\nFINAL-LINE-ONLY-IN-FULL-TEXT'
const EXCERPT = FULL_PROMPT.slice(0, 300)
session.promptExcerpt = EXCERPT; session.promptMore = true
const second = {...session, id:'session-insights-002', title:'Second session', promptExcerpt:'SECOND-SESSION-PROMPT is a short one.', promptMore:false}
async function open(width=1600,theme='dark', supported=true, hold=null, opts={}) {
 const calls=[], box={fail:false, answer:null}, prompts=[]
 const first = {...session, ...opts.session}
 const inspect = async request=>{
   const q=new URL(request.url()).searchParams,view=q.get('view'),path=q.get('path');calls.push({view,path})
   if(hold&&view==='status')await hold
   if(box.answer)return box.answer(view,path)
   if(box.fail)return {status:503,json:{error:'Machine unavailable'}}
   if(view==='status')return {...snapshot(),...opts.status}
   if(view==='github'){const answer=pull();return opts.noPull?{...snapshot(),...opts.status}:{...answer,...opts.status,pull:{...answer.pull,...opts.pull}}}
   if(view==='files')return {checkedAt:Date.now(),files:path?[{path:'src/session.ts',added:0,removed:0}]:[{path:'src',directory:true,added:0,removed:0},{path:'README.md',added:0,removed:0}],ahead:0,behind:0,changed:0,conflicts:0}
   return {checkedAt:Date.now(),text:view==='diff'?'@@ -1 +1 @@\n-old value\n+new value':'export const status = "working"',ahead:0,behind:0,changed:0,conflicts:0}
 }
 const promptRoute = id => async request=>{prompts.push(id);if(opts.promptGate&&id===first.id)await opts.promptGate;return opts.prompt?opts.prompt(request,id):{prompt:id===first.id?FULL_PROMPT:second.promptExcerpt}}
 const routes={
  [`/api/v2/terminal/sessions/${first.id}/prompt`]:promptRoute(first.id),
  [`/api/v2/terminal/sessions/${second.id}/prompt`]:promptRoute(second.id),
  [`/api/v2/terminal/sessions/${second.id}/seen`]:{},
  [`/api/v2/terminal/sessions/${second.id}/skills`]:{skills:[],machineSeen:Date.now(),host:'studio'},
  [`/api/v2/terminal/sessions/${second.id}/inspect`]:inspect,
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/machines':{machines:[{...machine,capabilities:{...caps,inspect:supported}}]},
  '/api/v2/machines/health':()=>({serverNow:Date.now(),machines:[{...machine,state:'connected',connection:{connected:true,reportFresh:true,reportAt:Date.now(),contactAt:Date.now(),missedProbes:0,reconnects:0,path:'direct'}}]}),
  '/api/v2/terminal/sessions':{sessions:opts.second?[first,second]:[first]},
  '/api/v2/folder-projects':{projects:[{id:'p1',name:'reet',host:'a1',machine:'studio',dir:session.dir,isRepo:true,baseBranch:'main',worktreeDefault:'inherit',colour:'green',defaultTool:'opencode',defaultModel:'',hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1}],savedWaiting:0},
  '/api/v2/projects':{projects:[]},
  [`/api/v2/terminal/sessions/${session.id}/seen`]:{},
  [`/api/v2/terminal/sessions/${session.id}/skills`]:{skills:[],machineSeen:Date.now(),host:'studio'},
  '/api/v2/yolo':{allowed:false},
  [`/api/v2/terminal/sessions/${session.id}/inspect`]:inspect,
 }
 const opened=await openWorkspace(browser,{width,height:1000,touch:width<800,theme,routes})
 await opened.page.addInitScript(delay=>{
  class Stream {
   constructor(){this.listeners={};window.__insightStream=this;setTimeout(()=>{this.onopen?.({});this.emit('reset',{truncated:false});this.emit('chat',{seq:1,kind:'agent',messageId:'reply',text:'The workspace is connected. I am checking the browser layout.'});this.emit('chat',{seq:2,kind:'tool',toolCallId:'done',title:'Read session component',toolKind:'read',status:'completed'});this.emit('chat',{seq:3,kind:'tool',toolCallId:'current',title:'Run browser checks',toolKind:'execute',status:'in_progress',command:'npm run check'});this.emit('chat',{seq:4,kind:'status',state:'working'})},delay)}
   addEventListener(type,fn){(this.listeners[type]??=[]).push(fn)}
   emit(type,data){for(const fn of this.listeners[type]??[])fn({data:JSON.stringify(data)})}
   close(){}
  }
  window.EventSource=Stream
  sessionStorage.setItem('reet-session-visit:session-insights-001',JSON.stringify({seq:1,at:Date.now()-600000}))
 },opts.streamDelay||0)
 await opened.page.goto(`${base}/sessions?agent=a1&session=${session.id}`,{timeout:120000})
 const panel=opened.page.getByRole('complementary',{name:'Session context'})
 if(width<1100){
  // A phone opens a Gaffer chat in super focus; Details waits behind Exit focus.
  await opened.page.locator('.session-screen header.head').waitFor({state:'visible',timeout:60000})
  const exitFocus=opened.page.getByRole('button',{name:'Exit focus mode',exact:true})
  if(await exitFocus.count())await exitFocus.click()
  await opened.page.getByRole('button',{name:'Details',exact:true}).click()
 }
 try { await expect(panel).toBeVisible({timeout:60000}) } catch(e) { console.error('Runtime errors:',opened.errors,'Unexpected routes:',opened.state.unexpected); throw e }
 return {...opened,calls,box,panel,prompts}
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
  await panel.getByRole('tab',{name:/^GitHub/}).click()
  await expect(panel.getByText('Resolve merge conflicts')).toBeVisible()
  await expect(panel.getByText('GitHub reports conflicts with the base branch. 1 check is also failing.')).toBeVisible()
  await expect(panel.getByText('1 failing',{exact:true})).toBeVisible()
  await expect(panel.getByText('1 running',{exact:true})).toBeVisible()
  await expect(panel.getByText('1 passed',{exact:true})).toBeVisible()
  await expect(panel.getByText('Changes requested',{exact:true})).toBeVisible()
  await expect(panel.getByText('Conflicts with base',{exact:true})).toBeVisible()
  await expect(panel.getByRole('link',{name:'Open failing check'})).toHaveAttribute('href','https://github.com/example/project/actions/runs/1')
  await audit(page,panel)
  await expect(panel.getByText('Browser checks',{exact:true})).toBeVisible()
  await expect(panel.getByText('Failed',{exact:true})).toBeVisible()
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
  if(process.env.REET_UI_SCREENSHOTS){await mkdir(process.env.REET_UI_SCREENSHOTS,{recursive:true});await panel.getByRole('tab',{name:/^GitHub/}).click();await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/session-github-${theme}.png`});await panel.getByRole('tab',{name:'Overview',exact:true}).click();await page.screenshot({path:`${process.env.REET_UI_SCREENSHOTS}/session-overview-${theme}.png`})}
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
 // The context panel has the sessions panel's controls: a resize separator (drag, keys, double-click) and a collapse
 // control that hands focus back to Details, with the width and open state kept for the person.
 for(const theme of ['dark','light']){
  const {page,context,panel,errors}=await open(1600,theme)
  const details=page.locator('.session-screen').getByRole('button',{name:'Details',exact:true})
  const separator=page.getByRole('separator',{name:'Resize session workspace'})
  const width=async()=>Math.round((await panel.boundingBox()).width)
  expect(await width()).toBe(360)
  await expect(separator).toBeVisible()
  // Dragging the separator left widens the panel.
  const grip=await separator.boundingBox();await page.mouse.move(grip.x+grip.width/2,grip.y+grip.height/2);await page.mouse.down();await page.mouse.move(grip.x+grip.width/2-60,grip.y+grip.height/2,{steps:6});await page.mouse.up()
  await expect.poll(width).toBeGreaterThan(400)
  // Arrow keys follow the drag's direction; End reaches the widest, Home the narrowest.
  await separator.focus();const widened=await width()
  await page.keyboard.press('ArrowRight');await expect.poll(width).toBeLessThan(widened)
  await page.keyboard.press('ArrowLeft');await expect.poll(width).toBe(widened)
  await page.keyboard.press('End');const widest=await width();expect(widest).toBeGreaterThan(widened)
  await page.keyboard.press('Home');await expect.poll(width).toBe(300)
  await page.keyboard.press('End');expect(await width()).toBe(widest)
  // Double-click resets to the default, and collapsing remembers the width when Details brings it back.
  await separator.dblclick();await expect.poll(width).toBe(360)
  await panel.getByRole('button',{name:'Collapse session workspace'}).click()
  await expect(panel).toBeHidden();await expect(separator).toBeHidden()
  await expect(details).toBeFocused();await expect(details).toHaveAttribute('aria-expanded','false')
  await details.click();await expect(panel).toBeVisible();await expect(separator).toBeVisible();expect(await width()).toBe(360)
  await expect(details).toHaveAttribute('aria-expanded','true')
  expect(errors).toEqual([]);await context.close()
 }

 // The opening prompt: a two-line quote under the status header, and the whole text one tap away.
 {
  const {page,context,panel,errors,prompts}=await open(1600,'dark',true,null,{second:true})
  await context.grantPermissions(['clipboard-read','clipboard-write'])
  const card=panel.getByRole('group',{name:'You asked'}),quote=card.locator('blockquote p')
  await expect(quote).toContainText('Rebuild the session overview')
  const clamp=await quote.evaluate(el=>({lines:Math.round(el.clientHeight/parseFloat(getComputedStyle(el).lineHeight)),cut:el.scrollHeight>el.clientHeight}))
  expect(clamp,'the quote shows two lines and cuts the rest').toEqual({lines:2,cut:true})
  await expect(page.locator('body')).not.toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
  const trigger=card.getByRole('button',{name:'Read full prompt'})
  await trigger.click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog).toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
  await expect(dialog.locator('[data-shell-body] .text')).toHaveCSS('white-space','pre-wrap')
  await expect(dialog.getByText('opencode',{exact:true})).toBeVisible();await expect(dialog.getByText('provider/model',{exact:true})).toBeVisible()
  expect(prompts).toEqual(['session-insights-001'])
  await dialog.getByRole('button',{name:'Copy prompt'}).click()
  await expect(dialog.getByRole('button',{name:'Copied'})).toBeVisible()
  expect(await page.evaluate(()=>navigator.clipboard.readText())).toBe(FULL_PROMPT)
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden();await expect(trigger).toBeFocused()
  expect(errors).toEqual([]);await context.close()
 }
 // The chips say how the session was started, including the auto-approve decision.
 {
  const {page,context,panel,errors}=await open(1600,'light',true,null,{session:{yolo:true,effort:'high'}})
  await panel.getByRole('button',{name:'Read full prompt'}).click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog.getByText('high',{exact:true})).toBeVisible();await expect(dialog.getByText('YOLO',{exact:true})).toBeVisible()
  expect(errors).toEqual([]);await context.close()
 }
 // A prompt that arrives after the person has moved to another session is dropped, never shown under the wrong one.
 {
  let release;const gate=new Promise(done=>{release=done})
  const {page,context,panel,errors,prompts}=await open(1600,'dark',true,null,{second:true,promptGate:gate})
  await panel.getByRole('button',{name:'Read full prompt'}).click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog).toContainText('Rebuild the session overview')
  await expect.poll(()=>prompts.length).toBe(1)
  // The open dialog makes the rest of the page inert, so the switch is made from the page, as a late deep link would.
  await page.getByRole('button',{name:/Second session/}).first().evaluate(el=>el.click())
  await expect(panel.getByRole('group',{name:'You asked'})).toContainText('SECOND-SESSION-PROMPT')
  release()
  await page.waitForTimeout(300)
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.locator('body')).not.toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
  await expect(page.locator('body')).not.toContainText('Rebuild the session overview')
  expect(errors).toEqual([]);await context.close()
 }
 // A session started before prompts were kept says so, in the same slot, with no console errors and no request.
 {
  const {page,context,panel,errors,prompts}=await open(1600,'dark',true,null,{session:{promptExcerpt:undefined,promptMore:undefined},streamDelay:700})
  const card=panel.getByRole('group',{name:'You asked'}),where=panel.getByRole('heading',{name:"Where it's working"})
  await expect(card).toContainText('Loading')
  await expect(card.getByText('This session started before reet kept opening prompts.')).toHaveCount(0)
  const loading={card:Math.round((await card.boundingBox()).height),where:Math.round((await where.boundingBox()).y)}
  await expect(card.getByText('This session started before reet kept opening prompts.')).toBeVisible()
  expect({card:Math.round((await card.boundingBox()).height),where:Math.round((await where.boundingBox()).y)},'the blocks below the prompt moved when it loaded').toEqual(loading)
  await expect(card.getByRole('button',{name:'Read full prompt'})).toHaveCount(0)
  expect(prompts).toEqual([]);expect(errors).toEqual([]);await context.close()
 }
 // The server keeps no text for this session: a 404 reads the same as an old session.
 {
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{prompt:()=>({status:404,json:{error:'No prompt kept.'}})})
  await panel.getByRole('button',{name:'Read full prompt'}).click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog.getByText('This session started before reet kept opening prompts.')).toBeVisible()
  await expect(dialog.getByText("Couldn't load the full prompt. Try again.")).toHaveCount(0)
  expect(errors).toEqual([]);await context.close()
 }
 // A failed read keeps the excerpt, says so, and the retry fetches again.
 {
  let calls=0
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{prompt:()=>++calls===1?{status:500,json:{error:'The server could not read it.'}}:{prompt:FULL_PROMPT}})
  await panel.getByRole('button',{name:'Read full prompt'}).click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog.getByText("Couldn't load the full prompt. Try again.")).toBeVisible()
  await expect(dialog).toContainText('Rebuild the session overview');await expect(dialog).not.toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
  await dialog.getByRole('button',{name:'Try again'}).click()
  await expect(dialog).toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
  await expect(dialog.getByText("Couldn't load the full prompt. Try again.")).toHaveCount(0)
  expect(calls).toBe(2);expect(errors).toEqual([]);await context.close()
 }
 // On a phone the dialog fits and the card adds no sideways scroll.
 {
  const {page,context,panel,errors}=await open(375,'dark')
  await panel.getByRole('button',{name:'Read full prompt'}).click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog).toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
  const box=await dialog.boundingBox();expect(box.x).toBeGreaterThanOrEqual(0);expect(box.x+box.width).toBeLessThanOrEqual(375)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect((await dialog.getByRole('button',{name:'Copy prompt'}).boundingBox()).height).toBeGreaterThanOrEqual(42)
  expect(errors).toEqual([]);await context.close()
 }
 // In the phone sheet the dialog keeps its own keys: Tab stays inside it, and Escape closes it, not the sheet behind it.
 {
  const {page,context,panel,errors}=await open(375,'dark')
  const trigger=panel.getByRole('button',{name:'Read full prompt'})
  await trigger.click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog).toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
  for(let i=0;i<5;i++){await page.keyboard.press('Tab');expect(await panel.evaluate(p=>{const a=document.activeElement;return p.contains(a)&&!a.closest('dialog')}),`Tab ${i+1} moved focus into the sheet behind the dialog`).toBe(false)}
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden();await expect(panel).toBeVisible();await expect(trigger).toBeFocused()
  expect(errors).toEqual([]);await context.close()
 }

 // The run clock: one line under the subline, a real time element, and nothing that a screen reader announces every minute.
 {
  const {context,panel,errors}=await open(1600,'dark')
  const line=panel.locator('.run-line')
  await expect(line).toHaveText(/^Started \d\d:\d\d · running 1h \d+m$/)
  await expect(line.locator('time')).toHaveAttribute('datetime',/^\d{4}-\d\d-\d\dT/)
  expect(await line.locator('time').getAttribute('title')).toMatch(/\d{4}/)
  expect(await line.evaluate(el=>!el.closest('[aria-live],[role=status],[role=alert]')&&!el.querySelector('[aria-live]'))).toBe(true)
  expect(errors).toEqual([]);await context.close()
 }
 {
  const created=Date.now()-3*3600000
  const done=await open(1600,'light',true,null,{session:{state:'exited',exitCode:0,createdAt:created,completedAt:created+4320000}})
  await expect(done.panel.locator('.run-line')).toHaveText('Ran 1h 12m')
  expect(done.errors).toEqual([]);await done.context.close()
  const lost=await open(1600,'light',true,null,{session:{state:'lost',createdAt:created}})
  await expect(lost.panel.locator('.run-line')).toHaveText(/^Started \d\d:\d\d$/)
  expect(lost.errors).toEqual([]);await lost.context.close()
 }
 // The Where card: the repo is a link only for a well-formed owner/name, branch and worktree copy, the base shows once known, and the old facts stay.
 {
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{status:{remote:'acme/widgets'},session:{worktreeRepo:'widgets',worktreePath:'/home/dev/.reet/worktrees/a-very-long-folder-name/another-long-folder/reet-session-insights-001'}})
  await context.grantPermissions(['clipboard-read','clipboard-write'])
  const where=panel.getByRole('region',{name:"Where it's working"}).or(panel.locator('section[aria-labelledby$="-where"]'))
  const row=name=>where.locator('.where > div').filter({has:page.getByText(name,{exact:true})})
  const repo=row('Repo').getByRole('link',{name:'acme/widgets'})
  await expect(repo).toHaveAttribute('href','https://github.com/acme/widgets')
  await expect(repo).toHaveAttribute('target','_blank');await expect(repo).toHaveAttribute('rel','noopener noreferrer')
  await expect(row('Machine')).toContainText('studio');await expect(row('Machine')).toContainText('online')
  await expect(row('Project')).toContainText('reet')
  await expect(row('Branch')).toContainText('feature/session-workspace')
  await expect(row('Agent')).toContainText('opencode');await expect(row('Agent')).toContainText('provider/model')
  await expect(row('Started')).toContainText(/\d\d:\d\d/)
  await expect(row('Worktree')).toContainText('Own worktree')
  const shown=await row('Worktree').locator('.path').textContent()
  expect(shown).toContain('…');expect(shown.length).toBeLessThan(60);expect(shown.endsWith('session-insights-001')).toBe(true)
  await expect(row('Worktree').locator('.path')).toHaveAttribute('title','/home/dev/.reet/worktrees/a-very-long-folder-name/another-long-folder/reet-session-insights-001')
  await row('Branch').getByRole('button',{name:'Copy branch name'}).click()
  expect(await page.evaluate(()=>navigator.clipboard.readText())).toBe('feature/session-workspace')
  await row('Worktree').getByRole('button',{name:'Copy worktree path'}).click()
  expect(await page.evaluate(()=>navigator.clipboard.readText())).toBe('/home/dev/.reet/worktrees/a-very-long-folder-name/another-long-folder/reet-session-insights-001')
  expect(errors).toEqual([]);await context.close()
 }
 for(const [remote,expected] of [['not a repo/..','widgets'],['javascript:alert(1)//x','widgets'],[undefined,'widgets']]){
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{status:{remote},session:{worktreeRepo:'widgets'}})
  const repo=panel.locator('.where > div').filter({has:page.getByText('Repo',{exact:true})})
  await expect(repo).toContainText(expected);await expect(repo.getByRole('link')).toHaveCount(0)
  expect(errors).toEqual([]);await context.close()
 }
 {
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{status:{remote:undefined}})
  const repo=panel.locator('.where > div').filter({has:page.getByText('Repo',{exact:true})})
  await expect(repo).toContainText('Not recorded')
  await expect(panel.locator('.where > div').filter({has:page.getByText('Worktree',{exact:true})})).toContainText('Shared checkout')
  await expect(panel.locator('.where')).not.toContainText('Base →')
  expect(errors).toEqual([]);await context.close()
 }
 // The base branch appears next to the branch once the pull request read has named it.
 {
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{pull:{base:'main'}})
  await expect(panel.locator('.where > div').filter({has:page.getByText('Branch',{exact:true})})).toContainText('Base → main')
  expect(errors).toEqual([]);await context.close()
 }
 // The GitHub tab says what the pull request needs, in its name and not only in colour; with no pull request it says nothing.
 for(const [label,pullFields,name,tone] of [
  ['failing checks',{mergeable:'MERGEABLE',review:'REVIEW_REQUIRED'},'GitHub, needs attention','bad'],
  ['conflicts',{mergeable:'CONFLICTING',checks:[]},'GitHub, needs attention','bad'],
  ['checks running',{mergeable:'MERGEABLE',review:'REVIEW_REQUIRED',checks:[{name:'Build',state:'IN_PROGRESS'}]},'GitHub, in progress','warn'],
  ['merged',{state:'MERGED',mergeable:'UNKNOWN',checks:[]},'GitHub, merged','good'],
 ]){
  const {context,panel,errors}=await open(1600,'dark',true,null,{pull:pullFields})
  const tab=panel.getByRole('tab',{name})
  await expect(tab,`${label}: the tab names the state`).toBeVisible()
  await expect(tab.locator('.tab-dot')).toHaveAttribute('data-tone',tone)
  await expect(tab.locator('span').filter({hasText:/^GitHub$/})).toBeVisible()
  expect(errors).toEqual([]);await context.close()
 }
 {
  const {context,panel,errors}=await open(1600,'dark',true,null,{noPull:true})
  await expect(panel.getByText('3 changed files',{exact:true})).toBeVisible()
  await expect(panel.getByRole('tab',{name:'GitHub',exact:true})).toBeVisible()
  await expect(panel.locator('.tab-dot')).toHaveCount(0)
  expect(errors).toEqual([]);await context.close()
 }
 // The Where card and the run line hold their height between the first read and the loaded view, at both widths.
 for(const width of [1600,375]){
  let release;const held=new Promise(done=>{release=done})
  const {page,context,panel,errors}=await open(width,'dark',true,held,{status:{remote:'acme/widgets'},pull:{base:'main'}})
  const heights=()=>panel.evaluate(el=>({rows:[...el.querySelectorAll('.where > div')].map(r=>Math.round(r.getBoundingClientRect().height)).join(),run:Math.round(el.querySelector('.run-line').getBoundingClientRect().height),where:Math.round(el.querySelector('section[aria-labelledby$="-where"]').getBoundingClientRect().height)}))
  const loading=await heights();release()
  await expect(panel.getByRole('link',{name:'acme/widgets'})).toBeVisible()
  expect(await heights(),`${width}px: the Where card or run line changed height when the read arrived`).toEqual(loading)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(errors).toEqual([]);await context.close()
 }
 console.log('Session workspace: tabs, return summary, changes, previews, folders, CI, stale data, sidebar signals, context panel resize/collapse and phone/tablet focus passed.')
}finally{await browser.close()}
