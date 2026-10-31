import argparse, json, os, selectors, shlex, shutil, signal, subprocess, tempfile, time
from pathlib import Path

parser = argparse.ArgumentParser(
    description="Capture real ACP initialize/new contracts using synthetic credentials and isolated homes. Requires preinstalled candidate packages; installs nothing."
)
parser.add_argument("--workspace", type=Path, required=True)
parser.add_argument("--node", default=shutil.which("node"))
options = parser.parse_args()
ROOT = options.workspace.resolve()
if options.node is None:
    parser.error("Node 22 or newer is required")
NODE = str(Path(options.node).resolve())
if not Path(NODE).is_file():
    parser.error("Node 22 or newer is required")
NODE_DIR = str(Path(NODE).parent)


def probe(agent):
    run = Path(tempfile.mkdtemp(prefix="reet-" + agent + "-", dir=ROOT))
    for name in [
        "home",
        "home/.claude",
        "home/.codex",
        "project",
        "config",
        "data",
        "cache",
    ]:
        (run / name).mkdir(parents=True, exist_ok=True)
    entry = (
        ROOT
        / agent
        / "node_modules/@agentclientprotocol"
        / (agent + "-agent-acp" if agent == "claude" else "codex-acp")
        / "dist/index.js"
    )
    env = {
        "PATH": str(ROOT / agent / "node_modules/.bin")
        + ":"
        + NODE_DIR
        + ":/usr/bin:/bin",
        "HOME": str(run / "home"),
        "XDG_CONFIG_HOME": str(run / "config"),
        "XDG_DATA_HOME": str(run / "data"),
        "XDG_CACHE_HOME": str(run / "cache"),
        "LANG": "C.UTF-8",
        "CLAUDE_CONFIG_DIR": str(run / "home/.claude"),
        "CODEX_HOME": str(run / "home/.codex"),
        # Every inherited credential is excluded. These are synthetic values,
        # and provider addresses are closed local endpoints, never public APIs.
        "ANTHROPIC_BASE_URL": "http://127.0.0.1:9",
        "ANTHROPIC_API_KEY": "probe-placeholder-only",
        "OPENAI_BASE_URL": "http://127.0.0.1:9/v1",
        "OPENAI_API_KEY": "probe-placeholder-only",
    }
    version = subprocess.run(
        [NODE, str(entry), "--version"],
        cwd=run / "project",
        env=env,
        capture_output=True,
        text=True,
        timeout=10,
    )
    records = [
        {
            "stage": "version",
            "exit": version.returncode,
            "stdout": version.stdout[:1000],
            "stderr": version.stderr[:1000],
        }
    ]
    proc = subprocess.Popen(
        [NODE, str(entry)],
        cwd=run / "project",
        env=env,
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        start_new_session=True,
    )
    selector = selectors.DefaultSelector()
    selector.register(proc.stdout, selectors.EVENT_READ, "stdout")
    selector.register(proc.stderr, selectors.EVENT_READ, "stderr")
    buffers = {"stdout": b"", "stderr": b""}

    def send(message):
        records.append({"direction": "client", "message": message})
        proc.stdin.write((json.dumps(message) + "\n").encode())
        proc.stdin.flush()

    def request(identifier, method, params):
        send({"jsonrpc": "2.0", "id": identifier, "method": method, "params": params})
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            if proc.poll() is not None and not selector.get_map():
                return {"exit": proc.returncode}
            for key, _ in selector.select(timeout=0.2):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                channel = key.data
                buffers[channel] += chunk
                if len(buffers[channel]) > 1024 * 1024:
                    raise RuntimeError("oversized protocol stream")
                while b"\n" in buffers[channel]:
                    line, buffers[channel] = buffers[channel].split(b"\n", 1)
                    text = line.decode(errors="replace")
                    if channel == "stderr":
                        records.append({"direction": "stderr", "text": text[:4000]})
                        continue
                    try:
                        message = json.loads(text)
                    except json.JSONDecodeError:
                        records.append({"direction": "non-json", "text": text[:1000]})
                        continue
                    records.append({"direction": "agent", "message": message})
                    if "method" in message and "id" in message:
                        send(
                            {
                                "jsonrpc": "2.0",
                                "id": message["id"],
                                "error": {
                                    "code": -32601,
                                    "message": "probe callback not available",
                                },
                            }
                        )
                    elif message.get("id") == identifier:
                        return message
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
                },
                "clientInfo": {"name": "reet-probe", "version": "1"},
            },
        )
        if "result" in initialized:
            if agent == "codex":
                auth = request(
                    3,
                    "authenticate",
                    {
                        "methodId": "api-key",
                        "_meta": {"api-key": {"apiKey": "probe-placeholder-only"}},
                    },
                )
                records.append({"syntheticAuthentication": auth})
            created = request(
                2, "session/new", {"cwd": str(run / "project"), "mcpServers": []}
            )
        else:
            created = {"notAttempted": "initialize failed"}
        records.append({"summary": {"initialize": initialized, "new": created}})
        if initialized.get("result", {}).get("protocolVersion") != 1:
            raise RuntimeError("unexpected ACP protocol version")
        expected = {
            "claude": ("@agentclientprotocol/claude-agent-acp", "0.99.1"),
            "codex": ("@agentclientprotocol/codex-acp", "1.13.1"),
        }[agent]
        info = initialized.get("result", {}).get("agentInfo", {})
        if (
            info.get("name"),
            info.get("version"),
        ) != expected or "result" not in created:
            raise RuntimeError(
                "adapter identity or session creation did not match the pinned probe contract"
            )
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
        selector.close()
        (ROOT / (agent + "-protocol.json")).write_text(
            json.dumps(records, indent=2) + "\n"
        )
    print(
        json.dumps(
            {"agent": agent, "version": records[0], "summary": records[-1]}, indent=2
        )
    )


for agent in ["claude", "codex"]:
    probe(agent)
