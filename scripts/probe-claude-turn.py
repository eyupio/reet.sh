import argparse, http.server, json, os, selectors, shlex, shutil, signal, subprocess, tempfile, threading, time
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("decision", choices=["allow", "deny"])
parser.add_argument(
    "--tool", choices=["Bash", "Write", "AskUserQuestion"], default="Bash"
)
parser.add_argument("--workspace", type=Path, required=True)
parser.add_argument("--node", default=shutil.which("node"))
args = parser.parse_args()
if args.tool == "AskUserQuestion" and args.decision != "allow":
    parser.error("The question probe currently covers accept only")
root = args.workspace.resolve()
if args.node is None:
    parser.error("Node 22 or newer is required")
node = str(Path(args.node).resolve())
node_dir = str(Path(node).parent)
run = Path(
    tempfile.mkdtemp(
        prefix="reet-claude-" + args.tool.lower() + "-" + args.decision + "-", dir=root
    )
)
for p in ["home/.claude", "project", "config", "cache", "data"]:
    (run / p).mkdir(parents=True, exist_ok=True)
marker = run / "project" / "probe-marker.txt"
command = "printf reet-probe > " + shlex.quote(str(marker))
tool_input = {"command": command, "description": "Write an isolated probe marker"}
if args.tool == "Write":
    tool_input = {"file_path": str(marker), "content": "reet-probe"}
if args.tool == "AskUserQuestion":
    tool_input = {
        "questions": [
            {
                "question": "Which probe option?",
                "header": "Probe",
                "options": [
                    {"label": "Option A", "description": "First synthetic option"},
                    {"label": "Option B", "description": "Second synthetic option"},
                ],
                "multiSelect": False,
            }
        ]
    }
records = []


class Stub(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_GET(self):
        payload = json.dumps(
            {
                "data": [
                    {
                        "id": "claude-sonnet-4-5",
                        "type": "model",
                        "display_name": "Probe",
                        "created_at": "2026-01-01T00:00:00Z",
                    }
                ],
                "has_more": False,
            }
        ).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def do_POST(self):
        size = int(self.headers.get("Content-Length", "0"))
        if size > 2 * 1024 * 1024:
            self.send_error(413)
            return
        body = json.loads(self.rfile.read(size) or b"{}")
        records.append(
            {"http": self.path, "tools": [t.get("name") for t in body.get("tools", [])]}
        )
        if "count_tokens" in self.path:
            payload = b'{"input_tokens":100}'
            kind = "application/json"
        else:
            completed = any(
                isinstance(m.get("content"), list)
                and any(
                    c.get("type") == "tool_result"
                    for c in m["content"]
                    if isinstance(c, dict)
                )
                for m in body.get("messages", [])
            )
            content = (
                {"type": "text", "text": "Probe completed."}
                if completed
                else {
                    "type": "tool_use",
                    "id": "toolu_probe",
                    "name": args.tool,
                    "input": tool_input,
                }
            )
            message = {
                "id": "msg_probe",
                "type": "message",
                "role": "assistant",
                "model": "claude-sonnet-4-5",
                "content": [content],
                "stop_reason": "end_turn" if completed else "tool_use",
                "stop_sequence": None,
                "usage": {"input_tokens": 100, "output_tokens": 20},
            }
            if body.get("stream"):
                start = {**message, "content": [], "stop_reason": None}
                block = (
                    {"type": "text", "text": ""}
                    if completed
                    else {**content, "input": {}}
                )
                delta = (
                    {"type": "text_delta", "text": "Probe completed."}
                    if completed
                    else {
                        "type": "input_json_delta",
                        "partial_json": json.dumps(content["input"]),
                    }
                )
                events = [
                    ("message_start", {"type": "message_start", "message": start}),
                    (
                        "content_block_start",
                        {
                            "type": "content_block_start",
                            "index": 0,
                            "content_block": block,
                        },
                    ),
                    (
                        "content_block_delta",
                        {"type": "content_block_delta", "index": 0, "delta": delta},
                    ),
                    ("content_block_stop", {"type": "content_block_stop", "index": 0}),
                    (
                        "message_delta",
                        {
                            "type": "message_delta",
                            "delta": {
                                "stop_reason": message["stop_reason"],
                                "stop_sequence": None,
                            },
                            "usage": {"output_tokens": 20},
                        },
                    ),
                    ("message_stop", {"type": "message_stop"}),
                ]
                payload = "".join(
                    "event: " + k + "\ndata: " + json.dumps(v) + "\n\n"
                    for k, v in events
                ).encode()
                kind = "text/event-stream"
            else:
                payload = json.dumps(message).encode()
                kind = "application/json"
        self.send_response(200)
        self.send_header("Content-Type", kind)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Stub)
threading.Thread(target=server.serve_forever, daemon=True).start()
(run / "home/.claude/settings.json").write_text(
    json.dumps(
        {
            "permissions": {"ask": ["Bash(*)", "Edit(*)", "Write(*)"]},
            "disableAllHooks": True,
        }
    )
)
env = {
    "PATH": node_dir + ":/usr/bin:/bin",
    "HOME": str(run / "home"),
    "CLAUDE_CONFIG_DIR": str(run / "home/.claude"),
    "XDG_CONFIG_HOME": str(run / "config"),
    "XDG_DATA_HOME": str(run / "data"),
    "XDG_CACHE_HOME": str(run / "cache"),
    "LANG": "C.UTF-8",
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:" + str(server.server_port),
    "ANTHROPIC_API_KEY": "probe-placeholder-only",
    "ANTHROPIC_MODEL": "claude-sonnet-4-5",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "DISABLE_AUTOUPDATER": "1",
    "DISABLE_TELEMETRY": "1",
    "CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION": "false",
}
entry = root / "claude/node_modules/@agentclientprotocol/claude-agent-acp/dist/index.js"
proc = subprocess.Popen(
    [node, str(entry)],
    cwd=run / "project",
    env=env,
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    start_new_session=True,
)
sel = selectors.DefaultSelector()
sel.register(proc.stdout, selectors.EVENT_READ, "stdout")
sel.register(proc.stderr, selectors.EVENT_READ, "stderr")
buffers = {"stdout": b"", "stderr": b""}


