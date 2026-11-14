# Wallpaparr

Centrale wallpaper-opslag + clients voor Windows en Linux/macOS.

- **Server** — PocketBase (REST API + donkere web-UI) om wallpapers centraal op te slaan,
  te taggen en te filteren. Draait in Docker op een eigen LXC.
- **Client** — één Go-binary per platform (`wallpaparr`) die wallpapers van de server haalt
  en als bureaubladachtergrond zet, inclusief **multi-monitor** (per monitor een andere
  wallpaper waar het besturingssysteem dat ondersteunt).
- **Geen login** — alle PocketBase-regels staan open (`""`), alles werkt anoniem op het LAN.

## Server

| Onderdeel | Waarde |
|---|---|
| UI + API | `https://wallpaparr.xdio.nl/` (via Zoraxy) of poort 80 op het LXC-IP |
| Interne poort | 8090 (host-poort 80) |
| LXC | eigen Proxmox-container (2 GB) |
| Data | named volume `wallpaparr_data` → `/pb/pb_data` |
| Collectie | `wallpapers` (title, file, tags, width, height, bytes, source) |
| Regels | list/view/create/update/delete = publiek (`""`) |

Uploaden kan via de web-UI (slepen of kiezen), tags zijn kommagescheiden en de client
filtert met `?filter=(tags~'natuur')`.

In de web-UI staan achter de zoekbalk vaste filters op **beeldverhouding** (1:1, 1:2, 2:3,
16:9, 21:9, 27:9), meerdere tegelijk te combineren met zoeken en tagfilters. Kleine
afwijkingen vallen in dezelfde groep (logaritmische vergelijking, 10% tolerantie), zodat
bijvoorbeeld 2912×1632 bij 16:9 hoort. De groepen zijn symmetrisch: 9:16 hoort bij 16:9 en
2:1 bij 1:2 — het is immers dezelfde verhouding, gedraaid. Wallpapers die nergens in de
buurt komen (bijv. 4:3) verschijnen onder een extra knop **overig**, die alleen verschijnt
als zulke afbeeldingen er echt zijn.

Zodra er een verhoudingfilter actief is, laat de tagbalk **alleen de tags zien die binnen
die selectie voorkomen** — je filtert dus nooit op een tag die in deze verhouding toch niet
voorkomt. Een tag die je al had aangevinkt blijft staan (met teller 0) zodat je hem kunt
blijven uitzetten.

## Client

```bash
# eenmalig: server instellen
wallpaparr config --server https://wallpaparr.xdio.nl

# één wallpaper zetten (nieuwste)
wallpaparr set

# willekeurige wallpaper met tag, per monitor een andere
wallpaparr set --tag natuur --random --per-monitor

# blijven wisselen (daemon)
wallpaparr rotate --every 30m --tag "abstract,ruimte" --per-monitor

# wat staat er op de server?
wallpaparr list --tag natuur
```

Configuratie staat in `%APPDATA%\wallpaparr\config.json` (Windows) of
`~/.config/wallpaparr/config.json` (Unix); de server is ook te zetten via de
omgevingsvariabele `WALLPAPARR_SERVER`.

### Windows tray-app (instellingen + systeembalk)

Er zijn twee Windows-binaries:

| Bestand | Waarvoor |
|---|---|
| `wallpaparr-windows-amd64.exe` | console-versie: alle CLI-opdrachten (`list`, `set`, `rotate`, …) én `gui` |
| `wallpaparr-windows-amd64-tray.exe` | **geen consolevenster** — dubbelklik of zet hem in de opstartmap voor de tray-app |

Dubbelklik (of `wallpaparr gui`): je krijgt een instellingenvenster met server-URL, tags,
interval (minuten), weergavemodus, per monitor, willekeurige keuze en *bij opstarten alleen in
de systeembalk*, plus de knoppen **Opslaan en toepassen**, **Nu wisselen** en
**Verbinding testen**. Er is geen extra runtime of framework nodig.

De tray-variant heeft geen console, dus fouten gaan naar
`%LOCALAPPDATA%\wallpaparr\gui.log` (met tijdstempel: start, elke wissel, fouten en panics).

- **Kruisje** = het venster verdwijnt naar de **systeembalk**; de client blijft draaien en
  wisselt volgens het interval.
- **Rechtsklik op het tray-icoon**: Instellingen…, Nu wisselen, Volgende wallpaper,
  Verbinding testen, Per monitor aan/uit, Interval (15/30/60/180/360/1440 minuten of uit)
  en **Afsluiten**.
- Twee keer starten levert geen tweede icoon op (single-instance).
- Wisselen herhaalt nooit twee keer achter elkaar hetzelfde beeld (per monitor bijgehouden).
- Als de taakbalk (Explorer) opnieuw opstart, meldt de client het icoon automatisch opnieuw aan.
- **Zie je geen icoon?** Windows 11 zet nieuwe tray-iconen standaard in de verborgen-iconenoverloop:
  klik op het pijltje **^** naast de klok en sleep het Wallpaparr-icoon naar de taakbalk (of zet het
  aan via Instellingen → Personalisatie → Taakbalk → Systeembalkpictogrammen). Bij de eerste start
  laat de client ook eenmalig een melding zien.
