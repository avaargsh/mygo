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
"$launcher" >"$work/app.log" 2>&1 &
pid=$!
# A visible window, not merely a living process, is required for startup.
for ((i=0; i<100; i++)); do
  if ! kill -0 "$pid" 2>/dev/null; then
    echo 'Installed process exited before startup' >&2
    exit 1
  fi
  if xdotool search --onlyvisible --name '^MyGo Workspace$' >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done
xdotool search --onlyvisible --name '^MyGo Workspace$' >/dev/null
test -d "$XDG_CONFIG_HOME/MyGoWorkspace"
# This is a startup probe, not a graceful-quit or persistence test.
kill "$pid"
wait "$pid" 2>/dev/null || true
pid=''
printf 'retained data\n' > "$XDG_CONFIG_HOME/MyGoWorkspace/acceptance-marker"
sh ./install.sh --uninstall
test ! -e "$launcher"
test ! -L "$launcher"
grep -Fx 'retained data' "$XDG_CONFIG_HOME/MyGoWorkspace/acceptance-marker"
echo 'PASS: user install, visible startup, uninstall, data preservation'
