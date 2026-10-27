// A skill card leads with the action that changes a machine when nothing has the skill yet, and with its next shelf step once something does.
import { chromium, expect } from '@playwright/test'
import { openWorkspace, skill, machineWith } from './helpers/workspace.mjs'
const base = process.env.REET_UI_URL || 'http://127.0.0.1:4173'
const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/chromium', args:['--no-sandbox']})
// REET_E2E_ONLY=<section> runs one section, so each can be watched failing before its fix.
const only = process.env.REET_E2E_ONLY
const run = name => !only || only === name
const open = options => openWorkspace(browser, options)

try {
 // Installed nowhere: Copy install leads, the shelf step is secondary, and the prompt waits behind Expand.
 if (run('not-installed')) {
  const {context,page,errors} = await open({width:1440,skills:[skill('code-review')]})
  await page.goto(`${base}/skills`)
  await expect(page.getByRole('button',{name:'Save to test'})).toBeVisible()
  await expect(page.getByRole('button',{name:'Copy install command for code-review'})).toHaveClass(/primary/)
  await expect(page.getByRole('button',{name:'Save to test'})).not.toHaveClass(/move/)
  await expect(page.getByRole('button',{name:'Copy prompt for code-review'})).toBeHidden()
  await page.getByRole('button',{name:/^Expand/}).first().click()
  await expect(page.getByRole('button',{name:'Copy prompt for code-review'})).toBeVisible()
  expect(errors).toEqual([])
  await context.close()
 }

 // Installed on a machine: the shelf step leads, Copy install is an ordinary button.
 if (run('installed')) {
  const mine = skill('code-review')
  const {context,page} = await open({width:1440,skills:[mine],inventory:machineWith('dev-laptop',[mine])})
  await page.goto(`${base}/skills`)
  await expect(page.getByText('dev-laptop',{exact:true}).first()).toBeVisible()
  await expect(page.getByRole('button',{name:'Save to test'})).toHaveClass(/move/)
  await expect(page.getByRole('button',{name:'Copy install command for code-review'})).not.toHaveClass(/primary/)
  await context.close()
 }
 console.log('Skill card: Copy install leads when installed nowhere, the shelf step leads once installed, the prompt waits behind Expand.')
} finally {await browser.close()}
