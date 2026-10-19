package weddingmedia

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const videoCopyProbeLimit = 64 << 20

func videoCopyMetadataEligible(p videoProbe, size int64) bool {
	if validateVideoOutput(p, p, size) != nil || !strings.Contains(","+p.Format.FormatName+",", ",mp4,") || p.Format.StartTime != "0.000000" {
		return false
	}
	for _, v := range p.Streams {
		bitrate, err := strconv.ParseInt(v.BitRate, 10, 64)
		if err != nil || bitrate <= 0 || v.StartTime != "0.000000" || len(v.SideData) != 0 || (v.Tags["rotate"] != "" && v.Tags["rotate"] != "0") {
			return false
		}
		if v.CodecType == "video" {
			fps := videoCopyRatio(v.RFrameRate)
			if bitrate > 500000 || fps <= 0 || fps > 30 || v.SampleAspectRatio != "1:1" || v.ColorTransfer != "bt709" || v.ColorSpace != "bt709" || v.ColorPrimaries != "bt709" || v.ColorRange != "tv" || v.FieldOrder != "progressive" {
				return false
			}
		} else if bitrate > 80000 {
			return false
		}
	}
	return true
}

func videoCopyRatio(s string) float64 {
	n, d, ok := strings.Cut(s, "/")
	if !ok {
		return 0
	}
	a, e1 := strconv.ParseFloat(n, 64)
	b, e2 := strconv.ParseFloat(d, 64)
	if e1 != nil || e2 != nil || !videoCopyFinite(a) || !videoCopyFinite(b) || a <= 0 || b <= 0 {
		return 0
	}
	return a / b
}

func videoCopyFinite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }

// Stream compact probe records; neither packet lists nor frame payloads are retained.
type videoCopyLines struct {
	buffer []byte
	total  int
	line   func(string) error
}

func (w *videoCopyLines) Write(p []byte) (int, error) {
	if len(p) > videoCopyProbeLimit-w.total {
		return 0, errors.New("video copy probe too large")
	}
	w.total += len(p)
	for i, b := range p {
		if b == '\n' {
			if err := w.line(string(w.buffer)); err != nil {
				return i, err
			}
			w.buffer = w.buffer[:0]
		} else {
			if len(w.buffer) >= 1024 {
				return i, errors.New("video copy probe line too large")
			}
			w.buffer = append(w.buffer, b)
		}
	}
	return len(p), nil
}

type videoCopyPackets struct {
	index                       int
	video                       bool
	count, bytes                int64
	lastDTS, end, bucket        float64
	currentBytes, previousBytes int64
}

