package weddingmedia

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func copyCompatibleProbe() videoProbe {
	return videoProbe{Format: videoFormat{Duration: "6", FormatName: "mov,mp4,m4a,3gp,3g2,mj2", StartTime: "0.000000"}, Streams: []videoStream{{
		CodecType: "video", CodecName: "h264", Profile: "Main", Level: 31, PixFmt: "yuv420p", Width: 320, Height: 240,
		AvgFrameRate: "25/1", RFrameRate: "25/1", SampleAspectRatio: "1:1", ColorTransfer: "bt709", ColorSpace: "bt709", ColorPrimaries: "bt709", ColorRange: "tv", FieldOrder: "progressive", BitRate: "200000", StartTime: "0.000000",
	}}}
}

func TestVideoCopyMetadataRejectsUnknownOrIncompatibleSources(t *testing.T) {
	if !videoCopyMetadataEligible(copyCompatibleProbe(), 150000) {
		t.Fatal("compatible source rejected")
	}
	for name, mutate := range map[string]func(*videoProbe){
		"unknown bitrate": func(p *videoProbe) { p.Streams[0].BitRate = "" },
		"high bitrate":    func(p *videoProbe) { p.Streams[0].BitRate = "500001" },
		"high profile":    func(p *videoProbe) { p.Streams[0].Profile = "High" },
		"large frame":     func(p *videoProbe) { p.Streams[0].Width = 856 },
		"fast frames":     func(p *videoProbe) { p.Streams[0].RFrameRate = "60/1" },
		"HDR":             func(p *videoProbe) { p.Streams[0].ColorTransfer = "smpte2084" },
		"unknown SDR":     func(p *videoProbe) { p.Streams[0].ColorTransfer = "" },
		"wide gamut":      func(p *videoProbe) { p.Streams[0].ColorPrimaries = "bt2020" },
		"anamorphic":      func(p *videoProbe) { p.Streams[0].SampleAspectRatio = "2:1" },
		"rotation":        func(p *videoProbe) { p.Streams[0].Tags = map[string]string{"rotate": "90"} },
		"display matrix":  func(p *videoProbe) { p.Streams[0].SideData = []videoStreamSideData{{Type: "Display Matrix"}} },
		"interlaced":      func(p *videoProbe) { p.Streams[0].FieldOrder = "tt" },
		"nonzero start":   func(p *videoProbe) { p.Format.StartTime = "4" },
		"unknown format":  func(p *videoProbe) { p.Format.FormatName = "" },
		"extra stream":    func(p *videoProbe) { p.Streams = append(p.Streams, videoStream{CodecType: "subtitle"}) },
		"unknown audio":   func(p *videoProbe) { p.Streams = append(p.Streams, videoStream{CodecType: "audio"}) },
	} {
		t.Run(name, func(t *testing.T) {
			p := copyCompatibleProbe()
			mutate(&p)
			if videoCopyMetadataEligible(p, 150000) {
				t.Fatal("unsafe copy candidate accepted")
			}
		})
	}
	if videoCopyMetadataEligible(copyCompatibleProbe(), videoMaxOutput+1) {
		t.Fatal("oversized source accepted")
	}
}

func copyTestService(t *testing.T) *Service {
	t.Helper()
	s := &Service{}
	var err error
	s.cfg.VideoProcessing.FFmpegPath, err = exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	s.cfg.VideoProcessing.FFprobePath, err = exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	s.cfg.VideoProcessing.SystemdRunPath = videoTestScript(t, "while test \"$1\" != --; do shift; done\nshift\nexec \"$@\"\n")
	s.cfg.VideoProcessing.SystemctlPath = videoTestScript(t, "exit 0\n")
	return s
}

func copyTestFixture(t *testing.T, s *Service, gop string, extra ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.mp4")
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=size=320x240:rate=25", "-f", "lavfi", "-i", "sine=sample_rate=48000", "-t", "6", "-vf", "setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=limited", "-c:v", "libx264", "-threads", "1", "-profile:v", "main", "-level:v", "3.1", "-g", gop, "-sc_threshold", "0", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", "-ar", "48000", "-b:a", "64k"}
	args = append(args, extra...)
	args = append(args, path)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if data, err := exec.CommandContext(ctx, s.cfg.VideoProcessing.FFmpegPath, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, data)
	}
	return path
}

const copyTestUnit = "wedding-video-0123456789abcdef0123456789abcdef.service"

