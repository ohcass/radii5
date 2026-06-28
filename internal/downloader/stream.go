package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/ohcass/radii5/internal/progress"
)

func streamDownload(url, dest string, size int64, silent bool, tp *TrackProgress) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; radii5/0.1)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	var writers []io.Writer
	writers = append(writers, f)
	var bar *progress.Bar
	if !silent {
		bar = progress.NewBar(size)
		writers = append(writers, bar)
	}
	if tp != nil {
		writers = append(writers, &tpWriter{tp: tp})
	}

	buf := pool256k.Get().([]byte)
	defer pool256k.Put(buf)
	_, err = io.CopyBuffer(io.MultiWriter(writers...), resp.Body, buf)
	if bar != nil {
		bar.Finish()
	}
	return err
}
