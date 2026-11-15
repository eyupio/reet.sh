import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { COMPOSE_MAX, composeText, appendDictation, speechRecognition, dictationProblem } = await import(`data:text/javascript;base64,${Buffer.from(compile('./compose.ts')).toString('base64')}`)

test('a draft is sent with one newline style and no trailing blank space', () => {
  assert.equal(composeText('fix the build  \r\nthen run the tests\t\n\n'), 'fix the build\nthen run the tests')
  assert.equal(composeText('  indented stays'), '  indented stays')
  assert.equal(composeText(' \n\t\n'), '')
  assert.equal(composeText('a'.repeat(COMPOSE_MAX + 50)).length, COMPOSE_MAX)
})

test('a dictated phrase joins what is written with one space, and keeps its own capitals', () => {
  assert.equal(appendDictation('', 'Refactor the store'), 'Refactor the store')
  assert.equal(appendDictation('Refactor the store', ' and run the tests. '), 'Refactor the store and run the tests.')
  assert.equal(appendDictation('First line\n', 'second line'), 'First line\nsecond line')
  assert.equal(appendDictation('kept', '   '), 'kept')
  assert.equal(appendDictation('a'.repeat(COMPOSE_MAX), 'more').length, COMPOSE_MAX)
})

test('speech recognition is found under either name, or not at all', () => {
  const standard = function () {}, prefixed = function () {}
  assert.equal(speechRecognition({ SpeechRecognition: standard, webkitSpeechRecognition: prefixed }), standard)
  assert.equal(speechRecognition({ webkitSpeechRecognition: prefixed }), prefixed)
  assert.equal(speechRecognition({}), undefined)
})

test('a dictation that stops says why only when there is something to do about it', () => {
  assert.match(dictationProblem('not-allowed'), /did not allow the microphone/)
  assert.match(dictationProblem('service-not-allowed'), /did not allow the microphone/)
  assert.match(dictationProblem('audio-capture'), /No microphone/)
  assert.match(dictationProblem('network'), /speech service/)
  assert.equal(dictationProblem('no-speech'), '')
  assert.equal(dictationProblem('aborted'), '')
  assert.match(dictationProblem('something-new'), /type instead/)
})
