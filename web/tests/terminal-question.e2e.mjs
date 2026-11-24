import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

// A terminal agent waiting on a dialog: its question and options on the session screen (under the header, above the
// terminal) and inside the session's item in Home's Waiting for you (the whole card only when it is small), answered
// from reet. The dialogs are the shapes the machine sends; a standing-permission or cut option needs a second tap.
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-terminal-question'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now()
const option=(index,label,extra={})=>({index,label,...extra})
const LONG_OPTION=`Yes, and apply the same answer to every later command in this whole workspace without asking again, including ones that write outside the project folder ${'and-keep-going-'.repeat(8)}end`
const STANDING=/ask again|asking again|without asking|every time|every later|from now on|remember|trust|auto-accept|auto accept|always|for this session|all edits|don['’]t ask|do not ask/i
const fits=q=>q.options.length<=3&&!q.truncated&&!q.options.some(o=>o.truncated)
const fixtures={
 claude:{tool:'claude',title:'Run the test suite',question:{fingerprint:'fp-claude',question:'Bash command\ngo test ./...\nRun the test suite\nDo you want to proceed?',options:[option(1,'Yes',{selected:true}),option(2,"Yes, and don't ask again for go test commands"),option(3,'No, and tell Claude what to do differently (esc)')]}},
 plan:{tool:'claude',title:'Plan the export',question:{fingerprint:'fp-plan',truncated:true,question:'Ready to code?\nHere is Claude’s plan:\n1. Add a CSV export button to the billing page\n2. Write the handler and its tests\nWould you like to proceed?',options:[option(1,'Yes, auto-accept edits',{selected:true}),option(2,'Yes, manually approve edits'),option(3,'No, keep planning')]}},
 codex:{tool:'codex',title:'Migrate the database',question:{fingerprint:'fp-codex',question:'$ make migrate\nWould you like to run the following command?',options:[option(1,'Yes, proceed (y)',{selected:true}),option(2,"Yes, and don't ask again for commands that start with `make migrate` (p)"),option(3,'No, and tell Codex what to do differently (esc)')]}},
 long:{tool:'claude',title:'Everything at once',question:{fingerprint:'fp-long',truncated:true,question:Array.from({length:7},(_,i)=>`Subject line ${i+1} ${'x'.repeat(30)}`).join('\n').padEnd(470,'y')+'\nProceed with all of it?',options:Array.from({length:9},(_,i)=>option(i+1,i===1?LONG_OPTION:i===4?LONG_OPTION.slice(0,200):`Choice number ${i+1}`,i===0?{selected:true}:{}))}},
 cut:{tool:'claude',title:'Edit the app',question:{fingerprint:'fp-cut',question:'Edit file\nsrc/app.ts\nDo you want to make this edit?',options:[option(1,'Yes',{selected:true}),option(2,'Yes, allow all edits during this session'),option(3,LONG_OPTION.slice(0,200),{truncated:true}),option(4,'No')]}},
 markup:{tool:'claude',title:'Print a tag',question:{fingerprint:'fp-markup',question:'Bash command\necho "<img src=x onerror=alert(1)>"\nPrint it\nDo you want to proceed?',options:[option(1,'Yes',{selected:true}),option(2,'No')]}},
}
const idOf=key=>`q-${key}-session0001`
const sessionOf=(key,extra={})=>({id:idOf(key),agentId:'a1',title:fixtures[key].title,state:'running',createdAt:now-600000,updatedAt:now-60000*(1+Object.keys(fixtures).indexOf(key)),exitCode:null,tool:fixtures[key].tool,dir:'/work/site',attention:'waiting',attentionSource:'terminal',changedSinceSeen:true,bellAt:now-60000,terminalQuestion:fixtures[key].question,...extra})
const machine={agentId:'a1',host:'studio',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'dev',terminalEnabled:true,terminalControl:true,healthEnabled:true,readiness:'ready',launch:true,tools:['claude','codex']}}
const answered=(r,b,key)=>{b.sessions=b.sessions.map(s=>s.id===idOf(key)?{...s,terminalQuestion:undefined,attention:'working',changedSinceSeen:false}:s);return{json:{answered:true}}}

