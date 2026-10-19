package weddingmedia

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Keep progressive viewing requests below the request deadline on slow links.
// This bounds transfer size, without allocating a video buffer or caching files.
func boundedVideoRange(value string) (string, error) {
	const chunkSize int64 = 2 * 1024 * 1024
	if value == "" {
		value = "bytes=0-"
	}
	if len(value) > 100 || !byteRange.MatchString(value) {
		return "", fail(416, "invalid byte range")
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if parts[0] == "" {
		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || size <= 0 {
			return "", fail(416, "invalid byte range")
		}
		return fmt.Sprintf("bytes=-%d", min(size, chunkSize)), nil
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", fail(416, "invalid byte range")
	}
	end := int64(math.MaxInt64)
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return "", fail(416, "invalid byte range")
		}
	}
	if end-start >= chunkSize {
		end = start + chunkSize - 1
	}
	return fmt.Sprintf("bytes=%d-%d", start, end), nil
}
