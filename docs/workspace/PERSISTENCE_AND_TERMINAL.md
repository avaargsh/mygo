# Workspace persistence and terminal acceptance contract

## Terminal
- A tab is a view, not the terminal process owner.
- A close attempt hides the resource even if teardown fails. Failed requests remain retryable through visible or hidden terminal controls. An accepted request is not repeated. Done is the sole process-exit signal; replacement is blocked until Done and successful cleanup request.
- The native plugin's Close is one-shot and may return nil on subsequent calls; a retry is not proof of native cleanup. Native failure and process/descendant exit require separate installed-platform tests.

## Workspace state (v1 JSON)
- LoadState validates the complete bounded snapshot; missing state is distinct from corrupt or unsupported state. Do not overwrite invalid state automatically. Back up the original before explicit recovery.
- SaveState accepts a caller-owned Workspace and theme. The caller serializes workspace mutations. In-process saves are serialized; independent writers are last successful replacement wins, not a merge. No cross-process lock or conflict resolution is provided.
- Validate and encode, write a same-directory temporary file, sync and close it, replace the destination, then sync the parent directory on Linux/macOS. Failures before replacement preserve the prior state. After replacement, directory sync failure returns an error; the new snapshot may be visible, and the caller must not report a durable save.
- Other platforms retain existing rename semantics. Filesystem, OS and hardware behavior can limit durability. Do not claim power-loss safety or atomic Windows replacement from unit tests.
- Layout, note and chat archive transactions remain independent. No SQLite migration or silent recovery is introduced.

## Acceptance
- Unit: failed/retried close, idempotent accepted close, no replacement before exit, cleanup retry after exit.
- Unit: replacement fault preserves previous snapshot and removes temp file; directory sync ordering and failure reporting; existing version, bounds, restart and invalid-state cases.
- Integration before release: exact-head full CI, native PTY lifecycle and installed layout restart; concurrent independent writer/read stress on target filesystems; kill during write, before/after replacement and before directory sync; recover or explicitly reject corrupt snapshot without overwriting it. Windows and macOS installed checks, cross-process writer policy and actual power-loss tests remain separate gates.
