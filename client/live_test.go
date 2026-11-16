package main

import (
	"os"
	"testing"
)

// TestLiveSchermOpzoeken is een proef tegen de ECHTE server (geen mock). Zet
// WALLPAPARR_LIVE=http://10.5.0.165 (en WALLPAPARR_DEVICE=<machinenaam>) om hem te
// draaien; in CI wordt hij overgeslagen. Dit ving de filterbug die mocks misten:
// een schermsleutel met backslashes werd nooit gevonden, waarna de client het
// scherm elke poll opnieuw POSTte (HTTP 400 value must be unique).
func TestLiveSchermOpzoeken(t *testing.T) {
	server := os.Getenv("WALLPAPARR_LIVE")
	if server == "" {
		t.Skip("zet WALLPAPARR_LIVE=<server-URL> om deze proef te draaien")
	}
	device := os.Getenv("WALLPAPARR_DEVICE")
	if device == "" {
		device = hostname()
	}
	screens, err := screensForDevice(server, device)
	if err != nil {
		t.Fatalf("schermen van %q ophalen: %v", device, err)
	}
	if len(screens) == 0 {
		t.Skipf("server kent nog geen schermen voor %q", device)
	}
	for _, s := range screens {
		gevonden, err := screenByKey(server, s.Key)
		if err != nil {
			t.Errorf("opzoeken van %q: %v", s.Key, err)
			continue
		}
		if gevonden == nil {
			t.Errorf("scherm %q niet gevonden op zijn eigen sleutel — de client zou het opnieuw POSTen (400)", s.Key)
			continue
		}
		if gevonden.ID != s.ID {
			t.Errorf("scherm %q: kreeg id %s, wil %s", s.Key, gevonden.ID, s.ID)
		}
	}
	t.Logf("%d schermen van %q gevonden op hun sleutel", len(screens), device)
}
