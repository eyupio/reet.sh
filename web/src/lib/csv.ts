// CSV the browser writes, by the controller's rule for its own export (internal/api/api.go csvCell).

/**
 * One CSV cell. Text a spreadsheet would read as a formula, one that starts with =, +, -, @, a tab or a carriage return,
 * gets a quote in front (CSV injection), since names come from other people's skills and lists. A cell with a quote, a
 * comma or a line break is then quoted. A number is written as it is: it is never a formula.
 */
export function csvCell(value: string | number): string {
  let text = String(value)
  if (typeof value === 'string' && /^[=+\-@\t\r]/.test(text)) text = `'${text}`
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text
}

/** Rows of cells as CSV text: cells joined by commas, rows by newlines. */
export function csvText(rows: (string | number)[][]): string {
  return rows.map(row => row.map(csvCell).join(',')).join('\n')
}
