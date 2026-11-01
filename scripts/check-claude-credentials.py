#!/usr/bin/env python3
"""Claude credential checks for the ACP adapter (SC4 gate).

Runs the pinned adapter with the same environment shape the connector's Claude
profile uses (acpagent.ClaudeCandidate): an explicit environment, only the two
credential variables forwarded, background traffic switched off, user settings
only, and a temporary project directory.

Scenarios:
  session  Initialize and create a session. Makes NO model call and cannot spend anything.
           Records whether the adapter advertises login methods and whether session
           creation notices a missing login. (Observed: it does not; creation succeeds.)
  prompt   Also sends ONE short prompt asking for the word "ok", allows no tools, and records
           the outcome. If the account is logged in this makes a real model call and
           uses a small amount of its usage, so it needs --yes-spend. On a logged-out
           machine it should fail without billing; the error shape is the evidence.

--home isolated (default) uses an empty temporary home. --home real uses this
machine's own HOME so an existing login is found; ANTHROPIC_API_KEY and
CLAUDE_CODE_OAUTH_TOKEN are forwarded from the environment if set.

CAUTION: an empty home and no credential variables do NOT prove the run is
logged out. In the SC4 session the adapter authenticated with both, because the
sandbox supplies credentials outside the process. To check the missing-login
behaviour, run the prompt scenario on a machine or OS account that is logged out of
Claude, and read the "authenticated" hint in the summary (files an authenticated
session writes into the home) as a hint, not a proof.

It prints a JSON summary with environment variable NAMES only, never values, and
writes the full message log to <workspace>/claude-credentials-<scenario>.json.
It installs nothing. Install the adapter first:

  npm install --ignore-scripts --no-audit --no-fund --prefix "$WORKSPACE/claude" \
    @agentclientprotocol/claude-agent-acp@0.99.1
"""
import argparse, json, os, selectors, shutil, signal, subprocess, tempfile, time
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("scenario", choices=["session", "prompt"])
parser.add_argument("--home", choices=["isolated", "real"], default="isolated")
parser.add_argument("--workspace", type=Path, required=True, help="directory holding claude/node_modules and the output")
parser.add_argument("--node", default=shutil.which("node"))
parser.add_argument("--yes-spend", action="store_true", help="required for the prompt scenario: if logged in it makes one small model call")
parser.add_argument("--timeout", type=int, default=90)
args = parser.parse_args()
if args.node is None:
    parser.error("Node 22 or newer is required")
if args.scenario == "prompt" and not args.yes_spend:
    parser.error("the prompt scenario may make one small model call; pass --yes-spend to allow it")
root = args.workspace.resolve()
node = str(Path(args.node).resolve())
entry = root / "claude/node_modules/@agentclientprotocol/claude-agent-acp/dist/index.js"
if not entry.is_file():
    parser.error("the adapter is not installed under " + str(root / "claude") + "; see the install command in this script's header")
run = Path(tempfile.mkdtemp(prefix="reet-claude-cred-" + args.scenario + "-", dir=root))
project = run / "project"
for p in ["project", "home/.claude", "config", "cache", "data"]:
    (run / p).mkdir(parents=True, exist_ok=True)

env = {
    "PATH": str(Path(node).parent) + ":/usr/bin:/bin",
    "LANG": "C.UTF-8",
    "XDG_CONFIG_HOME": str(run / "config"),
    "XDG_DATA_HOME": str(run / "data"),
    "XDG_CACHE_HOME": str(run / "cache"),
    "DISABLE_AUTOUPDATER": "1",
    "DISABLE_TELEMETRY": "1",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION": "false",
}
if args.home == "isolated":
    env["HOME"] = str(run / "home")
else:
    # The real login lives under the real home. CLAUDE_CONFIG_DIR is deliberately not forwarded.
    env["HOME"] = os.environ.get("HOME", str(run / "home"))
if args.home == "real" or args.scenario == "prompt":
    for name in ("ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"):
        if os.environ.get(name):
            env[name] = os.environ[name]

records = []
proc = subprocess.Popen([node, str(entry)], cwd=project, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE, start_new_session=True)
sel = selectors.DefaultSelector()
sel.register(proc.stdout, selectors.EVENT_READ, "stdout")
sel.register(proc.stderr, selectors.EVENT_READ, "stderr")
buf = {"stdout": b"", "stderr": b""}
notes, permissions = [], []


