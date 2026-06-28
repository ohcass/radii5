package downloader

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ohcass/radii5/internal/metadata"
	"github.com/ohcass/radii5/internal/progress"
)

type PlaylistEntry struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	WebpageURL    string `json:"webpage_url"`
	PlaylistTitle string `json:"playlist_title"`
}

type resolvedEntry struct {
	entry PlaylistEntry
	info  *VideoInfo
	err   error
}

func ResolvePlaylist(playlistURL string) ([]PlaylistEntry, string, error) {
	ytdlp := findBin("yt-dlp")
	cmd := exec.Command(ytdlp,
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		playlistURL,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", err
	}
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("yt-dlp not found")
	}

	var entries []PlaylistEntry
	playlistTitle := ""
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e PlaylistEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.WebpageURL == "" && e.ID != "" {
			e.WebpageURL = "https://youtube.com/watch?v=" + e.ID
		}
		if e.WebpageURL != "" {
			entries = append(entries, e)
		}
		if playlistTitle == "" && e.PlaylistTitle != "" {
			playlistTitle = e.PlaylistTitle
		}
	}

	if playlistTitle == "" {
		playlistTitle = "playlist"
	}

	return entries, playlistTitle, nil
}



func runBatch(entries []PlaylistEntry, format, outputDir string, threads, workers int, mediaType string, quality int,
	done *atomic.Int64, failed *atomic.Int64, total int64, json bool, retrying bool, spinner *progress.Spinner, playlistTitle string) []PlaylistEntry {

	type result struct {
		entry PlaylistEntry
		err   error
	}

	resolveQueue := make(chan PlaylistEntry, len(entries))
	downloadQueue := make(chan resolvedEntry, workers*3)
	results := make(chan result, len(entries))

	for _, e := range entries {
		resolveQueue <- e
	}
	close(resolveQueue)

	slots := make([]*TrackProgress, workers)
	for i := range slots {
		slots[i] = &TrackProgress{}
	}

	var (
		mu       sync.Mutex
		barPos   = 11
		barDir   = -1
		barCycle = 0
		barWait  = 0
		dispPct  = 0
		dispTime = time.Now()
	)

	render := func() {
		d := done.Load()

		if barWait > 0 {
			barWait--
		} else {
			barPos += barDir
			lo := -9
			hi := 11
			if barPos > hi {
				barDir = -1
				barCycle++
				if barCycle%1 == 0 {
					barWait = 20
				}
			} else if barPos < lo {
				barDir = 1
			}
		}

		var sb strings.Builder
		sb.Grow(8 * 12)
		cols := [][]string{
			{
				"\033[38;2;104;163;235m",
				"\033[38;2;101;157;248m",
				"\033[38;2;70;105;165m",
				"\033[38;2;48;73;110m",
				"\033[38;2;36;51;74m",
				"\033[38;2;26;37;52m",
			},
			{
				"\033[38;2;235;150;70m",
				"\033[38;2;200;125;55m",
				"\033[38;2;165;100;45m",
				"\033[38;2;130;75;35m",
				"\033[38;2;95;55;25m",
				"\033[38;2;60;35;15m",
			},
		}
		pal := 0
		if retrying {
			pal = 1
		}
		for i := 0; i < 8; i++ {
			bi := i - barPos
			if bi >= 0 && bi < 6 {
				idx := bi
				if barDir > 0 {
					idx = 5 - bi
				}
				sb.WriteString(cols[pal][idx])
				sb.WriteString("■")
			} else {
				sb.WriteString("\033[38;5;239m·")
			}
		}
		sb.WriteString("\033[0m")

		pct := dispPct
		if total > 0 {
			partial := 0.0
			for _, tp := range slots {
				cur := tp.Current.Load()
				ttl := tp.Total.Load()
				if ttl > 0 {
					partial += float64(cur) / float64(ttl)
				}
			}
			rawPct := int((float64(d) + partial) / float64(total) * 100)
			if rawPct > 100 {
				rawPct = 100
			}
			if rawPct > dispPct {
				now := time.Now()
				elapsed := now.Sub(dispTime).Seconds()
				rate := float64(rawPct-dispPct) / elapsed
				step := 1
				switch {
				case rate > 50:
					step = 10
				case rate > 10:
					step = 5
				}
				disp := (rawPct / step) * step
				if disp > dispPct || rawPct >= 100 {
					dispPct = disp
					dispTime = now
				}
			}
			pct = dispPct
		}

		fmt.Printf("\033[2K\r  %s \033[1m%d%%\033[0m", sb.String(), pct)
	}

	const resolvers = 8
	var resolveWg sync.WaitGroup
	for i := 0; i < resolvers; i++ {
		resolveWg.Add(1)
		go func() {
			defer resolveWg.Done()
			for entry := range resolveQueue {
				info, err := resolve(entry.WebpageURL, mediaType, quality)
				downloadQueue <- resolvedEntry{entry: entry, info: info, err: err}
			}
		}()
	}

	go func() {
		resolveWg.Wait()
		close(downloadQueue)
	}()

	var dlWg sync.WaitGroup
	for i := 0; i < workers; i++ {
		dlWg.Add(1)
		slotIdx := i
		go func() {
			defer dlWg.Done()
			tp := slots[slotIdx]
			for re := range downloadQueue {
				if re.err != nil {
					tp.Reset(re.entry.Title, 0, re.entry.WebpageURL)
					tp.Failed.Store(true)
					results <- result{entry: re.entry, err: fmt.Errorf("could not resolve URL: %w", re.err)}
					continue
				}

				tp.Reset(re.info.Title, re.info.Filesize, re.entry.WebpageURL)
				if re.info.FilesizeApprox > 0 && re.info.Filesize == 0 {
					tp.Total.Store(re.info.FilesizeApprox)
				}

				err := downloadResolved(re.info, re.entry.WebpageURL, format, outputDir, threads, tp, mediaType, quality)
				if err != nil {
					tp.Failed.Store(true)
				} else {
					tp.Done.Store(true)
				}
				results <- result{entry: re.entry, err: err}
			}
		}()
	}

	go func() {
		dlWg.Wait()
		close(results)
	}()

	stopRender := make(chan struct{})
	if json {
		go func() {
			type slotState struct {
				current    int64
				total      int64
				done       bool
				failed     bool
				converting bool
				convertPct int64
				title      string
				url        string
			}
			prev := make([]slotState, workers)
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopRender:
					return
				case <-ticker.C:
					func() {
						mu.Lock()
						defer mu.Unlock()
						for i, tp := range slots {
							title := tp.Title
							url := tp.URL
							if title == "" {
								continue
							}
							p := &prev[i]

							if title != p.title || url != p.url {
								*p = slotState{title: title, url: url, current: -1, total: -1, convertPct: -1}
							}

							cur := tp.Current.Load()
							total := tp.Total.Load()
							done := tp.Done.Load()
							failed := tp.Failed.Load()
							converting := tp.Converting.Load()
							convertPct := tp.ConvertPct.Load()

							if done && !p.done {
								writeJSONEvent("complete", map[string]any{"title": title, "url": url})
							}
							if failed && !p.failed {
								writeJSONEvent("error", map[string]any{"title": title, "url": url, "error": "download failed"})
							}
							if (cur != p.current || total != p.total) && total > 0 {
								writeJSONEvent("progress", map[string]any{
									"title":   title,
									"url":     url,
									"current": cur,
									"total":   total,
									"percent": float64(cur) / float64(total) * 100,
								})
							}
							if converting && convertPct != p.convertPct {
								writeJSONEvent("convert_progress", map[string]any{
									"title":   title,
									"url":     url,
									"percent": convertPct,
								})
							}

							p.current = cur
							p.total = total
							p.done = done
							p.failed = failed
							p.converting = converting
							p.convertPct = convertPct
						}
					}()
				}
			}
		}()
	} else {
		go func() {
			stopped := spinner == nil
			ticker := time.NewTicker(80 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopRender:
					if !stopped {
						spinner.Stop()
					}
					return
				case <-ticker.C:
					if !stopped {
						var started bool
						for _, tp := range slots {
							if tp.Current.Load() > 0 || tp.Done.Load() {
								started = true
								break
							}
						}
						if started {
							spinner.Stop()
							if !json && playlistTitle != "" {
								fmt.Printf("  %s\n", playlistTitle)
								fmt.Print("\033[?25l")
							}
							stopped = true
						}
					}
					if stopped {
						mu.Lock()
						render()
						mu.Unlock()
					}
				}
			}
		}()
	}

	var failedEntries []PlaylistEntry
	for r := range results {
		mu.Lock()
		if r.err != nil {
			failed.Add(1)
			failedEntries = append(failedEntries, r.entry)
		} else {
			done.Add(1)
		}
		mu.Unlock()
	}

	close(stopRender)
	return failedEntries
}

