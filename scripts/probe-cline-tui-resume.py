#!/usr/bin/env python3
"""Check whether Cline's TUI can take over, and give back, a Gaffer chat's conversation.

Gaffer | Cline TUI switching is not offered for Cline because nobody has checked
what `cline --tui --id <chat id>` does with an id that `cline --acp` created. The
catalogue row has no resume flag, so the controller refuses the launch before it
starts. This probe answers the open questions, with NO model call:

  1. does `cline --acp` create a session with the `<ms>_<5 chars>_cli` handle,
  2. does `cline history` list that id,
  3. does `cline --tui --auto-approve false --id <id>` start on it (run in a
     pseudo-terminal of the size a reet switch uses) or refuse it,
  4. does the TUI keep that id, or create a different one (a fork reet could not
     follow back to Gaffer: reet only accepts `_cli` handles), and
  5. does `session/load` still open the id afterwards (the TUI -> Gaffer trip).

It copies a Cline folder (default ~/.cline) to a private temporary directory and
points CLINE_DIR and HOME at the copy, so the real sign-in, settings and history
are never changed. It installs nothing and types nothing into the TUI.

Run it on a machine where Cline is signed in, then paste the JSON into
docs/research/cline-acp-probe.md.
"""
import argparse
import fcntl
import json
import os
import pty
import re
import selectors
import shlex
import shutil
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time
from pathlib import Path

# The terminal size a reet switch to the agent's TUI starts with
# (internal/terminalbroker/session_mode.go: switchCols, switchRows).
COLS, ROWS = 100, 30

# A Cline task id: milliseconds, an underscore, five characters, an optional "_cli".
TASK_ID = re.compile(r"\b\d{10,}_[A-Za-z0-9_-]{5}(?:_cli)?\b")
# The shape reet stores for a chat (internal/terminalproto: validClineSessionID).
CHAT_ID = re.compile(r"^\d{1,20}_[A-Za-z0-9_-]{5}_cli$")
ANSI = re.compile(r"\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-Za-z0-9]|\x1b[=>]")
REFUSAL = re.compile(r"not found|no such|unknown session|invalid (?:session|id)|cannot resume|could not (?:find|resume)|does not exist", re.I)


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
        stop(self.proc)


def stop(proc):
    """End a child and its whole process group."""
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        proc.wait(timeout=3)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        proc.wait()


def open_agent(argv, env, workdir):
    """Start `cline --acp` and initialize it."""
    agent = Agent(argv, env, workdir)
    agent.call("initialize", {
        "protocolVersion": 1,
        "clientInfo": {"name": "reet-probe", "version": "1"},
        "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False},
    })
    return agent


def run_history(cline, env, workdir):
    """`cline history` text and the task ids in it. It never starts a turn."""
    try:
        done = subprocess.run(cline + ["history"], cwd=workdir, env=env, capture_output=True, timeout=30, stdin=subprocess.DEVNULL)
        text = ANSI.sub("", (done.stdout + done.stderr).decode("utf-8", "replace"))
        return {"exit": done.returncode, "ids": sorted(set(TASK_ID.findall(text))), "text": text[-1500:]}
    except (OSError, subprocess.TimeoutExpired) as error:
        return {"exit": None, "ids": [], "text": "", "error": str(error)}


