#!/usr/bin/env python3
"""Codex ACP adapter start probe (SC4): what reet's Codex profile has to get right to start a chat.

Makes NO model call and needs no network. Runs the pinned codex-acp adapter with a temporary HOME and CODEX_HOME and
no credential variables, then records, in order:
  1. initialize: agentInfo.name and version, authMethods;
  2. session/new BEFORE authenticate (does it say "Authentication required"?);
  3. session/new after a synthetic api-key authenticate: starting mode and the options offered;
  4. the mode switch reet uses (session/set_config_option configId=mode) and the one the earlier probes used (session/set_mode):
     what each replies, and whether a fresh session/new in the same process reflects the change is not tested.
Prints a JSON summary and writes the full message log to <workspace>/codex-start.json. Installs nothing.
"""
import argparse, json, os, selectors, shutil, signal, subprocess, tempfile, time
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("--workspace", type=Path, required=True, help="directory holding codex/node_modules")
parser.add_argument("--node", default=shutil.which("node"))
parser.add_argument("--mode", default="read-only")
parser.add_argument("--auth", choices=["api-key", "chat-gpt", "none"], default="api-key", help="authenticate method to try; none makes no authenticate call (reet's approach); chat-gpt with no login on the machine records what the adapter does (a login URL, an error, or a wait)")
parser.add_argument("--codex-home", type=Path, default=None, help="use this directory as CODEX_HOME instead of an empty temporary one, e.g. a COPY of a signed-in ~/.codex; the adapter may write there")
parser.add_argument("--auth-timeout", type=int, default=15)
args = parser.parse_args()
if args.node is None:
    parser.error("Node 22 or newer is required")
root, node = args.workspace.resolve(), str(Path(args.node).resolve())
entry = root / "codex/node_modules/@agentclientprotocol/codex-acp/dist/index.js"
if not entry.is_file():
    parser.error("the adapter is not installed under " + str(root / "codex"))
run = Path(tempfile.mkdtemp(prefix="reet-codex-start-", dir=root))
for p in ["project", "home", "codex-home", "config", "data", "cache"]:
    (run / p).mkdir()
env = {"PATH": str(root / "codex/node_modules/.bin") + ":" + str(Path(node).parent) + ":/usr/bin:/bin", "HOME": str(run / "home"),
       "CODEX_HOME": str(args.codex_home.resolve() if args.codex_home else run / "codex-home"), "XDG_CONFIG_HOME": str(run / "config"), "XDG_DATA_HOME": str(run / "data"),
       "XDG_CACHE_HOME": str(run / "cache"), "LANG": "C.UTF-8"}
proc = subprocess.Popen([node, str(entry)], cwd=run / "project", env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE, start_new_session=True)
sel = selectors.DefaultSelector()
sel.register(proc.stdout, selectors.EVENT_READ, "stdout")
sel.register(proc.stderr, selectors.EVENT_READ, "stderr")
buf, records = {"stdout": b"", "stderr": b""}, []


def send(m):
    records.append({"client": m})
    proc.stdin.write((json.dumps(m) + "\n").encode())
    proc.stdin.flush()


def call(ident, method, params, timeout=40):
    send({"jsonrpc": "2.0", "id": ident, "method": method, "params": params})
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        for key, _ in sel.select(0.2):
            data = os.read(key.fileobj.fileno(), 65536)
            if not data:
                sel.unregister(key.fileobj)
                continue
            buf[key.data] += data
            while b"\n" in buf[key.data]:
                line, buf[key.data] = buf[key.data].split(b"\n", 1)
                if key.data == "stderr":
                    records.append({"stderr": line.decode(errors="replace")[:500]})
                    continue
                m = json.loads(line)
                records.append({"agent": m})
                if "method" in m and "id" in m:
                    send({"jsonrpc": "2.0", "id": m["id"], "error": {"code": -32601, "message": "probe does not implement this callback"}})
                elif m.get("id") == ident:
                    return m
    return {"timeout": ident}


def modes(res):
    out = {"configOptions": [{k: o.get(k) for k in ("id", "currentValue")} for o in res.get("configOptions") or []]}
    if res.get("modes"):
        out["modes"] = {"currentModeId": res["modes"].get("currentModeId"), "available": [m.get("id") for m in res["modes"].get("availableModes", [])]}
    return out


summary = {}
try:
    init = call(1, "initialize", {"protocolVersion": 1, "clientInfo": {"name": "reet-probe", "version": "1"},
                "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False}})
    r = init.get("result", {})
    summary["initialize"] = {"agentInfo": r.get("agentInfo"), "authMethods": [m.get("id") for m in r.get("authMethods") or []]}
    before = call(2, "session/new", {"cwd": str(run / "project"), "mcpServers": []})
    summary["newBeforeAuthenticate"] = before.get("error", "ok" if "result" in before else before)
    if args.auth == "none":
        auth = {"result": None}
    elif args.auth == "api-key":
        auth = call(3, "authenticate", {"methodId": "api-key", "_meta": {"api-key": {"apiKey": "probe-placeholder-only"}}})
    else:
        auth = call(3, "authenticate", {"methodId": "chat-gpt"}, timeout=args.auth_timeout)
    summary["authenticate"] = {"method": args.auth, "outcome": auth.get("error", "ok" if "result" in auth else auth), "result": auth.get("result")}
    created = call(4, "session/new", {"cwd": str(run / "project"), "mcpServers": []})
    if "result" in created:
        native = created["result"]["sessionId"]
        summary["newAfterAuthenticate"] = modes(created["result"])
        cfg = call(5, "session/set_config_option", {"sessionId": native, "configId": "mode", "value": args.mode})
        summary["setConfigOptionMode"] = {"error": cfg.get("error"), "result": modes(cfg["result"]) if "result" in cfg else None, "rawKeys": sorted(cfg.get("result", {}))}
        legacy = call(6, "session/set_mode", {"sessionId": native, "modeId": args.mode})
        summary["setMode"] = {"error": legacy.get("error"), "rawResult": legacy.get("result")}
    else:
        summary["newAfterAuthenticate"] = created
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
    (root / "codex-start.json").write_text(json.dumps(summary | {"log": records}, indent=2, default=str) + "\n")
