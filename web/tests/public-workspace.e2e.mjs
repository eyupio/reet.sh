// Public entry points must explain the workspace without loading private inventories.
import { chromium, expect } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const screenshots = process.env.REET_UI_SCREENSHOTS
if (screenshots) await mkdir(screenshots,{recursive:true})
try {
 for (const colorScheme of ['light','dark']) {
  for (const width of [1440,390,320]) {
   const context = await browser.newContext({viewport:{width,height:960},colorScheme})
   const page = await context.newPage()
   const errors=[], unexpected=[]
   let status={mode:'accounts',user:null,setupRequired:false,signupEnabled:false,contactEmail:'owner@example.test',passwordReset:true}
   page.on('pageerror',e=>errors.push(e.message))
   await page.route('**/api/**',route=>{
    const path=new URL(route.request().url()).pathname
    if(path==='/api/v2/auth/status')return route.fulfill({json:status})
    if(path==='/api/version')return route.fulfill({json:{version:'test',commit:'fixture'}})
    if(path==='/api/v2/auth/sign-in')return route.fulfill({status:401,json:{error:'Email or password is incorrect.'}})
    if(path==='/api/v2/auth/invite')return route.fulfill({json:{email:'new@example.test',role:'member',expiresAt:Date.now()+86400000}})
    unexpected.push(path)
    return route.fulfill({status:403,json:{error:'Private API must not be called'}})
   })
   const layout=async()=>expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
   const noOldPitch=async()=>expect(await page.locator('body').innerText()).not.toMatch(/skills\.sh|your shelf|skills reported/i)
   await page.goto(base)
   await expect(page.getByRole('heading',{level:1})).toContainText('One command on any machine.')
   await expect(page.getByRole('heading',{level:1})).toContainText('Your whole AI setup shows up, ready to control.')
   // The lede says what the product is for: command agents anywhere, sync, back up, and start an empty machine.
   await expect(page.locator('.welcome .lede')).toHaveText('reet.sh is the control room for your AI agents. Run them on any machine from one place, sync skills and setups across the lot, back it all up, and bring a brand-new, empty machine up to speed with one command.')
   await expect(page).toHaveTitle('Your whole AI setup, on every machine · reet.sh')
   await expect(page.getByText('Ey up. Your AI control room.',{exact:true})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Enrol a machine with one command.'})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Command your agents from one place.'})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Sync and restore.'})).toBeVisible()
   await expect(page.getByRole('heading',{name:'Agents, skills and setups stay in sync.'})).toHaveCount(0)
   await expect(page.getByRole('heading',{name:'Put your agents to work.'})).toHaveCount(0)
   // The illustration shows the real enrolment: the product's own two commands for an empty machine, then the fleet in sync.
   await expect(page.locator('.workspace-preview')).toContainText('reet agent enroll --server')
   await expect(page.locator('.workspace-preview')).toContainText("--env 'fresh-vps'")
   await expect(page.locator('.workspace-preview')).toContainText('reet agent run')
   await expect(page.locator('.workspace-preview .hubsvg:visible .node')).toHaveCount(4)
   await expect(page.locator('.workspace-preview .hubsvg:visible')).toHaveCount(1)
   await expect(page.locator('.workspace-preview').getByRole('button',{name:'Replay'})).toBeVisible()
   // One line to copy sits under the buttons: the install command a visitor from search can run first.
   const chip=page.locator('.enrol-chip')
   await expect(chip).toContainText('$ curl -fsSL https://raw.githubusercontent.com/eyupio/reet.sh/main/scripts/install.sh | sh')
   await expect(chip.getByRole('button',{name:/Copy/})).toBeVisible()
   // Four tiles say what it is for, each with a one-sentence promise and a small example.
   const tiles=page.getByRole('region',{name:'What reet.sh does'})
   await expect(tiles.locator('article')).toHaveCount(4)
   for(const [name,text,cap] of [
    ['Command','Open a terminal on any machine, start an agent in a folder, and pick the session up again later.','claude code · hetzner-vps'],
    ['Sync','Send a skill or setup to one machine or the whole fleet. You review the exact plan before anything changes.','plan ready · approve to apply'],
    ['Back up','Setups and skills are kept on the controller, sealed and verified, so a machine can be put back as it was.','✓ sealed · verified 2 h ago'],
    ['Start fresh','Enrol an empty machine and it picks up your skills and setups. It ends up like the rest.','→ 4 skills, 1 setup · in sync']]){
    const tile=tiles.locator('article').filter({has:page.getByRole('heading',{name,exact:true})})
    await expect(tile).toContainText(text)
    await expect(tile).toContainText(cap)
   }
   // Start with nothing, end with everything: a before and after of one empty machine, drawn by a labelled slider.
   const fresh=page.getByRole('region',{name:'Start with nothing. End with everything.'})
   await expect(fresh.getByText('A new machine',{exact:true})).toBeVisible()
   await expect(fresh).toContainText('Got a new laptop, a fresh VPS or a lab box with nothing on it? Enrol it with one command. Your skills and setups arrive, you review the plan, and it ends up like the rest.')
   const slider=fresh.getByRole('slider',{name:'Compare fresh-vps before and after it is enrolled'})
   await expect(slider).toHaveValue('50')
   await slider.fill('20')
   await expect(fresh.locator('.compare')).toHaveAttribute('data-position','20')
   expect(await fresh.locator('.compare .after').evaluate(el=>getComputedStyle(el).clipPath)).toContain('inset')
   await slider.fill('50')
   await expect(page.getByText(/orchestrat/i)).toHaveCount(0)
   // Guides turn the public docs into a next step from the landing page.
   const guides=page.getByRole('region',{name:'Start with a job to do.'})
   await expect(guides.getByRole('link',{name:/Run Claude Code on a VPS/})).toHaveAttribute('href','https://reet.sh/docs/run-claude-code-on-a-vps/')
   await expect(guides.getByRole('link',{name:/Sync skills between machines/})).toHaveAttribute('href','https://reet.sh/docs/sync-skills-between-machines/')
   await expect(guides.getByRole('link',{name:/Connect an AI client with MCP/})).toHaveAttribute('href','https://reet.sh/docs/self-host-mcp-server/')
   // The colour choice is in the header, as a three-way control that applies at once and is remembered in this browser.
   const picker=page.locator('header .theme-picker')
   await expect(picker.getByRole('button')).toHaveCount(3)
   // Three icons, light then dark then the device, each named for a screen reader and ringed when chosen.
   await expect(picker.getByRole('button').nth(0)).toHaveAccessibleName('Light')
   await expect(picker.getByRole('button').nth(1)).toHaveAccessibleName('Dark')
   await expect(picker.getByRole('button').nth(2)).toHaveAccessibleName('Match this device')
   expect(await picker.locator('svg').count()).toBe(3)
   await picker.getByRole('button',{name:'Dark'}).click()
   expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBe('dark')
   await expect(picker.getByRole('button',{name:'Dark'})).toHaveAttribute('aria-pressed','true')
   await page.reload()
   expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBe('dark')
   await page.locator('header .theme-picker').getByRole('button',{name:'Match this device'}).click()
   expect(await page.evaluate(()=>document.documentElement.dataset.theme)).toBeUndefined()
   await expect(page.getByText('On the workbench',{exact:true})).toBeVisible()
   await expect(page.locator('.next-up')).toContainText('OpenCode today, more agents to come')
   await expect(page.locator('.next-up')).not.toContainText('Agent installation, updates and removal')
   // Invite-only: a visitor from search can install the product or browse the public community before signing in.
   const cta=page.locator('.cta-row')
   await expect(cta.getByRole('link',{name:'Install reet.sh'})).toHaveAttribute('href','#self-host')
   await expect(cta.getByRole('link',{name:'Install reet.sh'})).toHaveClass(/primary/)
   await expect(cta.getByRole('link',{name:'Explore the community'})).toHaveAttribute('href','/community')
   await expect(cta.getByRole('link',{name:'Sign in'})).toHaveAttribute('href','/signin')
   await expect(cta.getByRole('link',{name:'Sign in'})).not.toHaveClass(/primary/)
   await expect(cta.getByRole('link',{name:'Request an invite'})).toHaveAttribute('href',/^mailto:owner@example.test/)
   await expect(cta.getByRole('link',{name:'Request an invite'})).not.toHaveClass(/quiet/)
   await expect(cta.getByRole('link',{name:'Create an account'})).toHaveCount(0)
   // The trust line says the product is free and open source, at a readable size.
   await expect(page.getByLabel('Why reet.sh')).toContainText('AGPL-3.0')
   expect(await page.locator('.trust span').first().evaluate(el=>parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThanOrEqual(12)
   // Nothing in the illustration, the tiles' examples or the slider is below 11px.
   for(const selector of ['.workspace-preview','.tiles','.guides','.compare']){
   const tiny=await page.locator(selector).first().evaluate(root=>{const found=[];const walk=document.createTreeWalker(root,NodeFilter.SHOW_TEXT);for(let node;node=walk.nextNode();){if(!node.textContent.trim())continue;const size=parseFloat(getComputedStyle(node.parentElement).fontSize);if(size<11)found.push(`${size}px "${node.textContent.trim().slice(0,30)}"`)}return found})
   expect(tiny,`${selector} text under 11px`).toEqual([])
   }
   await expect(page.getByRole('figure')).toContainText('Illustration.')
   // Public pages carry the help and trust links, and the roadmap link opens a short roadmap, not an internal log.
   const footer=page.getByRole('contentinfo')
   await expect(footer.getByRole('link',{name:'GitHub'})).toHaveAttribute('href','https://github.com/eyupio/reet.sh')
   await expect(footer.getByRole('link',{name:'Docs'})).toHaveAttribute('href','https://reet.sh/docs/')
   await expect(footer.getByRole('link',{name:'AGPL-3.0'})).toHaveAttribute('href','https://github.com/eyupio/reet.sh/blob/main/LICENSE')
   await expect(page.getByRole('link',{name:/See what.s next/})).toHaveAttribute('href','https://github.com/eyupio/reet.sh/blob/main/docs/roadmap.md')
   await layout(); await noOldPitch()
   if(screenshots)await page.screenshot({path:`${screenshots}/public-${width}-${colorScheme}.png`,fullPage:true})
   await cta.getByRole('link',{name:'Install reet.sh'}).click()
   await expect(page.getByRole('heading',{name:'Give your machines a home.'})).toBeInViewport()
   await page.evaluate(()=>scrollTo(0,0))
   await cta.getByRole('link',{name:'Sign in'}).click()
   await expect(page.getByRole('heading',{name:'Sign in to your workspace'})).toBeVisible()
   await expect(page.getByText('Your machines, with their agents, skills and setups in sync, and the sessions you left running.')).toBeVisible()
   // A wide screen with a keyboard starts in the field; a phone is not handed its soft keyboard on load.
   if(width>=700)await expect(page.locator('#signin-email')).toBeFocused()
   else await expect(page.locator('#signin-email')).not.toBeFocused()
   await page.locator('#signin-email').fill('engineer@example.test')
   await page.getByLabel('Password',{exact:true}).fill('incorrect-fixture')
   await page.getByRole('button',{name:'Sign in',exact:true}).click()
   await expect(page.getByRole('alert')).toContainText('Email or password is incorrect.')
   await layout();await noOldPitch()
   if(screenshots)await page.screenshot({path:`${screenshots}/signin-${width}-${colorScheme}.png`,fullPage:true})
   status={...status,signupEnabled:true}
   await page.goto(base)
   // A visitor who can sign up is led to sign-up; returning members still have a Sign in button.
   await expect(page.locator('.cta-row').getByRole('link',{name:'Create an account'})).toHaveClass(/primary/)
   await expect(page.locator('.cta-row').getByRole('link',{name:'Sign in'})).not.toHaveClass(/primary/)
   await page.getByRole('link',{name:'Create an account'}).click()
   await expect(page.getByRole('heading',{name:'Create your account'})).toBeVisible()
   await expect(page.getByText('Enrol the machines you work on, and your agents, skills and setups go with you to every one of them.')).toBeVisible()
   await expect(page.getByLabel('Confirm password',{exact:true})).toBeVisible()
   await layout();await noOldPitch()
   status={...status,setupRequired:true,bootstrapRequired:true,signupEnabled:false}
   await page.goto(base)
   // Before setup no account exists, so the header does not offer to sign in.
   await expect(page.getByRole('navigation',{name:'Public navigation'}).getByRole('link',{name:/Sign in/})).toHaveCount(0)
   await page.getByRole('link',{name:'Set up this instance'}).click()
   await expect(page.getByRole('heading',{name:'Create your account'})).toBeVisible()
   await expect(page.getByText('Enrol your first machine, enable terminal access')).toBeVisible()
   await expect(page.getByLabel('Access token',{exact:true})).toBeVisible()
   await layout();await noOldPitch()
   // An invitation names the person and what joining gets them, in the welcome page's words.
   await page.goto(`${base}/join?token=t`)
   await expect(page.getByRole('heading',{name:'Join reet.sh'})).toBeVisible()
   await expect(page.getByText("Your team's agents, skills and setups are one sign-in away.")).toBeVisible()
   await layout()
   status={mode:'token'}
   await page.goto(base)
   await expect(page.getByRole('heading',{name:'Open your workspace'})).toBeVisible()
   await layout();await noOldPitch()
   expect(unexpected).toEqual([])
   expect(errors).toEqual([])
   await context.close()
  }
 }

 // The control-room illustration. With reduced motion it is drawn once in its final state and nothing moves;
 // otherwise it replays enrolment step by step and ends in the same place.
 for (const width of [1440,390,320]) {
  const context = await browser.newContext({viewport:{width,height:960},colorScheme:'light',reducedMotion:'reduce'})
  const page = await context.newPage()
  const errors=[]
  page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname
   if(path==='/api/v2/auth/status')return route.fulfill({json:{mode:'accounts',user:null,setupRequired:false,signupEnabled:false,contactEmail:'owner@example.test',passwordReset:true}})
   if(path==='/api/version')return route.fulfill({json:{version:'test',commit:'fixture'}})
   return route.fulfill({status:403,json:{error:'Private API must not be called'}})
  })
  await page.goto(base)
  const figure=page.locator('.workspace-preview')
  const hub=figure.locator('.hubsvg:visible')
  // Reduced motion: the final state at once, every terminal line shown, no running wire, no progress bar, no Replay.
  await expect(hub).toHaveAttribute('data-state','sync')
  await expect(figure.locator('.hub-count')).toHaveText('4 machines enrolled')
  await expect(figure.locator('.shell-line.on')).toHaveCount(await figure.locator('.shell-line').count())
  await expect(hub.locator('.fnode-sub')).toHaveText('Claude Code · 4 skills')
  await expect(hub.locator('.fnode-badge')).toHaveText('in sync')
  await expect(hub.locator('.fprog')).toBeHidden()
  expect(await hub.locator('.wire').first().evaluate(el=>getComputedStyle(el).animationName)).toBe('none')
  await expect(figure.getByRole('button',{name:'Replay'})).toHaveCount(0)
  // The terminal is decoration with the same words in the illustration's label; a screen reader skips it.
  await expect(figure.locator('.preview-shell')).toHaveAttribute('aria-hidden','true')
  await expect(figure).toHaveAttribute('aria-label',/Illustration/)
  // The words in every machine fit inside its box.
  const overflow=await hub.evaluate(svg=>{const bad=[];for(const g of svg.querySelectorAll('.node-group')){const box=g.querySelector('.node').getBBox();for(const t of g.querySelectorAll('text')){const b=t.getBBox();if(b.x<box.x||b.x+b.width>box.x+box.width)bad.push(t.textContent+' '+Math.round(b.x+b.width-box.x-box.width))}}return bad})
  expect(overflow,'text wider than its machine box').toEqual([])
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
  expect(errors).toEqual([])
  await context.close()
 }
 {
  const context = await browser.newContext({viewport:{width:1280,height:900},colorScheme:'dark'})
  const page = await context.newPage()
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname
   if(path==='/api/v2/auth/status')return route.fulfill({json:{mode:'accounts',user:null,setupRequired:false,signupEnabled:false}})
   if(path==='/api/version')return route.fulfill({json:{version:'test',commit:'fixture'}})
   return route.fulfill({status:403,json:{error:'Private API must not be called'}})
  })
  await page.goto(base)
  const figure=page.locator('.workspace-preview')
  const hub=figure.locator('.hubsvg:visible')
  // Motion allowed: it starts with an empty machine waiting, passes through enrolling and restoring, and ends in sync.
  await expect(hub).toHaveAttribute('data-state','empty')
  await expect(figure.locator('.hub-count')).toHaveText('3 machines enrolled')
  await expect(hub.locator('.fnode-badge')).toHaveText('waiting')
  await expect(hub).toHaveAttribute('data-state','fill',{timeout:5000})
  await expect(hub.locator('.fprog')).toBeVisible()
  const overflowWhileFilling=await hub.evaluate(svg=>{const bad=[];for(const g of svg.querySelectorAll('.node-group')){const box=g.querySelector('.node').getBBox();for(const t of g.querySelectorAll('text')){const b=t.getBBox();if(b.x+b.width>box.x+box.width)bad.push(t.textContent)}}return bad})
  expect(overflowWhileFilling,'text wider than its machine box while restoring').toEqual([])
  await expect(hub).toHaveAttribute('data-state','sync',{timeout:8000})
  await expect(figure.locator('.hub-count')).toHaveText('4 machines enrolled')
  await expect(hub.locator('.fprog')).toBeHidden()
  // Replay starts again from the empty machine and lands in the same place.
  await figure.getByRole('button',{name:'Replay'}).click()
  await expect(hub).toHaveAttribute('data-state','empty')
  await expect(figure.locator('.hub-count')).toHaveText('3 machines enrolled')
  await expect(hub).toHaveAttribute('data-state','sync',{timeout:8000})
  await context.close()
 }
 // The slider moves with the keyboard and says what it shows.
 {
  const context = await browser.newContext({viewport:{width:390,height:900},colorScheme:'light',reducedMotion:'reduce'})
  const page = await context.newPage()
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname
   if(path==='/api/v2/auth/status')return route.fulfill({json:{mode:'accounts',user:null,setupRequired:false,signupEnabled:false}})
   if(path==='/api/version')return route.fulfill({json:{version:'test',commit:'fixture'}})
   return route.fulfill({status:403,json:{error:'Private API must not be called'}})
  })
  await page.goto(base)
  const slider=page.getByRole('slider',{name:'Compare fresh-vps before and after it is enrolled'})
  await slider.focus()
  await page.keyboard.press('Home')
  await expect(slider).toHaveValue('0')
  await expect(slider).toHaveAttribute('aria-valuetext','Showing the machine before it is enrolled')
  await page.keyboard.press('End')
  await expect(slider).toHaveValue('100')
  await expect(slider).toHaveAttribute('aria-valuetext','Showing the machine after it is enrolled')
  await page.keyboard.press('ArrowLeft')
  await expect(slider).toHaveAttribute('aria-valuetext',/99% shows the machine after it is enrolled/)
  await context.close()
 }
 console.log('Public workspace: light/dark, desktop/phone, invite/signup/setup/token entry points passed; no private API requests.')
} finally {await browser.close()}
