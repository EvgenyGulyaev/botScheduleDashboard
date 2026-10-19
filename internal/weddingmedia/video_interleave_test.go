package weddingmedia

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestVideoPreviewGroupsSamplesIntoSmallInterleavedChunks(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	dir := t.TempDir()
	input, output := filepath.Join(dir, "input.mp4"), filepath.Join(dir, "output.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fixture := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=size=96x64:rate=25", "-f", "lavfi", "-i", "sine=sample_rate=48000", "-t", "6", "-c:v", "libx264", "-threads", "1", "-c:a", "aac", input)
	if data, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	cmd := exec.CommandContext(ctx, ffmpeg, videoFFmpegArgs(input, output, videoProbe{})...)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("encode: %v %s", err, data)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	tracks, chunks, keyframes := 0, uint32(0), uint32(0)
	var inspect func([]byte)
	inspect = func(data []byte) {
		for len(data) >= 8 {
			size := int(binary.BigEndian.Uint32(data[:4]))
			if size < 8 || size > len(data) {
				t.Fatal("invalid MP4 box")
			}
			switch string(data[4:8]) {
			case "moov", "trak", "mdia", "minf", "stbl":
				inspect(data[8:size])
			case "stco":
				if size < 16 {
					t.Fatal("invalid chunk table")
				}
				tracks++
				chunks += binary.BigEndian.Uint32(data[12:16])
			case "stss":
				if size < 16 {
					t.Fatal("invalid keyframe table")
				}
				keyframes += binary.BigEndian.Uint32(data[12:16])
			}
			data = data[size:]
		}
	}
	inspect(data)
	// Six seconds of audio/video should use roughly 12 chunks per track,
	// rather than one chunk per video frame and hundreds of index entries.
	if tracks != 2 || chunks < 12 || chunks > 48 {
		t.Fatalf("chunk tables: tracks=%d chunks=%d; want 2 tracks and 12..48 chunks", tracks, chunks)
	}
	if keyframes < 3 {
		t.Fatalf("six-second preview has %d keyframes; HLS needs a keyframe at least every two seconds", keyframes)
	}
}
