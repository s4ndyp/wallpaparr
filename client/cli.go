// Command-line interface for wallpaparr.
//
// De client is nu een luisteraar (polling, een paar seconden vertraging ok):
// configuratie, monitoren, status en het "luister"-mode.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "wallpaparr", "state.json"), nil
}

func hostname() string {
	if h := os.Getenv("COMPUTERNAME"); h != "" {
		return h
	}
	if h := os.Getenv("HOSTNAME"); h != "" {
		return h
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "wallpaparr-client"
}

// kiesDevice geeft het device-ID. Een pad (oude configs vulden de thuismap) of
// een leeg veld wordt vervangen door de machinenaam.
func kiesDevice(cfg *Config) string {
	d := strings.TrimSpace(cfg.Device)
	if d == "" || strings.HasPrefix(d, "/") || strings.ContainsAny(d, `\\/`) {
		d = hostname()
		cfg.Device = d
	}
	return d
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fout:", err)
	os.Exit(1)
}

func cmdConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	var o struct {
		Server string
	}
	fs.StringVar(&o.Server, "server", "", "server-adres (bv. http://10.5.0.165)")
	_ = fs.Parse(args)

	cfg, err := loadConfig()
	if err != nil {
		fail(err)
	}
	if o.Server != "" {
		cfg.Server = normalizeServer(o.Server)
		if cfg.Device == "" {
			cfg.Device = hostname()
		}
	}
	if err := cfg.Save(); err != nil {
		fail(err)
	}
	path, err := configPath()
	if err != nil {
		fail(err)
	}
	fmt.Printf("Config : %s\nServer : %s\nDevice : %s\nPoll   : %d s\n", path, cfg.Server, cfg.Device, cfg.Poll)
}

func cmdMonitors() {
	mons, err := listMonitors()
	if err != nil {
		fail(err)
	}
	fmt.Printf("%d monitor(en): %s\n", len(mons), strings.Join(mons, ", "))
}

func cmdStatus() {
	cfg, err := loadConfig()
	if err != nil {
		fail(err)
	}
	if cfg.Server == "" {
		fail(fmt.Errorf("geen server ingesteld — eerst: wallpaparr config --server <url>"))
	}
	screens, err := screensForDevice(cfg.Server, kiesDevice(&cfg))
	if err != nil {
		fail(err)
	}
	if len(screens) == 0 {
		fmt.Println("Geen schermen bekend bij de server.")
		return
	}
	for _, s := range screens {
		var wp Record
		if s.HuidigeWallpaper != "" {
			wp, err = getWallpaper(cfg.Server, s.HuidigeWallpaper)
			if err != nil {
				wp.Title = "onbekend"
			}
		}
		fmt.Printf("%-40s %dx%d | %-28s %-14s %-10s %s\n",
			s.Monitor, s.Width, s.Height, s.Key,
			wp.Title, s.Bron, formatSince(s.LaatstGewien))
	}
}

func cmdSet(args []string) {
	fs := flag.NewFlagSet("set", flag.ExitOnError)
	var o struct {
		Server string
		ID     string
	}
	fs.StringVar(&o.Server, "server", "", "server-adres (override)")
	fs.StringVar(&o.ID, "id", "", "afbeelding-id om vast te zetten")
	_ = fs.Parse(args)

	cfg, err := loadConfig()
	if err != nil {
		fail(err)
	}
	if o.Server != "" {
		cfg.Server = normalizeServer(o.Server)
	}
	if cfg.Server == "" {
		fail(fmt.Errorf("geen server ingesteld — eerst: wallpaparr config --server <url>"))
	}
	if o.ID == "" {
		fail(fmt.Errorf("geef een afbeelding: wallpaparr set --id <id>"))
	}
	recs, err := fetchRecords(cfg.Server, "")
	if err != nil {
		fail(err)
	}
	var kies *Record
	for i := range recs {
		if recs[i].ID == o.ID {
			kies = &recs[i]
			break
		}
	}
	if kies == nil {
		fail(fmt.Errorf("afbeelding %q niet gevonden", o.ID))
	}
	mons, err := listMonitors()
	if err != nil {
		fail(err)
	}
	for i, m := range mons {
		wpath, err := download(cfg.Server, *kies)
		if err != nil {
			fail(err)
		}
		if err := setMonitorWallpaper(m, idxWithin(mons, i), wpath, cfg.Position); err != nil {
			fail(err)
		}
		fmt.Printf("Pin: %-5s %s (%dx%d) zodat %s (%s)\n", m, kies.Title, kies.Width, kies.Height, kies.File, m)
	}
	fmt.Printf("Pin gezet: %s (%dx%d) op alle %d monitoren\n", kies.Title, kies.Width, kies.Height, len(mons))
}

