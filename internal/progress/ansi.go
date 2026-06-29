package progress

import "fmt"

const (
	cursorHide = "\033[?25l"
	cursorShow = "\033[?25h"
)

// HideCursor / ShowCursor emit pure terminal-state signals. Multi-step
// sequences (erase + content, erase + show cursor, etc.) are intentionally
// inlined at the call site as a single atomic Print.
func HideCursor() { fmt.Print(cursorHide) }
func ShowCursor() { fmt.Print(cursorShow) }
