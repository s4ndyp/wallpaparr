// Wallpaparr-client: haalt wallpapers op van de centrale Wallpaparr-server
// (PocketBase) en zet ze als bureaubladachtergrond, met ondersteuning voor
// multi-monitor setups en per-monitor profielen (tags + beeldverhouding).
//
// DE SERVER IS DE BAAS. Per scherm houdt de server in zijn playlist bij welke
// tags en verhoudingen mogen worden gebruikt, met welk interval en of het
// willekeurig is. De server wisselt elke X minuten een nieuwe afbeelding.
//
// De client luistert (polling, een paar seconden vertraging ok):
//   - meldt zijn monitoren aan de server (screen-upsert),
//   - pollt de server elke N seconde,
//   - zet een afbeelding op een monitor pas als de server ernaar verwijst,
//   - bewaart lokaal welke afbeelding er al op elke monitor zit,
//   - noteert het toegepaste beeld in de geschiedenis.
//
// Aan de server liggen de playlists (tags/verhoudingen/interval/selectie),
// de cron-rotatie (pb_hooks) en de geschiedenis.
//
// Gebruik:
//
//	wallpaparr config --server http://<host>       server opslaan
//	wallpaparr monitors                            je monitoren tonen
//	wallpaparr status                              wat de server NU op jouw schermen zet
//	wallpaparr luister                             blijven wisselen (polling, server stuurt)
//	wallpaparr set --id <id>                       pin: een afbeelding vastzetten
//	wallpaparr version                             versie
package main

import (
	"fmt"
	"os"
	"runtime"
)

const version = "0.7.0"

func usage() {
	fmt.Fprintf(os.Stderr, `wallpaparr %s — wallpaper-client voor de Wallpaparr-server (de server stuurt)

De server is de baas. Per scherm houdt de server in zijn playlist bij welke
tags en verhoudingen mogen worden gebruikt, met welk interval en of het
willekeurig is. De server wisselt elke X minuten een nieuwe afbeelding.

De client:
  - meldt zijn monitoren aan de server (screen-upsert),
  - pollt de server elke N seconde (standaard 5, 'slechts een paar seconden
    vertraging is acceptabel, SSE is overbodig),
  - zet een afbeelding op een monitor pas als de server ernaar verwijst,
  - bewaart lokaal welke afbeelding er al op elke monitor zit,
  - noteert het toegepaste beeld in de geschiedenis.

Gebruik:
  wallpaparr config --server http://<host>       server opslaan
  wallpaparr monitors                            je monitoren tonen
  wallpaparr status                              wat de server NU op jouw schermen zet
  wallpaparr luister                             blijven wisselen (polling, server stuurt)
  wallpaparr set --id <id>                       pin: een afbeelding vastzetten
  wallpaparr version                             versie
`, version)
}

func main() {
	if len(os.Args) < 2 {
		// Windows: dubbelklik = tray-app. Op Unix: luister.
		if runtime.GOOS == "windows" {
			if maybeStartGUI() {
				return
			}
			fmt.Fprintln(os.Stderr, "Geen argumenten en geen Windows-tray beschikbaar — gebruik: wallpaparr luister")
			os.Exit(2)
		}
		cmdLuister(nil)
		return
	}
	switch os.Args[1] {
	case "config":
		cmdConfig(os.Args[2:])
	case "monitors":
		cmdMonitors()
	case "status":
		cmdStatus()
	case "luister":
		cmdLuister(os.Args[2:])
	case "set":
		cmdSet(os.Args[2:])
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

// runDefault: zonder argumenten op Windows een DoubleKlik de tray-app; op Unix
// luister met de standaardconfiguratie.
func runDefault() {
	if runtime.GOOS == "windows" {
		if maybeStartGUI() {
			return
		}
		fmt.Fprintln(os.Stderr, "Geen argumenten en geen Windows-tray beschikbaar — gebruik: wallpaparr luister")
		os.Exit(2)
	}
	cmdLuister(nil)
}