func TestVideoCopyVerifiesIDRCadenceAndRemuxes(t *testing.T) {
	s := copyTestService(t)
	for _, tc := range []struct {
		name, gop string
		extra     []string
		eligible  bool
	}{
		{"closed GOP every two seconds", "50", nil, true},
		{"long GOP", "150", nil, false},
		{"short GOP missing two-second boundaries", "37", nil, false},
		{"open GOP recovery frames", "50", []string{"-x264-params", "open-gop=1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := copyTestFixture(t, s, tc.gop, tc.extra...)
			probe, err := s.probeVideo(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(input)
			eligible, err := s.videoCopyEligible(context.Background(), copyTestUnit, input, probe, info.Size())
			if err != nil || eligible != tc.eligible {
				t.Fatalf("copy eligible=%v err=%v; want %v; %+v", eligible, err, tc.eligible, probe)
			}
			if !tc.eligible {
				return
			}
			output := filepath.Join(t.TempDir(), "output.mp4")
			if _, err = s.convertVideo(context.Background(), copyTestUnit, input, output, probe, info.Size()); err != nil {
				t.Fatal(err)
			}
			// Copy keeps the elementary video/audio packets byte for byte.
			packetHashes := func(path string) string {
				var hashes strings.Builder
				for _, stream := range []string{"v:0", "a:0"} {
					data, err := exec.Command(s.cfg.VideoProcessing.FFprobePath, "-v", "error", "-select_streams", stream, "-show_packets", "-show_entries", "packet=data_hash:packet_side_data=", "-show_data_hash", "sha256", "-of", "csv=p=0", path).CombinedOutput()
					if err != nil {
						t.Fatal(err)
					}
					hashes.Write(data)
				}
				return hashes.String()
			}
			if packetHashes(input) != packetHashes(output) {
				t.Fatal("compatible packets were re-encoded")
			}
			// Exercise the real HLS muxer against the copied MP4. Its cut at
			// four seconds must remain independent and match the saved ranges.
			dir := t.TempDir()
			hls, playlist := filepath.Join(dir, "hls.mp4"), filepath.Join(dir, "playlist.m3u8")
			if err := s.runVideoUnit(context.Background(), copyTestUnit, videoHLSFFmpegArgs(output, hls, playlist)); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(playlist)
			if err != nil {
				t.Fatal(err)
			}
			index, err := parseVideoHLS(string(raw))
			if err != nil || len(index.Segments) != 2 || index.Segments[0].Duration != 4 || index.Segments[1].Duration != 2 {
				t.Fatalf("copied HLS cadence: %+v %v", index, err)
			}
			media, err := os.ReadFile(hls)
			if err != nil {
				t.Fatal(err)
			}
			for _, segment := range index.Segments {
				if segment.Offset+segment.Length > int64(len(media)) {
					t.Fatal("HLS range beyond file")
				}
				part := append(append([]byte(nil), media[:index.Init]...), media[segment.Offset:segment.Offset+segment.Length]...)
				path := filepath.Join(dir, "segment.mp4")
				if err := os.WriteFile(path, part, 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(s.cfg.VideoProcessing.FFmpegPath, "-v", "error", "-xerror", "-i", path, "-f", "null", "-")
				if data, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("independent HLS segment: %v %s", err, data)
				}
			}
		})
	}
}

func TestVideoCopyFallsBackToEncodeAfterRemuxFailure(t *testing.T) {
	s := copyTestService(t)
	input := copyTestFixture(t, s, "50")
	probe, err := s.probeVideo(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(input)
	realFFmpeg := s.cfg.VideoProcessing.FFmpegPath
	s.cfg.VideoProcessing.FFmpegPath = videoTestScript(t, "for last do :; done\ncase \"$last\" in *output.mp4)\nfor arg do if test \"$arg\" = copy; then printf partial > \"$last\"; exit 1; fi; done;; esac\nexec '"+realFFmpeg+"' \"$@\"\n")
	output := filepath.Join(t.TempDir(), "output.mp4")
	if _, err = s.convertVideo(context.Background(), copyTestUnit, input, output, probe, info.Size()); err != nil {
		t.Fatal("copy failure prevented encoding", err)
	}
	out, err := s.probeVideo(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(output)
	if err = validateVideoOutput(probe, out, st.Size()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.convertVideo(ctx, copyTestUnit, input, output, probe, info.Size()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestVideoCopyFallsBackAfterInvalidRemuxOutput(t *testing.T) {
	s := copyTestService(t)
	input := copyTestFixture(t, s, "50")
	probe, err := s.probeVideo(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(input)
	realFFmpeg := s.cfg.VideoProcessing.FFmpegPath
	s.cfg.VideoProcessing.FFmpegPath = videoTestScript(t, "for last do :; done\ncase \"$last\" in *output.mp4)\nfor arg do if test \"$arg\" = copy; then exec '"+realFFmpeg+"' -t 1 \"$@\"; fi; done;; esac\nexec '"+realFFmpeg+"' \"$@\"\n")
	output := filepath.Join(t.TempDir(), "output.mp4")
	if _, err = s.convertVideo(context.Background(), copyTestUnit, input, output, probe, info.Size()); err != nil {
		t.Fatal("partial copy prevented encoding", err)
	}
	out, err := s.probeVideo(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(output)
	if err = validateVideoOutput(probe, out, st.Size()); err != nil {
		t.Fatal(err)
	}
}

func TestVideoCopyDoesNotFallbackWhileUnitMayStillRun(t *testing.T) {
	s := copyTestService(t)
	input := copyTestFixture(t, s, "50")
	probe, err := s.probeVideo(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(input)
	realFFmpeg := s.cfg.VideoProcessing.FFmpegPath
	s.cfg.VideoProcessing.FFmpegPath = videoTestScript(t, "for last do :; done\ncase \"$last\" in *output.mp4) exit 1;; esac\nexec '"+realFFmpeg+"' \"$@\"\n")
	s.cfg.VideoProcessing.SystemctlPath = videoTestScript(t, "exit 1\n")
	if _, err = s.convertVideo(context.Background(), copyTestUnit, input, filepath.Join(t.TempDir(), "output.mp4"), probe, info.Size()); !errors.Is(err, errVideoUnitStopFailed) {
		t.Fatalf("unsafe fallback: %v", err)
	}
}

func TestVideoCopyLineWriterBoundsAndPartialWrites(t *testing.T) {
	var got []string
	w := videoCopyLines{line: func(line string) error { got = append(got, line); return nil }}
	for _, p := range []string{"a", "b\nc", "d\n"} {
		if _, err := w.Write([]byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(got, ",") != "ab,cd" {
		t.Fatal(got)
	}
	if _, err := w.Write([]byte(strings.Repeat("x", 1025))); err == nil {
		t.Fatal("unbounded line accepted")
	}
	w = videoCopyLines{total: videoCopyProbeLimit, line: func(string) error { return nil }}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Fatal("unbounded output accepted")
	}
}

func TestVideoCopyPacketValidation(t *testing.T) {
	for name, mutate := range map[string]func([]string){
		"valid":                func([]string) {},
		"unknown stream":       func(lines []string) { lines[10] = "stream_index=2|pts_time=1|dts_time=1|duration_time=0.1|size=1000" },
		"non-finite timestamp": func(lines []string) { lines[10] = "stream_index=0|pts_time=NaN|dts_time=1|duration_time=0.1|size=1000" },
		"reordered DTS":        func(lines []string) { lines[10] = lines[9] },
		"excess frame rate":    func(lines []string) { lines[10] = "stream_index=0|pts_time=1|dts_time=1|duration_time=0.01|size=1000" },
		"peak bitrate":         func(lines []string) { lines[10] = "stream_index=0|pts_time=1|dts_time=1|duration_time=0.1|size=125001" },
		"truncated stream": func(lines []string) {
			for i := 30; i < len(lines); i++ {
				lines[i] = ""
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := copyTestService(t)
			lines := make([]string, 60)
			for i := range lines {
				lines[i] = fmt.Sprintf("stream_index=0|pts_time=%.1f|dts_time=%.1f|duration_time=0.1|size=1000", float64(i)/10, float64(i)/10)
			}
			mutate(lines)
			s.cfg.VideoProcessing.FFprobePath = videoTestScript(t, "cat <<'PACKETS'\n"+strings.Join(lines, "\n")+"\nPACKETS\n")
			s.cfg.VideoProcessing.FFmpegPath = videoTestScript(t, "printf '#tb 0: 1/25\\n0, 0, 0, 1, 100, 0\\n0, 50, 50, 1, 100, 0\\n0, 100, 100, 1, 100, 0\\n'\n")
			eligible, err := s.videoCopyEligible(context.Background(), copyTestUnit, "source.mp4", copyCompatibleProbe(), 150000)
			if err != nil || eligible != (name == "valid") {
				t.Fatalf("eligible=%v err=%v", eligible, err)
			}
		})
	}
}
