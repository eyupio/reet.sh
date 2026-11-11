// Explicit stop leaves the view after confirmed closure; general Gaffer can select a worker without granting access.
import {chromium,expect} from '@playwright/test'
import {openWorkspace} from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const pid='a'.repeat(32),cid='b'.repeat(32),now=Date.now()
// Sessions carry attention as the server lists it: a quiet chat the person has seen needs nothing.
const make=(id,title,over={})=>({id,agentId:'a1',title,structured:true,tool:'opencode',model:'worker-model',state:'running',dir:'/work',createdAt:now,updatedAt:now,exitCode:null,attention:'quiet',attentionSource:'chat',changedSinceSeen:false,...over})
// A user-ended plain terminal, as the server reports it: the machine kills the shell and answers ended with no exit code
// (internal/terminalhost/manager.go End and reconcile), and AttentionState gives an ended session no attention word
// (internal/store/store_attention.go), though updated_at moves past the seen time. It must raise no card.
const ended=item=>{item.state='ended';item.exitCode=null;delete item.attention;item.changedSinceSeen=true;item.updatedAt=Date.now()}
async function fixture(width,crossMachine=false,plain=false){
 const box={sessions:[make('regression-session-001','First worker'),make('regression-session-002','Second worker')],writes:[],pending:false,refuse:false,reads:0}
 if(crossMachine){box.sessions[1].agentId='a2';box.sessions.push(make('regression-session-003','Workshop survivor'))}
 if(plain)box.sessions.unshift(make('regression-session-004','Plain shell',{structured:false,attentionSource:'terminal',activityAt:now-60_000}))
 const provider={id:pid,revision:1,name:'Gaffer fixture',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,hasKey:true,createdAt:now,updatedAt:now}
 const routes={
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':()=>{box.reads++;return{sessions:box.sessions}},
  '/api/v2/terminal/machines':{machines:[['a1','Workshop'],['a2','Builder']].map(([agentId,host])=>({agentId,host,ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalControl:true,terminalEnabled:true,readiness:'ready',acp:true,acpHistory:true,acpLiveControls:true}}))},
  '/api/v2/gaffer/providers':{providers:[provider],available:true,canUseLocal:true,presets:[]},
  '/api/v2/gaffer/conversations':request=>({conversation:{id:cid,sessionId:new URL(request.url()).searchParams.get('sessionId')||'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}),
  [`/api/v2/gaffer/conversations/${cid}/goal`]:{goal:null,pins:{providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},allowed:true},
 }
 for(const item of box.sessions){
  const path=`/api/v2/terminal/sessions/${item.id}`
  routes[path+'/end']=request=>{box.writes.push({path:path+'/end',body:request.postDataJSON()});if(box.refuse)return{status:503,json:{error:'Stop could not be confirmed'}};if(box.pending)item.state='end_requested';else ended(item);return{}}
  routes[path+'/chat/controls']={controls:{working:false,waiting:false,model:'worker-model',options:[]}}
  routes[path+'/chat/queue']={queue:[]};routes[path+'/seen']={};routes[path+'/skills']={skills:[]}
  routes[path+'/attachments']={ticket:'fixture-ticket00000000000'}
 }
 const w=await openWorkspace(browser,{width,height:900,touch:width<800,routes})
 await w.page.addInitScript(()=>{window.EventSource=class{constructor(){setTimeout(()=>this.onopen?.({}),0)}addEventListener(){}close(){}}})
 // The 15-second polls still run; this also lets the test run them now, so an attention count is read after a fresh list.
 await w.page.addInitScript(()=>{const set=window.setInterval.bind(window),clear=window.clearInterval.bind(window),polls=new Map();window.setInterval=(f,d,...a)=>{const id=set(f,d,...a);if(d===15000)polls.set(id,()=>f(...a));return id};window.clearInterval=id=>{polls.delete(id);clear(id)};window.__poll=()=>{for(const f of [...polls.values()])f()}})
 if(plain)await w.page.routeWebSocket('**/api/v2/terminal/socket',socket=>{socket.onMessage(message=>{if(typeof message==='string'&&JSON.parse(message).ticket){socket.send('{"type":"ready"}');socket.send(Buffer.from('plain shell\r\nready> '))}})})
 w.page.on('request',r=>{if(r.method()!=='GET'&&!r.url().endsWith('/end')&&!r.url().endsWith('/seen'))box.writes.push({path:new URL(r.url()).pathname})})
 return{...w,box}
}
// The attention count every surface shows, read after the bell's and the workspace's lists are read again: the tab title's
// prefix, the bell, and the workspace sidebar's badge when the sidebar is on screen.
async function needs(page,box,n){
 const before=box.reads;await page.evaluate(()=>window.__poll())
 await expect.poll(()=>box.reads).toBeGreaterThan(before)
 await page.waitForTimeout(300)
 await expect.poll(()=>page.title()).toMatch(n?new RegExp(`^\\(${n}\\) `):/^[^(]/)
 const bell=page.getByRole('button',{name:/^Needs you: /})
 if(await bell.count())await expect(bell).toHaveAttribute('aria-label',`Needs you: ${n}`)
 const side=page.getByRole('complementary',{name:'Session navigation'})
 if(await side.count()){const badge=side.locator('header .need-badge');if(n)await expect(badge).toHaveText(String(n));else await expect(badge).toHaveCount(0)}
}
async function stop(page){
 const focus=page.getByRole('button',{name:'Exit focus mode',exact:true})
 if(await focus.isVisible())await focus.click()
 await page.getByRole('button',{name:'Session actions',exact:true}).click()
 await page.getByRole('dialog',{name:'Session actions',exact:true}).getByRole('button',{name:'End session…',exact:true}).click()
 await page.getByRole('dialog').getByRole('button',{name:'End session',exact:true}).click()
}
try{
 for(const width of [1440,390]){
  const w=await fixture(width),p=w.page
  await p.goto(base+'/')
  await p.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
  const chat=p.getByRole('dialog',{name:'Gaffer',exact:true})
  await expect(chat).toContainText('General Gaffer chat')
  await chat.getByRole('button',{name:'Factory floor',exact:true}).click()
  const picker=p.getByRole('dialog',{name:'Factory floor',exact:true})
  await expect(picker).toContainText('Ticking shares nothing and starts nothing')
  await picker.getByRole('link',{name:'Open First worker',exact:true}).click()
  await expect(p.locator('.session-screen .title')).toHaveText('First worker')
  await expect(chat).toContainText('First worker')
  await chat.getByRole('button',{name:'Look after this session',exact:true}).click()
  await expect(p.getByRole('dialog',{name:'Look after this session',exact:true})).toContainText('Give Gaffer a goal')
  expect(w.box.writes.filter(x=>!x.path.endsWith('/seen'))).toEqual([])
  expect(w.errors).toEqual([]);await w.context.close()

  const e=await fixture(width),q=e.page
  await q.goto(`${base}/sessions?agent=a1&session=regression-session-001`)
  await expect(q.locator('.session-screen .title')).toHaveText('First worker')
  await q.evaluate(()=>window.dispatchEvent(new CustomEvent('reet:show-session',{detail:'regression-session-002',cancelable:true})))
  await expect(q.locator('.session-screen .title')).toHaveText('Second worker')
  await q.evaluate(()=>window.dispatchEvent(new CustomEvent('reet:show-session',{detail:'regression-session-001',cancelable:true})))
  await expect(q.locator('.session-screen .title')).toHaveText('First worker')
  await needs(q,e.box,0)
  await stop(q)
  await expect(q.locator('.session-screen .title')).toHaveText('Second worker')
  expect(e.box.sessions[0].state).toBe('ended')
  // A confirmed end the person asked for is dismissed and raises no card: no count on the bell, title or sidebar.
  await needs(q,e.box,0)
  e.box.refuse=true;await stop(q)
  await expect(q.getByRole('alert')).toContainText('Stop could not be confirmed')
  await expect(q.locator('.session-screen .title')).toHaveText('Second worker')
  // An uncertain stop holds the view and counts nothing.
  await needs(q,e.box,0)
  e.box.refuse=false;e.box.pending=true;await stop(q)
  await expect(q.getByText(/End requested. Waiting for this machine/)).toBeVisible()
  // A pending end holds the view and counts nothing.
  await needs(q,e.box,0)
  // A lost connection holds the view and is one Connection lost card while unseen, as the server lists it.
  Object.assign(e.box.sessions[1],{state:'lost',attention:'failed',changedSinceSeen:true,updatedAt:Date.now()})
  await q.evaluate(()=>window.dispatchEvent(new Event('online')))
  await expect(q.locator('.session-screen .pill')).toContainText('Connection lost')
  await expect(q.locator('.session-screen .title')).toHaveText('Second worker')
  await needs(q,e.box,1)
  ended(e.box.sessions[1])
  await q.evaluate(()=>window.dispatchEvent(new Event('online')))
  await expect(q).toHaveURL(base+'/sessions?view=list')
  // Its confirmed end clears the card once the view is dismissed.
  await needs(q,e.box,0)
  // Keeping history is separate from dismissing a stopped view.
  await q.goto(`${base}/sessions?agent=a1&session=regression-session-001`)
  await expect(q.locator('.session-screen .title')).toHaveText('First worker')
  await expect(q.getByText('This reet session has ended.',{exact:true})).toBeVisible()
  // Reopening an ended session from history raises no card either.
  await needs(q,e.box,0)
  expect(e.errors).toEqual([]);await e.context.close()
 }
 const cross=await fixture(1440,true),cp=cross.page
 await cp.goto(`${base}/sessions?agent=a1&session=regression-session-001`)
 await expect(cp.locator('.session-screen .title')).toHaveText('First worker')
 await cp.evaluate(()=>window.dispatchEvent(new CustomEvent('reet:show-session',{detail:'regression-session-002',cancelable:true})))
 await expect(cp.locator('.session-screen .title')).toHaveText('Second worker')
 await cp.evaluate(()=>window.dispatchEvent(new CustomEvent('reet:show-session',{detail:'regression-session-001',cancelable:true})))
 await expect(cp.locator('.session-screen .title')).toHaveText('First worker')
 await stop(cp);await expect(cp.locator('.session-screen .title')).toHaveText('Second worker')
 await cp.getByRole('button',{name:'Find machine',exact:true}).click()
 await cp.getByLabel('Machine or shell user').fill('Workshop')
 await cp.getByRole('navigation',{name:'Matching machines'}).getByRole('button',{name:/^Workshop/}).click()
 await expect(cp.locator('.session-screen .title')).toHaveText('Workshop survivor')
 expect(cross.box.sessions[1].state).toBe('running')
 expect(cross.errors).toEqual([]);await cross.context.close()
 const uncertain=await fixture(1440),up=uncertain.page
 await up.goto(`${base}/sessions?agent=a1&session=regression-session-001`)
 await expect(up.locator('.session-screen .title')).toHaveText('First worker')
 uncertain.box.refuse=true;await stop(up)
 await expect(up.getByRole('alert')).toContainText('Stop could not be confirmed')
 await needs(up,uncertain.box,0)
 ended(uncertain.box.sessions[0])
 await up.evaluate(()=>window.dispatchEvent(new Event('online')))
 await expect(up).toHaveURL(base+'/sessions?view=list')
 await needs(up,uncertain.box,0)
 expect(uncertain.box.sessions[1].state).toBe('running')
 await uncertain.context.close()
 // A user-ended plain terminal (not a chat): once its confirmed end is dismissed, no Failed or Finished card and no count
 // appears for it anywhere, here or on Home.
 const plain=await fixture(1440,false,true),pp=plain.page
 await pp.goto(`${base}/sessions?agent=a1&session=regression-session-004`)
 await expect(pp.locator('.session-screen .title')).toHaveText('Plain shell')
 await expect(pp.locator('.pane')).toHaveAttribute('data-connection-state','live')
 await needs(pp,plain.box,0)
 await stop(pp)
 // The confirmed end dismisses the stopped view with a full page load (location.assign), and the address changes before the
 // old page is torn down: wait for the list to be loaded, or the next evaluate can land in the destroyed page ("Execution
 // context was destroyed") when the machine is slow. Reproduced with Chromium's CPU throttled eight-fold.
 await expect(pp).toHaveURL(base+'/sessions?view=list')
 await pp.waitForLoadState('load')
 expect(plain.box.sessions[0]).toMatchObject({id:'regression-session-004',state:'ended',changedSinceSeen:true})
 await needs(pp,plain.box,0)
 await pp.goto(base+'/')
 await expect(pp.getByRole('button',{name:'Needs you: 0',exact:true})).toBeVisible()
 await expect(pp.locator('#needs-you .card')).toHaveCount(0)
 await expect.poll(()=>pp.title()).toBe('Home · reet.sh')
 expect(plain.errors).toEqual([]);await plain.context.close()
 console.log('Session end and Gaffer entry: confirmed dismissal, neighbour selection, uncertain/pending holds, retained history, general-to-session follow-up and no implicit grants passed on desktop and phone; attention counts held at 0 across dismissal, holds and reopening, 1 for a lost connection, and a user-ended plain terminal raised no card.')
}finally{await browser.close()}
