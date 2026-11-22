// The first-session guide on Home: after the first machine connects, until a first reply (or Not now), Home shows which agents
// are ready, offers one read-only question, and keeps Factory, Sessions, terminals and New session where they are.
import { chromium, expect } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { openWorkspace, terminalMachine, active, modelList, modelEntry } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const shots = process.env.REET_UI_SCREENSHOTS
if (shots) mkdirSync(shots, {recursive:true})
// 1280 light is a whole page; 390 dark is the viewport with the guide at its top, so the fixed bottom bar is where it really is.
const shot = async (page, name, g) => {
 if (!shots) return
 const narrow = page.viewportSize().width < 600
 if (narrow && g) await g.evaluate(el => el.scrollIntoView({block:'start'}))
 await page.mouse.move(0, 0)
 await page.waitForTimeout(300)
 await page.screenshot({path:`${shots}/${name}-${narrow ? '390-dark' : '1280-light'}.png`, fullPage:!narrow})
}
const now = Date.now()
const PROMPT = 'Say which folder you are in and list the files and folders you can see there. Do not change, create or delete anything.'

const account = (tool, over = {}) => ({tool, state:'signed_in', method:'subscription', account:'sam@example.test', plan:'pro', limitsState:'not_asked', ...over})
const accountsOf = accounts => ({state:'read', sentence:'', planLimits:true, checkedAt:Date.now(), accounts})
const descriptor = (tool, adapter) => ({tool, adapter, state:'available'})

