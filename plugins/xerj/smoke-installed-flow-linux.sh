#!/usr/bin/env bash
# Disposable synthetic-corpus acceptance only; never use an existing node.
set -euo pipefail
out=$(mktemp -d)
pid=
cleanup() {
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf -- "$out"
}
trap cleanup EXIT
export GOMODCACHE="$(go env GOMODCACHE)" GOCACHE="$(go env GOCACHE)"
export HOME="$out/home" XDG_CONFIG_HOME="$out/config" XDG_STATE_HOME="$out/state" XDG_DATA_HOME="$out/share"
export XERJ_DISABLE_FEEDBACK=true
mkdir -p "$HOME" "$XDG_CONFIG_HOME" "$XDG_STATE_HOME" "$XDG_DATA_HOME" "$out/fixture"
asset=xerj-1.0.0-rc.103-x86_64-unknown-linux-musl.tar.gz
archive="$out/$asset"
curl -fLSs --max-time 120 "https://github.com/xerj-org/xerj/releases/download/v1.0.0-rc.103/$asset" -o "$archive"
echo "cfb2b680ef0f8528c873a5056d4e1e6842b0a53b30d3ee5282dc7cb6d9dbe78b  $archive" | sha256sum -c -
entry=$(tar -tzf "$archive" | grep -Ex '(\./)?([^/]+/)?xerj')
tar -xOzf "$archive" "$entry" > "$out/xerj"
chmod 700 "$out/xerj"
"$out/xerj" --version
digest=$(sha256sum "$out/xerj" | cut -d ' ' -f1)
endpoint=http://127.0.0.1:9200
test -z "$(ss -Hltn 'sport = :9200')"
(
  cd "$out"
  exec "$out/xerj" --insecure --bind 127.0.0.1 --data-dir "$out/data"
) >"$out/server.log" 2>&1 &
pid=$!
for ((i=0; i<60; i++)); do
  kill -0 "$pid"
  if curl -fsS --max-time 1 "$endpoint/" >/dev/null 2>&1; then break; fi
  sleep 1
done
test "$i" -lt 60
test "$(ss -Hltn 'sport = :9200' | awk '{print $4}')" = '127.0.0.1:9200'
# The expected suffix is a fixed hypothesis from the reviewed one-document
# Search fixture, not a name dynamically authorized by the test.
# Verify it separately with the actual lifecycle corpus shape; fail closed
# if this pinned version uses a different naming contract.
printf 'mygoinstalledalpha\n' >"$out/fixture/mygo-xerj-fixture.txt"
"$out/xerj" autoindex "$out/fixture" --url "$endpoint" --prefix mygoslugprobe --state-dir "$out/probe-state" --workers 1 --no-semantic --progress none --disable-feedback
curl -fsS "$endpoint/_cat/indices?format=json&h=index" | python3 -c '
import json, sys
names = [r["index"] for r in json.load(sys.stdin) if "mygoslugprobe" in r["index"]]
assert names == ["mygoslugprobe-docs"], names
'
echo "Verified independent fixture suffix: docs"
# Inspect only this known synthetic document, never any user index.
# The request mirrors TextIndexVerifier; do not change its checks.
curl -fsS --max-time 5 -H 'Content-Type: application/json' \
  -X POST "$endpoint/mygoslugprobe-docs/_search" \
  --data '{"size":1,"track_total_hits":true,"query":{"match_all":{}},"_source":["ax_path","body"]}' \
  | python3 -c 'import json,sys; print("Synthetic verifier response:",json.dumps(json.load(sys.stdin),ensure_ascii=True))'

# Compare bounded read-only shapes against the same known synthetic fixture.
# No query is used to authorize publication; the original verifier still runs.
for payload in '{"size":8,"track_total_hits":true,"query":{"match_all":{}}}' '{"size":8,"track_total_hits":true,"query":{"match_all":{}},"fields":["_passage","body"]}' '{"size":8,"track_total_hits":true,"query":{"match":{"body":"mygoinstalledalpha"}},"fields":["_passage"],"_source":true}'; do
  printf 'Synthetic query: %s\n' "$payload"
  curl -fsS --max-time 5 -H 'Content-Type: application/json' -X POST "$endpoint/mygoslugprobe-docs/_search" --data "$payload" | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin),ensure_ascii=True))'
done
printf 'Synthetic count response: '
curl -sS --max-time 5 -H 'Content-Type: application/json' -X POST "$endpoint/mygoslugprobe-docs/_count" --data '{"query":{"match_all":{}}}' | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin),ensure_ascii=True))'
MYGO_XERJ_ACCEPTANCE=1 MYGO_XERJ_NORMALIZED_PROBE=1 MYGO_XERJ_ENDPOINT="$endpoint" MYGO_XERJ_BINARY="$out/xerj" MYGO_XERJ_SHA256="$digest" MYGO_XERJ_DATASET_SUFFIX=docs \
  go test -race -count=1 -timeout=10m -run '^Test(LiveInstalledFlow|LiveInstalledNormalizedBytesRejected|VerifierRC103FileAndSection) -v ./plugins/xerj
 -v ./plugins/xerj
