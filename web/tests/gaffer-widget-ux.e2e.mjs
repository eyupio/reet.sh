import {chromium,expect} from '@playwright/test'
import {mkdir} from 'node:fs/promises'
import {openWorkspace} from './helpers/workspace.mjs'

const base=process.env.REET_UI_URL||'http://127.0.0.1:4173',shots=process.env.REET_UI_SCREENSHOTS||'/tmp/reet-gaffer-widget'
await mkdir(shots,{recursive:true})
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH||'/usr/bin/chromium',args:['--no-sandbox']})
const now=Date.now(),pid='a'.repeat(32),cid='b'.repeat(32),sid='widget-layout-session'
const provider={id:pid,revision:1,name:'Workshop Gaffer',kind:'openai_compatible',baseUrl:'https://example.test/v1',model:'gaffer-model',enabled:true,isDefault:true,localOnly:false,allowStatus:false,allowSessions:false,allowCode:false,hasKey:true,createdAt:now,updatedAt:now}
const view={conversation:{id:cid,sessionId:sid,providerId:pid,model:'',revision:1,createdAt:now,updatedAt:now},turns:[]}
const session={id:sid,agentId:'a1',title:'Improve the release workflow',tool:'opencode',model:'working-model',structured:true,state:'running',createdAt:now,updatedAt:now,exitCode:null,dir:'/work'}
const routes={
  '/api/v2/gaffer/providers':{providers:[provider],presets:[],available:true,canUseLocal:true,maxProviders:12},
  '/api/v2/gaffer/conversations':view,
  '/api/v2/terminal/status':{configured:true,available:true,reason:''},
  '/api/v2/terminal/sessions':{sessions:[session]},
  '/api/v2/terminal/machines':{machines:[{agentId:'a1',host:'Workshop',ready:true,serverEnabled:true,capabilities:{protocolVersion:1,os:'linux',osUser:'test',terminalEnabled:true,terminalControl:true,readiness:'ready',acp:true,acpLiveControls:true}}]},
  '/api/v2/projects':{projects:[]},
  [`/api/v2/terminal/sessions/${sid}/seen`]:{},
  [`/api/v2/terminal/sessions/${sid}/chat/controls`]:{controls:{working:false,waiting:false,model:'working-model',options:[]}},
  [`/api/v2/terminal/sessions/${sid}/chat/queue`]:{queue:[]},
}
try{
  for(const theme of ['light','dark'])for(const [width,height] of [[1440,1000],[800,1000],[767,1000],[390,1000],[320,1000],[844,390]]){
    const w=await openWorkspace(browser,{width,height,theme,touch:width<1000,routes})
    await w.page.addInitScript(()=>{window.EventSource=class{addEventListener(){}close(){}}})
    try{
      await w.page.goto(`${base}/sessions?agent=a1&session=${sid}`)
      const launch=w.page.getByRole('button',{name:'Ask Gaffer',exact:true}),chat=w.page.getByRole('dialog',{name:'Gaffer',exact:true})
      await expect(launch).toHaveCount(1)
      // The launcher floats over the workspace instead of reserving a column, but it must stay in view and
      // lift clear of the composer, queue and other opt-in docks that grow at the bottom of the screen.
      await expect.poll(()=>launch.evaluate(el=>{
        const a=el.getBoundingClientRect()
        const docks=[...document.querySelectorAll('[data-floating-clearance]')].map(node=>node.getBoundingClientRect()).filter(r=>r.width&&r.height)
        return {width:a.width,height:a.height,inView:a.bottom<=innerHeight&&a.top>=0,clear:docks.every(dock=>a.bottom<=dock.top||a.top>=dock.bottom||a.right<=dock.left||a.left>=dock.right)}
      }),{message:`${theme} ${width}x${height}: launcher fits and clears composer and queue docks`}).toEqual({width:56,height:56,inView:true,clear:true})
      await launch.click();await expect(chat).toBeVisible()
      for(const name of ['New conversation','Conversation history','Chat options','Close Gaffer'])await expect(chat.getByRole('button',{name,exact:true})).toBeVisible()
      await expect(chat.getByRole('group',{name:'Gaffer panel tools',exact:true})).toHaveCount(1)
      // This fixture's saved chat holds no messages, context or goal, so there
      // is nothing for a fresh chat to clear.
      await expect(chat.getByRole('button',{name:'New conversation',exact:true})).toBeDisabled()
      await expect(chat.getByRole('button',{name:'Conversation history',exact:true})).toBeVisible()
      await expect(chat.getByRole('button',{name:'Look after this session',exact:true})).toBeVisible()
      await chat.getByRole('textbox',{name:'Message Gaffer',exact:true}).fill('Preserve this draft while I resize')
      if(width>=768&&height>500){
        for(const [name,remWidth,remHeight]of [['Compact',24,32],['Default',28,42],['Expanded',46,52]]){
          await chat.getByRole('button',{name,exact:true}).click()
          const rect=await chat.boundingBox();expect(Math.abs(rect.width-Math.min(remWidth*16,width-32))).toBeLessThan(2);expect(Math.abs(rect.height-Math.min(remHeight*16,936))).toBeLessThan(2)
        }
        await chat.evaluate(el=>{el.style.width='370px';el.style.height='440px'})
        await chat.getByRole('button',{name:'Expanded',exact:true}).click()
        expect(await chat.evaluate(el=>el.style.width)).toBe('')
        await chat.getByRole('button',{name:'Place Gaffer on the left',exact:true}).click();expect((await chat.boundingBox()).x).toBe(16)
        const handle=chat.getByRole('button',{name:'Move Gaffer panel with arrow keys or by dragging',exact:true})
        await handle.focus();await w.page.keyboard.press('ArrowRight');expect((await chat.boundingBox()).x).toBe(26)
        // The pointer drags from the identity button, and the whole header is
        // also a drag area, so an empty stretch moves the panel too.
        const beforeDrag=await chat.boundingBox(),grip=await handle.boundingBox()
        await w.page.mouse.move(grip.x+grip.width/2,grip.y+grip.height/2)
        await w.page.mouse.down()
        await w.page.mouse.move(grip.x+grip.width/2+40,grip.y+grip.height/2+30,{steps:6})
        await w.page.mouse.up()
        const dragged=await chat.boundingBox()
        // Synthetic pointer input may coalesce its last move, so allow a couple
        // of pixels; the panel follows the release point within that.
        expect(dragged.x).toBeGreaterThan(beforeDrag.x+30);expect(dragged.x).toBeLessThan(beforeDrag.x+50)
        expect(dragged.y).toBeGreaterThan(beforeDrag.y+20);expect(dragged.y).toBeLessThan(beforeDrag.y+40)
        await expect(chat).toHaveAttribute('data-gaffer-placed','true')
        await chat.getByRole('button',{name:'Snap Gaffer back to its corner',exact:true}).click()
        expect(await chat.getAttribute('data-gaffer-placed')).toBeNull()
        const snapped=await chat.boundingBox()
        expect(snapped.x).toBe(16);expect(Math.round(snapped.y)).toBe(Math.round(beforeDrag.y))
        await handle.focus();await w.page.keyboard.press('ArrowDown')
        expect(Math.round((await chat.boundingBox()).y)).toBe(Math.round(beforeDrag.y)+10)
        // Selecting words in the transcript must not snap the panel back.
        await chat.getByRole('heading',{name:/Ey up/}).dblclick()
        expect(Math.round((await chat.boundingBox()).y)).toBe(Math.round(beforeDrag.y)+10)
        await chat.getByRole('heading',{name:'Gaffer',exact:true}).dblclick()
        expect(Math.round((await chat.boundingBox()).y)).toBe(Math.round(beforeDrag.y))
        await chat.getByRole('button',{name:'Full screen',exact:true}).click()
        expect(await chat.evaluate(el=>el.matches(':modal'))).toBe(true)
        expect(Math.abs((await chat.boundingBox()).width-(width-32))).toBeLessThan(2)
        await chat.getByRole('button',{name:'Restore panel',exact:true}).click()
        expect(await chat.evaluate(el=>el.matches(':modal'))).toBe(false)
        expect((await chat.boundingBox()).x).toBe(16)
        await chat.getByRole('button',{name:'Default',exact:true}).click()
      }else{expect(await chat.evaluate(el=>el.matches(':modal'))).toBe(true);await expect(chat.getByRole('group',{name:'Gaffer panel size',exact:true})).toBeHidden()}
      const size=await chat.boundingBox();expect(size.x).toBeGreaterThanOrEqual(0);expect(size.y).toBeGreaterThanOrEqual(0);expect(size.x+size.width).toBeLessThanOrEqual(width+1)
      expect(await chat.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true)
      if(width===1440||width===390)await chat.screenshot({path:`${shots}/gaffer-widget-${width}-${theme}.png`})
      await w.page.keyboard.press('Escape');await expect(launch).toBeFocused()
      const parked=await launch.boundingBox()
      if(width===390){
        await w.page.locator('.chat-pane .composer textarea').focus()
        await w.page.evaluate(()=>{const vv=window.visualViewport;Object.defineProperty(vv,'height',{configurable:true,get:()=>Math.floor(innerHeight*.55)});vv.dispatchEvent(new Event('resize'))})
        await expect(launch).toHaveCount(0)
        await expect.poll(()=>w.page.locator('[data-gaffer-space]').evaluate(el=>el.getBoundingClientRect().height)).toBe(0)
        await w.page.evaluate(()=>{const vv=window.visualViewport;delete vv.height;vv.dispatchEvent(new Event('resize'))})
        await expect(launch).toBeVisible()
        await expect.poll(async()=>(await launch.boundingBox()).y).toBeCloseTo(parked.y,0)
      }
      await w.page.mouse.wheel(0,500);await expect.poll(async()=>(await launch.boundingBox()).y).toBeCloseTo(parked.y,0)
      await launch.click();await expect(chat.getByRole('textbox',{name:'Message Gaffer',exact:true})).toHaveValue('Preserve this draft while I resize')
      if(width>=768&&height>500){await expect(chat.getByRole('button',{name:'Default',exact:true})).toHaveAttribute('aria-pressed','true');expect(await chat.evaluate(el=>el.style.width)).toBe('')}
      expect(w.errors).toEqual([])
    }finally{await w.context.close()}
  }
  // A saved chat that holds messages offers a fresh start; confirming clears
  // the saved conversation and returns the composer, without touching the worker.
  {
    const saved={...view,turns:[{id:'c'.repeat(32),conversationId:cid,requestRevision:1,prompt:'Keep the release notes tidy.',answer:'Righto — they are tidy.',state:'completed',providerId:pid,providerName:'Workshop Gaffer',model:'gaffer-model',inputTokens:12,outputTokens:8,usageReported:true,cut:false,startedAt:now-2000,updatedAt:now-1000,finishedAt:now-1000}]}
    let removed=0
    const fresh=await openWorkspace(browser,{width:1440,height:1000,theme:'dark',routes:{...routes,
      '/api/v2/gaffer/conversations':saved,
      [`/api/v2/gaffer/conversations/${cid}`]:request=>{if(request.method()==='DELETE'){removed++;return {removed:true}}return {status:404,json:{error:'Unmocked conversation request'}}}}})
    try{
      await fresh.page.goto(`${base}/sessions?agent=a1&session=${sid}`)
      const launch=fresh.page.getByRole('button',{name:'Ask Gaffer',exact:true}),chat=fresh.page.getByRole('dialog',{name:'Gaffer',exact:true})
      await launch.click();await expect(chat).toBeVisible()
      const tool=chat.getByRole('button',{name:'New conversation',exact:true})
      await expect(tool).toBeEnabled();await tool.click()
      const ask=fresh.page.getByRole('dialog',{name:'Start a new Gaffer conversation?',exact:true})
      await expect(ask).toBeVisible()
      await ask.getByRole('button',{name:'Start fresh chat',exact:true}).click()
      await expect.poll(()=>removed).toBe(1)
      await expect(chat.getByText('Fresh chat ready. Ask away.',{exact:true})).toBeVisible()
      await expect(tool).toBeDisabled()
      await expect(chat.getByRole('textbox',{name:'Message Gaffer',exact:true})).toBeFocused()
      expect(fresh.errors).toEqual([])
    }finally{await fresh.context.close()}
  }
  console.log('Widget UX: twelve width/theme views, including a landscape phone, stable clear 56 px launcher, mascot, header tools, three presets, native resize reset, left/right placement, pointer drag, snap back, keyboard move, modal full screen/phone, fresh chat, drafts and focus passed.')
}finally{await browser.close()}
