/// <reference path="../pb_data/types.d.ts" />
/*
 * Vrije schijfruimte van de opslag waar de wallpapers staan
 * (GET /api/wallpaparr/disk). De web-UI toont dit als balkje op de hoofdpagina.
 *
 * De logica staat bewust volledig ín de handler: PocketBase draait een hook in een
 * eigen JS-runtime, waar functies van bestandsniveau niet zichtbaar zijn.
 *
 * Waarom `df -kP` zonder pad: met een pad erbij kiest busybox (in de container) de
 * verkeerde regel uit /proc/mounts — voor /pb/pb_data kwam daar "/etc/hostname" uit
 * (Docker bindt dat bestand). Daarom vragen we de hele lijst op en zoeken zelf de
 * langste mount die bij het datapad hoort, met "/" als terugval.
 *
 * df -kP levert blokken van 1024 bytes (POSIX-uitvoer; busybox in de container én
 * coreutils kennen deze vlag). Let op: $app.db().newQuery().one() laat de JSVM
 * panieken (geen catchbare fout) — daarom tellen we de wallpapers met findAllRecords.
 */
routerAdd("GET", "/api/wallpaparr/disk", (c) => {
  const pad = $app.dataDir();
  const uit = {
    pad: pad,
    beschikbaar: false,
    bestandssysteem: "",
    mount: "",
    totaal: 0,
    gebruikt: 0,
    vrij: 0,
    procent: 0,
    wallpapers: 0,
    wallpapersBytes: 0,
  };

  // df -kP: de hele lijst, want de mount die erbij hoort zoeken we zelf.
  function leesSchijven(tekst) {
    const rijen = [];
    for (const regel of String(tekst).trim().split("\n")) {
      const delen = regel.trim().split(/\s+/);
      if (delen.length < 6) continue;
      const blokken = parseInt(delen[delen.length - 5], 10);
      const gebruikt = parseInt(delen[delen.length - 4], 10);
      const vrij = parseInt(delen[delen.length - 3], 10);
      if (isNaN(blokken) || isNaN(gebruikt) || isNaN(vrij) || blokken <= 0) continue;
      rijen.push({
        bestandssysteem: delen.slice(0, delen.length - 5).join(" "),
        mount: delen[delen.length - 1],
        blokken: blokken,
        gebruikt: gebruikt,
        vrij: vrij,
      });
    }
    return rijen;
  }

  try {
    const rijen = leesSchijven(toString($os.cmd("df", "-kP").output()));
    if (rijen.length) {
      // de langste mount die een voorstuk van het datapad is; anders de root; anders de eerste
      let keuze = null;
      for (const r of rijen) {
        if (!r.mount) continue;
        if (pad === r.mount || pad.indexOf(r.mount + "/") === 0) {
          if (!keuze || r.mount.length > keuze.mount.length) keuze = r;
        }
      }
      if (!keuze) keuze = rijen.find((r) => r.mount === "/") || rijen[0];
      uit.beschikbaar = true;
      uit.bestandssysteem = keuze.bestandssysteem;
      uit.mount = keuze.mount;
      uit.totaal = keuze.blokken * 1024;
      uit.gebruikt = keuze.gebruikt * 1024;
      uit.vrij = keuze.vrij * 1024;
      uit.procent = Math.round((uit.gebruikt / uit.totaal) * 1000) / 10;
    }
  } catch (e) {
    console.error("schijfruimte opvragen mislukt: " + e);
  }

  // Hoeveel nemen de wallpapers zelf in? Dat verklaart het grootste deel van de balk.
  try {
    const rijen = $app.findAllRecords("wallpapers");
    uit.wallpapers = rijen.length;
    for (const r of rijen) uit.wallpapersBytes += r.getInt("bytes");
  } catch (e) {
    // geen wallpapers-collectie: dan alleen de schijf tonen
  }

  return c.json(200, uit);
});
