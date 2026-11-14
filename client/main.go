// Wallpaparr-client: haalt wallpapers op van de centrale Wallpaparr-server
// (PocketBase) en zet ze als bureaubladachtergrond, met ondersteuning voor
// multi-monitor setups.
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

const version = "0.1.0"

func usage() {
	fmt.Fprintf(os.Stderr, `wallpaparr %s — wallpaper-client voor de Wallpaparr-server

Gebruik:
  wallpaparr config --server http://10.5.0.171     server opslaan
  wallpaparr list  [--tag natuur] [--json]         wallpapers op de server tonen
  wallpaparr monitors                              monitoren tonen
  wallpaparr set   [--tag natuur] [--random] [--per-monitor]
                                                   één wallpaper zetten
  wallpaparr rotate --every 30m [--tag natuur] [--per-monitor]
                                                   blijven wisselen

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
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
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
	for {
		if err := applyOnce(cfg, o); err != nil {
			fmt.Fprintf(os.Stderr, "let op: %v\n", err)
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

	want := 1
	var mons []string
	if cfg.PerMonitor {
		if m, err := listMonitors(); err == nil && len(m) > 0 {
			mons = m
			want = len(m)
		}
	}
	if want > len(recs) {
		want = len(recs)
	}

	chosen := pick(recs, want, o.id, o.random)
	paths := make([]string, 0, len(chosen))
	for _, r := range chosen {
		p, err := download(cfg.Server, r)
		if err != nil {
			return err
		}
		paths = append(paths, p)
	}

	if len(paths) == 1 || len(mons) <= 1 {
		if err := setWallpaperAll(paths[0], cfg.Position); err != nil {
			return err
		}
		fmt.Printf("wallpaper gezet: %s\n", chosen[0].Title)
		return nil
	}

	// Per monitor een andere wallpaper; valt terug op één als het platform
	// (of de aanwezige tools) het niet ondersteunen.
	if err := setAllMonitors(mons, paths, cfg.Position); err != nil {
		fmt.Fprintf(os.Stderr, "let op: per-monitor niet gelukt (%v) — zelfde wallpaper op alle monitoren\n", err)
		if err := setWallpaperAll(paths[0], cfg.Position); err != nil {
			return err
		}
		return nil
	}
	for i, m := range mons {
		fmt.Printf("monitor %d (%s): %s\n", i, friendlyMonitor(m), chosen[i%len(chosen)].Title)
	}
	return nil
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

// pick kiest de records die gezet gaan worden.
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
