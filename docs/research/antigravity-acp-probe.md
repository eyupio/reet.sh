# Google Antigravity ACP compatibility probe

5 November 2026. The official AGY 1.5.0 Linux x86-64 executable was tested with
an isolated home and project, a synthetic API-key selection, and a local
Gemini-compatible HTTP stub. No real provider request, Google login or model
usage occurred. Existing user configuration and credentials were not read or
changed.

The archive and SHA-256 are the ones recorded in
`internal/terminalhost/testdata/help/sources.json`. Its extracted executable is
called `antigravity`; the official installation exposes it as `agy`.

## Observed contract

| Surface | Observed with the native executable |
| --- | --- |
| Start | `agy acp --disable-telemetry`; ACP protocol 1; identity `Antigravity ACP Server`, version `1.5.0` |
| Missing login | `session/new` returns code `-32000`, `Authentication required`; reet names the local `agy` sign-in step |
| Mode | `mode=default`; the descriptor also offers `accept-edits` and `plan`, which this integration does not expose |
| Model | `session/new` reports a configured model; `session/set_config_option` with `configId=model` confirms an explicit model in its returned `configOptions` |
| Effort | `thought_level`; low/high for the default Gemini 3.1 Pro, low/medium/high after choosing Gemini 4 Argon; changes are read back before a prompt |
| Streaming | `agent_message_chunk` text and tool-call updates reach Gaffer through the shared ACP client |
| Commands | `rawInput.CommandLine` carries the exact command; `allow_once` / `reject_once` use option IDs `allow` / `deny` |
| Writes | `rawInput.Path` and `Content`, with typed diff content `{type: "diff", path, newText}` |
| Approval effects | Deny prevented each command/file marker; Allow once created the expected marker |
| Images | An embedded PNG reached the local provider as image input |
| History | Lower-case UUID session IDs; `session/load` after restarting ACP replayed user and agent messages |
| Stop | Stopping an approved long command retired the ACP process group and prevented its later marker write |

The narrowed runtime frames are in
[the contract fixture](structured-adapters/fixtures/antigravity.json).
Ordinary CI reproduces those shapes with a deterministic ACP peer; the optional
native test drives the actual executable through reet's production adapter.

## Permission and customization boundaries

Default mode uses AGY's configured permission rules. It is not a promise that
every operation asks: in particular, the native executable allowed writes in
`/tmp` without an approval request. The edit-approval test therefore runs in an
isolated workspace outside `/tmp`. Gaffer displays requests AGY makes and does
not infer permission from a tool's title. Its command normalizer recognizes the
measured `CommandLine` field; an absent command or diff still disables Allow.

The bundled AGY customization guide describes workspace roots `.agents`,
`.agent`, `_agents` and `_agent`, traversal to the repository root, global
configuration, and inherited plugin/hook configurations. Reet requires folder
trust for every AGY chat rather than claiming a filename scan rules these out.
It checks `mode=default` in the native configuration before a turn, does not
inherit `ANTIGRAVITY_PERM_GRANTS` or a provider URL override, and does not pass
the CLI's dangerous permission-bypass flag. Existing Gaffer YOLO remains a
separate, explicitly confirmed account/machine setting.

Real OAuth and enterprise credentials were not exercised. Production reet uses
AGY's existing machine-local login and never sends `authenticate`, starts an
interactive OAuth flow, or changes the selected account. The native test's
synthetic authentication is test setup only.

## Reproduce

Use the checksum-verified 1.5.0 executable from the terminal help capture:

```sh
mkdir -p work/agy-native-tmp
TMPDIR="$PWD/work/agy-native-tmp" \
REET_AGY_150="$PWD/work/agy-capture/antigravity" \
go test -race ./internal/acpagent -run 'TestRealAntigravityGaffer|TestAntigravity' -count=1 -v
```

`TestRealAntigravityGaffer` is skipped without `REET_AGY_150`. It also explains
the non-`/tmp` requirement rather than misreading AGY's temporary-file policy as
a Gaffer approval failure. The test only talks to its own loopback provider and
uses a fixed placeholder credential.
