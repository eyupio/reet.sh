import type { PageGroup } from './tabs'

/** The context key App.svelte uses to share the current page's group with every routed page's header. */
export const PAGE_GROUP = Symbol('page-group')

/** What App.svelte puts in context. `group` is a getter, so a reader sees the current value as the More list changes. */
export type PageGroupContext = { readonly group: PageGroup | null }
