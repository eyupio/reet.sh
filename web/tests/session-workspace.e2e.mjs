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
// A prompt whose image line is all the server kept of the attachment; the browser holds the thumbnail under the same path.
const IMAGE_PATH = '/home/dev/reet/launch-uploads/img-shot.png'
const IMAGE_PROMPT = 'Show me the screenshot that came with this job.\n\nImage: ' + IMAGE_PATH
const THUMB_URL = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
const ok = name => ({name,state:'SUCCESS'})
const second = {...session, id:'session-insights-002', title:'Second session', promptExcerpt:'SECOND-SESSION-PROMPT is a short one.', promptMore:false}
async function open(width=1600,theme='dark', supported=true, hold=null, opts={}) {
 const calls=[], box={fail:false, answer:null, pull:null, reason:null, offline:false, lists:0}, prompts=[]
 const first = {...session, ...opts.session}
 const inspect = async request=>{
   const q=new URL(request.url()).searchParams,view=q.get('view'),path=q.get('path');calls.push({view,path})
   if(hold&&view==='status')await hold
   if(box.answer)return box.answer(view,path)
   if(box.fail)return {status:503,json:{error:'Machine unavailable'}}
   if(view==='status')return {...snapshot(),...opts.status}
   if(view==='github'&&opts.reason)return {...snapshot(),...opts.status,reason:opts.reason}
   if(view==='github'&&box.reason)return {...snapshot(),reason:box.reason}
   if(view==='github'&&box.pull)return box.pull()
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
  // `box.offline` makes the next health read say the machine stopped reporting, as a machine that went away would.
  '/api/v2/machines/health':()=>({serverNow:Date.now(),machines:[{...machine,state:box.offline?'disconnected':'connected',connection:{connected:!box.offline,reportFresh:!box.offline,reportAt:Date.now(),contactAt:Date.now(),missedProbes:0,reconnects:0,path:'direct'}}]}),
  // Counted, so a test can tell that the workspace read its session list again.
  '/api/v2/terminal/sessions':()=>{box.lists++;return {sessions:opts.second?[first,second]:[first]}},
  '/api/v2/folder-projects':{projects:[{id:'p1',name:'reet',host:'a1',machine:'studio',dir:session.dir,isRepo:true,baseBranch:'main',worktreeDefault:'inherit',colour:'green',defaultTool:'opencode',defaultModel:'',hasStarterPrompt:false,note:'',position:0,createdAt:1,updatedAt:1,version:1}],savedWaiting:0},
  '/api/v2/projects':{projects:[]},
  [`/api/v2/terminal/sessions/${session.id}/seen`]:{},
  [`/api/v2/terminal/sessions/${session.id}/skills`]:{skills:[],machineSeen:Date.now(),host:'studio'},
  '/api/v2/yolo':{allowed:false},
  [`/api/v2/terminal/sessions/${session.id}/inspect`]:inspect,
 }
 const opened=await openWorkspace(browser,{width,height:1000,touch:width<800,theme,routes})
 await opened.page.addInitScript(({delay,thumb})=>{
  class Stream {
   constructor(){this.listeners={};window.__insightStream=this;setTimeout(()=>{this.onopen?.({});this.emit('reset',{truncated:false});this.emit('chat',{seq:1,kind:'agent',messageId:'reply',text:'The workspace is connected. I am checking the browser layout.'});this.emit('chat',{seq:2,kind:'tool',toolCallId:'done',title:'Read session component',toolKind:'read',status:'completed'});this.emit('chat',{seq:3,kind:'tool',toolCallId:'current',title:'Run browser checks',toolKind:'execute',status:'in_progress',command:'npm run check'});this.emit('chat',{seq:4,kind:'status',state:'working'})},delay)}
   addEventListener(type,fn){(this.listeners[type]??=[]).push(fn)}
   emit(type,data){for(const fn of this.listeners[type]??[])fn({data:JSON.stringify(data)})}
   close(){}
  }
  window.EventSource=Stream
  // A thumbnail the browser kept when the prompt's image was sent, so the full prompt can show a picture instead of its path.
  if(thumb)localStorage.setItem('reet.chat.thumbs.v1',JSON.stringify({order:[thumb.path],map:{[thumb.path]:{n:thumb.name,u:thumb.url}}}))
 },{delay:opts.streamDelay||0,thumb:opts.thumb||null})
 await opened.page.goto(`${base}/sessions?agent=a1&session=${session.id}`,{timeout:120000})
 const panel=opened.page.getByRole('complementary',{name:'Session context'})
 // Details is optional on every screen; an existing saved choice still takes precedence.
 await opened.page.locator('.session-screen header.head').waitFor({state:'visible',timeout:60000})
 const exitFocus=opened.page.getByRole('button',{name:'Exit focus mode',exact:true})
 if(await exitFocus.count())await exitFocus.click()
 const details=opened.page.getByRole('button',{name:'Session details panel',exact:true})
 if(await details.getAttribute('aria-expanded')!=='true')await details.click()
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
  await expect(panel.getByText('Since your last visit')).toHaveCount(0)
  // Gaffer's entry sits in the panel rather than a floating column beside the workspace.
  await expect(panel.getByRole('button',{name:'Ask Gaffer'})).toBeVisible()
  await expect(page.locator('.gaffer-launcher')).toBeHidden()
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
  await page.keyboard.press('Escape');await expect(panel).not.toBeVisible();await expect(page.getByRole('button',{name:'Session details panel',exact:true})).toBeFocused()
  expect(errors).toEqual([]);await context.close()
 }
 // The overview keeps every section where it is while the current task changes: only the text inside the live slots changes.
 for(const width of [1600,375]){
  const steady=await open(width,'dark'),{page:sp,panel:sw}=steady
  await expect(sw.getByText('3 changed files',{exact:true})).toBeVisible()
  await expect(sw.getByText('Latest agent update')).toBeVisible()
  // The pull request card is taller than the loading row it replaces, once; the baseline is taken after that.
  await expect(sw.getByRole('group',{name:'Pull request'})).toContainText('#42')
  const pinned=()=>sw.evaluate(el=>{const top=n=>Math.round(n.getBoundingClientRect().top),h=name=>[...el.querySelectorAll('.panel-body h4')].find(n=>n.textContent.trim().toLowerCase()===name);return{changes:top(h('workspace changes')),where:top(h("where it's working")),details:top(el.querySelector('.session-details > summary'))}})
  const settle=()=>sp.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  const baseline=await pinned()
  expect(Object.values(baseline),`${width}px: every block was found for the baseline`).not.toContain(null)
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
  // The pull request slot holds its height from loading to "no pull request", so only the changes card is what arrives here.
  const first=await open(1600,'dark',true,held,{reason:'no_pr'})
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
  const details=page.getByRole('button',{name:'Session details panel',exact:true})
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
 // A prompt that carried an image shows the picture, not the upload path, and the picture opens at full size.
 {
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{session:{promptExcerpt:IMAGE_PROMPT,promptMore:false},prompt:()=>({prompt:IMAGE_PROMPT}),thumb:{path:IMAGE_PATH,name:'shot.png',url:THUMB_URL}})
  const card=panel.getByRole('group',{name:'You asked'})
  await expect(card).toContainText('Show me the screenshot')
  await expect(card).not.toContainText('img-shot.png')
  await card.getByRole('button',{name:'Read full prompt'}).click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog).toContainText('Show me the screenshot')
  await expect(dialog).not.toContainText('launch-uploads')
  const picture=dialog.getByRole('button',{name:'Preview shot.png'})
  await expect(picture).toBeVisible()
  await picture.click()
  const lightbox=page.getByRole('dialog',{name:'Image preview: shot.png'})
  await expect(lightbox).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(lightbox).toBeHidden()
  await expect(dialog).toBeVisible()
  expect(errors).toEqual([]);await context.close()
 }
 // A refresh of the session list hands the card a fresh copy of the same session: the open dialog, its whole text and the focus stay.
 // (The workspace refreshes every 15 s and after every action, so a dialog that closed on it would vanish while being read.)
 for(const width of [1600,375]){
  const {page,context,panel,errors,box,prompts}=await open(width,'dark')
  await panel.getByRole('button',{name:'Read full prompt'}).click()
  const dialog=page.getByRole('dialog',{name:'You asked'})
  await expect(dialog).toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
  await dialog.getByRole('button',{name:'Copy prompt'}).focus()
  for(let round=0;round<2;round++){
   const before=box.lists
   // The workspace's own refresh, as its 15 s poll and a reconnect run it.
   await page.evaluate(()=>window.dispatchEvent(new Event('online')))
   await expect.poll(()=>box.lists,`${width}px: the session list was read again`).toBeGreaterThan(before)
   await page.waitForTimeout(400)
   await expect(dialog,`${width}px: the dialog closed when the session list refreshed`).toBeVisible()
   await expect(dialog).toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
   await expect(dialog.getByRole('button',{name:'Copy prompt'})).toBeFocused()
  }
  expect(prompts,`${width}px: the full prompt was read once`).toEqual(['session-insights-001'])
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden();await expect(panel.getByRole('button',{name:'Read full prompt'})).toBeFocused()
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
 // The fixtures start a session an hour (or three) before now. The line leads with a bare HH:MM only when that start is on
 // today's date; in the first hours after midnight it correctly adds the day ("Nov 8, 23:49"), so the tests accept both.
 // runClock.test.mjs pins each form against a fixed clock.
 const startedAt=String.raw`(?:[A-Za-z]+ \d{1,2}, |\d{1,2} [A-Za-z]+, )?\d\d:\d\d`
 {
  const {context,panel,errors}=await open(1600,'dark')
  const line=panel.locator('.run-line')
  await expect(line).toHaveText(new RegExp(`^Started ${startedAt} · running 1h \\d+m$`))
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
  await expect(lost.panel.locator('.run-line')).toHaveText(new RegExp(`^Started ${startedAt}$`))
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
 // What this session started from: the notice the machine's base report earned, kept with the session. A base that matched
 // GitHub says nothing, so the row is not there at all.
 {
  const started={baseName:'main',baseRelation:'ahead',baseStart:'abc1234def5678901234567890abcdef12345678'}
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{session:started})
  const from=panel.locator('.where > div').filter({has:page.getByText('Started from',{exact:true})})
  await expect(from).toContainText("main on studio has commits GitHub doesn't have. This session starts from the local main (abc1234).")
  expect(errors).toEqual([]);await context.close()
  const agreed=await open(1600,'dark',true,null,{session:{...started,baseRelation:'same'}})
  await expect(agreed.panel.locator('.where')).not.toContainText('Started from')
  expect(agreed.errors).toEqual([]);await agreed.context.close()
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

 // The stage tracker and the Next line: five steps, each with an icon and a word, one Next sentence, and honest "not read".
 {
  const detail = (over={}) => ({detailKnown:true,reviews:{approved:0,changesRequested:0,commented:0,requested:0},threads:{open:0,resolved:0},...over})
  const WORD = {done:'Done',current:'Now',attention:'Needs you',upcoming:'Not yet',unknown:'Not read'}
  const PR = 'https://github.com/example/project/pull/42', RUN = 'https://github.com/example/project/actions/runs/1'
  const scenarios = [
   ['conflicts',{pull:{mergeable:'CONFLICTING',review:'REVIEW_REQUIRED',checks:[ok('Build')],base:'main',...detail()}},['done','done','done','current','attention'],'merge','Conflicts with main.',['Open pull request',PR],'Ask the agent to fix the conflicts'],
   ['failing checks',{pull:{mergeable:'MERGEABLE',review:'REVIEW_REQUIRED'}},['done','done','attention','current','upcoming'],'checks','1 check failing.',['Open failing check',RUN],'Ask the agent to fix the checks'],
   ['running checks',{pull:{mergeable:'MERGEABLE',review:'APPROVED',checks:[{name:'Build',state:'IN_PROGRESS'},{name:'Lint',state:'QUEUED'},ok('Unit tests')],...detail()}},['done','done','current','done','upcoming'],'checks','Waiting on Build and Lint.',null,null],
   ['approved with no open threads',{pull:{mergeable:'MERGEABLE',review:'APPROVED',checks:[ok('Build'),ok('Unit tests')],...detail({threads:{open:0,resolved:2}})}},['done','done','done','done','current'],'merge','Ready. GitHub decides whether it can merge.',['Open pull request',PR],null],
   ['no review decision and none requested',{pull:{mergeable:'MERGEABLE',review:'',checks:[ok('Build')],...detail()}},['done','done','done','done','current'],'merge','Ready. GitHub decides whether it can merge.',['Open pull request',PR],null],
   ['open review threads',{pull:{mergeable:'MERGEABLE',review:'REVIEW_REQUIRED',checks:[ok('Build')],...detail({reviews:{approved:0,changesRequested:0,commented:0,requested:1},threads:{open:3,resolved:2}})}},['done','done','done','attention','upcoming'],'review','3 unresolved review threads.',['Open review',PR],'Ask the agent to address the threads'],
   ['an older machine',{pull:{mergeable:'MERGEABLE',review:'APPROVED',checks:[ok('Build')]}},['done','done','done','current','upcoming'],'review','Nothing needs you right now.',null,null],
   ['github unavailable',{reason:'github_unavailable'},['current','unknown','unknown','unknown','unknown'],'code','GitHub status is not available on this machine.',null,null],
   ['no pull request',{reason:'no_pr'},['current','upcoming','upcoming','upcoming','upcoming'],'code','Nothing needs you right now.',null,null],
  ]
  for(const [name,opts,states,current,next,primary,fix] of scenarios){
   const {page,context,panel,errors}=await open(1600,'dark',true,null,opts)
   const tracker=panel.getByRole('group',{name:'Progress'})
   await expect(tracker.locator('.next')).toContainText(next)
   const steps=await tracker.locator('ol > li').evaluateAll(els=>els.map(e=>({key:e.dataset.step,state:e.dataset.state,word:e.querySelector('.word').textContent.trim(),current:e.getAttribute('aria-current')})))
   expect(steps.map(x=>x.key),`${name}: the five steps`).toEqual(['code','pr','checks','review','merge'])
   expect(steps.map(x=>x.state),`${name}: step states`).toEqual(states)
   expect(steps.map(x=>x.word),`${name}: every state has a word`).toEqual(states.map(x=>WORD[x]))
   // The step announced is the one the Next line is about: the first that needs the person, else the first in progress.
   expect(steps.filter(x=>x.current).map(x=>[x.key,x.current]),`${name}: aria-current marks one step`).toEqual([[current,'step']])
   if(primary){
    const link=tracker.locator('.next').getByRole('link',{name:primary[0]})
    await expect(link).toHaveAttribute('href',primary[1]);await expect(link).toHaveAttribute('target','_blank');await expect(link).toHaveAttribute('rel',/noopener noreferrer/)
   } else await expect(tracker.locator('.next a')).toHaveCount(0)
   const fixButton=tracker.getByRole('button',{name:/^Ask the agent to (fix|address)/})
   if(fix){
    await expect(fixButton).toHaveAccessibleName(fix);await expect(fixButton).toHaveText(fix.replace(/ the \w+$/,''))
    // The shortcut only calls its callback for now: nothing is sent, read or opened.
    const seen=[];page.on('request',r=>{if(r.url().includes('/api/'))seen.push(r.method()+' '+new URL(r.url()).pathname)})
    await fixButton.focus();await page.keyboard.press('Enter');await page.waitForTimeout(250)
    expect(seen,`${name}: asking the agent to fix sends nothing`).toEqual([])
   } else await expect(fixButton).toHaveCount(0)
   // Every link in the tracker and the card is https with a safe rel.
   const links=await panel.locator('.tracker a, .pull a').evaluateAll(els=>els.map(a=>({href:a.href,rel:a.rel,target:a.target})))
   for(const l of links){expect(l.href.startsWith('https:'),`${name}: ${l.href}`).toBe(true);expect(l.rel).toContain('noopener');expect(l.rel).toContain('noreferrer');expect(l.target).toBe('_blank')}
   expect(errors).toEqual([]);await context.close()
  }
  // A merged pull request collapses the tracker and the Next line into one row of the same total height.
  {
   const live=await open(1600,'dark',true,null,{pull:{mergeable:'MERGEABLE',review:'REVIEW_REQUIRED',checks:[ok('Build')],...detail()}})
   const tracker=live.panel.getByRole('group',{name:'Progress'}),where=live.panel.getByRole('heading',{name:"Where it's working"})
   await expect(tracker.locator('ol > li')).toHaveCount(5)
   const expanded={tracker:Math.round((await tracker.boundingBox()).height),where:Math.round((await where.boundingBox()).y)}
   live.box.pull=()=>({...snapshot(),pull:{...pull().pull,state:'MERGED',mergeable:'UNKNOWN',checks:[],mergedAt:Date.now()-12*60000,...detail()}})
   await live.panel.getByRole('button',{name:'Refresh session workspace'}).click()
   const row=tracker.locator('.merged')
   await expect(row).toContainText('Merged');await expect(row).toContainText('12m ago')
   await expect(tracker.locator('ol')).toHaveCount(0)
   const link=row.getByRole('link',{name:'Open pull request'})
   await expect(link).toHaveAttribute('href',PR);await expect(link).toHaveAttribute('rel',/noopener noreferrer/)
   expect({tracker:Math.round((await tracker.boundingBox()).height),where:Math.round((await where.boundingBox()).y)},'merging moved the blocks below').toEqual(expanded)
   // Without a merge time the row says nothing about when.
   live.box.pull=()=>({...snapshot(),pull:{...pull().pull,state:'MERGED',mergeable:'UNKNOWN',checks:[]}})
   await live.panel.getByRole('button',{name:'Refresh session workspace'}).click()
   await expect(row).not.toContainText('ago');await expect(row).toContainText('Merged')
   expect(live.errors).toEqual([]);await live.context.close()
  }
 }
 // A GitHub read the machine can no longer vouch for is marked "Last known" on the card and beside the Next line; a fresh one is not.
 {
  const {page,context,panel,errors,box}=await open(1600,'dark')
  const card=panel.getByRole('group',{name:'Pull request'}),next=panel.getByRole('group',{name:'Progress'}).locator('.next')
  await expect(card).toContainText('#42')
  await expect(card).not.toContainText('Last known');await expect(next).not.toContainText('Last known')
  const before=box.lists
  box.offline=true;await page.evaluate(()=>window.dispatchEvent(new Event('online')))
  await expect.poll(()=>box.lists).toBeGreaterThan(before)
  await expect(panel.locator('.connection')).toHaveText('Machine offline')
  await expect(card.locator('.ident')).toContainText('Last known · checked just now')
  // The Next line still says what was last true, marked so it does not read as current.
  await expect(next).toContainText('Last known · checked just now');await expect(next).toContainText('Conflicts with')
  expect(errors).toEqual([]);await context.close()
 }
 {
  const {context,panel,errors}=await open(1600,'dark',true,null,{status:{checkedAt:Date.now()-12*60000}})
  const card=panel.getByRole('group',{name:'Pull request'}),next=panel.getByRole('group',{name:'Progress'}).locator('.next')
  await expect(card).toContainText('#42')
  await expect(card.locator('.ident')).toContainText('Last known · checked 12m ago')
  await expect(next).toContainText('Last known · checked 12m ago')
  expect(errors).toEqual([]);await context.close()
 }
 // At 14px the five steps sit side by side from 340 px up; a cell is too narrow for "Planning" only in a panel of about 306 to 353 px,
 // where the step reads "Plan", and the 300 px panel sets them three over two, wide enough for "Planning" again. No word is ever cut.
 for(const [width,word,rows] of [[300,'Planning',2],[340,'Plan',1],[360,'Planning',1],[520,'Planning',1]]){
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{session:{agentMode:'plan'}})
  await page.locator('.session-screen').evaluate((el,w)=>el.style.setProperty('--context-width',w+'px'),width)
  const tracker=panel.getByRole('group',{name:'Progress'}),label=tracker.locator('li[data-step=code] .label')
  await expect(label).toBeVisible()
  await page.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  expect(await label.evaluate(e=>e.innerText),`${width}px: the planning step's label`).toBe(word)
  const cut=await tracker.locator('li .label').evaluateAll(els=>els.filter(e=>e.scrollWidth>e.clientWidth).map(e=>e.innerText))
  expect(cut,`${width}px: a step label is cut off`).toEqual([])
  const words=await tracker.locator('li .word').evaluateAll(els=>els.filter(e=>e.scrollWidth>e.clientWidth||e.scrollHeight>e.clientHeight).map(e=>e.innerText))
  expect(words,`${width}px: a step's state word is cut off`).toEqual([])
  const tops=await tracker.locator('ol > li').evaluateAll(els=>new Set(els.map(e=>Math.round(e.getBoundingClientRect().top))).size)
  expect(tops,`${width}px: the steps' rows`).toBe(rows)
  expect(errors).toEqual([]);await context.close()
 }
 // Asking the agent to fix: the message goes into the chat composer for the person to read and send, or onto the clipboard. Nothing is sent for them.
 {
  const detail = {detailKnown:true,reviews:{approved:0,changesRequested:0,commented:0,requested:0},threads:{open:0,resolved:0}}
  const conflicted = {pull:{mergeable:'CONFLICTING',review:'REVIEW_REQUIRED',checks:[ok('Build')],base:'main',...detail}}
  const MESSAGE = 'Resolve the merge conflicts with main on this branch, then push.'
  const ready = async opts => {
   const o = await open(1600,'dark',true,null,opts)
   await o.context.grantPermissions(['clipboard-read','clipboard-write'])
   const tracker = o.panel.getByRole('group',{name:'Progress'})
   const requests = []
   const watch = () => o.page.on('request',r=>{requests.push(r.method()+' '+new URL(r.url()).pathname)})
   return {...o,tracker,requests,watch,composer:o.page.locator('.composer-input')}
  }
  // Nothing but reads: no write method, and no route that sends a chat message.
  const onlyReads = (requests,name) => {
   expect(requests.filter(r=>!/^(GET|HEAD) /.test(r)),`${name}: nothing is written`).toEqual([])
   expect(requests.filter(r=>/chat|message|prompt/i.test(r)&&!/^GET /.test(r)),`${name}: no message is sent`).toEqual([])
  }
  {
   const t = await ready(conflicted)
   await expect(t.composer).toBeVisible()
   t.watch()
   const fix = t.tracker.getByRole('button',{name:'Ask the agent to fix the conflicts'})
   await expect(fix).toHaveText('Ask the agent to fix')
   await fix.click()
   await expect(t.composer).toHaveValue(MESSAGE)
   await expect(t.composer).toBeFocused()
   await t.page.waitForTimeout(300)
   onlyReads(t.requests,'a chat with an empty composer')
   expect(t.errors).toEqual([]);await t.context.close()
  }
  {
   // On a phone the panel is a sheet over the chat: a filled composer the person cannot see would be no help, so the sheet goes.
   const t = await open(375,'dark',true,null,conflicted)
   const tracker = t.panel.getByRole('group',{name:'Progress'}), composer = t.page.locator('.composer-input')
   await tracker.getByRole('button',{name:'Ask the agent to fix the conflicts'}).click()
   await expect(t.panel,'the Details sheet stayed over the filled composer').toBeHidden()
   await expect(composer).toBeVisible();await expect(composer).toHaveValue(MESSAGE);await expect(composer).toBeFocused()
   expect(t.errors).toEqual([]);await t.context.close()
  }
  {
   // A copy on the phone keeps the sheet open, so the note saying what happened stays in sight.
   const t = await open(375,'dark',true,null,conflicted)
   await t.context.grantPermissions(['clipboard-read','clipboard-write'])
   const tracker = t.panel.getByRole('group',{name:'Progress'})
   await t.page.locator('.composer-input').evaluate((el,v)=>{el.value=v;el.dispatchEvent(new Event('input',{bubbles:true}))},'x'.repeat(1990))
   await tracker.getByRole('button',{name:'Ask the agent to fix the conflicts'}).click()
   await expect(tracker.getByRole('status')).toHaveText('Copied. The composer is nearly full.')
   await expect(t.panel).toBeVisible()
   expect(t.errors).toEqual([]);await t.context.close()
  }
  {
   const t = await ready(conflicted)
   await t.composer.fill('Also check the lockfile')
   t.watch()
   await t.tracker.getByRole('button',{name:'Ask the agent to fix the conflicts'}).click()
   await expect(t.composer).toHaveValue('Also check the lockfile\n'+MESSAGE)
   await expect(t.composer).toBeFocused()
   onlyReads(t.requests,'a half-typed draft')
   expect(t.errors).toEqual([]);await t.context.close()
  }
  {
   // Asking twice, slowly or in a blink, leaves the message in the composer once and never reports it as too full.
   for(const quick of [false,true]){
    const t = await ready(conflicted)
    t.watch()
    const fix = t.tracker.getByRole('button',{name:'Ask the agent to fix the conflicts'})
    if(quick){await fix.dblclick()} else {await fix.click();await expect(t.composer).toHaveValue(MESSAGE);await fix.click()}
    await t.page.waitForTimeout(700)
    await expect(t.composer,`${quick?'two quick clicks':'two clicks'}: the message is there once`).toHaveValue(MESSAGE)
    await expect(t.tracker.getByRole('status')).toHaveText('')
    await expect(t.composer).toBeFocused()
    onlyReads(t.requests,'repeat clicks')
    expect(t.errors).toEqual([]);await t.context.close()
   }
  }
  {
   const t = await ready(conflicted)
   const draft = 'x'.repeat(1990)
   await t.composer.fill(draft)
   t.watch()
   await t.tracker.getByRole('button',{name:'Ask the agent to fix the conflicts'}).click()
   const note = t.tracker.getByRole('status')
   await expect(note).toHaveText('Copied. The composer is nearly full.')
   await expect(note).toHaveAttribute('aria-live','polite')
   expect(await t.page.evaluate(()=>navigator.clipboard.readText())).toBe(MESSAGE)
   await expect(t.composer).toHaveValue(draft)
   onlyReads(t.requests,'a nearly full composer')
   expect(t.errors).toEqual([]);await t.context.close()
  }
  {
   const t = await ready({...conflicted,session:{structured:false}})
   // A terminal asks for its own attach ticket as it opens; that is the page loading, not the click, so let it finish first.
   await t.page.waitForTimeout(2000)
   t.watch()
   const fix = t.tracker.getByRole('button',{name:/conflicts/})
   await expect(fix).toHaveText('Copy message for the agent')
   await fix.click()
   await expect(t.tracker.getByRole('status')).toHaveText('Copied')
   expect(await t.page.evaluate(()=>navigator.clipboard.readText())).toBe(MESSAGE)
   onlyReads(t.requests,'a terminal session')
   await t.context.close()
  }
 }
 // The pull request card: facts only, "not read" where the machine did not read, and a placeholder when there is no pull request.
 {
  const detail = (over={}) => ({detailKnown:true,reviews:{approved:1,changesRequested:1,commented:0,requested:0},threads:{open:3,resolved:2},...over})
  const withPull = over => open(1600,'dark',true,null,{pull:{mergeable:'CONFLICTING',review:'CHANGES_REQUESTED',base:'main',lastPushAt:Date.now()-4*60000,...detail(),...over}})
  {
   const {page,context,panel,errors,box,calls}=await withPull({})
   const card=panel.getByRole('group',{name:'Pull request'}),fold=card.locator('details.more'),summary=fold.locator('summary')
   const isOpen=()=>fold.evaluate(d=>d.open)
   // Folded by default: the summary holds what a person scans for, and the rest waits behind Details.
   await expect(card).toContainText('#42 · open');await expect(card.locator('.title')).toHaveText('Improve the session workspace')
   await expect(card.locator('.title')).toHaveAttribute('title','Improve the session workspace')
   await expect(card).toContainText('Resolve merge conflicts')
   await expect(card.getByRole('img',{name:'Checks: 1 failing, 1 running, 1 passed',exact:true})).toBeVisible()
   await expect(card.locator('.summary-facts .review')).toHaveText('Changes requested')
   await expect(card.locator('.summary-facts .conflicts')).toHaveText('Conflicts')
   await expect(card.getByRole('link',{name:'Open pull request'})).toHaveAttribute('href','https://github.com/example/project/pull/42')
   expect(await isOpen(),'the Details fold starts closed').toBe(false)
   await expect(summary).toHaveText('Details');await expect(summary).toBeVisible()
   for(const hidden of [card.getByText('1 failing',{exact:true}),card.getByText('1 approved · changes requested by 1'),card.getByText('3 open threads'),card.getByText('pushed 4m ago'),card.getByRole('link',{name:'Open failing check'}),card.getByRole('link',{name:'Open review'}),card.getByRole('button',{name:'Copy pull request link'}),card.getByRole('button',{name:'Full detail'})])await expect(hidden,'a secondary fact or action shows while the fold is closed').toBeHidden()
   // Details opens the fold with the keyboard as well as the pointer, and holds every secondary fact and action.
   await summary.focus();await page.keyboard.press('Enter')
   await expect.poll(isOpen,'Details opened the fold').toBe(true)
   await expect(card.getByText('1 failing',{exact:true})).toBeVisible();await expect(card.getByText('1 running',{exact:true})).toBeVisible();await expect(card.getByText('1 passed',{exact:true})).toBeVisible()
   await expect(card.getByRole('img',{name:'1 failing, 1 running, 1 passed',exact:true})).toBeVisible()
   await expect(card.getByText('1 approved · changes requested by 1')).toBeVisible()
   await expect(card.getByText('3 open threads')).toBeVisible()
   await expect(card.getByText('Conflicts with main')).toBeVisible()
   await expect(card).toContainText('+45');await expect(card).toContainText('13')
   await expect(card.getByText('pushed 4m ago')).toBeVisible()
   await expect(card).not.toContainText('can merge')
   await expect(card.getByRole('link',{name:'Open failing check'})).toHaveAttribute('href','https://github.com/example/project/actions/runs/1')
   await expect(card.getByRole('link',{name:'Open review'})).toHaveAttribute('href','https://github.com/example/project/pull/42')
   await context.grantPermissions(['clipboard-read','clipboard-write'])
   await card.getByRole('button',{name:'Copy pull request link'}).click()
   expect(await panel.evaluate(()=>navigator.clipboard.readText())).toBe('https://github.com/example/project/pull/42')
   // The fold is the person's: neither a refresh nor the workspace's own poll closes it, while the facts inside it move on.
   box.pull=()=>({...snapshot(),pull:{...pull().pull,mergeable:'CONFLICTING',review:'CHANGES_REQUESTED',base:'main',...detail({threads:{open:5,resolved:2}})}})
   await panel.getByRole('button',{name:'Refresh session workspace'}).click()
   await expect(card.getByText('5 open threads')).toBeVisible()
   expect(await isOpen(),'a refresh closed the Details fold').toBe(true)
   // The Overview's own GitHub poll, once its cadence has passed.
   const reads=()=>calls.filter(c=>c.view==='github').length,before=reads()
   box.pull=()=>({...snapshot(),pull:{...pull().pull,mergeable:'CONFLICTING',review:'CHANGES_REQUESTED',base:'main',...detail({threads:{open:6,resolved:2}})}})
   await page.clock.install();await page.clock.runFor(130000)
   await expect.poll(reads,'the Overview polled GitHub').toBeGreaterThan(before)
   await expect(card.getByText('6 open threads')).toBeVisible()
   expect(await isOpen(),'a poll closed the Details fold').toBe(true)
   await summary.click();await expect.poll(isOpen,'Details closes the fold again').toBe(false)
   await summary.click();await expect.poll(isOpen).toBe(true)
   await card.getByRole('button',{name:'Full detail'}).click()
   await expect(panel.getByRole('tab',{name:/^GitHub/})).toHaveAttribute('aria-selected','true')
   await expect(panel.getByText('Resolve merge conflicts')).toBeVisible()
   expect(errors).toEqual([]);await context.close()
  }
  {
   const {context,panel,errors}=await withPull({detailKnown:undefined,reviews:undefined,threads:undefined,lastPushAt:undefined,review:'APPROVED',mergeable:'MERGEABLE'})
   const card=panel.getByRole('group',{name:'Pull request'})
   await expect(card.locator('.summary-facts .review')).toHaveText('Approved')
   // The honest words are one tap away, never replaced by a count the machine did not read.
   await card.locator('summary').click()
   await expect(card.getByText('Review threads not read on this machine')).toBeVisible()
   await expect(card).not.toContainText('No open threads');await expect(card).not.toContainText('0 open')
   await expect(card).not.toContainText('pushed')
   await expect(card.getByRole('link',{name:'Open review'})).toHaveCount(0)
   expect(errors).toEqual([]);await context.close()
  }
  {
   const {context,panel,errors}=await withPull({threads:{open:0,resolved:0,more:true},review:'APPROVED',mergeable:'MERGEABLE'})
   const card=panel.getByRole('group',{name:'Pull request'})
   await card.locator('summary').click()
   await expect(card.getByText('100+ threads, not all read')).toBeVisible()
   expect(errors).toEqual([]);await context.close()
  }
  for(const [reason,text] of [['no_pr','No pull request yet.'],['github_unavailable','GitHub status is not available on this machine.'],['timed_out','GitHub status could not be read.']]){
   const {context,panel,errors}=await open(1600,'dark',true,null,{reason})
   const card=panel.getByRole('group',{name:'Pull request'})
   await expect(card).toContainText(text)
   expect(Math.round((await card.locator('.placeholder').boundingBox()).height),`${reason}: the placeholder row is 76px, as tall as the changes card`).toBe(76)
   expect(errors).toEqual([]);await context.close()
  }
  {
   const {context,panel,errors}=await open(1600,'dark',false)
   await expect(panel.getByRole('group',{name:'Pull request'})).toContainText('Update reet on this machine for GitHub status.')
   await expect(panel.getByRole('group',{name:'Progress'}).locator('.next')).toContainText('Update reet on this machine for GitHub status.')
   expect(errors).toEqual([]);await context.close()
  }
 }
 // The Overview reads GitHub once per cadence for a visible session, and not at all on its own with Live updates off.
 {
  const live=await open(1600,'dark')
  await expect(live.panel.getByRole('group',{name:'Pull request'})).toContainText('#42')
  const gh=()=>live.calls.filter(c=>c.view==='github').length
  await live.page.waitForTimeout(400)
  expect(gh(),'the panel and the navigation share the first read').toBe(1)
  await live.page.clock.install()
  await live.page.clock.runFor(130000)
  await expect.poll(gh).toBe(2)
  await live.page.clock.runFor(10000);await live.page.waitForTimeout(200)
  expect(gh(),'one read per cadence').toBe(2)
  await live.panel.getByRole('checkbox',{name:'Live updates'}).uncheck()
  await live.page.clock.runFor(700000);await live.page.waitForTimeout(300)
  expect(gh(),'with Live updates off the Overview makes no further GitHub reads').toBe(2)
  await live.page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')))
  await live.page.waitForTimeout(300)
  expect(gh(),'coming back to the tab does not read either').toBe(2)
  // A manual refresh still reads at once.
  await live.panel.getByRole('button',{name:'Refresh session workspace'}).click()
  await expect.poll(gh).toBe(3)
  expect(live.errors).toEqual([]);await live.context.close()
 }
 // A warm store: the navigation has already read the other session's pull request, nothing is in flight, and opening its panel reads nothing more.
 {
  const warm=await open(1600,'dark',true,null,{second:true})
  const gh=()=>warm.calls.filter(c=>c.view==='github').length
  await expect.poll(gh).toBe(2)
  await warm.page.waitForTimeout(500)
  expect(gh(),'one read each for the two sessions').toBe(2)
  await warm.page.getByRole('button',{name:/Second session/}).first().click()
  await expect(warm.panel.getByRole('group',{name:'You asked'})).toContainText('SECOND-SESSION-PROMPT')
  await expect(warm.panel.getByRole('group',{name:'Pull request'})).toContainText('#42')
  await warm.page.waitForTimeout(700)
  expect(gh(),'opening a session whose pull request was read moments ago makes no extra read').toBe(2)
  expect(warm.errors).toEqual([]);await warm.context.close()
 }
 // Nothing below the tracker moves when the pull request changes, at the panel widths and on the phone sheet, in both themes,
 // and the folded pull request card is one height in every state that has a pull request.
 for(const theme of ['dark','light']) for(const width of [300,340,360,520,375]){
  const phone=width===375
  const {page,context,panel,errors,box}=await open(phone?375:1600,theme,true,null,{pull:{mergeable:'MERGEABLE',review:'REVIEW_REQUIRED',checks:[ok('Build')],detailKnown:true,reviews:{approved:0,changesRequested:0,commented:0,requested:1},threads:{open:0,resolved:0}}})
  if(!phone)await page.locator('.session-screen').evaluate((el,w)=>el.style.setProperty('--context-width',w+'px'),width)
  await expect(panel.getByRole('group',{name:'Pull request'})).toContainText('#42')
  const settle=()=>page.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  const pinned=()=>panel.evaluate(el=>{const top=n=>Math.round(n.getBoundingClientRect().top),h=name=>[...el.querySelectorAll('.panel-body h4')].find(n=>n.textContent.trim().toLowerCase()===name);return{tracker:top(h('progress')),pull:top(h('pull request')),changes:top(h('workspace changes')),where:top(h("where it's working")),details:top(el.querySelector('.session-details > summary'))}})
  const baseline=await pinned()
  expect(Object.values(baseline),'every block was found for the baseline').not.toContain(null)
  const folded=()=>panel.evaluate(el=>Math.round(el.querySelector('.pull').getBoundingClientRect().height))
  const cardHeight=await folded()
  const states=[
   ['conflicts',{mergeable:'CONFLICTING',base:'a-very-long-branch-name-that-cannot-possibly-fit-in-the-slot-at-three-hundred-pixels'}],
   ['failing checks with long names',{mergeable:'MERGEABLE',checks:[{name:'A check with a name long enough to need cutting off somewhere sensible',state:'FAILURE',url:'https://github.com/example/project/actions/runs/9'},{name:'Other',state:'FAILURE'},{name:'Third',state:'FAILURE'}]}],
   ['running checks',{mergeable:'MERGEABLE',checks:[{name:'Build',state:'IN_PROGRESS'},{name:'Lint',state:'QUEUED'},{name:'Unit tests with a long name here',state:'PENDING'}]}],
   ['open threads',{mergeable:'MERGEABLE',checks:[ok('Build')],threads:{open:12,resolved:1}}],
   ['a long title',{title:'Improve the session workspace so that the overview keeps every section where it is while the pull request moves on and on'}],
   ['an older machine',{detailKnown:undefined,reviews:undefined,threads:undefined,lastPushAt:undefined}],
   ['approved and ready',{mergeable:'MERGEABLE',review:'APPROVED',checks:[ok('Build')],detailKnown:true,threads:{open:0,resolved:0}}],
   ['merged',{state:'MERGED',mergeable:'UNKNOWN',checks:[],mergedAt:Date.now()-720000}],
   ['closed',{state:'CLOSED',mergeable:'UNKNOWN',checks:[]}],
   ['a draft',{state:'OPEN',draft:true,mergeable:'MERGEABLE',checks:[]}],
  ]
  for(const [label,over] of states){
   box.pull=()=>({...snapshot(),pull:{...pull().pull,...over}})
   await panel.getByRole('button',{name:'Refresh session workspace'}).click()
   await expect(panel.getByRole('button',{name:'Refresh session workspace'})).toBeEnabled()
   await settle()
   expect(await pinned(),`${theme} ${width}px: blocks moved for ${label}`).toEqual(baseline)
   expect(await folded(),`${theme} ${width}px: the folded card changed height for ${label}`).toBe(cardHeight)
  }
  // Every step still shows its icon and word, and nothing scrolls sideways or is cut by the panel.
  box.pull=()=>({...snapshot(),pull:{...pull().pull,mergeable:'CONFLICTING'}})
  await panel.getByRole('button',{name:'Refresh session workspace'}).click()
  await expect(panel.getByRole('group',{name:'Progress'}).locator('li[data-step=merge]')).toHaveAttribute('data-state','attention')
  const fits=await panel.evaluate(el=>[...el.querySelectorAll('.tracker, .pull')].every(n=>n.scrollWidth<=n.clientWidth+1))
  expect(fits,`${theme} ${width}px: the tracker or card overflows sideways`).toBe(true)
  // The fixed words (step labels and states, the Next and fix buttons, the card's buttons and its Details summary) are never cut, at
  // any of these widths, with the fold closed and open.
  const cutWords=()=>panel.evaluate(el=>[...el.querySelectorAll('.tracker li .label, .tracker li .word, .tracker .primary, .tracker .fix, .tracker .fix-note, .pull .btn, .pull .copy, .pull summary, .pull .summary-facts .review')].filter(n=>n.checkVisibility()&&(n.scrollWidth>n.clientWidth+1||n.scrollHeight>n.clientHeight+1)).map(n=>n.textContent.trim()))
  expect(await cutWords(),`${theme} ${width}px: a fixed word is cut`).toEqual([])
  // At 340 px and wider the five steps are one row and the two Next buttons share one; the 300 px panel has three steps over two.
  const layout=await panel.evaluate(el=>{const top=n=>Math.round(n.getBoundingClientRect().top);return{rows:new Set([...el.querySelectorAll('.tracker ol > li')].map(top)).size,buttons:top(el.querySelector('.tracker .primary'))===top(el.querySelector('.tracker .fix'))}})
  expect(layout,`${theme} ${width}px: the steps' rows and the Next buttons`).toEqual(width===300?{rows:2,buttons:false}:{rows:1,buttons:true})
  await panel.getByRole('group',{name:'Pull request'}).locator('summary').click()
  await expect(panel.getByRole('group',{name:'Pull request'}).getByRole('button',{name:'Full detail'})).toBeVisible()
  expect(await cutWords(),`${theme} ${width}px: a fixed word is cut in the open fold`).toEqual([])
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  if(phone){for(const target of await panel.locator('.tracker button, .tracker a, .pull button, .pull a, .pull summary').all()){const b=await target.boundingBox();if(b)expect(b.height,'a touch target under 44px').toBeGreaterThanOrEqual(43)}}
  expect(errors).toEqual([]);await context.close()
 }
 // A terminal session's Next slot is taller where its longer Copy button needs a row of its own, and the merged row must match it,
 // or merging (which arrives on a poll) would move every block below the tracker.
 for(const width of [340,360,375]){
  const phone=width===375
  const {page,context,panel,errors,box}=await open(phone?375:1600,'dark',true,null,{session:{structured:false},pull:{mergeable:'CONFLICTING',review:'REVIEW_REQUIRED',checks:[ok('Build')],base:'main'}})
  if(!phone)await page.locator('.session-screen').evaluate((el,w)=>el.style.setProperty('--context-width',w+'px'),width)
  await expect(panel.getByRole('group',{name:'Progress'}).getByRole('button',{name:/conflicts/})).toBeVisible()
  await page.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  const pinned=()=>panel.evaluate(el=>{const top=n=>Math.round(n.getBoundingClientRect().top),h=name=>[...el.querySelectorAll('.panel-body h4')].find(n=>n.textContent.trim().toLowerCase()===name);return{pull:top(h('pull request')),changes:top(h('workspace changes')),where:top(h("where it's working"))}})
  const baseline=await pinned()
  box.pull=()=>({...snapshot(),pull:{...pull().pull,state:'MERGED',mergeable:'UNKNOWN',checks:[],mergedAt:Date.now()-720000}})
  await panel.getByRole('button',{name:'Refresh session workspace'}).click()
  await expect(panel.getByRole('group',{name:'Progress'}).locator('.merged')).toBeVisible()
  await page.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  expect(await pinned(),`${phone?'phone sheet':width+'px'}: merging a terminal session's pull request moved the blocks below`).toEqual(baseline)
  expect(errors).toEqual([]);await context.close()
 }
 // The review word is never cut, even with two-digit check counts beside it and conflicts after it.
 const many=[...Array.from({length:12},(_,i)=>({name:'f'+i,state:'FAILURE'})),...Array.from({length:13},(_,i)=>({name:'r'+i,state:'IN_PROGRESS'})),...Array.from({length:32},(_,i)=>ok('p'+i))]
 for(const width of [300,340,360,520,375]){
  const phone=width===375
  const {page,context,panel,errors}=await open(phone?375:1600,'dark',true,null,{pull:{mergeable:'CONFLICTING',review:'CHANGES_REQUESTED',checks:many,base:'main'}})
  if(!phone)await page.locator('.session-screen').evaluate((el,w)=>el.style.setProperty('--context-width',w+'px'),width)
  const review=panel.getByRole('group',{name:'Pull request'}).locator('.summary-facts .review')
  await expect(review).toHaveText('Changes requested')
  await page.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  expect(await review.evaluate(n=>n.scrollWidth>n.clientWidth||n.scrollHeight>n.clientHeight),`${phone?'phone sheet':width+'px'}: the review word is cut`).toBe(false)
  expect(errors).toEqual([]);await context.close()
 }
 // The longest fix note, a failed copy, is whole at the 300 px minimum.
 {
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{session:{structured:false},pull:{mergeable:'CONFLICTING',review:'REVIEW_REQUIRED',checks:[ok('Build')],base:'main'}})
  await page.locator('.session-screen').evaluate(el=>el.style.setProperty('--context-width','300px'))
  await page.waitForTimeout(2000)
  await page.evaluate(()=>{Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:()=>Promise.reject(new Error('refused'))}});document.execCommand=()=>false})
  const tracker=panel.getByRole('group',{name:'Progress'})
  await tracker.getByRole('button',{name:/conflicts/}).click()
  const note=tracker.getByRole('status')
  await expect(note).toHaveText('Could not copy the message. Try again.')
  expect(await note.evaluate(n=>n.scrollWidth>n.clientWidth||n.scrollHeight>n.clientHeight),'300px: the failed-copy note is cut').toBe(false)
  expect(errors).toEqual([]);await context.close()
 }
 // With no pull request, or none read, the card is the one documented exception: the placeholder row, 76 px at every width, as tall
 // as the changes card. (A reason never replaces a pull request already shown, so each starts from a fresh read.)
 for(const width of [300,340,360,520,375]) for(const reason of ['github_unavailable','no_pr']){
  const phone=width===375
  const {page,context,panel,errors}=await open(phone?375:1600,'dark',true,null,{reason})
  if(!phone)await page.locator('.session-screen').evaluate((el,w)=>el.style.setProperty('--context-width',w+'px'),width)
  const placeholder=panel.getByRole('group',{name:'Pull request'}).locator('.placeholder')
  await expect(placeholder).toContainText(reason==='no_pr'?'No pull request yet.':'GitHub status is not available on this machine.')
  await page.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  expect(Math.round((await placeholder.boundingBox()).height),`${width}px: the ${reason} placeholder`).toBe(76)
  expect(errors).toEqual([]);await context.close()
 }

 // Every slot of the Overview stays put while the headline, the notice, the activity and the pull request all change, at the panel widths and on the phone sheet.
 for(const theme of ['dark','light']) for(const width of [300,340,360,520,'phone']){
  const phone=width==='phone',viewport=phone?340:1600
  const {page,context,panel,errors,box}=await open(viewport,theme,true,null,{second:true,pull:{mergeable:'MERGEABLE',review:'REVIEW_REQUIRED',checks:[ok('Build')],detailKnown:true,reviews:{approved:0,changesRequested:0,commented:0,requested:1},threads:{open:0,resolved:0}}})
  if(!phone)await page.locator('.session-screen').evaluate((el,w)=>el.style.setProperty('--context-width',w+'px'),width)
  const label=`${theme} ${phone?'phone sheet':width+'px'}`
  await expect(panel.getByRole('group',{name:'Pull request'})).toContainText('#42')
  await expect(panel.getByRole('group',{name:'You asked'})).toContainText('Rebuild the session overview')
  const settle=()=>page.evaluate(()=>new Promise(done=>requestAnimationFrame(()=>requestAnimationFrame(done))))
  const pinned=()=>panel.evaluate(el=>{const top=n=>n?Math.round(n.getBoundingClientRect().top):null,h=name=>[...el.querySelectorAll('.panel-body h4')].find(n=>n.textContent.trim().toLowerCase()===name);return{run:top(el.querySelector('.run-line')),notice:top(el.querySelector('.session-summary .status-line')),facts:top(h('in progress')),latest:top(h('latest agent update')),prompt:top(h('you asked')),tracker:top(h('progress')),pull:top(h('pull request')),changes:top(h('workspace changes')),where:top(h("where it's working")),details:top(el.querySelector('.session-details > summary'))}})
  const baseline=await pinned()
  expect(Object.values(baseline),'every block was found for the baseline').not.toContain(null)
  const long='Allow the agent to run a command that rewrites several generated files across the whole repository before the build starts?'
  const tool=(id,title,status,extra={})=>({kind:'tool',toolCallId:id,title,toolKind:'edit',status,...extra})
  // One change per round, each followed by a settle and a comparison of every block against the baseline.
  const rounds=[
   ['a long task title',async()=>{await page.evaluate(e=>window.__insightStream.emit('chat',e),{seq:50,...tool('x1','Edit web/src/lib/sessions/ContextPanel.svelte so the overview keeps every section where it is','in_progress',{path:'a.ts'})})}],
   ['a long agent update',async()=>{await page.evaluate(e=>window.__insightStream.emit('chat',e),{seq:51,kind:'agent',messageId:'long2',text:Array.from({length:14},(_,i)=>`Paragraph ${i+1} of a long update that wraps in a narrow panel.`).join('\n\n')})}],
   ['the agent asks permission',async()=>{await page.evaluate(e=>window.__insightStream.emit('chat',e),{seq:52,kind:'permission',permissionId:'ask9',title:long,toolKind:'execute'})}],
   ['the chat reports an error',async()=>{await page.evaluate(e=>window.__insightStream.emit('chat',e),{seq:53,kind:'status',state:'error',detail:long+' '+long})}],
   ['a pull request with conflicts and a long base',async()=>{box.pull=()=>({...snapshot(),pull:{...pull().pull,mergeable:'CONFLICTING',base:'a-very-long-branch-name-that-cannot-possibly-fit-in-the-slot-at-three-hundred-pixels'}});await panel.getByRole('button',{name:'Refresh session workspace'}).click()}],
   ['failing checks with long names',async()=>{box.pull=()=>({...snapshot(),pull:{...pull().pull,mergeable:'MERGEABLE',checks:[{name:'A check with a name long enough to need cutting off somewhere sensible',state:'FAILURE',url:'https://github.com/example/project/actions/runs/9'},{name:'Other',state:'FAILURE'}]}});await panel.getByRole('button',{name:'Refresh session workspace'}).click()}],
   ['the pull request is merged',async()=>{box.pull=()=>({...snapshot(),pull:{...pull().pull,state:'MERGED',mergeable:'UNKNOWN',checks:[],mergedAt:Date.now()-720000}});await panel.getByRole('button',{name:'Refresh session workspace'}).click()}],
   ['the pull request is read from an older machine',async()=>{box.pull=()=>({...snapshot(),pull:{...pull().pull,detailKnown:undefined,reviews:undefined,threads:undefined,lastPushAt:undefined}});await panel.getByRole('button',{name:'Refresh session workspace'}).click()}],
   ['there is no pull request',async()=>{box.reason='no_pr';await panel.getByRole('button',{name:'Refresh session workspace'}).click()}],
  ]
  for(const [name,change] of rounds){
   await change();await settle()
   await expect(panel.getByRole('button',{name:'Refresh session workspace'})).toBeEnabled()
   await settle()
   expect(await pinned(),`${label}: blocks moved for ${name}`).toEqual(baseline)
  }
  const fits=await panel.evaluate(el=>[...el.querySelectorAll('.prompt, .tracker, .pull, .where, .session-summary, .live')].every(n=>n.scrollWidth<=n.clientWidth+1))
  expect(fits,`${label}: a slot overflows sideways`).toBe(true)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`${label}: the page scrolls sideways`).toBe(true)
  if(phone){
   expect(await page.evaluate(()=>matchMedia('(pointer: coarse)').matches),'the phone context has a coarse pointer').toBe(true)
   const targets=await panel.locator('.prompt button, .prompt a, .tracker button, .tracker a, .pull button, .pull a, .pull summary, .where button, .where a, .notice-action, .session-summary button').all()
   expect(targets.length,'there are touch targets to measure').toBeGreaterThan(0)
   for(const target of targets){const b=await target.boundingBox();if(b)expect(b.height,`${label}: a touch target under 44px`).toBeGreaterThanOrEqual(43)}
  }
  expect(errors).toEqual([]);await context.close()
 }
 // No text in the panel, the Overview's cards or the prompt dialog is under 14px: quiet labels are quiet in colour and weight, never in size.
 // Every visible text node is measured by its parent's computed size, in each state that brings its own words.
 {
  const small=root=>root.evaluate(el=>{
   const out=new Set(),walk=document.createTreeWalker(el,NodeFilter.SHOW_TEXT)
   for(let n=walk.nextNode();n;n=walk.nextNode()){
    const p=n.parentElement
    if(!n.textContent.trim()||!p||p.closest('.xterm')||!p.checkVisibility({visibilityProperty:true}))continue
    const size=parseFloat(getComputedStyle(p).fontSize)
    if(size<14)out.add(`${p.tagName.toLowerCase()}${p.className&&typeof p.className==='string'?'.'+p.className.trim().split(/\s+/).join('.'):''} ${size}px "${n.textContent.trim().slice(0,32)}"`)
   }
   return [...out]
  })
  const detail={detailKnown:true,reviews:{approved:1,changesRequested:1,commented:0,requested:0},threads:{open:3,resolved:2},lastPushAt:Date.now()-4*60000,base:'main'}
  for(const width of [1600,375]){
   const {page,context,panel,errors,box}=await open(width,'dark',true,null,{pull:detail})
   const label=`${width}px${width<800?' touch':''}`
   await expect(panel.getByRole('group',{name:'Pull request'})).toContainText('#42')
   await expect(panel.getByRole('group',{name:'You asked'})).toContainText('Rebuild the session overview')
   if(width<800)expect(await page.evaluate(()=>matchMedia('(pointer: coarse)').matches),'the phone context has a coarse pointer').toBe(true)
   expect(await small(panel),`${label}: text under 14px in the Overview`).toEqual([])
   // The fold's own words are measured too, so it is opened first; it stays open for every state below.
   await panel.getByRole('group',{name:'Pull request'}).locator('summary').click()
   await expect(panel.getByRole('group',{name:'Pull request'}).getByText('pushed 4m ago')).toBeVisible()
   expect(await small(panel),`${label}: text under 14px in the open Details fold`).toEqual([])
   // The Last known marker on the card and the Next line, then the machine back.
   let before=box.lists
   box.offline=true;await page.evaluate(()=>window.dispatchEvent(new Event('online')))
   await expect.poll(()=>box.lists).toBeGreaterThan(before)
   await expect(panel.getByRole('group',{name:'Pull request'}).locator('.ident')).toContainText('Last known')
   await expect(panel.getByRole('group',{name:'Progress'}).locator('.next')).toContainText('Last known')
   expect(await small(panel),`${label}: text under 14px with the Last known marker`).toEqual([])
   before=box.lists
   box.offline=false;await page.evaluate(()=>window.dispatchEvent(new Event('online')))
   await expect.poll(()=>box.lists).toBeGreaterThan(before)
   await expect(panel.locator('.connection')).not.toHaveText('Machine offline')
   // The merged row.
   box.pull=()=>({...snapshot(),pull:{...pull().pull,state:'MERGED',mergeable:'UNKNOWN',checks:[],mergedAt:Date.now()-720000}})
   await panel.getByRole('button',{name:'Refresh session workspace'}).click()
   await expect(panel.getByRole('group',{name:'Progress'}).locator('.merged')).toBeVisible()
   expect(await small(panel),`${label}: text under 14px with the pull request merged`).toEqual([])
   // A question from the agent brings the notice's own words and the Next line's Review button.
   await page.evaluate(e=>window.__insightStream.emit('chat',e),{seq:60,kind:'permission',permissionId:'ask60',title:'Allow the agent to run the build?',toolKind:'execute'})
   await expect(panel.locator('.session-summary .notice-action')).toBeVisible()
   expect(await small(panel),`${label}: text under 14px while the agent asks`).toEqual([])
   // The prompt dialog, with its chips and the whole text.
   await panel.getByRole('button',{name:'Read full prompt'}).click()
   const dialog=page.getByRole('dialog',{name:'You asked'})
   await expect(dialog).toContainText('FINAL-LINE-ONLY-IN-FULL-TEXT')
   expect(await small(dialog),`${label}: text under 14px in the prompt dialog`).toEqual([])
   await page.keyboard.press('Escape')
   expect(errors).toEqual([]);await context.close()
  }
  // A terminal session's fix button copies, and says so beside it.
  {
   const {page,context,panel}=await open(1600,'dark',true,null,{pull:{mergeable:'CONFLICTING',review:'REVIEW_REQUIRED',checks:[ok('Build')],base:'main'},session:{structured:false}})
   await context.grantPermissions(['clipboard-read','clipboard-write'])
   await page.waitForTimeout(2000)
   const tracker=panel.getByRole('group',{name:'Progress'})
   await tracker.getByRole('button',{name:/conflicts/}).click()
   await expect(tracker.getByRole('status')).toHaveText('Copied')
   expect(await small(panel),'text under 14px beside the fix button').toEqual([])
   await context.close()
  }
 }
 // The chosen view is sticky: picking GitHub keeps it when another session opens, and after a reload.
 {
  const {page,context,panel,errors}=await open(1600,'dark',true,null,{second:true})
  await panel.getByRole('tab',{name:/^GitHub/}).click()
  await expect(panel.getByRole('tab',{name:/^GitHub/})).toHaveAttribute('aria-selected','true')
  await page.getByRole('button',{name:/Second session/}).first().click()
  await expect(page.locator('.session-screen .title')).toHaveText('Second session')
  await expect(panel.getByRole('tab',{name:/^GitHub/})).toHaveAttribute('aria-selected','true')
  await page.reload()
  await page.locator('.session-screen header.head').waitFor({state:'visible'})
  const reloaded=page.getByRole('complementary',{name:'Session context'})
  await expect(reloaded.getByRole('tab',{name:/^GitHub/})).toHaveAttribute('aria-selected','true')
  expect(errors).toEqual([]);await context.close()
 }
 console.log('Session workspace: tabs, status line, changes, previews, folders, CI, stale data, sidebar signals, context panel resize/collapse and phone/tablet focus passed.')
}finally{await browser.close()}
