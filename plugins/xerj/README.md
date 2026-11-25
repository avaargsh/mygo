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

These are separate trusted-application primitives, not an automatic pre/post build gate. Manager.BeginSet now supports atomic binding/publication of an explicitly supplied set and Manager.Search queries each exact index. Multi-format content validation and automatic build integration remain unimplemented. See https://xerj.org/llms-full.txt for the CAT endpoint and prefix naming contract.

## Multi-index workspace generations

Manager.BeginSet(workspace, indices) reserves 1–8 exact names as one generation; all names must be distinct and unreserved. Begin remains the single-index convenience API. A trusted caller must independently verify every dataset before Publish. Rebuild remains a single-index coordinator; multi-dataset automatic orchestration is not implemented.

Manager.Search queries each bound exact index sequentially under one five-second context budget and returns at most the requested global limit (20 maximum). Results are interleaved round-robin in sorted index-name order, preserving per-index order and adding the source index to Hit. This is not global relevance ranking or deduplication. Any request failure discards all hits; a superseding generation also discards the entire result.

The persistent manifest now writes version 2 with an indices array per binding and accepts version 1 on reopen. The next successful transition writes version 2; old readers cannot read v2. As before, reopen retains reservations and generations but marks bindings FAILED until a new verified generation is built. Back up the manifest before downgrading. Atomicity is within the application Manager and its existing persistence contract, not across engine mutations, processes or power loss.

## Bounded multi-dataset build pipeline

BuildTextSet(ctx, workspace, config, expected) composes the pinned CLI adapter, catalog discovery and per-dataset TextIndexVerifier. The caller supplies the immutable approved expected index/digest map independently of the engine. The pipeline checks that a fresh prefix has no prior catalog entries or historical reservations, reserves the complete generation, runs autoindex once, checks the exact resulting catalog, verifies each dataset, checks the catalog again and publishes READY only for the current generation. Concurrent pipeline attempts with the same prefix cannot both reserve it in one Manager. Failures after reservation mark the generation FAILED; invalid or occupied preflight leaves the previous binding unchanged. The caller must exclusively control the node and avoid direct engine mutations or bypassing this pipeline with manual publication.

The current pilot handles 1–8 text datasets, each with 1–32 untransformed records and a bounded verification response. It is a synchronous pipeline, not a durable job queue or file watcher. Test fixtures use a fake builder and HTTP server, while the production entry calls AutoindexConfig.Run. No live XERJ end-to-end run or mixed-format completeness proof is claimed.

## Process-local build queue

NewBuildQueue(manager, capacity) creates an application-owned FIFO, with capacity 1–64 counting both queued and active jobs. Use one queue per exclusively owned Manager and node. Submit(workspace, config, expected) snapshots the bounded expected digest maps and rejects duplicate workspace or prefix among pending and active jobs. The approved staged corpus and pinned binary must remain immutable. Submission does not reserve a generation or invalidate the previous READY binding; the pipeline does that when execution begins.

The application invokes RunNext(ctx) from an off-UI worker loop; it executes one pending job synchronously and returns whether a job was processed and its error. An empty queue returns false, nil; concurrent worker calls return ErrBuildWorkerBusy. Supply a context deadline for the whole pipeline. BuildJob.Status(), Cancel() and Wait(ctx) expose QUEUED/RUNNING/CANCELING/SUCCEEDED/FAILED/CANCELED and the execution outcome. A queued cancellation removes the job without running the pipeline. Running cancellation is cooperative; a publication that already succeeded is not rolled back. Wait cancellation affects only the observer. Terminal handles are retained only by callers.

The default queue is intentionally in-memory; a process restart loses pending jobs and job handles. The opt-in journal below preserves status evidence only. Persistent Manager manifests still fence previous generations on restart. There is no automatic retry, durable replay queue or cleanup; see the application-owned Serve loop below. Applications must not silently re-submit an old prefix or bypass the pipeline. The public Submit integration test uses a POSIX executable fixture plus an HTTP fixture, not the real XERJ engine.

