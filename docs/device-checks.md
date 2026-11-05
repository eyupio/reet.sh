# Checks that need a real device or a real login

Two gates cannot be closed from CI or from a sandbox. This page says exactly what to do and what to write down. Do not turn on typed questions
(`elicitation.form`) or Claude chats (`REET_EXPERIMENTAL_CLAUDE_CHAT`) for anyone until the matching section below has been run and its results recorded
in `docs/structured-chat-plan.md` and the PR that records them.

## Recorded results

The signed-out half of both login gates and the connector-level Claude start are recorded; the signed-in runs and the phone and
reader checks are not. Typed questions (`elicitation.form`), Claude chats (`REET_EXPERIMENTAL_CLAUDE_CHAT`) and Codex chats
(`REET_EXPERIMENTAL_CODEX_CHAT`) therefore stay off.

| Gate | Evidence to record | State |
| --- | --- | --- |
| 1. Typed questions on a phone and with a screen reader | device and OS, browser and reader versions, each item pass or fail | Not run |
| 2. Claude: real login and no login | Runs A and B output, `authenticatedHint`, the logged-out error text, Run C | Run A and Run C recorded 4 November; Run B (signed in) not run |
| 3. Codex sign-in | the signed-in Run B result or its "Authentication required" answer, `authStatusUpdates`, then Run A | Run A recorded 4 November; Run B (signed in) deferred by the owner on 2 November, not run |

Record each result here with the date and the pull request that carries it, and in `docs/structured-chat-plan.md` beside the
slice it unblocks. A pass recorded anywhere else does not remove a gate.

### Recorded on 5 November 2026

