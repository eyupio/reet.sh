#!/usr/bin/env python3
"""Describe the non-permission option values a pinned ACP adapter reports.

This makes NO model call. It starts the adapter with an isolated HOME and
project, calls initialize and session/new, and prints the configOptions that
have a category other than mode or model. Codex needs a synthetic api-key
authenticate before session/new; that is a placeholder, not a real credential
and not something reet does in a chat. It installs nothing.

Use it to refresh the measured contract in docs/research/structured-adapters/
after changing an adapter's pinned version.
"""
import argparse
import json
import os
import selectors
import shutil
import signal
import subprocess
import tempfile
import time
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("--workspace", type=Path, required=True, help="directory holding the adapter npm install")
parser.add_argument("--agent", choices=["claude", "codex"], required=True)
parser.add_argument("--node", default=shutil.which("node"))
parser.add_argument("--auth", choices=["none", "api-key"], default=None, help="Codex defaults to api-key; Claude defaults to none")
args = parser.parse_args()
if args.node is None:
    parser.error("Node 22 or newer is required")
auth = args.auth or ("api-key" if args.agent == "codex" else "none")

root = args.workspace.resolve()
entry = root / args.agent / "node_modules" / ("@agentclientprotocol/claude-agent-acp" if args.agent == "claude" else "@agentclientprotocol/codex-acp") / "dist" / "index.js"
if not entry.is_file():
    parser.error("the adapter is not installed under " + str(root / args.agent))
run = Path(tempfile.mkdtemp(prefix="reet-options-", dir=root))
for part in ["project", "home", "config", "data", "cache"]:
    (run / part).mkdir()
env = {
    "PATH": str(root / args.agent / "node_modules/.bin") + ":" + str(Path(args.node).parent) + ":/usr/bin:/bin",
    "HOME": str(run / "home"),
    "XDG_CONFIG_HOME": str(run / "config"),
    "XDG_DATA_HOME": str(run / "data"),
    "XDG_CACHE_HOME": str(run / "cache"),
    "LANG": "C.UTF-8",
}
proc = subprocess.Popen(
    [str(Path(args.node).resolve()), str(entry)],
    cwd=run / "project",
    env=env,
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.DEVNULL,
    start_new_session=True,
)
sel = selectors.DefaultSelector()
sel.register(proc.stdout, selectors.EVENT_READ)
buf, answers, records = b"", {}, []


def send(message):
    records.append({"client": message})
    proc.stdin.write((json.dumps(message) + "\n").encode())
    proc.stdin.flush()


def take(data):
    global buf
    buf += data
    while b"\n" in buf:
        line, buf = buf.split(b"\n", 1)
        message = json.loads(line)
        records.append({"agent": message})
        if "method" in message and "id" in message:
            send({"jsonrpc": "2.0", "id": message["id"], "error": {"code": -32601, "message": "probe does not implement this callback"}})
        elif "id" in message:
            answers[message["id"]] = message


def call(ident, method, params, timeout=40):
    send({"jsonrpc": "2.0", "id": ident, "method": method, "params": params})
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        for key, _ in sel.select(0.2):
            data = os.read(key.fileobj.fileno(), 65536)
            if data:
                take(data)
        if ident in answers:
            return answers.pop(ident)
    return {"timeout": ident}


summary = {}
try:
    initialized = call(1, "initialize", {
        "protocolVersion": 1,
        "clientInfo": {"name": "reet-probe", "version": "1"},
        "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False},
    })
    result = initialized.get("result", {})
    summary["initialize"] = {"agentInfo": result.get("agentInfo")}
    next_id = 2
    if auth == "api-key":
        summary["authenticate"] = call(next_id, "authenticate", {"methodId": "api-key", "_meta": {"api-key": {"apiKey": "probe-placeholder-only"}}})
        next_id += 1
    created = call(next_id, "session/new", {"cwd": str(run / "project"), "mcpServers": []})
    if "result" not in created:
        summary["new"] = created
    else:
        summary["new"] = {"sessionId": "(omitted)", "configOptions": [
            {
                "id": option.get("id"),
                "category": option.get("category"),
                "type": option.get("type"),
                "values": [value.get("value") for value in option.get("options") or []],
            }
            for option in created["result"].get("configOptions") or []
            if option.get("category") not in ("mode", "model")
        ]}
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
    (root / ("agent-options-" + args.agent + ".json")).write_text(json.dumps(summary | {"log": records}, indent=2, default=str) + "\n")
