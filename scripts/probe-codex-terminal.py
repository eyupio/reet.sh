#!/usr/bin/env python3
"""Codex CLI terminal probe (slice 3 of the Codex native-support design): what the argv reet builds for a Codex terminal session does,
measured against a loopback Responses-API stand-in so no account, network or real model is involved.

Modes (each runs codex in a throwaway HOME and CODEX_HOME whose config.toml points at the stand-in):
  effort   `codex exec -c model_reasoning_effort=<value>` for several values; records the reasoning effort the stand-in's request carries
           (whether the CLI honours the override, and what it does with a value it does not know).
  chat-id  `codex exec resume <id>` of a session the ACP adapter created (run `probe-codex-lifecycle.py load` first against the same
           workspace), from the same folder and from another; records whether the earlier turns reach the provider.
  argv     the interactive TUI under a pty with the exact argv given after a `--`, recording the model, effort, approval policy and user
           messages the stand-in received. `{ID}` in the argv is replaced by the id of a session made first (--pre exec: with `codex exec`;
           --pre tui: by running the TUI once with a prompt, which is what `resume --last` needs: it continues the latest interactive session).

Needs <workspace>/codex with the adapter and its codex installed. Prints a JSON summary. Sends nothing to any real service.
"""
import argparse, fcntl, glob, http.server, json, os, pty, select, shutil, signal, struct, subprocess, sys, tempfile, termios, threading, time
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("mode", choices=["effort", "chat-id", "argv"])
parser.add_argument("--workspace", type=Path, required=True, help="directory holding codex/node_modules")
parser.add_argument("--pre", choices=["none", "exec", "tui"], default="none", help="argv mode: make a session first")
raw, tail = sys.argv[1:], []
if "--" in raw:  # everything after -- is the argv handed to codex, untouched
    cut = raw.index("--")
    raw, tail = raw[:cut], raw[cut + 1:]
args = parser.parse_args(raw)
args.argv = tail
root = args.workspace.resolve()
bindir = root / "codex/node_modules/.bin"
codex = str(bindir / "codex")
node = shutil.which("node")
if not (bindir / "codex").exists() or node is None:
    parser.error("the codex CLI is not installed under " + str(root / "codex") + ", or Node is missing")
requests = []


def texts_of(items):
    out = []
    for item in items:
        if isinstance(item, dict) and isinstance(item.get("content"), list):
            out += [c.get("text", "") for c in item["content"] if isinstance(c, dict)]
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
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))) or b"{}")
        requests.append({"model": body.get("model"), "effort": (body.get("reasoning") or {}).get("effort"), "texts": texts_of(body.get("input", []))})
        item = {"type": "message", "role": "assistant", "id": "m1", "content": [{"type": "output_text", "text": "Probe reply."}]}
        events = [{"type": "response.created", "response": {"id": "r"}}, {"type": "response.output_item.done", "item": item},
                  {"type": "response.completed", "response": {"id": "r", "usage": {"input_tokens": 1, "input_tokens_details": None, "output_tokens": 1, "output_tokens_details": None, "total_tokens": 2}}}]
        payload = "".join("event: " + e["type"] + "\ndata: " + json.dumps(e) + "\n\n" for e in events).encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Stub)
threading.Thread(target=server.serve_forever, daemon=True).start()
base = "http://127.0.0.1:%d/v1" % server.server_port


def config(project):
    return ('model = "probe-model"\nmodel_provider = "stub"\n\n[projects."%s"]\ntrust_level = "trusted"\n\n[model_providers.stub]\nname = "stub"\nbase_url = "%s"\n'
            'env_key = "OPENAI_API_KEY"\nwire_api = "responses"\n' % (project, base))


def environment(home, codex_home):
    return {"PATH": str(bindir) + ":/usr/bin:/bin:" + os.path.dirname(node), "HOME": str(home), "CODEX_HOME": str(codex_home), "LANG": "C.UTF-8", "TERM": "xterm-256color",
            "OPENAI_API_KEY": "probe-placeholder-only"}


def workspace():
    d = Path(tempfile.mkdtemp(prefix="reet-codex-term-", dir=root))
    for name in ("home", "ch", "proj"):
        (d / name).mkdir()
    (d / "ch/config.toml").write_text(config(d / "proj"))
    return d


def session_id(codex_home):
    files = sorted(glob.glob(str(codex_home / "sessions/**/rollout-*.jsonl"), recursive=True), key=os.path.getmtime)
    # rollout-<date>T<hh-mm-ss>-<uuid>.jsonl
    return Path(files[-1]).stem.split("rollout-")[1][20:] if files else ""


