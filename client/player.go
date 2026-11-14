// Player: luistert naar de server. Elke poll voert deze stappen uit:
//
//  1. de schermen opzoeken (sync op device-ID),
//  2. per monitor het scherm-register upserte (laatst_gezien + index + resolutie),
//  3. elke screen waarvan de server een nieuwe wallpaper heeft ingesteld,
//     downloaden en op die monitor zetten,
//  4. het toegepaste beeld in de geschiedenis zetten en de swap-state bewaren.
//
// De server bepaalt dus wat er op welk scherm komt; de client past alleen aan
// wanneer de server ernaar verwijst. Polling met enkele seconden vertraging is
// acceptabel (SSE is overbodig).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// applier wordt aangeroepen wanneer de server een nieuwe wallpaper voor een
// monitor heeft ingesteld. De client downloadt (uit de cache), zet het op die
// monitor en noteert het in de geschiedenis.
type applier func(screenID, monitor string, wp Record) error

// runPlayer de luister-loop: elke poll seconden zoekt de server naar de schermen
// van device, zorgt dat elk lokaal bemonsterd een scherm-record heeft en past
// aan wanneer de server verwijst.
func runPlayer(server string, monitoren []string, poll time.Duration, statePath string, apply applier) error {
	device := hostname()
	if device == "" {
		device = "wallpaparr-client"
	}
	last := loadState(statePath)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		now := time.Now().Format("2006-01-02 15:04:05.000Z")

		// stap 1-2: de schermen van de server ophalen en per monitor upsrvaten
		screens, err := screensForDevice(server, device)
		if err != nil {
			return fmt.Errorf("schermen ophalen: %w", err)
		}
		byKey := map[string]Screen{}
		for _, s := range screens {
			byKey[s.Key] = s
		}

		// per monitor: zorg dat er een scherm-register is
		for _, m := range monitoren {
			n := countWithin(monitoren, m)
			s := Screen{
				Key:          device + "|" + m + "|" + strconv.Itoa(n),
				Device:       device,
				Monitor:      m,
				Index:        n,
				LaatstGewien: now,
			}
			if err := screenUpsert(server, s); err != nil {
				fmt.Fprintf(os.Stderr, "let op: scherm %q kon niet registreren: %v\n", s.Key, err)
			}
		}

		// stap 3: apply wanneer de server ernaar verwijst
		for _, s := range screens {
			if s.HuidigeWallpaper == "" {
				continue
			}
			if s.HuidigeWallpaper == last[s.Key] {
				continue // met de swap-state: geen oneffenheid
			}
			wp, err := getWallpaper(server, s.HuidigeWallpaper)
			if err != nil {
				fmt.Fprintf(os.Stderr, "let op: wallpaper %q kon niet geladen worden: %v\n", s.HuidigeWallpaper, err)
				continue
			}
			if err := apply(s.ID, s.Monitor, wp); err != nil {
				fmt.Fprintf(os.Stderr, "let op: %s kon niet ingesteld: %v\n", s.Monitor, err)
				continue
			}
			last[s.Key] = s.HuidigeWallpaper
		}
		saveState(statePath, last)
		<-ticker.C
	}
}

// loadState / saveState houden bij welke afbeelding er op welk scherm al staat,
// zodat de client nooit hetzelfde beeld twee keer op verschillende monitoren zet
// (meebeweegt door de server — de client sluit de oneffenheid alleen af).
func loadState(p string) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(p)
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}

func saveState(p string, m map[string]string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// werk: download + zet + geschiedenis (gebruikt door de GUI in de tray-app).
func werk(server string, screenID, monitor string, wp Record, gebrouwen bool) error {
	wpath, err := download(server, wp)
	if err != nil {
		return err
	}
	if err := setMonitorWallpaper(monitor, 0, wpath, "fill"); err != nil {
		return err
	}
	return geschiedenisCreate(server, screenID, wp.ID, "", "rotatie")
}