// `box.answer` is the server's reply to the next answer; `box.sessions` is what the list says now; `box.offline` makes the machine stop reporting.
async function open(keys,{width=1280,theme='light',where='screen',height,answer,hold,offline=false,locked=false}={}){
 keys=[].concat(keys);const key=keys[0]
 const box={sessions:keys.map(k=>sessionOf(k)),posts:[],reads:0,offline,locked}
 box.answer=answer||((r,b)=>answered(r,b,key))
 const routes={
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  // `box.locked`: the machine's owner locked it, so it is not ready and says so.
  '/api/v2/terminal/machines':()=>({machines:[box.locked?{...machine,ready:false,locked:true,capabilities:{...machine.capabilities,locked:true}}:machine]}),
  '/api/v2/machines/health':()=>({serverNow:Date.now(),machines:[{agentId:'a1',host:'studio',state:box.offline?'disconnected':'connected',connection:{connected:!box.offline,reportFresh:!box.offline,reportAt:Date.now(),contactAt:Date.now(),missedProbes:0,reconnects:0,path:'direct'}}]}),
  '/api/v2/terminal/sessions':()=>{box.reads++;return{sessions:box.sessions}},
 }
 for(const k of keys){
  routes[`/api/v2/terminal/sessions/${idOf(k)}/seen`]={}
  routes[`/api/v2/terminal/sessions/${idOf(k)}/skills`]={skills:[],host:'studio',machineSeen:now}
  routes[`/api/v2/terminal/sessions/${idOf(k)}/attachments`]={ticket:'fixture-ticket00000000000'}
  routes[`/api/v2/terminal/sessions/${idOf(k)}/answer`]=async request=>{box.posts.push(request.postDataJSON());if(hold)await hold;return box.answer(request,box)}
 }
 const w=await openWorkspace(browser,{width,height:height||(where==='home'?1100:900),touch:width<700,theme,scale:2,sessionFocus:false,routes})
 await w.page.routeWebSocket('**/api/v2/terminal/socket',socket=>socket.onMessage(m=>{if(typeof m==='string'&&JSON.parse(m).ticket){socket.send('{"type":"ready"}');socket.send(Buffer.from('\x1b[2J\x1b[Hready> '))}}))
 await w.page.goto(where==='home'?`${base}/`:`${base}/sessions?agent=a1&session=${idOf(key)}`)
 const card=w.page.locator('[data-terminal-question]')
 // On Home a big question shows its sentence and Open instead of the card.
 await expect(card).toHaveCount(where==='home'?keys.filter(k=>fits(fixtures[k].question)).length:1)
 const item=w.page.getByRole('region',{name:'Waiting for you'}).locator('article').filter({hasText:fixtures[key].title})
 return{...w,box,id:idOf(key),card,where,item}
}
const noSideways=page=>expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1&&[...document.querySelectorAll('[data-terminal-question]')].every(c=>c.scrollWidth<=c.clientWidth+1))).toBe(true)
const text=o=>`${o.index}. ${o.label}`
const opt=(w,n)=>w.card.locator('.options button').nth(n-1)
const errorsOf=w=>expect(w.errors).toEqual([])
const name=key=>fixtures[key].tool==='codex'?'Codex':'Claude Code'

