#!/usr/bin/env python3
"""Codex ACP adapter lifecycle probe (slice 2 of the Codex native-support design): what a stored conversation, an image and a chosen effort
or model do, measured against a loopback Responses-API stub so no account, network or real model is involved.

Modes (each starts its own adapter in a throwaway HOME and CODEX_HOME with a stub provider in config.toml):
  load    run one text turn, stop the adapter, start a second adapter on the same CODEX_HOME and session/load the stored id with the
          same folder and with a different one; record the replayed update kinds, the answer, and whether a follow-up prompt on the loaded
          session reaches the stub. Also session/load of an id that was never stored.
  images  send a prompt with an image block and record whether the stub's request carries it as an image input, and what the turn ends with.
  options set the reasoning effort and then the model with session/set_config_option and run a turn after each, recording the effort and
          model the stub's request carries, so a chosen setting is shown to reach the provider and not only to read back.

Needs <workspace>/codex with the adapter installed. Prints a JSON summary and writes the message log to <workspace>/codex-lifecycle-<mode>.json.
Sends no prompt to any real service; the stub answers every request with one short text message. Installs nothing.
"""
import argparse, base64, http.server, json, os, selectors, shutil, signal, subprocess, tempfile, threading, time, zlib, struct
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("mode", choices=["load", "images", "options"])
parser.add_argument("--workspace", type=Path, required=True, help="directory holding codex/node_modules")
parser.add_argument("--node", default=shutil.which("node"))
args = parser.parse_args()
if args.node is None:
    parser.error("Node 22 or newer is required")
root, node = args.workspace.resolve(), str(Path(args.node).resolve())
entry = root / "codex/node_modules/@agentclientprotocol/codex-acp/dist/index.js"
if not entry.is_file():
    parser.error("the adapter is not installed under " + str(root / "codex"))
run = Path(tempfile.mkdtemp(prefix="reet-codex-life-" + args.mode + "-", dir=root))
for p in ["home", "codex-home", "project", "other-project", "config", "cache", "data"]:
    (run / p).mkdir()
project, other = run / "project", run / "other-project"
records, requests = [], []


def png():
    """A 2x2 PNG, built here so the probe carries no binary file."""
    def chunk(kind, data):
        body = kind + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body) & 0xFFFFFFFF)
    raw = b"".join(b"\x00" + b"\xff\x00\x00" * 2 for _ in range(2))
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 2, 2, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


def kinds_of(items):
    """The content types in a request's input, without the text itself."""
    out = []
    for item in items:
        if isinstance(item, dict):
            content = item.get("content")
            if isinstance(content, list):
                out.extend(sorted({c.get("type") for c in content if isinstance(c, dict) and c.get("type")}))
            else:
                out.append(item.get("type"))
    return out


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
        if size > 8 * 1024 * 1024:
            self.send_error(413)
            return
        try:
            body = json.loads(self.rfile.read(size) or b"{}")
        except ValueError:
            body = {}
        items = body.get("input", [])
        requests.append({"model": body.get("model"), "effort": (body.get("reasoning") or {}).get("effort"), "inputKinds": kinds_of(items),
                         "imageParts": sum(1 for i in items if isinstance(i, dict) and isinstance(i.get("content"), list) for c in i["content"] if isinstance(c, dict) and c.get("type") == "input_image"),
                         "inputItems": len(items)})
        item = {"type": "message", "role": "assistant", "id": "msg_1", "content": [{"type": "output_text", "text": "Probe reply."}]}
        events = [{"type": "response.created", "response": {"id": "resp_probe"}}, {"type": "response.output_item.done", "item": item},
                  {"type": "response.completed", "response": {"id": "resp_probe", "usage": {"input_tokens": 10, "input_tokens_details": None, "output_tokens": 5, "output_tokens_details": None, "total_tokens": 15}}}]
        payload = "".join("event: " + e["type"] + "\ndata: " + json.dumps(e) + "\n\n" for e in events).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Stub)
threading.Thread(target=server.serve_forever, daemon=True).start()
base = "http://127.0.0.1:" + str(server.server_port) + "/v1"
(run / "codex-home/config.toml").write_text('model = "probe-model"\nmodel_provider = "stub"\n\n[model_providers.stub]\nname = "stub"\nbase_url = "' + base + '"\nenv_key = "OPENAI_API_KEY"\nwire_api = "responses"\n')
env = {"PATH": str(root / "codex/node_modules/.bin") + ":" + str(Path(node).parent) + ":/usr/bin:/bin", "HOME": str(run / "home"), "CODEX_HOME": str(run / "codex-home"),
       "XDG_CONFIG_HOME": str(run / "config"), "XDG_DATA_HOME": str(run / "data"), "XDG_CACHE_HOME": str(run / "cache"), "LANG": "C.UTF-8", "OPENAI_API_KEY": "probe-placeholder-only"}


