import { effortChoices } from '../models'
import { structuredAgent, type Capabilities } from './api'

/** What a start can carry: the controller's Create.Effort takes only these three. */
const START_EFFORTS = ['low', 'medium', 'high']

/** Launch suggestions are installation evidence. The running agent must still
 * confirm that the chosen model accepts an effort before any prompt is sent.
 * A chat start offers only the values a start can carry; the live control offers every value the session reports. */
export function launchEfforts(tool: string, caps: Capabilities | undefined, chat: boolean): string[] {
 if (!chat) return effortChoices(tool)
 if (!caps?.acpStartOptions) return ['']
 const selector = structuredAgent(caps, tool)?.options?.find(option => option.category === 'thought_level')
 return ['', ...new Set(selector?.values.filter(value => START_EFFORTS.includes(value)) ?? [])]
}

export function effortHelp(tool: string, caps: Capabilities | undefined, chat: boolean): string {
 if (!chat) return effortChoices(tool).length > 1 ? 'Use the agent default, or choose its thinking effort.' : 'This agent’s terminal interface uses its own effort.'
 if (!caps?.acpStartOptions) return 'Update reet on this machine to choose Gaffer effort before starting.'
 if (launchEfforts(tool, caps, true).length === 1) return 'This agent reports effort choices after it starts. Agent default is used here.'
 return 'The agent confirms this effort for the chosen model before it receives your opening prompt.'
}