func (s *Service) videoCopyEligible(ctx context.Context, unit, path string, p videoProbe, size int64) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if !videoCopyMetadataEligible(p, size) {
		return false, nil
	}
	duration, _ := videoDuration(p)
	// The HLS playlist admits at most 4096 two-second segments.
	if duration > 8192 {
		return false, nil
	}
	streams := make([]videoCopyPackets, len(p.Streams))
	for i, v := range p.Streams {
		streams[i] = videoCopyPackets{index: v.Index, video: v.CodecType == "video"}
	}
	invalid := errors.New("incompatible video copy packets")
	packets := videoCopyLines{line: func(line string) error {
		fields := make(map[string]string, 5)
		for _, field := range strings.Split(line, "|") {
			if field == "" {
				continue
			}
			key, value, ok := strings.Cut(field, "=")
			if !ok || fields[key] != "" {
				return invalid
			}
			fields[key] = value
		}
		index, e1 := strconv.Atoi(fields["stream_index"])
		pts, e2 := strconv.ParseFloat(fields["pts_time"], 64)
		dts, e3 := strconv.ParseFloat(fields["dts_time"], 64)
		d, e4 := strconv.ParseFloat(fields["duration_time"], 64)
		bytes, e5 := strconv.ParseInt(fields["size"], 10, 64)
		if len(fields) != 5 || e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || !videoCopyFinite(pts) || !videoCopyFinite(dts) || !videoCopyFinite(d) || pts < -0.05 || pts > duration || dts < -0.2 || dts > duration || d <= 0 || d > 1 || bytes <= 0 {
			return invalid
		}
		for i := range streams {
			v := &streams[i]
			if v.index != index {
				continue
			}
			peak := int64(20000)
			if v.video {
				peak = 125000
			}
			if bytes > peak || (v.video && d < 1.0/30-0.000002) || (v.count > 0 && (dts <= v.lastDTS || dts-v.lastDTS > 1.05 || (v.video && dts-v.lastDTS < 1.0/30-0.000002))) {
				return invalid
			}
			bucket := math.Floor(dts)
			if v.count == 0 || bucket != v.bucket {
				v.previousBytes = 0
				if v.count > 0 && bucket == v.bucket+1 {
					v.previousBytes = v.currentBytes
				}
				v.currentBytes, v.bucket = 0, bucket
			}
			v.currentBytes += bytes
			// Adjacent buckets bound every one-second delivery window, including bursts.
			if v.currentBytes+v.previousBytes > peak {
				return invalid
			}
			v.count++
			v.bytes += bytes
			v.lastDTS, v.end = dts, max(v.end, pts+d)
			return nil
		}
		return invalid
	}}
	args := []string{"-threads", "1", "-v", "error", "-show_packets", "-show_entries", "packet=stream_index,pts_time,dts_time,duration_time,size:packet_side_data=", "-of", "compact=p=0", path}
	if err := s.runVideoCommand(ctx, unit, s.cfg.VideoProcessing.FFprobePath, args, &packets, 30*time.Second); err != nil {
		return videoCopyProbeFailure(ctx, err)
	}
	if len(packets.buffer) != 0 {
		return false, nil
	}
	for _, v := range streams {
		bitrate := float64(80000)
		if v.video {
			bitrate = 500000
		}
		if v.count == 0 || math.Abs(v.end-duration) > 0.1 || float64(v.bytes)*8/duration > bitrate {
			return false, nil
		}
	}
	// HLS needs closed GOPs. Key flags also mark open-GOP recovery frames, so
	// retain only actual H.264 IDR NAL units, without decoding any frames.
	timebase, lastIDR, nextBoundary, count := float64(0), float64(0), float64(2), 0
	frameTime := float64(0)
	for _, stream := range p.Streams {
		if stream.CodecType == "video" {
			frameTime = 1 / videoCopyRatio(stream.RFrameRate)
		}
	}
	idrs := videoCopyLines{line: func(line string) error {
		if strings.HasPrefix(line, "#tb 0: ") {
			timebase = videoCopyRatio(strings.TrimSpace(strings.TrimPrefix(line, "#tb 0: ")))
			if timebase <= 0 {
				return invalid
			}
			return nil
		}
		if strings.HasPrefix(line, "#") {
			return nil
		}
		fields := strings.Split(line, ",")
		if len(fields) < 6 || strings.TrimSpace(fields[0]) != "0" || timebase <= 0 {
			return invalid
		}
		pts, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		t := pts * timebase
		if err != nil || !videoCopyFinite(t) || t < 0 || t > duration || (count == 0 && t > 0.05) || (count > 0 && (t <= lastIDR || t-lastIDR > 2.05)) {
			return invalid
		}
		// Match the encoder's keyframe at or just after each two-second
		// boundary; a merely short GOP can still delay four-second HLS cuts.
		if t > nextBoundary+frameTime+0.000002 {
			return invalid
		}
		if t >= nextBoundary-0.000002 {
			nextBoundary += 2
		}
		lastIDR, count = t, count+1
		return nil
	}}
	args = []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-threads", "1", "-i", path, "-map", "0:v:0", "-c:v", "copy", "-bsf:v", "h264_mp4toannexb,filter_units=pass_types=5", "-f", "framecrc", "-"}
	if err := s.runVideoCommand(ctx, unit, s.cfg.VideoProcessing.FFmpegPath, args, &idrs, 30*time.Second); err != nil {
		return videoCopyProbeFailure(ctx, err)
	}
	return len(idrs.buffer) == 0 && count > 0 && duration-lastIDR <= 2.05 && nextBoundary >= duration-frameTime-0.000002, nil
}

func videoCopyProbeFailure(ctx context.Context, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if errors.Is(err, errVideoUnitStopFailed) {
		return false, err
	}
	return false, nil
}

func videoCopyFFmpegArgs(input, output string) []string {
	return []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-threads", "1", "-i", input, "-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy", "-chunk_duration", "500000", "-movflags", "+faststart", "-fs", strconv.FormatInt(videoMaxOutput, 10), "-f", "mp4", output}
}

func (s *Service) convertVideo(ctx context.Context, unit, input, output string, probe videoProbe, sourceSize int64) (int64, error) {
	copy, err := s.videoCopyEligible(ctx, unit, input, probe, sourceSize)
	if err != nil {
		return 0, err
	}
	if copy {
		size, err := s.runVideoOutput(ctx, unit, output, probe, videoCopyFFmpegArgs(input, output))
		if err == nil {
			return size, nil
		}
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if errors.Is(err, errVideoUnitStopFailed) {
			return 0, err
		}
	}
	return s.runVideoOutput(ctx, unit, output, probe, videoFFmpegArgs(input, output, probe))
}

func (s *Service) runVideoOutput(ctx context.Context, unit, path string, input videoProbe, args []string) (int64, error) {
	if err := s.runVideoUnit(ctx, unit, args); err != nil {
		return 0, err
	}
	output, err := s.probeVideo(ctx, path, unit)
	if err != nil {
		return 0, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0, errors.New("invalid video output file")
	}
	if err := validateVideoOutput(input, output, info.Size()); err != nil {
		return 0, fmt.Errorf("invalid converted video: %w", err)
	}
	return info.Size(), nil
}
