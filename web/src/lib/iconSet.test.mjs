import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join, relative } from 'node:path'
import { test } from 'node:test'
import assert from 'node:assert/strict'

// A dialog's close control is a similar action wherever it appears, so it carries the one shared close icon rather than a
// text multiplication sign that inherits whatever font the button happens to use.
const CLOSE = /<button\b[^>]*\bdata-shell-close\b[^>]*>([\s\S]*?)<\/button>/g
const src = fileURLToPath(new URL('../', import.meta.url))

const files = (dir, found = []) => {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) files(path, found)
    else if (entry.name.endsWith('.svelte')) found.push(path)
  }
  return found
}

export function textCloseControls(text) {
  return [...text.matchAll(CLOSE)].filter(match => match[1].includes('×')).map(match => match[0])
}

test('a text multiplication sign in a close control is found, with its whole button', () => {
  const button = '<button type="button" data-shell-close aria-label="Close">Close ×</button>'
  assert.deepEqual(textCloseControls(button), [button])
  assert.deepEqual(textCloseControls('<button type="button" data-shell-close aria-label="Close"><Icon name="x" /></button>'), [])
  assert.deepEqual(textCloseControls('<button type="button" aria-label="Remove"><span aria-hidden="true">×</span></button>'), [])
})

test('every dialog close control uses the shared close icon', () => {
  const offences = []
  for (const file of files(src)) {
    for (const button of textCloseControls(readFileSync(file, 'utf8'))) {
      offences.push(`${relative(src, file)}: ${button.replace(/\s+/g, ' ').slice(0, 120)}`)
    }
  }
  assert.deepEqual(offences, [])
})
