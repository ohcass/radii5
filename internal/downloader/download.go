package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
	"github.com/ohcass/radii5/internal/metadata"
	"github.com/ohcass/radii5/internal/progress"
)

type TrackProgress struct {
	Title      string
	URL        string
	Current    atomic.Int64
	Total      atomic.Int64
	Done       atomic.Bool
	Failed     atomic.Bool
	Converting atomic.Bool
	ConvertPct atomic.Int64
}

func (t *TrackProgress) Reset(title string, total int64, url string) {
	t.Title = title
	t.URL = url
	t.Current.Store(0)
	t.Total.Store(total)
	t.Done.Store(false)
	t.Failed.Store(false)
	t.Converting.Store(false)
	t.ConvertPct.Store(0)
}

func Download(url, format, outputDir string, threads int, silent bool, tp *TrackProgress, mediaType string, quality int, json bool) error {
	if json {
		silent = true
	}

	var spinner *progress.Spinner
	if json {
		writeJSONEvent("resolving", map[string]any{"url": url})
	} else if !silent {
		spinner = progress.NewSpinner("Resolving track")
		spinner.Start()
	}

	info, err := resolve(url, mediaType, quality)

	if spinner != nil {
		spinner.Stop()
	}
	if err != nil {
		if json {
			writeJSONEvent("error", map[string]any{"url": url, "error": err.Error()})
		}
		if tp != nil {
			tp.Failed.Store(true)
		}
		return fmt.Errorf("could not resolve URL: %w", err)
	}

	if tp != nil {
		size := info.Filesize
		if size == 0 {
			size = info.FilesizeApprox
		}
		tp.Reset(info.Title, size, url)
	}

	if json {
		writeJSONEvent("track_info", map[string]any{
			"title":    info.Title,
			"artist":   info.DisplayArtist(),
			"duration": info.Duration,
			"filesize": func() int64 {
				if info.Filesize > 0 {
					return info.Filesize
				}
				return info.FilesizeApprox
			}(),
			"format": format,
			"url":    url,
		})
	} else if !silent {
		color.New(color.FgHiWhite, color.Bold).Printf("  %s\n", info.Title)
	}

	safeTitle := sanitizeFilename(info.Title)

	outExt := format
	if mediaType == "video" {
		outExt = "mp4"
		outputDir = filepath.Join(outputDir, "video")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("cannot create output dir: %w", err)
	}
	outFile := filepath.Join(outputDir, safeTitle+"."+outExt)

	var monitorStop chan struct{}
	if json {
		if tp == nil {
			tp = &TrackProgress{}
			tp.Reset(info.Title, info.Filesize, url)
		}
		monitorStop = make(chan struct{})
		defer func() {
			if monitorStop != nil {
				close(monitorStop)
			}
		}()
		go func() {
			var prev int64 = -1
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-monitorStop:
					return
				case <-ticker.C:
					title := tp.Title
					if title == "" {
						continue
					}
					cur := tp.Current.Load()
					total := tp.Total.Load()
					if cur != prev && total > 0 {
						writeJSONEvent("progress", map[string]any{
							"title":   title,
							"url":     tp.URL,
							"current": cur,
							"total":   total,
							"percent": float64(cur) / float64(total) * 100,
						})
						prev = cur
					}
					if tp.Converting.Load() {
						writeJSONEvent("convert_progress", map[string]any{
							"title":   title,
							"url":     tp.URL,
							"percent": tp.ConvertPct.Load(),
						})
					}
				}
			}
		}()
	}

	if mediaType == "video" {
		if err := ytDlpFallback(url, format, outFile, threads, silent, mediaType, quality, tp); err != nil {
			if json {
				writeJSONEvent("error", map[string]any{"url": url, "error": err.Error()})
			}
			if tp != nil {
				tp.Failed.Store(true)
			}
			return err
		}
		if json {
			writeJSONEvent("complete", map[string]any{"title": info.Title, "url": url, "path": outFile})
		}
		if tp != nil {
			tp.Done.Store(true)
		}
		return nil
	}

	tmpFile := filepath.Join(outputDir, safeTitle+".tmp")

	defer func() {
		if r := recover(); r != nil {
			os.Remove(tmpFile)
			panic(r)
		}
		if err != nil {
			os.Remove(tmpFile)
		}
	}()

	isHLS := strings.Contains(info.URL, ".m3u8")
	if info.URL != "" && !isHLS {
		size := info.Filesize
		if size == 0 {
			size = info.FilesizeApprox
		}

		start := time.Now()

		_, supportsRange, _ := probeURL(info.URL)
		tmpPath := outFile + ".tmp"

		if supportsRange && size > 0 && threads > 1 {
			if err := parallelDownload(info.URL, tmpPath, size, threads, silent, tp); err != nil {
				os.Remove(tmpPath)
				if json {
					writeJSONEvent("error", map[string]any{"url": url, "error": err.Error()})
				}
				if tp != nil {
					tp.Failed.Store(true)
				}
				return fmt.Errorf("download failed: %w", err)
			}
		} else {
			if err := streamDownload(info.URL, tmpPath, size, silent, tp); err != nil {
				os.Remove(tmpPath)
				if json {
					writeJSONEvent("error", map[string]any{"url": url, "error": err.Error()})
				}
				if tp != nil {
					tp.Failed.Store(true)
				}
				return fmt.Errorf("download failed: %w", err)
			}
		}

		if format != info.Ext {
			if tp != nil {
				tp.Converting.Store(true)
				tp.ConvertPct.Store(3)
			}
			convSpinner := progress.NewSpinner("Converting")
			if !silent {
				convSpinner.Start()
			}
			if err := convertAudio(tmpPath, outFile, format); err != nil {
				if !silent {
					convSpinner.Stop()
				}
				os.Remove(tmpPath)
				if json {
					writeJSONEvent("error", map[string]any{"url": url, "error": err.Error()})
				}
				if tp != nil {
					tp.Converting.Store(false)
					tp.Failed.Store(true)
				}
				return fmt.Errorf("conversion failed: %w", err)
			}
			if !silent {
				convSpinner.Stop()
			}
			if tp != nil {
				tp.Converting.Store(false)
			}
			os.Remove(tmpPath)
			elapsed := time.Since(start)
			if json {
				writeJSONEvent("complete", map[string]any{"title": info.Title, "url": url, "path": outFile, "elapsed": elapsed.Round(time.Millisecond).String()})
			} else if !silent {
				fmt.Print("\033[1A\033[2K\r  Done\n")
			}
		} else {
			if err := os.Rename(tmpPath, outFile); err != nil {
				os.Remove(tmpPath)
				if json {
					writeJSONEvent("error", map[string]any{"url": url, "error": err.Error()})
				}
				if tp != nil {
					tp.Failed.Store(true)
				}
				return fmt.Errorf("rename failed: %w", err)
			}
			elapsed := time.Since(start)
			if json {
				writeJSONEvent("complete", map[string]any{"title": info.Title, "url": url, "path": outFile, "elapsed": elapsed.Round(time.Millisecond).String()})
			} else if !silent {
				fmt.Print("\033[1A\033[2K\r  Done\n")
			}
		}

		if format == "mp3" && mediaType != "video" {
			_ = metadata.WriteMP3Tags(outFile, info.Title, info.DisplayArtist(), info.Album, info.Thumbnail)
		}

		if tp != nil {
			tp.Done.Store(true)
		}

	} else {
		if err := ytDlpFallback(url, format, outFile, threads, silent, mediaType, quality, tp); err != nil {
			if json {
				writeJSONEvent("error", map[string]any{"url": url, "error": err.Error()})
			}
			if tp != nil {
				tp.Failed.Store(true)
			}
			return err
		}
		if json {
			writeJSONEvent("complete", map[string]any{"title": info.Title, "url": url, "path": outFile})
		}
		if tp != nil {
			tp.Done.Store(true)
		}
	}

	return nil
}
