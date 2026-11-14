package main

import (
	"strings"
	"testing"
)

func rec(id string) Record { return Record{ID: id, Title: "titel-" + id} }

func TestBuildFilter(t *testing.T) {
	if got := buildFilter(""); got != "" {
		t.Errorf("leeg filter moet leeg zijn, kreeg %q", got)
	}
	if got := buildFilter("  ,  "); got != "" {
		t.Errorf("alleen spaties/komma's moet leeg zijn, kreeg %q", got)
	}
	if got := buildFilter("natuur"); got != "(tags~'natuur')" {
		t.Errorf("enkele tag: %q", got)
	}
	if got := buildFilter(" a , b "); got != "(tags~'a'||tags~'b')" {
		t.Errorf("twee tags met spaties: %q", got)
	}
	if got := buildFilter("o'brien"); !strings.Contains(got, `o\'brien`) {
		t.Errorf("quote moet geescaped worden: %q", got)
	}
}

func TestNormalizeServer(t *testing.T) {
	gevallen := map[string]string{
		"":                            "",
		"10.5.0.165":                  "http://10.5.0.165",
		"http://10.5.0.165/":          "http://10.5.0.165",
		"https://wallpaparr.xdio.nl/": "https://wallpaparr.xdio.nl",
		"  http://x  ":                "http://x",
	}
	for in, wil := range gevallen {
		if got := normalizeServer(in); got != wil {
			t.Errorf("normalizeServer(%q) = %q, wil %q", in, got, wil)
		}
	}
}

func TestPickMetId(t *testing.T) {
	recs := []Record{rec("1"), rec("2"), rec("3")}
	if got := pick(recs, 1, "2", false); len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("pick met id gaf %+v", got)
	}
	if got := pick(recs, 1, "onbekend", false); got[0].ID != "1" {
		t.Fatalf("onbekend id moet terugvallen op de eerste: %+v", got)
	}
}

func TestNextPickSlaatVorigeOver(t *testing.T) {
	recs := []Record{rec("1"), rec("2"), rec("3")}
	for i := 0; i < 30; i++ {
		got := nextPick(recs, "1", 1, true)
		if len(got) != 1 || got[0].ID == "1" {
			t.Fatalf("vorige wallpaper opnieuw gekozen: %+v", got)
		}
	}
	// niet-willekeurig: pakt de eerste die niet de vorige is
	if got := nextPick(recs, "1", 1, false); got[0].ID == "1" {
		t.Fatalf("moet een andere zijn dan de vorige: %+v", got)
	}
	// per monitor: unieke keuzes, vorige niet erin als dat kan
	got := nextPick(recs, "1", 2, true)
	if len(got) != 2 || got[0].ID == got[1].ID {
		t.Fatalf("per-monitor keuzes moeten verschillen: %+v", got)
	}
	for _, r := range got {
		if r.ID == "1" {
			t.Fatalf("vorige mag niet in de per-monitor selectie zitten: %+v", got)
		}
	}
}

func TestNextPickMetEenEnkeleWallpaper(t *testing.T) {
	recs := []Record{rec("1")}
	if got := nextPick(recs, "1", 1, true); got[0].ID != "1" {
		t.Fatalf("met één wallpaper is herhalen onvermijdelijk: %+v", got)
	}
	recs2 := []Record{rec("1"), rec("2")}
	if got := nextPick(recs2, "1", 2, true); len(got) != 2 {
		t.Fatalf("vraagt 2 van 2: moet beide geven ondanks de vorige: %+v", got)
	}
}
