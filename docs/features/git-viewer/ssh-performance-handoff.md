# SSH performance handoff

Date: 2026-11-01
Repository: git-viewer-planning-mygo-native-v1.3

## Follow-up implementation

The connection-reuse stage has now been implemented and verified with full tests, targeted race tests, loopback SSH/SFTP, vet, local build and Intel packaging. See [execution record](ssh-performance-execution.md) for current behavior and limits. The historical state below describes the starting point, not the current implementation. The real-host performance comparison and optional Go helper remain outstanding.

The implementation uses application-owned foreground masters, short private socket directories, shared Git/SFTP connections, isolated Git writes and bounded concurrent ordinary reads. The Shell batch protocol below has been removed.

## Historical state at handoff

The read-only SSH path was not ready to ship as-is.

- `internal/infrastructure/ssh/executor.go` now has OpenSSH ControlMaster reuse for read-only work. Writes intentionally stay on independent SSH processes so cancellation and hook state remain verifiable.
- `internal/infrastructure/ssh/batch.go` is work in progress. It contains debugging `println` calls and is not yet correct.
- `TestRemoteReaderReadsHistoryAndCommitDiff` currently fails on the two-command CommitDetails batch.
- Do not revert the dirty worktree. Several UI, Makefile, terminal, diff, and theme changes predate this work.

## Measured baseline

Target: `n1:/root/prj/lotos`, Git 2.25.1.

- A fresh SSH process is about 0.48-0.63 seconds.
- Five fresh SSH Git queries took 2.483 seconds.
- Five queries through ControlMaster took 0.446 seconds after the master was established.
- Opening a branch previously issued eight SSH connections.
- Selecting a commit previously issued three or four more SSH connections.
- Opening the SSH repository also creates an SFTP session, which is separate from the Git read path.

The slowdown is connection setup, not remote Git execution. OpenSSH ControlPath has a hard UNIX socket length limit (104 bytes on macOS). A path under `TMPDIR` is too long here; a short path in `~/.ssh` works.

## Recommended implementation

Do not continue the current `sh -c` batch protocol. Prefer a persistent ControlMaster and ordinary fixed Git operations first.

1. Keep `ConnectionArgs()` and the write path unchanged.
2. Use a short, unique `~/.ssh/gv-<hash>` ControlPath. Establish it with `ssh -N -f -o ControlMaster=yes -o ControlPath=PATH -o ControlPersist=300 ...` before read calls, or manage the master explicitly.
3. Pass `-o ControlPath=PATH` on read-only Git and SFTP calls.
4. Keep a stderr buffer while starting the master. `-f` can return success even if the eventual authentication fails.
5. Remove a socket after failure or after `ssh -O exit`.
6. Keep the write path isolated until the cancellation semantics have an integration test for a multiplexed connection.

This alone should reduce repeated 0.5-second handshakes to about 0.03-0.09 seconds. Measure before adding another protocol layer.

If batching is still needed after that:

- Use a persistent stdin/stdout request/response protocol, not command strings crossing multiple shell layers.
- Use a simple binary framing format with a fixed magic value and a fixed-length payload count, or NUL framing with a binary parser.
- Cover NUL bytes, empty output, non-zero status, output limits, cancellation, and a very long path in tests.
- Run the loopback SSH integration test. The fixture must model how OpenSSH joins remote words, rather than passing them as local argv.

## Performance work already present

- `LocalReader.Batch` is in `internal/infrastructure/git/reader.go`.
- `ListRefs` in `internal/infrastructure/git/lists.go` groups branch, tag, and stash reads.
- CommitDetails in `internal/infrastructure/git/commit.go` attempts to group message and changed files.
- The UI history load calls `RefLists` in `internal/ui/local/features.go`.
- `internal/infrastructure/git` tests pass with the current changes.

The Git package is not the blocker. The SSH transport and its test fixture are.

## Verification before resuming

```sh
.tools/go/bin/go test ./internal/infrastructure/ssh -count=1
.tools/go/bin/go test ./internal/infrastructure/git -count=1
.tools/go/bin/go test ./... -count=1
.tools/go/bin/go vet ./...
git diff --check
make build
.tools/go/bin/go tool mygo build -platform darwin/amd64 -skip-dmg
```

Do not run the current SSH package tests expecting success before fixing the batch fixture. They can also leave an integration-test sshd draining for several seconds on shutdown; inspect for leftover test processes if needed.

After a clean build, run the packaged app with the real host and compare:

- first open time;
- time to first history page;
- branch switch time;
- commit selection to diff time;
- Source Control refresh time.

The five-query ControlMaster benchmark is the target baseline for repeated reads.
