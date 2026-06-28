package downloader

func DetermineThreads(fileSize int64, userThreads int) int {
	if userThreads > 0 {
		return userThreads
	}
	switch {
	case fileSize < 5*1024*1024:
		return 2
	case fileSize < 20*1024*1024:
		return 4
	case fileSize < 100*1024*1024:
		return 8
	default:
		return 12
	}
}
