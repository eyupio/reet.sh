# Home starts OpenCode as a native chat

Follow-up to #258. The Home launch bar was still the terminal path for OpenCode: the chat choice existed only in the Start
an agent dialog. This change makes the normal Home OpenCode launch the native chat and leaves the terminal one word away.

- `LaunchChoice.chat` is optional. `initialChoice`, `withAgent`, `withProject`, `withHome` and `initialChoice` derive it
  from the machine's `acp` capability and preserve an explicit terminal choice (`chat:false`). A late machine read promotes
  an untouched OpenCode choice to chat.
- The sentence reads "Run OpenCode as a chat …"; the Session slot offers `a chat` / `a terminal`. Model, worktree and
  thinking disappear in chat; the terminal choice brings them back. A chat runs in the project folder, not a worktree.
- `toCreate` sets `structured` and `trust` and omits model, worktree, base, effort and YOLO for chat. Images continue through
  the launch-upload path. A chat starts in one folder; a multi-project selection must switch to a terminal.
- `startable` checks the machine's `acp`, readiness and prompt; it does not need the model list. The launch timeout is 60 s
  for a chat, like the dialog.
- Tests: launchBar unit cases for default/terminal/no-acp and sentence slots; `launch.e2e.mjs` covers the Home sentence,
  the Session toggle, hidden model/worktree slots, and the create body. The existing Home/phone cases stay.
