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

### Schijfruimte

Bovenaan de hoofdpagina staat een balkje met de **vrije ruimte van de opslag** waar de
wallpapers staan (het volume achter `/pb/pb_data`): `12,3 GB vrij van 46,9 GB · 73% vol ·
wallpapers 1,2 GB`. De balk wordt geel boven 75% en rood boven 90%; de tooltip noemt het
bestandssysteem en het mountpunt. De server levert dit via `GET /api/wallpaparr/disk`
(`pb_hooks/disk.pb.js`, `df -kP` in de container).

### Exporteren en importeren

Met **Exporteren** zet je de bibliotheek in één bestand: de wallpapers, hun **tags** én de
**instellingen** (playlists per scherm en welke playlist aan welk scherm hangt).

| Keuze | Wat je krijgt |
|---|---|
| Alles | alle wallpapers + alle instellingen |
| Alleen huidige selectie | wat je aangevinkt hebt, of (zonder aanvinkjes) wat het filter nu toont |
| Zip met afbeeldingen | compleet bestand — hiermee zet je alles op een andere server terug |
| Alleen gegevens (json) | titels, tags, playlists en schermkoppelingen, zonder plaatjes |

De zip wordt in de browser zelf geschreven (geen compressie, `files/w0--naam.jpg` +
`manifest.json`) en is met gewone gereedschappen te lezen; bij het lezen worden ook
gecomprimeerde zip-bestanden van andere programma's ondersteund.

**Importeren** accepteert beide vormen. Wallpapers waarvan de bestandsnaam hier al bestaat
kun je **overslaan** of **bijwerken** (titel + tags; de afbeelding zelf blijft staan). Bij
een gegevens-only-bestand worden alleen bestaande wallpapers bijgewerkt — er wordt niets
geüpload. Playlists komen er als **nieuwe** playlists bij en worden op sleutel
(`device|monitor|index`) weer aan dezelfde schermen gehangen; schermen die de client nog
niet gemeld heeft, worden overgeslagen.

## Client

De **server is de baas**: rotatie, tags, verhoudingen, intervallen en keuzes stel je in de
web-UI in. De client meldt alleen zijn monitoren aan, pollt de server (elke paar seconden) en
zet wat de server opdraagt. De client roteert dus niet zelf en bewaart geen profielen meer.

```bash
# eenmalig: server instellen
wallpaparr config --server https://wallpaparr.xdio.nl

# je monitoren tonen
wallpaparr monitors

# wat zet de server NU op jouw schermen?
wallpaparr status

# blijven luisteren (polling; de server bepaalt wanneer er gewisseld wordt)
wallpaparr luister

# afwijkend interval of een subset van de monitoren
wallpaparr luister --poll 5 --monitors "AOC2702,AOC2702"

# één afbeelding vastzetten (pin; in de web-UI zet je hem terug op rotatie)
wallpaparr set --id <record-id>

# automatisch starten bij het aanmelden (Windows; in de web-UI-kant niets te doen)
wallpaparr autostart aan          # of: uit | status

# versie
wallpaparr version
```

Configuratie (server-URL, poll-interval, weergave, verborgen starten, automatisch starten)
staat in `%APPDATA%\wallpaparr\config.json` (Windows) of `~/.config/wallpaparr/config.json`
(Unix); de server is ook te zetten via de omgevingsvariabele `WALLPAPARR_SERVER`.

### Windows tray-app

Er zijn twee Windows-binaries:

| Bestand | Waarvoor |
|---|---|
| `wallpaparr-windows-amd64.exe` | console-versie: alle CLI-opdrachten én de GUI |
| `wallpaparr-windows-amd64-tray.exe` | **geen consolevenster** — dubbelklik; automatisch starten zet je aan in het venster |

