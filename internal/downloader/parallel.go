package downloader

import (
	"fmt"
	"os"
	"sync"

	"github.com/ohcass/radii5/internal/progress"
)

func parallelDownload(url, dest string, size int64, threads int, silent bool, tp *TrackProgress) error {
	if threads < 1 {
		threads = 1
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		return err
	}

	chunkSize := size / int64(threads)
	var wg sync.WaitGroup
	errCh := make(chan error, threads)
	var bar *progress.Bar
	if !silent {
		bar = progress.NewBar(size)
		bar.Set(0)
	}

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := int64(i) * chunkSize
			end := start + chunkSize - 1
			if i == threads-1 {
				end = size - 1
			}
			if err := fetchWithRetry(url, f, start, end, bar, tp); err != nil {
				errCh <- fmt.Errorf("chunk %d: %w", i, err)
			}
		}(i)
	}

	wg.Wait()
	f.Close()
	if bar != nil {
		bar.Finish()
	}
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}
