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
- The content manager registers Home, Runtime info, Notes, Browser, Terminal and Files as built-in
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

## Browser (M2 second slice)

Open Browser from the toolbar. Enter an explicit HTTP or HTTPS address and
choose Go (or submit the address field). A native WebView is created only
on a successful explicit navigation request; restored tabs do not start
network requests. Back, Forward, Reload and Use current URL are available.
The address field is an editable request address, not a continuously
synchronized navigation indicator.

The singleton browser session keeps its native resource when tab views
close or move. Only one visible pane mounts the WebView; another view can
take ownership using Show browser here. Close browser page releases the
native resource; when no browser tab is visible, Close hidden browser page
provides the same action. The next Go creates a new view. Browser addresses,
history and page state are not persisted by the workspace.

All browser pages are untrusted, with no application service bindings.
The app rejects non-HTTP(S) navigation requests and retains the framework's
default denial of permissions, downloads and popups for untrusted pages.
This is not a network sandbox, an incognito browser, or a guarantee of
cookie isolation. URL validation does not restrict subresources or private
network access. Actual platform behavior must be verified.

Manual desktop acceptance: launch Workspace, open Browser, navigate to a
user-chosen test page, exercise back/forward/reload and current URL, split
the window and open Browser in both panes. Verify only one WebView is
mounted, move focus and confirm retained state, close the visible tab and
reopen it, then release the hidden resource. Save layout, restart and
confirm no page is loaded automatically. Verify a file URL is rejected and
permission/download/popup behavior is denied. No desktop pass is claimed.

Local evidence: 24 isolated core tests passed (3 new tests RED to GREEN),
race detection and vet passed, using Go 1.23.2 with GO111MODULE=off. The
two browser core files match the remotely committed Git blob hashes.
Native integration and headless shell acceptance test are unexecuted
because GitHub DNS and the required full Go 1.27.3+ toolchain remain
unavailable locally.

## Terminal (M2 third slice)

Open Terminal and explicitly choose Start shell. The existing MyGo terminal
plugin starts the user's local default shell with Ghostty/libghostty-vt.
No shell starts from a restored tab, nor is any command automatically typed.
The lifecycle owner distinguishes idle, active (running or launching),
stopping, and exited. Exit code is available only after the plugin's Done
signal. Natural exit keeps output until released; Close terminal requests
teardown and refuses a replacement until Done. Closing a tab leaves the
session active, and Close hidden terminal releases a hidden session.
Only one pane mounts the singleton terminal; window closure requests cleanup.
Program-initiated clipboard reads and writes are denied in the plugin options.
The user's interactive shell runs with the user's normal privileges; it is
not an agent command API or a sandbox.

On a compatible graphical host, prepare the native terminal library as in
docs/plugins/terminal.md (the MyGo CLI packages native libraries). Run the
repository build and tests, then launch the workspace and open Terminal.
Explicitly start the shell, type a harmless command, switch/split/move views,
reopen a closed tab, exit the shell, inspect the exit code, restart, close
the hidden terminal, and confirm that the process exits. Save the workspace,
restart and verify the restored terminal remains idle. Check the native
library loading error path. Descendant process cleanup and platform-specific
PTY behavior are not yet verified.

Actual local evidence: Go 1.23.2, isolated Core 27/27 tests (3 new lifecycle
tests RED to GREEN), race and vet passed. New terminal core file hashes
match the committed blobs. The lifecycle tests use a fake resource with a
controllable Done signal; they do not execute a native shell. The desktop
integration test, full module build and native Ghostty/PTTY smoke remain
pending. The user's updated host is not connected to this execution container.

## Files (M2 fourth slice)

Open Files, choose a folder using the native picker, and browse its directory
entries. Parent and Refresh are supported. The production adapter uses Go
os.OpenRoot and Root.FS for traversal containment; the core validates single
directory names and refuses non-directory entries. Root grants are memory-only
and can be revoked even when their view is hidden. Restoring a Files tab does
not access any directory. Bounded UTF-8 text preview is available; editing, upload, recursive scan
and deletion are not implemented. Directory reads are synchronous; the
5000-entry display limit is checked after enumeration. Large or slow
filesystems still need a bounded asynchronous listing implementation.

Core tests: 30/30 passed in the isolated Go 1.23.2 container, including 3 new
RED-to-GREEN file navigation tests, race detection and vet. The remote files.go
and files_test.go hashes match the locally tested source. Native root grant
test and complete UI compilation require the module's Go 1.27.3+ toolchain
and have not been executed here.

## Cloud development host verification

From a clean checkout of the exact PR commit in the already configured
Go 1.27.4 cloud-host environment, run:

    bash apps/workspace/verify-cloud.sh

This records the HEAD and Go version, verifies dependencies, executes
Workspace tests and Core/system-adapter race tests, runs vet, builds the entire repository
cross-compiles the system adapter for macOS and Windows, and emits the desktop binary and log in a temporary evidence directory.
Set MYGO_FULL_TEST=1 to additionally run the entire repository test suite.
No package install, checkout reset or automatic desktop launch occurs.
The script is syntax-checked but not yet executed on the updated cloud host
from this conversation. Real display/WebView/PTY smoke tests remain separate.

## System Info (M2 fifth slice)

Open System Info and explicitly refresh to sample the local host. Linux currently
uses procfs CPU counters, MemTotal/MemAvailable, /proc/net/dev interface byte
counters (loopback excluded), and statfs for the root filesystem. CPU utilization
is derived from two monotonic counter samples; the first sample intentionally
shows Sampling rather than a fabricated 0%. Memory uses MemAvailable and is
reported unavailable if the kernel does not expose it. Disk uses unprivileged
available blocks. Network values are cumulative byte counters, not rates.
Refresh is user initiated; there is no background polling.

Each metric is optional. Missing, malformed, overflowing or unsupported data
is surfaced as Unavailable, never replaced with zero. The Linux provider is
implemented and fixture-tested. macOS and Windows currently compile through an
explicit unsupported adapter and report all metrics unavailable; native
providers for those platforms remain future work. GPU metrics are not part of
this provider.

Local isolated evidence: 4 core system tests and 6 Linux provider tests passed,
including race detection and vet, on Go 1.23.2. These checks used a temporary
local compatibility shim for the x/sys Statfs API and do not replace the
repository's full Go 1.27.3+ build. The exact Linux provider integration,
native UI build and updated cloud-host verification remain pending.

## Files text preview (M2 sixth slice)

Files now marks regular files and symlinks separately. A selected regular file
can be previewed as UTF-8 text. The core only reads a single filename present
in the current directory snapshot, rejects path separators, directories,
symlinks and non-regular files, checks the opened file type and size, and
limits one read to at most 1 MiB. The UI uses a stricter 256 KiB limit and
rejects invalid UTF-8. Preview is read-only; there is no edit, save, delete,
upload, recursive indexing or arbitrary file-open action. A directory grant
is still memory-only and revocable.

The file provider remains synchronous. Large directory enumeration and
preview reads may block the UI; bounded asynchronous I/O and cancellation
are follow-up work. The OS-root adapter confines access to the selected root,
but this feature is not a malware scanner and does not inspect file contents
for safety.
