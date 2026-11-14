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
