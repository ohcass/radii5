package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/ohcass/radii5/internal/progress"
)

func ytDlpFallback(url, format, outFile string, threads int, silent bool, mediaType string, quality int, size int64, tp *TrackProgress) error {
	adaptiveThreads := DetermineThreads(0, threads)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	ytdlp := findBin("yt-dlp")
	var args []string
	if mediaType == "audio" {
		args = []string{
			"--no-playlist",
			"-x",
			"--audio-format", format,
			"--audio-quality", "2",
			"--concurrent-fragments", fmt.Sprintf("%d", adaptiveThreads),
			"--no-colors",
			"--progress", "--newline",
			"-o", outFile,
			url,
		}
	} else {
		formatStr := "bestvideo+bestaudio/best"
		if quality > 0 {
			formatStr = fmt.Sprintf("bestvideo[height<=?%d]+bestaudio/best[height<=?%d]", quality, quality)
		}
		args = []string{
			"--no-playlist",
			"--format", formatStr,
			"--merge-output-format", "mp4",
			"--concurrent-fragments", fmt.Sprintf("%d", adaptiveThreads),
			"--no-colors",
			"--progress", "--newline",
			"-o", outFile,
			url,
		}
	}

	cmd := exec.CommandContext(ctx, ytdlp, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if _, e := exec.LookPath(ytdlp); e != nil {
			return fmt.Errorf("yt-dlp not found — run the installer: %w", e)
		}
		return fmt.Errorf("yt-dlp failed to start: %w", err)
	}

	var bar *progress.Bar
	// Pre-create the bar when size is known so the render frame is on
	// screen immediately — no blank while yt-dlp boots.
	if !silent && size > 0 {
		bar = progress.NewBar(size)
		bar.Set(0)
	}

	scan := func(r io.Reader, isErr bool) {
		scanner := newLineScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			_, dlTotal, dlCurrent, ok := parseYtDlpProgress(line)
			if ok {
				if tp != nil {
					if dlCurrent <= 0 && dlTotal > 0 {
						dlCurrent = 1
					}
					tp.Total.Store(dlTotal)
					tp.Current.Store(dlCurrent)
				}
				if bar != nil {
					bar.Set(dlCurrent)
				}
			} else if isErr && strings.Contains(line, "ERROR") && !silent {
				fmt.Fprintf(os.Stderr, "  %s\n", color.RedString(line))
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scan(stdout, false) }()
	go func() { defer wg.Done(); scan(stderr, true) }()

	err = cmd.Wait()
	wg.Wait()

	if bar != nil {
		bar.Finish()
	}

	if err != nil {
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("yt-dlp canceled: %w", ctx.Err())
		}
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("yt-dlp timeout after 30 minutes: %w", err)
		}
		return fmt.Errorf("yt-dlp failed: %w", err)
	}

	return nil
}

func parseYtDlpProgress(line string) (pct float64, total, current int64, ok bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[download]") {
		return
	}
	line = strings.TrimPrefix(line, "[download]")
	line = strings.TrimSpace(line)

	ofIdx := strings.Index(line, "% of")
	if ofIdx < 0 {
		return
	}

	pctStr := strings.TrimSpace(line[:ofIdx])
	if _, err := fmt.Sscanf(pctStr, "%f", &pct); err != nil {
		return
	}

	rest := strings.TrimSpace(line[ofIdx+4:])
	rest = strings.TrimLeft(rest, "~ ")
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return
	}
	total = parseSizeStr(fields[0])
	if total <= 0 {
		return
	}
	current = int64(float64(total) * pct / 100)
	ok = true
	return
}

func parseSizeStr(s string) int64 {
	s = strings.ReplaceAll(s, ",", "")
	var val float64
	var unit string
	fmt.Sscanf(s, "%f%s", &val, &unit)
	unit = strings.ToLower(unit)
	switch {
	case strings.HasPrefix(unit, "gib") || strings.HasPrefix(unit, "gb"):
		return int64(val * 1073741824)
	case strings.HasPrefix(unit, "mib") || strings.HasPrefix(unit, "mb"):
		return int64(val * 1048576)
	case strings.HasPrefix(unit, "kib") || strings.HasPrefix(unit, "kb"):
		return int64(val * 1024)
	default:
		return int64(val)
	}
}
