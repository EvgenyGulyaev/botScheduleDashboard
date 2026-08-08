# Go and Dependency Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the service to Go 1.26.5 and current compatible modules, then remove behavior-preserving static-analysis debt.

**Architecture:** Keep the existing application structure and APIs. Update module metadata first, prove compatibility with the full suite, then use the existing `staticcheck` failures as the red check for a deletion-first cleanup.

**Tech Stack:** Go 1.26.5, Go modules, `go test`, `go vet`, `staticcheck`, `govulncheck`

## Global Constraints

- Work directly on the existing `main` branch as explicitly requested.
- Do not add dependencies, abstractions, public APIs, or behavior changes.
- Retain the latest module version that passes the existing suite if a newest release is incompatible.

---

### Task 1: Upgrade Go and Modules

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: the existing Go module graph.
- Produces: a tidy module graph using Go 1.26.5 and current direct dependency releases.

- [ ] **Step 1: Record the passing baseline**

Run: `rtk go test ./...`
Expected: all packages pass.

- [ ] **Step 2: Update the toolchain and direct modules**

Run:

```bash
rtk go mod edit -go=1.26.0 -toolchain=go1.26.5
rtk go get github.com/SherClockHolmes/webpush-go@latest github.com/go-www/silverlining@latest github.com/gobwas/ws@latest github.com/golang-jwt/jwt/v5@latest github.com/joho/godotenv@latest github.com/nats-io/nats.go@latest go.etcd.io/bbolt@latest golang.org/x/crypto@latest
rtk go mod tidy
```

Expected: `go.mod` names the direct imports separately, and `go.sum` reflects the selected graph.

- [ ] **Step 3: Verify dependency compatibility**

Run: `rtk go test ./...`
Expected: all packages pass under Go 1.26.5.

### Task 2: Remove Static Analysis Debt

**Files:**
- Modify: `internal/chat/events.go`
- Modify: `internal/chat/hub_test.go`
- Modify: `internal/http/middleware/admin.go`
- Modify: `internal/http/middleware/jwt.go`
- Modify: `internal/http/routes/access_helpers.go`
- Modify: `internal/http/routes/admin_users_routes_test.go`
- Modify: `internal/http/routes/alice_routes_test.go`
- Modify: `internal/http/routes/postLogin.go`
- Modify: `internal/http/routes/postRegister.go`
- Modify: `internal/http/server.go`
- Modify: `internal/store/chat.go`

**Interfaces:**
- Consumes: the current private implementation and test helpers.
- Produces: identical public behavior with no `staticcheck` diagnostics.

- [ ] **Step 1: Confirm the red check**

Run: `rtk go run honnef.co/go/tools/cmd/staticcheck@latest ./...`
Expected: the known U1000, S1002, S1023, and ST1019 diagnostics fail the command.

- [ ] **Step 2: Apply the minimum cleanup**

Delete private declarations with no references:

- the eleven `gateway*Payload` aliases in `internal/chat/events.go`;
- `readGatewayEnvelope`, `(*Jwt).getEmailByToken`, `conversationMembersVisibleForUser`, `adminUsersResponse`, `trimMessages`, and `buildDeliveredTo`;
- one duplicate `net/http` import and replace `nethttp.` uses with `http.` in `alice_routes_test.go`.

Replace the three boolean comparisons:

```go
if !data.IsAdmin {
if !userValidator.Validate() {
if !mailValidator.Validate() {
```

Delete the final redundant `return` from `handleGet` and `handlePost`, then run:

```bash
rtk gofmt -w internal/chat/events.go internal/chat/hub_test.go internal/http/middleware/admin.go internal/http/middleware/jwt.go internal/http/routes/access_helpers.go internal/http/routes/admin_users_routes_test.go internal/http/routes/alice_routes_test.go internal/http/routes/postLogin.go internal/http/routes/postRegister.go internal/http/server.go internal/store/chat.go
```

- [ ] **Step 3: Verify the green check**

Run: `rtk go run honnef.co/go/tools/cmd/staticcheck@latest ./...`
Expected: exit 0 with no diagnostics.

### Task 3: Full Verification

**Files:**
- Verify: `go.mod`
- Verify: `go.sum`
- Verify: all modified Go files.

**Interfaces:**
- Consumes: Tasks 1 and 2.
- Produces: verified main-branch changes ready for handoff.

- [ ] **Step 1: Run all checks**

```bash
rtk go test ./...
rtk go vet ./...
rtk go run honnef.co/go/tools/cmd/staticcheck@latest ./...
rtk go run golang.org/x/vuln/cmd/govulncheck@latest ./...
rtk go mod tidy -diff
```

Expected: tests pass, vet/staticcheck are clean, no called vulnerabilities are found, and tidy produces no diff.

- [ ] **Step 2: Review the final diff**

Run: `rtk git diff --check && rtk git diff --stat && rtk git status --short`
Expected: no whitespace errors and only the planned files are modified.

