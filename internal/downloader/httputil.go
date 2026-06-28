package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ohcass/radii5/internal/progress"
)

func probeURL(url string) (int64, bool, error) {
	resp, err := httpClient.Head(url)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	size := resp.ContentLength
	supportsRange := resp.Header.Get("Accept-Ranges") == "bytes"
	return size, supportsRange, nil
}

type chunkWriter struct {
	f      *os.File
	offset int64
	pos    int64
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	n, err := w.f.WriteAt(p, w.offset+w.pos)
	w.pos += int64(n)
	return n, err
}

type tpWriter struct {
	tp *TrackProgress
}

func (w *tpWriter) Write(p []byte) (int, error) {
	w.tp.Current.Add(int64(len(p)))
	return len(p), nil
}

func fetchWithRetry(url string, f *os.File, start, end int64, bar *progress.Bar, tp *TrackProgress) error {
	current := start
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := 1 << uint(min(attempt-1, 5))
			time.Sleep(time.Duration(delay) * time.Second)
		}
		written, err := fetchRangeToDisk(url, f, current, end, bar, tp)
		current += written
		if err == nil {
			return nil
		}
		if current > end {
			return nil
		}
	}
	return fmt.Errorf("failed after %d retries", maxRetries)
}

func fetchRangeToDisk(url string, f *os.File, start, end int64, bar *progress.Bar, tp *TrackProgress) (int64, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; radii5/0.1)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}

	cw := &chunkWriter{f: f, offset: start}
	var writers []io.Writer
	writers = append(writers, cw)
	if bar != nil {
		writers = append(writers, bar)
	}
	if tp != nil {
		writers = append(writers, &tpWriter{tp: tp})
	}

	buf := pool256k.Get().([]byte)
	defer pool256k.Put(buf)
	n, err := io.CopyBuffer(io.MultiWriter(writers...), resp.Body, buf)
	return n, err
}
