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
- Before general availability: verify against a pinned real XERJ release, add actual builder integration, restart reconciliation beyond conservative rebuild, cross-platform persistence validation and per-workspace access enforcement, stale-source detection, typed IPC, UI and a disabled/unavailable state. Test desktop packaging separately.

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

Do not expose Manager lifecycle methods, raw workspace IDs or index selection to untrusted frontends or agents. The caller must authorize workspace identity, enforce the approved corpus, and revalidate source identity before opening results. For NewManager reservation is process-local; the persistent variant below retains reservations across reopening. Actual build/cleanup operations, engine-side authorization, and source freshness are not implemented. READY means the trusted caller published a binding, not that the node is currently healthy. Incremental in-place updates are intentionally deferred until a consistent generation protocol is verified.

## Optional persistent index manifest

CreatePersistentManager(endpoint, absolutePath) initializes a versioned JSON manifest exclusively and refuses to replace an existing file. OpenPersistentManager(endpoint, absolutePath) requires an existing valid manifest and the same configured endpoint. Use a private application state directory outside all indexed trees, with exclusive ownership of the node and manifest enforced by the application; this module does not implement interprocess locking. Do not silently create a replacement after an open failure.

Begin, Publish and Fail persist generation, state and historical exact index reservations before reporting success. The file is bounded to 4 MiB. An update writes and syncs a temporary file and replaces the manifest in the same directory. A write failure disables further searches and transitions on that Manager; stop it and inspect persisted state before reopening. This is not a power-loss guarantee: directory durability, Windows replacement behavior and multi-process exclusion are not independently established.

Reopening deliberately marks previously known workspaces FAILED, including READY and BUILDING entries. Generation counters and all historical reservations survive, but a fresh verified generation is required before search resumes. Old index names remain unavailable for reuse. The endpoint string is configuration, not a verified node identity. This conservative restart behavior is intentional until corpus identity, worker reconciliation and engine freshness can be verified. No automatic build, cleanup or index reuse is introduced.

Run the additional manifest tests with the package race suite. The tests use real temporary filesystem paths and simulate a failed update by moving the state directory. They do not establish live XERJ behavior or power-loss durability.

## Build coordinator

The optional Manager.Rebuild(ctx, workspaceID, freshExactIndex, build, verify) API reserves a fresh generation, runs the trusted builder, runs the independent trusted verifier, and publishes READY only if both succeed and the generation is still current. It persists transitions when the persistent manager is used.

Callbacks receive a BuildTarget with the exact reserved index, endpoint, workspace and generation. Run off the UI thread and provide a context deadline appropriate to the approved corpus. Callbacks must cooperate with cancellation and restrict work to the dedicated approved corpus/index; they are not a sandbox. A build or verification error marks the current attempt FAILED, and an already superseded attempt cannot overwrite a newer binding. Cancellation observed before publication prevents READY; cancellation can race an already completed publication. Invalid callbacks or a pre-canceled context do not fence a valid current binding.

This is synchronous orchestration, not an XERJ command runner, queue, watcher, or proof that a real corpus is indexed. No CLI invocation or directory discovery is added. Actual builder and verifier adapters must be tested against a pinned real node before enabling user-facing search. The application remains responsible for authorization, resource limits and callback termination.

## Autoindex process adapter (development)

AutoindexConfig.Run is an opt-in build callback for Manager.Rebuild, not a standalone publication mechanism. Configure an already installed executable by absolute path and a trusted SHA-256 from a pinned release. Provide a preapproved, staged corpus directory and separate private state root. The application must reserve a fresh prefix on a dedicated loopback development node; the expected exact index must belong to that prefix. XERJ can create more than one dataset index under a prefix, so do not assume prefix equals index name or that a single index covers the corpus.

The adapter verifies the binary digest, resolves the two directory roots, rejects overlapping roots, creates a fresh private journal directory, and invokes the fixed autoindex arguments directly without a shell. It requests one worker, lexical-only ingestion, disables feedback, avoids inheriting application environment, and discards process output because it may contain source content. Subprocess execution is capped at five minutes or the caller's earlier context deadline; this does not bound the preceding filesystem operations. No additional approval flag is sent. Exit 0 permits the independent verifier to run, not publication by itself. Exit 3 (partial success/junk) and exit 4 (decision required), and all other nonzero exits, are treated as unverified in this strict pilot.

The caller must ensure the executable and staged corpus cannot be changed concurrently, the node is dedicated, state and manifest directories are outside indexed trees, and the prefix is unique across generations. This is not a filesystem sandbox, multi-tenant boundary, or process-tree supervisor. A separately implemented verifier must validate the actual catalog, dataset completeness, and source identity against the staged corpus before READY. No automatic retry, journal cleanup, file watching, or multi-index publication is implemented.

Local tests execute a POSIX shell fixture as the fake binary to check the argv and exit handling; they do not execute XERJ or establish upstream binary compatibility. Windows native execution and real pinned-release acceptance remain open. Upstream command and exit-code reference: https://xerj.org/llms.txt and https://xerj.org/docs/cli.

## Bounded text index verification

TextIndexVerifier.Verify can serve as the independent verifier callback to Manager.Rebuild for a deliberately small single-dataset pilot. Supply an immutable, trusted map of slash-relative source paths to SHA-256 digests computed from the approved staged UTF-8 source bytes before building. Do not derive expected digests from index responses.

The verifier queries only the reserved exact index, requires an exact total and all expected records (1–32), rejects partial responses and unexpected index/path, and compares each returned full body to the independent digest. Responses are capped at 2 MiB. Duplicate paths are rejected by consuming expected entries once. This is an acceptance-only full-body request, separate from normal passage retrieval. Transformed, chunked, deduplicated or multi-dataset corpora cannot use this strict one-record-per-source contract.

This does not discover catalog datasets, verify the identity of the running node, prove search ranking correctness, or guarantee index freshness after the response. The dedicated node and staged corpus must remain exclusively controlled during and after verification. Full mixed-format catalog reconciliation and real-node acceptance are open. The fixture requires hits.total.relation=eq; if the pinned engine lacks this response shape the verifier fails closed until an independently validated exact-count protocol is implemented. See https://xerj.org/llms-full.txt for the upstream autoindex source-field contract.

## Read-only dataset discovery

DiscoverIndices(ctx, endpoint, prefix) lists index names through the documented CAT indices endpoint using JSON format and the index column. It filters by the exact prefix plus hyphen, sorts the names, rejects malformed or duplicate entries and limits the response to 1 MiB. The configured endpoint must be a numeric loopback origin; no wildcard is passed to search. The JSON CAT projection is fixture-tested, not yet confirmed on the pinned real XERJ release.

Use an exclusively owned local node. Before a build, check that the fresh prefix has no existing datasets. After a successful CLI build, compare the returned names with an independently approved exact set using MatchIndexSet(discovered, expected). Empty, unexpected, duplicate or missing sets cannot pass this gate. Then independently verify the source content per dataset. A catalog listing cannot establish source authorization, prefix ownership or completeness; do not derive the expected set from the same untrusted listing.

These are separate trusted-application primitives, not an automatic pre/post build gate. The existing Manager still publishes one exact index per workspace generation. Atomic publication of multiple datasets, search fan-out, multi-format content validation and automatic build integration remain unimplemented. See https://xerj.org/llms-full.txt for the CAT endpoint and prefix naming contract.
