# Workspace Content Lifecycle Contract

This contract applies to app-owned content, not the framework's native
window or WebView APIs. It defines the rules new providers and background
executions must follow. It does not introduce a new snapshot format or
a background executor.

## Objects and ownership

| Object | Owner and identity | Responsibility |
| --- | --- | --- |
| Content Provider | Application registry; stable provider ID | Defines a content kind, its capabilities, and factories for a default or explicitly created instance. |
| Content Instance | Workspace content registry; stable instance ID within its workspace | Identifies one logical browser, chat, terminal, document, or agent workspace. A provider may expose a default instance or support several. |
| Session | Content manager; one current runtime binding per instance | Owns live state, resource handles and permission to act for that instance. A new process may bind a fresh, idle session to the same instance. |
| View | Workspace layout; a mounted presentation of a Tab reference | Shows or interacts with a session. Zero views is valid. Multiple tabs may refer to the same instance; a singleton native surface may mount in only one pane at a time. |
| Execution | App runtime or an approved durable executor; independent run ID | Tracks one unit of work, its state and cancellation. A session associates the work and provides inputs/permissions; a view never owns it. |

In snapshot v1, the field named `Tab.SessionID` stores the stable instance
reference. `ContentSession.ID` currently shares that value. This is a
compatibility alias, not permission to serialize the session or conflate a
restored session with the original process. An execution ID is not a Tab,
Pane, or Session ID.

## State changes

- **Open, close, move or select a Tab:** attach, detach or move a view only.
  Opening the same instance reuses its current session. Selecting or
  hiding a view cannot send a request, start a shell, stop a run, or evict a
  session. Closing the last reference has no implicit stop or reset rule.
- **Mount selection:** an active view in the focused pane wins. Otherwise
  the first active view in layout order wins. No active view means no
  mount. A hidden tab remains a reference. `ResolveContentViews` makes
  these three facts independent of the provider and does not perform I/O.
- **Start, stop, release:** a user- or policy-authorized action controls
  a session resource or one execution. Releasing a browser page is not the
  same as closing its view. Stopping one Chat run does not stop another
  instance. Report cancellation separately from successful completion.
- **Evict or dispose:** the application, not the Tab, is responsible for
  this decision. A provider may propose idleness or detach policies; they
  must be explicit, must not discard unsaved data, and must not silently
  cancel authorized background work. The current registry does not
  implement automatic eviction.
- **Window close and process exit:** honor save/discard/cancel before
  shutting down. App-owned ephemeral work gets explicit cleanup; future
  durable work can outlive the window only if its executor has separately
  defined persistence, policy and recovery. Current terminal/Chat work
  is not an external durable run.

## Restore and safety

Save layout and instance references independently of runtime state.
Restore known and allowed instances as idle bindings. Reading explicitly
saved local document data is allowed; opening a tab or restoring a
snapshot is not consent to execute a command, navigate a page, index
files, or send a model or Agent request. Unknown references must remain
unavailable rather than silently falling back to another provider.

Do not put credentials, in-memory drafts, unsaved results, process
handles, or active execution records in the layout. Notes and opt-in
Chat archives have their own storage contract. A durable Agent executor
must own its checkpointing, authorization and explicit recovery; it must
not infer a resume decision from a restored Tab.

## Practical cases

| Case | Detach or switch view | Separate explicit action |
| --- | --- | --- |
| Browser | Unmount the WebView when not shown; keep its session in the current process. | Close page releases the WebView; Go navigates. |
| Terminal | Keep the current shell when the view is hidden. | Start shell or Close terminal; no shell starts on restore. |
| Chat | Keep the selected instance's draft and current generation in memory as applicable. Other instances are independent. | Send, Stop generation, or explicitly save/load an archive. |
| Agent background task | Closing or switching a future Agent view must not cancel its run. | The runtime owns start, cancel, retry, checkpoint, and policy-authorized resume. |

The first shared implementation covers view counts, mount selection,
retain-on-detach and layout/runtime separation. It preserves existing
provider switches. A generic execution registry, durability, independent
window lifetimes and resource eviction are follow-up work, not implemented
by this contract.
