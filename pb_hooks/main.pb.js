/// <reference path="../pb_data/types.d.ts" />
/*
 * Server-side rotatie: één keer per minuut kijkt deze cron welke schermen aan de
 * beurt zijn en zet een nieuwe wallpaper op het scherm-record. De client pollt dat
 * record en zet het beeld — de server is dus de baas, de client alleen speler.
 *
 * Handmatige pins (bron = "handmatig") worden met rust gelaten tot je in de web-UI
 * weer "rotatie" kiest. De staat staat volledig in de records, dus een herstart of
 * redeploy verliest niets.
 */

// PocketBase's cron werkt in UTC; de intervallen rekenen met tijdstempels, dus dat
// maakt voor het wisselen niet uit.
cronAdd("wallpaper-rotatie", "* * * * *", () => {
  try {
    wisselSchermen();
  } catch (e) {
    console.error("wallpaper-rotatie mislukt: " + e);
  }
});

function wisselSchermen() {
  const nu = new Date();
  const schermen = $app.findRecordsByFilter("screens", "", "created", 1000, 0);
  for (const scherm of schermen) {
    try {
      wisselScherm(scherm, nu);
    } catch (e) {
      console.error("scherm " + scherm.getString("key") + " overgeslagen: " + e);
    }
  }
}

function wisselScherm(scherm, nu) {
  if (scherm.getString("bron") === "handmatig") return; // pin: niets doen

  const playlistId = scherm.getString("playlist");
  if (!playlistId) return;

  let playlist;
  try {
    playlist = $app.findRecordById("playlists", playlistId);
  } catch (e) {
    return; // playlist weg
  }
  if (!playlist.getBool("actief")) return;

  const interval = playlist.getInt("interval");
  if (interval <= 0) return;

  const laatste = alsTijd(scherm.getString("gewijzigd"));
  if (laatste && (nu.getTime() - laatste.getTime()) / 60000 < interval) return;

  const huidig = scherm.getString("huidige_wallpaper");
  const keuze = kiesWallpaper(playlist, huidig);
  if (!keuze) return;

  scherm.set("huidige_wallpaper", keuze.id);
  scherm.set("bron", "rotatie");
  scherm.set("gewijzigd", nu);
  $app.save(scherm);
}

// kiesWallpaper: handmatige selectie gaat boven de filters; anders filteren we op
// tags (server-side) en beeldverhouding (zelfde groepen als de web-UI).
function kiesWallpaper(playlist, huidigId) {
  const handmatig = alsLijst(playlist.get("wallpapers"));
  let kandidaten = [];

  if (handmatig.length > 0) {
    for (const id of handmatig) {
      if (id === huidigId && handmatig.length > 1) continue;
      kandidaten.push(String(id));
    }
    if (kandidaten.length === 0) return null;
    return { id: kandidaten[Math.floor(Math.random() * kandidaten.length)] };
  }

  const tags = splitLijst(playlist.getString("tags"));
  const ratios = splitLijst(playlist.getString("ratios"));

  let filter = "";
  for (const t of tags) {
    const veilig = String(t).replace(/'/g, "\\'");
    filter += (filter ? "&&" : "") + "tags~'" + veilig + "'";
  }

  const rijen = $app.findRecordsByFilter("wallpapers", filter, "-created", 2000, 0);
  for (const r of rijen) {
    if (rijen.length > 1 && r.id === huidigId) continue;
    if (ratios.length > 0 && !bevatGroep(ratios, ratioBucket(r.getInt("width"), r.getInt("height")))) {
      continue;
    }
    kandidaten.push(r);
  }
  if (kandidaten.length === 0) return null;
  return kandidaten[Math.floor(Math.random() * kandidaten.length)];
}

// ratioBucket volgt de web-UI: 10% tolerantie, niet symmetrisch.
function ratioBucket(w, h) {
  if (!w || !h) return "overig";
  const x = w / h;
  const groepen = [
    ["1:1", 1],
    ["1:2", 0.5],
    ["2:3", 2 / 3],
    ["16:9", 16 / 9],
    ["21:9", 21 / 9],
    ["27:9", 27 / 9],
  ];
  let best = "";
  let bestD = 999;
  for (const [naam, v] of groepen) {
    const d = Math.abs(Math.log(x / v));
    if (d < bestD) {
      bestD = d;
      best = naam;
    }
  }
  return bestD <= 0.1 ? best : "overig";
}

function bevatGroep(lijst, groep) {
  for (const g of lijst) {
    if (String(g).trim() === groep) return true;
  }
  return false;
}

function splitLijst(s) {
  if (!s) return [];
  const uit = [];
  for (const deel of String(s).split(",")) {
    const t = deel.trim();
    if (t) uit.push(t);
  }
  return uit;
}

function alsLijst(v) {
  if (!v) return [];
  if (Array.isArray(v)) return v;
  return [v];
}

function alsTijd(s) {
  if (!s) return null;
  let d = new Date(s);
  if (isNaN(d.getTime())) d = new Date(String(s).replace(" ", "T"));
  return isNaN(d.getTime()) ? null : d;
}