async function looks(w,key,tag){
 const q=fixtures[key].question
 const group=w.page.getByRole('group',{name:`${name(key)} is asking in ${fixtures[key].title} on studio`})
 await expect(group).toBeVisible()
 // Every line of the question, as text and in order; markup in it is shown, never run.
 const lines=q.question.split('\n')
 await expect(group.locator('.line')).toHaveCount(lines.length)
 for(const [i,line] of lines.entries())await expect(group.locator('.line').nth(i)).toHaveText(line)
 await expect(group.locator('img')).toHaveCount(0)
 expect(await group.locator('.line').first().evaluate(e=>getComputedStyle(e).unicodeBidi)).toBe('isolate')
 if(lines.length>1)expect(await group.locator('.line').first().evaluate(e=>getComputedStyle(e).fontFamily)).toContain('Plex Mono')
 // The options, in the dialog's order, equal in weight; the terminal's default marked in plain muted text.
 const buttons=group.locator('.options button')
 await expect(buttons).toHaveCount(q.options.length)
 for(const [i,o] of q.options.entries()){
  await expect(buttons.nth(i)).toContainText(text(o))
  await expect(buttons.nth(i)).not.toHaveClass(/primary/)
  expect(await buttons.nth(i).locator('.default').count()).toBe(o.selected?1:0)
  if(o.selected)await expect(buttons.nth(i)).toContainText('· terminal’s default')
 }
 for(const d of await group.locator('.default').all())expect(await d.evaluate(e=>getComputedStyle(e).borderTopWidth)).toBe('0px')
 // An option that keeps applying says so under its label; a cut one says it is cut.
 await expect(group.getByText('Keeps applying after this answer.')).toHaveCount(q.options.filter(o=>STANDING.test(o.label)).length)
 await expect(group.locator('.cut')).toHaveCount(q.options.filter(o=>o.truncated).length)
 for(const c of await group.locator('.cut').all())await expect(c).toContainText('This option is cut short. Read it in the terminal before choosing.')
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
 if(w.page.viewportSize().width<700){
  for(const b of await buttons.all())expect((await b.boundingBox()).height).toBeGreaterThanOrEqual(44)
  for(const l of await group.locator('.way').all())expect((await l.boundingBox()).height).toBeGreaterThanOrEqual(44)
 }
 await group.screenshot({path:`${shots}/${tag}.png`})
}
// Home's compact line for a big question: the sentence, text only, and Open to the session screen.
async function compact(w,key,tag){
 const q=fixtures[key].question,sentence=q.question.split('\n').at(-1)
 await expect(w.item).toContainText(sentence)
 await expect(w.item.locator('[data-terminal-question]')).toHaveCount(0)
 await expect(w.item.locator('.options')).toHaveCount(0)
 await expect(w.item.getByRole('link',{name:`Open ${fixtures[key].title}`})).toHaveAttribute('href',`/sessions?agent=a1&session=${idOf(key)}`)
 expect((await w.item.locator('.said').boundingBox()).height).toBeLessThan(60)
 await noSideways(w.page)
 await w.item.screenshot({path:`${shots}/${tag}.png`})
}
const chipOf=key=>{const o=fixtures[key].question.options;return o.some(x=>/^(yes|allow|approve)\b/i.test(x.label))&&o.some(x=>/^(no|deny|reject)\b/i.test(x.label))?'Approval':'Question'}