## Persistent build status journal (opt-in)

CreateJournaledBuildQueue(manager, capacity, absoluteJournalPath) creates a separate private status snapshot for an exclusively owned persistent Manager. OpenJournaledBuildQueue reopens it after the previous owner has stopped and the persistent Manager has been reopened. Keep both files outside indexed trees; the application must enforce exclusive ownership and non-aliased, trusted paths. Do not recreate a missing or corrupt journal silently.

Submit, dispatch, cancellation and completion record state before acknowledging the transition. History returns a copy of the last persisted records (workspace, prefix, state), not executable inputs. The journal is bounded to 1024 records and 4 MiB. Historical prefixes cannot be reused, even when canceled before execution. Capacity of live jobs remains 1–64. No automatic compaction is provided; reaching the history limit requires an explicit future retention policy.

On restart, QUEUED, RUNNING and CANCELING records become INTERRUPTED; terminal records remain historical evidence. No job is automatically replayed, and old successful status is not proof that the recovered index is READY. Re-authorize the corpus and submit a new prefix for retry. A journal write failure freezes new dispatch and resolves pending handles as interrupted. A build already in progress can still complete; an inability to persist its terminal state does not undo index publication. Reconcile the separately persisted Manager state before proceeding. A failed cancellation write returns false; an active cancel request may still have been issued.

The journal uses synced temporary-file replacement, not a transaction with the index manifest. Directory durability, interprocess exclusion, Windows replacement behavior and crash/power-loss atomicity remain unverified. This is a status journal with conservative recovery, not a durable replay queue or autonomous worker. The tests exercise filesystem persistence, recovery without replay and a simulated failed write.

## Application-owned worker loop

BuildQueue.Serve(ctx, jobTimeout) blocks and consumes pending jobs in FIFO order. Invoke it from an application-owned background goroutine with a lifetime context and join that goroutine before releasing the queue or Manager. Submission and journal faults signal a coalescing wakeup; an empty queue waits without periodic polling. While Serve owns dispatch, a second Serve or manual RunNext is rejected. A manual RunNext already executing also prevents Serve from starting.

The per-job timeout must be positive and at most 30 minutes. It bounds the pipeline cooperatively through its context, not all filesystem operations or process descendants. Ordinary job errors and job deadlines are reported through the corresponding handle; the loop continues to the next distinct job, without retrying failed work. Journal or manifest errors stop the loop and require operator reconciliation.

Cancel the lifetime context to request cooperative shutdown. The loop waits for the current pipeline to return; pending jobs remain queued in memory and can be processed by a new Serve invocation. If the process exits, the existing journal recovery policy marks unfinished records INTERRUPTED and never replays them automatically. Already published indices are not rolled back on cancellation. This is an application-owned dispatch loop, not a separately supervised process or UI integration.

## Approved text snapshot and change detection

CaptureTextSnapshot(ctx, absoluteRoot, approvedPaths) captures SHA-256 digests of an explicit application-approved list of slash-relative UTF-8 text files. It reads at most 32 files, 128 KiB per file and 256 KiB aggregate. No recursive directory discovery or content upload occurs. Duplicate names, invalid paths, symlink components below the resolved root, missing files, nonregular files, invalid UTF-8 and oversized inputs fail the entire capture. The caller must control the root and prevent concurrent mutations; portable path and metadata checks are not a filesystem sandbox, atomic snapshot or protection against adversarial races.

TextSnapshot.Digests returns a copy for the independent verifier's expected map. Diff(next) reports sorted added, removed and modified paths based on exact content bytes, independent of input ordering. An approved file missing from an unchanged list is an error, not an accepted deletion; refresh the approved list explicitly. A nonnil empty list can describe a change to an empty corpus but cannot be published through the nonempty text build pipeline. The zero-value or a failed capture is not evidence of a successful snapshot; compare only successful captures of the same trusted corpus scope.