- **Cline, real signed-in CLI, `cline` 3.0.74:** `REET_CLINE_ACP=…/cline CLINE_DIR=…/.cline go test ./internal/acpagent
  -run TestRealClineStart` passes — the version floor, the reported identity, `act` mode, a boolean `auto_approve`
  of `false`, a confirmed plan/act switch and a confirmed model switch, with no prompt and therefore no model call.
  Two prompts with every permission rejected recorded the read, editor and command cards, and a stored conversation
  replayed through `session/load`; the frames are in
  [the Cline ACP probe](https://github.com/eyupio/reet.sh/blob/main/docs/research/cline-acp-probe.md). Cline is not
  behind a switch.

### Recorded on 4 November 2026

The probe machine is signed in to neither agent (`~/.claude` and `~/.codex` absent, no credential variables set), so these runs
are the logged-out behaviour of the pinned adapters on Linux with Node 24.

- **Claude, Run A (logged out), `@agentclientprotocol/claude-agent-acp@0.99.1`:** `initialize` advertises **no** authentication
  methods, `session/new` succeeds, and the first prompt fails in 0.1 s with `-32000 Authentication required`
  (`replyIsOk: false`, `authenticatedHint: false`, no tools requested). A missing login therefore appears only at the prompt, which
  is where the browser now names the sign-in step.
- **Claude, Run C (connector profile, no model call):** `REET_CLAUDE_ACP=…/claude-agent-acp go test ./internal/acpagent -run
  TestRealClaudeStart` passes — version floor `0.99.1`, adapter identity, the required `default` mode, and process retirement.
- **Codex, Run A (signed out), `@agentclientprotocol/codex-acp@1.13.1` with `@openai/codex@0.156.1`:** `initialize` advertises
  `api-key` and `chat-gpt`; `session/new` before any authenticate fails `-32000 Authentication required`; `_auth/status_update` is
  `{"kind": "none", "label": "Not logged in"}`. This is the refusal the profile already reports as "Sign in to Codex on that
  machine first."


## 1. Typed questions on a phone and with a screen reader

**What is already automated.** `web/tests/question-card.e2e.mjs` drives the real chat pane on a fixture page. It checks the accessibility tree
(one group named by the question, labelled radios, checkboxes and text boxes, and the Cancel question, Skip and Send answer buttons), a keyboard answer, the
"Choose one." explanation, and, on an emulated 390 px touch phone, no sideways scroll, every control on screen and every control at least 40 px tall. That
is Chromium emulation. It does not tell you what a screen reader says or how it feels under a thumb.

**Start the fixture.** From `web/`, with Node 22 and dependencies installed (`npm ci`):

```
npm run fixture:questions
```

It prints addresses such as `http://192.168.1.20:5180/tests/fixtures/question-card.html`. Open one on the phone (same Wi-Fi). It serves a test page, not the
app: nothing is sent to a real server, the answers you send are only listed under the chat. Anyone on the network can open it while it runs; stop it with Ctrl+C.
Add `?theme=light` for the light theme and `?tool=claude` for Claude Code wording.

**On the page.** Tap **Start a turn**, then **Ask: choose one**, **Ask: choose several** or **Ask: long text**. Tap **Agent saw the answer** to decide a question and
**Time the question out** to close one with its reason. One question shows at a time.

**Touch (iPhone Safari and an Android phone with Chrome, portrait and landscape).** Write down pass or fail for each:

1. The card, its buttons and the text box fit the screen. Nothing needs sideways scrolling, and nothing is hidden behind the keyboard when the text box is focused.
2. Each radio, checkbox and button can be pressed on the first try with a thumb, without hitting its neighbour (Skip next to Send answer in particular).
3. Pressing **Send answer** with nothing chosen shows "Choose one." and moves attention to the field; choosing more than the limit shows "Choose at most 2.".
4. After answering, the card shows the decision ("Answered.") and its controls are gone. After **Time the question out** it says the question timed out.
5. Rotating the phone with a question open keeps what you had chosen.
6. Pinch zoom to 200% keeps every control reachable.

**Screen reader (VoiceOver on iPhone Safari, TalkBack on Android Chrome, and one desktop reader such as NVDA or VoiceOver on macOS).**

1. When a question appears, is it announced, or is it silent until you move to it? Write down which. (It is a live region only if the reader says so.)
2. Moving through the card, you hear the question first, then each field with its label and whether it is required, then the three buttons by name.
3. For a radio group you hear the group name and "1 of N"; for checkboxes you hear checked or not checked.
4. A validation message ("Choose one.") is read when it appears, or when you move to the field.
5. After sending, you hear that the question was answered, not nothing.
6. **Cancel question** and **Skip** are clearly different by name alone.

**Record.** Device and OS version, browser and version, reader and version, and each item as pass, fail or not applicable with one line on any failure. Add them
to `docs/structured-chat-plan.md` under SC3 and fix what failed before enabling typed questions.

## 2. Claude: real login and no login

**Why it is needed.** The sandbox the Claude work was done in supplies a login outside the process, so "no login" could not be observed there. The adapter
advertises no login methods and creates a session even with no login, so a missing login only shows at the first prompt; the error shape is what the browser
will have to explain. See "Claude credential check" in `docs/research/structured-adapters/probe-report.md`.

**Install the adapter once** (any scratch directory, Node 22):

```
export WORKSPACE=$HOME/claude-probe && mkdir -p $WORKSPACE
npm install --ignore-scripts --no-audit --no-fund --prefix "$WORKSPACE/claude" @agentclientprotocol/claude-agent-acp@0.99.1
```

**Run A, a machine or OS account that is logged out of Claude** (no `ANTHROPIC_API_KEY`, no `CLAUDE_CODE_OAUTH_TOKEN`, no login under the home directory):

```
python3 -I scripts/check-claude-credentials.py prompt --workspace "$WORKSPACE" --yes-spend --home real
```

This sends one short prompt that allows no tools. Logged out, it should fail without any usage being billed. Keep the JSON it prints and
`$WORKSPACE/claude-credentials-prompt.json`. Check `authenticatedHint` is false, and note the error code and message verbatim.

**Run B, the same on a logged-in machine:** the same command. Expect a reply of `ok` (`replyIsOk` true) and `authenticatedHint` true. This spends a very small
amount of the account's usage.

**Run C, the connector profile against the real adapter, no model call:**

```
REET_CLAUDE_ACP=$WORKSPACE/claude/node_modules/.bin/claude-agent-acp go test ./internal/acpagent -run TestRealClaudeStart -v
```

**Record** the adapter version, the OS, whether each run was logged in, `authenticatedHint`, and the logged-out error text. Put it in the probe report and the PR.
Only then decide whether the browser needs a "sign in to Claude on that machine" message, and whether to retire `REET_EXPERIMENTAL_CLAUDE_CHAT`.

## 3. Codex sign-in (needed before Codex chats can start)

**Current order (2 November):** run signed-in **Run B first**. The owner approved building login access from source evidence, with this run as final
verification before Codex is enabled. If `session/new` still says "Authentication required", stop and bring that result back before any Run A follow-up.
The implementation and fake-child tests do not count as a real signed-in result.

Codex chats stay behind `REET_EXPERIMENTAL_CODEX_CHAT` and cannot start until reet knows how the adapter behaves for a person who is already signed in to Codex (ChatGPT).
`scripts/probe-codex-start.py` makes no model call and no network request. Install the pinned adapter and CLI once:

```
export WORKSPACE=$HOME/codex-probe && mkdir -p $WORKSPACE
npm install --ignore-scripts --no-audit --no-fund --prefix "$WORKSPACE/codex" @agentclientprotocol/codex-acp@1.13.1 @openai/codex@0.156.1
```

**Run A, signed out** (an OS account with no `~/.codex/auth.json`): `python3 -I scripts/probe-codex-start.py --workspace "$WORKSPACE" --auth none`. Expect `session/new` to fail with
"Authentication required" and an `_auth/status_update` of `kind: none`. (`--auth chat-gpt` would make the adapter try to open a browser; reet never does that, so only use it to
reproduce the earlier finding.)

**Run B, signed in:** by default the script uses an empty temporary `CODEX_HOME`, so it cannot see your login. Copy your Codex directory and point the script at the copy (the adapter
may write to it): `cp -r ~/.codex "$WORKSPACE/codex-home-copy"`, then `python3 -I scripts/probe-codex-start.py --workspace "$WORKSPACE" --auth none --codex-home "$WORKSPACE/codex-home-copy"`.
**Record** whether `session/new` succeeds with no `authenticate` call, that it starts in `agent`, and the `authStatusUpdates` (kind and label) the script prints (the email is never included
out of what you share). Delete the copy afterwards; it holds your login.

Codex login access and its "Sign in to Codex on that machine first" message are already merged in #391, authorized from source evidence. Run B remains their real-login verification before retiring the switch. The owner deferred this run on 2 November; no pass is recorded.
