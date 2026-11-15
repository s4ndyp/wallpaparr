/// <reference path="../pb_data/types.d.ts" />
/*
 * Vrije schijfruimte van de opslag waar de wallpapers staan
 * (GET /api/wallpaparr/disk). De web-UI toont dit als balkje op de hoofdpagina.
 *
 * De logica staat bewust volledig ín de handler: PocketBase draait een hook in een
 * eigen JS-runtime, waar functies van bestandsniveau niet zichtbaar zijn.
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

  try {
    const tekst = toString($os.cmd("df", "-kP", pad).output());
    const regels = tekst.trim().split("\n");
    if (regels.length >= 2) {
      const delen = regels[regels.length - 1].trim().split(/\s+/);
      if (delen.length >= 6) {
        const blokken = parseInt(delen[delen.length - 5], 10);
        const gebruikt = parseInt(delen[delen.length - 4], 10);
        const vrij = parseInt(delen[delen.length - 3], 10);
        if (!isNaN(blokken) && !isNaN(vrij) && blokken > 0) {
          uit.beschikbaar = true;
          uit.bestandssysteem = delen.slice(0, delen.length - 5).join(" ");
          uit.mount = delen[delen.length - 1];
          uit.totaal = blokken * 1024;
          uit.gebruikt = gebruikt * 1024;
          uit.vrij = vrij * 1024;
          uit.procent = Math.round((uit.gebruikt / uit.totaal) * 1000) / 10;
        }
      }
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
