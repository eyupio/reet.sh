import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// A terminal agent waiting on a dialog: its question and options on the session screen (under the header, above the
// terminal) and inside the session's item in Home's Waiting for you, answered from reet. Four dialogs, six results.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-terminal-question'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now()
const option=(index,label,selected)=>({index,label,...(selected?{selected:true}:{})})
const LONG_OPTION=`Yes, and apply the same answer to every later command in this whole workspace without asking again, including ones that write outside the project folder ${'and-keep-going-'.repeat(8)}end`
const fixtures={
 claude:{tool:'claude',title:'Build the site',question:{fingerprint:'fp-claude',question:'Bash command\nnpm run build -- --out "<img src=x onerror=alert(1)>"\nBuild the project\nDo you want to proceed?',options:[option(1,'Yes',true),option(2,"Yes, and don't ask again for npm run commands in /work/site"),option(3,'No, and tell Claude what to do differently')]}},
 plan:{tool:'claude',title:'Plan the export',question:{fingerprint:'fp-plan',truncated:true,question:'Here is Claude’s plan:\nAdd a CSV export button to the billing page\nWrite the handler and its tests\nWould you like to proceed?',options:[option(1,'Yes, auto-accept edits',true),option(2,'Yes, manually approve edits'),option(3,'No, keep planning')]}},
 codex:{tool:'codex',title:'Tidy the repo',question:{fingerprint:'fp-codex',question:'Would you like to run the following command?\ngit status --short\nWould you like to run the following command?',options:[option(1,'Yes, proceed',true),option(2,"Yes, and don't ask again for commands that start with `git status`"),option(3,'No, and tell Codex what to do differently')]}},
 long:{tool:'claude',title:'Everything at once',question:{fingerprint:'fp-long',truncated:true,question:Array.from({length:7},(_,i)=>`Subject line ${i+1} ${'x'.repeat(30)}`).join('\n').padEnd(470,'y')+'\nProceed with all of it?',options:Array.from({length:9},(_,i)=>option(i+1,i===1?LONG_OPTION:i===4?LONG_OPTION.slice(0,200):`Choice number ${i+1}`,i===0))}},
}
const sessionOf=(key,extra={})=>({id:`q-${key}-session0001`,agentId:'a1',title:fixtures[key].title,state:'running',createdAt:now-600000,updatedAt:now,exitCode:null,tool:fixtures[key].tool,dir:'/work/site',attention:'waiting',attentionSource:'terminal',changedSinceSeen:true,bellAt:now-60000,terminalQuestion:fixtures[key].question,...extra})
const machine={agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['claude','codex']}}

