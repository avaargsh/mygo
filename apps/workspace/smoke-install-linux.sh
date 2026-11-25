#!/usr/bin/env bash
# CI acceptance only: exercise a local build in a disposable HOME.
set -euo pipefail
if [[ $# -lt 1 || $# -gt 2 || ( $# == 2 && $2 != --deb-installed ) ]]; then
  echo 'Usage: smoke-install-linux.sh DIST_DIRECTORY [--deb-installed]' >&2
  exit 2
fi
dist=$(cd "$1" && pwd -P)
mode=${2:-}
probe=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/probe-installed-search-linux.py
work=$(mktemp -d)
pid=''
wid=''
cleanup() {
  rc=$?
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    # A failed UI assertion can leave a dirty test window or dialog open.
    for ((i=0; i<20; i++)); do
      if ! kill -0 "$pid" 2>/dev/null; then break; fi
      sleep 0.1
    done
    if (( i == 20 )); then kill -KILL "$pid" 2>/dev/null || true; fi
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
archive=mygo-workspace-0.1.0-linux-amd64.tar.gz
if [[ "$mode" == --deb-installed ]]; then
  test "$(dpkg-query -W -f='${Status} ${Version}' mygo-workspace)" = 'install ok installed 0.1.0'
  launcher=/usr/bin/mygo-workspace
  app=/opt/mygo-workspace
  installed_notice=/usr/share/doc/mygo-workspace/copyright
  desktop=/usr/share/applications/mygo-workspace.desktop
else
  sh ./install.sh "./$archive"
  launcher="$HOME/.local/bin/mygo-workspace"
  app="$HOME/.local/mygo-workspace.app"
  installed_notice="$app/copyright"
  desktop="$XDG_DATA_HOME/applications/mygo-workspace.desktop"
fi
test -x "$launcher"
test -s "$app/libghostty-vt.so"
test -s "$installed_notice"
tar -xOzf "$archive" copyright | cmp - "$installed_notice"
test -s "$desktop"
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
find_close_dialog() {
  local id
  while IFS= read -r id; do
    if [[ "$id" =~ ^[0-9]+$ && "$id" != "$wid" ]]; then
      printf '%s\n' "$id"
      return 0
    fi
  done < <(xdotool search --onlyvisible --name '^MyGo Workspace$')
  return 1
}
cancel_dirty_close() {
  local dialog='' i
  xdotool key --clearmodifiers ctrl+w
  for ((i=0; i<100; i++)); do
    kill -0 "$pid" || { echo 'Workspace exited without protecting changes' >&2; return 1; }
    if dialog=$(find_close_dialog); then break; fi
    sleep 0.2
  done
  [[ -n "$dialog" ]] || { echo 'Unsaved-change dialog did not appear' >&2; return 1; }
  timeout 5s xdotool windowfocus --sync "$dialog"
  xdotool key --clearmodifiers Escape
  for ((i=0; i<100; i++)); do
    kill -0 "$pid" || { echo 'Cancel did not keep the Workspace open' >&2; return 1; }
    if ! find_close_dialog >/dev/null; then
      timeout 5s xdotool windowfocus --sync "$wid"
      [[ ! -e "$state" ]] || { echo 'Cancel unexpectedly wrote workspace state' >&2; return 1; }
      echo 'PASS: canceling an unsaved close keeps the installed Workspace open without writing state'
      return
    fi
    sleep 0.2
  done
  echo 'Unsaved-change dialog did not close after Escape' >&2
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
cancel_dirty_close
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
if [[ -n "${MYGO_XERJ_APP_LIVE_INDEX:-}" ]]; then
  /usr/bin/python3 "$probe"
  # Opening a Search tab changes layout. Save it explicitly before shutdown.
  rm -- "$state"
  xdotool key --clearmodifiers ctrl+s
  wait_for_state
fi
close_app
echo 'PASS: installed save, graceful close, restart and restored layout'
if [[ "$mode" == --deb-installed ]]; then
  echo 'PASS: apt-installed app startup and installed copyright'
  exit 0
fi
printf 'retained data\n' > "$XDG_CONFIG_HOME/MyGoWorkspace/acceptance-marker"
sh ./install.sh --uninstall
test ! -e "$launcher"
test ! -L "$launcher"
grep -Fx 'retained data' "$XDG_CONFIG_HOME/MyGoWorkspace/acceptance-marker"
echo 'PASS: user install, visible startup, uninstall, data preservation'
