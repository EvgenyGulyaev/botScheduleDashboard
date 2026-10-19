package weddingmedia

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const videoMaxOutput int64 = 512 << 20
const videoDiskReserve int64 = 1 << 30
const videoConversionVersion = "auto-h264720-v1"

var errVideoUnitStopFailed = errors.New("video processing unit may still be running")

type videoFormat struct {
	Duration string `json:"duration"`
}
type videoStream struct {
	CodecType     string `json:"codec_type"`
	CodecName     string `json:"codec_name"`
	Profile       string `json:"profile"`
	Level         int    `json:"level"`
	PixFmt        string `json:"pix_fmt"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	AvgFrameRate  string `json:"avg_frame_rate"`
	ColorTransfer string `json:"color_transfer"`
	Channels      int    `json:"channels"`
	SampleRate    string `json:"sample_rate"`
}
type videoProbe struct {
	Streams []videoStream `json:"streams"`
	Format  videoFormat   `json:"format"`
}

func (s *Service) processVideo(ctx context.Context, job videoJob) (previewID string, err error) {
	if !validID.MatchString(job.ID) || !validID.MatchString(s.cfg.VideoProcessing.FolderID) || s.cfg.VideoProcessing.FolderID == s.cfg.FolderID {
		return "", errors.New("invalid video processing identity")
	}
	original, err := s.metadata(ctx, job.ID)
	if err != nil {
		return "", errors.New("cannot validate original video")
	}
	size, err := strconv.ParseInt(original.Size, 10, 64)
	if err != nil || size <= 0 || size > MaxFileSize || original.ID != job.ID || !strings.HasPrefix(original.MIMEType, "video/") {
		return "", errors.New("invalid original video")
	}
	if id, err := s.findVideoCopy(ctx, job.ID); err != nil {
		return "", err
	} else if id != "" {
		return id, nil
	}
	dir, err := s.videoWorkDir(job.ID, true)
	if err != nil {
		return "", err
	}
	defer func() {
		if !errors.Is(err, errVideoUnitStopFailed) {
			_ = s.cleanupVideoJob(job)
		}
	}()
	var stat syscall.Statfs_t
	if err = syscall.Statfs(dir, &stat); err != nil {
		return "", errors.New("cannot check video work space")
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if available > math.MaxInt64 {
		available = math.MaxInt64
	}
	if err = checkVideoSpace(size, int64(available)); err != nil {
		return "", err
	}
	inputPath, outputPath := filepath.Join(dir, "input"), filepath.Join(dir, "output.mp4")
	input, err := os.OpenFile(inputPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return "", errors.New("cannot create video input")
	}
	err = s.downloadVideo(ctx, job.ID, size, input)
	closeErr := input.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", errors.New("cannot close video input")
	}
	hash := sha256.Sum256([]byte(job.ID))
	unit := "wedding-video-" + hex.EncodeToString(hash[:16]) + ".service"
	if err = s.setVideoJobUnit(job.ID, unit); err != nil {
		return "", err
	}
	probe, err := s.probeVideo(ctx, inputPath, unit)
	if err != nil {
		return "", err
	}
	if _, err = videoDuration(probe); err != nil {
		return "", err
	}
	hasVideo := false
	for _, stream := range probe.Streams {
		hasVideo = hasVideo || stream.CodecType == "video"
	}
	if !hasVideo {
		return "", errors.New("input has no video stream")
	}
	// Reserve the fixed output path without following a stale symlink.
	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", errors.New("cannot create video output")
	}
	if err = out.Close(); err != nil {
		return "", errors.New("cannot close video output")
	}
	if err = s.runVideoUnit(ctx, unit, videoFFmpegArgs(inputPath, outputPath, probe)); err != nil {
		return "", err
	}
	output, err := s.probeVideo(ctx, outputPath, unit)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(outputPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("invalid video output file")
	}
	if err = validateVideoOutput(probe, output, info.Size()); err != nil {
		return "", err
	}
	return s.uploadVideoCopy(ctx, job.ID, outputPath, info.Size())
}

func checkVideoSpace(size, available int64) error {
	if size <= 0 || size > MaxFileSize {
		return errors.New("invalid video source size")
	}
	if available < size+videoMaxOutput+videoDiskReserve {
		return fmt.Errorf("video processing needs more free disk: %w", errVideoProcessingDeferred)
	}
	return nil
}

func (s *Service) videoWorkDir(id string, create bool) (string, error) {
	if !validID.MatchString(id) || !filepath.IsAbs(s.cfg.VideoProcessing.WorkDir) {
		return "", errors.New("invalid video work directory")
	}
	root := filepath.Clean(s.cfg.VideoProcessing.WorkDir)
	if create {
		if err := os.MkdirAll(root, 0700); err != nil {
			return "", errors.New("cannot create video work directory")
		}
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("video work directory must be private")
	}
	dir := filepath.Join(root, id)
	if create {
		if err = os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", errors.New("cannot create video job directory")
		}
	}
	info, err = os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("video job directory must be private")
	}
	return dir, nil
}

func (s *Service) cleanupVideoJob(job videoJob) error {
	dir, err := s.videoWorkDir(job.ID, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, name := range []string{"input", "output.mp4"} {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return errors.New("unexpected video work file")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	}
	// Leave unknown files intact; never recursively remove the work directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(dir)
	}
	return nil
}

// FFprobe output is bounded even when the input contains many streams/tags.
type videoProbeWriter struct{ buffer bytes.Buffer }

func (w *videoProbeWriter) Write(p []byte) (int, error) {
	if w.buffer.Len()+len(p) > 64<<10 {
		return 0, errors.New("video probe output too large")
	}
	return w.buffer.Write(p)
}
func (w *videoProbeWriter) Bytes() []byte  { return w.buffer.Bytes() }
func (w *videoProbeWriter) String() string { return w.buffer.String() }
func (s *Service) probeVideo(ctx context.Context, path string, unit ...string) (videoProbe, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"-threads", "1", "-v", "error", "-show_entries", "format=duration:stream=codec_type,codec_name,profile,level,pix_fmt,width,height,avg_frame_rate,color_transfer,channels,sample_rate", "-of", "json", path}
	var out videoProbeWriter
	if len(unit) > 1 {
		return videoProbe{}, errors.New("invalid video probe unit")
	}
	if len(unit) == 1 {
		if err := s.runVideoCommand(ctx, unit[0], s.cfg.VideoProcessing.FFprobePath, args, &out, 30*time.Second); err != nil {
			return videoProbe{}, err
		}
	} else {
		// Direct execution is for local fixture checks; production always supplies its tracked unit.
		cmd := exec.CommandContext(ctx, s.cfg.VideoProcessing.FFprobePath, args...)
		cmd.Stdout = &out
		cmd.Stderr = io.Discard
		cmd.WaitDelay = 2 * time.Second
		if err := cmd.Run(); err != nil {
			return videoProbe{}, errors.New("video probe failed")
		}
	}
	var probe videoProbe
	if json.Unmarshal(out.Bytes(), &probe) != nil {
		return probe, errors.New("invalid video probe response")
	}
	return probe, nil
}

func videoFFmpegArgs(input, output string, probe videoProbe) []string {
	filter := "scale=w='trunc(iw*min(1,min(1280/max(iw,ih),720/min(iw,ih)))/2)*2':h='trunc(ih*min(1,min(1280/max(iw,ih),720/min(iw,ih)))/2)*2',"
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			if stream.ColorTransfer == "smpte2084" || stream.ColorTransfer == "arib-std-b67" {
				// Bound frames at 10-bit precision before allocating float RGB for tone mapping.
				filter += "format=yuv420p10le,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,"
			}
			break
		}
	}
	filter += "setsar=1,fps=fps='min(source_fps,30)',format=yuv420p"
	return []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-threads", "1", "-filter_threads", "1", "-i", input, "-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "-1", "-map_chapters", "-1", "-vf", filter, "-c:v", "libx264", "-preset", "veryfast", "-profile:v", "main", "-level:v", "3.1", "-pix_fmt", "yuv420p", "-threads", "1", "-maxrate", "2500k", "-bufsize", "5000k", "-c:a", "aac", "-profile:a", "aac_low", "-ac", "2", "-b:a", "128k", "-ar", "48000", "-movflags", "+faststart", "-fs", strconv.FormatInt(videoMaxOutput, 10), "-f", "mp4", output}
}

func validVideoUnit(unit string) bool {
	if !strings.HasPrefix(unit, "wedding-video-") || !strings.HasSuffix(unit, ".service") {
		return false
	}
	hash := strings.TrimSuffix(strings.TrimPrefix(unit, "wedding-video-"), ".service")
	decoded, err := hex.DecodeString(hash)
	return err == nil && len(decoded) == 16
}
func (s *Service) stopVideoUnit(ctx context.Context, unit string) error {
	if unit == "" {
		return nil
	}
	if !validVideoUnit(unit) {
		return errors.New("invalid video processing unit")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.VideoProcessing.SystemctlPath, "stop", unit)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if cmd.Run() != nil {
		// --collect unloads completed transient units; an absent unit is already stopped.
		show := exec.CommandContext(ctx, s.cfg.VideoProcessing.SystemctlPath, "show", "--property=LoadState", "--value", unit)
		var state videoProbeWriter
		show.Stdout = &state
		show.Stderr = io.Discard
		show.WaitDelay = 2 * time.Second
		_ = show.Run()
		if strings.TrimSpace(state.String()) != "not-found" {
			return errors.New("cannot stop video processing unit")
		}
	}
	return nil
}
func (s *Service) runVideoUnit(ctx context.Context, unit string, ffmpegArgs []string) error {
	return s.runVideoCommand(ctx, unit, s.cfg.VideoProcessing.FFmpegPath, ffmpegArgs, io.Discard, 4*time.Hour)
}

func (s *Service) runVideoCommand(ctx context.Context, unit, executable string, commandArgs []string, stdout io.Writer, runtime time.Duration) (err error) {
	if !validVideoUnit(unit) {
		return errors.New("invalid video processing unit")
	}
	defer func() {
		if err != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if stopErr := s.stopVideoUnit(stopCtx, unit); stopErr != nil {
				err = errors.Join(err, errVideoUnitStopFailed, stopErr)
			}
		}
	}()
	args := []string{"--quiet", "--wait", "--pipe", "--collect", "--service-type=exec", "--unit=" + unit, "--property=CPUQuota=50%", "--property=MemoryMax=192M", "--property=MemorySwapMax=128M", "--property=Nice=15", "--property=TasksMax=32", "--property=RuntimeMaxSec=" + strconv.FormatInt(int64(runtime/time.Second), 10), "--property=KillMode=control-group", "--", executable}
	args = append(args, commandArgs...)
	ctx, cancel := context.WithTimeout(ctx, runtime+time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.VideoProcessing.SystemdRunPath, args...)
	cmd.Stdout = stdout
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if cmd.Run() != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("video processing command failed")
	}
	return nil
}

func videoDuration(probe videoProbe) (float64, error) {
	d, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil || math.IsNaN(d) || math.IsInf(d, 0) || d <= 0 {
		return 0, errors.New("invalid video duration")
	}
	return d, nil
}
func validateVideoOutput(input, output videoProbe, size int64) error {
	if size <= 0 || size > videoMaxOutput {
		return errors.New("invalid video output size")
	}
	inDuration, err := videoDuration(input)
	if err != nil {
		return err
	}
	outDuration, err := videoDuration(output)
	if err != nil {
		return err
	}
	if math.Abs(inDuration-outDuration) > 2 {
		return errors.New("incomplete video output")
	}
	needAudio := false
	inputWidth, inputHeight := 0, 0
	for _, stream := range input.Streams {
		needAudio = needAudio || stream.CodecType == "audio"
		if stream.CodecType == "video" && inputWidth == 0 {
			inputWidth, inputHeight = stream.Width, stream.Height
		}
	}
	videos, audios := 0, 0
	for _, stream := range output.Streams {
		switch stream.CodecType {
		case "video":
			videos++
			if inputWidth > 0 && inputHeight > 0 && (max(stream.Width, stream.Height) > max(inputWidth, inputHeight) || min(stream.Width, stream.Height) > min(inputWidth, inputHeight)) {
				return errors.New("upscaled video output")
			}
			rate := strings.Split(stream.AvgFrameRate, "/")
			fps := math.NaN()
			if len(rate) == 2 {
				n, e1 := strconv.ParseFloat(rate[0], 64)
				d, e2 := strconv.ParseFloat(rate[1], 64)
				if e1 == nil && e2 == nil {
					fps = n / d
				}
			}
			if stream.CodecName != "h264" || stream.Profile != "Main" || stream.Level != 31 || stream.PixFmt != "yuv420p" || stream.Width <= 0 || stream.Height <= 0 || stream.Width%2 != 0 || stream.Height%2 != 0 || max(stream.Width, stream.Height) > 1280 || min(stream.Width, stream.Height) > 720 || math.IsNaN(fps) || math.IsInf(fps, 0) || fps <= 0 || fps > 30.001 {
				return errors.New("incompatible video output")
			}
		case "audio":
			audios++
			if stream.CodecName != "aac" || stream.Profile != "LC" || stream.Channels != 2 || stream.SampleRate != "48000" {
				return errors.New("incompatible video audio")
			}
		default:
			return errors.New("unexpected video output stream")
		}
	}
	if videos != 1 || audios > 1 || (needAudio && audios != 1) {
		return errors.New("missing video output stream")
	}
	return nil
}
