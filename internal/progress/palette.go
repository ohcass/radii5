package progress

import (
	"fmt"
	"math"
	"strings"
)

// Animation parameters shared by single Bar and the playlist aggregate view.
const (
	trackLen   = 8
	trailSteps = 6
	holdStart  = 30
	holdEnd    = 9

	inactiveFac = 0.6
	minAlpha    = 0.3

	CycleLen = trackLen + holdEnd + (trackLen-1) + holdStart // 54

	// TickMs is the animation tick rate used by both single-track Bar and
	// the playlist aggregate view.
	TickMs = 45
)

// RGB is an 8-bit color triple.
type RGB struct{ R, G, B uint8 }

// Palette pairs a base color with its precomputed trail gradient.
// The trail is indexed [0..trailSteps-1]; index 0 is the brightest.
type Palette struct {
	Base  RGB
	Trail [trailSteps]RGB
}

func derivePalette(r, g, b uint8) Palette {
	baseR, baseG, baseB := float64(r), float64(g), float64(b)
	pal := Palette{Base: RGB{R: r, G: g, B: b}}
	for i := 0; i < trailSteps; i++ {
		var alpha, factor float64
		switch i {
		case 0:
			alpha, factor = 1.0, 1.0
		case 1:
			alpha, factor = 0.9, 1.15
		default:
			alpha = math.Pow(0.65, float64(i-1))
			factor = 1.0
		}
		rr := baseR * factor
		gg := baseG * factor
		bb := baseB * factor
		if rr > 255 {
			rr = 255
		}
		if gg > 255 {
			gg = 255
		}
		if bb > 255 {
			bb = 255
		}
		pal.Trail[i] = RGB{
			R: uint8(rr * alpha),
			G: uint8(gg * alpha),
			B: uint8(bb * alpha),
		}
	}
	return pal
}

// BluePalette is the standard bar palette.
func BluePalette() Palette { return derivePalette(104, 163, 235) }

// OrangePalette is used for the playlist retry pass.
func OrangePalette() Palette { return derivePalette(235, 150, 70) }

type frameState struct {
	activePos int
	isHolding bool
	holdProg  int
	holdTotal int
	moveProg  int
	moveTotal int
	forward   bool
}

func getFrameState(f int) frameState {
	switch {
	case f < trackLen:
		return frameState{
			activePos: f,
			moveProg:  f, moveTotal: trackLen,
			forward: true,
		}
	case f < trackLen+holdEnd:
		return frameState{
			activePos: trackLen - 1,
			isHolding: true,
			holdProg:  f - trackLen, holdTotal: holdEnd,
			forward: true,
		}
	case f < trackLen+holdEnd+(trackLen-1):
		backIdx := f - trackLen - holdEnd
		return frameState{
			activePos: trackLen - 2 - backIdx,
			moveProg:  backIdx, moveTotal: trackLen - 1,
			forward: false,
		}
	default:
		return frameState{
			activePos: 0,
			isHolding: true,
			holdProg:  f - trackLen - holdEnd - (trackLen - 1),
			holdTotal: holdStart,
			forward: false,
		}
	}
}

func (s frameState) colorIdx(ch int) int {
	var dirDist int
	if s.forward {
		dirDist = s.activePos - ch
	} else {
		dirDist = ch - s.activePos
	}
	if s.isHolding {
		return dirDist + s.holdProg
	}
	if dirDist < 0 {
		return -1
	}
	if dirDist < trailSteps {
		return dirDist
	}
	return -1
}

func (s frameState) dotAlpha() float64 {
	if s.isHolding && s.holdTotal > 0 {
		prog := float64(s.holdProg) / float64(s.holdTotal)
		if prog > 1 {
			prog = 1
		}
		fade := 1 - prog*(1-minAlpha)
		if fade < minAlpha {
			fade = minAlpha
		}
		return inactiveFac * fade
	}
	if !s.isHolding && s.moveTotal > 0 {
		den := s.moveTotal - 1
		if den < 1 {
			den = 1
		}
		prog := float64(s.moveProg) / float64(den)
		if prog > 1 {
			prog = 1
		}
		fade := minAlpha + prog*(1-minAlpha)
		return inactiveFac * fade
	}
	return inactiveFac
}

// RenderAnimation returns the 8-character ANSI-encoded string for `frame`,
// using the given palette. Output ends with the color-reset sequence;
// append the percent label and any extra content after that.
func RenderAnimation(frame int, pal Palette) string {
	state := getFrameState(frame)
	var sb strings.Builder
	sb.Grow(trackLen * 24)
	for ch := 0; ch < trackLen; ch++ {
		idx := state.colorIdx(ch)
		if idx >= 0 && idx < trailSteps {
			c := pal.Trail[idx]
			fmt.Fprintf(&sb, "\033[38;2;%d;%d;%dm■", c.R, c.G, c.B)
		} else {
			a := state.dotAlpha()
			r := uint8(float64(pal.Base.R) * a)
			g := uint8(float64(pal.Base.G) * a)
			b := uint8(float64(pal.Base.B) * a)
			fmt.Fprintf(&sb, "\033[38;2;%d;%d;%dm⬝", r, g, b)
		}
	}
	sb.WriteString("\033[0m")
	return sb.String()
}