class Adapter:
    """One adapter process. Every message in and out goes to the shared log, and update notifications are kept by the call that was waiting."""

    def __init__(self, label):
        self.label, self.ident = label, 0
        self.proc = subprocess.Popen([node, str(entry)], cwd=project, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        self.sel = selectors.DefaultSelector()
        self.sel.register(self.proc.stdout, selectors.EVENT_READ, "stdout")
        self.sel.register(self.proc.stderr, selectors.EVENT_READ, "stderr")
        self.buf = {"stdout": b"", "stderr": b""}

    def send(self, m):
        records.append({self.label + ">": m})
        self.proc.stdin.write((json.dumps(m) + "\n").encode())
        self.proc.stdin.flush()

    def call(self, method, params, timeout=60):
        self.ident += 1
        want, updates = self.ident, []
        self.send({"jsonrpc": "2.0", "id": want, "method": method, "params": params})
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            for key, _ in self.sel.select(0.2):
                data = os.read(key.fileobj.fileno(), 65536)
                if not data:
                    self.sel.unregister(key.fileobj)
                    continue
                self.buf[key.data] += data
                while b"\n" in self.buf[key.data]:
                    line, self.buf[key.data] = self.buf[key.data].split(b"\n", 1)
                    if key.data == "stderr":
                        records.append({self.label + " stderr": line.decode(errors="replace")[:500]})
                        continue
                    m = json.loads(line)
                    records.append({"<" + self.label: m})
                    if "method" in m and "id" in m:
                        self.send({"jsonrpc": "2.0", "id": m["id"], "error": {"code": -32601, "message": "probe does not implement this callback"}})
                    elif m.get("method") == "session/update":
                        updates.append((m.get("params") or {}).get("update") or {})
                    elif m.get("id") == want:
                        return m, updates
        return {"timeout": want}, updates

    def start(self):
        self.call("initialize", {"protocolVersion": 1, "clientInfo": {"name": "reet-probe", "version": "1"}, "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False}})
        self.call("authenticate", {"methodId": "api-key", "_meta": {"api-key": {"apiKey": "probe-placeholder-only"}}})

    def stop(self):
        for sig in (signal.SIGTERM, signal.SIGKILL):
            try:
                os.killpg(self.proc.pid, sig)
            except ProcessLookupError:
                return
            try:
                self.proc.wait(timeout=3)
                return
            except subprocess.TimeoutExpired:
                continue


def update_kinds(updates):
    return sorted({u.get("sessionUpdate") for u in updates if u.get("sessionUpdate")})


def replay_text(updates):
    """Which sides of the conversation the replay carried, by update kind, without the message text."""
    return {k: sum(1 for u in updates if u.get("sessionUpdate") == k) for k in update_kinds(updates)}


def options_of(res):
    return {o.get("id"): o.get("currentValue") for o in (res or {}).get("configOptions") or []}


def prompt(adapter, native, blocks):
    before = len(requests)
    res, updates = adapter.call("session/prompt", {"sessionId": native, "prompt": blocks}, 40)
    return {"stopReason": (res.get("result") or {}).get("stopReason"), "error": res.get("error"), "requests": requests[before:], "updateKinds": update_kinds(updates)}


summary = {"mode": args.mode}
first = Adapter("a")
second = None
try:
    first.start()
    new, _ = first.call("session/new", {"cwd": str(project), "mcpServers": []})
    if "result" not in new:
        summary["new"] = new
        print(json.dumps(summary, indent=2, default=str))
        raise SystemExit(1)
    native = new["result"]["sessionId"]
    if args.mode == "load":
        summary["firstTurn"] = prompt(first, native, [{"type": "text", "text": "Remember the word amber."}])
        first.stop()
        second = Adapter("b")
        second.start()
        for label, cwd in (("sameFolder", project), ("otherFolder", other)):
            loaded, updates = second.call("session/load", {"sessionId": native, "cwd": str(cwd), "mcpServers": []})
            result = loaded.get("result")
            entry_ = {"error": loaded.get("error"), "answered": result is not None, "options": options_of(result), "replay": replay_text(updates)}
            if result is not None and label == "sameFolder":
                entry_["followUp"] = prompt(second, native, [{"type": "text", "text": "What was the word?"}])
            summary[label] = entry_
        stray, updates = second.call("session/load", {"sessionId": "00000000-0000-4000-8000-000000000000", "cwd": str(project), "mcpServers": []})
        summary["neverStored"] = {"error": stray.get("error"), "replay": replay_text(updates)}
    elif args.mode == "images":
        image = base64.b64encode(png()).decode()
        summary["withImage"] = prompt(first, native, [{"type": "text", "text": "Describe this picture."}, {"type": "image", "mimeType": "image/png", "data": image}])
        summary["textOnly"] = prompt(first, native, [{"type": "text", "text": "And now no picture."}])
    else:
        table = options_of(new["result"])
        summary["startingOptions"] = table
        summary["baseline"] = prompt(first, native, [{"type": "text", "text": "one"}])
        chosen = {}
        offered = {o.get("id"): [v.get("value") for v in o.get("options") or []] for o in new["result"].get("configOptions") or []}
        # The stub's own model name has no effort option, so the model is chosen first and the effort from what that model then offers.
        others = [m for m in offered.get("model", []) if m != table.get("model")]
        after = new["result"]
        if others:
            res, _ = first.call("session/set_config_option", {"sessionId": native, "configId": "model", "value": others[0]})
            after = res.get("result") or {}
            chosen["model"] = {"asked": others[0], "readBack": options_of(after).get("model"), "effortOffered": [v.get("value") for o in after.get("configOptions") or [] if o.get("id") == "reasoning_effort" for v in o.get("options") or []],
                               "turn": prompt(first, native, [{"type": "text", "text": "two"}])}
        values = [v.get("value") for o in after.get("configOptions") or [] if o.get("id") == "reasoning_effort" for v in o.get("options") or []]
        for effort in [e for e in ("high", "low") if e in values]:
            res, _ = first.call("session/set_config_option", {"sessionId": native, "configId": "reasoning_effort", "value": effort})
            chosen["effort:" + effort] = {"asked": effort, "readBack": options_of(res.get("result")).get("reasoning_effort"), "turn": prompt(first, native, [{"type": "text", "text": "three " + effort}])}
        summary["chosen"] = chosen
    summary["stubRequests"] = len(requests)
    print(json.dumps(summary, indent=2, default=str))
finally:
    first.stop()
    if second:
        second.stop()
    server.shutdown()
    server.server_close()
    (root / ("codex-lifecycle-" + args.mode + ".json")).write_text(json.dumps(records, indent=2, default=str) + "\n")
