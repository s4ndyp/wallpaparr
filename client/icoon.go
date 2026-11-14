package main

// Het programmapictogram (multi-size .ico) en het uitzoeken van het juiste beeld.
// Dit staat bewust in een draagbaar bestand: zo test CI de logica ook op Linux.

import (
	_ "embed"
	"encoding/binary"
)

//go:embed assets/wallpaparr.ico
var icoData []byte

type icoEntry struct {
	breedte int
	off     int
	size    int
}

// icoEntries leest de inhoudsopgave van een .ico-bestand.
func icoEntries(data []byte) []icoEntry {
	if len(data) < 22 {
		return nil
	}
	aantal := int(binary.LittleEndian.Uint16(data[4:6]))
	var uit []icoEntry
	for i := 0; i < aantal; i++ {
		start := 6 + i*16
		if start+16 > len(data) {
			break
		}
		breedte := int(data[start])
		if breedte == 0 {
			breedte = 256
		}
		size := int(binary.LittleEndian.Uint32(data[start+8 : start+12]))
		off := int(binary.LittleEndian.Uint32(data[start+12 : start+16]))
		if size > 0 && off > 0 && off+size <= len(data) {
			uit = append(uit, icoEntry{breedte: breedte, off: off, size: size})
		}
	}
	return uit
}

// kiesIcoon zoekt het beeld dat het beste bij de gevraagde maat past: exact,
// anders de kleinste grotere, anders de grootste die er is.
func kiesIcoon(entries []icoEntry, maat int) (icoEntry, bool) {
	for _, e := range entries {
		if e.breedte == maat {
			return e, true
		}
	}
	var beste icoEntry
	gevonden := false
	for _, e := range entries {
		if e.breedte > maat && (!gevonden || e.breedte < beste.breedte) {
			beste, gevonden = e, true
		}
	}
	if gevonden {
		return beste, true
	}
	for _, e := range entries {
		if !gevonden || e.breedte > beste.breedte {
			beste, gevonden = e, true
		}
	}
	return beste, gevonden
}
