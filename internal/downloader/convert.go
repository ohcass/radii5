package downloader

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

func convertAudio(input, output, format string) error {
	ffmpeg := findBin("ffmpeg")
	var args []string
	switch format {
	case "mp3":
		args = []string{"-i", input, "-codec:a", "libmp3lame", "-qscale:a", "2", "-y", output}
	case "flac":
		args = []string{"-i", input, "-codec:a", "flac", "-compression_level", "5", "-y", output}
	case "m4a":
		args = []string{"-i", input, "-codec:a", "aac", "-b:a", "256k", "-y", output}
	default:
		args = []string{"-i", input, "-y", output}
	}
	cmd := exec.Command(ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w\nstderr:\n%s", err, stderr.String())
	}
	return nil
}

func convertAudioProgress(input, output, format string, durationSecs float64, tp *TrackProgress) error {
	ffmpeg := findBin("ffmpeg")
	var args []string
	switch format {
	case "mp3":
		args = []string{"-i", input, "-codec:a", "libmp3lame", "-qscale:a", "2", "-progress", "pipe:1", "-y", output}
	case "flac":
		args = []string{"-i", input, "-codec:a", "flac", "-compression_level", "5", "-progress", "pipe:1", "-y", output}
	case "m4a":
		args = []string{"-i", input, "-codec:a", "aac", "-b:a", "256k", "-progress", "pipe:1", "-y", output}
	default:
		args = []string{"-i", input, "-progress", "pipe:1", "-y", output}
	}

	cmd := exec.Command(ffmpeg, args...)
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return convertAudio(input, output, format)
	}
	if err := cmd.Start(); err != nil {
		return convertAudio(input, output, format)
	}

	durationMs := int64(durationSecs * 1000000)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "out_time_ms=") {
			val := strings.TrimPrefix(line, "out_time_ms=")
			if ms, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil && durationMs > 0 {
				pct := int64(float64(ms) / float64(durationMs) * 100)
				if pct > 100 {
					pct = 100
				}
				if pct < 0 {
					pct = 0
				}
				tp.ConvertPct.Store(pct)
			}
		}
	}

	return cmd.Wait()
}
