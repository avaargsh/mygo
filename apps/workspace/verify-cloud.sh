#!/usr/bin/env bash
# Run from a checked-out MyGo repository on the authorized cloud development host.
set -euo pipefail
repo=$(git rev-parse --show-toplevel)
cd "$repo"
if [[ ! -f apps/workspace/core/workspace.go || ! -f go.mod ]]; then
  echo 'Not a MyGo Workspace checkout' >&2
  exit 2
fi
out=$(mktemp -d "${TMPDIR:-/tmp}/mygo-verify.XXXXXXXX")
echo "Evidence directory: $out"
exec > >(tee "$out/verify.log") 2>&1
trap 'rc=$?; echo "Verification exit=$rc; evidence=$out"' EXIT
export GOTOOLCHAIN=local
printf 'Commit: '; git rev-parse HEAD
printf 'Toolchain: '; go version
printf 'Module minimum: '; awk '$1=="go" {print $2; exit}' go.mod
if [[ -n $(git status --porcelain --untracked-files=no) ]]; then
  echo 'WARNING: tracked checkout is modified; evidence is not attributable solely to HEAD.'
fi
# The module-aware commands enforce the minimum Go version in go.mod.
go mod download
go mod verify
go test -count=1 ./apps/workspace/...
go test -race -count=1 ./apps/workspace/core
go vet ./apps/workspace/...
go build ./...
go build -o "$out/mygo-workspace" ./apps/workspace/cmd/mygo
if [[ ${MYGO_FULL_TEST:-0} == 1 ]]; then
  go test -count=1 ./...
fi
printf 'Built binary: %s\n' "$out/mygo-workspace"
echo 'PASS: build and selected automated checks. Native desktop/WebView/PTY smoke tests are NOT included.'