func downloadResolved(info *VideoInfo, originalURL, format, outputDir string, threads int, tp *TrackProgress, mediaType string, quality int) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("cannot create output dir: %w", err)
	}

	safeTitle := sanitizeFilename(info.Title)
	outExt := format
	if mediaType == "video" {
		outExt = "mp4"
	}
	outFile := filepath.Join(outputDir, safeTitle+"."+outExt)

	isHLS := strings.Contains(info.URL, ".m3u8")
	if info.URL != "" && !isHLS {
		size := info.Filesize
		if size == 0 {
			size = info.FilesizeApprox
		}
		if tp != nil {
			tp.Total.Store(size)
		}

		tmpFile := filepath.Join(outputDir, safeTitle+".tmp")
		_, supportsRange, _ := probeURL(info.URL)

		if supportsRange && size > 0 && threads > 1 {
			if err := parallelDownload(info.URL, tmpFile, size, threads, true, tp); err != nil {
				os.Remove(tmpFile)
				return fmt.Errorf("download failed: %w", err)
			}
		} else {
			if err := streamDownload(info.URL, tmpFile, size, true, tp); err != nil {
				os.Remove(tmpFile)
				return fmt.Errorf("download failed: %w", err)
			}
		}

		if mediaType == "video" {
			if err := os.Rename(tmpFile, outFile); err != nil {
				return fmt.Errorf("rename failed: %w", err)
			}
		} else if format != info.Ext {
			if tp != nil {
				tp.Converting.Store(true)
				tp.ConvertPct.Store(3)
			}
			if err := convertAudioProgress(tmpFile, outFile, format, info.Duration, tp); err != nil {
				os.Remove(tmpFile)
				if tp != nil {
					tp.Converting.Store(false)
				}
				return fmt.Errorf("conversion failed: %w", err)
			}
			os.Remove(tmpFile)
		} else {
			if err := os.Rename(tmpFile, outFile); err != nil {
				return fmt.Errorf("rename failed: %w", err)
			}
		}

		if format == "mp3" && mediaType != "video" {
			_ = metadata.WriteMP3Tags(outFile, info.Title, info.DisplayArtist(), info.Album, info.Thumbnail)
		}

	} else {
		if err := ytDlpFallback(originalURL, format, outFile, threads, true, mediaType, quality, tp); err != nil {
			return err
		}
	}

	return nil
}

