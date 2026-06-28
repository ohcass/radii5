package progress

import (
	"fmt"
	"sync/atomic"
	"time"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Spinner struct {
	msg     string
	stop    chan struct{}
	done    chan struct{}
	started atomic.Bool
}

func NewSpinner(msg string) *Spinner {
	return &Spinner{msg: msg, stop: make(chan struct{}), done: make(chan struct{})}
}

func (s *Spinner) Start() {
	if !s.started.CompareAndSwap(false, true) {
		return
	}
	fmt.Print("\033[?25l")
	go func() {
		i := 0
		for {
			select {
			case <-s.stop:
				fmt.Print("\033[2K\r\033[?25h")
				close(s.done)
				return
			case <-time.After(80 * time.Millisecond):
				fmt.Printf("\033[2K\r\033[38;5;39m%s\033[0m  %s", frames[i%len(frames)], s.msg)
				i++
			}
		}
	}()
}

func (s *Spinner) Stop() {
	if !s.started.Load() {
		return
	}
	close(s.stop)
	<-s.done
}
