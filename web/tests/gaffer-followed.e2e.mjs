import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { openWorkspace } from './helpers/workspace.mjs'

const base=process.env.REET_UI_URL||'http://127.0.0.1:4173'
const shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-followed'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),generalId='f'.repeat(32)
function fixture(){
  const pins={providerId:pid,providerRevision:1,model:'gaffer-model',contextRevision:1,workerRevision:1}
  const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowSessions:true,allowCode:true,hasKey:true,createdAt:now,updatedAt:now}
  const view={conversation:{id:generalId,sessionId:'',providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
  function row(n,title,state,attention,available=true){
    const id=n.toString(16).repeat(32),sid=`followed-session-${n}`
    return {goal:{...pins,id,conversationId:id,sessionId:sid,revision:1,text:'Raise the release PR and address its checks. Ask me before changing scope. Do not merge.',state,note:attention?'The worker needs your decision before continuing.':'Waiting for new worker progress.',maxReviews:12,usedReviews:2,expiresAt:now+3600000,createdAt:now-7200000,updatedAt:now,lastSeq:42,lastObservedAt:now-60000,lastReviewedAt:now-30000,nextAt:now,summary:'The worker reports a PR is open. One check still needs a fix. A broader dependency update needs your decision before the worker continues.',inputTokens:120,outputTokens:80,usageReported:true},session:{id:sid,agentId:'a1',title,state:'running'},available,attention,reason:available?'':'This session is archived or unavailable. Use Conversation history to remove the saved chat.'}
  }
  const urgent=row(1,'Release needs a decision','needs_user',true)
  urgent.delivery={id:'9'.repeat(32),sessionId:urgent.goal.sessionId,queueId:'8'.repeat(32),text:'Check the failing test and report back. Do not merge.',state:'uncertain',revision:3,grantRevision:1,createdAt:now-45000}
  urgent.goal.pendingId=urgent.delivery.id
  const active=row(2,'Backup improvements','active',false),paused=row(3,'Paused design review','paused',false),archived=row(4,'Archived session','paused',false,false),complete=row(5,'Finished release','completed',false)
  active.delivery={...urgent.delivery,id:'7'.repeat(32),state:'sent',sessionId:active.goal.sessionId};active.goal.pendingId=active.delivery.id
  const box={rows:[urgent,active,paused,archived,complete],controls:[],unexpectedWrites:[],reads:0,fail:false}
  const routes={
    '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
    '/api/v2/gaffer/conversations':request=>new URL(request.url()).searchParams.get('all')==='true'?{conversations:[view.conversation]}:view,
    '/api/v2/gaffer/goals':request=>{expect(request.method()).toBe('GET');box.reads++;return box.fail?{status:503,json:{error:'The controller could not read the job sheet.'}}:{goals:box.rows,readAt:now}},
  }
  for(const r of box.rows){
    const path=`/api/v2/gaffer/conversations/${r.goal.conversationId}/goal`
    routes[path]=request=>{expect(request.method()).toBe('GET');return {goal:r.goal,pins,allowed:true}}
    routes[path+'/control']=request=>{const input=request.postDataJSON();expect(input.revision).toBe(r.goal.revision);box.controls.push(input);r.goal.state={pause:'paused',resume:'active',complete:'completed',stop:'stopped'}[input.action];r.goal.revision++;r.attention=false;return {goal:r.goal,pins,allowed:true}}
  }
  return {box,routes,active}
}
try {
  for(const theme of ['light','dark'])for(const width of [1440,800,390,320]){
    const f=fixture(),w=await openWorkspace(browser,{width,height:1000,theme,touch:width<1000,routes:f.routes})
    w.page.on('request',r=>{if(new URL(r.url()).pathname.startsWith('/api/')&&r.method()!=='GET'&&!r.url().includes('/goal/control'))f.box.unexpectedWrites.push(r.url())})
    try {
      await w.page.goto(base+'/');await w.page.getByRole('button',{name:'Ask Gaffer',exact:true}).click()
      const chat=w.page.getByRole('dialog',{name:'Gaffer',exact:true})
      await chat.getByPlaceholder('Talk it through with Gaffer…').fill('Keep my general chat draft')
      const open=async()=>{await chat.getByRole('button',{name:'Chat options',exact:true}).click();await w.page.getByRole('dialog',{name:'Chat options',exact:true}).getByRole('button',{name:'Factory floor',exact:true}).click()}
      await open()
      const overview=w.page.getByRole('dialog',{name:'Factory floor',exact:true})
      await expect(overview.getByRole('listitem')).toHaveCount(5)
      await expect(overview.getByRole('listitem').first().getByRole('heading',{name:'Release needs a decision',exact:true})).toBeVisible()
      await expect(overview.getByText('Delivery uncertain',{exact:false}).first()).toBeVisible()
      await expect(overview.getByText('Handed to worker',{exact:false}).first()).toBeVisible()
      expect(await overview.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      const archived=overview.getByRole('listitem').filter({has:w.page.getByRole('heading',{name:'Archived session',exact:true})})
      await expect(archived.getByRole('link',{name:'Open session',exact:true})).toHaveCount(0)
      await expect(archived.getByRole('button',{name:'Review goal',exact:true})).toHaveCount(0)
      await expect(archived.getByRole('button',{name:'Conversation history',exact:true})).toHaveCount(1)
      if(width===1440||width===390)await overview.screenshot({path:`${shots}/gaffer-followed-${width}-${theme}.png`})
      f.box.rows[0].goal.summary+=' <img src=x onerror="window.__overviewXss=true">'
      await overview.getByRole('button',{name:'Refresh',exact:true}).click()
      await overview.getByRole('button',{name:/^Needs review/}).click();await expect(overview.getByRole('listitem')).toHaveCount(1)
      await overview.getByText('Review & evidence',{exact:true}).click()
      await expect(overview.getByText('<img src=x onerror="window.__overviewXss=true">',{exact:false}).first()).toBeVisible()
      expect(await w.page.evaluate(()=>window.__overviewXss)).toBeUndefined()
      await expect(overview.locator('img[src="x"]')).toHaveCount(0)
      await expect(overview.getByText('Check the failing test and report back. Do not merge.',{exact:true})).toBeVisible()
      await expect(overview.getByText('Handoff records delivery to the worker, not completion of the work.',{exact:true})).toBeVisible()
      await overview.getByRole('button',{name:/^Following/}).click();await expect(overview.getByRole('listitem')).toHaveCount(1)
      await expect(overview.getByRole('link',{name:'Open session',exact:true})).toHaveAttribute('href','/sessions?agent=a1&session=followed-session-2&gaffer=1')
      f.active.delivery.state='queued';await overview.getByRole('button',{name:'Refresh',exact:true}).click()
      await expect(overview.getByText('Queued for next turn',{exact:true}).first()).toBeVisible()
      await overview.getByRole('button',{name:'Review goal',exact:true}).click()
      const goal=w.page.getByRole('dialog',{name:'Look after this session',exact:true})
      await goal.getByRole('button',{name:'Pause follow-up',exact:true}).click()
      await expect(goal.getByRole('heading',{name:'Paused',exact:true})).toBeVisible()
      expect(f.box.controls).toHaveLength(1)
      await w.page.keyboard.press('Escape');await expect(goal).toHaveCount(0)
      // A paused row leaves the active filter; remaining entries and draft survive.
      await expect(overview.getByText('No followed sessions in this view.',{exact:true})).toBeVisible()
      await expect(overview.getByRole('button',{name:'Refresh',exact:true})).toBeFocused()
      await overview.getByRole('button',{name:'All',exact:true}).click()
      f.box.fail=true;await overview.getByRole('button',{name:'Refresh',exact:true}).click()
      await expect(overview.getByRole('alert')).toHaveText('The controller could not read the job sheet.')
      await expect(overview.getByRole('listitem')).toHaveCount(5)
      f.box.fail=false;f.box.rows=[];await overview.getByRole('button',{name:'Refresh',exact:true}).click()
      await expect(overview.getByRole('heading',{name:'Nowt on the job sheet yet.',exact:true})).toBeVisible()
      await w.page.keyboard.press('Escape');await expect(chat.getByRole('button',{name:'Chat options',exact:true})).toBeFocused()
      await expect(chat.getByPlaceholder('Talk it through with Gaffer…')).toHaveValue('Keep my general chat draft')
      expect(f.box.unexpectedWrites).toEqual([]);expect(w.errors).toEqual([])
    } finally { await w.context.close() }
  }
  console.log('Followed sessions: eight theme/width views, general-chat entry, decision filters, retained evidence/escaping, receipt semantics, nested goal control, stale-read retry, empty state, draft and focus passed. No model/worker writes.')
} finally { await browser.close() }