Het venster toont bovenaan **Wallpaparr v<versie>** (ook in de titelbalk en de tray-tip), de
server-URL, de weergavemodus, het poll-interval, *bij opstarten alleen in de systeembalk* en
*automatisch starten wanneer ik me aanmeld bij Windows*. Daaronder staat **per monitor wat de
server er nu op zet en sinds wanneer**. Knoppen: **Opslaan**, **Nu synchroniseren**,
**Verbinding testen** en **Open web-UI**.

- **Kruisje** = naar de **systeembalk**; de client blijft de server volgen.
- **Rechtsklik op het tray-icoon**: versie, Instellingen…, Nu synchroniseren,
  Verbinding testen, Open web-UI en **Afsluiten**.
- Fouten gaan naar `%LOCALAPPDATA%\wallpaparr\gui.log` (start, elke toepassing, fouten, panics).
- Twee keer starten levert geen tweede icoon op (single-instance).
- Dezelfde afbeelding komt nooit op twee monitoren: de server kiest per scherm en de client
  onthoudt lokaal wat er al staat (`state.json`).
- Als de taakbalk (Explorer) opnieuw opstart, meldt de client het icoon automatisch opnieuw aan.
- **Zie je geen icoon?** Windows 11 zet nieuwe tray-iconen standaard in de verborgen-iconenoverloop:
  klik op het pijltje **^** naast de klok en sleep het Wallpaparr-icoon naar de taakbalk (of zet het
  aan via Instellingen → Personalisatie → Taakbalk → Systeembalkpictogrammen).

#### Automatisch starten bij het aanmelden

Het vinkje **Automatisch starten wanneer ik me aanmeld bij Windows** in het venster (of
`wallpaparr autostart aan`) zet de client in de registersleutel
`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`. Dat werkt zonder beheerdersrechten en
zonder snelkoppeling in de opstartmap: er komt één tekenreeks met het pad van de draaiende
exe in, tussen aanhalingstekens zodat een pad met spaties goed blijft werken.

- Het vinkje toont altijd de **echte registerstand**, niet wat er in `config.json` staat — je
  ziet dus meteen of het goed staat.
- Start de app na het aanmelden meteen onzichtbaar? Zet dan ook *bij opstarten alleen in de
  systeembalk* aan.
- Verplaats of hernoem je de exe, vink dan opnieuw aan (of uit en weer aan) zodat het pad in
  het register wordt bijgewerkt.
- `wallpaparr autostart status` laat zien of (en met welk pad) de client in het register staat.

De GUI gebruikt dezelfde `config.json` als de CLI-opdrachten.

Op **Unix blijft het de CLI** (`wallpaparr luister`): een tray-icoon vraagt daar
GTK/AppIndicator (extra afhankelijkheden en CGO), en dat weegt niet op.

### Waar stel je wat in?

| | Waar |
|---|---|
| Rotatie aan/uit, interval, willekeurig of op volgorde | web-UI (per scherm) |
| Welke tags en verhoudingen per scherm meedoen | web-UI (playlist per scherm) |
| Nu een bepaalde afbeelding op een scherm (pin) | web-UI, of `wallpaparr set --id` |
| Wat staat waar / geschiedenis | web-UI |
| Server-URL, poll-interval, weergavemodus, verborgen starten | client (venster of `config.json`) |
| Automatisch starten bij aanmelden (Windows) | client (venster, `wallpaparr autostart` of `config.json`) |

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
node tests/ratio-test.mjs                 # 27 checks (verhoudingfilters)
node tests/taghelp-test.mjs               # 18 checks (tagvoorstellen)
node tests/screens-test.mjs               # 41 checks (schermenpaneel)
node tests/export-test.mjs                # 46 checks (schijfbalk, zip, export/import)
```

Daarnaast is er een proef tegen een **echte** PocketBase in plaats van een mock (zet zelf
drie wallpapers, een playlist en een scherm neer, exporteert, gooit weg, zet terug):

```bash
cd ~/dev/pb-test && ./pocketbase serve --http=127.0.0.1:8099 --https= --dir=./pb_data &
node tests/export-harness.mjs
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