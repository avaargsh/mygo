#!/usr/bin/env bash
# Opt-in CI acceptance only. No installer, system changes, or user data.
set -euo pipefail
out=$(mktemp -d)
pid=
cleanup() {
  local rc=$?
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  if [[ $rc != 0 && -f "$out/server.log" ]]; then
    tail -100 "$out/server.log" >&2
  fi
  rm -rf -- "$out"
}
trap cleanup EXIT
export GOMODCACHE="$(go env GOMODCACHE)" GOCACHE="$(go env GOCACHE)"
export HOME="$out/home" XDG_CONFIG_HOME="$out/config" XDG_STATE_HOME="$out/state" XDG_DATA_HOME="$out/share"
export XERJ_DISABLE_FEEDBACK=true
mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_STATE_HOME" "$XDG_DATA_HOME" "$out/corpus"
asset=xerj-1.0.0-rc.103-x86_64-unknown-linux-musl.tar.gz
archive="$out/$asset"
curl -fLSs --max-time 120 "https://github.com/xerj-org/xerj/releases/download/v1.0.0-rc.103/$asset" -o "$archive"
echo "cfb2b680ef0f8528c873a5056d4e1e6842b0a53b30d3ee5282dc7cb6d9dbe78b  $archive" | sha256sum -c -
entry=$(tar -tzf "$archive" | grep -Ex '(\./)?([^/]+/)?xerj')
tar -xOzf "$archive" "$entry" > "$out/xerj"
chmod 700 "$out/xerj"
"$out/xerj" --version
endpoint=http://127.0.0.1:9200
if curl -s --max-time 1 "$endpoint/" >/dev/null; then
  echo 'Fixture needs an unused loopback port 9200' >&2
  exit 1
fi
printf '# Queue operations\n\nAsterquill is the synthetic runbook verification marker.\n' >"$out/corpus/guide.md"
(
  cd "$out"
  exec "$out/xerj" --insecure --bind 127.0.0.1 --data-dir "$out/data"
) > "$out/server.log" 2>&1 &
pid=$!
for ((i=0; i<60; i++)); do
  kill -0 "$pid"
  if curl -fsS --max-time 1 "$endpoint/" >/dev/null; then
    break
  fi
  sleep 1
done
test "$i" -lt 60
test "$(ss -Hltn 'sport = :9200' | awk '{print $4}')" = '127.0.0.1:9200'
"$out/xerj" autoindex "$out/corpus" --url "$endpoint" --prefix mygofixture --state-dir "$out/autoindex" --workers 1 --no-semantic --progress none --disable-feedback
index=$(curl -fsS "$endpoint/_cat/indices?format=json&h=index" | python3 -c '
import json, re, sys
rows = [row["index"] for row in json.load(sys.stdin) if re.fullmatch(r"(?:ax-)?mygofixture(?:-[a-z0-9_-]+)?", row["index"])]
assert len(rows) == 1, rows
print(rows[0])
')
echo "Live fixture index: $index"
export MYGO_XERJ_APP_LIVE_ENDPOINT="$endpoint" MYGO_XERJ_APP_LIVE_INDEX="$index"
if [[ $# -gt 0 ]]; then
  "$@"
else
  go test -v -count=1 -timeout=2m -run '^TestWorkspaceLocalSearchLive$' ./apps/workspace/cmd/mygo
fi
