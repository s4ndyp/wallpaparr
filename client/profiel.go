// Wallpaparr-client — per-monitor profielen: welke tags en beeldverhoudingen
// tellen mee voor welke monitor. Een wallpaper die niet voldoet wordt overgeslagen.
package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
)

// MonitorProfiel bepaalt welke wallpapers voor één monitor in aanmerking komen.
// Lege Tags = alle tags; lege Ratios = alle verhoudingen.
type MonitorProfiel struct {
	Monitor string   `json:"monitor"`
	Tags    []string `json:"tags,omitempty"`
	Ratios  []string `json:"ratios,omitempty"`
}

// verhoudingen volgt de web-UI: 10% tolerantie en NIET symmetrisch, dus een
// gekantelde 9:16 hoort bewust niet bij de 16:9-groep.
var verhoudingen = []struct {
	Key string
	V   float64
}{
	{"1:1", 1},
	{"1:2", 0.5},
	{"2:3", 2.0 / 3},
	{"16:9", 16.0 / 9},
	{"21:9", 21.0 / 9},
	{"27:9", 27.0 / 9},
}

const verhoudingTolerantie = 0.10

// ratioBucket geeft de verhoudinggroep van een record, of "overig".
func ratioBucket(r Record) string {
	if r.Width <= 0 || r.Height <= 0 {
		return "overig"
	}
	x := float64(r.Width) / float64(r.Height)
	best, bestD := "", math.Inf(1)
	for _, v := range verhoudingen {
		d := math.Abs(math.Log(x / v.V))
		if d < bestD {
			bestD, best = d, v.Key
		}
	}
	if bestD <= verhoudingTolerantie {
		return best
	}
	return "overig"
}

// heeftTag: staat deze tag (hoofdletterongevoelig) op het record?
func heeftTag(r Record, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return true
	}
	for _, t := range tagList(r.Tags) {
		if strings.ToLower(t) == tag {
			return true
		}
	}
	return false
}

// kandidatenVoor filtert de records op het profiel van één monitor. gebruikGlobaal
// laat de globale tag meewegen (als de client niet per monitor werkt).
func kandidatenVoor(recs []Record, p MonitorProfiel, globaalTag string, gebruikGlobaal bool) []Record {
	tags := p.Tags
	if len(tags) == 0 && gebruikGlobaal {
		tags = tagList(globaalTag)
	}
	ratioSet := map[string]bool{}
	for _, r := range p.Ratios {
		ratioSet[r] = true
	}
	uit := make([]Record, 0, len(recs))
	for _, r := range recs {
		ok := true
		for _, t := range tags {
			if !heeftTag(r, t) {
				ok = false
				break
			}
		}
		if ok && len(ratioSet) > 0 && !ratioSet[ratioBucket(r)] {
			ok = false
		}
		if ok {
			uit = append(uit, r)
		}
	}
	return uit
}

// profielVoor zoekt het profiel van een monitor: eerst exact, anders op deelnaam
// (COM-paden veranderen als je een monitor opnieuw aansluit).
func (c Config) profielVoor(monitor string) MonitorProfiel {
	if monitor == "" {
		return MonitorProfiel{}
	}
	for _, p := range c.Monitors {
		if strings.EqualFold(p.Monitor, monitor) {
			return p
		}
	}
	for _, p := range c.Monitors {
		if p.Monitor == "" {
			continue
		}
		if strings.Contains(strings.ToLower(monitor), strings.ToLower(p.Monitor)) {
			return p
		}
	}
	return MonitorProfiel{}
}

// metProfiel vervangt of voegt het profiel van één monitor toe.
func (c Config) metProfiel(p MonitorProfiel) Config {
	uit := make([]MonitorProfiel, 0, len(c.Monitors)+1)
	vervangen := false
	for _, bestaand := range c.Monitors {
		if strings.EqualFold(bestaand.Monitor, p.Monitor) {
			uit = append(uit, p)
			vervangen = true
		} else {
			uit = append(uit, bestaand)
		}
	}
	if !vervangen {
		uit = append(uit, p)
	}
	c.Monitors = uit
	return c
}

// profielOmschrijving maakt een leesbare samenvatting van een profiel.
func profielOmschrijving(p MonitorProfiel) string {
	tags := "alle tags"
	if len(p.Tags) > 0 {
		tags = strings.Join(p.Tags, ",")
	}
	ratios := "alle verhoudingen"
	if len(p.Ratios) > 0 {
		ratios = strings.Join(p.Ratios, ",")
	}
	return tags + " · " + ratios
}

// zonderGebruikt laat beelden weg die in deze ronde al op een ander scherm staan.
// Geen terugval: één wallpaper mag maar op één monitor komen — is er niets unieks
// meer over, dan wordt die monitor overgeslagen in plaats van het beeld te herhalen.
func zonderGebruikt(kand []Record, gebruikt map[string]bool) []Record {
	if len(gebruikt) == 0 {
		return kand
	}
	uit := make([]Record, 0, len(kand))
	for _, r := range kand {
		if !gebruikt[r.ID] {
			uit = append(uit, r)
		}
	}
	return uit
}