// `box.answer` is the server's reply to the next answer; `box.sessions` is what the list says now.
async function open(key,{width=1280,theme='light',where='screen',height,answer,hold}={}){
 const box={sessions:[sessionOf(key)],posts:[],reads:0,answer:answer||((r,b)=>{b.sessions=[{...b.sessions[0],terminalQuestion:undefined,attention:'working',changedSinceSeen:false}];return{json:{answered:true}}})}
 const id=box.sessions[0].id
 const w=await openWorkspace(browser,{width,height:height||(where==='home'?1100:900),touch:width<700,theme,scale:2,sessionFocus:false,routes:{
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/machines':{machines:[machine]},
  '/api/v2/machines/health':()=>({serverNow:Date.now(),machines:[{agentId:'a1',host:'studio',state:'connected',connection:{connected:true,reportFresh:true,reportAt:Date.now(),contactAt:Date.now(),missedProbes:0,reconnects:0,path:'direct'}}]}),
  '/api/v2/terminal/sessions':()=>{box.reads++;return{sessions:box.sessions}},
  [`/api/v2/terminal/sessions/${id}/seen`]:{},
  [`/api/v2/terminal/sessions/${id}/skills`]:{skills:[],host:'studio',machineSeen:now},
  [`/api/v2/terminal/sessions/${id}/attachments`]:{ticket:'fixture-ticket00000000000'},
  [`/api/v2/terminal/sessions/${id}/answer`]:async request=>{box.posts.push(request.postDataJSON());if(hold)await hold;return box.answer(request,box)},
 }})
 await w.page.routeWebSocket('**/api/v2/terminal/socket',socket=>socket.onMessage(m=>{if(typeof m==='string'&&JSON.parse(m).ticket){socket.send('{"type":"ready"}');socket.send(Buffer.from('\x1b[2J\x1b[Hready> '))}}))
 await w.page.goto(where==='home'?`${base}/`:`${base}/sessions?agent=a1&session=${id}`)
 const card=w.page.locator('[data-terminal-question]')
 await expect(card).toHaveCount(1)
 return{...w,box,id,card,where}
}
const noSideways=page=>expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1&&[...document.querySelectorAll('[data-terminal-question]')].every(c=>c.scrollWidth<=c.clientWidth+1))).toBe(true)
const text=o=>`${o.index}. ${o.label}`
async function looks(w,key,tag){
 const q=fixtures[key].question,name=fixtures[key].tool==='codex'?'Codex':'Claude Code'
 const group=w.page.getByRole('group',{name:`${name} is asking in ${fixtures[key].title} on studio`})
 await expect(group).toBeVisible()
 // Every line of the question, as text and in order; markup in it is shown, never run.
 const lines=q.question.split('\n')
 await expect(group.locator('.line')).toHaveCount(lines.length)
 for(const [i,line] of lines.entries())await expect(group.locator('.line').nth(i)).toHaveText(line)
 await expect(group.locator('img')).toHaveCount(0)
 expect(await group.locator('.line').first().evaluate(e=>getComputedStyle(e).unicodeBidi)).toBe('isolate')
 if(lines.length>1)expect(await group.locator('.line').first().evaluate(e=>getComputedStyle(e).fontFamily)).toContain('Plex Mono')
 // The options, in the dialog's order, equal in weight; the terminal's default marked.
 const buttons=group.locator('.options button')
 await expect(buttons).toHaveCount(q.options.length)
 for(const [i,o] of q.options.entries()){
  await expect(buttons.nth(i)).toContainText(text(o))
  await expect(buttons.nth(i)).not.toHaveClass(/primary/)
  if(o.selected)await expect(buttons.nth(i)).toContainText('· terminal’s default')
  expect(await buttons.nth(i).locator('.default').count()).toBe(o.selected?1:0)
 }
 // Only a terminal default is marked, in plain muted text: no border, no pill.
 for(const d of await group.locator('.default').all())expect(await d.evaluate(e=>getComputedStyle(e).borderTopWidth)).toBe('0px')
 // An option that keeps applying says so under its label.
 const standing=q.options.filter(o=>/don['’]t ask again|always|for this session|all edits|auto[- ]accept/i.test(o.label)).length
 await expect(group.getByText('Keeps applying after this answer.')).toHaveCount(standing)
 // A cut question leads its note with the "!" mark, in ink, and says to read the terminal first.
 const note=group.locator('.more')
 await expect(note).toHaveCount(q.truncated?1:0)
 if(q.truncated){
  await expect(note).toContainText('Part of this is only in the terminal. Read it there before answering.')
  await expect(note.locator('.mark')).toHaveText('!')
  expect(await note.evaluate(e=>getComputedStyle(e).color)).toBe(await group.locator('.line').last().evaluate(e=>getComputedStyle(e).color))
  await expect(w.page.getByRole(w.where==='home'?'link':'button',{name:w.where==='home'?'Open the terminal':'Go to the terminal'})).toBeVisible()
 }
 await noSideways(w.page)
 if(w.page.viewportSize().width<700)for(const b of await buttons.all())expect((await b.boundingBox()).height).toBeGreaterThanOrEqual(44)
 await group.screenshot({path:`${shots}/${tag}.png`})
}
const errorsOf=w=>expect(w.errors).toEqual([])

try{
 // Each dialog, on the session screen and on Home, wide and light, then a phone and dark.
 for(const key of Object.keys(fixtures))for(const [width,theme] of [[1280,'light'],[390,'dark']]){
  const w=await open(key,{width,theme})
  try{
   await expect(w.page.locator('.session-screen .title')).toHaveText(fixtures[key].title)
   // The card starts under the header and its holder ends above the terminal (a tall card scrolls inside it on a phone).
   const [c,h,t,hd]=await Promise.all([w.card.boundingBox(),w.page.locator('.below-head').boundingBox(),w.page.getByRole('region',{name:'Remote terminal screen'}).boundingBox(),w.page.locator('.session-screen .head').boundingBox()])
   expect(c.y).toBeGreaterThanOrEqual(hd.y+hd.height-1)
   expect(h.y+h.height).toBeLessThanOrEqual(t.y+1)
   await looks(w,key,`${key}-screen-${width}-${theme}`)
   errorsOf(w)
  }finally{await w.context.close()}
  const h=await open(key,{width,theme,where:'home'})
  try{
   const item=h.page.locator('article').filter({has:h.page.locator('[data-terminal-question]')})
   await expect(item).toContainText(fixtures[key].title)
   await expect(item).toContainText('Question')
   if(fixtures[key].question.truncated)await expect(item.getByRole('link',{name:'Open the terminal'})).toHaveAttribute('href',`/sessions?agent=a1&session=${h.id}`)
   await looks(h,key,`${key}-home-${width}-${theme}`)
   // The whole item: which session and which machine, before a tap.
   await expect(item).toContainText('studio')
   await item.screenshot({path:`${shots}/${key}-home-item-${width}-${theme}.png`})
   errorsOf(h)
  }finally{await h.context.close()}
 }

 // Answering: every option is disabled while it goes, the pressed one shows progress, the POST names the fingerprint and the option.
 for(const where of ['screen','home']){
  let release;const gate=new Promise(r=>{release=r})
  const w=await open('claude',{where,hold:gate,answer:()=>({json:{answered:true}})})
  try{
   const second=w.card.getByRole('button',{name:/^2\. Yes, and don't ask again/})
   // A standing-permission option is one of equals, and a key does nothing.
   await expect(second).not.toHaveClass(/primary/)
   await w.page.keyboard.press('1');await w.page.keyboard.press('Enter')
   expect(w.box.posts).toEqual([])
   // A second tap lands on a disabled button and sends nothing; on Home that is the check, on the screen the focus is.
   if(where==='home')await second.dblclick();else await second.click()
   await expect(w.card.getByRole('button')).toHaveCount(3)
   for(const b of await w.card.getByRole('button').all())await expect(b).toBeDisabled()
   await expect(second).toHaveAttribute('aria-busy','true')
   // Focus waits on the result line while it goes, not on the page.
   if(where==='screen')await expect(w.card.locator('.result')).toBeFocused()
   await expect(w.card.locator('.result')).toHaveText('Sending your answer…')
   await expect.poll(()=>w.box.posts.length).toBe(1)
   release()
   await expect(w.card.getByRole('status').filter({hasText:'Answered:'})).toHaveText(/Answered: Yes, and don't ask again for npm run commands/)
   await expect(w.card.getByRole('button')).toHaveCount(0)
   if(where==='screen')await expect(w.card.locator('.result')).toBeFocused()
   await expect(w.card.getByRole('heading',{name:'Claude Code was asked'})).toBeVisible()
   await expect(w.card.locator('.result .mark')).toHaveCount(0)
   await w.card.screenshot({path:`${shots}/result-answered-${where}.png`})
   expect(w.box.posts).toEqual([{fingerprint:'fp-claude',option:2}])
   // Until a read says otherwise the card keeps saying so; the next read removes the question, and the card with it.
   w.box.sessions=[{...w.box.sessions[0],terminalQuestion:undefined,attention:'working',changedSinceSeen:false}]
   await w.page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')))
   await expect(w.card).toHaveCount(0)
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // Already answered reads the same.
 {
  const w=await open('codex',{answer:()=>({json:{answered:true,alreadyAnswered:true}})})
  try{
   await w.card.getByRole('button',{name:/^1\. Yes, proceed/}).click()
   await expect(w.card).toContainText('Answered: Yes, proceed')
   expect(w.box.posts).toEqual([{fingerprint:'fp-codex',option:1}])
  }finally{await w.context.close()}
 }

 // The question changed: the server's sentence, the sessions read again at once, the new question shown and nothing answered for the person.
 {
  const newer={fingerprint:'fp-next',question:'Edit file\nsrc/app.ts\nDo you want to make this edit?',options:[option(1,'Yes',true),option(2,'No')]}
  const w=await open('claude',{answer:(r,box)=>{box.sessions=[{...box.sessions[0],terminalQuestion:newer}];return{status:409,json:{error:'The question on the terminal changed. Look at it again before answering.',code:'question_changed'}}}})
  try{
   const before=w.box.reads
   await w.card.getByRole('button',{name:/^1\. Yes/}).click()
   await expect(w.card).toContainText('Do you want to make this edit?')
   await expect(w.card).toContainText('The question on the terminal changed. Look at it again before answering.')
   expect(w.box.reads).toBeGreaterThan(before)
   await expect(w.card.getByRole('button')).toHaveCount(2)
   for(const b of await w.card.getByRole('button').all())await expect(b).toBeEnabled()
   expect(w.box.posts).toHaveLength(1)
   // A change is a plain fact, not an error; focus is on that sentence, and Enter does not answer the new question.
   await expect(w.card.locator('.result .mark')).toHaveCount(0)
   await expect(w.card.locator('.result')).toBeFocused()
   await w.page.keyboard.press('Enter');await w.page.keyboard.press('Space')
   await w.page.waitForTimeout(300)
   expect(w.box.posts).toHaveLength(1)
   await w.card.screenshot({path:`${shots}/result-changed.png`})
  }finally{await w.context.close()}
 }

 // Refusals: the card's own sentence by the buttons. Update required and read-only keep them disabled; busy, offline and others re-enable them. Only real errors lead with "!".
 for(const [name,status,json,locked,say,error] of [
  ['update-required',409,{error:'Update reet on this machine to answer from here.',code:'update_required'},true,'Update reet on this machine to answer from here.',false],
  ['read-only',403,{error:'terminal access denied'},true,'Your account can look but not answer.',false],
  ['offline',503,{error:'terminal machine is reconnecting; retry after it reconnects'},false,'The machine is not connected. Try again when it is back.',true],
  ['busy',409,{error:'An answer is already on its way.',code:'answer_busy'},false,'Another answer is on its way. Wait a moment.',false],
  ['failed',404,{error:'that reet session is not available'},false,'The answer could not be sent.',true],
 ]){
  const w=await open('plan',{width:name==='offline'?390:1280,theme:name==='offline'?'dark':'light',answer:()=>({status,json})})
  try{
   await w.card.getByRole('button',{name:/^3\. No, keep planning/}).click()
   await expect(w.card.locator('.result')).toHaveText(error?`!${say}`:say)
   await expect(w.card.locator('.result')).toBeFocused()
   await expect(w.card).not.toContainText('wire')
   await expect(w.card.locator('.result .mark')).toHaveCount(error?1:0)
   for(const b of await w.card.locator('.options button').all())locked?await expect(b).toBeDisabled():await expect(b).toBeEnabled()
   await expect(w.card).not.toContainText('Answered')
   await expect(w.card).toHaveCount(1)
   await w.card.screenshot({path:`${shots}/result-${name}.png`})
   await noSideways(w.page)
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // A long question on a short phone: the text scrolls inside its own 30% and the last option can still be reached.
 {
  const w=await open('long',{width:390,height:560,theme:'light'})
  try{
   const said=w.card.locator('.subjects'),last=w.card.locator('.options button').last()
   expect((await said.boundingBox()).height).toBeLessThanOrEqual(560*0.3+1)
   // The question sentence is outside that scroller, so it is always in view with it.
   await expect(w.card.locator('.said > .line')).toHaveText('Proceed with all of it?')
   await w.page.screenshot({path:`${shots}/long-screen-390x560-top-light.png`})
   await last.scrollIntoViewIfNeeded()
   const box=await last.boundingBox()
   expect(box.y).toBeGreaterThanOrEqual(0);expect(box.y+box.height).toBeLessThanOrEqual(561)
   await noSideways(w.page)
   await w.page.screenshot({path:`${shots}/long-screen-390x560-light.png`})
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // The same case at the narrowest width the app supports (320px, as 200% zoom on a phone leaves it): the card fits, nothing runs off sideways.
 {
  const w=await open('long',{width:320,height:560,theme:'dark'})
  try{
   const b=await w.card.boundingBox()
   expect(b.x+b.width).toBeLessThanOrEqual(321)
   expect(await w.card.evaluate(e=>e.scrollWidth<=e.clientWidth+1&&[...e.querySelectorAll('.option,.line,.more')].every(c=>c.getBoundingClientRect().right<=e.getBoundingClientRect().right+1))).toBe(true)
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // Absent question: nothing is drawn (an older machine, or a session that asks nothing).
 {
  const w=await openWorkspace(browser,{width:1280,routes:{'/api/v2/terminal/status':{configured:true,available:true,reason:''},'/api/v2/terminal/machines':{machines:[machine]},'/api/v2/terminal/sessions':{sessions:[sessionOf('claude',{terminalQuestion:undefined})]},'/api/v2/terminal/sessions/q-claude-session0001/seen':{}}})
  try{await w.page.goto(`${base}/`);await expect(w.page.getByRole('region',{name:'Waiting for you'}).getByRole('heading',{name:'Build the site'})).toBeVisible();await expect(w.page.locator('[data-terminal-question]')).toHaveCount(0)}finally{await w.context.close()}
 }
 console.log('terminal-question: ok')
}finally{await browser.close()}
