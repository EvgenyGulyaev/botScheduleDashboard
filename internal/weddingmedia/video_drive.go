package weddingmedia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
)

func (s *Service) downloadVideo(ctx context.Context, id string, size int64, file *os.File) error {
	if !validID.MatchString(id) || size <= 0 || size > MaxFileSize {
		return errors.New("invalid video download")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	next := int64(0)
	var firstErr error
	var wg sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buffer := make([]byte, ChunkSize)
			for {
				mu.Lock()
				start := next
				next += ChunkSize
				mu.Unlock()
				if start >= size || ctx.Err() != nil {
					return
				}
				end := min(start+ChunkSize, size) - 1
				err := func() error {
					req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/drive/v3/files/"+url.PathEscape(id)+"?alt=media", nil)
					if err != nil {
						return errors.New("cannot create video download")
					}
					req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
					res, err := s.do(req)
					if err != nil {
						return errors.New("video download failed")
					}
					defer res.Body.Close()
					length := end - start + 1
					if res.StatusCode != 206 || res.Header.Get("Content-Range") != fmt.Sprintf("bytes %d-%d/%d", start, end, size) || (res.ContentLength >= 0 && res.ContentLength != length) {
						return errors.New("invalid video download range")
					}
					data := buffer[:int(length)]
					if _, err = io.ReadFull(res.Body, data); err != nil {
						return errors.New("incomplete video download")
					}
					var extra [1]byte
					n, readErr := res.Body.Read(extra[:])
					if n != 0 || readErr != io.EOF {
						return errors.New("invalid video download body")
					}
					if n, err := file.WriteAt(data, start); err != nil || n != len(data) {
						return errors.New("cannot save video input")
					}
					return nil
				}()
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					cancel()
					return
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

type videoDriveFile struct {
	driveFile
	AppProperties map[string]string `json:"appProperties"`
}

const videoMetadataFields = metadataFields + ",appProperties"

func (s *Service) validVideoCopy(f videoDriveFile, originalID string, size int64, version string) bool {
	actual, err := strconv.ParseInt(f.Size, 10, 64)
	if err != nil || actual <= 0 || actual > videoMaxOutput || (size > 0 && actual != size) || !validID.MatchString(f.ID) || f.ID == originalID || f.Trashed || f.MIMEType != "video/mp4" || f.AppProperties["wedding_original_id"] != originalID || f.AppProperties["wedding_conversion_version"] != version {
		return false
	}
	private := false
	for _, parent := range f.Parents {
		if parent == s.cfg.FolderID {
			return false
		}
		private = private || parent == s.cfg.VideoProcessing.FolderID
	}
	return private
}

func (s *Service) confirmVideoCopy(ctx context.Context, originalID, id string, size int64, version string) (string, error) {
	if !validID.MatchString(id) {
		return "", errors.New("invalid video copy identity")
	}
	res, err := s.get(ctx, s.baseURL+"/drive/v3/files/"+url.PathEscape(id)+"?fields="+url.QueryEscape(videoMetadataFields))
	if err != nil {
		return "", errors.New("cannot confirm video copy")
	}
	defer res.Body.Close()
	var file videoDriveFile
	if res.StatusCode != 200 || decode(res.Body, &file) != nil || file.ID != id || !s.validVideoCopy(file, originalID, size, version) {
		return "", errors.New("invalid completed video copy")
	}
	return id, nil
}

func (s *Service) findVideoCopy(ctx context.Context, originalID, version string) (string, error) {
	// Both IDs are validated before interpolation into Drive's query language.
	if !validID.MatchString(originalID) || !validID.MatchString(s.cfg.VideoProcessing.FolderID) || !validID.MatchString(version) || len(version) > 80 {
		return "", errors.New("invalid video copy query")
	}
	query := fmt.Sprintf("'%s' in parents and trashed = false and appProperties has { key='wedding_original_id' and value='%s' } and appProperties has { key='wedding_conversion_version' and value='%s' }", s.cfg.VideoProcessing.FolderID, originalID, version)
	params := url.Values{"q": {query}, "fields": {"nextPageToken,incompleteSearch,files(" + videoMetadataFields + ")"}, "pageSize": {"100"}}
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		res, err := s.get(ctx, s.baseURL+"/drive/v3/files?"+params.Encode())
		if err != nil {
			return "", errors.New("cannot search video copies")
		}
		var result struct {
			Files      []videoDriveFile `json:"files"`
			Next       string           `json:"nextPageToken"`
			Incomplete bool             `json:"incompleteSearch"`
		}
		err = decode(res.Body, &result)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || result.Incomplete {
			return "", errors.New("invalid video copy search response")
		}
		for _, file := range result.Files {
			if s.validVideoCopy(file, originalID, 0, version) {
				return s.confirmVideoCopy(ctx, originalID, file.ID, 0, version)
			}
		}
		if result.Next == "" {
			return "", nil
		}
		if len(result.Next) > 2048 || seen[result.Next] {
			return "", errors.New("invalid video copy pagination")
		}
		seen[result.Next] = true
		params.Set("pageToken", result.Next)
	}
	return "", errors.New("video copy search exceeded page limit")
}

func (s *Service) uploadVideoCopy(ctx context.Context, originalID, path string, size int64, version string) (string, error) {
	if !validID.MatchString(originalID) || !validID.MatchString(s.cfg.VideoProcessing.FolderID) || s.cfg.VideoProcessing.FolderID == s.cfg.FolderID || size <= 0 || size > videoMaxOutput || !validID.MatchString(version) || len(version) > 80 {
		return "", errors.New("invalid video copy upload")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open video copy")
	}
	defer file.Close()
	body, _ := json.Marshal(map[string]any{"name": originalID + "-preview.mp4", "mimeType": "video/mp4", "parents": []string{s.cfg.VideoProcessing.FolderID}, "appProperties": map[string]string{"wedding_original_id": originalID, "wedding_conversion_version": version}})
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/upload/drive/v3/files?uploadType=resumable&fields="+url.QueryEscape(videoMetadataFields), bytes.NewReader(body))
	if err != nil {
		return "", errors.New("cannot create video upload")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Upload-Content-Type", "video/mp4")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))
	res, err := s.do(req)
	if err != nil {
		return "", errors.New("cannot start video upload")
	}
	res.Body.Close()
	target := res.Header.Get("Location")
	location, err := url.Parse(target)
	base, baseErr := url.Parse(s.baseURL)
	if err != nil || baseErr != nil || res.StatusCode != 200 || location.Scheme != base.Scheme || location.Host != base.Host || location.User != nil || !strings.HasPrefix(location.Path, "/upload/") || location.Fragment != "" || location.Opaque != "" {
		return "", errors.New("invalid video upload endpoint")
	}
	buffer := make([]byte, ChunkSize)
	offset := int64(0)
	failures := 0
	for offset < size {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		length := min(ChunkSize, size-offset)
		data := buffer[:int(length)]
		if n, err := file.ReadAt(data, offset); err != nil || int64(n) != length {
			return "", errors.New("cannot read video copy")
		}
		end := offset + length
		response, requestErr := s.videoUploadRequest(ctx, target, fmt.Sprintf("bytes %d-%d/%d", offset, end-1, size), data)
		if requestErr != nil {
			// An accepted chunk may lose its response; ask Drive before resending bytes.
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			failures++
			if failures > 3 {
				return "", errors.New("video upload repeatedly interrupted")
			}
			response, requestErr = s.videoUploadRequest(ctx, target, fmt.Sprintf("bytes */%d", size), nil)
			if requestErr != nil {
				return "", errors.New("cannot recover video upload offset")
			}
		}
		if response.StatusCode == 200 || response.StatusCode == 201 {
			if end != size {
				response.Body.Close()
				return "", errors.New("premature video upload completion")
			}
			var completed videoDriveFile
			err = decode(response.Body, &completed)
			response.Body.Close()
			if err != nil || !s.validVideoCopy(completed, originalID, size, version) {
				return "", errors.New("invalid video upload completion")
			}
			return s.confirmVideoCopy(ctx, originalID, completed.ID, size, version)
		}
		if response.StatusCode != 308 {
			response.Body.Close()
			return "", errors.New("unexpected video upload response")
		}
		confirmed, parseErr := videoUploadOffset(response.Header.Get("Range"), size)
		response.Body.Close()
		if parseErr != nil || confirmed < offset || confirmed > end {
			return "", errors.New("invalid video upload offset")
		}
		if confirmed == offset {
			failures++
			if failures > 3 {
				return "", errors.New("video upload made no progress")
			}
		} else {
			failures = 0
		}
		offset = confirmed
	}
	return "", errors.New("video upload has no completion metadata")
}

func (s *Service) videoUploadRequest(ctx context.Context, target, contentRange string, data []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", target, bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("cannot create video upload request")
	}
	req.Header.Set("Content-Range", contentRange)
	if len(data) > 0 {
		req.Header.Set("Content-Type", "video/mp4")
	}
	res, err := s.do(req)
	if err != nil {
		return nil, errors.New("video upload request failed")
	}
	return res, nil
}

func videoUploadOffset(value string, size int64) (int64, error) {
	if value == "" {
		return 0, nil
	}
	if !strings.HasPrefix(value, "bytes=0-") {
		return 0, errors.New("invalid upload range")
	}
	end, err := strconv.ParseInt(strings.TrimPrefix(value, "bytes=0-"), 10, 64)
	if err != nil || end < 0 || end >= size {
		return 0, errors.New("invalid upload range")
	}
	return end + 1, nil
}
