# Background Video Processing Implementation Plan

> Execute inline with superpowers:executing-plans. Each task has a failing test, minimal implementation and verification before proceeding.

**Goal:** Automatically create a compatible viewing version after a video original is safely uploaded, without delaying uploads or losing jobs on restart.

**Architecture:** Existing Bolt database owns durable jobs and ready preview IDs. One Go worker invokes resource-limited FFmpeg tasks and stores validated copies privately on Drive. Guest metadata exposes preparation state; original downloads remain unchanged.

**Tech Stack:** Go, bbolt, existing Drive REST client, FFmpeg/ffprobe, systemd, Vue.

## Global constraints

- Follow the approved design in `docs/superpowers/specs/2026-10-19-video-background-processing-design.md`.
- Prefix shell commands with `rtk`. Keep protected credentials and Drive sessions out of Git and logs.
- One conversion at a time. CPUQuota=50%, MemoryMax=192M, MemorySwapMax=128M, Nice=15, TasksMax=32, runtime four hours.
- At most 1 GiB input and 512 MiB output; require a 1 GiB free disk reserve. Downloads always preserve the original.
- No new Go, UI or queue dependencies. Processing disabled until configured and verified.

## Task 1: Durable jobs and metadata contract

Files: create `internal/weddingmedia/video_jobs.go` and `video_jobs_test.go`; modify `service.go` and `video_preview_test.go`.

Interfaces: add `VideoProcessingConfig` to protected Config; add `Media.PreviewStatus`; define `videoJob` and a `video_jobs` Bolt bucket. Add `previewState(id string) (string, string)`, `decorateMedia(Media) Media`, and transaction-local `enqueueVideoJob(tx *bbolt.Tx, u *upload) error`.

- [ ] Write tests with a temporary persistent service: completing a video stores one queued job, a repeated completion stores no second job, images store no job, and ready jobs select the preview while downloads still select the original. Configure the new field through JSON so missing behavior fails assertions rather than compilation.
- [ ] Run `rtk go test ./internal/weddingmedia -run 'VideoJob|VideoPreview' -count=1`; confirm the new behavioral assertions fail.
- [ ] Implement job creation inside the existing upload persistence transaction. Repeated completion also persists before returning. Decorate item/list/summary-independent metadata at response time. Manual mappings retain priority and invalid IDs/configuration fail startup.
- [ ] Re-run targeted tests, then the full media package.

## Task 2: Worker lifecycle and retry state

Files: extend `video_jobs.go` and tests; create `video_worker.go` and tests; modify `service.go` lifecycle and `internal/http/routes/wedding_media.go` initialization.

Interfaces: define `type videoProcessor func(context.Context, videoJob) (string, error)` for a ready Drive ID; implement `StartVideoWorker()`, an internal injectable worker runner, oldest-due claim, durable retry/ready transitions, interruption recovery and cancellation.

- [ ] Test that a blocked fake processor does not delay upload completion, no two jobs process concurrently, failures back off and stop after three attempts, successful jobs become ready, restart requeues interrupted work, and Close joins before closing Bolt.
- [ ] Run the new worker tests and confirm failure.
- [ ] Implement one cancellable worker with a buffered wake signal plus bounded polling. Persist each transition; do not expose raw error text in public metadata. Deferral for storage does not consume an attempt.
- [ ] Run package and race tests.

## Task 3: Bounded conversion and private Drive publication

Files: create `video_processing.go`, `video_processing_test.go`, `video_drive.go`, `video_drive_test.go`; extend protected config validation and setup docs.

Interfaces: implement a production `videoProcessor`, input download in 8 MiB ranges, probe/FFmpeg argument builders, free-space validation, transient unit naming/cleanup, output validation and resumable private copy upload. Use appProperties for original ID and conversion version to reuse completed copies after interruption.

- [ ] Test exact input ranges/body sizes, controlled ffmpeg arguments (including portrait/no-upscale/HDR paths), resource limits, invalid duration/partial output rejection, upload offsets and completion, existing private copy reuse and stale response handling.
- [ ] Confirm tests fail before implementing their behavior.
- [ ] Implement private job directories (0700), fixed filenames, validated IDs and chunk buffers. Run subprocesses with a context; persist transient unit names before launch and stop recorded units on recovery/shutdown. Clean only worker-owned files.
- [ ] Implement output validation through ffprobe before upload. Preserve the source on every failure. Publish the ready ID only after Drive completion is confirmed.
- [ ] Verify with local generated fixtures, then full Go tests/race/build.

## Task 4: Gallery preparation state and viewer polling

Frontend files: modify `src/components/WeddingMedia.vue`, `src/components/MediaViewer.vue`; create a small video-preparation composable and tests if polling lifecycle needs extraction; extend existing media CSS only where necessary.

Contract: `preview_status` is empty for legacy behavior and otherwise `queued`, `processing`, `ready` or `failed`. One batch `/previews?ids=...` refreshes current-page and selected pending video status from Bolt without Drive calls. Closing/changing/unmounting cancels polling; ready state mounts the normal video player with the unchanged content URL.

- [ ] Write a runnable check for pending-to-ready polling, selected-file changes, close cleanup, transient network failures and failed preparation. Confirm failure first.
- [ ] Add the card status and viewer preparation/failed messages using existing typography and controls. Keep the original download action available.
- [ ] Run frontend tests/build and browser checks at mobile and desktop widths, including delayed preparation and navigation during polling.

## Task 5: Production enablement and end-to-end verification

- [ ] Review the full diff and run `rtk go test ./...`, `rtk go test -race ./internal/weddingmedia`, `rtk go build ./...`; frontend `rtk npm test -- --runInBand` and `rtk npm run build`.
- [ ] Install Ubuntu FFmpeg/ffprobe after checking package size and free disk. Inspect the actual ffmpeg build for x264, AAC and HDR filters. Configure the existing private viewing folder and a worker-owned temporary directory; retain 0600 credentials.
- [ ] Deploy backend/frontend to main with processing disabled, verify health, then enable processing and restart once.
- [ ] Upload a generated, clearly named disposable short video through the normal guest API. Verify original completion precedes processing; status changes to ready; copies stay outside the album; viewing uses the compatible stream and original download preserves the fixture bytes. Verify resource limits and cleanup. Do not modify guest originals.
- [ ] Document final checks, deployment commit and any actual limitations. Obtain the user's iPhone playback confirmation when available.