This is a synchronous change detection primitive, not an OS watcher, immutable staged directory, automatic invalidation or incremental index writer. Do not pass a source directory containing unapproved files directly to autoindex: separately stage only approved files and verify the staged corpus. Debouncing, automatic rebuild submission, invalidation of stale READY results and real engine acceptance remain open.

## Approved corpus staging

StageTextCorpus(ctx, sourceRoot, stagingRoot, approvedPaths) copies 1–32 explicitly approved UTF-8 files into a fresh private directory under a disjoint trusted staging root. It uses the same bounded capture routine as CaptureTextSnapshot (128 KiB per file, 256 KiB aggregate). The SHA-256 digests are computed from the exact bytes written, not from a subsequent source read. The returned StagedCorpus.Root() identifies the dedicated corpus directory; Digests() returns a copy of its expected digest map.

Use a separate trusted state directory for CLI journals, outside the staged corpus. A trusted application may use the returned Root as AutoindexConfig.Corpus and partition the returned digests into independently approved expected dataset mappings before queue submission. Do not submit the original workspace directory. On failure, the new partial stage is removed (cleanup errors are reported). On success, the application owns retention and cleanup and must keep the staged files unchanged until all build and verification users finish. No automatic queue integration or success-path cleanup is added here.

The copy is independent of subsequent source edits, but it is not OS-enforced immutable storage or protection against hostile concurrent filesystem changes. Both roots must be application-controlled and stable, and the source authorization list must be trusted. Real engine corpus ingestion and cross-platform permission semantics still need acceptance.

## Queue-owned approved text staging

SubmitApprovedText(ctx, workspace, config, sourceRoot, stagingRoot, datasets) is a trusted application convenience entry point combining staging and queue submission. Supply exact expected dataset names under the fresh prefix and explicitly approved relative file paths for each dataset. Paths must be assigned once across the entire request, and the existing bounded text corpus limits apply (32 files total, 1–8 datasets). The caller must leave config.Corpus empty: the queue derives the corpus path and expected per-dataset digest maps from the staged copy. The staging root and CLI state root must be disjoint.

A rejected submission removes its new stage; a successfully queued job owns that stage until its terminal outcome. Success, failure, queued cancellation and pending-job interruption due to journal failure all attempt cleanup. Running cancellation does not delete the stage until the build callback returns. Cleanup errors are available through BuildJob.CleanupError() after completion and do not change the build outcome or imply index rollback. Filesystem cleanup runs under the queue lock, so these APIs must be used off the UI thread. Concurrent submissions may create temporary stages before admission; the queue capacity bounds admitted jobs, not concurrent staging calls.

The trusted application must exclusively control all roots and keep staged bytes unchanged until execution completes. No crash-orphan GC, process-tree supervision, source freshness invalidation or automatic retries are provided. Cleanup assumes the engine has fully materialized indexed content and no longer needs the staged files; verify this on a pinned real XERJ release before enabling this path for users. The fixture tests do not establish this engine property.

## Explicit source invalidation

Manager.Invalidate(workspace) is the trusted application's source-change and authorization-change notification. It immediately fences READY or BUILDING bindings by setting FAILED and persisting the existing manifest; historical reservations remain. In-flight searches fail their final generation/status check, and old builders cannot publish. It neither deletes engine indices nor forcibly cancels a subprocess.

Submit and SubmitApprovedText capture a process-local source revision at entry (before staging or digest copy). BuildTextSet captures one before catalog preflight. The pipeline checks that revision before contacting the node and atomically again when reserving the new generation. A change during preflight therefore cannot start an obsolete build. A stale queued job fails on dispatch with ErrStaleGeneration and its owned stage is released through the normal terminal cleanup path. The queue does not eagerly remove stale pending jobs or automatically enqueue replacements. Revision fences for unbound workspaces are process-local; no old jobs are replayed on restart, and persistent bindings already reopen FAILED.

