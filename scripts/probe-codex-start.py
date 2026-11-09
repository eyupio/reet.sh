#!/usr/bin/env python3
"""Codex ACP adapter start probe (SC4): what reet's Codex profile has to get right to start a chat.

Makes NO model call and needs no network. Runs the pinned codex-acp adapter with a temporary HOME and CODEX_HOME and
no credential variables, then records, in order:
  1. initialize: agentInfo.name and version, authMethods;
  2. session/new BEFORE authenticate (does it say "Authentication required"?);
  3. session/new after a synthetic api-key authenticate: starting mode and the options offered;
  4. the mode switch reet uses (session/set_config_option configId=mode) and the one the earlier probes used (session/set_mode):
     what each replies, and whether a fresh session/new in the same process reflects the change is not tested;
  5. what the adapter advertises it can do (agentCapabilities: image prompts, session/load, the session methods);
  6. every option with its type and values, and a sweep that sets each mode, each reasoning effort and each model on one session and
     records what the adapter reads back (the new current value, and which effort values the model offers), so the descriptor can say only what was seen;
  7. session/load of a session id that was never stored: the error it answers.
Steps 5 to 7 are signed-out measurements; a signed-in run (Run B) adds what needs a real session.
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
buf, records, auth_updates = {"stdout": b"", "stderr": b""}, [], []


def send(m):
    records.append({"client": m})
    proc.stdin.write((json.dumps(m) + "\n").encode())
    proc.stdin.flush()


def take(data, channel, answers):
    """Parse every complete line now buffered, recording all of them. Responses are collected by id so a notification that arrives in the same read as a response is never dropped."""
    buf[channel] += data
    while b"\n" in buf[channel]:
        line, buf[channel] = buf[channel].split(b"\n", 1)
        if channel == "stderr":
            records.append({"stderr": line.decode(errors="replace")[:500]})
            continue
        m = json.loads(line)
        records.append({"agent": m})
        if "method" in m and "id" in m:
            send({"jsonrpc": "2.0", "id": m["id"], "error": {"code": -32601, "message": "probe does not implement this callback"}})
        elif m.get("method") == "_auth/status_update":
            status = (m.get("params") or {}).get("authStatus") or {}
            # Kind and label only: the payload can carry the account email, which does not belong in a shared summary.
            auth_updates.append({"kind": status.get("kind"), "label": status.get("label")})
        elif "id" in m:
            answers[m["id"]] = m


def call(ident, method, params, timeout=40):
    send({"jsonrpc": "2.0", "id": ident, "method": method, "params": params})
    answers = {}
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        for key, _ in sel.select(0.2):
            data = os.read(key.fileobj.fileno(), 65536)
            if not data:
                sel.unregister(key.fileobj)
                continue
            take(data, key.data, answers)
        if ident in answers:
            return answers[ident]
    return {"timeout": ident}


def drain(seconds=1.0):
    """Read what is still arriving after the last call, so a trailing notification is recorded before the adapter is stopped."""
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        for key, _ in sel.select(0.2):
            data = os.read(key.fileobj.fileno(), 65536)
            if data:
                take(data, key.data, {})
            else:
                sel.unregister(key.fileobj)


def modes(res):
    out = {"configOptions": [{k: o.get(k) for k in ("id", "currentValue")} for o in res.get("configOptions") or []]}
    if res.get("modes"):
        out["modes"] = {"currentModeId": res["modes"].get("currentModeId"), "available": [m.get("id") for m in res["modes"].get("availableModes", [])]}
    return out


def option_table(res):
    """Every option: its type, current value and the values offered, so a change in what Codex offers shows up in the record."""
    out = {}
    for o in res.get("configOptions") or []:
        values = [v.get("value") for v in o.get("options") or [] if isinstance(v, dict)]
        out[o.get("id")] = {"type": o.get("type"), "currentValue": o.get("currentValue"), "values": values}
    return out


seq = [100]


def ask(method, params):
    seq[0] += 1
    return call(seq[0], method, params)


def set_option(native, config_id, value):
    """What the adapter answers to one option change and what it reports as the option's value afterwards (the read-back)."""
    res = ask("session/set_config_option", {"sessionId": native, "configId": config_id, "value": value})
    if "result" not in res:
        return {"value": value, "error": res.get("error", res)}
    table = option_table(res["result"])
    return {"value": value, "readBack": (table.get(config_id) or {}).get("currentValue"), "effortValues": (table.get("reasoning_effort") or {}).get("values"),
            "effortNow": (table.get("reasoning_effort") or {}).get("currentValue"), "optionIds": sorted(table)}


def fresh():
    """A new session from the default state, so one sweep's changes do not colour the next."""
    res = ask("session/new", {"cwd": str(run / "project"), "mcpServers": []})
    return res["result"]["sessionId"] if "result" in res else None


def sweeps(native):
    """Set each mode and each effort in turn, then each model on a session of its own (what that model offers and defaults to), then
    carry one chosen effort across every model switch (does the choice survive, and what happens when the new model lacks it)."""
    base = option_table(ask("session/set_config_option", {"sessionId": native, "configId": "mode", "value": args.mode}).get("result") or {})
    out = {cid: [set_option(native, cid, v) for v in (base.get(cid) or {}).get("values") or []] for cid in ("mode", "reasoning_effort")}
    models = (base.get("model") or {}).get("values") or []
    out["model"] = [set_option(fresh(), "model", v) for v in models]
    for effort in ("high", "ultra"):
        carried = []
        for v in models:
            sid = fresh()
            set_option(sid, "reasoning_effort", effort)
            carried.append(set_option(sid, "model", v))
        out["effortCarried:" + effort] = carried
    return out


def unstored_load():
    res = ask("session/load", {"sessionId": "00000000-0000-4000-8000-000000000000", "cwd": str(run / "project"), "mcpServers": []})
    return {"error": res.get("error"), "result": res.get("result")}


summary = {}
try:
    init = call(1, "initialize", {"protocolVersion": 1, "clientInfo": {"name": "reet-probe", "version": "1"},
                "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False}})
    r = init.get("result", {})
    summary["initialize"] = {"agentInfo": r.get("agentInfo"), "authMethods": [m.get("id") for m in r.get("authMethods") or []]}
    caps = r.get("agentCapabilities") or {}
    summary["capabilities"] = {"loadSession": caps.get("loadSession"), "promptCapabilities": caps.get("promptCapabilities"),
                               "sessionCapabilities": sorted((caps.get("sessionCapabilities") or {}).keys()), "mcpCapabilities": caps.get("mcpCapabilities")}
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
        summary["options"] = option_table(created["result"])
        summary["sweeps"] = sweeps(native)
        summary["unstoredLoad"] = unstored_load()
    else:
        summary["newAfterAuthenticate"] = created
    drain()
    summary["authStatusUpdates"] = auth_updates
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
