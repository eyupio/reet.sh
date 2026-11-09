#!/usr/bin/env python3
"""Describe how Cline's ACP session exposes and switches its provider.

This makes NO model call. It copies a Cline folder (default ~/.cline) to a
private temporary directory and runs `cline --acp --auto-approve false` against
that copy, so the real sign-in and settings are never changed. It then:

  1. reads the `provider` option the session reports (its values and current),
  2. sets each value with session/set_config_option and records the read-back
     and the size of the model catalogue that follows,
  3. starts a second process on the same copy and reads the provider again, to
     see whether a switch outlives the session, and
  4. lists the files in the copy that changed (logs excluded).

The report answers three questions the reet design leaves open: whether
`provider` can be set at all, how a free source is exposed, and whether a
switch is per chat or written to Cline's own folder. It installs nothing.

Run it on a machine where Cline is signed in, then paste the JSON into
docs/research/cline-acp-probe.md.
"""
import argparse
import hashlib
import json
import os
import selectors
import shlex
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path


class Agent:
    """One `cline --acp` child spoken to as newline-delimited JSON-RPC."""

    def __init__(self, argv, env, cwd):
        self.proc = subprocess.Popen(
            argv, cwd=cwd, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, start_new_session=True,
        )
        self.sel = selectors.DefaultSelector()
        self.sel.register(self.proc.stdout, selectors.EVENT_READ)
        self.buf, self.answers, self.next_id = b"", {}, 1

    def _send(self, message):
        self.proc.stdin.write((json.dumps(message) + "\n").encode())
        self.proc.stdin.flush()

    def _take(self, data):
        self.buf += data
        while b"\n" in self.buf:
            line, self.buf = self.buf.split(b"\n", 1)
            try:
                message = json.loads(line)
            except ValueError:
                continue
            if "method" in message and "id" in message:
                self._send({"jsonrpc": "2.0", "id": message["id"], "error": {"code": -32601, "message": "probe does not implement this callback"}})
            elif "id" in message:
                self.answers[message["id"]] = message

    def call(self, method, params, timeout=40):
        ident, self.next_id = self.next_id, self.next_id + 1
        self._send({"jsonrpc": "2.0", "id": ident, "method": method, "params": params})
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            for key, _ in self.sel.select(0.2):
                data = os.read(key.fileobj.fileno(), 65536)
                if data:
                    self._take(data)
            if ident in self.answers:
                return self.answers.pop(ident)
            if self.proc.poll() is not None:
                break
        return {"error": {"code": None, "message": "no answer to " + method}}

    def close(self):
        try:
            os.killpg(self.proc.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            self.proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            os.killpg(self.proc.pid, signal.SIGKILL)
            self.proc.wait()


def option(options, config_id):
    """The configOptions entry with this id, or None."""
    return next((o for o in options if o.get("id") == config_id), None)


def values_of(entry):
    """The select values of one configOptions entry, flattening groups."""
    out = []
    for choice in (entry or {}).get("options") or []:
        if "value" in choice:
            out.append(choice["value"])
        for nested in choice.get("options") or []:
            out.append(nested.get("value"))
    return [v for v in out if v]


def snapshot(root):
    """Relative path -> sha256 of every file under root, logs excluded."""
    out = {}
    for path in sorted(root.rglob("*")):
        if not path.is_file() or "logs" in path.relative_to(root).parts:
            continue
        out[str(path.relative_to(root))] = hashlib.sha256(path.read_bytes()).hexdigest()
    return out


def start_session(argv, env, workdir):
    """Start a child, initialize and open a session; return (agent, configOptions or None, error)."""
    agent = Agent(argv, env, workdir)
    agent.call("initialize", {
        "protocolVersion": 1,
        "clientInfo": {"name": "reet-probe", "version": "1"},
        "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False},
    })
    created = agent.call("session/new", {"cwd": str(workdir), "mcpServers": []})
    if "result" not in created:
        return agent, None, created.get("error")
    agent.session_id = created["result"].get("sessionId")
    return agent, created["result"].get("configOptions") or [], None


def probe(argv, cline_dir, workdir):
    """Run the whole probe against a private copy of cline_dir and return the report."""
    home = Path(tempfile.mkdtemp(prefix="reet-cline-probe-"))
    copy = home / "cline"
    shutil.copytree(cline_dir, copy, symlinks=False)
    env = {k: v for k, v in os.environ.items() if k in ("PATH", "LANG", "CLINE_API_KEY")}
    env.update({"HOME": str(home), "CLINE_DIR": str(copy)})
    report = {"providerValues": [], "switches": [], "persistedAfterRestart": False, "changedFiles": [], "freeLike": []}
    first = second = None
    try:
        before = snapshot(copy)
        first, options, error = start_session(argv, env, workdir)
        if options is None:
            report["error"] = {"stage": "session/new", "detail": error}
            return report
        provider = option(options, "provider")
        report["providerValues"] = values_of(provider)
        original = (provider or {}).get("currentValue")
        report["providerCurrent"] = original
        free = [v for v in report["providerValues"] if "free" in v.lower()]
        last_ok = None
        for value in report["providerValues"]:
            answer = first.call("session/set_config_option", {"sessionId": first.session_id, "configId": "provider", "value": value})
            entry = {"value": value, "ok": "result" in answer, "code": (answer.get("error") or {}).get("code")}
            if "result" in answer:
                replied = answer["result"].get("configOptions") or []
                read_back = (option(replied, "provider") or {}).get("currentValue")
                models = values_of(option(replied, "model"))
                entry.update({"readBack": read_back, "modelCount": len(models)})
                free += [m for m in models if "free" in m.lower()]
                if read_back == value:
                    last_ok = value
            report["switches"].append(entry)
        report["freeLike"] = sorted(set(free))
        first.close()
        first = None
        report["changedFiles"] = sorted(p for p, h in snapshot(copy).items() if before.get(p) != h)
        second, again, _ = start_session(argv, env, workdir)
        if again is not None and last_ok is not None and last_ok != original:
            report["persistedAfterRestart"] = (option(again, "provider") or {}).get("currentValue") == last_ok
        return report
    finally:
        for agent in (first, second):
            if agent is not None:
                agent.close()
        shutil.rmtree(home, ignore_errors=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--cline", default="cline", help="the Cline command (default: cline); it is run as `<cline> --acp --auto-approve false`")
    parser.add_argument("--cline-dir", type=Path, default=Path.home() / ".cline", help="a signed-in Cline folder to copy (default: ~/.cline)")
    args = parser.parse_args()
    if not args.cline_dir.is_dir():
        parser.error("no Cline folder at " + str(args.cline_dir))
    workdir = Path(tempfile.mkdtemp(prefix="reet-cline-probe-project-"))
    try:
        report = probe(shlex.split(args.cline) + ["--acp", "--auto-approve", "false"], args.cline_dir, workdir)
    finally:
        shutil.rmtree(workdir, ignore_errors=True)
    print(json.dumps(report, indent=2, default=str))
    return 2 if "error" in report else 0


if __name__ == "__main__":
    sys.exit(main())
