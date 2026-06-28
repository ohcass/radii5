package progress

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var segColors = []string{
	"\033[38;2;104;163;235m",
	"\033[38;2;101;157;248m",
	"\033[38;2;70;105;165m",
	"\033[38;2;48;73;110m",
	"\033[38;2;36;51;74m",
	"\033[38;2;26;37;52m",
}

const (
	trackLen  = 8
	boxCount  = 6
	offExtra  = 3
	tickMs    = 45
	waitTicks = 20
	waitEvery = 1
)

type Bar struct {
	mu          sync.Mutex
	total       int64
	current     int64
	finished    bool
	pos         int
	dir         int
	wait        int
	cycle       int
	started     bool
	stop        chan struct{}
	displayPct  int
	displayTime time.Time
}

func NewBar(total int64) *Bar {
	return &Bar{
		total:       total,
		pos:         trackLen + offExtra,
		dir:         -1,
		stop:        make(chan struct{}),
		displayTime: time.Now(),
	}
}

func (b *Bar) Write(p []byte) (int, error) {
	n := len(p)
	b.mu.Lock()
	if !b.finished {
		b.current += int64(n)
		b.startOnce()
	}
	b.mu.Unlock()
	return n, nil
}

func (b *Bar) Set(current int64) {
	b.mu.Lock()
	if !b.finished {
		b.current = current
		b.startOnce()
	}
	b.mu.Unlock()
}

func (b *Bar) startOnce() {
	if !b.started {
		b.started = true
		fmt.Print("\033[?25l")
		go b.animate()
	}
}

func (b *Bar) animate() {
	t := time.NewTicker(tickMs * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.mu.Lock()
			if b.finished {
				b.mu.Unlock()
				return
			}

			if b.wait > 0 {
				b.wait--
				b.renderLocked()
				b.mu.Unlock()
				continue
			}

			b.pos += b.dir

			lo := -boxCount - offExtra
			hi := trackLen + offExtra
			if b.pos > hi {
				b.dir = -1
				b.cycle++
				if b.cycle%waitEvery == 0 {
					b.wait = waitTicks
				}
			} else if b.pos < lo {
				b.dir = 1
			}

			b.renderLocked()
			b.mu.Unlock()
		}
	}
}

func (b *Bar) renderLocked() {
	var s strings.Builder
	s.Grow(trackLen * 12)
	for i := 0; i < trackLen; i++ {
		bi := i - b.pos
		if bi >= 0 && bi < boxCount {
			idx := bi
			if b.dir > 0 {
				idx = boxCount - 1 - bi
			}
			s.WriteString(segColors[idx])
			s.WriteString("■")
		} else {
			s.WriteString("\033[38;5;239m·")
		}
	}
	s.WriteString("\033[0m")
	extra := ""
	if b.total > 0 {
		pct := float64(b.current) / float64(b.total) * 100
		if pct > 100 {
			pct = 100
		}
		ipct := int(pct)
		if ipct > b.displayPct {
			now := time.Now()
			elapsed := now.Sub(b.displayTime).Seconds()
			rate := float64(ipct-b.displayPct) / elapsed
			step := 1
			switch {
			case rate > 50:
				step = 10
			case rate > 10:
				step = 5
			}
			disp := (ipct / step) * step
			if disp > b.displayPct || ipct >= 100 {
				b.displayPct = disp
				b.displayTime = now
			}
		}
		extra = fmt.Sprintf(" \033[1m%d%%\033[0m", b.displayPct)
	}
	fmt.Printf("\033[2K\r  %s%s", s.String(), extra)
}

func (b *Bar) Finish() {
	b.mu.Lock()
	if b.finished {
		b.mu.Unlock()
		return
	}
	b.finished = true
	close(b.stop)
	b.mu.Unlock()
	fmt.Print("\033[2K\r\033[?25h")
}
