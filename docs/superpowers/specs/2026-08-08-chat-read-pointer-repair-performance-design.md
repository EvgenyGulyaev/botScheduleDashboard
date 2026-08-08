# Chat Read-Pointer Repair Performance Design

## Goal

Remove redundant message-bucket scans from message deletion, conversation clearing, and automatic trimming without changing stored data or public behavior.

## Approach

`DeleteMessage` and the trimming path already load the surviving messages before repairing member read pointers. `ClearConversationMessages` knows the surviving set is empty. Change `repairMemberReadPointersAfterTrim` to accept the surviving message slice instead of loading it internally, and pass the data already available at each of its three callers.

No new bucket, index, migration, dependency, or public API is introduced.

## Benchmark

Add a Go benchmark for `DeleteMessage` using a conversation with 1000 messages. Fixture creation and restoration are excluded from timed work; the measured operation is the repository deletion path. Record `ns/op`, `B/op`, and `allocs/op` before and after the change using identical benchmark settings.

## Correctness

Existing store and route tests remain the behavioral contract. The benchmark must use the real bbolt repository, and the full suite must pass after the helper signature and caller order change.

## Success Criteria

- The benchmark demonstrates a repeatable improvement in deletion time or allocations.
- `go test ./...`, `go vet ./...`, and `staticcheck` pass.
- The final diff is limited to the benchmark, the helper signature, and its three callers.

