// Wallpaparr-client: haalt wallpapers op van de centrale Wallpaparr-server
// (PocketBase) en zet ze als bureaubladachtergrond, met ondersteuning voor
// multi-monitor setups. Op Windows start zonder argumenten de tray-app met
// instellingenvenster; op Unix gebruik je de opdrachten hieronder.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

const version = "0.2.0"

func usage() {
	fmt.Fprintf(os.Stderr, `wallpaparr %s — wallpaper-client voor de Wallpaparr-server

Gebruik:
  wallpaparr gui                                   tray-app met instellingenvenster (Windows)
  wallpaparr config --server https://<host>        server opslaan
  wallpaparr list  [--tag natuur] [--json]         wallpapers op de server tonen
  wallpaparr monitors                              monitoren tonen
  wallpaparr set   [--tag natuur] [--random] [--per-monitor]
                                                   één wallpaper zetten
  wallpaparr rotate --every 30m [--tag natuur] [--per-monitor]
                                                   blijven wisselen

Op Windows start een dubbelklik op de exe (zonder argumenten) de tray-app; het
kruisje verbergt hem naar de systeembalk, rechtsklik geeft het menu (o.a. Afsluiten).

Opties:
  --server URL      server-adres (ook via $WALLPAPARR_SERVER)
  --tag t1,t2       filter op tag(s); meerdere tags = OF
  --id ID           specifiek record gebruiken
  --random          willekeurige keuze in plaats van de nieuwste
  --per-monitor     per monitor een andere wallpaper (waar ondersteund)
  --position MODE   fill (standaard) | fit | stretch | center | span | tile
  --every DURATION  wisselinterval voor rotate (bv. 30m, 6h)
`, version)
}

type opts struct {
	server     string
	tag        string
	id         string
	random     bool
	perMonitor bool
	position   string
	every      string
	jsonOut    bool
}

func main() {
	if len(os.Args) < 2 {
		// dubbelklik op Windows: tray-app. Op Unix: gebruik tonen.
		if maybeStartGUI() {
			return
		}
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "gui":
		if !maybeStartGUI() {
			fail(fmt.Errorf("de GUI zit alleen in de Windows-client; gebruik op Unix set/rotate"))
		}
	case "config":
		cmdConfig(os.Args[2:])
	case "list":
		cmdList(os.Args[2:])
	case "monitors":
		cmdMonitors()
	case "set":
		cmdSet(os.Args[2:], false)
	case "rotate":
		cmdSet(os.Args[2:], true)
	case "version":
		fmt.Println("wallpaparr", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "onbekende opdracht %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func addCommon(fs *flag.FlagSet, o *opts) {
	fs.StringVar(&o.server, "server", "", "server-adres")
	fs.StringVar(&o.tag, "tag", "", "filter op tag(s)")
	fs.StringVar(&o.position, "position", "fill", "weergavemodus")
}

func cmdConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	var o opts
	fs.StringVar(&o.server, "server", "", "server-adres")
	_ = fs.Parse(args)
	cfg, err := loadConfig()
	if err != nil {
		fail(err)
	}
	if o.server != "" {
		cfg.Server = normalizeServer(o.server)
	}
	if err := saveConfig(cfg); err != nil {
		fail(err)
	}
	path, _ := configPath()
	fmt.Printf("Config: %s\nServer : %s\n", path, cfg.Server)
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	var o opts
	addCommon(fs, &o)
	fs.BoolVar(&o.jsonOut, "json", false, "ruwe JSON")
	_ = fs.Parse(args)

	cfg := mustConfig(o.server)
	recs, err := fetchRecords(cfg.Server, o.tag)
	if err != nil {
		fail(err)
	}
	if o.jsonOut {
		b, _ := json.MarshalIndent(recs, "", "  ")
		fmt.Println(string(b))
		return
	}
	if len(recs) == 0 {
		fmt.Println("Geen wallpapers gevonden.")
		return
	}
	fmt.Printf("%d wallpaper(s) op %s\n\n", len(recs), cfg.Server)
	for _, r := range recs {
		res := ""
		if r.Width > 0 {
			res = fmt.Sprintf("%dx%d", r.Width, r.Height)
		}
		fmt.Printf("  %-15s %-38s %-11s %s\n", r.ID, truncate(r.Title, 38), res, r.Tags)
	}
}

func cmdMonitors() {
	mons, err := listMonitors()
	if err != nil {
		fail(err)
	}
	fmt.Printf("%d monitor(en):\n", len(mons))
	for i, m := range mons {
		fmt.Printf("  [%d] %s\n", i, friendlyMonitor(m))
	}
}

func cmdSet(args []string, loop bool) {
	fs := flag.NewFlagSet("set", flag.ExitOnError)
	var o opts
	addCommon(fs, &o)
	fs.BoolVar(&o.random, "random", false, "willekeurige keuze")
	fs.BoolVar(&o.perMonitor, "per-monitor", false, "per monitor een andere wallpaper")
	fs.StringVar(&o.id, "id", "", "specifiek record")
	fs.StringVar(&o.every, "every", "", "wisselinterval")
	_ = fs.Parse(args)

	cfg := mustConfig(o.server)
	cfg = applyDefaults(cfg, o)
	if cfg.Server == "" {
		fail(fmt.Errorf("geen server ingesteld — gebruik: wallpaparr config --server http://<ip>"))
	}

	if !loop {
		if err := applyOnce(cfg, o); err != nil {
			fail(err)
		}
		return
	}

	d, err := time.ParseDuration(cfg.Every)
	if err != nil {
		fail(fmt.Errorf("ongeldig interval %q: %v", cfg.Every, err))
	}
	laatste := ""
	for {
		rec, err := applyChoice(cfg, o.random, laatste)
		if err != nil {
			fmt.Fprintf(os.Stderr, "let op: %v\n", err)
		} else {
			laatste = rec.ID
			fmt.Printf("wallpaper gezet: %s\n", rec.Title)
		}
		next := time.Now().Add(d).Format("15:04:05")
		fmt.Printf("volgende wissel om %s (elke %s) — stoppen met Ctrl+C\n", next, d)
		time.Sleep(d)
	}
}

// applyDefaults: CLI-waarden overschrijven de config, en de config vult aan.
func applyDefaults(cfg Config, o opts) Config {
	if o.server != "" {
		cfg.Server = normalizeServer(o.server)
	}
	if o.tag != "" {
		cfg.Tag = o.tag
	}
	if o.perMonitor {
		cfg.PerMonitor = true
	}
	if o.every != "" {
		cfg.Every = o.every
	}
	if o.position != "" {
		cfg.Position = o.position
	}
	if cfg.Every == "" {
		cfg.Every = "30m"
	}
	if cfg.Position == "" {
		cfg.Position = "fill"
	}
	return cfg
}

func applyOnce(cfg Config, o opts) error {
	recs, err := fetchRecords(cfg.Server, cfg.Tag)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return fmt.Errorf("geen wallpapers gevonden (tag=%q)", cfg.Tag)
	}
	want, mons := selectie(cfg)
	if want > len(recs) {
		want = len(recs)
	}
	chosen := pick(recs, want, o.id, o.random)
	if err := applyChosen(cfg, chosen, mons); err != nil {
		return err
	}
	rapporteer(chosen, mons)
	return nil
}

