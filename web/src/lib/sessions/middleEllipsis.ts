// A long path is most useful at both ends: the root says where it lives, the tail says which one it is.
// Cutting the middle keeps the folder name a person looks for, which CSS's end-of-line ellipsis would lose.
export function middleEllipsis(text: string, max = 40): string {
  const chars = Array.from(text)
  if (max < 3 || chars.length <= max) return text
  const keep = max - 1
  // The tail gets the larger share: the last segment is what tells two worktrees apart.
  const tail = Math.ceil(keep * 0.6), head = keep - tail
  return `${chars.slice(0, head).join('')}…${chars.slice(chars.length - tail).join('')}`
}
