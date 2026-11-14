# Agent sign-ins and plan limits

A machine can say which of its AI agents are signed in, whose account it is and which plan it is on. When your account
turns **plan limits** on, the machine also reads how much of each plan's allowance is left: the session window, the
week, and any window a plan keeps for one model. Both are read on the machine. No credential reaches the reet
controller: an answer is a state, a name, a plan word and whole percentages.

Claude Code and Codex are read today. Every other agent shows nothing here: reet cannot tell, and does not guess.

## What you see

- **Machines → a machine's ⋮ → Settings… → Agents.** A **Sign-ins** row says when the machine last read them, with
  **Check sign-ins** to ask it now. Each agent it can read then says *Signed in as person@example.com · Max plan*,
  *Signed in with an API key*, *No sign-in found on this machine*, or that reet can't read the sign-in. With plan
  limits on, a signed-in plan lists its windows as what is left (*Session 81% left · resets in 2 h 10 min*), marked when
  15% or less is left.
- **Start an agent.** If the machine looked and found no sign-in for the chosen agent, the dialog says so before you
  start, because the agent would refuse its first message. It is advice: nothing is disabled, and a machine that has
  not answered yet shows nothing.
- **Settings → Agents → Plan limits.** The switch for your account, off until you turn it on.

Figures are the provider's, passed on as numbers; reet attaches no price. When the latest read failed, the last figures
stay, with why and how old they are.

## What a machine reads

Sign-in, from files only. Nothing is sent anywhere, no agent is started and no command is run.

| Agent | Sign-in | Account and plan |
| --- | --- | --- |
| Claude Code | `.credentials.json` in `~/.claude`, or in the folder `CLAUDE_CONFIG_DIR` names | `oauthAccount` in `~/.claude.json` (inside the folder when it was moved); the plan is the sign-in's `subscriptionType` |
| Codex | `auth.json` in `~/.codex`, or in the folder `CODEX_HOME` names | the address and plan claims of the identity token; an `OPENAI_API_KEY` there is a sign-in with no plan |

What this cannot see is said plainly rather than guessed:

- On macOS Claude Code keeps its sign-in in the keychain, which reet does not open. A Mac with no sign-in file reads
  as *reet can't read this agent's sign-in*, never as signed out.
- Codex can be set to keep its sign-in in the system keyring. Such a machine reads as *No sign-in found*: the file was
  looked for and is not there.
- An agent signed in only through an environment variable is not seen.
- A folder variable that is not an absolute path, a file that is not a plain file, one over 256 KiB or one that does
  not parse reads as unreadable.

Plan limits, only when your account has them on. The machine asks the agent's own provider, with the agent's own
sign-in, from the machine:

| Agent | Asked | Windows |
| --- | --- | --- |
| Claude Code | `GET https://api.anthropic.com/api/oauth/usage` | five hours (session), seven days (week), and the week's share for Opus and for Sonnet when the plan limits them separately |
| Codex | `GET https://chatgpt.com/backend-api/wham/usage` | the plan's one or two windows, named by their length: up to six hours a session, up to a day and a half a day, up to eight days a week, longer a month |

These are the endpoints the agents' own clients use for the same figures. They are not published interfaces, so a
provider may change or close them; when an answer is not in the shape above the machine reports a failure and keeps the
last figures it read.

## The rules the machine keeps

- **The sign-in stays on the machine.** It is put in the `Authorization` header of that one request and nowhere else:
  it is never returned, logged or put in an error, and the protocol's answer has no field it could travel in.
- **A sign-in is never renewed or rewritten.** Agents rotate their renewal tokens, so renewing one from outside can
  sign the agent out. A token that has run out is reported as needing renewal, which the agent does itself the next
  time it runs: start it on the machine once. Nothing is sent with a token that expires within 30 seconds.
- **No redirect is followed**, so a sign-in is never sent on to another host.
- **Providers are asked sparingly.** Figures answer for five minutes; a provider is asked at most once in 30 seconds
  for an agent, a pressed **Check sign-ins** included; a failure stands for a minute before an unprompted retry, and a
  rate limit for as long as the provider's `Retry-After` says, up to an hour. Requests time out after 10 seconds and
  answers are capped at 256 KiB.
- **Another account never inherits figures.** If the signed-in account changes, the remembered figures are dropped.
- **One read at a time** per machine; a second request is told the machine is busy.

## Gates

- **Terminal access.** A machine reports the capability, and answers a request, only while terminal access is on
  there, and it reads the switch again for each request. With access off the page says so and nothing is read.
- **Your account's setting.** Plan limits are off for every account until it turns them on in **Settings → Agents**.
  With them off the controller asks machines for sign-ins only, and figures kept from when they were on are no longer
  shown. There is no extra switch on the machine.
- **Who can ask.** The routes sit behind the same account sign-in and role as the other terminal routes; another
  account's machine answers as an offline one and shows nothing.

## Capability, request and answer

`agent-accounts-v1` is advertised with the other private control features. A connector reports `agentAccounts` only
when the feature was negotiated, terminal control is available and terminal access is on. The request is
`Accounts{limits, refresh}` (`refresh` asks for figures newer than the machine's five-minute memory and is valid only
with `limits`); the reply is `AccountsResult{accounts, reason}` with one entry per agent the machine can read:

```
{tool, state: signed_in | none | unreadable, method: subscription | api_key, account, plan,
 limitsState: read | not_asked | unsupported | renew | refused | rate_limited | failed,
 limits: [{kind: session | day | week | month, label, usedPercent, resetsAt}], limitsAt}
```

`reason` is `off`, `conflict` or `timed_out` and then carries no accounts. The decoder refuses unknown fields, more
than 16 agents, an agent twice, a name with control characters, a percentage outside 0–100 and an answer over 8 KiB.
An answer that arrives after its request gave up is counted and dropped; it does not end the connection.

## Controller cache and API

The controller keeps the last answer per account and machine in `machine_agent_accounts`, replaced whole by each new
answer, so an account that signed out never keeps a name from the answer before. Forgetting a machine or revoking its
enrollment deletes the row. It holds no credential.

```
GET  /api/v2/machines/{agentId}/accounts            # the kept answer while recent, else one read of the machine
POST /api/v2/machines/{agentId}/accounts/refresh    # a person asking now
```

A read serves the kept answer for five minutes. An older one, none, or one made before the account turned plan limits
on has an online machine asked once, synchronously (14 seconds at most). A machine that cannot be asked answers with
what was kept and a `state` saying why it is not newer: `offline`, `update` (an older reet there), `off` (terminal
access), `busy` or `failed`. The response is `{state, sentence, planLimits, checkedAt, accounts}`.

A refresh is limited to 6 a minute per account and machine and writes the `machine-accounts-read` audit row, which
names the machine, the outcome and whether limits were asked for, never an account or plan.

## What is here, and what is next

Checked with synthetic sign-in files and a stand-in provider only. Nothing was run against a real account, so the
providers' present answers were not observed: the request and field names follow the agents' own clients. Both
endpoints were seen to exist: asked with no sign-in, one answered 401 and the other a rate limit, neither not-found.
Not done: GitHub Copilot, Google
Antigravity, OpenCode and Cline; the macOS keychain and system keyrings; several accounts for one agent; signing in
from the browser; limits beside a session or in the sessions list.