The application must notify invalidation promptly and serialize approved source revisions, then submit freshly approved data with a fresh prefix. Direct low-level Begin/Publish APIs remain trusted manual operations and do not carry a source revision from before invocation. The revision map is process-local and retains notified workspace identities for the Manager lifetime. No OS watcher, implicit source freshness proof, automatic retry or direct engine authorization is implemented.

## Approved text polling and change coalescing

Manager.WatchTextChanges(ctx, workspace, root, paths, baseline, interval, stableSamples, notify) is an opt-in blocking polling loop for a fixed, explicitly approved file list. Supply a successful baseline for the same corpus and accepted source revision. Intervals are 100 ms to 1 minute; stability requires 2–10 unchanged follow-up samples. It samples immediately on entry, invalidates on every observed content change (including a change back to an earlier version), and invokes the notification once the observed burst stabilizes. Invalidation happens before debounce, not afterward.

A capture error other than context cancellation invalidates the workspace and returns the error; a missing approved file is not silently accepted as a deletion. Stop and join the old watcher before changing the approved scope, explicitly invalidate, and reauthorize before restarting. The caller must run one watcher per workspace off the UI thread and own its lifetime. Duplicate watchers are not prevented by the API.

The synchronous callback receives a digest snapshot only. It must not treat that snapshot as proof of current source freshness, nor block the watcher for a long build. Callback errors stop the loop. No automatic submission, retry or native filesystem-event watcher is added. Polling may miss intermediate changes, and edits between samples or during a callback are not immediately detected. Explicit application invalidation remains necessary for authorization changes and when stopping monitoring if freshness cannot be assured. The corpus must remain trusted and the capture limits still apply.

## Approved automatic rebuild controller

BuildQueue.WatchAndRebuildText connects the fixed approved-text polling loop to staged build admission. Supply a successful baseline for the accepted revision, explicit dataset suffixes (not full index names), approved relative paths, an AutoindexConfig with empty Prefix and Corpus, the source/staging roots, polling interval, stable sample count and a short onJob callback. The dataset mapping is copied on entry. The controller generates a new prefix using 128 random bits per admission attempt; the pipeline still checks historical reservations and the node catalog. Dataset suffixes must match the intended pinned XERJ dataset naming contract.

When an observed change stabilizes, the controller stages the approved files and compares the exact staged digests to the observed snapshot before queue admission. A mismatch discards the stage and defers to a subsequent sample. A temporarily full queue or occupied workspace also defers admission while retaining the dirty notification; intervening observed changes reset the stability window and invalidate prior revisions. Journal faults, exhausted history and other non-deferred errors stop the controller. Cleanup errors are not swallowed as ordinary deferrals.

The application must independently run BuildQueue.Serve (or dispatch RunNext), observe job outcomes, and own both lifetimes. This does not initialize an unchanged corpus, retry a failed execution, eagerly cancel a stale running job or bypass source revision checks. The queue retains its existing capacity bounds; this is a polling controller with deferred admission, not a durable scheduler. A stopped watcher does not guarantee source freshness: explicitly invalidate before relying on subsequent search, stop and join the loops before changing approved scope, and reauthorize new inputs. Changes between samples or after staging may not be immediately detected. Real engine acceptance, source ownership and stage-cleanup assumptions still apply.

## Installed-engine lifecycle acceptance gate

Unlike TestLiveSearch (read-only, externally indexed fixture), TestLiveInstalledFlow exercises the actual pinned autoindex subprocess via the application queue. It creates synthetic approved and unapproved files, stages only the approved file, builds and independently verifies two fresh generations, queries each after its staging directory has been removed, rejects retrieval between invalidation and rebuild, then reopens the persistent Manager and status journal and checks that reservations/history survive while search is fenced. The reopen is a logical owner reopen, not a process-crash or power-loss test.

