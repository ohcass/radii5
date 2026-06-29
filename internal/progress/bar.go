package progress

import (
	"fmt"
	"sync"
	"time"
)

type Bar struct {
	mu          sync.Mutex
	total       int64
	current     int64
	finished    bool
	frameNum    int
	started     bool
	stop        chan struct{}
	displayPct  int
	displayTime time.Time
	palette     Palette
}

func NewBar(total int64) *Bar {
	return NewBarWithPalette(total, BluePalette())
}

func NewBarWithPalette(total int64, palette Palette) *Bar {
	return &Bar{
		total:       total,
		palette:     palette,
		stop:        make(chan struct{}),
		displayTime: time.Now(),
	}
}

func (b *Bar) SetTotal(total int64) {
	b.mu.Lock()
	if !b.finished {
		b.total = total
	}
	b.mu.Unlock()
}

func (b *Bar) Total() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total
}

func (b *Bar) Write(p []byte) (int, error) {
	n := len(p)
	b.mu.Lock()
	if !b.finished {
		b.current += int64(n)
		b.startOnce()
		// Do NOT render here: every HTTP range chunk callback would
		// otherwise trigger a renderLock that races with the animator,
		// causing the rapid bar flicker the user reported. The animate
		// goroutine is the single source of visual updates.
	}
	b.mu.Unlock()
	return n, nil
}

func (b *Bar) Set(current int64) {
	b.mu.Lock()
	if !b.finished {
		b.current = current
		b.startOnce()
		// See Write — render only on tick.
	}
	b.mu.Unlock()
}

func (b *Bar) renderLocked() {
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
	// One Printf so Windows Console Host cannot split the erase and the
	// bar content across two adjacent writes to the same os.Stdout handle.
	fmt.Printf("\033[2K\r  %s%s", RenderAnimation(b.frameNum, b.palette), extra)
}

func (b *Bar) startOnce() {
	if !b.started {
		b.started = true
		HideCursor()
		go b.animate()
	}
}

func (b *Bar) animate() {
	t := time.NewTicker(TickMs * time.Millisecond)
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
			b.frameNum = (b.frameNum + 1) % CycleLen
			b.renderLocked()
			b.mu.Unlock()
		}
	}
}

func (b *Bar) Finish() {
	b.mu.Lock()
	if b.finished {
		b.mu.Unlock()
		return
	}
	b.finished = true
	close(b.stop)
	// One Print, still under mu: serializes against any final animate tick.
	fmt.Print("\033[2K\r\033[?25h")
	b.mu.Unlock()
}
