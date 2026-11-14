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
| UI + API | `http://<lxc-ip>/` (host-poort 80) |
| Interne poort | 8090 |
| Data | named volume `wallpaparr_data` → `/pb/pb_data` |
| Collectie | `wallpapers` (title, file, tags, width, height, bytes, source) |
| Regels | list/view/create/update/delete = publiek (`""`) |

Uploaden kan via de web-UI (slepen of kiezen), tags zijn kommagescheiden en de client
filtert met `?filter=(tags~'natuur')`.

## Client

```bash
# eenmalig: server instellen
wallpaparr config --server http://10.5.0.171

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

## Bouwen

Alles gaat via GitHub Actions (push naar `main`):

- server-image → `ghcr.io/s4ndyp/wallpaparr:latest`
- clients → workflow-artifacts en, bij een tag `v*`, als release-assets
  (`wallpaparr-windows-amd64.exe`, `wallpaparr-linux-amd64`, …)

Lokaal bouwen: `cd client && go build -o wallpaparr .`

## Uitrol

De stack staat in `github.com/s4ndyp/compose` → `wallpaparr/compose.yaml`
en wordt via Dockhand op een eigen LXC gedeployed.
