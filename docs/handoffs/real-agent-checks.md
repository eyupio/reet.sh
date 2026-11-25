# Checks to run against real agents, machines and phones

Everything below is built and has passed its tests, but only against fixtures: nothing has run against a real agent, a real
machine, a real push service or a real reverse proxy. This is the list of what to try, in the order that finds the most for the
least effort, what a pass looks like, and what to send back when it does not pass. About two hours in all; each part stands alone.

You need: a reet controller on `https` with accounts on; one machine enrolled with the agent running as a service, terminal
access on (`reet agent terminal enable`); a phone; and whichever of Claude Code, Codex, OpenCode, Cline, Antigravity and Copilot you
have installed. Do not run this on a machine whose sessions you cannot afford to end.

## 1. Enrolment and tokens (10 minutes)

| Try | A pass looks like | If it fails, send |
| --- | --- | --- |
| **Machines → Add a machine**, copy the command, run it on the machine, type the code at the hidden prompt | The machine appears, online, with no "Enrolled with a personal token" mark | The command's output and the page |
| On a machine enrolled before codes (or one you enrol with a token on purpose), run `reet agent enroll --again` with a new code | Same machine, same name, new secret, every local setting kept (remote apply and terminal access as they were), the mark gone | The output; `reet agent` settings before and after |
| Revoke the personal token under Account, then look at the old machine | Its public-address routes stop; its private connection is documented to keep working until you remove it (SECURITY.md says so). Check that claim is true | What the Machines page shows |
| With a personal token, try to create or revoke a token, make an enrolment code, or run a backup | Each refuses with "do this from a signed-in browser" | The command and the answer |

## 2. The machine lock (15 minutes)

On the machine: start a chat and a terminal session from reet, then run `reet agent lock`.

| Try | A pass looks like |
| --- | --- |
| `reet agent lock status` | Says locked, since when, and how many sessions are still running |
| In the browser, start a new session, attach to the running one, type into it, browse folders, upload a file | Each is refused with the plain "Locked on the machine" sentence; attachments drop within about a second |
| The Machines page, the session panel, Sync and the Factory readiness line | Each says it is locked on the machine and only `reet agent unlock` lifts it |
| Inside the running session (as the agent), run `reet agent unlock` | It works. That is documented: the lock does not contain running processes. Confirm, then re-lock |
| `reet agent lock --end-sessions` | Locks, then ends every session and chat reet runs there; says how many |
| Restart the agent while locked, then unlock | Recorded chats do not restart while locked; after unlock they are still there |
| Update `reet` from the controller while locked | Refused |

Send back anything that started while locked, and what was on screen.

## 3. Agent questions (30 minutes, the most valuable)

Claude Code and Codex are the two agents the terminal question card understands. For each, in a **terminal** session:

1. Ask it to do something that makes it show a permission or choice dialog (run a shell command it must ask about; edit a file).
2. In reet the question and its options should appear as a card under the session header, and in Home if it has three or fewer
   options and nothing is cut.
3. Answer from the phone. The agent should act on the option you chose. Then ask it again and check a second dialog is a fresh
   question, not the old one.
4. Choose a standing-permission option ("don't ask again"): it should take a second tap.
5. While a question is up, press a key in the terminal yourself, then answer in reet: it should say "The question on the terminal
   changed. Look at it again before answering." and send nothing.

For **Cline, Antigravity and Copilot** (not built, so nothing should appear): get each to show a question or approval dialog in a
terminal session, and save the whole screen as plain text at that moment (in tmux: `tmux capture-pane -p -S -50`) plus a screenshot.
Those captures are what the roadmap's "waiting detection for Cline, Antigravity and Copilot" item needs to be built; send two or
three per agent, covering a yes/no and a multi-choice dialog.

## 4. Chat features (20 minutes)

In a structured **chat** with each agent that supports chat:

| Try | A pass looks like | If it fails, send |
| --- | --- | --- |
| Type `/` in the message box | The agent's own slash commands are listed, grouped; one that needs the terminal says so | The list and which agent |
| Choose one, then send | It goes to the agent as typed | |
| Ask the agent to start a subagent or a background command and let the turn end | The session shows as working in the background, not finished, with a count | The session screen and what the agent said |
| Wait out a background task you start with a long `sleep` | Finishes only when the task does; after 30 minutes of nothing it says "Stopped waiting for its background work; it may still be running." and shows the amber mark | The transcript |

## 5. Phone (30 minutes)

| Try | A pass looks like |
| --- | --- |
| Type half a message in a chat, switch to another app for a few minutes (or force-close the browser), come back | The words are back with "Draft from earlier. Edit or send." Nothing was sent |
| Do the same in the terminal's Compose box | The Compose key has a dot while a draft is kept |
| Put the phone in aeroplane mode for ten seconds with a chat open and type | The box stays typeable; Send is off with "Reconnecting. Your message is kept. Send turns on when you are back."; nothing sends by itself |
| **Notifications → Turn on**; make an agent need you; lock the phone | A notification arrives; tapping it opens that session, in the window already open if there is one |
| Turn on **Play a sound**, press Try it, keep a reet tab in front, trigger a notification | A short quiet two-note tone once. If it is silent, say whether you had pressed anything on the page since loading |
| Clear this site's data in the browser, or turn the site's notifications off and on in browser settings, then open reet | The Notifications card and the Sessions page say "Notifications stopped reaching this phone." and Repair fixes it in one tap |
| Touch targets on the terminal toolbar, key row, header, footer and the chat queue | Comfortable to hit one-handed at 390 px wide; note any that are not, and the model of phone |
| A very small phone (320 px wide) with a terminal open | One fewer row than before is expected; say if it feels cramped |

## 6. Sign-in behind a reverse proxy (10 minutes)

Only if you run reet behind one. Set `REET_TRUSTED_PROXIES` to the proxy's address, then fail a password sign-in six times from
one phone and once from another.

Pass: the first phone is slowed after the fifth try for that email and the second is not. If both are slowed, or a failed attempt
is charged to the proxy's address, send the proxy's `X-Forwarded-For` header as the controller sees it.

## 7. What a relay can see

Only if you can look at the relay or network path. Open a terminal session and watch traffic to the relay: it should be encrypted
WireGuard packets only, with no terminal text or credentials. This is the one statement in SECURITY.md that follows from how
the connection is built rather than from anything checked.

## Reporting

For each failure, send what is under "If it fails, send", the `reet` version on the machine and the controller, and which agent
and version. A screenshot of the whole page is better than a description.
