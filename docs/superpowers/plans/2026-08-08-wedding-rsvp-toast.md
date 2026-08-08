# Wedding RSVP Toast Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist and return RSVP toast participation and companion fields.

**Architecture:** Extend the shared `model.WeddingRSVP` response and persistence type. The repository already normalizes and JSON-serializes this model into bbolt; the existing HTTP route decodes and returns it.

**Tech Stack:** Go, bbolt, Silverlining, standard `testing` package.

## Global Constraints

- Do not add dependencies or endpoints.
- Trim `toast_with`, limit it to 160 runes, and clear it when `will_give_toast` is false.

---

### Task 1: Persist toast fields in RSVP

**Files:**
- Modify: `internal/http/routes/wedding_routes_test.go:30-49`
- Modify: `internal/model/wedding.go:16-25`
- Modify: `internal/store/wedding.go:383-400`

**Interfaces:**
- Consumes: `POST /wedding/rsvps` JSON fields `will_give_toast` (bool) and `toast_with` (string).
- Produces: `model.WeddingRSVP.WillGiveToast` and `model.WeddingRSVP.ToastWith` in the create response and persisted bbolt record.

- [ ] **Step 1: Write the failing HTTP test**

Add to the existing request body in `TestWeddingPublicSettingsAndCreateRSVP`:

```go
"will_give_toast": true,
"toast_with":       "  Мария Петрова  ",
```

Then require the response to contain:

```go
if !created.WillGiveToast || created.ToastWith != "Мария Петрова" {

    t.Fatalf("expected toast fields in response, got %#v", created)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `rtk go test ./internal/http/routes -run '^TestWeddingPublicSettingsAndCreateRSVP$'`

Expected: FAIL because `WeddingRSVP` has no toast fields.

- [ ] **Step 3: Add the minimal model and normalization**

Add to `WeddingRSVP`:

```go
WillGiveToast bool   `json:"will_give_toast"`
ToastWith     string `json:"toast_with"`
```

Add to `normalizeWeddingRSVP`:

```go
WillGiveToast: input.WillGiveToast,
ToastWith: truncateWeddingText(strings.TrimSpace(input.ToastWith), weddingTextMaxLength),
```

Then clear `rsvp.ToastWith` when `!rsvp.WillGiveToast`.

- [ ] **Step 4: Run the focused test to verify it passes**

Run: `rtk go test ./internal/http/routes -run '^TestWeddingPublicSettingsAndCreateRSVP$'`

Expected: PASS.

- [ ] **Step 5: Run the complete suite and commit**

Run: `rtk go test ./...`

Expected: PASS with exit code 0.

Commit:

```bash
rtk git add internal/model/wedding.go internal/store/wedding.go internal/http/routes/wedding_routes_test.go
rtk git commit -m "feat: store wedding RSVP toast details"
```