try{
 // Each dialog, on the session screen and as a whole Home item, wide and light, then a phone and dark.
 for(const key of ['claude','plan','codex','long','cut'])for(const [width,theme] of [[1280,'light'],[390,'dark']]){
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
   await expect(h.item).toContainText('studio')
   // Chosen as the other approvals are: an approval-looking dialog is chipped Approval, any other Question.
   await expect(h.item.locator('.chip')).toHaveText(chipOf(key))
   if(fits(fixtures[key].question)){
    // In the row, not a box inside a box.
    expect(await h.card.evaluate(e=>getComputedStyle(e).borderTopWidth)).toBe('0px')
    await looks(h,key,`${key}-home-${width}-${theme}`)
    await h.item.screenshot({path:`${shots}/${key}-home-item-${width}-${theme}.png`})
   }else await compact(h,key,`${key}-home-item-${width}-${theme}`)
   errorsOf(h)
  }finally{await h.context.close()}
 }

 // Markup in a dialog is shown as text, never run.
 {
  const w=await open('markup')
  try{await expect(w.card.locator('.line').nth(1)).toHaveText('echo "<img src=x onerror=alert(1)>"');await expect(w.card.locator('img')).toHaveCount(0);errorsOf(w)}finally{await w.context.close()}
 }

 // One tap sends a one-off option once, even tapped twice; no key answers.
 for(const where of ['screen','home']){
  let release;const gate=new Promise(r=>{release=r})
  const w=await open('claude',{where,hold:gate,answer:()=>({json:{answered:true}})})
  try{
   const first=w.card.getByRole('button',{name:/^1\. Yes/})
   await w.page.keyboard.press('1');await w.page.keyboard.press('Enter')
   expect(w.box.posts).toEqual([])
   await first.dblclick()
   await expect(w.card.locator('.options button')).toHaveCount(3)
   for(const b of await w.card.locator('.options button').all())await expect(b).toBeDisabled()
   await expect(first).toHaveAttribute('aria-busy','true')
   await expect(w.card.locator('.result')).toHaveText('Sending your answer…')
   await expect.poll(()=>w.box.posts.length).toBe(1)
   release()
   await expect(w.card.locator('.result')).toHaveText(/Answered: Yes$/)
   await expect(w.card.getByRole('heading',{name:'Claude Code was asked'})).toBeVisible()
   await expect(w.card.locator('.options')).toHaveCount(0)
   if(where==='screen')await expect(w.card.locator('.result')).toBeFocused()
   await w.card.screenshot({path:`${shots}/result-answered-${where}.png`})
   expect(w.box.posts).toEqual([{fingerprint:'fp-claude',option:1}])
   // Until a read says otherwise the card keeps saying so; the next read removes the question, and the card with it.
   w.box.sessions=[{...w.box.sessions[0],terminalQuestion:undefined,attention:'working',changedSinceSeen:false}]
   await w.page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')))
   await expect(w.card).toHaveCount(0)
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // A standing-permission option takes a second tap: the first sends nothing and asks, the second sends once.
 for(const where of ['screen','home']){
  const w=await open('claude',{where,theme:where==='home'?'dark':'light',answer:()=>({json:{answered:true}})})
  try{
   const standing=opt(w,2)
   await expect(standing).toContainText('Keeps applying after this answer.')
   await standing.click()
   expect(w.box.posts).toEqual([])
   await expect(standing).toContainText('Tap again to confirm')
   await expect(standing).toContainText("2. Yes, and don't ask again for go test commands")
   await expect(w.page.locator('.vh[role=status]').first()).toContainText('Tap again to confirm: 2. Yes')
   await w.card.screenshot({path:`${shots}/confirm-step-${where}.png`})
   await standing.click()
   await expect.poll(()=>w.box.posts.length).toBe(1)
   expect(w.box.posts).toEqual([{fingerprint:'fp-claude',option:2}])
   errorsOf(w)
  }finally{await w.context.close()}
 }
 // Escape, another option, the keyboard, and a changed question each have their say.
 {
  const w=await open('claude',{answer:()=>({json:{answered:true}})})
  try{
   const standing=opt(w,2)
   await standing.click();await expect(standing).toContainText('Tap again to confirm')
   await w.page.keyboard.press('Escape')
   await expect(standing).not.toContainText('Tap again to confirm')
   expect(w.box.posts).toEqual([])
   // Enter is a tap, Space is a tap: two steps from the keyboard too.
   await standing.focus();await w.page.keyboard.press('Enter')
   await expect(standing).toContainText('Tap again to confirm');expect(w.box.posts).toEqual([])
   await w.page.keyboard.press('Space')
   await expect.poll(()=>w.box.posts.length).toBe(1)
  }finally{await w.context.close()}
  const x=await open('claude',{answer:()=>({json:{answered:true}})})
  try{
   const standing=opt(x,2)
   await standing.click();await expect(standing).toContainText('Tap again to confirm')
   await opt(x,1).click()
   await expect.poll(()=>x.box.posts.length).toBe(1)
   expect(x.box.posts).toEqual([{fingerprint:'fp-claude',option:1}])
  }finally{await x.context.close()}
  const y=await open('claude',{where:'home'})
  try{
   const standing=opt(y,2)
   await standing.click();await expect(standing).toContainText('Tap again to confirm')
   y.box.sessions=[{...y.box.sessions[0],terminalQuestion:{...fixtures.claude.question,fingerprint:'fp-next'}}]
   await y.page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')))
   await expect.poll(()=>y.page.locator('[data-terminal-question]').evaluate(e=>e.textContent.includes('Tap again to confirm'))).toBe(false)
   expect(y.box.posts).toEqual([])
  }finally{await y.context.close()}
 }

 // An answer still on its way when a different question arrives is dropped: the new question keeps its options and is not called answered.
 {
  let release;const gate=new Promise(r=>{release=r})
  const w=await open('claude',{where:'home',hold:gate,answer:()=>({json:{answered:true}})})
  try{
   await opt(w,1).click()
   await expect.poll(()=>w.box.posts.length).toBe(1)
   w.box.sessions=[{...w.box.sessions[0],terminalQuestion:{fingerprint:'fp-next',question:'Edit file\nsrc/app.ts\nDo you want to make this edit?',options:[option(1,'Yes',{selected:true}),option(2,'No')]}}]
   await w.page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')))
   await expect(w.card).toContainText('Do you want to make this edit?')
   release()
   await w.page.waitForTimeout(500)
   await expect(w.card.locator('.options button')).toHaveCount(2)
   for(const b of await w.card.locator('.options button').all())await expect(b).toBeEnabled()
   await expect(w.card).not.toContainText('Answered')
   await expect(w.card.getByRole('heading',{name:'Claude Code is asking'})).toBeVisible()
   expect(w.box.posts).toHaveLength(1)
   // The new question can be answered in its own right.
   await opt(w,2).click()
   await expect.poll(()=>w.box.posts.length).toBe(2)
   expect(w.box.posts[1]).toEqual({fingerprint:'fp-next',option:2})
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // A cut option cannot be read whole, so it takes the second tap as well.
 {
  const w=await open('cut',{answer:()=>({json:{answered:true}})})
  try{
   const cut=opt(w,3)
   await cut.click()
   expect(w.box.posts).toEqual([])
   await expect(cut).toContainText('Tap again to confirm')
   await cut.click()
   await expect.poll(()=>w.box.posts.length).toBe(1)
   expect(w.box.posts).toEqual([{fingerprint:'fp-cut',option:3}])
  }finally{await w.context.close()}
 }

 // An answer that leaves the agent asking for words says so and shows the way to the terminal.
 for(const [where,width,theme] of [['screen',390,'dark'],['home',1280,'light']]){
  const w=await open('claude',{where,width,theme,answer:()=>({json:{answered:true}})})
  try{
   await w.card.getByRole('button',{name:/^3\. No, and tell Claude/}).click()
   const result=w.card.locator('.result')
   await expect(result).toContainText('Answered: No, and tell Claude what to do differently (esc). Claude Code is waiting for your words in the terminal.')
   const way=result.locator('.way')
   await expect(way).toBeVisible()
   if(where==='home')await expect(way).toHaveAttribute('href',`/sessions?agent=a1&session=${w.id}`)
   if(width<700)expect((await way.boundingBox()).height).toBeGreaterThanOrEqual(44)
   await w.card.screenshot({path:`${shots}/result-words-${where}-${width}-${theme}.png`})
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // Locked from the first paint when the page knows: the machine is not connected, so the options start disabled with the reason.
 {
  const w=await open('plan',{offline:true,theme:'dark',width:390})
  try{
   await expect(w.card.locator('.result')).toHaveText('The machine is not connected. Try again when it is back.')
   for(const b of await w.card.locator('.options button').all())await expect(b).toBeDisabled()
   await w.card.screenshot({path:`${shots}/locked-offline.png`})
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // A terminal session open on a machine that becomes locked: the card says what still works instead of offering what would be
 // refused, Terminal here is off with that reason, End session stays, and the lock is announced once through an element that was
 // already there.
 for(const [width,theme] of [[1280,'light'],[390,'dark']]){
  const SENTENCE='Locked on the machine. The agent keeps running; from here you can only stop its current turn or end the session.'
  const w=await open('claude',{width,theme})
  try{
   const live=w.page.locator('[data-lock-status]')
   await expect(live).toHaveText('')
   for(const b of await w.card.locator('.options button').all())await expect(b).toBeEnabled()
   w.box.locked=true
   await w.page.evaluate(()=>window.dispatchEvent(new Event('online')))
   await expect(live).toHaveText(SENTENCE)
   await expect(w.card.locator('.result')).toHaveText(SENTENCE)
   for(const b of await w.card.locator('.options button').all())await expect(b).toBeDisabled()
   await w.page.screenshot({path:`${shots}/terminal-session-locked-${width}-${theme}.png`})
   const screen=w.page.locator('.session-screen')
   if(!await screen.getByRole('button',{name:'End session…',exact:true}).isVisible())await screen.getByRole('button',{name:'Session actions',exact:true}).click()
   await expect(screen.getByRole('button',{name:'End session…',exact:true})).toBeEnabled()
   await expect(screen.getByRole('button',{name:'Terminal here',exact:true})).toBeDisabled()
   await expect(screen.getByRole('button',{name:'Terminal here',exact:true})).toHaveAttribute('title',SENTENCE)
   expect(await w.page.evaluate(()=>document.body.innerText.includes('`')),'a literal backtick').toBe(false)
   expect(w.box.posts).toHaveLength(0)
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // The question changed: the server's sentence, the sessions read again at once, the new question shown and nothing answered for the person.
 {
  const newer={fingerprint:'fp-next',question:'Edit file\nsrc/app.ts\nDo you want to make this edit?',options:[option(1,'Yes',{selected:true}),option(2,'No')]}
  const w=await open('claude',{answer:(r,box)=>{box.sessions=[{...box.sessions[0],terminalQuestion:newer}];return{status:409,json:{error:'The question on the terminal changed. Look at it again before answering.',code:'question_changed'}}}})
  try{
   const before=w.box.reads
   await w.card.getByRole('button',{name:/^1\. Yes/}).click()
   await expect(w.card).toContainText('Do you want to make this edit?')
   await expect(w.card).toContainText('The question on the terminal changed. Look at it again before answering.')
   expect(w.box.reads).toBeGreaterThan(before)
   await expect(w.card.locator('.options button')).toHaveCount(2)
   for(const b of await w.card.locator('.options button').all())await expect(b).toBeEnabled()
   expect(w.box.posts).toHaveLength(1)
   // A change is a plain fact, not an error; focus is on that sentence with no ring after a mouse press, and Enter does not answer the new question.
   await expect(w.card.locator('.result .mark')).toHaveCount(0)
   await expect(w.card.locator('.result')).toBeFocused()
   expect(await w.card.locator('.result').evaluate(e=>getComputedStyle(e).outlineStyle)).toBe('none')
   await w.card.screenshot({path:`${shots}/result-changed.png`})
   await w.page.keyboard.press('Enter');await w.page.keyboard.press('Space')
   await w.page.waitForTimeout(300)
   expect(w.box.posts).toHaveLength(1)
  }finally{await w.context.close()}
 }

 // Refusals: the card's own sentence by the buttons, the machine named where it matters. Update required and read-only keep the options disabled; busy, offline and others re-enable them. Only real errors lead with "!".
 for(const [label,status,json,locked,say,error] of [
  ['update-required',409,{error:'Update reet on this machine to answer from here.',code:'update_required'},true,'Update reet on studio to answer from here.',false],
  ['read-only',403,{error:'terminal access denied'},true,'Your account can look but not answer.',false],
  ['offline',503,{error:'terminal machine is reconnecting; retry after it reconnects'},false,'The machine is not connected. Try again when it is back.',true],
  ['busy',409,{error:'An answer is already on its way.',code:'answer_busy'},false,'Another answer is on its way. Wait a moment.',false],
  ['failed',404,{error:'that reet session is not available'},false,'The answer could not be sent. Try again, or answer in the terminal.',true],
 ]){
  const w=await open('plan',{width:label==='offline'?390:1280,theme:label==='offline'?'dark':'light',answer:()=>({status,json})})
  try{
   await w.card.getByRole('button',{name:/^3\. No, keep planning/}).click()
   await expect(w.card.locator('.result')).toHaveText(error?`!${say}`:say)
   await expect(w.card.locator('.result')).toBeFocused()
   await expect(w.card).not.toContainText('wire')
   await expect(w.card.locator('.result .mark')).toHaveCount(error?1:0)
   for(const b of await w.card.locator('.options button').all())locked?await expect(b).toBeDisabled():await expect(b).toBeEnabled()
   await expect(w.card).not.toContainText('Answered')
   await expect(w.card).toHaveCount(1)
   await w.card.screenshot({path:`${shots}/result-${label}.png`})
   await noSideways(w.page)
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // A long question on a short phone: the subject lines scroll inside their own 30% and the last option can still be reached.
 {
  const w=await open('long',{width:390,height:560,theme:'light'})
  try{
   const said=w.card.locator('.subjects'),last=w.card.locator('.options button').last()
   expect((await said.boundingBox()).height).toBeLessThanOrEqual(560*0.3+1)
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
 // On a desktop window the nine-option dialog caps its holder, so the terminal below keeps real room.
 {
  const w=await open('long',{width:1280,height:900})
  try{
   expect((await w.page.getByRole('region',{name:'Remote terminal screen'}).boundingBox()).height).toBeGreaterThanOrEqual(150)
   await w.page.screenshot({path:`${shots}/long-screen-1280x900-light.png`})
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

 // Three terminal sessions waiting together in Home: each says which session and machine, with its own chip and form.
 for(const [width,theme] of [[1280,'light'],[390,'dark']]){
  const w=await open(['claude','plan','long'],{where:'home',width,theme})
  try{
   const list=w.page.getByRole('region',{name:'Waiting for you'})
   for(const k of ['claude','plan','long']){
    const item=list.locator('article').filter({hasText:fixtures[k].title})
    await expect(item).toContainText('studio')
    await expect(item.locator('.chip')).toHaveText(chipOf(k))
   }
   await expect(w.page.locator('[data-terminal-question]')).toHaveCount(1)
   await noSideways(w.page)
   await list.screenshot({path:`${shots}/home-three-waiting-${width}-${theme}.png`})
   errorsOf(w)
  }finally{await w.context.close()}
 }

 // Absent question: nothing is drawn (an older machine, or a session that asks nothing).
 {
  const w=await openWorkspace(browser,{width:1280,routes:{'/api/v2/terminal/status':{configured:true,available:true,reason:''},'/api/v2/terminal/machines':{machines:[machine]},'/api/v2/terminal/sessions':{sessions:[sessionOf('claude',{terminalQuestion:undefined})]},[`/api/v2/terminal/sessions/${idOf('claude')}/seen`]:{}}})
  try{await w.page.goto(`${base}/`);await expect(w.page.getByRole('region',{name:'Waiting for you'}).getByRole('heading',{name:fixtures.claude.title})).toBeVisible();await expect(w.page.locator('[data-terminal-question]')).toHaveCount(0)}finally{await w.context.close()}
 }
 console.log('terminal-question: ok')
}finally{await browser.close()}
