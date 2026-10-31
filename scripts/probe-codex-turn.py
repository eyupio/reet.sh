#!/usr/bin/env python3
"""Codex ACP adapter turn probe (SC2 open gates).

Runs the pinned codex-acp adapter against a loopback Responses-API stub with a
temporary CODEX_HOME and project, synthetic credentials and an explicit
environment. Decisions: allow, deny, cancel. The stub asks the model for one
harmless command (or --tool-name/--tool-args for another tool). With --discover
the stub only returns text and records the tool catalogue it was offered.
It installs nothing and records observations; assertions are stated below.
"""
import argparse, http.server, json, os, selectors, shlex, shutil, signal, subprocess, tempfile, threading, time
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("decision", choices=["allow", "deny", "cancel", "error"])
parser.add_argument("--workspace", type=Path, required=True)
parser.add_argument("--node", default=shutil.which("node"))
parser.add_argument("--discover", action="store_true")
parser.add_argument("--tool-name", default="shell_command")
parser.add_argument("--outside", action="store_true", help="write the marker outside the project directory")
parser.add_argument("--config-extra", default="", help="extra top-level config.toml lines, e.g. approval_policy=\"untrusted\"")
parser.add_argument("--mode", default="", help="session config mode to select before the turn (observation only)")
args = parser.parse_args()
if args.node is None:
    parser.error("Node 22 or newer is required")
root = args.workspace.resolve()
node = str(Path(args.node).resolve())
run = Path(tempfile.mkdtemp(prefix="reet-codex-" + args.decision + "-", dir=root))
for p in ["home", "codex-home", "project", "config", "cache", "data"]:
    (run / p).mkdir(parents=True, exist_ok=True)
project = run / "project"
marker = (run / "outside-marker.txt") if args.outside else (project / "probe-marker.txt")
command = "printf reet-probe > " + shlex.quote(str(marker))
records = []
offered = {"tools": None}


def sse(events):
    return "".join("event: " + e["type"] + "\ndata: " + json.dumps(e) + "\n\n" for e in events).encode()


