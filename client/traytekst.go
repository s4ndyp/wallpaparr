package main

// Constanten en tekstverwerking voor het systeembalk-icoon. Bewust in een
// draagbaar bestand: deze waarden zijn foutgevoelig (een verkeerde NIM_*-waarde
// WIST het icoon in plaats van het bij te werken) en CI test ze daarom op Linux.

import (
	"strings"
	"unicode/utf16"
)

const (
	// Shell_NotifyIconW-berichten. Let op: NIM_MODIFY is 1 en NIM_DELETE is 2 —
	// ze omdraaien betekent dat elke statusupdate het icoon verwijdert.
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	// NOTIFYICONDATA-vlaggen.
	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04
	nifShowTip = 0x80 // NIF_SHOWTIP: gebruik de standaardtooltip (hoort bij v4)
)

// putStr zet een string in een Win32-tekenbuffer (UTF-16) en sluit die ALTIJD af
// met een nul. Zonder die nul leest de shell voorbij het veld — bijvoorbeeld bij
// de tray-tooltip — en dan verdwijnt het icoon zodra je eroverheen zweeft.
func putStr(dst []uint16, s string) {
	if len(dst) == 0 {
		return
	}
	src := utf16.Encode([]rune(s))
	n := len(src) + 1 // inclusief de afsluitende nul
	if n > len(dst) {
		n = len(dst)
	}
	copy(dst[:n-1], src[:n-1])
	dst[n-1] = 0
}

// kortStatus houdt de tray-tooltip kort: één regel, maximaal 100 tekens.
// SzTip bewaart 128 tekens; langere tekst wordt afgekapt en dat mag nooit.
func kortStatus(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	r := []rune(s)
	if len(r) > 100 {
		s = strings.TrimSpace(string(r[:100])) + "…"
	}
	if s == "" {
		s = "klaar"
	}
	return s
}
