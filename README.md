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
| UI + API | `http://10.5.0.165/` (LAN) of `https://wallpaparr.xdio.nl/` |
| Interne poort | 8090 (host-poort 80) |
| LXC | CT 165 op Proxnux2 (2 GB) |
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
npm install && node tests/ui-test.mjs     # 27 checks
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
| LXC | CT 165 (Proxnux2, 2048 MB) — `10.5.0.165` |
| Dockhand | omgeving `wallpaparr` (id 37), git-stack `wallpaparr` (id 39) |
| Proxy | Zoraxy-regel `wallpaparr.xdio.nl` → `10.5.0.165:80` (geen Pocket-ID-gate: geen login) |
| DNS | AdGuard-rewrite `wallpaparr.xdio.nl` → `10.1.2.250` op beide instanties |
