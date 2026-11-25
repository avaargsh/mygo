# MyGo Workspace

Personal AI Workspace — Universal Content, Local-first, Agent-native, MCP-native.

This is an independent product layer within the existing MyGo Go module.
It does not change framework APIs or require a second module.

## Run from the repository root

Requires the toolchain specified in root go.mod (Go 1.27.3 at the
foundation baseline) and the platform prerequisites in docs/getting-started.md.

    go test ./apps/workspace/core
    go test -race ./apps/workspace/core
    go build -o workspace-dev ./apps/workspace/cmd/mygo
    go run ./apps/workspace/cmd/mygo

On a graphical desktop, open Runtime info, split right or down, focus a pane,
open Home or Runtime info in it, resize with the split's +/- buttons, move
an active tab to the next pane, then close an empty pane. Repeat with nested
splits. Verify that moving a tab retains its session reference and closing a
nonempty pane is disabled. The runtime page displays real Go runtime facts,
not sampled CPU utilization or a simulated system monitor.

These are desktop smoke-test steps, not verified desktop test results.

## Architecture and status

- core owns tab identity, ordering and active selection; a tab is a view
  reference, not an owner of a content session.
- cmd/mygo is the native UI composition root. Only the UI thread mutates
  the workspace. The workspace owns the layout and panes; layout leaves and
  pane identities must match. Tab IDs are unique across the workspace. Background services must later marshal commands to that owner.
- The current two built-in views are static session references; there is no
  general session manager, persistent storage or background task lifecycle yet.
- Versioned workspace snapshots round-trip layout, panes, tabs and selection;
  invalid or unknown versions are rejected. Disk storage and restart recovery
  are not implemented yet. The shell currently uses buttons rather than drag
  handles, and moves tabs to the next pane in tree order.
- Reuse framework native UI, embedded WebViews, terminal and SQLite where
  validated. Do not fork the rendering or window layer.

## Roadmap and release gates

M0: standalone entry, real desktop smoke test, framework capability matrix.
Verify multiple independently managed embedded WebViews on the target
platform, native/WebView focus, clipboard and layout, and existing CI.
M1: themes, workspace identity, recursive split tree, tab operations,
command palette, layout persistence and recovery.
M2: Browser, Terminal, Notes, Files and System Info, with independent
session lifecycle and real data.
M3: model streaming, MCP, scoped context, policy and approvals, artifacts
and run journal. M4 onward: system and application management, advanced
content, extensions and cross-platform release.

No privileged IPC, shell automation, remote listeners, external telemetry,
or model/tool calls are introduced in M0. Later external content and tool
integrations must be isolated from privileged application APIs, default deny
access, and record approvals and actual execution evidence.

Local evidence: 13 core tests passed, including 5 new workspace tests and
8 prior regression tests, with race detection and go vet using Go 1.23.2 in
an isolated standard-library-only directory. The new tests failed to compile
before workspace implementation (RED), and passed after (GREEN).
The committed workspace core files match the locally tested blob hashes. This does not
prove full repository compatibility. Full Go 1.27.3 builds, existing CI,
actual desktop startup, packaging and multiple-WebView behavior remain
unverified until run in a compatible environment.
