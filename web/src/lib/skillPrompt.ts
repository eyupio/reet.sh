/** Insert a skill's ready-to-run prompt at the caret, keeping everything the person already wrote. The inserted text is
 *  ordinary prompt text, so it stays editable like anything else in the box. */
export function insertSkillPrompt(prompt: string, insert: string, start = prompt.length, end = start): { text: string; caret: number } {
  const size = prompt.length
  const at = Math.min(Math.max(0, Number.isFinite(start) ? Math.floor(start) : size), size)
  const to = Math.min(Math.max(at, Number.isFinite(end) ? Math.floor(end) : at), size)
  const before = prompt.slice(0, at)
  const after = prompt.slice(to)
  // A blank line separates the skill prompt from what is already there, but never doubles an existing break.
  const lead = before === '' || /\n[ \t]*\n[ \t]*$/.test(before) ? '' : before.endsWith('\n') ? '\n' : '\n\n'
  const trail = after === '' || /^[ \t]*\n[ \t]*\n/.test(after) ? '' : after.startsWith('\n') ? '\n' : '\n\n'
  return { text: before + lead + insert + trail + after, caret: (before + lead + insert).length }
}
