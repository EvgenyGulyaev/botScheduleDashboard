# Background compatible video processing

## Approved behavior

After a guest video has finished uploading to Drive, persist a background job. The HTTP upload response does not wait for conversion. Closing the browser does not cancel processing. Images keep their existing upload path. Preserve the uploaded original for downloads and use the compatible copy for viewing when ready.

The gallery displays preparation state while a video is queued or processing. The viewer polls metadata while it is preparing, then enables playback of the ready version. Failed preparation leaves the original downloadable and displays a clear message. Existing manually prepared video mappings continue to work.

## Architecture and alternatives

Use the existing Go backend and Bolt database for a durable queue, with one worker. This avoids another database or broker. An external conversion service would add cost and another provider; conversion on the visitor's phone would stop when the browser closes and would vary by device. The approved VPS approach keeps processing independent of the visitor.

The VPS has one CPU, about 550 MiB RAM and approximately 4 GiB free disk. Run each FFmpeg task in a transient systemd service with CPUQuota=50%, MemoryMax=192M, MemorySwapMax=128M, Nice=15, TasksMax=32 and a four-hour runtime limit. Only one task runs at a time. Uploads and the API keep their existing service resources.

Download the original in bounded chunks into a private temporary directory. Before starting, require free space for the source, a 512 MiB preview ceiling and a 1 GiB reserve. Output is a seekable MP4 with faststart, H.264 Main level 3.1, yuv420p, at most 1280 pixels on the long edge / 720 on the short edge, at most 30 fps, and AAC-LC stereo. Preserve portrait orientation and avoid upscaling. HDR input is tone mapped to SDR. Verify output dimensions, codecs, duration and size before publishing; a size-limited partial encode must never become ready.

Upload the result resumably to the existing private viewing-copy Drive folder, outside the guest album. Store the ready ID in the jobs bucket, not the protected OAuth JSON. The static `video_previews` mapping remains a fallback for previously prepared files. Direct access to copies through the guest album endpoint remains denied.

## Durable state and recovery

The jobs bucket is keyed by original file ID. Its record stores original metadata, status (`queued`, `processing`, `ready`, `failed`), attempts, retry time, preview ID, timestamps and the current transient unit name. Completion of the original upload and creation of its job happen in the same Bolt transaction. Repeated completion/status requests do not enqueue duplicates.

The worker claims the oldest due job, persists the claim before external work, and retries failed work up to three total attempts with backoff. Insufficient free space defers the job without consuming an attempt. Service startup stops orphaned recorded units, removes only its own temporary job directory, and requeues interrupted work. Service shutdown cancels processing and joins the worker before closing Bolt.

Tag Drive copies with original ID and conversion version. Before encoding, look for an already completed matching copy. This recovers a crash after upload but before the ready transaction without creating another copy. Originals are never overwritten or deleted.

## API and frontend

Add optional `preview_status` to video metadata. Decorate cached list pages at response time so job status is not stuck behind the gallery metadata cache. Ready content supports the existing Range proxy. `?download=1` always uses the original. Queue records and errors do not expose credentials, upload session URLs, filesystem paths or raw FFmpeg logs to guests.

Preparing videos show their status in cards. The viewer displays preparation state with the existing download action, polls only while open and preparing, and stops polling when closed, changed or unmounted. Existing videos without a job keep their current behavior. No separate gallery item is created for a viewing copy.

## Validation and rollout

Test transactional enqueue/deduplication, photos excluded, ready mappings/download preservation, retries and restart recovery, storage deferral, cancellation, range preservation, verified output, private copy reuse, metadata state and frontend polling cleanup. Run full Go tests, media race tests, builds, frontend tests and production build. Verify a small disposable fixture through the production upload/job/ready path and inspect actual video playback, while leaving guest originals untouched. Deploy with processing initially disabled, install FFmpeg from Ubuntu repositories, enable the private configuration and verify limits, then enable automatic processing.