class Stub(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_GET(self):
        payload = json.dumps({"data": [], "models": []}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_POST(self):
        size = int(self.headers.get("Content-Length", "0"))
        if size > 4 * 1024 * 1024:
            self.send_error(413)
            return
        raw = self.rfile.read(size) or b"{}"
        try:
            body = json.loads(raw)
        except ValueError:
            body = {}
        tools = [t.get("name") or t.get("type") for t in body.get("tools", [])]
        offered["tools"] = tools
        records.append({"http": self.path, "tools": tools, "model": body.get("model")})
        if args.decision == "error":
            payload = json.dumps({"error": {"message": "probe provider failure", "type": "server_error"}}).encode()
            self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
            return
        items = body.get("input", [])
        followed = any(isinstance(i, dict) and i.get("type", "").endswith("_output") for i in items)
        if followed or args.discover:
            item = {"type": "message", "role": "assistant", "id": "msg_1", "content": [{"type": "output_text", "text": "Probe completed."}]}
        else:
            call_args = {"command": command} if args.tool_name == "shell_command" else {"cmd": command}
            item = {"type": "function_call", "id": "fc_1", "call_id": "call_probe", "name": args.tool_name, "arguments": json.dumps(call_args)}
        events = [
            {"type": "response.created", "response": {"id": "resp_probe"}},
            {"type": "response.output_item.done", "item": item},
            {"type": "response.completed", "response": {"id": "resp_probe", "usage": {"input_tokens": 10, "input_tokens_details": None, "output_tokens": 5, "output_tokens_details": None, "total_tokens": 15}}},
        ]
        payload = sse(events)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Stub)
threading.Thread(target=server.serve_forever, daemon=True).start()
base = "http://127.0.0.1:" + str(server.server_port) + "/v1"
(run / "codex-home/config.toml").write_text(
    'model = "probe-model"\nmodel_provider = "stub"\n' + args.config_extra.replace(";", "\n") + "\n"
    "[model_providers.stub]\n"
    'name = "stub"\nbase_url = "' + base + '"\nenv_key = "OPENAI_API_KEY"\nwire_api = "responses"\n'
)
env = {
    "PATH": str(root / "codex/node_modules/.bin") + ":" + str(Path(node).parent) + ":/usr/bin:/bin",
    "HOME": str(run / "home"),
    "CODEX_HOME": str(run / "codex-home"),
    "XDG_CONFIG_HOME": str(run / "config"),
    "XDG_DATA_HOME": str(run / "data"),
    "XDG_CACHE_HOME": str(run / "cache"),
    "LANG": "C.UTF-8",
    "OPENAI_API_KEY": "probe-placeholder-only",
}
entry = root / "codex/node_modules/@agentclientprotocol/codex-acp/dist/index.js"
proc = subprocess.Popen([node, str(entry)], cwd=project, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE, start_new_session=True)
sel = selectors.DefaultSelector()
sel.register(proc.stdout, selectors.EVENT_READ, "stdout")
sel.register(proc.stderr, selectors.EVENT_READ, "stderr")
buffers = {"stdout": b"", "stderr": b""}
permissions, notifications = [], []


def send(m):
    records.append({"client": m})
    proc.stdin.write((json.dumps(m) + "\n").encode())
    proc.stdin.flush()


def pump(until_id, timeout=60):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        for key, _ in sel.select(0.2):
            data = os.read(key.fileobj.fileno(), 65536)
            if not data:
                sel.unregister(key.fileobj)
                continue
            ch = key.data
            buffers[ch] += data
            if len(buffers[ch]) > 4 * 1024 * 1024:
                raise RuntimeError("stream too large")
            while b"\n" in buffers[ch]:
                line, buffers[ch] = buffers[ch].split(b"\n", 1)
                if ch == "stderr":
                    records.append({"stderr": line.decode(errors="replace")[:1000]})
                    continue
                m = json.loads(line)
                records.append({"agent": m})
                if "method" in m and "id" in m:
                    if m["method"] == "session/request_permission":
                        permissions.append(m["params"])
                        if args.decision == "cancel":
                            send({"jsonrpc": "2.0", "method": "session/cancel", "params": {"sessionId": m["params"]["sessionId"]}})
                            continue
                        want = "allow_once" if args.decision == "allow" else "reject_once"
                        option = next((o["optionId"] for o in m["params"]["options"] if o["kind"] == want), None)
                        result = {"outcome": {"outcome": "selected", "optionId": option}} if option else {"outcome": {"outcome": "cancelled"}}
                        send({"jsonrpc": "2.0", "id": m["id"], "result": result})
                    else:
                        send({"jsonrpc": "2.0", "id": m["id"], "error": {"code": -32601, "message": "probe does not implement this callback"}})
                elif "method" in m:
                    notifications.append(m)
                elif m.get("id") == until_id:
                    return m
    return {"timeout": until_id}


def call(ident, method, params, timeout=60):
    send({"jsonrpc": "2.0", "id": ident, "method": method, "params": params})
    return pump(ident, timeout)


summary = {"decision": args.decision}
try:
    init = call(1, "initialize", {"protocolVersion": 1, "clientInfo": {"name": "reet-probe", "version": "1"},
                "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False}})
    summary["authMethods"] = [m.get("id") for m in init.get("result", {}).get("authMethods", [])]
    summary["agentCapabilities"] = init.get("result", {}).get("agentCapabilities")
    auth = call(2, "authenticate", {"methodId": "api-key", "_meta": {"api-key": {"apiKey": "probe-placeholder-only"}}})
    summary["authenticate"] = auth.get("result", auth.get("error"))
    created = call(3, "session/new", {"cwd": str(project), "mcpServers": []})
    if "result" not in created:
        summary["new"] = created
    else:
        res = created["result"]
        native = res["sessionId"]
        summary["new"] = {k: res.get(k) for k in ("sessionId", "modes", "models", "configOptions")}
        if args.mode:
            summary["setMode"] = call(4, "session/set_mode", {"sessionId": native, "modeId": args.mode})
        started = time.monotonic()
        done = call(5, "session/prompt", {"sessionId": native, "prompt": [{"type": "text", "text": "Reet ACP probe: run the isolated marker command, then finish."}]}, 40)
        summary["turn"] = done.get("result", done)
        summary["turnSeconds"] = round(time.monotonic() - started, 1)
        summary["permissions"] = [{"kind": p["toolCall"].get("kind"), "title": p["toolCall"].get("title"), "rawInput": p["toolCall"].get("rawInput"),
                                   "content": p["toolCall"].get("content"), "options": p["options"]} for p in permissions]
        updates = [n["params"].get("update", {}) for n in notifications if n.get("method") == "session/update"]
        summary["updateKinds"] = sorted({u.get("sessionUpdate") for u in updates if u.get("sessionUpdate")})
        summary["otherNotifications"] = sorted({n.get("method") for n in notifications if n.get("method") != "session/update"})
        summary["errorUpdates"] = [u for u in updates if "error" in json.dumps(u).lower()][:3]
        summary["markerExists"] = marker.exists()
        summary["markerContent"] = marker.read_text() if marker.exists() else None
    summary["offeredTools"] = offered["tools"]
    summary["httpRequests"] = sum("http" in r for r in records)
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
    server.shutdown()
    server.server_close()
    (root / ("codex-turn-" + args.decision + ".json")).write_text(json.dumps(records, indent=2, default=str) + "\n")