Prerequisites: a dedicated disposable numeric-loopback HTTP node with no other users, launched manually from a pinned, independently checksum-verified XERJ release; network/feedback disabled as appropriate in the environment; the same absolute executable path and trusted digest for the test. Inspect the pinned CLI help and establish the expected dataset slug for a single plain-text fixture separately. Autoindex derives dataset names from content clustering rather than the source directory name; do not guess a slug, authorize whatever the test discovers, or weaken exact catalog/body verification to force a pass.

From the MyGo checkout at the candidate commit, with its pinned Go toolchain:

    export MYGO_XERJ_ACCEPTANCE=1
    export MYGO_XERJ_ENDPOINT=http://127.0.0.1:9200
    export MYGO_XERJ_BINARY=/absolute/path/to/verified/xerj
    export MYGO_XERJ_SHA256=YOUR_INDEPENDENTLY_VERIFIED_SHA256
    export MYGO_XERJ_DATASET_SUFFIX=YOUR_VERIFIED_FIXTURE_SLUG
    go test -race -count=1 -timeout=10m -run '^TestLiveInstalledFlow$' -v ./plugins/xerj

Replace all example values before running. Use the actual disposable node port. The test creates two random fresh prefixes, prints them and the digest, and leaves the engine indices for inspection. Stop and dispose of the dedicated node/data after collecting evidence; it does not install software, launch a server, delete engine indices or use any real workspace data. Missing required configuration fails the opted-in test rather than skipping. Ordinary package tests skip it unless explicitly enabled.

Record candidate Git SHA, Go version, XERJ version and trusted SHA-256, OS/architecture, expected fixture slug, exact generated prefixes, exit code and complete test output. A failure at catalog, content verification, passage retrieval or post-cleanup retrieval is a compatibility blocker, not permission to bypass verification. TestInstalledFlowFixture runs the same lifecycle against a POSIX test executable and simulated HTTP server; this proves the acceptance harness, not upstream behavior. Real-engine acceptance, automatic watcher-to-worker installed integration, UI and full repository CI are separate gates.

## Workspace search presentation service

NewWorkspaceSearchService(manager, trustedWorkspaceID) creates an opt-in, workspace-bound read-only presentation service. A nil manager explicitly disables it. The application must authorize the workspace before creating the service and must not let a frontend or model select an arbitrary workspace, index or engine endpoint. This adds no core dependency or IPC registration.

Run service.Search(ctx, query, limit) off the UI thread. The request accepts only a nonblank query (at most 4096 bytes) and a limit of 1–20. The returned WorkspaceSearchResult has a stable state and a non-null hits array: MATCHES, EMPTY, DISABLED, INVALID_REQUEST, NOT_READY, STALE, UNAVAILABLE, CANCELED or TIMEOUT. Only successful retrieval can be EMPTY or MATCHES. No raw engine errors or partial hits are returned. This intentionally trades detailed error diagnosis for a simple presentation contract; logging/diagnostics remain an application integration concern.

The service does not perform authorization, start the engine, own the worker or manage UI request ordering. Cancel superseded UI requests and discard late replies before displaying them, even if retrieval succeeded at its linearization point. Discard the service when the workspace changes and revalidate authorization and source identity before opening a result. Treat hit path, line, index and text as untrusted display data; never use them directly as unrestricted file-open or execution instructions.

    service, err := xerj.NewWorkspaceSearchService(manager, approvedWorkspaceID)
    // Handle err before invoking on a worker goroutine.
    reply := service.Search(ctx, query, 5)
    // Present reply.State; render reply.Hits as plain text.

TestWorkspaceSearchStates, TestWorkspaceSearchDisabledAndInvalid and TestWorkspaceSearchDropsCanceledReply cover presentation behavior using a local interface double. TestWorkspaceSearchServiceManager is a repository integration test with the real Manager and an HTTP fixture. Native search panel, typed IPC, service lifecycle wiring and installed XERJ acceptance are separate gates.
