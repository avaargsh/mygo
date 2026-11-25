#!/usr/bin/env python3
"""Drive the installed GTK application using AT-SPI labels and X11 input."""

import json
import os
import subprocess
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Event, Thread

import pyatspi


def walk(node):
    yield node
    try:
        for i in range(node.childCount):
            child = node.getChildAtIndex(i)
            if child is not None:
                yield from walk(child)
    except (LookupError, RuntimeError):
        return


def find(name, *, contains=False, component=False):
    deadline = time.monotonic() + 12
    observed = []
    while time.monotonic() < deadline:
        observed = []
        for node in walk(pyatspi.Registry.getDesktop(0)):
            try:
                label = node.name or ""
                observed.append(label)
                match = name.casefold() in label.casefold() if contains else name == label
                if not match:
                    continue
                if not component:
                    return node
                bounds = node.queryComponent().getExtents(pyatspi.DESKTOP_COORDS)
                if bounds.width > 0 and bounds.height > 0:
                    return bounds
            except (LookupError, NotImplementedError, RuntimeError):
                continue
        time.sleep(0.15)
    raise RuntimeError(f"Missing on-screen {name!r}; labels: {sorted(set(observed))[:100]}")


def xdotool(*args):
    subprocess.run(["xdotool", *args], check=True, timeout=5)


def click(label):
    bounds = find(label, component=True)
    xdotool("mousemove", str(bounds.x + bounds.width // 2), str(bounds.y + bounds.height // 2), "click", "1")


def enter(label, value):
    click(label)
    xdotool("key", "--clearmodifiers", "ctrl+a")
    xdotool("type", "--clearmodifiers", "--delay", "5", value)
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        actual = find(label).queryText().getText(0, -1)
        if actual == value:
            return
        time.sleep(0.05)
    raise RuntimeError(f"Input {label!r} value was {actual!r}, expected {value!r}")


NOTE = "Workspace note acceptance marker"
QUESTION = "What is the synthetic Workspace marker?"
ANSWER = "The synthetic Workspace marker is blue."

def exercise_workspace():
    page_requested, model_requested = Event(), Event()

    class Fixture(BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path != "/workspace-probe":
                self.send_error(404)
                return
            page = b"<!doctype html><title>Workspace fixture</title><p>Local web page</p>"
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(page)))
            self.end_headers()
            self.wfile.write(page)
            page_requested.set()

        def do_POST(self):
            if self.path != "/v1/chat/completions":
                self.send_error(404)
                return
            request = json.loads(
                self.rfile.read(int(self.headers["Content-Length"]))
            )
            if (
                request.get("model") != "workspace-fixture"
                or request.get("stream") is not True
                or request.get("messages") != [{"role": "user", "content": QUESTION}]
                or self.headers.get("Authorization")
            ):
                self.send_error(400)
                return
            events = [
                {"choices": [{"index": 0, "delta": {"role": "assistant", "content": ANSWER}}]},
                {"choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}]},
            ]
            body = ("".join(f"data: {json.dumps(e)}\n\n" for e in events) + "data: [DONE]\n\n").encode()
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            model_requested.set()

    with ThreadingHTTPServer(("127.0.0.1", 0), Fixture) as server:
        worker = Thread(target=server.serve_forever, daemon=True)
        worker.start()
        origin = f"http://127.0.0.1:{server.server_port}"
        try:
            divider = find("divider", contains=True, component=True)
            x, y = divider.x + divider.width // 2, divider.y + divider.height // 2
            xdotool("mousemove", str(x), str(y), "mousedown", "1", "mousemove", str(x + 70), str(y), "sleep", "0.2", "mouseup", "1")
            click("Browser")
            enter("Browser address", origin + "/workspace-probe")
            click("Go")
            if not page_requested.wait(10):
                raise RuntimeError("Browser tab did not request the loopback web page")
            print("PASS: installed Browser loaded the loopback fixture page")

            click("Terminal")
            click("Start shell")
            find("Terminal: running")
            print("PASS: installed Terminal started a local shell")

            click("Notes")
            enter("Markdown source", NOTE)
            click("Save note")
            find("Note saved")
            note = os.path.join(os.environ["XDG_CONFIG_HOME"], "MyGoWorkspace", "notes.md")
            with open(note, encoding="utf-8") as file:
                if file.read().strip() != NOTE:
                    raise RuntimeError("Saved note did not match typed text")
            print("PASS: installed Notes saved edited Markdown")

            click("Chat")
            enter("Chat completions endpoint", origin + "/v1/chat/completions")
            enter("Model name", "workspace-fixture")
            enter("Message", QUESTION)
            send = find("Send message")
            if not send.getState().contains(pyatspi.STATE_ENABLED):
                raise RuntimeError("Send message stayed disabled after text entry")
            bounds = send.queryComponent().getExtents(pyatspi.DESKTOP_COORDS)
            window = find("MyGo Workspace", component=True)
            if not (
                window.x <= bounds.x < window.x + window.width
                and window.y <= bounds.y
                and bounds.y + bounds.height <= window.y + window.height
            ):
                raise RuntimeError(f"Send message is clipped: control {bounds}, window {window}")
            click("Send message")
            if not model_requested.wait(10):
                labels = sorted({node.name for node in walk(pyatspi.Registry.getDesktop(0)) if node.name})
                raise RuntimeError(f"Chat did not reach the local model fixture. Labels: {labels[:100]}")
            find(ANSWER, contains=True)
            find("Finish: stop")
            print("PASS: installed Chat rendered and completed a streamed response without credentials")

            click("Move tab to next pane")
            click("Close Home")
            click("Search")
            find("Search local documents")
        finally:
            server.shutdown()
            worker.join(timeout=5)


def check_restored_workspace():
    click("Notes")
    value = find("Markdown source").queryText().getText(0, -1)
    if value.strip() != NOTE:
        raise RuntimeError("Saved note did not reopen with its contents")
    click("Terminal")
    find("Start shell to open a local interactive terminal.")
    click("Search")
    find("Search local documents")
    print("PASS: saved note restored and restored Terminal remained idle")

def main():
    if sys.argv[1:] == ["--workspace"]:
        exercise_workspace()
        return
    if sys.argv[1:] == ["--check-workspace"]:
        check_restored_workspace()
        return
    if sys.argv[1:] == ["--check-idle"]:
        find("Search local documents")
        for label in ("Exact index name", "Search text"):
            value = find(label).queryText().getText(0, -1)
            if value:
                raise RuntimeError(f"Restart restored private {label}")
        if find("Find passages").getState().contains(pyatspi.STATE_ENABLED):
            raise RuntimeError("Restored Search should require fresh input")
        print("PASS: installed Search tab reopened idle without saved index or query")
        return

    index = os.environ["MYGO_XERJ_APP_LIVE_INDEX"]
    click("Search")
    find("Search local documents")
    enter("Exact index name", index)
    enter("Search text", "asterquill")
    click("Find passages")
    find('"guide.md":1', contains=True)
    find("synthetic runbook verification marker", contains=True)
    print(f"PASS: installed GTK Search returned the real XERJ excerpt and guide.md:1 from {index}")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, subprocess.SubprocessError) as error:
        sys.exit(str(error))
