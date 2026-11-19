export type QuickAction = 'like' | 'save' | 'used'

/** A javascript: URL that opens this instance's /quick page for the skills.sh page being viewed; that page asks before it adds. */
export function bookmarklet(action: QuickAction, origin: string, token: string): string {
  const t = token ? `&t=${encodeURIComponent(token)}` : ''
  const base = `${origin}/quick?action=${action}${t}&url=`
  return `javascript:(function(){window.open(${JSON.stringify(base)}+encodeURIComponent(location.href),'reetq','width=420,height=400')})()`
}
