# Agent sign-ins and plan limits

A machine can say which of its AI agents are signed in, whose account it is and which plan it is on, and how much of
each plan's allowance is left: the session window, the week or the month, and any window a plan keeps for part of
itself (one model, other models, Copilot's premium requests). This is read for every account; there is no switch. Both
are read on the machine. No credential reaches the reet controller: an answer is a state, a name, a plan word and whole
percentages.

Claude Code, Codex, OpenCode (an OpenCode Go plan), Google Antigravity and GitHub Copilot are read, by a machine
running a reet that reads them; a machine with an older reet reads Claude Code and Codex only, and the controller shows
whatever each machine reports. Every other agent has no plan figure: reet cannot tell, and does not guess. The
[Usage page](usage.md) lists every agent and says, for each one with no figure, why.

## What you see

- **Machines → a machine's ⋮ → Settings… → Agents.** A **Sign-ins** row says when the machine last read them, with
  **Check sign-ins** to ask it now. Each agent it can read then says *Signed in as person@example.com · Max plan*,
  *Signed in with an API key*, *No sign-in found on this machine*, or that reet can't read the sign-in. A signed-in plan lists its windows as what is left (*Session 81% left · resets in 2 h 10 min*), marked when
  15% or less is left.
- **Start an agent.** If the machine looked and found no sign-in for the chosen agent, the dialog says so before you
  start, because the agent would refuse its first message. It is advice: nothing is disabled, and a machine that has
  not answered yet shows nothing.
- **Plan meters**, wherever the work is. Each account is a chip: the agent's mark, the share
  left of the window with the least left (the one that stops the agent first) and a hairline meter under it. A chip
  turns to the warning colour at 15% or less; figures kept from a read that has since failed are dimmed and never
  alarm. A mouse over the chips, or keyboard focus on them, lists every account's windows; a click or a tap opens
  **Plan limits**: per account its agent, plan and address, then each window with when it resets and a meter, which
  machine read it and how long ago, and why if the latest read failed, with **See all usage**, which opens the
  [Usage page](usage.md). Escape or a click elsewhere closes it.
  - **Home**, in the page head beside the counts, and the **Sessions** toolbar: every account across your machines.
    The same account signed in on two machines is one allowance, so it is one chip, from the machine that read it
    last. More than fit fold into **+N**; the panel lists them all.
  - **A chat's status strip**, beside the context left: the plan of that chat's agent.
  - **A Factory pipeline's detail**, as **Plan left** among the stage's facts: the plan of the agent doing the stage
    in hand, with its windows in words, until the pipeline is done.
- **Usage** (More → Usage): every agent, each account's plan left, and the tokens its sessions reported. See
  [Usage](usage.md).
- **Settings → Agents → Usage.** A sentence on what is read, and a link to the Usage page. There is no switch.

These show the figures each machine last read, which the controller keeps; they look again every two minutes. A look
that finds the kept figures more than five minutes old has the controller ask an online machine to read once, as any
read of them does (see [Controller cache and API](#controller-cache-and-api)), so a page left open has each machine
read about every six minutes. Only **Check sign-ins** asks for figures newer than the machine's own five-minute
memory. Only machines that read sign-ins are asked. An agent with no plan to read (an API key,
no sign-in, or an agent reet does not read) has no chip. The marks are reet's own, an initial on a tile, not the
vendors' logos.

## What a machine reads

Sign-in, from the agent's own files, from the keychain or keyring where its CLI keeps one (read with the platform's
own read-only tool), and from the GitHub CLI's own token for Copilot. Nothing is sent anywhere and no agent is
started; the only commands run are those read-only tools and `gh auth`.

| Agent | Sign-in | Account and plan |
| --- | --- | --- |
| Claude Code | `.credentials.json` in `~/.claude`, or in the folder `CLAUDE_CONFIG_DIR` names; on macOS also the login keychain's `Claude Code-credentials` item, the later token winning | `oauthAccount` in `~/.claude.json` (inside the folder when it was moved); the plan is the sign-in's `subscriptionType` |
| Codex | `auth.json` in `~/.codex`, or in the folder `CODEX_HOME` names; on macOS also the login keychain's `Codex Auth` item when the file holds none, the later token winning | the address and plan claims of the identity token; an `OPENAI_API_KEY` there is a sign-in with no plan |
| OpenCode | its key for OpenCode's own models (`opencode-go`, else `opencode`), from the first of: OpenCode 2's store `opencode.db` in `$XDG_DATA_HOME/opencode` or `~/.local/share/opencode` (or the file `OPENCODE_DB` names), opened read-only and only when it has OpenCode 2's tables; `auth.json` in that folder, not read when that store is there; `OPENCODE_API_KEY` in the environment reet's connector runs with | no name: the account shows as *Signed in*, on its machine; the plan word comes from the provider's answer (*Go*) |
| Google Antigravity | `antigravity-oauth-token` in `~/.gemini/antigravity-cli`, or in the folder `ANTIGRAVITY_APP_DATA_DIR` names, plain or in its keyring library's `go-keyring-base64:` form; also the Linux Secret Service item `service gemini, username antigravity` through `secret-tool`, or the macOS login keychain's `gemini`/`antigravity` item, the later token winning | the address claim of the identity token; no plan word |
| GitHub Copilot | `apps.json` and `hosts.json` in `$XDG_CONFIG_HOME/github-copilot` or `~/.config/github-copilot`, entries for `github.com` only (a key named `github.com` or `github.com:` and an app id; another host's key, even one whose name starts the same way, is never read); and the GitHub CLI's own sign-in for `github.com`, through `gh auth status` or, on an older CLI, `gh auth token`: its token for the login those files name, or for every account it holds when they name none; the tokens are tried in turn | the login; the plan word comes from the provider's answer (*Free*, or the Copilot plan) |

What this cannot see is said plainly rather than guessed:

- On macOS Claude Code and Codex keep their sign-in in the login keychain, which reet reads through `security`. A
  keychain item this session cannot read (a locked keychain) reads as *reet can't read this agent's sign-in* for
  Claude, never as signed out.
- On Linux Google Antigravity keeps its sign-in in the desktop keyring, which reet reads through `secret-tool`. Where
  `secret-tool` is not installed, a machine with no sign-in file reads as *No sign-in found*.
- GitHub Copilot can be used with only the GitHub CLI's sign-in, which reet reads through `gh`. Where `gh` is not
  installed, only the files above are read.
- An agent signed in only through an environment variable is not seen, except OpenCode's `OPENCODE_API_KEY` when it is
  set for reet's connector itself.
- A folder variable that is not an absolute path, a file that is not a plain file, one over 256 KiB or one that does
  not parse reads as unreadable; so does an OpenCode store that cannot be opened or holds a key that does not parse.

Plan limits, for every account. The machine asks the agent's own provider, with the agent's own
sign-in, from the machine:

| Agent | Asked | Windows |
| --- | --- | --- |
| Claude Code | `GET https://api.anthropic.com/api/oauth/usage` | five hours (session), seven days (week), and the week's share for Opus and for Sonnet when the plan limits them separately |
| Codex | `GET https://chatgpt.com/backend-api/wham/usage` | the plan's one or two windows, named by their length: up to six hours a session, up to a day and a half a day, up to eight days a week, longer a month |
| OpenCode | `GET https://opencode.ai/zen/go/v1/usage`, with the key as a bearer token | the Go plan's rolling (session), weekly and monthly windows. A key with no Go plan is answered 403, shown as *no plan with limits to read*, not as an error |
| Google Antigravity | `POST https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary` with `{}`; when that fails (no answer, a server error or not found), once `https://cloudcode-pa.googleapis.com/…` instead. A refused sign-in or a rate limit is not tried again elsewhere | five hours (session) and the week for the plan's own models, and the same two labelled *Other models*. An answer with no buckets is *no plan with limits to read* |
| GitHub Copilot | `GET https://api.github.com/copilot_internal/user`, with the token as `Authorization: token …` | the month's premium requests, chats and completions (*Premium*, *Chat*, *Completions*); a window without a limit is left out. A free plan's are worked out from its monthly quotas and what is left of them. An account without Copilot (404 for every token) or with no limited window is *no plan with limits to read* |

These are the providers' own internal endpoints, the ones the agents' own clients use for the same figures. They are
not published interfaces and can change or close without notice; when an answer is not in the shape above the machine
reports a failure and keeps the last figures it read. The Copilot request presents itself as an editor's Copilot Chat
(its `User-Agent`, `Editor-Version`, `Editor-Plugin-Version` and `X-GitHub-Api-Version` headers), because the endpoint
answers nothing else. The Antigravity request likewise names itself as the agent's own client (`User-Agent:
antigravity`). A token is only ever sent to its own provider, and reet never refreshes one.

## The rules the machine keeps

- **The sign-in stays on the machine.** It is put in the `Authorization` header of that one request and nowhere else:
  it is never returned, logged or put in an error, and the protocol's answer has no field it could travel in.
- **A sign-in is never renewed or rewritten.** Agents rotate their renewal tokens, so renewing one from outside can
  sign the agent out. A token that has run out is reported as needing renewal, which the agent does itself the next
  time it runs: start it on the machine once. Nothing is sent with a token that expires within 30 seconds.
- **No redirect is followed**, so a sign-in is never sent on to another host.
- **Providers are asked sparingly.** Figures answer for five minutes; a provider is asked at most once in 30 seconds
  for an agent, a pressed **Check sign-ins** included; a failure stands for a minute before an unprompted retry, and a
  rate limit for as long as the provider's `Retry-After` says, up to an hour. An answer that the account has no plan to
  read stands for five minutes too. Everything one agent's figures take (Antigravity's second endpoint, Copilot's
  further tokens included) times out after 9 seconds, and answers are capped at 256 KiB. Reading OpenCode's own
  store is given a second, so a whole read stays inside the 12 seconds the machine waits for it.
- **Another account never inherits figures.** If the signed-in account changes, the remembered figures are dropped.
- **One read at a time** per machine; a second request is told the machine is busy.

## Gates

- **Terminal access.** A machine reports the capability, and answers a request, only while terminal access is on
  there, and it reads the switch again for each request. With access off the page says so and nothing is read.
- **No account setting.** Plan limits are read for every account. Earlier versions kept them off until an account
  turned them on in **Settings → Agents**; that switch is gone, and an older browser tab that still sends it changes
  nothing. There is no switch on the machine either: terminal access is the machine's gate.
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

A read serves the kept answer for five minutes. An older one, none, or one kept before plan limits were read for every
account (it says `not_asked` for a signed-in plan) has an online machine asked once, synchronously (14 seconds at most). A machine that cannot be asked answers with
what was kept and a `state` saying why it is not newer: `offline`, `update` (an older reet there), `off` (terminal
access), `busy` or `failed`. The response is `{state, sentence, planLimits, checkedAt, accounts}`; `planLimits` is always `true`, kept for older
browser tabs.

A refresh is limited to 6 a minute per account and machine and writes the `machine-accounts-read` audit row, which
names the machine, the outcome and that limits were asked for, never an account or plan.

## What is here, and what is next

Checked with synthetic sign-in files and a stand-in provider only. Nothing was run against a real account, so the
providers' present answers were not observed: the request and field names follow the agents' own clients. The Claude
Code and Codex endpoints were seen to exist: asked with no sign-in, one answered 401 and the other a rate limit,
neither not-found. The OpenCode, Antigravity and Copilot readers have not been tried against their providers or a real
install of those agents at all.

`unsupported` means an API key, which has no plan, or a plan sign-in whose provider said it has no plan with limits
to read (an OpenCode key without Go, a GitHub account without Copilot, a plan with every window unlimited).

Not done: Cline; several accounts for one agent; signing in from the browser; limits beside a session or in the
sessions list.