def send(m):
    records.append({"client": m})
    proc.stdin.write((json.dumps(m) + "\n").encode())
    proc.stdin.flush()


def pump(until_id, timeout):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        for key, _ in sel.select(0.2):
            data = os.read(key.fileobj.fileno(), 65536)
            if not data:
                sel.unregister(key.fileobj)
                continue
            ch = key.data
            buf[ch] += data
            if len(buf[ch]) > 4 * 1024 * 1024:
                raise RuntimeError("stream too large")
            while b"\n" in buf[ch]:
                line, buf[ch] = buf[ch].split(b"\n", 1)
                if ch == "stderr":
                    records.append({"stderr": line.decode(errors="replace")[:500]})
                    continue
                m = json.loads(line)
                records.append({"agent": m})
                if "method" in m and "id" in m:
                    if m["method"] == "session/request_permission":
                        # No tool is wanted. Refuse anything the model tries.
                        permissions.append(m["params"].get("toolCall", {}).get("title"))
                        reject = next((o["optionId"] for o in m["params"]["options"] if o["kind"] == "reject_once"), None)
                        send({"jsonrpc": "2.0", "id": m["id"], "result": {"outcome": {"outcome": "selected", "optionId": reject} if reject else {"outcome": "cancelled"}}})
                    else:
                        send({"jsonrpc": "2.0", "id": m["id"], "error": {"code": -32601, "message": "not available"}})
                elif "method" in m:
                    notes.append(m)
                elif m.get("id") == until_id:
                    return m
    return {"timeout": until_id}


def call(ident, method, params, timeout=None):
    send({"jsonrpc": "2.0", "id": ident, "method": method, "params": params})
    return pump(ident, timeout or args.timeout)


summary = {"scenario": args.scenario, "environmentNames": sorted(env), "credentialNamesForwarded": sorted(n for n in env if n in ("ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"))}
try:
    init = call(1, "initialize", {"protocolVersion": 1, "clientInfo": {"name": "reet-credential-check", "version": "1"},
                "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False}})
    result = init.get("result", init)
    summary["agentInfo"] = result.get("agentInfo") if isinstance(result, dict) else None
    summary["authMethods"] = [{"id": m.get("id"), "name": m.get("name")} for m in (result.get("authMethods") or [])] if isinstance(result, dict) else None
    created = call(2, "session/new", {"cwd": str(project), "mcpServers": [],
                   "_meta": {"claudeCode": {"options": {"settingSources": ["user"]}}}})
    summary["sessionNew"] = {"ok": "result" in created, "error": created.get("error"), "timeout": created.get("timeout")}
    if "result" in created:
        res = created["result"]
        native = res["sessionId"]
        summary["mode"] = next((o.get("currentValue") for o in res.get("configOptions", []) if o.get("id") == "mode"), None)
        if args.scenario == "prompt":
            started = time.monotonic()
            done = call(3, "session/prompt", {"sessionId": native, "prompt": [{"type": "text", "text": 'Reply with exactly the single word: ok. Do not use any tool.'}]})
            summary["prompt"] = {"result": done.get("result"), "error": done.get("error"), "timeout": done.get("timeout"), "seconds": round(time.monotonic() - started, 1)}
            text = "".join(n["params"]["update"].get("content", {}).get("text", "") for n in notes
                           if n.get("method") == "session/update" and n["params"]["update"].get("sessionUpdate") == "agent_message_chunk")
            summary["replyLength"] = len(text)
            summary["replyIsOk"] = text.strip().lower().strip(".") == "ok"
            summary["toolsRequested"] = permissions
    home = Path(env["HOME"])
    summary["authenticatedHint"] = (home / ".claude/remote-settings.json").exists() or (home / ".claude/policy-limits.json").exists()
    summary["stderrTail"] = [r["stderr"] for r in records if "stderr" in r][-5:]
    print(json.dumps(summary, indent=2, default=str))
finally:
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        proc.wait(timeout=3)
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        proc.wait()
    (root / ("claude-credentials-" + args.scenario + ".json")).write_text(json.dumps(summary | {"log": records}, indent=2, default=str) + "\n")
