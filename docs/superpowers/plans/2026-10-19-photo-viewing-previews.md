# Photo viewing previews implementation plan

User approved all three optimizations and direct main commit/push. Originals remain intact; nginx must not change. Execute with subagent-driven-development, one implementer at a time, then review and production verification.

## Contracts and constraints

- Backend GET `/wedding/media/:id/preview` returns an authenticated Drive rendition up to1600px for images. Revalidate original membership on every request, preserve original `/content` and `?download=1`, never expose signed Drive URLs. Known thumbnail path suffix `=sNNN` becomes `=s1600`; unknown paths fail safely. Preserve existing HTTPS/googleusercontent allowlist. No new dependency or disk cache.
- Thumbnail and preview entries share at most8MiB and64entries; TTL remains one minute. Evict expired/least recently used entries individually. Cache at most512KiB per thumbnail and2MiB per large preview. A response too large/without known length may stream without caching. Private no-store response policy stays intact.
- Frontend `previewUrl(id)` uses the new endpoint. Normal photo viewing requests card thumbnail plus1600px preview, never original when a preview is available. Explicit zoom may request original; download always original. Missing/unavailable large preview leaves usable thumbnail and original download/zoom. Photos explicitly lacking thumbnails may use original foreground fallback; unsupported originals retain preview-only display.
- Visible and neighboring photos prefetch only previewUrl, sequentially; retain existing cancellation, data-saver and visibility gates. A large source size does not disqualify a lightweight preview. No original prefetch, UI library or style redesign.
- Every shell command uses rtk/rtk proxy. Preserve unrelated frontend README.md, AGENTS.md, output/, tmp/. Secrets never enter git/logs. Both repos main; user expressly authorized main push.

## Task1 Backend preview endpoint and bounded cache

Files: internal/weddingmedia/service.go, optional focused thumbnail_cache.go, thumbnail_cache_test.go. Extend existing service_test.go checks where appropriate.

- [x] RED: integration tests for1600px URL transformation, thumbnail/view-cache separation, original bytes unchanged, auth/folder checks on cache hits, unknown URL/video rejection, and20+entries without full cache reset. Test byte/entry cap and least-recently-used/expired eviction.
- [x] Implement preview route and image response using existing validated metadata/content path. Reuse cache helpers; never cache membership authorization.
- [x] GREEN: media package/race tests and diff check. Full Go tests/build and independent backend review also passed before release.

## Task2 Frontend lightweight viewing and prefetch

Files: src/lib/media-api.js, src/lib/media-photo-preload.js, src/composables/useMediaPhoto.js, src/components/MediaViewer.vue, src/components/WeddingMedia.vue, associated Node tests.

- [x] RED: API previewUrl contract; normal viewing/no original; original only explicit zoom; late preview cannot replace requested original; image decode/network failure fallback; HEIC/TIFF; sequential preview-only prefetch, cache reuse, cancellation/page/hidden/data-saver gates.
- [x] Implement previewUrl propagation, foreground preview and explicit original loading. Reuse existing cache leases and layer layout; maintain zoom, swipe, keyboard, download and video behavior.
- [x] GREEN: `rtk npm test -- --runInBand` (101 pass), `rtk npm run build`, diff check.

## Task3 Review, verify and release

- [x] Review combined diff and resolve findings. Backend `rtk go test ./...`, `rtk go test -race ./internal/weddingmedia`, `rtk go build ./...`; frontend required test/build commands. Review-found same-item reopen readiness issue fixed and independently retested.
- [x] Browser checks Chrome/WebKit desktop+mobile: opening and neighbor prefetch make no original request; zoom requests original once; repeat uses cache; download original, failures and navigation work. Both normal/reduced motion and close cancellation verified.
- [x] Commit/push backend main first, verify deployment and live preview size/privacy/original Range; then commit/push frontend main, verify deployment and live preview-only viewing. Nginx untouched. Guest files untouched; all production checks were read-only.

## Release evidence

- Backend `7586469`, frontend `d353059`: both main pushes and deployment workflows succeeded.
- Live photo: original2,372,709bytes, preview358,332bytes at1600x1200; original Range SHA-256 unchanged after backend rollout. Unauthenticated preview returned401; foreign private copy returned404.
- Live Chrome1440/WebKit390: displayed largepreview and two neighborpreviews, zero originalrequests during normal/repeated viewing, cached repeat62/64ms. Initial page-to-preview measurements9.4/15.8s include website/network loading and are not a universal speed guarantee.
- Local Chrome/WebKit1440/390: preview-only warmup/neighbors; zoomoriginal once; cached repeat; downloadbytes; HEIC; largepreviewfailure fallback; same-photo close/reopen and retained exit animation all passed.
- Final frontend101/101 tests and build passed. FullGo tests, media race tests and build passed. Independent task/final reviews approved after the close/reopen regression fix.
