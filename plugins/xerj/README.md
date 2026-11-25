# Optional XERJ local retrieval adapter

This package is a first integration slice for the workspace search feature, not a desktop UI or process supervisor. It adds no dependency to MyGo core. XERJ runs as a separately installed local binary; it is not downloaded or launched by this package.

## Local smoke test

Install a pinned, checksum-verified XERJ release from https://xerj.org/docs/install. Use a disposable folder containing only approved non-sensitive files. Keep the data directory outside the indexed tree and disable outbound feedback.

In a separate terminal:

    XERJ_DISABLE_FEEDBACK=true xerj --insecure --data-dir "$HOME/.local/share/mygo-xerj"

Wait for the local node to become healthy, then:

    XERJ_DISABLE_FEEDBACK=true xerj autoindex /path/to/approved-corpus --prefix mygo-demo
    xerj autoindex map --json

Select the exact index from the catalog (not a wildcard). In Go:

    import "github.com/egoist/mygo/plugins/xerj"
    // In an application worker, not on the UI thread:
    client, err := xerj.New("http://127.0.0.1:9200", exactIndex)
    // Handle err before using client.
    hits, err := client.Search(ctx, "workspace query", 5)

Check results against the original files. Path and line are unverified search metadata; never use them as unrestricted file-open capability. Empty hits are valid. No match is not proof that a fact is false.

## Scope and limitations

- Local development only: the loopback service is not an authentication or multi-tenant boundary. Never use this insecure setup for remote or shared data.
- The application chooses the exact approved index; never bind the constructor or raw Elasticsearch DSL to the model or web frontend.
- Read-only lexical search with `_passage` projection (no whole document request), exact hit index check, incomplete-result rejection, bounded results, five-second deadline, no redirects or proxy; no memory writes, indexing, external reranking or arbitrary tool execution.
- Index only explicitly approved directories; exclude secrets, credentials, personal files and generated state.
- Retrieved text is untrusted. Opening a source requires workspace authorization and current-file validation.
- Before general availability: verify against a pinned real XERJ release, add durable index lifecycle, actual builder integration and per-workspace access enforcement, stale-source detection, typed IPC, UI and a disabled/unavailable state. Test desktop packaging separately.

## Tests

    go test -race ./plugins/xerj
    go vet ./plugins/xerj

Unit tests use an HTTP fixture. They establish adapter behavior only. The opt-in live test is skipped unless explicitly enabled; a skip is not a real-node pass. For upstream interface details see https://xerj.org/llms.txt.

## Opt-in live acceptance

Use a fresh disposable local node on an unused port (not an existing personal or shared index), with a pinned verified binary. Confirm its version using `xerj --version`. Create a dedicated temporary corpus and keep the node data directory outside it:

    mkdir -p /path/to/disposable-corpus
    printf 'mygoxerjfixturemarker\n' > /path/to/disposable-corpus/mygo-xerj-fixture.txt
    XERJ_DISABLE_FEEDBACK=true xerj autoindex /path/to/disposable-corpus --url http://127.0.0.1:9200 --prefix mygo-fixture
    xerj autoindex map --url http://127.0.0.1:9200 --json

The port above is an example; use the actual disposable node port. Inspect the catalog and use the exact index for the fixture (never a wildcard):

    MYGO_XERJ_LIVE=1 MYGO_XERJ_ENDPOINT=http://127.0.0.1:9200 MYGO_XERJ_INDEX=<exact-index> go test -race -count=1 -run '^TestLiveSearch$' -v ./plugins/xerj

Replace the placeholder with the actual name before running. A passing test requires the known source file and marker in a returned passage. The test does not install or launch software, index files, or send feedback. A passage offset alone does not establish a source line; unknown line numbers are omitted. Do not expose search metadata as a trusted file-open path.

Record the binary version, fixture index, and test output before marking the live gate complete. Object/array passage fixtures are defensive compatibility tests, not evidence of actual upstream wire behavior.

## Workspace binding lifecycle (in-memory)

A single application-owned Manager per local node coordinates trusted workspace bindings. Each generation needs a fresh dedicated exact index name; old names remain reserved for the lifetime of the Manager. The trusted caller must independently build and validate the authorized corpus before publishing the generation. This package does not launch autoindex or validate index ownership at the engine.

    manager := xerj.NewManager("http://127.0.0.1:9200")
    ticket, err := manager.Begin(workspaceID, freshExactIndex)
    // Handle err; build and verify the dedicated corpus externally.
    // Only after successful verification:
    err = manager.Publish(ticket)
    // Handle err before searching.
    hits, err := manager.Search(ctx, workspaceID, "workspace query", 5)

Begin invalidates the previous ready binding immediately. Status distinguishes UNBOUND, BUILDING, READY and FAILED; Fail records a failed build, and retries require a new generation. Stale completion tickets and results from a superseded in-flight query are rejected. There is no fallback to older indices. This chooses availability loss during rebuild over serving outdated results.

Do not expose Manager lifecycle methods, raw workspace IDs or index selection to untrusted frontends or agents. The caller must authorize workspace identity, enforce the approved corpus, and revalidate source identity before opening results. Reservation is process-local; persistence, restart reconciliation, actual build/cleanup operations, engine-side authorization, and source freshness are not implemented. READY means the trusted caller published a binding, not that the node is currently healthy. Incremental in-place updates are intentionally deferred until a consistent generation protocol is verified.
