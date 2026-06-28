package downloader

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var findBin = func(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		name += ".exe"
	}
	if dir := selfDir(); dir != "" {
		if candidate := filepath.Join(dir, name); fileExists(candidate) {
			return candidate
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if candidate := filepath.Join(home, ".radii5", "bin", name); fileExists(candidate) {
			return candidate
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
}

func selfDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "",
		"?", "", "\"", "", "<", "", ">", "", "|", "",
	)
	return strings.TrimSpace(replacer.Replace(name))
}
