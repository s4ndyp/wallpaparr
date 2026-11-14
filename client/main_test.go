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

/* ---------- verhoudinggroepen ---------- */

func recWH(id string, w, h int, tags string) Record {
	return Record{ID: id, Title: "titel-" + id, Width: w, Height: h, Tags: tags}
}

func TestRatioBucket(t *testing.T) {
	gevallen := []struct {
		w, h int
		wil  string
	}{
		{2912, 1632, "16:9"}, // de AI-wallpapers van de gebruiker
		{1920, 1080, "16:9"},
		{3840, 2160, "16:9"},
		{3360, 1440, "21:9"},
		{3440, 1440, "21:9"},
		{2400, 1080, "21:9"},
		{3840, 1200, "27:9"}, // 3,2 zit binnen 10% van 3,0
		{1000, 1000, "1:1"},
		{2000, 3000, "2:3"},
		{1080, 1920, "overig"}, // gekanteld hoort er bewust niet bij
		{2000, 1000, "overig"}, // 2:1 is geen 1:2
		{1600, 1200, "overig"}, // 4:3
		{0, 0, "overig"},
	}
	for _, g := range gevallen {
		r := Record{Width: g.w, Height: g.h}
		if got := ratioBucket(r); got != g.wil {
			t.Errorf("ratioBucket(%dx%d) = %q, wil %q", g.w, g.h, got, g.wil)
		}
	}
}

/* ---------- per-monitor profielen ---------- */

func TestKandidatenVoorTagsEnVerhouding(t *testing.T) {
	recs := []Record{
		recWH("a", 3360, 1440, "natuur,nacht"),
		recWH("b", 3360, 1440, "abstract"),
		recWH("c", 2912, 1632, "natuur"),
		recWH("d", 1000, 1000, "natuur"),
	}
	got := kandidatenVoor(recs, MonitorProfiel{Tags: []string{"natuur"}, Ratios: []string{"21:9"}}, "", true)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("profiel natuur+21:9 gaf %v", ids(got))
	}
	got = kandidatenVoor(recs, MonitorProfiel{Ratios: []string{"16:9"}}, "", true)
	if len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("profiel 16:9 gaf %v", ids(got))
	}
	got = kandidatenVoor(recs, MonitorProfiel{}, "", true)
	if len(got) != 4 {
		t.Fatalf("leeg profiel gaf %v", ids(got))
	}
	got = kandidatenVoor(recs, MonitorProfiel{Tags: []string{"natuur", "nacht"}}, "", true)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("tags moeten allemaal aanwezig zijn (AND): %v", ids(got))
	}
	got = kandidatenVoor(recs, MonitorProfiel{}, "abstract", true)
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("globale tag gaf %v", ids(got))
	}
	got = kandidatenVoor(recs, MonitorProfiel{}, "abstract", false)
	if len(got) != 4 {
		t.Fatalf("bij per-monitor mag de globale tag niet meewegen: %v", ids(got))
	}
	got = kandidatenVoor(recs, MonitorProfiel{Tags: []string{"bestaatniet"}}, "", true)
	if len(got) != 0 {
		t.Fatalf("verwachtte geen kandidaten, kreeg %v", ids(got))
	}
}

func TestProfielVoor(t *testing.T) {
	cfg := Config{Monitors: []MonitorProfiel{
		{Monitor: "AOC2702", Tags: []string{"natuur"}},
		{Monitor: `\\?\DISPLAY#DELD128#5&1234`, Ratios: []string{"27:9"}},
	}}
	if p := cfg.profielVoor("AOC2702"); len(p.Tags) != 1 {
		t.Fatalf("exacte match mislukt: %+v", p)
	}
	// deelnaam: het COM-pad verandert als je een monitor opnieuw aansluit
	if p := cfg.profielVoor(`\\?\DISPLAY#AOC2702#9&9999&0&UID4352`); len(p.Tags) != 1 {
		t.Fatalf("deelnaam-match mislukt: %+v", p)
	}
	if p := cfg.profielVoor("Onbekend"); len(p.Tags) != 0 || len(p.Ratios) != 0 {
		t.Fatalf("onbekende monitor hoort een leeg profiel te geven: %+v", p)
	}
}

func TestMetProfielVervangt(t *testing.T) {
	cfg := Config{Monitors: []MonitorProfiel{{Monitor: "A", Tags: []string{"x"}}}}
	cfg = cfg.metProfiel(MonitorProfiel{Monitor: "A", Tags: []string{"y"}})
	if len(cfg.Monitors) != 1 || cfg.Monitors[0].Tags[0] != "y" {
		t.Fatalf("vervangen mislukt: %+v", cfg.Monitors)
	}
	cfg = cfg.metProfiel(MonitorProfiel{Monitor: "B", Ratios: []string{"21:9"}})
	if len(cfg.Monitors) != 2 {
		t.Fatalf("toevoegen mislukt: %+v", cfg.Monitors)
	}
}

func TestZonderGebruikt(t *testing.T) {
	kand := []Record{rec("a"), rec("b"), rec("c")}
	if got := zonderGebruikt(kand, nil); len(got) != 3 {
		t.Fatalf("zonder gebruikte beelden moet alles blijven: %v", ids(got))
	}
	got := zonderGebruikt(kand, map[string]bool{"a": true, "b": true})
	if len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("al gebruikte beelden moeten wegvallen: %v", ids(got))
	}
	alles := map[string]bool{"a": true, "b": true, "c": true}
	if got := zonderGebruikt(kand, alles); len(got) != 3 {
		t.Fatalf("als alles gebruikt is mag herhalen: %v", ids(got))
	}
}

func ids(recs []Record) []string {
	uit := make([]string, 0, len(recs))
	for _, r := range recs {
		uit = append(uit, r.ID)
	}
	return uit
}
