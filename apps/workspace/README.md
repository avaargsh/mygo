# MyGo Workspace

Personal AI Workspace — Universal Content, Local-first, Agent-native, MCP-native.

This is an independent product layer within the existing MyGo Go module.
It does not change framework APIs or require a second module.

## Linux x64 preview: first session

Use all files from the same Ubuntu x64 preview bundle. Extract the bundle
to a folder and verify it before installing:

    sha256sum -c SHA256SUMS

Choose one install path (not both):

- System installation: `sudo apt install ./mygo-workspace_0.1.0_amd64.deb`
- Per-user installation: `sh ./install.sh ./mygo-workspace-0.1.0-linux-amd64.tar.gz`.
  The host needs GTK 3 and WebKitGTK 4.1. Install the distributions' required
  desktop packages if the installer reports them missing.

Launch **MyGo Workspace** from the application menu or run
`mygo-workspace` (for the per-user method, the installed command is at
`~/.local/bin/mygo-workspace`).

1. Open **Browser**, type an HTTP(S) URL, and choose **Go**.
2. Open **Terminal** and choose **Start shell**. It runs your local shell
   as your current user; it is not an isolated sandbox.
3. Use **Split right** or **Split down**, then drag the divider or focus it
   and use arrow keys. Choose **Move tab to next pane** to move the active
   tab; use each tab's close control to close its view.
4. Open **Notes**, edit the Markdown, and click **Save note**.
5. Open **Chat**. Enter the model name and its OpenAI-compatible chat
   completions URL. A remote endpoint must use HTTPS. If your provider
   requires an API key, enter it in the password field, then send a message.
6. Open **Search** and select an exact index on a separately running,
   local XERJ node. Enter keywords and choose **Find passages**. MyGo does
   not build an index or start the XERJ node on your behalf.
7. Click **Save workspace**, quit, and reopen. The split sizes, tabs,
   selection, theme and saved note return. Web navigation and terminal
   processes do not resume automatically. Search configuration/results,
   credentials and the default Chat conversation are not part of the
   workspace snapshot.

A test-only CI run may exercise these steps without publishing a download.
Use a distribution artifact explicitly provided for your platform; a CI
link by itself is not an installer.

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
Unsaved changes are explicitly marked. Closing the window or quitting asks whether
to save or discard layout and note changes, including edits in a closed Notes
tab. A failed save keeps the window open. An OS shutdown is not cancelable;
save before ending your session. New IDs avoid
collisions with restored IDs. The app writes state.json under MyGoWorkspace
in os.UserConfigDir(). A corrupt or unsupported file stops startup without
overwriting it. To recover manually, back up the file and move it aside.

The Linux preview carries a `copyright` file in the per-user app directory
(`~/.local/mygo-workspace.app/copyright`) and in the Debian package at
`/usr/share/doc/mygo-workspace/copyright`. The distribution job also records
the packaged binary's Go module inventory in `GO-BUILD-INFO.txt`. That
inventory is an input to reviewing additional linked dependency notices,
not a substitute for them.

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
choose Go. A native WebView is created only
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

## Command Palette (M2 next slice)

Choose Commands in the window header, search by title, stable command ID or
keywords (including some Chinese terms), and click an enabled Run action.
The existing toolbar and palette share one built-in registry. Layout actions
recheck the active workspace immediately before dispatch. Opening Terminal,
Browser or Files still does not start a shell, navigate or grant directory
access. This registry is UI-owned, not a shell executor or Agent/MCP tool API.
Search is side-effect free; a successful dispatch closes the palette. A
dispatch result only confirms that a registered callback ran; operation-specific
errors remain in the workspace status. Keyboard shortcut binding and an
overlay layout are deferred pending native UI validation.

Evidence: three command registry tests were run in the isolated Go 1.23.2
container (RED compilation failure, GREEN after implementation), plus race
and vet. The two new core file Git blob hashes match the locally tested files.
The existing isolated core directory also passed regression tests, but it
does not contain every latest remote file. Native palette tests and complete
Go 1.27.3+ desktop build are not yet run. The Browser input uses the explicit Go
button rather than relying on an unverified submit callback API.

## Model transport (M3 first slice)

The standard-library-only model adapter implements explicit text-only
OpenAI-compatible Chat Completions calls, using the full endpoint URL and
model name selected by the caller. It does not connect on construction,
automatically attach workspace contents, execute tools, or store credentials.
API keys are passed per call. HTTPS is required except literal loopback HTTP;
redirects and environment proxies are disabled. Requests and responses are
bounded (1 MiB and 2 MiB respectively), requests have a 60-second timeout,
and caller context cancellation is supported. Remote error bodies and raw
transport errors are not exposed. Model replies containing tool calls or
legacy function calls are rejected. Returned text is untrusted content, not
an instruction to execute.

