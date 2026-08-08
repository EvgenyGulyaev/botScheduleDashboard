# Chat Read-Pointer Repair Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove redundant message decoding during delete, clear, and trim operations and prove the improvement with a real bbolt benchmark.

**Architecture:** Keep the current storage schema and public repository API. Pass each caller's already-known surviving messages into the private read-pointer repair helper instead of scanning the message bucket again.

**Tech Stack:** Go 1.26.5, testing benchmarks, bbolt

## Global Constraints

- Work directly on `main` as requested.
- Do not add buckets, migrations, dependencies, or public APIs.
- Preserve read-pointer repair behavior for deleted and surviving messages.

---

### Task 1: Add the Baseline Benchmark

**Files:**
- Modify: `internal/store/chat_test.go`
- Create: `internal/store/chat_benchmark_test.go`

**Interfaces:**
- Consumes: `ChatRepository.DeleteMessage`, the existing real bbolt test repository, and private persistence helpers.
- Produces: `BenchmarkDeleteMessage1000` reporting time and allocations for deleting one message from a 1000-message conversation.

- [ ] **Step 1: Generalize the existing repository helper**

Change the signature without changing its body:

```go
func newChatRepo(t testing.TB) *ChatRepository
```

- [ ] **Step 2: Add the benchmark**

Create `internal/store/chat_benchmark_test.go`:

```go
package store

import (
	"botDashboard/internal/model"
	"fmt"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func BenchmarkDeleteMessage1000(b *testing.B) {
	repo := newChatRepo(b)
	conversation, err := repo.CreateDirectConversation(
		model.ChatMember{Email: "alice@example.com", Login: "alice"},
		model.ChatMember{Email: "bob@example.com", Login: "bob"},
	)
	if err != nil {
		b.Fatalf("create conversation: %v", err)
	}

	var target model.ChatMessage
	err = repo.repo.Update(func(tx *bolt.Tx) error {
		for i := range 1000 {
			createdAt := time.Unix(int64(i), 0).UTC()
			message := model.ChatMessage{
				ID:             fmt.Sprintf("%06d", i),
				ConversationID: conversation.ID,
				SenderEmail:    "alice@example.com",
				SenderLogin:    "alice",
				Type:           "text",
				Text:           "benchmark message",
				CreatedAt:      createdAt,
				UpdatedAt:      createdAt,
			}
			if i > 0 && i <= 10 {
				message.ReplyToMessageID = "000000"
			}
			if i == 0 {
				target = message
			}
			if err := saveMessage(tx, message); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatalf("seed messages: %v", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		err := repo.repo.Update(func(tx *bolt.Tx) error {
			member, err := loadConversationMember(tx, conversation.ID, "bob@example.com")
			if err != nil {
				return err
			}
			member.LastReadMessageID = target.ID
			if err := saveMember(tx, member); err != nil {
				return err
			}
			return saveMessage(tx, target)
		})
		if err != nil {
			b.Fatalf("restore fixture: %v", err)
		}
		b.StartTimer()

		if _, err := repo.DeleteMessage(conversation.ID, target.ID, "alice@example.com"); err != nil {
			b.Fatalf("delete message: %v", err)
		}
	}
}
```

- [ ] **Step 3: Record the baseline**

Run: `rtk go test ./internal/store -run '^$' -bench '^BenchmarkDeleteMessage1000$' -benchmem -count=5`

Expected: five successful samples with `ns/op`, `B/op`, and `allocs/op`.

### Task 2: Reuse Surviving Messages

**Files:**
- Modify: `internal/store/chat.go`

**Interfaces:**
- Consumes: surviving `[]model.ChatMessage` already known by each caller.
- Produces: `repairMemberReadPointersAfterTrim(tx, conversationID, messages)` without internal message I/O.

- [ ] **Step 1: Confirm behavioral coverage before refactoring**

Run: `rtk go test ./internal/store`
Expected: all store tests pass.

- [ ] **Step 2: Change the private helper**

Change the signature and remove its internal `loadMessages` block:

```go
func repairMemberReadPointersAfterTrim(tx *bolt.Tx, conversationID string, messages []model.ChatMessage) error
```

- [ ] **Step 3: Update all three callers**

Use:

```go
repairMemberReadPointersAfterTrim(tx, conversationID, messages)
```

in `DeleteMessage`, use `nil` in `ClearConversationMessages`, and in `trimMessagesWithResult` define:

```go
remaining := messages[removeCount:]
```

then pass `remaining` to repair and last-message updates instead of reloading it.

- [ ] **Step 4: Verify behavior**

Run: `rtk go test ./internal/store`
Expected: all store tests pass.

### Task 3: Compare and Verify

**Files:**
- Verify: `internal/store/chat.go`
- Verify: `internal/store/chat_test.go`
- Verify: `internal/store/chat_benchmark_test.go`

**Interfaces:**
- Consumes: Tasks 1 and 2.
- Produces: benchmark evidence and verified repository behavior.

- [ ] **Step 1: Record the optimized benchmark**

Run: `rtk go test ./internal/store -run '^$' -bench '^BenchmarkDeleteMessage1000$' -benchmem -count=5`

Expected: a repeatable reduction in time or allocations against Task 1 using identical settings.

- [ ] **Step 2: Run full verification**

```bash
rtk go test ./...
rtk go vet ./...
GOTOOLCHAIN=go1.26.5 rtk go run honnef.co/go/tools/cmd/staticcheck@latest ./...
rtk git diff --check
```

Expected: all tests pass, vet/staticcheck are clean, and the diff has no whitespace errors.

