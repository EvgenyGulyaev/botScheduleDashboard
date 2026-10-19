package weddingmedia

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func (s *Service) processVideoAndHLS(ctx context.Context, job videoJob) (string, error) {
	preview, err := s.processVideo(ctx, job)
	if err != nil {
		return "", err
	}
	hls, err := s.processVideoHLS(ctx, job, preview)
	if err != nil {
		return "", err
	}
	if err = s.setVideoJobHLS(job, hls); err != nil {
		return "", err
	}
	return preview, nil
}

// Remux after the original/MP4 work directory has been cleaned up, keeping
// peak disk use below the existing source + output + reserve budget.
func (s *Service) processVideoHLS(ctx context.Context, job videoJob, preview string) (hls VideoHLSPreview, err error) {
	if !validID.MatchString(preview) || preview == job.ID {
		return hls, errors.New("invalid HLS source")
	}
	res, err := s.get(ctx, s.baseURL+"/drive/v3/files/"+url.PathEscape(preview)+"?fields="+url.QueryEscape(videoMetadataFields))
	if err != nil {
		return hls, err
	}
	var source videoDriveFile
	err = decode(res.Body, &source)
	res.Body.Close()
	if err != nil || source.ID != preview || !s.validVideoCopy(source, job.ID, 0, videoConversionVersion) {
		return hls, errors.New("invalid compatible HLS source")
	}
	size, _ := strconv.ParseInt(source.Size, 10, 64)
	dir, err := s.videoWorkDir(job.ID, true)
	if err != nil {
		return hls, err
	}
	defer func() {
		if !errors.Is(err, errVideoUnitStopFailed) {
			_ = s.cleanupVideoJob(job)
		}
	}()
	var stat syscall.Statfs_t
	if err = syscall.Statfs(dir, &stat); err != nil {
		return hls, err
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if available > math.MaxInt64 {
		available = math.MaxInt64
	}
	if err = checkVideoSpace(size, int64(available)); err != nil {
		return hls, err
	}
	inputPath, outputPath, playlistPath := filepath.Join(dir, "input"), filepath.Join(dir, "hls.mp4"), filepath.Join(dir, "playlist.m3u8")
	input, err := os.OpenFile(inputPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return hls, err
	}
	err = s.downloadVideo(ctx, preview, size, input)
	closeErr := input.Close()
	if err != nil {
		return hls, err
	}
	if closeErr != nil {
		return hls, closeErr
	}
	hash := sha256.Sum256([]byte(job.ID))
	unit := "wedding-video-" + hex.EncodeToString(hash[:16]) + ".service"
	if err = s.setVideoJobUnit(job.ID, unit); err != nil {
		return hls, err
	}
	probe, err := s.probeVideo(ctx, inputPath, unit)
	if err != nil {
		return hls, err
	}
	if err = validateVideoOutput(probe, probe, size); err != nil {
		return hls, err
	}
	for _, path := range []string{outputPath, playlistPath} {
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if createErr != nil {
			return hls, createErr
		}
		if err = file.Close(); err != nil {
			return hls, err
		}
	}
	if err = s.runVideoCommand(ctx, unit, s.cfg.VideoProcessing.FFmpegPath, videoHLSFFmpegArgs(inputPath, outputPath, playlistPath), io.Discard, 15*time.Minute); err != nil {
		return hls, err
	}
	info, err := os.Lstat(outputPath)
	if err != nil || !info.Mode().IsRegular() {
		return hls, errors.New("invalid HLS output file")
	}
	output, err := s.probeVideo(ctx, outputPath, unit)
	if err != nil {
		return hls, err
	}
	if err = validateVideoOutput(probe, output, info.Size()); err != nil {
		return hls, err
	}
	playlistInfo, err := os.Lstat(playlistPath)
	if err != nil || !playlistInfo.Mode().IsRegular() || playlistInfo.Size() <= 0 || playlistInfo.Size() > 512<<10 {
		return hls, errors.New("invalid HLS output index")
	}
	data, err := os.ReadFile(playlistPath)
	if err != nil {
		return hls, err
	}
	playlist, err := parseVideoHLS(string(data))
	if err != nil {
		return hls, err
	}
	last := playlist.Segments[len(playlist.Segments)-1]
	if last.Offset+last.Length != info.Size() {
		return hls, errors.New("incomplete HLS output index")
	}
	// The layout digest prevents crash recovery from pairing an old file with
	// offsets produced by a different FFmpeg version or encoding profile.
	digest := sha256.Sum256(data)
	version := videoConversionVersion + "-hls1-" + hex.EncodeToString(digest[:16])
	id, err := s.findVideoCopy(ctx, job.ID, version)
	if err != nil {
		return hls, err
	}
	if id != "" {
		id, err = s.confirmVideoCopy(ctx, job.ID, id, info.Size(), version)
	} else {
		id, err = s.uploadVideoCopy(ctx, job.ID, outputPath, info.Size(), version)
	}
	if err != nil {
		return hls, err
	}
	return VideoHLSPreview{ID: id, Playlist: string(data)}, nil
}

func videoHLSFFmpegArgs(input, output, playlist string) []string {
	return []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-threads", "1", "-i", input, "-map", "0:v:0", "-map", "0:a:0?", "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy", "-hls_time", "4", "-hls_playlist_type", "vod", "-hls_segment_type", "fmp4", "-hls_flags", "single_file+independent_segments", "-hls_segment_filename", output, "-f", "hls", playlist}
}
