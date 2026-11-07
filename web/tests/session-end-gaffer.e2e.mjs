// Explicit stop leaves the view after confirmed closure; general Gaffer can select a worker without granting access.
import {chromium,expect} from '@playwright/test'
import {openWorkspace} from './helpers/workspace.mjs'
const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const pid='a'.repeat(32),cid='b'.repeat(32),now=Date.now()
const make=(id,title)=>({id,agentId:'a1',title,structured:true,tool:'opencode',model:'worker-model',state:'running',dir:'/work',createdAt:now,updatedAt:now,exitCode:null})
async function fixture(width,crossMachine=false){
 const box={sessions:[make('regression-session-001','First worker'),make('regression-session-002','Second worker')],writes:[],pending:false,refuse:false}
 if(crossMachine){box.sessions[1].agentId='a2';box.sessions.push(make('regression-session-003','Workshop survivor'))}
 const provider={id:pid,revision:1,name:'Gaffer fixture',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,hasKey:true,createdAt:now,updatedAt:now}
 const routes={
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':()=>({sessions:box.sessions}),
  '/api/v2/terminal/machines':{machines:[['a1','Workshop'],['a2','Builder']].map(([agentId,host])=>({agentId,host,ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalControl:true,terminalEnabled:true,readiness:'ready',acp:true,acpHistory:true,acpLiveControls:true}}))},
  '/api/v2/gaffer/providers':{providers:[provider],available:true,canUseLocal:true,presets:[]},
  '/api/v2/gaffer/conversations':request=>({conversation:{id:cid,sessionId:new URL(request.url()).searchParams.get('sessionId')||'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}),
  [`/api/v2/gaffer/conversations/${cid}/goal`]:{goal:null,pins:{providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1},allowed:true},
 }
 for(const item of box.sessions){
  const path=`/api/v2/terminal/sessions/${item.id}`
  routes[path+'/end']=request=>{box.writes.push({path:path+'/end',body:request.postDataJSON()});if(box.refuse)return{status:503,json:{error:'Stop could not be confirmed'}};item.state=box.pending?'end_requested':'ended';return{}}
  routes[path+'/chat/controls']={controls:{working:false,waiting:false,model:'worker-model',options:[]}}
  routes[path+'/chat/queue']={queue:[]};routes[path+'/seen']={};routes[path+'/skills']={skills:[]}
 }
 const w=await openWorkspace(browser,{width,height:900,touch:width<800,routes})
 await w.page.addInitScript(()=>{window.EventSource=class{constructor(){setTimeout(()=>this.onopen?.({}),0)}addEventListener(){}close(){}}})
 w.page.on('request',r=>{if(r.method()!=='GET'&&!r.url().endsWith('/end')&&!r.url().endsWith('/seen'))box.writes.push({path:new URL(r.url()).pathname})})
 return{...w,box}
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
  await chat.getByRole('button',{name:'Follow a session',exact:true}).click()
  const picker=p.getByRole('dialog',{name:'Choose a session for Gaffer',exact:true})
  await expect(picker).toContainText('Selecting a session does not enable follow-up')
  await picker.getByRole('link',{name:/First worker/}).click()
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
  await stop(q)
  await expect(q.locator('.session-screen .title')).toHaveText('Second worker')
  expect(e.box.sessions[0].state).toBe('ended')
  e.box.refuse=true;await stop(q)
  await expect(q.getByRole('alert')).toContainText('Stop could not be confirmed')
  await expect(q.locator('.session-screen .title')).toHaveText('Second worker')
  e.box.refuse=false;e.box.pending=true;await stop(q)
  await expect(q.getByText(/End requested. Waiting for this machine/)).toBeVisible()
  e.box.sessions[1].state='lost'
  await q.evaluate(()=>window.dispatchEvent(new Event('online')))
  await expect(q.locator('.session-screen .pill')).toContainText('Connection lost')
  await expect(q.locator('.session-screen .title')).toHaveText('Second worker')
  e.box.sessions[1].state='ended'
  await q.evaluate(()=>window.dispatchEvent(new Event('online')))
  await expect(q).toHaveURL(base+'/sessions?view=list')
  // Keeping history is separate from dismissing a stopped view.
  await q.goto(`${base}/sessions?agent=a1&session=regression-session-001`)
  await expect(q.locator('.session-screen .title')).toHaveText('First worker')
  await expect(q.getByText('This reet session has ended.',{exact:true})).toBeVisible()
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
 uncertain.box.sessions[0].state='ended'
 await up.evaluate(()=>window.dispatchEvent(new Event('online')))
 await expect(up).toHaveURL(base+'/sessions?view=list')
 expect(uncertain.box.sessions[1].state).toBe('running')
 await uncertain.context.close()
 console.log('Session end and Gaffer entry: confirmed dismissal, neighbour selection, uncertain/pending holds, retained history, general-to-session follow-up and no implicit grants passed on desktop and phone.')
}finally{await browser.close()}