- De GUI gebruikt dezelfde `config.json` als de CLI-opdrachten.

Op **Unix blijft het de CLI**: een tray-icoon vraagt daar GTK/AppIndicator (extra
afhankelijkheden en CGO), en dat weegt niet op tegen `wallpaparr set/rotate`.

### Per-monitor profielen (tags + verhouding)

Per monitor stel je in welke wallpapers meegenomen mogen worden:

| Veld | Betekenis |
|---|---|
| **Tags** | kommagescheiden; meerdere tags = allemaal moeten ze op de afbeelding staan. Leeg = **alle tags** |
| **Verhoudingen** | 1:1, 1:2, 2:3, 16:9, 21:9, 27:9 en *overig*. Niets aangevinkt = **alle verhoudingen** |

Een wallpaper die niet voldoet wordt voor díé monitor overgeslagen — handig voor een ultrawide
die alleen 21:9 of 27:9 wil terwijl de rest van je schermen alles mag pakken. Voorbeeld uit de
praktijk: op de live bibliotheek van 345 wallpapers verdeelt de client ze als 231× 21:9,
75× 16:9 en 39× 27:9 (3808×1280 valt binnen de tolerantie in de 27:9-groep).

Instellen kan in de GUI (Windows): elke monitor staat op **een eigen regel** met daarachter
een tagveld en de verhouding-vakjes. Invullen en op **Opslaan en toepassen** klikken is
genoeg; wisselen tussen monitoren kan dus niet meer misgaan. Het aantal rijen is
`maxMonitorRijen` (standaard 6); zijn er meer monitoren, dan gebruikt de rest alle tags en
verhoudingen en staat dat in het logbestand. Met **Profielen controleren** (knop of
tray-menu-item) zie je per monitor hoeveel wallpapers er met die instellingen in aanmerking
komen. In de JSON-config staat hetzelfde onder `"monitors"`:

```json
{
  "server": "https://wallpaparr.xdio.nl",
  "per_monitor": true,
  "monitors": [
    { "monitor": "AOC2702", "tags": ["natuur"], "ratios": ["16:9", "21:9"] },
    { "monitor": "DELD128", "ratios": ["27:9"] }
  ]
}
```

De monitornaam wordt eerst exact gematcht en anders op deelnaam — het COM-pad verandert
namelijk als je een monitor opnieuw aansluit. Op de CLI geeft `wallpaparr profiles` dezelfde
samenvatting als de knop in de GUI, en `wallpaparr list` toont per wallpaper in welke
verhoudinggroep hij valt.

De verhoudinggroepen zijn gelijk aan die in de web-UI: 10% tolerantie en **niet symmetrisch**,
dus een gekantelde 9:16-tegel hoort bewust niet bij 16:9.

### Multi-monitor

| Platform | Per monitor andere wallpaper | Methode |
|---|---|---|
| Windows 8+ | ja | `IDesktopWallpaper` COM |
| Linux (Wayland) | ja | `swww` |
| Linux (X11) | ja | `feh --xinerama-index` |
| Linux/macOS | nee (zelfde op alle) | gsettings/plasma/xfconf/feh/osascript |

Ontbreken de tools, dan valt de client netjes terug op één wallpaper voor alle monitoren
(hij meldt dat op de console).

## Testen

De web-UI heeft een headless interactietest (jsdom, geen server nodig) die de logica
controleert die een syntaxcheck niet vangt: selectiemodus, klikken op de afbeelding,
fullscreen-gedrag, tagverwijdering en de bulkacties.

```bash
npm install && node tests/ui-test.mjs     # 27 checks (web-UI)
node tests/ratio-test.mjs                 # 21 checks (verhoudingfilters)
cd client && go test ./...                # keuze-logica, filters, server-URL
```

Draait ook mee in CI als job `ui-test`.

## Bouwen

Alles gaat via GitHub Actions (push naar `main`):

- server-image → `ghcr.io/s4ndyp/wallpaparr:latest`
- clients → workflow-artifacts en, bij een tag `v*`, als release-assets
  (`wallpaparr-windows-amd64.exe`, `wallpaparr-linux-amd64`, …)

Lokaal bouwen: `cd client && go build -o wallpaparr .`

Downloadbare binaries staan bij de **Releases** van deze repo.

## Uitrol

De stack staat in `github.com/s4ndyp/compose` → `wallpaparr/compose.yaml` en draait als
Dockhand-git-stack op een eigen LXC:

| | |
|---|---|
| LXC | eigen Proxmox-container, 2048 MB |
| Dockhand | omgeving `wallpaparr` + git-stack `wallpaparr` |
| Proxy | Zoraxy-regel `wallpaparr.xdio.nl` → de stack (geen Pocket-ID-gate: geen login) |
| DNS | AdGuard-rewrite `wallpaparr.xdio.nl` → de Zoraxy-proxy, op beide instanties |