package main

import "testing"

func TestVerkortMonitor(t *testing.T) {
	gevallen := []struct {
		in   string
		want string
	}{
		{`\\?\DISPLAY#AOC2702#5&24a0a36a&0&UID4352#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`, "AOC2702 UID4352"},
		{`\\?\DISPLAY#DELD128#5&24a0a36a&0&UID4353#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}`, "DELD128 UID4353"},
		{`\\?\DISPLAY#AOC2702#5&24a0a36a&0&UID4357#{guid}`, "AOC2702 UID4357"},
		// geen COM-pad: ongewijzigd terug
		{"HDMI-1", "HDMI-1"},
		{"", ""},
		// wel de vorm maar zonder UID
		{`\\?\DISPLAY#AOC2702#5&24a0a36a&0#{guid}`, "AOC2702"},
	}
	for _, g := range gevallen {
		if uit := verkortMonitor(g.in); uit != g.want {
			t.Errorf("verkortMonitor(%q) = %q, wil %q", g.in, uit, g.want)
		}
	}
}

func TestIdxWithin(t *testing.T) {
	mons := []string{"AOC2702", "DELD128", "AOC2702", "AOC2702"}
	want := []int{0, 0, 1, 2}
	for i, w := range want {
		if n := idxWithin(mons, i); n != w {
			t.Errorf("idxWithin(%v, %d) = %d, wil %d", mons, i, n, w)
		}
	}
}

func TestIcoonBevatKleineMaten(t *testing.T) {
	entries := icoEntries(icoData)
	if len(entries) < 2 {
		t.Fatalf("het .ico moet meerdere maten bevatten, gevonden: %d", len(entries))
	}
	for _, maat := range []int{16, 24, 32, 48} {
		e, ok := kiesIcoon(entries, maat)
		if !ok {
			t.Fatalf("geen icoon gevonden voor maat %d", maat)
		}
		if e.breedte != maat {
			t.Errorf("maat %d: kreeg %dx%d, wil een exacte treffer", maat, e.breedte, e.breedte)
		}
		if e.off+e.size > len(icoData) {
			t.Errorf("maat %d: beeld wijst buiten het bestand (%d+%d > %d)", maat, e.off, e.size, len(icoData))
		}
	}
	// 16 is de maat die Windows voor de systeembalk gebruikt en moet er dus zijn
	if _, ok := kiesIcoon(entries, 16); !ok {
		t.Error("16x16 ontbreekt — dan is het tray-icoon onzichtbaar")
	}
	// een belachelijke maat valt terug op het grootste beeld
	e, ok := kiesIcoon(entries, 1024)
	if !ok || e.breedte != 256 {
		t.Errorf("maat 1024: kreeg %d (ok=%v), wil terugval op 256", e.breedte, ok)
	}
}