def tui(cwd, env, argv, seconds=40):
    """Run the interactive TUI under a pty until the stand-in has been asked, answering the terminal's capability queries."""
    before = len(requests)
    pid, fd = pty.fork()
    if pid == 0:
        try:
            os.chdir(cwd)
            os.execve(codex, [codex] + argv, env)
        finally:
            os._exit(127)  # exec failed: never let the child carry on as a second copy of this script
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    end = time.time() + seconds
    while time.time() < end:
        ready, _, _ = select.select([fd], [], [], 0.5)
        if ready:
            try:
                data = os.read(fd, 65536)
            except OSError:
                break
            if not data:
                break
            if b"\x1b[6n" in data:
                os.write(fd, b"\x1b[1;1R")
            if b"\x1b[c" in data or b"\x1b[0c" in data:
                os.write(fd, b"\x1b[?1;2c")
        if len(requests) > before:
            time.sleep(2)
            break
    # Stop it and wait for it to be gone: a session the TUI still holds reads as "open in another app" to the next run (codex 0.166).
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.kill(pid, sig)
        except ProcessLookupError:
            break
        time.sleep(1)
        if os.waitpid(pid, os.WNOHANG)[0] == pid:
            break
    time.sleep(1)
    os.close(fd)
    return requests[before:]


def stop_daemons(folder):
    """codex 0.166 starts a background app-server under CODEX_HOME that outlives the TUI and keeps the session it served: a later resume with a
    changed config reads it as "open in another app". Stop the ones started from this folder (matched by its path, which is unique to a run)."""
    for entry in Path("/proc").iterdir():
        if entry.name.isdigit() and int(entry.name) != os.getpid():
            try:
                cmd = (entry / "cmdline").read_bytes().decode(errors="replace")
            except OSError:
                continue
            if str(folder) in cmd and "app-server" in cmd:
                try:
                    os.kill(int(entry.name), signal.SIGTERM)
                    time.sleep(1)
                    os.kill(int(entry.name), signal.SIGKILL)  # a daemon that ignored TERM; ProcessLookupError below means it is gone
                except ProcessLookupError:
                    pass
    time.sleep(1)


def summary(rows):
    out = []
    for r in rows:
        approval = next((t.split("Approval policy is currently ")[1].split(".")[0] for t in r["texts"] if "Approval policy is currently" in t), None)
        out.append({"model": r["model"], "effort": r["effort"], "approval": approval, "userMessages": [t[:60] for t in r["texts"] if not t.startswith("<") and len(t) < 80]})
    return out


result = {"codex": subprocess.check_output([codex, "--version"]).decode().strip(), "mode": args.mode}
try:
    if args.mode == "effort":
        result["effort"] = {}
        for value in ["low", "medium", "high", "xhigh", "ultra", "bogus"]:
            d = workspace()
            before = len(requests)
            run = subprocess.run([codex, "exec", "--skip-git-repo-check", "-c", "model_reasoning_effort=" + value, "hello"], cwd=d / "proj", env=environment(d / "home", d / "ch"), capture_output=True, text=True, timeout=90)
            result["effort"][value] = {"exit": run.returncode, "requestEfforts": [r["effort"] for r in requests[before:]]}
            stop_daemons(d / "ch")
    elif args.mode == "chat-id":
        runs = sorted(glob.glob(str(root / "reet-codex-life-load-*")), key=os.path.getmtime)
        log = root / "codex-lifecycle-load.json"
        if not runs or not log.exists():
            parser.error("run probe-codex-lifecycle.py load against this workspace first")
        run = Path(runs[-1])
        records = json.load(open(log))
        sid = next(m["<a"]["result"]["sessionId"] for m in records if "<a" in m and isinstance(m["<a"].get("result"), dict) and "sessionId" in m["<a"]["result"])
        (run / "codex-home/config.toml").write_text(config(run / "project"))
        for label, folder in (("sameFolder", run / "project"), ("otherFolder", Path(tempfile.mkdtemp(prefix="reet-codex-other-", dir=root)))):
            before = len(requests)
            proc = subprocess.run([codex, "exec", "--skip-git-repo-check", "resume", sid, "Say it again."], cwd=folder, env=environment(run / "home", run / "codex-home"), capture_output=True, text=True, timeout=90)
            result[label] = {"exit": proc.returncode, "userMessages": summary(requests[before:])[:1]}
    else:
        argv = list(args.argv)
        d = workspace()
        env = environment(d / "home", d / "ch")
        if args.pre == "exec":
            subprocess.run([codex, "exec", "--skip-git-repo-check", "first turn word amber"], cwd=d / "proj", env=env, capture_output=True, text=True, timeout=90)
        elif args.pre == "tui":
            tui(d / "proj", env, ["--", "first turn word amber"])
            if os.environ.get("REET_PROBE_KEEP_DAEMON") != "1":
                stop_daemons(d / "ch")
        sid = session_id(d / "ch")
        argv = [a.replace("{ID}", sid) for a in argv]
        result["argv"], result["sessionMadeFirst"] = argv, bool(sid)
        result["requests"] = summary(tui(d / "proj", env, argv))
        stop_daemons(d / "ch")
    print(json.dumps(result, indent=2))
finally:
    server.shutdown()
    server.server_close()
