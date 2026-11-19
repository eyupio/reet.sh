import type { SkillQuery } from '../api'

// What the Skills page shows, as Library tools' Markdown and CSV export asks for it: the tab, the search, Liked and the
// tag. The page keeps it current; the app hands it to the tools under the page. The page's list filter and its other
// orders have no counterpart in the export, which keeps skills in name order.
export const skillsExport = $state<SkillQuery>({ tab: 'all', q: '', sort: 'name', stage: 'active', tags: [] })