func DownloadPlaylist(playlistURL, format, outputDir string, threads, workers int, mediaType string, quality int, json bool) error {
	var spinner *progress.Spinner
	if json {
		writeJSONEvent("resolving", map[string]any{"url": playlistURL})
	} else {
		spinner = progress.NewSpinner("Resolving playlist")
		spinner.Start()
	}

	entries, playlistTitle, err := ResolvePlaylist(playlistURL)

	if err != nil {
		if spinner != nil {
			spinner.Stop()
		}
		if json {
			writeJSONEvent("error", map[string]any{"url": playlistURL, "error": err.Error()})
		}
		return fmt.Errorf("could not resolve playlist: %w", err)
	}
	if len(entries) == 0 {
		if spinner != nil {
			spinner.Stop()
		}
		if json {
			writeJSONEvent("error", map[string]any{"url": playlistURL, "error": "no tracks found"})
		}
		return fmt.Errorf("no tracks found in playlist")
	}

	total := int64(len(entries))

	playlistDir := sanitizeFilename(playlistTitle)
	if mediaType == "video" {
		outputDir = filepath.Join(outputDir, "video", playlistDir)
	} else {
		outputDir = filepath.Join(outputDir, playlistDir)
	}

	if json {
		writeJSONEvent("playlist_resolved", map[string]any{
			"title": playlistTitle,
			"count": total,
			"format": func() string {
				if mediaType == "video" {
					return "mp4"
				}
				return format
			}(),
			"workers": workers,
			"threads": threads,
		})
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		if spinner != nil {
			spinner.Stop()
		}
		return fmt.Errorf("cannot create output dir: %w", err)
	}

	var (
		done      atomic.Int64
		failed    atomic.Int64
		startTime = time.Now()
	)

	failedEntries := runBatch(entries, format, outputDir, threads, workers, mediaType, quality,
		&done, &failed, total, json, false, spinner, playlistTitle)

	for len(failedEntries) > 0 {
		failed.Store(0)
		prev := len(failedEntries)

		failedEntries = runBatch(failedEntries, format, outputDir, threads, workers, mediaType, quality,
			&done, &failed, total, json, true, nil, "")

		if len(failedEntries) >= prev {
			break
		}
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)
	if json {
		writeJSONEvent("complete", map[string]any{
			"title":      playlistTitle,
			"downloaded": done.Load(),
			"failed":     failed.Load(),
			"total":      total,
			"elapsed":    elapsed.String(),
		})
	} else {
		fmt.Print("\033[?25h\033[2K\r\033[1A\033[2K\r  Done\n")
		if len(failedEntries) > 0 {
			for _, t := range failedEntries {
				fmt.Printf("  \033[31m✗  %s\033[0m\n", t.Title)
			}
		}
	}

	return nil
}
