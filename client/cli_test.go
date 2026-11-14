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

func TestTrayConstanten(t *testing.T) {
	// Een verkeerde NIM_*-waarde wist het icoon in plaats van bij te werken.
	if nimAdd != 0 || nimModify != 1 || nimDelete != 2 || nimSetVersion != 4 {
		t.Fatalf("NIM-constanten kloppen niet: add=%d modify=%d delete=%d setversion=%d",
			nimAdd, nimModify, nimDelete, nimSetVersion)
	}
	if nifMessage != 1 || nifIcon != 2 || nifTip != 4 || nifShowTip != 0x80 {
		t.Fatalf("NIF-constanten kloppen niet: message=%#x icon=%#x tip=%#x showtip=%#x",
			nifMessage, nifIcon, nifTip, nifShowTip)
	}
}

func TestPutStrSluitAltijdAf(t *testing.T) {
	// ruim passend
	buf := make([]uint16, 8)
	putStr(buf, "abc")
	if buf[0] != 'a' || buf[1] != 'b' || buf[2] != 'c' || buf[3] != 0 {
		t.Errorf("korte tekst verkeerd weggeschreven: %v", buf[:5])
	}
	// precies te lang: er MOET een nul in staan (dit was de tray-bug)
	buf = make([]uint16, 8)
	putStr(buf, "1234567890")
	if buf[len(buf)-1] != 0 {
		t.Errorf("lange tekst zonder afsluitende nul: %v", buf)
	}
	// lege tekst
	buf = make([]uint16, 4)
	putStr(buf, "")
	if buf[0] != 0 {
		t.Errorf("lege tekst levert geen nul: %v", buf)
	}
}

func TestKortStatus(t *testing.T) {
	lang := "draait op de achtergrond — klik op het Wallpaparr-icoon in de systeembalk (of start de app opnieuw om dit venster terug te halen)"
	uit := kortStatus(lang)
	if len([]rune(uit)) > 101 {
		t.Errorf("kortStatus laat %d tekens door", len([]rune(uit)))
	}
	if uit == "" {
		t.Error("kortStatus gaf een lege tekst")
	}
	// meerregelig wordt één regel
	if uit := kortStatus("gezet:\r\nA: beeld"); uit != "gezet:" {
		t.Errorf("kortStatus nam niet de eerste regel: %q", uit)
	}
	if uit := kortStatus("   "); uit != "klaar" {
		t.Errorf("lege status hoort 'klaar' te zijn: %q", uit)
	}
}
