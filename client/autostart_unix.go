//go:build !windows

package main

import "errors"

// Op Unix (Linux/macOS) regelt de gebruiker het starten bij het aanmelden zelf
// (systemd-user, .xprofile, launchd); de client blijft daar de CLI.
var errGeenAutostart = errors.New("automatisch starten bij aanmelden wordt alleen op Windows ondersteund")

func autostartIngesteld() (bool, string) { return false, "" }

func zetAutostart(string) error { return errGeenAutostart }

func verwijderAutostart() error { return errGeenAutostart }
