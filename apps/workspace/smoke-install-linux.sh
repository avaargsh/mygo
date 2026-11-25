#!/usr/bin/env bash
# CI acceptance only: install the locally built preview into a disposable HOME.
set -euo pipefail
if [[ $# -ne 1 ]]; then
  echo 'Usage: smoke-install-linux.sh DIST_DIRECTORY' >&2
  exit 2
fi
dist=$(cd "$1" && pwd -P)
work=$(mktemp -d)
pid=''
cleanup() {
  rc=$?
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  if [[ $rc != 0 && -f "$work/app.log" ]]; then
    cat "$work/app.log" >&2
  fi
  rm -rf -- "$work"
  exit "$rc"
}
trap cleanup EXIT
export HOME="$work/home"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_CACHE_HOME="$HOME/.cache"
mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"
cd "$dist"
sha256sum -c SHA256SUMS
sh ./install.sh ./mygo-workspace-0.1.0-linux-amd64.tar.gz
launcher="$HOME/.local/bin/mygo-workspace"
test -x "$launcher"
test -s "$HOME/.local/mygo-workspace.app/libghostty-vt.so"
test -s "$XDG_DATA_HOME/applications/mygo-workspace.desktop"
start_app() {
  "$launcher" >>"$work/app.log" 2>&1 &
  pid=$!
  for ((i=0; i<100; i++)); do
    kill -0 "$pid" || return 1
    if wid=$(xdotool search --onlyvisible --name '^MyGo Workspace$' | head -n 1) && [[ -n "$wid" ]]; then
      timeout 5s xdotool windowfocus --sync "$wid"
      return
    fi
    sleep 0.2
  done
  return 1
}
close_app() {
  xdotool key --clearmodifiers ctrl+w
  for ((i=0; i<100; i++)); do
    if ! kill -0 "$pid" 2>/dev/null; then
      wait "$pid"
      pid=''
      return
    fi
    sleep 0.2
  done
  echo 'Workspace did not close normally' >&2
  return 1
}
wait_for_state() {
  for ((i=0; i<100; i++)); do
    [[ ! -s "$state" ]] || return 0
    sleep 0.2
  done
  return 1
}
state="$XDG_CONFIG_HOME/MyGoWorkspace/state.json"
start_app
xdotool key --clearmodifiers ctrl+shift+e
xdotool key --clearmodifiers ctrl+s
wait_for_state
python3 - "$state" <<'PY'
import json, sys
with open(sys.argv[1]) as f:
    s = json.load(f)
w = s["workspace"]
assert s["version"] == 1 and len(w["panes"]) == 2, s
assert w["layout"]["direction"] == "horizontal", s
assert len(w["layout"]["children"]) == 2, s
assert w["activePane"] in w["panes"], s
PY
cp "$state" "$work/expected.json"
close_app
start_app
# Prove a save recreated the state from the restored in-memory layout.
rm -- "$state"
xdotool key --clearmodifiers ctrl+s
wait_for_state
cmp "$work/expected.json" "$state"
close_app
echo 'PASS: installed save, graceful close, restart and restored layout'
printf 'retained data\n' > "$XDG_CONFIG_HOME/MyGoWorkspace/acceptance-marker"
sh ./install.sh --uninstall
test ! -e "$launcher"
test ! -L "$launcher"
grep -Fx 'retained data' "$XDG_CONFIG_HOME/MyGoWorkspace/acceptance-marker"
echo 'PASS: user install, visible startup, uninstall, data preservation'
