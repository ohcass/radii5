package downloader

import (
	"io"
	"strings"
)

func newLineScanner(r io.Reader) *lineScanner {
	return &lineScanner{r: r, buf: make([]byte, 0, 4096)}
}

type lineScanner struct {
	r    io.Reader
	buf  []byte
	line string
	done bool
}

func (s *lineScanner) Scan() bool {
	if s.done {
		return false
	}
	tmp := make([]byte, 512)
	for {
		if idx := indexByte(s.buf, '\n'); idx >= 0 {
			s.line = strings.TrimRight(string(s.buf[:idx]), "\r")
			s.buf = s.buf[idx+1:]
			return true
		}
		n, err := s.r.Read(tmp)
		if n > 0 {
			s.buf = append(s.buf, tmp[:n]...)
		}
		if err != nil {
			if len(s.buf) > 0 {
				s.line = strings.TrimRight(string(s.buf), "\r\n")
				s.buf = nil
				s.done = true
				return s.line != ""
			}
			return false
		}
	}
}

func (s *lineScanner) Text() string { return s.line }

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}
