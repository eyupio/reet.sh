# Checks that need a real device or a real login

Two gates cannot be closed from CI or from a sandbox. This page says exactly what to do and what to write down. Do not turn on typed questions
(`elicitation.form`) or Claude chats (`REET_EXPERIMENTAL_CLAUDE_CHAT`) for anyone until the matching section below has been run and its results recorded
in `docs/structured-chat-plan.md` and the PR that records them.

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
**Record** whether `session/new` succeeds with no `authenticate` call, that it starts in `agent`, and the `_auth/status_update` `authStatus` kind and label it sends (leave the email
out of what you share). Delete the copy afterwards; it holds your login.

Only then wire the Codex start and its "Sign in to Codex on that machine first" message.