// One machine, `studio`, connected, with these agents installed. `world` is what the machine answers and what the controller keeps;
// a test changes it and the page sees the change on its next read.
function scene({tools = ['claude', 'opencode'], acpAgents = [descriptor('claude', 'claude-agent-acp')], accounts = [account('claude'), account('opencode', {method:'api_key', account:undefined, plan:undefined})], models = {}, sessions = [], projects = []} = {}) {
 const world = {accounts, models, sessions, projects, posts:[], answer:null}
 const machine = terminalMachine('a1', 'studio', true)
 machine.capabilities = {...machine.capabilities, launch:true, agentsV2:true, models:true, acp:true, acpAgents, tools}
 const routes = {
  '/api/v2/machines/health': {serverNow:Date.now(), machines:[{agentId:'a1', host:'studio', state:'connected', capabilities:machine.capabilities}]},
  '/api/v2/terminal/status': active,
  '/api/v2/terminal/machines': {machines:[machine]},
  '/api/v2/terminal/sessions': request => {
   if (request.method() !== 'POST') return {sessions:world.sessions}
   const body = request.postDataJSON()
   world.posts.push(body)
   if (world.answer) return world.answer
   const session = {id:'s1', agentId:'a1', title:body.title, state:'running', attention:'working', createdAt:Date.now(), updatedAt:Date.now(), exitCode:null, tool:body.tool, structured:!!body.structured}
   world.sessions = [session]
   return session
  },
  '/api/v2/folder-projects': () => ({projects:world.projects, savedWaiting:0}),
  '/api/v2/machines/a1/accounts': () => accountsOf(world.accounts),
  '/api/v2/machines/a1/accounts/refresh': () => accountsOf(world.accounts),
  '/api/v2/machines/a1/models': request => {
   const tool = new URL(request.url()).searchParams.get('tool')
   return world.models[tool] ?? modelList({tool, models:[modelEntry('anthropic/claude-sonnet-4-5')]})
  },
 }
 return {world, routes}
}
// Each state at 1280 light and at 390 dark: `check` gets the page, the guide, the world and a `capture(name)` for it.
const both = async (scenery, check, options = {}) => {
 for (const [width, theme] of [[1280, 'light'], [390, 'dark']]) {
  const {context, page, world, errors} = await open({width, height:width < 600 ? 844 : 800, theme, touch:width < 600, ...options}, scenery)
  await page.goto(base)
  const g = guide(page)
  await check({page, g, world, width, capture: name => shot(page, name, g)})
  expect(errors).toEqual([])
  await context.close()
 }
}
const open = async (options = {}, scenery = {}) => {
 const {world, routes} = scene(scenery)
 const opened = await openWorkspace(browser, {routes, ...options})
 return {...opened, world}
}
// Home reads again when the page is shown; a read already under way is let finish, so ask until the answer shows.
const readAgain = async (page, check) => expect(async () => { await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange'))); await check() }).toPass({timeout:15_000})
const guide = page => page.getByRole('region', {name:'Your first session'})
// What stays reachable the whole time: Factory and Sessions in the navigation, and a way to start a session on Home (an empty Home
// offers it as Start an agent session, with the terminal and Factory starts beside it).
const start = page => page.getByRole('button', {name:/^(New session|Start an agent session)$/})
const reachable = async (page, width) => {
 const nav = page.getByRole('navigation', {name:'Main navigation'})
 if (width >= 1100) await expect(nav.getByRole('link', {name:'Factory'})).toBeVisible()
 await expect(nav.getByRole('link', {name:'Sessions'})).toBeVisible()
 await expect(start(page)).toBeVisible()
}

try {
 // A connected machine and nothing yet: one quiet line per agent that is fine, and what is wrong said only where it is, in stage 3's words.
 if (run('ready')) {
  await both({models:{opencode:modelList({tool:'opencode', state:'failed', models:[], sentence:''})}}, async ({page, g, width, capture}) => {
   await expect(g.getByRole('heading', {name:'Your first session'})).toBeVisible()
   await expect(g.getByText('studio is connected. Ask an agent one question that changes nothing.')).toBeVisible()
   const claude = g.getByRole('radio', {name:/Claude Code/}), opencode = g.getByRole('radio', {name:/OpenCode/})
   await expect(claude).toBeChecked()
   await expect(g.locator('label').filter({hasText:'Claude Code'})).toContainText('Signed in as sam@example.test · Pro plan')
   // A model list that was read says nothing; one that could not be read says so.
   await expect(g.locator('label').filter({hasText:'Claude Code'})).not.toContainText('Models read from studio.')
   await expect(g.locator('label').filter({hasText:'OpenCode'})).toContainText('Signed in with an API key')
   await expect(g.locator('label').filter({hasText:'OpenCode'})).toContainText('That machine couldn’t read its models.')
   await expect(g.getByText(PROMPT)).toBeVisible()
   await expect(g.getByText('no YOLO')).toHaveCount(0)
   await expect(g.getByRole('button', {name:'Ask Claude Code'})).toBeEnabled()
   await expect(opencode).not.toBeChecked()
   await reachable(page, width)
   // While the guide shows, it holds the one filled step on the page: the workbench's own is plain, and the header's New session is not on Home yet.
   await expect(page.getByRole('button', {name:'Start an agent session'})).not.toHaveClass(/primary/)
   await expect(page.getByRole('button', {name:'Open terminal', exact:true})).toBeVisible()
   // The status region is there from the start, empty, so its first message is announced.
   await expect(g.getByRole('status')).toBeAttached()
   if (width < 600) for (const control of [g.getByRole('button', {name:'Ask Claude Code'}), g.getByRole('button', {name:'Not now'}), g.getByText('Use reet on your phone')]) {
    expect((await control.boundingBox()).height, 'touch target height').toBeGreaterThanOrEqual(43.5)
   }
   // Not now sits on the heading's line, at the content edge.
   const heading = await g.getByRole('heading', {name:'Your first session'}).boundingBox(), notNow = await g.getByRole('button', {name:'Not now'}).boundingBox(), box = await g.boundingBox()
   expect(Math.abs(heading.y + heading.height / 2 - (notNow.y + notNow.height / 2)), 'Not now on the heading line').toBeLessThan(8)
   expect(box.x + box.width - (notNow.x + notNow.width), 'Not now inside the guide').toBeLessThan(width < 600 ? 14 : 20)
   expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `no sideways scroll at ${width}px`).toBe(true)
   await capture('guide-ready')
  })
 }

 // No second filled step once the guide is gone.
 if (run('workbench')) {
  const {context, page} = await open({width:1280})
  await page.goto(base)
  await expect(page.getByRole('button', {name:'Start an agent session'})).not.toHaveClass(/primary/)
  await guide(page).getByRole('button', {name:'Not now'}).click()
  await expect(page.getByRole('button', {name:'Start an agent session'})).toHaveClass(/primary/)
  await context.close()
 }

 // Someone with sessions already, or a machine that is not connected, is not given the guide.
 if (run('not-first')) {
  let {context, page} = await open({width:1280}, {sessions:[{id:'old', agentId:'a1', title:'Old work', state:'running', attention:'working', createdAt:now, updatedAt:now, exitCode:null, tool:'claude'}]})
  await page.goto(base)
  await expect(page.getByRole('heading', {name:'Home', level:1})).toBeVisible()
  await expect(page.getByText('Old work').first()).toBeVisible()
  await expect(guide(page)).toHaveCount(0)
  await context.close()
  ;({context, page} = await open({width:1280, routes:{'/api/v2/machines/health':{serverNow:Date.now(), machines:[]}}}))
  await page.goto(base)
  await expect(page.getByRole('heading', {name:'Home', level:1})).toBeVisible()
  await expect(guide(page)).toHaveCount(0)
  await context.close()
 }

 // A first question: read-only, no YOLO, no folder chosen. The reply arrives, the guide says it worked, and a reload leaves it gone.
 if (run('reply')) {
  await both({}, async ({page, g, world, width, capture}) => {
   await g.getByRole('button', {name:'Ask Claude Code'}).click()
   await expect(g.getByText('Asking Claude Code. Waiting for its first reply.')).toBeVisible()
   expect(world.posts).toHaveLength(1)
   expect(world.posts[0]).toMatchObject({agentId:'a1', tool:'claude', structured:true, prompt:PROMPT})
   expect(world.posts[0].yolo).toBeUndefined()
   expect(world.posts[0].dir).toBeUndefined()
   await expect(g.getByRole('link', {name:'Open the session'})).toHaveAttribute('href', '/sessions?agent=a1&session=s1')
   await expect(g.getByRole('radio').first()).toBeDisabled()
   await reachable(page, width)
   await capture('guide-asking')
   // The same status twice says nothing new: an unchanged poll leaves the one sentence as it was.
   const status = g.getByRole('status')
   await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')))
   await expect(status).toHaveText('Asking Claude Code. Waiting for its first reply.')
   world.sessions = world.sessions.map(s => ({...s, attention:'waiting', changedSinceSeen:true}))
   await readAgain(page, () => expect(status).toHaveText('Claude Code replied. Nothing in your files was changed.', {timeout:1000}))
   await expect(g.getByRole('link', {name:'Read the reply'})).toBeVisible()
   await capture('guide-first-reply')
   await page.reload()
   await expect(page.getByRole('heading', {name:'Home', level:1})).toBeVisible()
   await expect(guide(page)).toHaveCount(0)
  })
 }

 // Done also puts it away, and a folder the person already has can be chosen.
 if (run('done')) {
  const project = {id:'fp1', host:'a1', machine:'studio', dir:'/home/dev/site', name:'site', isRepo:true, baseBranch:'main', worktreeDefault:'inherit', colour:'', defaultTool:'', defaultModel:'', defaultEffort:'', defaultYolo:false, hasStarterPrompt:false, note:'', position:0, createdAt:now, updatedAt:now, version:1}
  await both({projects:[project]}, async ({page, g, world, capture}) => {
   await g.getByLabel('Where it looks').selectOption('/home/dev/site')
   await capture('guide-folder')
   await g.getByRole('button', {name:'Ask Claude Code'}).click()
   await expect(g.getByRole('link', {name:'Open the session'})).toBeVisible()
   expect(world.posts[0].dir).toBe('/home/dev/site')
   expect(world.posts[0].projectId).toBeUndefined()
   world.sessions = world.sessions.map(s => ({...s, attention:'finished'}))
   await readAgain(page, () => expect(g.getByRole('button', {name:'Done'})).toBeVisible({timeout:1000}))
   await g.getByRole('button', {name:'Done'}).click()
   await expect(guide(page)).toHaveCount(0)
   await expect(start(page)).toBeFocused()
  })
 }

 // An agent that asks for trust shows the same checkbox as Start an agent, and Ask waits for it.
 if (run('trust')) {
  await both({tools:['opencode'], acpAgents:[descriptor('opencode', 'opencode')], accounts:[account('opencode')]}, async ({g, world, capture}) => {
   const ask = g.getByRole('button', {name:'Ask OpenCode'})
   await expect(g.getByLabel('I trust this folder; OpenCode plugins in .opencode may run.')).not.toBeChecked()
   await expect(ask).toBeDisabled()
   await capture('guide-trust')
   await g.getByLabel('I trust this folder; OpenCode plugins in .opencode may run.').check()
   await expect(ask).toBeEnabled()
   await ask.click()
   await expect(g.getByRole('link', {name:'Open the session'})).toBeVisible()
   expect(world.posts[0]).toMatchObject({tool:'opencode', structured:true, trust:true})
  })
 }

 // A session that ended before it answered says so and offers another go.
 if (run('ended')) {
  await both({}, async ({page, g, world, capture}) => {
   await g.getByRole('button', {name:'Ask Claude Code'}).click()
   await expect(g.getByRole('link', {name:'Open the session'})).toBeVisible()
   world.sessions = world.sessions.map(s => ({...s, state:'exited', exitCode:1}))
   await readAgain(page, () => expect(g.getByRole('status')).toHaveText('The session ended before Claude Code answered.', {timeout:1000}))
   await capture('guide-ended')
   await g.getByRole('button', {name:'Try again'}).click()
   await expect(g.getByRole('button', {name:'Ask Claude Code'})).toBeVisible()
  })
 }

 // A signed-out agent is a state, not an error: the existing sentence, another ready agent offered, and the guide moves on by itself.
 if (run('signed-out')) {
  await both({accounts:[account('claude', {state:'none', method:undefined, account:undefined, plan:undefined}), account('opencode')]}, async ({page, g, world, width, capture}) => {
   // The agent that is ready is the one asked first; choosing the signed-out one shows what to do.
   await expect(g.getByRole('radio', {name:/OpenCode/})).toBeChecked()
   await g.getByRole('radio', {name:/Claude Code/}).check()
   await expect(g.locator('label').filter({hasText:'Claude Code'})).toContainText('No Claude Code sign-in was found on studio. Sign in to Claude Code there first, or its first message may be refused.')
   await expect(g.getByRole('status')).toHaveText('Waiting for Claude Code to be signed in on studio. Asking the machine again every 15 seconds.')
   await expect(g.getByRole('button', {name:'Ask Claude Code'})).toHaveCount(0)
   await expect(g.getByRole('button', {name:'Use OpenCode instead'})).toBeVisible()
   await expect(page.getByRole('alert')).toHaveCount(0)
   await reachable(page, width)
   await capture('guide-signed-out')
   // The other agent takes the question.
   await g.getByRole('button', {name:'Use OpenCode instead'}).click()
   await expect(g.getByRole('button', {name:'Ask OpenCode'})).toBeEnabled()
   // The sign-in appears on the machine; the guide notices without being asked.
   await g.getByRole('radio', {name:/Claude Code/}).check()
   world.accounts = [account('claude'), account('opencode')]
   await expect(g.getByRole('button', {name:'Ask Claude Code'})).toBeEnabled({timeout:25_000})
   await expect(g.locator('label').filter({hasText:'Claude Code'})).toContainText('Signed in as sam@example.test')
   // Nothing is missing now, so the machine is not asked again: the accounts are read only when a person asks.
   let reads = 0
   await page.route('**/api/v2/machines/a1/accounts*', route => { reads++; return route.fallback() })
   await page.waitForTimeout(16_500)
   expect(reads, 'accounts read with nothing missing').toBe(0)
  })
 }

 // The only agent is signed out: Check again reads the machine at once and moves on when the sign-in is there.
 if (run('only-agent')) {
  await both({tools:['claude'], accounts:[account('claude', {state:'none'})]}, async ({g, world, capture}) => {
   await expect(g.getByRole('button', {name:'Use OpenCode instead'})).toHaveCount(0)
   await expect(g.getByRole('button', {name:'Check again'})).toBeVisible()
   await capture('guide-signed-out-only-agent')
   await g.getByRole('button', {name:'Check again'}).click()
   await expect(g.getByRole('button', {name:'Check again'})).toBeVisible()
   world.accounts = [account('claude')]
   await g.getByRole('button', {name:'Check again'}).click()
   await expect(g.getByRole('button', {name:'Ask Claude Code'})).toBeEnabled()
  })
 }

 // No agent installed: said once, with where to look next.
 if (run('no-agent')) {
  await both({tools:[], accounts:[]}, async ({g, capture}) => {
   await expect(g.getByText('No agent is installed on studio yet. Install Claude Code, Codex or OpenCode there; this looks again by itself.')).toBeVisible()
   await expect(g.getByRole('radio')).toHaveCount(0)
   await capture('guide-no-agent')
  })
 }

 // Not now sticks, and puts the focus on a control that is still there.
 if (run('dismiss')) {
  const {context, page} = await open({width:1280})
  await page.goto(base)
  await guide(page).getByRole('button', {name:'Not now'}).click()
  await expect(guide(page)).toHaveCount(0)
  await expect(start(page)).toBeFocused()
  await page.reload()
  await expect(page.getByRole('heading', {name:'Home', level:1})).toBeVisible()
  await expect(start(page)).toBeVisible()
  await expect(guide(page)).toHaveCount(0)
  await context.close()
 }

 // The phone step: where to open reet and how to keep it to hand, with the Notifications page one link away.
 if (run('phone')) {
  await both({}, async ({g, capture}) => {
   const summary = g.getByText('Use reet on your phone')
   // A disclosure shows its triangle like the others on Home, and is not styled as a link.
   expect(await summary.evaluate(el => getComputedStyle(el).display)).toBe('list-item')
   await summary.click()
   expect(await summary.evaluate(el => getComputedStyle(el).color)).not.toBe(await g.getByRole('link', {name:'Open Notifications'}).evaluate(el => getComputedStyle(el).color))
   await expect(g.getByText('Add to Home Screen')).toBeVisible()
   await expect(g.getByRole('link', {name:'Open Notifications'})).toHaveAttribute('href', '/notifications')
   await expect(g.getByText('A phone cannot open localhost.')).toBeVisible()
   await capture('guide-phone-step')
  })
 }

 // A machine that cannot take terminals yet shows its own readiness sentence, not agent rows.
 if (run('stuck')) {
  const routes = scene().routes
  routes['/api/v2/terminal/machines'] = {machines:[terminalMachine('a1', 'studio', false, false)]}
  for (const [width, theme] of [[1280, 'light'], [390, 'dark']]) {
   const {context, page} = await openWorkspace(browser, {width, height:width < 600 ? 844 : 800, theme, touch:width < 600, routes})
   await page.goto(base)
   await expect(guide(page).getByText('Enable terminal access locally on this machine.')).toBeVisible()
   await expect(guide(page).getByRole('radio')).toHaveCount(0)
   await shot(page, 'guide-machine-not-ready', guide(page))
   await context.close()
  }
 }
 console.log('First session: the guide shows once for a first machine, one line per ready agent, the read-only question and its reply, a signed-out agent and recovery, Not now, the phone step, and what stays reachable passed.')
} finally {await browser.close()}