// resultaat = wat er op één monitor is gezet.
type resultaat struct {
	Monitor string
	Index   int
	Rec     Record
}

// applyRound zet op elke monitor één wallpaper die aan het profiel voldoet en slaat
// beelden over die niet passen. laatste houdt per monitor het vorige id bij zodat een
// wissel niet hetzelfde beeld herhaalt (mag nil zijn).
func applyRound(cfg Config, random bool, laatste map[string]string) ([]resultaat, []string, error) {
	recs, err := fetchRecords(cfg.Server, cfg.Tag)
	if err != nil {
		return nil, nil, err
	}
	if len(recs) == 0 {
		return nil, nil, fmt.Errorf("geen wallpapers op de server (tag=%q)", cfg.Tag)
	}

	mons := []string{""}
	if cfg.PerMonitor {
		if m, err := listMonitors(); err == nil && len(m) > 0 {
			mons = m
		}
	}

	var uit []resultaat
	var meldingen []string
	gebruikt := map[string]bool{} // binnen deze ronde al geplaatste beelden
	for i, m := range mons {
		p := cfg.profielVoor(m)
		kand := kandidatenVoor(recs, p, cfg.Tag, !cfg.PerMonitor)
		if len(kand) == 0 {
			meldingen = append(meldingen, fmt.Sprintf("%s: geen wallpapers die aan %s voldoen", monitorLabel(m, i), profielOmschrijving(p)))
			continue
		}
		// sleutel per monitorrij (niet per naam): bij drie identieke schermen moet
		// elk scherm een andere afbeelding kunnen krijgen
		sleutel := monitorLabel(m, i)
		kand = zonderGebruikt(kand, gebruikt)
		if len(kand) == 0 {
			meldingen = append(meldingen, fmt.Sprintf(
				"%s: alle %d kandidaten staan al op een ander scherm — monitor overgeslagen",
				sleutel, len(kandidatenVoor(recs, cfg.profielVoor(m), cfg.Tag, !cfg.PerMonitor))))
			continue
		}
		kies := nextPick(kand, laatste[sleutel], 1, random)[0]
		gebruikt[kies.ID] = true
		if laatste != nil {
			laatste[sleutel] = kies.ID
		}
		pad, err := download(cfg.Server, kies)
		if err != nil {
			meldingen = append(meldingen, fmt.Sprintf("%s: %v", monitorLabel(m, i), err))
			continue
		}
		if m == "" {
			err = setWallpaperAll(pad, cfg.Position)
		} else {
			err = setMonitorWallpaper(m, i, pad, cfg.Position)
		}
		if err != nil {
			meldingen = append(meldingen, fmt.Sprintf("%s: %v", monitorLabel(m, i), err))
			continue
		}
		uit = append(uit, resultaat{Monitor: m, Index: i, Rec: kies})
	}
	if len(uit) == 0 {
		if len(meldingen) > 0 {
			return nil, meldingen, fmt.Errorf("%s", strings.Join(meldingen, "; "))
		}
		return nil, nil, fmt.Errorf("geen enkele monitor ingesteld")
	}
	return uit, meldingen, nil
}

// profielSamenvatting laat per monitor zien hoeveel wallpapers er met het huidige
// profiel in aanmerking komen (diagnose bij de instellingen).
func profielSamenvatting(cfg Config) ([]string, error) {
	recs, err := fetchRecords(cfg.Server, cfg.Tag)
	if err != nil {
		return nil, err
	}
	mons := []string{""}
	if cfg.PerMonitor {
		if m, err := listMonitors(); err == nil && len(m) > 0 {
			mons = m
		}
	}
	var uit []string
	for i, m := range mons {
		p := cfg.profielVoor(m)
		n := len(kandidatenVoor(recs, p, cfg.Tag, !cfg.PerMonitor))
		uit = append(uit, fmt.Sprintf("%s: %d van %d wallpapers — %s", monitorLabel(m, i), n, len(recs), profielOmschrijving(p)))
	}
	return uit, nil
}

func monitorLabel(m string, i int) string {
	if m == "" {
		return "alle monitoren"
	}
	return fmt.Sprintf("%s (#%d)", friendlyMonitor(m), i)
}

// nextPick kiest voor een wissel: de vorige wallpaper wordt overgeslagen zolang er
// genoeg andere zijn, zodat je nooit twee keer hetzelfde beeld krijgt.
func nextPick(recs []Record, lastID string, want int, random bool) []Record {
	kandidaten := recs
	if lastID != "" && len(recs) > want {
		var overig []Record
		for _, r := range recs {
			if r.ID != lastID {
				overig = append(overig, r)
			}
		}
		if len(overig) >= want {
			kandidaten = overig
		}
	}
	if want <= 1 || len(kandidaten) == 1 {
		if random {
			return []Record{kandidaten[rand.Intn(len(kandidaten))]}
		}
		return []Record{kandidaten[0]}
	}
	idx := rand.Perm(len(kandidaten))
	out := make([]Record, 0, want)
	for _, i := range idx[:want] {
		out = append(out, kandidaten[i])
	}
	return out
}
