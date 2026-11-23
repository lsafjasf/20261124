package pgs

import "fmt"

// ClockHz is the PGS presentation clock rate.
const ClockHz = 90000

// FormatPTS renders a 90 kHz PTS as HH:MM:SS.mmm with exact
// millisecond rounding (PTS ticks are 1/90000 s, so the value is
// exact to the tick).
func FormatPTS(pts uint32) string {
	ms := uint64(pts) * 1000 / ClockHz
	h := ms / 3600000
	m := ms / 60000 % 60
	s := ms / 1000 % 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms%1000)
}