The non-streaming transport has not been wired into a Chat Tab or keychain.
The later streaming slice adds a text-only SSE transport. No live external model or private gateway was contacted.
The six adapter tests ran against local httptest servers with Go 1.23.2,
including a RED-to-GREEN regression for legacy function replies. Race and
vet passed. The two remote blobs match the exact locally tested files.
Full repository/native UI and Go 1.27.4 cloud verification remain pending.


## Model SSE transport (M3 second slice)

Client.Stream shares the existing validated request, endpoint and credential
boundary with Complete. It requests stream=true and requires text/event-stream.
Each nonempty text delta is delivered synchronously to the caller; the caller
must return promptly and retain partial text separately if desired. A successful
Completion requires both a supported finish reason and a [DONE] event. EOF,
cancellation, callback failure, malformed events, unsupported tool/function
calls and unsupported choices fail instead of reporting a completed reply.
The 60-second HTTP timeout still applies. No automatic retry or resume occurs.
Limits: 256 KiB per SSE line/data event, 8 MiB response body read budget,
2 MiB accumulated text. Usage-only events after the final choice are ignored.
Only a single text choice is supported; this is not a general-purpose SSE
implementation or an Agent execution API.

Local evidence: exact model package sources tested on Go 1.23.2 against
local HTTP servers. 11 top-level tests passed (6 existing, 5 streaming);
race and vet passed. Incremental delivery was explicitly verified with a
server that waited for the consumer callback before sending the finish event.
Remote blobs match the locally tested sources. No live model, native Chat
Tab or complete Go 1.27.4 cloud-host build was verified.


## Chat session and native view (M3 third slice)

Open Chat from the toolbar or Commands. Enter a full Chat Completions endpoint,
model name, optional API key, and message, then explicitly choose Send message.
Opening/restoring the tab makes no network request. Only the prompt and prior
completed chat turns are sent to the selected endpoint. No notes, files,
browser, terminal, or system context is attached. Once a send is accepted,
the endpoint and model are bound to the conversation; clear it to switch.
The password field is cleared after accepting a send and on window close.
No credential or chat history is persisted; clearing a Go string is not
a guarantee of secure memory erasure.

The concurrent model.ChatSession supports one worker per session, snapshots,
incremental text, cancellation with a stopping state, and a terminal outcome.
Failed/canceled turns retain partial text locally but are excluded from future
requests. Completion requires transport success with matching delivered text
and supported finish reason. Limits: 32 turns, 32 KiB prompt, 256 KiB reply,
512 KiB outgoing history content (the transport also enforces its JSON limit).
The view polls active snapshots at 100 ms intervals, with no direct UI callback
from the network worker. Closing a tab retains its session. Hidden generation
can be stopped from the status area; closing the window requests cancellation.
Chat is a singleton, memory-only, text-only conversation, not a tool-executing
Agent. No automatic retries, reconnection, conversation persistence, model
profiles, keychain integration, or token accounting.

Evidence: Go 1.23.2 isolated model package, RED compilation failure before
ChatSession implementation and GREEN after. 15 top-level tests passed
(11 existing transport tests and 4 ChatSession tests), race and vet passed.
The new chat.go/chat_test.go remote blobs match locally tested sources.
One test uses the actual SSE Client with a local HTTP server; cancellation
and history tests use controlled stream functions. Native Chat UI tests
are committed but not executed; no external model endpoint or full
Go 1.27.4 cloud-host desktop build is claimed.


## Merge-readiness follow-up

A rejected chat delta now permanently fails the current turn, even if a
faulty stream adapter ignores the callback error and returns a successful
completion. Partial valid text is preserved and the failed turn is excluded
from subsequent requests. Regression reproduced RED and verified GREEN.
All 16 top-level model package tests, race detection and vet passed in the
Go 1.23.2 isolated model package; both changed blobs were verified against
the tested files.

The separate Workspace Gate workflow runs Linux Go 1.27.4 workspace tests,
core/model/system race tests, vet and desktop binary compilation with the
repository's JavaScript build and desktop runtime dependencies. It does not
replace the existing full cross-platform CI or actual desktop/WebView/PTY
smoke verification. Workflow YAML was parsed and structurally checked
locally; a successful Actions run is not yet established.

## Search an existing local XERJ index

Open **Search** in the toolbar or choose **Open Search** from Commands.
Enter the numeric loopback HTTP origin of a running XERJ node (default
`http://127.0.0.1:9200`), its exact index slug (not a project or folder
name), and search text. Choose **Find passages**. Results show a source
path, a line when supplied and supported by the passage offset, and a
plain-text excerpt. A source is a label, not a link or a verified citation.

See [the XERJ adapter guide](../../plugins/xerj/README.md) to prepare a local
node and look up its indexed slug. MyGo does not install or start XERJ,
scan folders, create indices, contact a remote node, or send results to a
model in this first search view. Connection settings, queries and results
are not written to workspace state. A saved Search tab reopens idle.
