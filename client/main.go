// Wallpaparr-client: haalt wallpapers op van de centrale Wallpaparr-server
// (PocketBase) en zet ze als bureaubladachtergrond, met ondersteuning voor
// multi-monitor setups en per-monitor profielen (tags + beeldverhouding).
// Op Windows start zonder argumenten de tray-app met instellingenvenster.
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

const version = "0.3.0"

func usage() {
	fmt.Fprintf(os.Stderr, `wallpaparr %s — wallpaper-client voor de Wallpaparr-server

Gebruik:
  wallpaparr gui                                   tray-app met instellingenvenster (Windows)
  wallpaparr config --server https://<host>        server opslaan
  wallpaparr list  [--tag natuur] [--json]         wallpapers op de server tonen
  wallpaparr monitors                              monitoren tonen
  wallpaparr profiles                              per monitor: hoeveel wallpapers passen
  wallpaparr set   [--tag natuur] [--random] [--per-monitor]
                                                   één wallpaper zetten
  wallpaparr rotate --every 30m [--per-monitor]    blijven wisselen

Per-monitor profielen staan in de config (of in de GUI van de Windows-client): per
monitor geef je op welke tags meetellen (leeg = alle tags) en welke beeldverhoudingen
mee mogen (niets aangevinkt = alle). Wallpapers die niet voldoen worden voor die monitor
overgeslagen — zo kun je een ultrawide alleen 21:9 of 27:9 laten pakken.

Op Windows start een dubbelklik op de exe (zonder argumenten) de tray-app; het
kruisje verbergt hem naar de systeembalk, rechtsklik geeft het menu (o.a. Afsluiten).

Opties:
  --server URL      server-adres (ook via $WALLPAPARR_SERVER)
  --tag t1,t2       filter op tag(s) — alleen als je niet per monitor werkt
  --id ID           specifiek record gebruiken (negeert de profielen)
  --random          willekeurige keuze in plaats van de nieuwste
  --per-monitor     per monitor een eigen wallpaper (via de profielen)
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
	case "profiles":
		cmdProfiles(os.Args[2:])
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
	if err := cfg.Save(); err != nil {
		fail(err)
	}
	path, _ := configPath()
	fmt.Printf("Config : %s\nServer : %s\n", path, cfg.Server)
	fmt.Printf("Globale tag: %q — per monitor: %v\n", cfg.Tag, cfg.PerMonitor)
	for _, p := range cfg.Monitors {
		fmt.Printf("Profiel %-22s %s\n", p.Monitor+":", profielOmschrijving(p))
	}
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
		fmt.Printf("  %-15s %-34s %-11s %-7s %s\n", r.ID, truncate(r.Title, 34), res, ratioBucket(r), r.Tags)
	}
}

func cmdMonitors() {
	mons, err := listMonitors()
	if err != nil {
		fail(err)
	}
	fmt.Printf("%d monitor(en):\n", len(mons))
	for i, m := range mons {
		fmt.Printf("  [%d] %s\n", i, monitorLabel(m, i))
	}
}

// cmdProfiles laat per monitor zien hoeveel wallpapers er met het profiel van die
// monitor in aanmerking komen — de snelste controle of je instellingen kloppen.
func cmdProfiles(args []string) {
	fs := flag.NewFlagSet("profiles", flag.ExitOnError)
	var o opts
	addCommon(fs, &o)
	_ = fs.Parse(args)
	cfg := applyDefaults(mustConfig(o.server), o) // --tag moet meetellen
	regels, err := profielSamenvatting(cfg)
	if err != nil {
		fail(err)
	}
	for _, r := range regels {
		fmt.Println(" ", r)
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

	// een expliciet record: profielen even niet meewegen
	if o.id != "" {
		if err := zetSpecifiek(cfg, o.id); err != nil {
			fail(err)
		}
		return
	}

	laatste := map[string]string{}
	ronde := func() error {
		res, meldingen, err := applyRound(cfg, o.random, laatste)
		for _, m := range meldingen {
			fmt.Fprintln(os.Stderr, "let op:", m)
		}
		if err != nil {
			return err
		}
		for _, r := range res {
			laatste[r.Monitor] = r.Rec.ID
			fmt.Printf("%-26s %s (%dx%d)\n", monitorLabel(r.Monitor, r.Index)+":",
				r.Rec.Title, r.Rec.Width, r.Rec.Height)
		}
		return nil
	}

	if !loop {
		if err := ronde(); err != nil {
			fail(err)
		}
		return
	}

	d, err := time.ParseDuration(cfg.Every)
	if err != nil {
		fail(fmt.Errorf("ongeldig interval %q: %v", cfg.Every, err))
	}
	for {
		if err := ronde(); err != nil {
			fmt.Fprintf(os.Stderr, "let op: %v\n", err)
		}
		next := time.Now().Add(d).Format("15:04:05")
		fmt.Printf("volgende wissel om %s (elke %s) — stoppen met Ctrl+C\n", next, d)
		time.Sleep(d)
	}
}

// zetSpecifiek zet één opgegeven record op alle monitoren (--id).
func zetSpecifiek(cfg Config, id string) error {
	recs, err := fetchRecords(cfg.Server, cfg.Tag)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return fmt.Errorf("geen wallpapers op de server")
	}
	kies := pick(recs, 1, id, false)[0]
	pad, err := download(cfg.Server, kies)
	if err != nil {
		return err
	}
	if err := setWallpaperAll(pad, cfg.Position); err != nil {
		return err
	}
	fmt.Printf("wallpaper gezet: %s\n", kies.Title)
	return nil
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

// pick kiest de records die gezet gaan worden (expliciet id of willekeurig).
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