def run_tui(cline, env, workdir, session_id, seconds):
    """Start the TUI on session_id in a pseudo-terminal, type nothing, and watch it."""
    argv = cline + ["--tui", "--auto-approve", "false", "--id", session_id]
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
    tui_env = dict(env, TERM="xterm-256color", COLUMNS=str(COLS), LINES=str(ROWS))
    proc = subprocess.Popen(argv, cwd=workdir, env=tui_env, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    raw, started = b"", time.monotonic()
    sel = selectors.DefaultSelector()
    sel.register(master, selectors.EVENT_READ)
    try:
        while time.monotonic() - started < seconds and proc.poll() is None:
            for key, _ in sel.select(0.2):
                try:
                    data = os.read(key.fileobj, 65536)
                except OSError:
                    data = b""
                if data:
                    raw += data
        exited = proc.poll()
    finally:
        stop(proc)
        os.close(master)
    text = ANSI.sub("", raw.decode("utf-8", "replace"))
    return {
        "argv": argv[1:],
        "stillRunningAfterSeconds": exited is None,
        "exitCode": exited,
        "mentionsRefusal": bool(REFUSAL.search(text)),
        "mentionsSessionId": session_id in text,
        "outputBytes": len(raw),
        "screenTail": text[-1500:],
    }


def probe(cline, cline_dir, workdir, seconds):
    """Run the whole probe against a private copy of cline_dir and return the report."""
    home = Path(tempfile.mkdtemp(prefix="reet-cline-tui-probe-"))
    copy = home / "cline"
    shutil.copytree(cline_dir, copy, symlinks=False)
    env = {k: v for k, v in os.environ.items() if k in ("PATH", "LANG", "CLINE_API_KEY")}
    env.update({"HOME": str(home), "CLINE_DIR": str(copy)})
    acp = cline + ["--acp", "--auto-approve", "false"]
    report = {}
    agent = None
    try:
        agent = open_agent(acp, env, workdir)
        created = agent.call("session/new", {"cwd": str(workdir), "mcpServers": []})
        if "result" not in created:
            report["error"] = {"stage": "session/new", "detail": created.get("error")}
            return report
        chat = created["result"].get("sessionId") or ""
        report["chatId"] = chat
        report["chatIdHasReetShape"] = bool(CHAT_ID.match(chat))
        agent.close()
        agent = None

        before = run_history(cline, env, workdir)
        report["historyBefore"] = {"exit": before["exit"], "listsChatId": chat in before["ids"], "idCount": len(before["ids"]), "text": before["text"]}

        tui = run_tui(cline, env, workdir, chat, seconds)
        report["tui"] = tui

        after = run_history(cline, env, workdir)
        new_ids = [i for i in after["ids"] if i not in before["ids"]]
        report["historyAfter"] = {"exit": after["exit"], "listsChatId": chat in after["ids"], "newIds": new_ids}
        report["newIdsAreChatHandles"] = {i: bool(CHAT_ID.match(i)) for i in new_ids}

        agent = open_agent(acp, env, workdir)
        loaded = agent.call("session/load", {"sessionId": chat, "cwd": str(workdir), "mcpServers": []})
        report["loadAfterTui"] = {"ok": "result" in loaded, "error": loaded.get("error")}
        for new in new_ids:
            again = agent.call("session/load", {"sessionId": new, "cwd": str(workdir), "mcpServers": []})
            report.setdefault("loadNewIds", {})[new] = "result" in again
        report["verdict"] = verdict(report)
        return report
    finally:
        if agent is not None:
            agent.close()
        shutil.rmtree(home, ignore_errors=True)


def verdict(report):
    """Plain sentences about what the numbers above allow reet to build."""
    out = []
    tui = report.get("tui", {})
    if not report.get("chatIdHasReetShape"):
        out.append("The ACP session id does not match reet's Cline chat handle; fix validClineSessionID first.")
    if tui.get("exitCode") not in (None, 0) or tui.get("mentionsRefusal"):
        out.append("The TUI refused or failed on the chat id: do NOT add `--id` to Cline's catalogue row.")
    elif tui.get("stillRunningAfterSeconds"):
        out.append("The TUI started on the chat id and stayed up: `--id` is accepted.")
    if report.get("historyAfter", {}).get("newIds"):
        out.append("Starting the TUI created new task ids: it may fork instead of resuming. Compare screenTail with the conversation.")
    else:
        out.append("Starting the TUI created no new task id: it kept the chat's id.")
    if report.get("loadAfterTui", {}).get("ok"):
        out.append("session/load still opens the chat id after the TUI ran: the TUI -> Gaffer trip can work.")
    else:
        out.append("session/load no longer opens the chat id after the TUI ran: the TUI -> Gaffer trip would fail.")
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--cline", default="cline", help="the Cline command (default: cline)")
    parser.add_argument("--cline-dir", type=Path, default=Path.home() / ".cline", help="a signed-in Cline folder to copy (default: ~/.cline)")
    parser.add_argument("--tui-seconds", type=float, default=12.0, help="how long to watch the TUI before stopping it (default: 12)")
    args = parser.parse_args()
    if not args.cline_dir.is_dir():
        parser.error("no Cline folder at " + str(args.cline_dir))
    workdir = Path(tempfile.mkdtemp(prefix="reet-cline-tui-probe-project-"))
    try:
        report = probe(shlex.split(args.cline), args.cline_dir, workdir, args.tui_seconds)
    finally:
        shutil.rmtree(workdir, ignore_errors=True)
    print(json.dumps(report, indent=2, default=str))
    return 2 if "error" in report else 0


if __name__ == "__main__":
    sys.exit(main())
