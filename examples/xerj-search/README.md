# Native XERJ search fixture

Run the standalone native example from the MyGo checkout using the toolchain pinned in go.mod:

    go run ./examples/xerj-search

It defaults to DISABLED and performs no engine requests. No core MyGo dependency, plugin auto-registration or workspace file opening is introduced.

## Optional live fixture

Use only a dedicated disposable numeric-loopback XERJ node that you exclusively own, launched with a pinned independently checksum-verified executable and feedback disabled. Prepare the single synthetic file documented in plugins/xerj/README.md (mygo-xerj-fixture.txt, exact UTF-8 bytes "mygoxerjfixturemarker" followed by a newline), index it in an otherwise empty dedicated exact index, and confirm the index name independently. Do not use personal or shared data. Do not mutate the fixture index during the demo.

    export MYGO_XERJ_PANEL=1
    export MYGO_XERJ_ENDPOINT=http://127.0.0.1:9200
    export MYGO_XERJ_INDEX=YOUR_EXACT_FIXTURE_INDEX
    go run ./examples/xerj-search

Replace the example port and index with the actual disposable fixture values. The bounded preflight checks the exact index cardinality, source path and full-body digest before binding it; a failed preflight leaves search disabled. The example does not download, launch, build or delete engine data. It does not verify the running node's identity or continuously revalidate its contents.

Type mygoxerjfixturemarker and press Enter or Search. Search executes on one application-owned worker. Cancel requests cooperative cancellation. A new submitted query supersedes an earlier one; editing the input without submitting does not change the displayed query's results. Completion requests a native window redraw without carrying a stale result payload. Closing the window closes the controller; the worker is joined after App.Run returns.

The panel displays source labels and plain text snippets, not clickable file-open actions. It is deliberately a fixture-only example, not integration into a real workspace or a permission boundary. Do not enable it for ordinary user files.

## Acceptance checklist (not yet completed)

- Build with the pinned Go toolchain: go build ./examples/xerj-search
- Run the disabled example and confirm it shows DISABLED without a node.
- Prepare the verified synthetic fixture and confirm matching, empty, canceled and unavailable states; verify latest submitted query wins.
- Close the window during a search and quit the application; confirm the worker stops.
- Check scrolling and layout on supported desktop platforms, and ensure core app sizes are unchanged by this opt-in example.

The presentation tests can be run separately (go test -race ./examples/xerj-search in the full checkout). State labels and source display formatting were tested in isolation; compilation against the native UI and a live desktop session are distinct gates.
