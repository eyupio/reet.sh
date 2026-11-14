import { effortChoices } from '../models'
import { structuredAgent, type Capabilities } from './api'

/** The levels a chat may pick when its descriptor has no thought-level list. A descriptor is
 * measured against its default model and may omit a level the live session reports, so its
 * silence is not proof that the agent has none; the agent still confirms the choice before
 * any prompt. These are the start levels the connector already maps, not a new vocabulary. */
const START_EFFORTS = ['low', 'medium', 'high']

/** Launch suggestions are installation evidence. The running agent must still
 * confirm that the chosen model accepts an effort before any prompt is sent.
 * The values are the agent's own thinking levels, read from its descriptor, not a list of ours.
 * A descriptor that names no thought level falls back to the start levels, because a live
 * session can still report them; the machine's start-options capability decides whether an
 * effort can be carried at all, and the agent refuses one it does not offer. */
export function launchEfforts(tool: string, caps: Capabilities | undefined, chat: boolean): string[] {
 if (!chat) return effortChoices(tool)
 const agent = structuredAgent(caps, tool)
 if (!caps?.acpStartOptions || !agent) return ['']
 const selector = agent.options?.find(option => option.category === 'thought_level')
 if (selector) return ['', ...new Set(selector.values.filter(value => value && value !== 'default'))]
 return ['', ...START_EFFORTS]
}

export function effortHelp(tool: string, caps: Capabilities | undefined, chat: boolean): string {
 if (!chat) return effortChoices(tool).length > 1 ? 'Use the agent default, or choose its thinking effort.' : 'This agent’s terminal interface uses its own effort.'
 if (!caps?.acpStartOptions) return 'Update reet on this machine to choose Gaffer effort before starting.'
 if (launchEfforts(tool, caps, true).length === 1) return 'This agent reports effort choices after it starts. Agent default is used here.'
 return 'The agent confirms this effort for the chosen model before it receives your opening prompt.'
}
