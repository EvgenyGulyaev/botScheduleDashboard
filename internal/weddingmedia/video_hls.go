package weddingmedia

import (
	"errors"
	"fmt"
	"go.etcd.io/bbolt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type VideoHLSPreview struct {
	ID       string `json:"file_id"`
	Playlist string `json:"playlist"`
}

type videoHLSSegment struct {
	Offset, Length int64
	Duration       float64
}

type videoHLSPlaylist struct {
	Init     int64
	Segments []videoHLSSegment
}

func parseVideoHLS(raw string) (videoHLSPlaylist, error) {
	var p videoHLSPlaylist
	invalid := errors.New("invalid HLS playlist")
	if len(raw) > 512<<10 {
		return p, invalid
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 8 || lines[0] != "#EXTM3U" {
		return p, invalid
	}
	target, version, vod, independent, ended := 0, false, false, false, false
	duration := float64(0)
	var length, offset int64
	for _, text := range lines[1:] {
		line := strings.TrimSpace(text)
		if line == "" {
			continue
		}
		if ended {
			return p, invalid
		}
		switch {
		case line == "#EXT-X-VERSION:7":
			version = true
		case line == "#EXT-X-PLAYLIST-TYPE:VOD":
			vod = true
		case line == "#EXT-X-MEDIA-SEQUENCE:0":
		case line == "#EXT-X-INDEPENDENT-SEGMENTS":
			independent = true
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			var err error
			target, err = strconv.Atoi(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"))
			if err != nil || target <= 0 || target > 60 {
				return p, invalid
			}
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			m := hlsInitRange.FindStringSubmatch(line)
			if len(m) != 2 || p.Init != 0 || duration != 0 {
				return p, invalid
			}
			var err error
			p.Init, err = strconv.ParseInt(m[1], 10, 64)
			if err != nil || p.Init < 16 || p.Init > 256<<10 {
				return p, invalid
			}
			offset = p.Init
		case strings.HasPrefix(line, "#EXTINF:"):
			if p.Init == 0 || duration != 0 || !strings.HasSuffix(line, ",") {
				return p, invalid
			}
			var err error
			duration, err = strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ","), 64)
			if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 60 {
				return p, invalid
			}
		case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
			m := hlsSegmentRange.FindStringSubmatch(line)
			if len(m) != 3 || duration == 0 || length != 0 {
				return p, invalid
			}
			var err error
			length, err = strconv.ParseInt(m[1], 10, 64)
			if err != nil || length <= 0 || length > 2<<20 {
				return p, invalid
			}
			start, err := strconv.ParseInt(m[2], 10, 64)
			if err != nil || start != offset || offset > videoMaxOutput-length {
				return p, invalid
			}
		case line == "hls.mp4":
			if duration == 0 || length == 0 || len(p.Segments) >= 4096 {
				return p, invalid
			}
			p.Segments = append(p.Segments, videoHLSSegment{Offset: offset, Length: length, Duration: duration})
			offset += length
			duration, length = 0, 0
		case line == "#EXT-X-ENDLIST":
			ended = true
		default:
			return p, invalid
		}
	}
	if !version || !vod || !independent || !ended || target == 0 || len(p.Segments) == 0 || duration != 0 || length != 0 {
		return p, invalid
	}
	for _, segment := range p.Segments {
		if math.Round(segment.Duration) > float64(target) {
			return p, invalid
		}
	}
	return p, nil
}

var hlsInitRange = regexp.MustCompile(`^#EXT-X-MAP:URI="hls\.mp4",BYTERANGE="([0-9]+)@0"$`)
var hlsSegmentRange = regexp.MustCompile(`^#EXT-X-BYTERANGE:([0-9]+)@([0-9]+)$`)

func (p videoHLSPlaylist) render() string {
	target := 1
	for _, segment := range p.Segments {
		target = max(target, int(math.Ceil(segment.Duration)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"?format=hls-data\",BYTERANGE=\"%d@0\"\n", target, p.Init)
	for _, segment := range p.Segments {
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n#EXT-X-BYTERANGE:%d@%d\n?format=hls-data\n", segment.Duration, segment.Length, segment.Offset)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

func (s *Service) hlsState(id string) (VideoHLSPreview, bool) {
	if hls, ok := s.cfg.VideoHLSPreviews[id]; ok {
		return hls, true
	}
	if s.db == nil {
		return VideoHLSPreview{}, false
	}
	var job videoJob
	err := s.db.View(func(tx *bbolt.Tx) error { return decodeVideoJob(tx, id, &job) })
	if err != nil || job.FolderID != s.cfg.FolderID || job.Status != "ready" || job.HLS == nil || !validID.MatchString(job.HLS.ID) || job.HLS.ID == id {
		return VideoHLSPreview{}, false
	}
	return *job.HLS, true
}

func (s *Service) setVideoJobHLS(job videoJob, hls VideoHLSPreview) error {
	if !validID.MatchString(hls.ID) || hls.ID == job.ID {
		return errors.New("invalid HLS copy")
	}
	if _, err := parseVideoHLS(hls.Playlist); err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		var current videoJob
		if err := decodeVideoJob(tx, job.ID, &current); err != nil {
			return err
		}
		if current.Status != "processing" || current.Attempts != job.Attempts || current.FolderID != s.cfg.FolderID {
			return errors.New("obsolete HLS job result")
		}
		current.HLS = &hls
		return putVideoJob(tx, current)
	})
}
