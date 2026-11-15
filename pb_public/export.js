/*
 * Wallpaparr — exporteren en importeren van de bibliotheek.
 *
 * Een export bevat de wallpapers (titel, tags, afmetingen) én de instellingen:
 * de playlists per scherm en welke playlist aan welk scherm hangt. Je kiest per export
 * of álles meegaat of alleen wat je nu ziet (filter/selectie), en of de afbeeldingen zelf
 * meekomen (zip) of alleen de gegevens (json).
 *
 * Alles zit in één IIFE met een eigen naamruimte: de web-UI laadt app.js, screens.js en
 * export.js als losse <script>-tags in dezelfde globale scope, dus eigen namen voor
 * constanten en functies zijn verplicht.
 */
const WPExport = (function () {
  "use strict";

  const WPR = "/api/collections/wallpapers/records";
  const PLR = "/api/collections/playlists/records";
  const SR = "/api/collections/screens/records";
  const FORMAAT = 1;
  const STATUS = { ok: 200, gemaakt: 201, weg: 204 };

  /* ================= zip (alleen opslaan, geen compressie) ================= */

  let crcTabel = null;

  function maakCrcTabel() {
    const t = new Uint32Array(256);
    for (let n = 0; n < 256; n++) {
      let c = n;
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
      t[n] = c >>> 0;
    }
    return t;
  }

  function crc32(bytes) {
    if (!crcTabel) crcTabel = maakCrcTabel();
    let c = 0xffffffff;
    for (let i = 0; i < bytes.length; i++) c = crcTabel[(c ^ bytes[i]) & 0xff] ^ (c >>> 8);
    return (c ^ 0xffffffff) >>> 0;
  }

  function dosTijdDatum(d) {
    const tijd = (d.getHours() << 11) | (d.getMinutes() << 5) | (d.getSeconds() >> 1);
    const datum = ((d.getFullYear() - 1980) << 9) | ((d.getMonth() + 1) << 5) | d.getDate();
    return { tijd, datum };
  }

  // maakZip schrijft een zip-bestand zonder compressie (store). Zelf schrijven houdt de
  // web-UI dependency-vrij; de lezer (leesZip) accepteert ook gewone compressie.
  function maakZip(entries) {
    const enc = new TextEncoder();
    const nu = dosTijdDatum(new Date());
    const rijen = entries.map((e) => {
      const naam = enc.encode(e.naam);
      const data = e.bytes instanceof Uint8Array ? e.bytes : new Uint8Array(e.bytes);
      return { naam, data, crc: crc32(data) };
    });

    let omvang = 22; // end-of-central-directory
    for (const r of rijen) omvang += 30 + r.naam.length + r.data.length + 46 + r.naam.length;

    const uit = new Uint8Array(omvang);
    const dv = new DataView(uit.buffer);
    let p = 0;
    const offsets = [];

    for (const r of rijen) {
      offsets.push(p);
      dv.setUint32(p, 0x04034b50, true); p += 4; // lokaal header
      dv.setUint16(p, 20, true); p += 2; // versie nodig
      dv.setUint16(p, 0x0800, true); p += 2; // namen in UTF-8
      dv.setUint16(p, 0, true); p += 2; // geen compressie
      dv.setUint16(p, nu.tijd, true); p += 2;
      dv.setUint16(p, nu.datum, true); p += 2;
      dv.setUint32(p, r.crc, true); p += 4;
      dv.setUint32(p, r.data.length, true); p += 4;
      dv.setUint32(p, r.data.length, true); p += 4;
      dv.setUint16(p, r.naam.length, true); p += 2;
      dv.setUint16(p, 0, true); p += 2; // geen extra veld
      uit.set(r.naam, p); p += r.naam.length;
      uit.set(r.data, p); p += r.data.length;
    }

    const cdStart = p;
    for (let i = 0; i < rijen.length; i++) {
      const r = rijen[i];
      dv.setUint32(p, 0x02014b50, true); p += 4; // centraal header
      dv.setUint16(p, 20, true); p += 2;
      dv.setUint16(p, 20, true); p += 2;
      dv.setUint16(p, 0x0800, true); p += 2;
      dv.setUint16(p, 0, true); p += 2;
      dv.setUint16(p, nu.tijd, true); p += 2;
      dv.setUint16(p, nu.datum, true); p += 2;
      dv.setUint32(p, r.crc, true); p += 4;
      dv.setUint32(p, r.data.length, true); p += 4;
      dv.setUint32(p, r.data.length, true); p += 4;
      dv.setUint16(p, r.naam.length, true); p += 2;
      dv.setUint16(p, 0, true); p += 2; // extra
      dv.setUint16(p, 0, true); p += 2; // commentaar
      dv.setUint16(p, 0, true); p += 2; // schijfnummer
      dv.setUint16(p, 0, true); p += 2; // interne attributen
      dv.setUint32(p, 0, true); p += 4; // externe attributen
      dv.setUint32(p, offsets[i], true); p += 4;
      uit.set(r.naam, p); p += r.naam.length;
    }

    dv.setUint32(p, 0x06054b50, true); p += 4; // end of central directory
    dv.setUint16(p, 0, true); p += 2;
    dv.setUint16(p, 0, true); p += 2;
    dv.setUint16(p, rijen.length, true); p += 2;
    dv.setUint16(p, rijen.length, true); p += 2;
    dv.setUint32(p, p - 12 - cdStart, true); p += 4;
    dv.setUint32(p, cdStart, true); p += 4;
    dv.setUint16(p, 0, true); p += 2;

    return uit;
  }

  async function pufUit(bytes) {
    if (typeof DecompressionStream === "undefined") {
      throw new Error("Deze browser kan geen gecomprimeerde zip lezen");
    }
    const stroom = new Blob([bytes]).stream().pipeThrough(new DecompressionStream("deflate-raw"));
    return new Uint8Array(await new Response(stroom).arrayBuffer());
  }

  // leesZip haalt alle bestanden uit een zip (opslaan én deflate).
  async function leesZip(bytes) {
    const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    let eocd = -1;
    const ondergrens = Math.max(0, bytes.length - 22 - 65535);
    for (let i = bytes.length - 22; i >= ondergrens; i--) {
      if (dv.getUint32(i, true) === 0x06054b50) {
        eocd = i;
        break;
      }
    }
    if (eocd < 0) throw new Error("Dit is geen zip-bestand");

    const aantal = dv.getUint16(eocd + 10, true);
    let p = dv.getUint32(eocd + 16, true);
    const dec = new TextDecoder();
    const uit = new Map();

    for (let i = 0; i < aantal; i++) {
      if (dv.getUint32(p, true) !== 0x02014b50) throw new Error("Beschadigd zip-bestand");
      const methode = dv.getUint16(p + 10, true);
      const compGrootte = dv.getUint32(p + 20, true);
      const naamLen = dv.getUint16(p + 28, true);
      const extraLen = dv.getUint16(p + 30, true);
      const commentLen = dv.getUint16(p + 32, true);
      const offset = dv.getUint32(p + 42, true);
      const naam = dec.decode(bytes.subarray(p + 46, p + 46 + naamLen));
      p += 46 + naamLen + extraLen + commentLen;

      const lokNaamLen = dv.getUint16(offset + 26, true);
      const lokExtraLen = dv.getUint16(offset + 28, true);
      const begin = offset + 30 + lokNaamLen + lokExtraLen;
      const ruw = bytes.subarray(begin, begin + compGrootte);
      if (methode === 0) uit.set(naam, ruw.slice());
      else if (methode === 8) uit.set(naam, await pufUit(ruw));
      else throw new Error("Onbekende zip-compressie: " + methode);
    }
    return uit;
  }

  /* ================= manifest ================= */

  // veiligeNaam: bestandsnaam zonder pad en zonder rare tekens (voor in de zip).
  function veiligeNaam(naam) {
    const kaal = String(naam || "").split(/[\\/]/).pop() || "bestand";
    return kaal.replace(/[^\w.() -]/g, "_").slice(0, 120);
  }

  // bestandsSleutel: waar twee uploads van dezelfde afbeelding op neerkomen.
  // PocketBase hernoemt elk geüpload bestand naar naam_<random>.ext, dus dezelfde plaat
  // heet na een tweede upload anders. Voor "staat deze er al?" vergelijken we daarom de
  // genormaliseerde kern: kleine letters, alles behalve letters/cijfers gelijk, en de
  // toegevoegde random staart(en) eraf. Zo blijft "e2e-een.png" gelijk aan
  // "e2e_een_9j8ottjze5.png" én aan "e2e_een_9j8ottjze5_ab12cd3456.png".
  function bestandsSleutel(naam) {
    let kern = String(naam || "").split(/[\\/]/).pop().toLowerCase();
    const punt = kern.lastIndexOf(".");
    if (punt > 0) kern = kern.slice(0, punt);
    kern = kern.replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
    for (;;) {
      const schoner = kern.replace(/_[a-z0-9]{9,12}$/, "");
      if (schoner === kern) break;
      kern = schoner;
    }
    return kern;
  }

  // bouwManifest zet de wallpapers + instellingen om in één overdraagbaar object.
  // Verwijzingen naar wallpapers gaan via 'sleutel' (w0, w1, …) in plaats van record-id's,
  // zodat een import op een andere server werkt.
  function bouwManifest(items, playlists, schermen, opties) {
    const opt = opties || {};
    const wallpapers = items.map((r, i) => {
      const bestand = r.file || "";
      return {
        sleutel: "w" + i,
        bestand: bestand,
        zipPad: bestand ? "files/w" + i + "--" + veiligeNaam(bestand) : "",
        titel: r.title || "",
        tags: r.tags || "",
        width: r.width || 0,
        height: r.height || 0,
        bytes: r.bytes || 0,
        source: r.source || "",
        created: r.created || "",
      };
    });
    const idNaarSleutel = new Map();
    items.forEach((r, i) => idNaarSleutel.set(r.id, "w" + i));

    const pls = (playlists || []).map((pl) => ({
      naam: pl.naam || "",
      tags: pl.tags || "",
      ratios: pl.ratios || "",
      interval: pl.interval || 0,
      random: !!pl.random,
      actief: !!pl.actief,
      wallpapers: (pl.wallpapers || []).map((id) => idNaarSleutel.get(id)).filter(Boolean),
    }));

    const schermenUit = (schermen || []).map((s) => {
      const pl = (s.expand && s.expand.playlist) || null;
      const idx = pl ? (playlists || []).findIndex((x) => x.id === pl.id) : -1;
      return { key: s.key || "", alias: s.alias || "", playlist: idx };
    });

    return {
      app: "wallpaparr",
      formaat: FORMAAT,
      gemaakt: new Date().toISOString(),
      bereik: opt.bereik || "alles",
      metAfbeeldingen: !!opt.metAfbeeldingen,
      wallpapers: wallpapers,
      playlists: pls,
      schermen: schermenUit,
    };
  }

  // leesBundel leest een zip (met afbeeldingen) of een json (alleen gegevens).
  async function leesBundel(bytes, naam) {
    const isZip = bytes.length > 4 && bytes[0] === 0x50 && bytes[1] === 0x4b;
    if (isZip) {
      const bestanden = await leesZip(bytes);
      const m = bestanden.get("manifest.json");
      if (!m) throw new Error("manifest.json ontbreekt in het zip-bestand");
      const manifest = JSON.parse(new TextDecoder().decode(m));
      controleerManifest(manifest);
      return { manifest, bestanden };
    }
    const manifest = JSON.parse(new TextDecoder().decode(bytes));
    controleerManifest(manifest);
    return { manifest, bestanden: new Map() };
  }

  function controleerManifest(m) {
    if (!m || m.app !== "wallpaparr" || !Array.isArray(m.wallpapers)) {
      throw new Error("Dit bestand is geen Wallpaparr-export");
    }
    if (m.formaat > FORMAAT) {
      throw new Error("Dit exportbestand is nieuwer dan deze server (formaat " + m.formaat + ")");
    }
  }

  /* ================= exporteren ================= */

  async function apiJSON(url, opt) {
    const r = await fetch(url, opt);
    if (!r.ok) throw new Error("HTTP " + r.status);
    return r.status === 204 ? {} : r.json();
  }

  async function laadPlaylists() {
    const d = await apiJSON(`${PLR}?perPage=200&sort=created`);
    return d.items || [];
  }

  async function laadSchermen() {
    const d = await apiJSON(`${SR}?perPage=200&expand=playlist`);
    return d.items || [];
  }

  // huidigeItems: wat de gebruiker nu op het scherm heeft (aangevinkt, anders het filter).
  function huidigeItems() {
    const st = window.__wpState;
    if (st.selected && st.selected.size) return st.items.filter((r) => st.selected.has(r.id));
    return visibleItems();
  }

  function downloadBlob(blob, naam) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = naam;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10000);
  }

  function stempel() {
    const d = new Date();
    const twee = (n) => String(n).padStart(2, "0");
    return (
      d.getFullYear() + "-" + twee(d.getMonth() + 1) + "-" + twee(d.getDate()) +
      "-" + twee(d.getHours()) + twee(d.getMinutes())
    );
  }

  async function exporteer(bereik, metAfbeeldingen, opStatus) {
    const meld = opStatus || function () {};
    const items = bereik === "selectie" ? huidigeItems() : window.__wpState.items;
    if (!items.length) throw new Error("Niets te exporteren");

    meld("instellingen ophalen…");
    const [playlists, schermen] = await Promise.all([laadPlaylists(), laadSchermen()]);
    const manifest = bouwManifest(items, playlists, schermen, { bereik, metAfbeeldingen });
    const naam = "wallpaparr-export-" + stempel();

    if (!metAfbeeldingen) {
      downloadBlob(
        new Blob([JSON.stringify(manifest, null, 2)], { type: "application/json" }),
        naam + ".json"
      );
      return { aantal: items.length, metAfbeeldingen: false };
    }

    const enc = new TextEncoder();
    const entries = [{ naam: "manifest.json", bytes: enc.encode(JSON.stringify(manifest, null, 2)) }];
    for (let i = 0; i < items.length; i++) {
      const r = items[i];
      meld(`afbeelding ${i + 1}/${items.length}…`);
      const resp = await fetch(fileURL(r, r.file));
      if (!resp.ok) throw new Error("Kon " + r.file + " niet ophalen (HTTP " + resp.status + ")");
      entries.push({ naam: manifest.wallpapers[i].zipPad, bytes: new Uint8Array(await resp.arrayBuffer()) });
    }
    meld("zip maken…");
    downloadBlob(new Blob([maakZip(entries)], { type: "application/zip" }), naam + ".zip");
    return { aantal: items.length, metAfbeeldingen: true };
  }

  /* ================= importeren ================= */

  async function patchRecord(url, id, body) {
    const r = await fetch(`${url}/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return r.ok;
  }

  async function maakWallpaper(rec, bytes) {
    const blob = new Blob([bytes]);
    const fd = new FormData();
    fd.set("title", rec.titel || prettify(rec.bestand || "wallpaper"));
    fd.set("tags", tagList(rec.tags).join(","));
    fd.set("width", String(rec.width || 0));
    fd.set("height", String(rec.height || 0));
    fd.set("bytes", String(rec.bytes || bytes.length));
    if (rec.source) fd.set("source", String(rec.source));
    fd.set("file", blob, veiligeNaam(rec.bestand) || "wallpaper");
    const r = await fetch(WPR, { method: "POST", body: fd });
    if (!r.ok) return null;
    return r.json();
  }

  // importeer zet een bundel terug: eerst de wallpapers, dan de playlists en tot slot
  // welke playlist aan welk scherm hangt (schermen zelf meldt de client aan).
  async function importeer(bundel, opties, opStatus) {
    const meld = opStatus || function () {};
    const opt = opties || {};
    const manifest = bundel.manifest;
    const bestanden = bundel.bestanden || new Map();
    const uit = {
      toegevoegd: 0, bijgewerkt: 0, overgeslagen: 0, mislukt: 0,
      playlists: 0, schermen: 0, schermenOnbekend: 0, fouten: [],
    };

    meld("bestaande wallpapers ophalen…");
    let bestaand = [];
    try {
      bestaand = await fetchAll();
    } catch (e) {
      uit.fouten.push("bestaande wallpapers: " + e.message);
    }
    const perBestand = new Map();
    const perSleutelBestand = new Map();
    for (const r of bestaand) {
      if (!r.file) continue;
      if (!perBestand.has(r.file)) perBestand.set(r.file, r);
      const k = bestandsSleutel(r.file);
      if (k && !perSleutelBestand.has(k)) perSleutelBestand.set(k, r);
    }

    const sleutelNaarId = new Map();
    const rijen = manifest.wallpapers || [];
    for (let i = 0; i < rijen.length; i++) {
      const rec = rijen[i];
      const bytes = bestanden.get(rec.zipPad);
      const dubbel = rec.bestand
        ? perBestand.get(rec.bestand) || perSleutelBestand.get(bestandsSleutel(rec.bestand))
        : null;
      meld(`wallpaper ${i + 1}/${rijen.length}…`);
      try {
        if (dubbel && opt.dubbel !== "bijwerken") {
          sleutelNaarId.set(rec.sleutel, dubbel.id);
          uit.overgeslagen++;
          continue;
        }
        if (dubbel) {
          const ok = await patchRecord(WPR, dubbel.id, {
            title: rec.titel || dubbel.title,
            tags: tagList(rec.tags).join(","),
          });
          if (!ok) throw new Error("bijwerken mislukt");
          sleutelNaarId.set(rec.sleutel, dubbel.id);
          uit.bijgewerkt++;
          continue;
        }
        if (!bytes) {
          // alleen-gegevens-export en deze wallpaper staat hier niet: niets om te doen
          uit.overgeslagen++;
          continue;
        }
        const nieuw = await maakWallpaper(rec, bytes);
        if (!nieuw || !nieuw.id) throw new Error("aanmaken mislukt");
        perBestand.set(rec.bestand, nieuw);
        const nk = bestandsSleutel(rec.bestand);
        if (nk) perSleutelBestand.set(nk, nieuw);
        sleutelNaarId.set(rec.sleutel, nieuw.id);
        uit.toegevoegd++;
      } catch (e) {
        uit.mislukt++;
        uit.fouten.push((rec.bestand || rec.titel || rec.sleutel) + ": " + e.message);
      }
    }

    // playlists opnieuw aanmaken; een verwijzing naar een niet-geïmporteerde wallpaper
    // valt weg (dat kan bij een export van alleen de selectie)
    const nieuwePlaylists = [];
    const plRijen = manifest.playlists || [];
    for (let i = 0; i < plRijen.length; i++) {
      const pl = plRijen[i];
      meld(`playlist ${i + 1}/${plRijen.length}…`);
      const ids = (pl.wallpapers || []).map((s) => sleutelNaarId.get(s)).filter(Boolean);
      try {
        const r = await fetch(PLR, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            naam: pl.naam || "geïmporteerde playlist",
            tags: pl.tags || "",
            ratios: pl.ratios || "",
            interval: pl.interval || 0,
            random: !!pl.random,
            actief: !!pl.actief,
            wallpapers: ids,
          }),
        });
        if (!r.ok) throw new Error("HTTP " + r.status);
        nieuwePlaylists.push((await r.json()).id);
        uit.playlists++;
      } catch (e) {
        nieuwePlaylists.push("");
        uit.fouten.push("playlist " + (pl.naam || i) + ": " + e.message);
      }
    }

    // schermen koppelen op sleutel (device|monitor|index) — dat is op elke server gelijk
    const schermenRijen = manifest.schermen || [];
    if (schermenRijen.length) {
      meld("schermen koppelen…");
      let hier = [];
      try {
        hier = (await apiJSON(`${SR}?perPage=200&fields=id,key,alias`)).items || [];
      } catch (e) {
        uit.fouten.push("schermen ophalen: " + e.message);
      }
      const perSleutel = new Map();
      for (const s of hier) if (s.key) perSleutel.set(s.key, s.id);
      for (const s of schermenRijen) {
        const plId = s.playlist >= 0 ? nieuwePlaylists[s.playlist] : "";
        const schermId = s.key ? perSleutel.get(s.key) : null;
        if (!schermId) {
          uit.schermenOnbekend++;
          continue;
        }
        if (!plId) continue;
        if (await patchRecord(SR, schermId, { playlist: plId })) uit.schermen++;
      }
    }

    return uit;
  }

  /* ================= vensters ================= */

  function maakVenster(binnenkant) {
    const oud = document.querySelector(".venster");
    if (oud) oud.remove();
    const wrap = document.createElement("div");
    wrap.className = "venster";
    wrap.innerHTML = `<div class="venster-kaart">${binnenkant}</div>`;
    document.body.appendChild(wrap);
    return wrap;
  }

  function sluitVenster() {
    const oud = document.querySelector(".venster");
    if (oud) oud.remove();
  }

  function openExport() {
    const st = window.__wpState;
    const geselecteerd = st.selected.size;
    const gefilterd = visibleItems().length;
    const aantal = geselecteerd || gefilterd;
    const selectieLabel = geselecteerd
      ? `Alleen de aangevinkte selectie (${geselecteerd} van ${st.items.length})`
      : `Alleen wat het filter nu toont (${gefilterd} van ${st.items.length})`;

    const wrap = maakVenster(`
      <h2>Exporteren</h2>
      <p class="sub">Wallpapers, tags én de scherminstellingen (playlists) in één bestand.</p>

      <div class="keuzekop">Wat gaat er mee?</div>
      <div class="keuzes">
        <label class="keuze"><input type="radio" name="ex-bereik" value="alles" checked>
          <span>Alles<span class="sub">${st.items.length} wallpapers + alle instellingen</span></span></label>
        <label class="keuze"><input type="radio" name="ex-bereik" value="selectie" ${aantal ? "" : "disabled"}>
          <span>${escapeHtml(selectieLabel)}<span class="sub">${aantal ? "de instellingen gaan volledig mee" : "vink eerst wallpapers aan of zet een filter"}</span></span></label>
      </div>

      <div class="keuzekop">In welke vorm?</div>
      <div class="keuzes">
        <label class="keuze"><input type="radio" name="ex-vorm" value="zip" checked>
          <span>Zip met de afbeeldingen<span class="sub">compleet: hiermee zet je alles op een andere server terug</span></span></label>
        <label class="keuze"><input type="radio" name="ex-vorm" value="json">
          <span>Alleen gegevens (json)<span class="sub">titels, tags, playlists en schermkoppelingen — geen plaatjes</span></span></label>
      </div>

      <div class="venster-knoppen">
        <span class="sub" id="ex-status"></span>
        <button class="btn" id="ex-stop">Annuleren</button>
        <button class="btn primary" id="ex-gaan">Exporteren</button>
      </div>`);

    const lees = (naam) => wrap.querySelector(`input[name="${naam}"]:checked`).value;
    wrap.querySelector("#ex-stop").onclick = sluitVenster;
    wrap.querySelector("#ex-gaan").onclick = async () => {
      const knop = wrap.querySelector("#ex-gaan");
      const status = wrap.querySelector("#ex-status");
      knop.disabled = true;
      wrap.querySelector("#ex-stop").disabled = true;
      try {
        const uit = await exporteer(lees("ex-bereik"), lees("ex-vorm") === "zip", (s) => (status.textContent = s));
        sluitVenster();
        toast(`${uit.aantal} wallpapers geëxporteerd${uit.metAfbeeldingen ? " (zip)" : " (json)"}`, "ok");
      } catch (e) {
        status.textContent = e.message;
        knop.disabled = false;
        wrap.querySelector("#ex-stop").disabled = false;
      }
    };
  }

  async function openImport(file) {
    let bundel;
    try {
      const buf = new Uint8Array(await file.arrayBuffer());
      bundel = await leesBundel(buf, file.name);
    } catch (e) {
      toast("Kon dit bestand niet lezen: " + e.message, "err");
      return;
    }
    const m = bundel.manifest;
    const platen = (m.wallpapers || []).filter((w) => bundel.bestanden.has(w.zipPad)).length;
    const samenvatting = [
      `${(m.wallpapers || []).length} wallpapers${platen ? ` (${platen} met afbeelding)` : " (alleen gegevens)"}`,
      `${(m.playlists || []).length} playlists`,
      `${(m.schermen || []).length} schermkoppelingen`,
    ].join(" · ");

    const wrap = maakVenster(`
      <h2>Importeren</h2>
      <p class="sub"><b>${escapeHtml(file.name)}</b><br>${escapeHtml(samenvatting)}<br>
        geëxporteerd ${escapeHtml((m.gemaakt || "").slice(0, 16).replace("T", " "))} · bereik: ${escapeHtml(m.bereik || "?")}</p>

      <div class="keuzekop">Wallpapers die hier al staan (zelfde bestandsnaam):</div>
      <div class="keuzes">
        <label class="keuze"><input type="radio" name="im-dubbel" value="overslaan" checked>
          <span>Overslaan<span class="sub">bestaande titel en tags blijven zoals ze zijn</span></span></label>
        <label class="keuze"><input type="radio" name="im-dubbel" value="bijwerken">
          <span>Titel en tags bijwerken<span class="sub">de afbeelding zelf wordt niet vervangen</span></span></label>
      </div>
      <p class="sub">Playlists worden als nieuwe playlists toegevoegd en aan dezelfde schermen gehangen
        (schermen die de client nog niet gemeld heeft, worden overgeslagen).</p>

      <div class="venster-knoppen">
        <span class="sub" id="im-status"></span>
        <button class="btn" id="im-stop">Annuleren</button>
        <button class="btn primary" id="im-gaan">Importeren</button>
      </div>`);

    wrap.querySelector("#im-stop").onclick = sluitVenster;
    wrap.querySelector("#im-gaan").onclick = async () => {
      const knop = wrap.querySelector("#im-gaan");
      const status = wrap.querySelector("#im-status");
      const dubbel = wrap.querySelector('input[name="im-dubbel"]:checked').value;
      knop.disabled = true;
      wrap.querySelector("#im-stop").disabled = true;
      try {
        const uit = await importeer(bundel, { dubbel }, (s) => (status.textContent = s));
        sluitVenster();
        const delen = [
          `${uit.toegevoegd} toegevoegd`,
          uit.bijgewerkt ? `${uit.bijgewerkt} bijgewerkt` : "",
          uit.overgeslagen ? `${uit.overgeslagen} overgeslagen` : "",
          uit.mislukt ? `${uit.mislukt} mislukt` : "",
          `${uit.playlists} playlists`,
          `${uit.schermen} schermen gekoppeld`,
        ].filter(Boolean);
        toast(delen.join(" · "), uit.mislukt ? "err" : "ok");
        if (uit.fouten && uit.fouten.length) console.warn("importproblemen:", uit.fouten);
        load();
        if (typeof laadSchermen === "function") laadSchermen();
      } catch (e) {
        status.textContent = e.message;
        knop.disabled = false;
        wrap.querySelector("#im-stop").disabled = false;
      }
    };
  }

  /* ================= knoppen ================= */

  function koppel() {
    const ex = document.querySelector("#export");
    if (ex) ex.onclick = openExport;
    const im = document.querySelector("#import");
    const bestand = document.querySelector("#importfile");
    if (im && bestand) {
      im.onclick = () => bestand.click();
      bestand.onchange = (e) => {
        const f = e.target.files && e.target.files[0];
        e.target.value = "";
        if (f) openImport(f);
      };
    }
  }

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") sluitVenster();
  });

  koppel();

  return {
    maakZip, leesZip, crc32, veiligeNaam, bestandsSleutel, bouwManifest, leesBundel, controleerManifest,
    exporteer, importeer, huidigeItems, openExport, openImport,
  };
})();

window.__wpExport = WPExport;