def send(m):
    records.append({"client": m})
    proc.stdin.write((json.dumps(m) + "\n").encode())
    proc.stdin.flush()


def request(identifier, method, params):
    send({"jsonrpc": "2.0", "id": identifier, "method": method, "params": params})
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        for key, _ in sel.select(0.2):
            data = os.read(key.fileobj.fileno(), 65536)
            if not data:
                sel.unregister(key.fileobj)
                continue
            channel = key.data
            buffers[channel] += data
            if len(buffers[channel]) > 1024 * 1024:
                raise RuntimeError("stream too large")
            while b"\n" in buffers[channel]:
                line, buffers[channel] = buffers[channel].split(b"\n", 1)
                if channel == "stderr":
                    records.append({"stderr": line.decode(errors="replace")[:2000]})
                    continue
                m = json.loads(line)
                records.append({"agent": m})
                if "id" in m and "method" in m:
                    if m["method"] == "elicitation/create":
                        schema = m["params"].get("requestedSchema", {})
                        properties = schema.get("properties", {})
                        choices = properties.get("question_0", {}).get("oneOf", [])
                        if not any(
                            option.get("const") == "Option A" for option in choices
                        ):
                            raise RuntimeError(
                                "unexpected question schema; refusing to guess an answer"
                            )
                        if any(
                            key != "question_0" for key in schema.get("required", [])
                        ):
                            raise RuntimeError("unexpected required question field")
                        answers = {"question_0": "Option A"}
                        send(
                            {
                                "jsonrpc": "2.0",
                                "id": m["id"],
                                "result": {"action": "accept", "content": answers},
                            }
                        )
                    elif m["method"] == "session/request_permission":
                        desired = (
                            "allow_once" if args.decision == "allow" else "reject_once"
                        )
                        option = next(
                            (
                                o["optionId"]
                                for o in m["params"]["options"]
                                if o["kind"] == desired
                            ),
                            None,
                        )
                        result = (
                            {"outcome": {"outcome": "selected", "optionId": option}}
                            if option
                            else {"outcome": {"outcome": "cancelled"}}
                        )
                        send({"jsonrpc": "2.0", "id": m["id"], "result": result})
                    else:
                        send(
                            {
                                "jsonrpc": "2.0",
                                "id": m["id"],
                                "error": {
                                    "code": -32601,
                                    "message": "probe does not implement this callback",
                                },
                            }
                        )
                elif m.get("id") == identifier:
                    return m
    return {"timeout": method}


try:
    initialized = request(
        1,
        "initialize",
        {
            "protocolVersion": 1,
            "clientCapabilities": {
                "fs": {"readTextFile": False, "writeTextFile": False},
                "terminal": False,
                **(
                    {"elicitation": {"form": {}}}
                    if args.tool == "AskUserQuestion"
                    else {}
                ),
            },
            "clientInfo": {"name": "reet-probe", "version": "1"},
        },
    )
    created = request(2, "session/new", {"cwd": str(run / "project"), "mcpServers": []})
    if "result" in created:
        native = created["result"]["sessionId"]
        result = request(
            3,
            "session/prompt",
            {
                "sessionId": native,
                "prompt": [
                    {
                        "type": "text",
                        "text": "Reet ACP probe: run the isolated marker command, then finish.",
                    }
                ],
            },
        )
    else:
        result = created
    summary = {
        "decision": args.decision,
        "created": created.get("result", {}).get("sessionId"),
        "prompt": result,
        "markerExists": marker.exists(),
        "markerContent": marker.read_text() if marker.exists() else None,
        "elicitation": [
            m["agent"]["params"]
            for m in records
            if m.get("agent", {}).get("method") == "elicitation/create"
        ],
        "permissions": [
            m["agent"]["params"]
            for m in records
            if m.get("agent", {}).get("method") == "session/request_permission"
        ],
        "httpRequests": sum("http" in r for r in records),
    }
    print(json.dumps(summary, indent=2))
    if "result" not in result or result["result"].get("stopReason") != "end_turn":
        raise RuntimeError("probe turn did not finish")
    if args.tool == "AskUserQuestion":
        if (
            not summary["elicitation"]
            or summary["permissions"]
            or summary["markerExists"]
        ):
            raise RuntimeError("expected a question without tool execution")
    else:
        if not summary["permissions"] or summary["markerExists"] != (
            args.decision == "allow"
        ):
            raise RuntimeError(
                "permission decision did not match the observed file effect"
            )
        if summary["markerExists"] and summary["markerContent"] != "reet-probe":
            raise RuntimeError("unexpected marker content")
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
    sel.close()
    (
        root / ("claude-turn-" + args.tool.lower() + "-" + args.decision + ".json")
    ).write_text(json.dumps(records, indent=2) + "\n")