// applyChoice haalt zelf de wallpapers op en zet er één (of per monitor één).
// lastID wordt overgeslagen zodat een wissel nooit hetzelfde beeld herhaalt.
func applyChoice(cfg Config, random bool, lastID string) (Record, error) {
	recs, err := fetchRecords(cfg.Server, cfg.Tag)
	if err != nil {
		return Record{}, err
	}
	if len(recs) == 0 {
		return Record{}, fmt.Errorf("geen wallpapers gevonden (tag=%q)", cfg.Tag)
	}
	want, mons := selectie(cfg)
	if want > len(recs) {
		want = len(recs)
	}
	chosen := nextPick(recs, lastID, want, random)
	if err := applyChosen(cfg, chosen, mons); err != nil {
		return Record{}, err
	}
	return chosen[0], nil
}

// selectie bepaalt hoeveel wallpapers er nodig zijn en welke monitoren meedoen.
func selectie(cfg Config) (int, []string) {
	want := 1
	var mons []string
	if cfg.PerMonitor {
		if m, err := listMonitors(); err == nil && len(m) > 0 {
			mons = m
			want = len(m)
		}
	}
	return want, mons
}

// applyChosen downloadt de gekozen wallpapers en zet ze als achtergrond.
func applyChosen(cfg Config, chosen []Record, mons []string) error {
	paths := make([]string, 0, len(chosen))
	for _, r := range chosen {
		p, err := download(cfg.Server, r)
		if err != nil {
			return err
		}
		paths = append(paths, p)
	}
	if len(paths) == 1 || len(mons) <= 1 {
		return setWallpaperAll(paths[0], cfg.Position)
	}
	if err := setAllMonitors(mons, paths, cfg.Position); err != nil {
		fmt.Fprintf(os.Stderr, "let op: per-monitor niet gelukt (%v) — zelfde wallpaper op alle monitoren\n", err)
		return setWallpaperAll(paths[0], cfg.Position)
	}
	return nil
}

func rapporteer(chosen []Record, mons []string) {
	if len(chosen) == 1 || len(mons) <= 1 {
		fmt.Printf("wallpaper gezet: %s\n", chosen[0].Title)
		return
	}
	for i, m := range mons {
		fmt.Printf("monitor %d (%s): %s\n", i, friendlyMonitor(m), chosen[i%len(chosen)].Title)
	}
}

func setAllMonitors(mons []string, paths []string, position string) error {
	ok := 0
	for i, m := range mons {
		p := paths[i%len(paths)]
		if err := setMonitorWallpaper(m, i, p, position); err != nil {
			if i == 0 {
				return err
			}
			fmt.Fprintf(os.Stderr, "  monitor %s overgeslagen: %v\n", m, err)
			continue
		}
		ok++
	}
	if ok == 0 {
		return fmt.Errorf("geen enkele monitor kon apart worden ingesteld")
	}
	return nil
}

// pick kiest de records die gezet gaan worden (CLI: expliciet id of willekeurig).
func pick(recs []Record, want int, id string, random bool) []Record {
	if id != "" {
		for _, r := range recs {
			if r.ID == id {
				return []Record{r}
			}
		}
		return []Record{recs[0]}
	}
	if want <= 1 || len(recs) == 1 {
		if random {
			return []Record{recs[rand.Intn(len(recs))]}
		}
		return []Record{recs[0]}
	}
	idx := rand.Perm(len(recs))
	out := make([]Record, 0, want)
	for _, i := range idx[:want] {
		out = append(out, recs[i])
	}
	return out
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

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fout:", err)
	os.Exit(1)
}
