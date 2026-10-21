export type QuickAction = 'like' | 'save' | 'used'

/** A javascript: URL that sends the skills.sh page being viewed to this instance. */
export function bookmarklet(action: QuickAction, origin: string, token: string): string {
  const t = token ? `&t=${encodeURIComponent(token)}` : ''
  const base = `${origin}/quick?action=${action}${t}&url=`
  return `javascript:(function(){window.open(${JSON.stringify(base)}+encodeURIComponent(location.href),'reetq','width=360,height=160')})()`
}
