package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type VideoInfo struct {
	Title          string  `json:"title"`
	Artist         string  `json:"artist"`
	Uploader       string  `json:"uploader"`
	Album          string  `json:"album"`
	Duration       float64 `json:"duration"`
	Height         int     `json:"height"`
	URL            string  `json:"url"`
	Thumbnail      string  `json:"thumbnail"`
	Ext            string  `json:"ext"`
	AudioCodec     string  `json:"acodec"`
	Filesize       int64   `json:"filesize"`
	FilesizeApprox int64   `json:"filesize_approx"`
}

func (v *VideoInfo) DisplayArtist() string {
	if v.Artist != "" {
		return v.Artist
	}
	return v.Uploader
}

func resolve(url string, mediaType string, quality int) (*VideoInfo, error) {
	url = sanitizeURL(url)
	if url == "" {
		return nil, fmt.Errorf("invalid URL")
	}

	if strings.Contains(url, "list=") || strings.Contains(url, "/playlist") {
		return nil, fmt.Errorf("playlist URLs not supported - use individual video URLs")
	}

	url = cleanURL(url)
	ytdlp := findBin("yt-dlp")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	formatFlag := "bestaudio"
	if mediaType == "video" {
		if quality > 0 {
			formatFlag = fmt.Sprintf("bestvideo[height<=?%d]+bestaudio/best[height<=?%d]", quality, quality)
		} else {
			formatFlag = "bestvideo+bestaudio/best"
		}
	}

	cmd := exec.CommandContext(ctx, ytdlp,
		"--dump-json",
		"--format", formatFlag,
		"--no-playlist",
		"--socket-timeout", "25",
		url,
	)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("yt-dlp resolve canceled: %w", ctx.Err())
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("yt-dlp resolve timeout after 30 seconds")
		}
		if _, e := exec.LookPath(ytdlp); e != nil {
			return nil, fmt.Errorf("yt-dlp not found — run the installer")
		}
		stderr := stderrBuf.String()
		if stderr != "" {
			return nil, fmt.Errorf("yt-dlp resolve failed: %w\nstderr: %s", err, stderr)
		}
		return nil, fmt.Errorf("yt-dlp resolve failed: %w", err)
	}

	var info VideoInfo
	if err := json.Unmarshal(stdoutBuf.Bytes(), &info); err != nil {
		return nil, fmt.Errorf("failed to parse track info: %w", err)
	}
	return &info, nil
}

func cleanURL(raw string) string {
	for _, param := range []string{"?si=", "&si="} {
		if idx := strings.Index(raw, param); idx != -1 {
			raw = raw[:idx]
		}
	}
	return raw
}

func sanitizeURL(inputURL string) string {
	inputURL = strings.TrimSpace(inputURL)
	if !strings.HasPrefix(inputURL, "http://") && !strings.HasPrefix(inputURL, "https://") {
		return ""
	}

	parsed, err := url.Parse(inputURL)
	if err != nil {
		return ""
	}

	if parsed.Host == "" {
		return ""
	}

	parsed.Fragment = ""
	return parsed.String()
}