func parseMonitoren(s string) []string {
	if s == "" {
		return nil
	}
	var uit []string
	for _, stuk := range strings.Split(s, ",") {
		stuk = strings.TrimSpace(stuk)
		if stuk == "" {
			continue
		}
		uit = append(uit, stuk)
	}
	return uit
}

// idxWithin geeft de positie van mons[i] tussen de eerdere gelijknamige monitoren
// (0,1,2 bij drie keer dezelfde naam). countWithin telt álle voorkomens en is dus
// ongeschikt voor de schermsleutel.
func idxWithin(mons []string, i int) int {
	n := 0
	for j := 0; j < i; j++ {
		if mons[j] == mons[i] {
			n++
		}
	}
	return n
}

// countWithin telt hoeveel keer naam al voorkomt (voor de key-index).
func countWithin(mons []string, naam string) int {
	n := 0
	for _, m := range mons {
		if m == naam {
			n++
		}
	}
	return n
}

// formatSince formateert een tijdstempel (of leeg) naar 13:41.
func formatSince(s string) string {
	if s == "" {
		return "-"
	}
	t, err := time.Parse("2006-01-02 15:04:05.000Z", s)
	if err != nil {
		return s
	}
	return t.Format("15:04")
}

// cmdLuister: de luister-loop — de server is de baas, de client pollt.
func cmdLuister(args []string) {
	fs := flag.NewFlagSet("luister", flag.ExitOnError)
	var o struct {
		Server   string
		Poll     int
		Monitors string
	}
	fs.StringVar(&o.Server, "server", "", "server-adres (override)")
	fs.IntVar(&o.Poll, "poll", 0, "poll-interval in seconden (0 = uit config, standaard 5)")
	fs.StringVar(&o.Monitors, "monitors", "", "komma-lijst met monitoren (standaard: alle)")
	_ = fs.Parse(args)

	cfg, err := loadConfig()
	if err != nil {
		fail(err)
	}
	if o.Server != "" {
		cfg.Server = normalizeServer(o.Server)
	}
	if cfg.Server == "" {
		fail(fmt.Errorf("geen server ingesteld — eerst: wallpaparr config --server <url>"))
	}
	poll := cfg.Poll
	if o.Poll > 0 {
		poll = o.Poll
	}
	if poll <= 0 {
		poll = 5
	}

	mons := parseMonitoren(o.Monitors)
	if len(mons) == 0 {
		mons, err = listMonitors()
		if err != nil {
			fail(err)
		}
	}
	if len(mons) == 0 {
		fail(fmt.Errorf("geen monitoren gevonden"))
	}

	device := kiesDevice(&cfg)
	if err := cfg.Save(); err != nil {
		fail(err)
	}

	sp, err := statePath()
	if err != nil {
		fail(err)
	}
	fmt.Printf("Wallpaparr-client v%s\n", version)
	fmt.Printf("Luisteren op %s — %d monitor(en), poll elke %d s (stop met Ctrl-C)\n", cfg.Server, len(mons), poll)
	fmt.Printf("Monitoren: %s\n", strings.Join(mons, ", "))

	apply := func(screenID, monitor string, wp Record) error {
		return werk(cfg.Server, screenID, monitor, wp, false)
	}
	if err := runPlayer(cfg.Server, device, mons, time.Duration(poll)*time.Second, sp, apply); err != nil {
		fail(err)
	}
}
