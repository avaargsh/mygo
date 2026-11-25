#!/usr/bin/env python3
"""Drive the installed GTK application using AT-SPI labels and X11 input."""

import os
import subprocess
import sys
import time

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
                match = name in label if contains else name == label
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


def main():
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
