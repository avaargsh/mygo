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
open Home or Runtime info in it, resize by dragging the native divider (double-click resets to 50%), move
an active tab to the next pane, then close an empty pane. Repeat with nested
splits. Verify that moving a tab retains its session reference and closing a
nonempty pane is disabled. The runtime page displays real Go runtime facts,
not sampled CPU utilization or a simulated system monitor.

Choose Light, Dark or System, click Save workspace, quit and reopen.
The saved layout, pane focus, tab order, active tabs and theme should return.
Unsaved changes are explicitly marked; save before quitting. New IDs avoid
collisions with restored IDs. The app writes state.json under MyGoWorkspace
in os.UserConfigDir(). A corrupt or unsupported file stops startup without
overwriting it. To recover manually, back up the file and move it aside.

To check the native view without a desktop window:
    go test ./apps/workspace/cmd/mygo -run TestWorkspaceShellSplitMoveSave

To probe two real embedded WebViews (does not touch workspace state):
    go run ./apps/workspace/cmd/mygo --webview-smoke

On a graphical Linux/macOS/Windows host, type different text into each page,
drag the divider, hide the second page and restore it; confirm the two pages
retain their own values and keyboard focus works. These are manual acceptance
steps, not verified desktop test results. The probe uses local HTML and
untrusted page options; it does not establish browser storage isolation.

## Architecture and status

- core owns tab identity, ordering and active selection; a tab is a view
  reference, not an owner of a content session.
- cmd/mygo is the native UI composition root. Only the UI thread mutates
  the workspace. The workspace owns the layout and panes; layout leaves and
  pane identities must match. Tab IDs are unique across the workspace. Background services must later marshal commands to that owner.
- The content manager registers Home, Runtime info and Notes as built-in
  singleton sessions. Closing a tab leaves the session alive until process
  exit; multiple views share one session. Unknown providers are rejected and
  failed opens can be retried. Multiple instances and resourceful session
  shutdown/suspension contracts are deferred until Browser/Terminal work.
- Versioned workspace snapshots round-trip layout, panes, tabs and selection;
  invalid or unknown versions are rejected. The desktop state uses validated
  versioned JSON, a 4 MiB cap and same-directory temp-file replacement;
  explicit Save avoids synchronous disk writes on each resize frame.
  This is a single-workspace bootstrap store, not SQLite or a concurrent
  multi-instance store. Power-loss durability across platforms is unverified.
  The shell uses native split handles and moves tabs to the next pane in tree order.
- Workspace-only theme tokens use native light/dark/system appearance, theme
  surface/border colors, shared spacing, radius and a green accent.
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

Local evidence: 17 core tests passed (13 existing and 4 new storage tests),
including race detection and go vet using Go 1.23.2 in an isolated
standard-library-only directory. The new storage tests failed compilation
before implementation (RED), and passed after (GREEN). The remote storage
file blob hashes match the tested files. Tests covered disk round trip,
replacement, invalid state, oversize input and write-failure cleanup.

Full Go 1.27.3 repository builds, the native view test, existing CI,
actual desktop startup, packaging and the dual-WebView probe remain
unverified: the local toolchain is Go 1.23.2 and cannot resolve GitHub.
No native UI or cross-platform crash durability pass is claimed.

## Notes (M2 first slice)

Open Notes from the toolbar, type Markdown source, and click Save note.
The note is stored as UTF-8 in notes.md next to state.json (1 MiB maximum).
Layout saving and note saving are independent. Closing and reopening the
Notes tab in the same process retains unsaved edits; the status area offers
Save note changes even when its tab is closed. Save before exiting: there is
no quit confirmation or automatic draft recovery yet. On restart saved
notes are read when their session is opened or restored. Invalid note data
is rejected rather than overwritten by opening. This is one local note,
not a full note library or conflict-aware multi-instance editor.

## M2 validation

On Go 1.23.2 the isolated, standard-library-only core passed 21 tests
(17 existing and 4 new), race detection and vet. New tests initially
failed compilation before the implementation. The two new core file
blob hashes match the locally tested files. Native shell integration and
its tests were committed but have not been compiled or executed locally.
The root build still needs Go 1.27.3+ and a reachable dependency source.
