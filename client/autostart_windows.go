//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// createNoWindow houdt reg.exe onzichtbaar. Zonder deze vlag opent Windows voor
// het hulp-programma een consolevenster, en dan flitst bij elke wijziging (en bij
// het opstarten van de tray-app) een zwart venster over het scherm.
const createNoWindow = 0x08000000

func regCmd(args ...string) ([]byte, error) {
	c := exec.Command("reg", args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return c.CombinedOutput()
}

// autostartPad geeft het volledige pad van de draaiende exe: dat is wat er in de
// Run-sleutel moet staan, zodat de client met dezelfde versie opstart.
func autostartPad() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

// autostartIngesteld vertelt of de client in de Run-sleutel staat (en met welk pad).
func autostartIngesteld() (bool, string) {
	uit, err := regCmd(regQueryArgs()...)
	if err != nil {
		return false, "" // niet gevonden: reg eindigt met exitcode 1
	}
	return parseRegQuery(string(uit))
}

// zetAutostart zet de client in de Run-sleutel. pad leeg = de draaiende exe.
func zetAutostart(pad string) error {
	if pad == "" {
		p, err := autostartPad()
		if err != nil {
			return err
		}
		pad = p
	}
	if uit, err := regCmd(regAddArgs(pad)...); err != nil {
		return fmt.Errorf("reg add: %v (%s)", err, strings.TrimSpace(string(uit)))
	}
	return nil
}

// verwijderAutostart haalt de client uit de Run-sleutel.
func verwijderAutostart() error {
	if uit, err := regCmd(regDeleteArgs()...); err != nil {
		return fmt.Errorf("reg delete: %v (%s)", err, strings.TrimSpace(string(uit)))
	}
	return nil
}

// pasAutostartAan brengt het register in lijn met de keuze in het venster en
// geeft één regel terug voor het statusvak. Staat de client er al met hetzelfde
// pad, dan gebeurt er niets (geen onnodig register-schrijven).
func pasAutostartAan(wil bool) string {
	aan, bestaand := autostartIngesteld()
	if wil {
		nieuw, err := autostartPad()
		if err != nil {
			return "automatisch starten mislukt: " + err.Error()
		}
		if aan && strings.EqualFold(bestaand, nieuw) {
			return "automatisch starten staat al aan"
		}
		if err := zetAutostart(nieuw); err != nil {
			return "automatisch starten mislukt: " + err.Error()
		}
		return "automatisch starten aangezet: " + nieuw
	}
	if !aan {
		return "automatisch starten staat uit"
	}
	if err := verwijderAutostart(); err != nil {
		return "automatisch starten uitzetten mislukt: " + err.Error()
	}
	return "automatisch starten uitgezet"
}
