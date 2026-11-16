// Autostart: de client kan zichzelf bij het aanmelden laten starten.
//
// Op Windows gebeurt dat met de registersleutel Run van de ingelogde gebruiker
// (HKCU\...\Run). Dat werkt zonder beheerdersrechten en zonder snelkoppeling in
// de opstartmap; één tekenreeks met het pad van de exe is genoeg.
//
// Dit bestand bevat alleen de pure stukken (de reg-opdracht en het lezen van de
// uitvoer) zodat ze op elk platform te testen zijn; het echte werk zit in
// autostart_windows.go met een lege variant voor de andere platformen.
package main

import "strings"

// autostartSleutel is de registersleutel met de opstartprogramma's van de
// gebruiker die is ingelogd.
const autostartSleutel = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// autostartNaam is de naam waaronder de client daarin staat.
const autostartNaam = "Wallpaparr"

// regAddArgs bouwt `reg add`: het pad tussen aanhalingstekens, zodat een pad met
// spaties (C:\Program Files\...) goed blijft werken.
func regAddArgs(pad string) []string {
	return []string{
		"add", autostartSleutel,
		"/v", autostartNaam,
		"/t", "REG_SZ",
		"/d", `"` + pad + `"`,
		"/f",
	}
}

// regDeleteArgs bouwt `reg delete` (de waarde weghalen).
func regDeleteArgs() []string {
	return []string{"delete", autostartSleutel, "/v", autostartNaam, "/f"}
}

// regQueryArgs bouwt `reg query` (lezen of de client er staat).
func regQueryArgs() []string {
	return []string{"query", autostartSleutel, "/v", autostartNaam}
}

// parseRegQuery haalt het pad uit de uitvoer van `reg query`:
//
//	HKEY_CURRENT_USER\Software\...\Run
//	    Wallpaparr    REG_SZ    "C:\Tools\wallpaparr-windows-amd64-tray.exe"
//
// Staat de waarde er niet (reg geeft dan een foutregel en exitcode 1), dan is
// het antwoord (false, "").
func parseRegQuery(uit string) (bool, string) {
	for _, regel := range strings.Split(uit, "\n") {
		velden := strings.Fields(strings.TrimSpace(regel))
		if len(velden) >= 3 && velden[0] == autostartNaam && strings.HasPrefix(velden[1], "REG_") {
			// het pad kan spaties bevatten: alles achter het type, zonder quotes
			pad := strings.Join(velden[2:], " ")
			return true, strings.Trim(pad, `"`)
		}
	}
	return false, ""
}
