/* Wallpaparr — schermenpaneel: per monitor een playlist, een pin en de geschiedenis.
   De server is de baas: hier stel je in wat waar moet komen, de client voert uit. */

const SCH = "/api/collections/screens/records";
const PLS = "/api/collections/playlists/records";
const HIS = "/api/collections/geschiedenis/records";

let schermen = [];

/* ---------- helpers ---------- */

async function apiJSON(url, opt) {
  const r = await fetch(url, opt);
  if (!r.ok) throw new Error("HTTP " + r.status);
  return r.status === 204 ? {} : r.json();
}

function kleurOnline(s) {
  const t = new Date((s.laatst_gezien || "").replace(" ", "T"));
  if (isNaN(t.getTime())) return "nooit online";
  const sec = (Date.now() - t.getTime()) / 1000;
  if (sec < 30) return "online";
  if (sec < 3600) return Math.round(sec / 60) + " min geleden";
  return Math.round(sec / 3600) + " uur geleden";
}

function isOnline(s) {
  const t = new Date((s.laatst_gezien || "").replace(" ", "T"));
  return !isNaN(t.getTime()) && (Date.now() - t.getTime()) / 1000 < 30;
}

function sinds(s) {
  const t = new Date((s.gewijzigd || "").replace(" ", "T"));
  if (isNaN(t.getTime())) return "-";
  return t.toLocaleTimeString("nl-NL", { hour: "2-digit", minute: "2-digit" });
}

function playlistVan(s) {
  return (s.expand && s.expand.playlist) || null;
}

/* ---------- roteren (zelfde regels als de server) ---------- */

// kiesVoorPlaylist zoekt een kandidaat voor deze playlist; handmatige selectie gaat voor.
async function kiesVoorPlaylist(pl, huidigId) {
  const handmatig = pl.wallpapers || [];
  if (handmatig.length) {
    const kandidaten = handmatig.filter((id) => id !== huidigId || handmatig.length === 1);
    if (!kandidaten.length) return null;
    return kandidaten[Math.floor(Math.random() * kandidaten.length)];
  }
  let url = `/api/collections/wallpapers/records?perPage=500&sort=-created`;
  const tags = tagList(pl.tags);
  if (tags.length) url += "&filter=" + encodeURIComponent(buildFilterClient(tags));
  const data = await apiJSON(url);
  const ratios = tagList(pl.ratios);
  const kandidaten = (data.items || []).filter((w) => {
    if (w.id === huidigId && (data.items || []).length > 1) return false;
    if (ratios.length && !ratios.includes(bucketOf(w))) return false;
    return true;
  });
  if (!kandidaten.length) return null;
  return kandidaten[Math.floor(Math.random() * kandidaten.length)].id;
}

// buildFilterClient maakt de PocketBase-filter voor meerdere tags (allemaal aanwezig).
function buildFilterClient(tags) {
  return tags.map((t) => `tags~'${String(t).replace(/'/g, "\\'")}'`).join("&&");
}

/* ---------- acties ---------- */

async function zetScherm(s, velden, melding) {
  const r = await fetch(`${SCH}/${s.id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(velden),
  });
  if (!r.ok) return toast("Bijwerken mislukt (" + r.status + ")", "err");
  if (melding) toast(melding, "ok");
  await laadSchermen();
}

async function nuTonen(s, wallpaperId) {
  await zetScherm(
    s,
    { huidige_wallpaper: wallpaperId, bron: "handmatig", gewijzigd: new Date().toISOString() },
    "Op " + (s.monitor || s.key) + " gezet (blijft staan)"
  );
}

async function terugNaarRotatie(s) {
  await zetScherm(s, { bron: "rotatie", gewijzigd: new Date().toISOString() }, "Rotatie hervat");
}

async function wisselNu(s) {
  const pl = playlistVan(s);
  const id = await kiesVoorPlaylist(pl || {}, s.huidige_wallpaper);
  if (!id) return toast("Geen kandidaat binnen deze playlist", "err");
  await zetScherm(
    s,
    { huidige_wallpaper: id, bron: "rotatie", gewijzigd: new Date().toISOString() },
    "Gewisseld"
  );
}

async function bewaarInstellingen(s, velden) {
  const pl = playlistVan(s);
  const body = {
    naam: "scherm " + (s.device || "") + " " + (s.monitor || s.key),
    tags: velden.tags,
    ratios: velden.ratios,
    interval: velden.interval,
    random: true,
    actief: velden.interval > 0,
    wallpapers: velden.wallpapers,
  };
  let plId = s.playlist;
  if (plId) {
    const r = await fetch(`${PLS}/${plId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!r.ok) return toast("Playlist opslaan mislukt (" + r.status + ")", "err");
  } else {
    const r = await fetch(PLS, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!r.ok) return toast("Playlist aanmaken mislukt (" + r.status + ")", "err");
    plId = (await r.json()).id;
    await fetch(`${SCH}/${s.id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ playlist: plId }),
    });
  }
  if (!pl) s.playlist = plId;
  toast("Instellingen opgeslagen", "ok");
  await laadSchermen();
}

/* ---------- render ---------- */

function ratioVakjes(pl, prefix) {
  const aan = tagList(pl.ratios);
  return RATIOS.map(
    (r) =>
      `<label class="ratiovak"><input type="checkbox" data-${prefix}="${r.key}" ${aan.includes(r.key) ? "checked" : ""}> ${r.key}</label>`
  ).join("");
}

function schermHTML(s) {
  const pl = playlistVan(s) || {};
  const wp = (s.expand && s.expand.huidige_wallpaper) || null;
  const online = isOnline(s);
  const thumb = wp
    ? `<img class="scherm-thumb" loading="lazy" src="/api/files/${wp.collectionId}/${wp.id}/${encodeURIComponent(wp.file)}?thumb=400x0" alt="">`
    : `<div class="scherm-thumb leeg">nog niets</div>`;
  return `
  <div class="scherm" data-id="${s.id}">
    <div class="scherm-kop">
      <b>${escapeHtml(s.device || "?")}</b>
      <span class="sub">${escapeHtml(s.monitor || "")} #${s.index ?? ""} · ${s.width || "?"}×${s.height || "?"}</span>
      <span class="stip ${online ? "aan" : "uit"}" title="${kleurOnline(s)}">${online ? "online" : "offline"}</span>
    </div>
    <div class="scherm-nu">
      ${thumb}
      <div class="scherm-nu-tekst">
        <b>${wp ? escapeHtml(wp.title || wp.file) : "nog geen opdracht"}</b>
        <span class="sub">${s.gewijzigd ? "sinds " + sinds(s) + " · " : ""}${s.bron === "handmatig" ? "vastgezet (pin)" : "rotatie"}</span>
      </div>
      <div class="scherm-nu-acties">
        <button class="btn klein" data-actie="wissel">Wissel nu</button>
        ${s.bron === "handmatig" ? '<button class="btn klein" data-actie="rotatie">Terug naar rotatie</button>' : ""}
      </div>
    </div>
    <div class="scherm-instellingen">
      <label>Interval <input type="number" min="0" step="1" class="input mini" data-veld="interval" value="${pl.interval ?? 30}"> min (0 = niet automatisch)</label>
      <label>Tags <input type="text" class="input" data-veld="tags" value="${escapeHtml(pl.tags || "")}" placeholder="leeg = alle tags"></label>
      <div class="ratio-rij">${ratioVakjes(pl, "ratio")}</div>
      <div class="scherm-knoppen">
        <span class="sub" id="sel-info-${s.id}">${(pl.wallpapers || []).length ? (pl.wallpapers || []).length + " vaste afbeelding(en)" : "selectie via tags/verhoudingen"}</span>
        <button class="btn klein" data-actie="opslaan">Opslaan</button>
        <button class="btn klein" data-actie="uitselectie">Selectie uit raster</button>
        <button class="btn klein" data-actie="leegselectie">Selectie wissen</button>
      </div>
    </div>
    <details class="scherm-historie"><summary>laatst getoond</summary><div data-historie>laden…</div></details>
  </div>`;
}

function renderSchermen() {
  const lijst = $("#schermenlijst");
  if (!lijst) return;
  if (!schermen.length) {
    lijst.innerHTML = `<p class="sub" style="padding:8px 2px">Nog geen schermen gemeld. Start de client (met "server bepaalt" aan) — die meldt zijn monitoren en verschijnt hier.</p>`;
    $("#schermen-status").textContent = "";
    return;
  }
  lijst.innerHTML = schermen.map(schermHTML).join("");
  $("#schermen-status").textContent =
    schermen.length + " scherm(en) · " + schermen.filter(isOnline).length + " online";
  for (const s of schermen) laadHistorie(s);
}

async function laadHistorie(s) {
  const vak = document.querySelector(`.scherm[data-id="${s.id}"] [data-historie]`);
  if (!vak) return;
  try {
    const data = await apiJSON(
      `${HIS}?perPage=5&sort=-toegepast&expand=wallpaper&filter=` + encodeURIComponent(`screen='${s.id}'`)
    );
    const items = data.items || [];
    vak.innerHTML = items.length
      ? items
          .map((h) => {
            const w = (h.expand && h.expand.wallpaper) || {};
            const t = new Date((h.toegepast || h.created || "").replace(" ", "T"));
            const tijd = isNaN(t.getTime()) ? "-" : t.toLocaleString("nl-NL", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
            return `<div class="historie-regel"><span class="sub">${tijd}</span> ${escapeHtml(w.title || "-")} <span class="sub">${h.bron || ""}</span></div>`;
          })
          .join("")
      : '<span class="sub">nog niets toegepast</span>';
  } catch (e) {
    vak.innerHTML = '<span class="sub">kon geschiedenis niet laden</span>';
  }
}

async function laadSchermen() {
  try {
    const data = await apiJSON(`${SCH}?perPage=100&sort=device,index&expand=playlist,huidige_wallpaper`);
    schermen = data.items || [];
    renderSchermen();
  } catch (e) {
    const lijst = $("#schermenlijst");
    if (lijst) lijst.innerHTML = `<p class="sub">Schermen konden niet geladen worden (${escapeHtml(e.message)})</p>`;
  }
}

/* ---------- events (gedelegeerd) ---------- */

const windowPatches = window.__wpPatches || (window.__wpPatches = []);

document.addEventListener("click", async (e) => {
  const knop = e.target.closest("#schermenlijst [data-actie]");
  if (!knop) return;
  const kaart = knop.closest(".scherm");
  const s = schermen.find((x) => x.id === kaart.dataset.id);
  if (!s) return;
  const actie = knop.dataset.actie;
  const leesVeld = (naam) => kaart.querySelector(`[data-veld="${naam}"]`).value;
  const ratios = [...kaart.querySelectorAll('[data-ratio]:checked')].map((c) => c.dataset.ratio).join(",");

  if (actie === "opslaan") {
    await bewaarInstellingen(s, {
      tags: leesVeld("tags"),
      ratios,
      interval: parseInt(leesVeld("interval"), 10) || 0,
      wallpapers: s._selectie || undefined,
    });
  } else if (actie === "wissel") {
    await wisselNu(s);
  } else if (actie === "rotatie") {
    await terugNaarRotatie(s);
  } else if (actie === "uitselectie") {
    // de aangevinkte wallpapers uit het raster worden de vaste selectie
    const gekozen = [...state.selected];
    if (!gekozen.length) return toast("Selecteer eerst wallpapers in het raster (knop Selecteren)", "err");
    s._selectie = gekozen;
    document.querySelector(`#sel-info-${s.id}`).textContent = gekozen.length + " vaste afbeelding(en)";
    toast("Klik nu op Opslaan om de selectie vast te leggen", "ok");
  } else if (actie === "leegselectie") {
    s._selectie = [];
    document.querySelector(`#sel-info-${s.id}`).textContent = "selectie via tags/verhoudingen";
  }
});

const schermenKnop = $("#schermen-vernieuw");
if (schermenKnop) schermenKnop.onclick = () => laadSchermen();

// "toon nu op scherm" vanaf een wallpaper-kaart
document.addEventListener("click", async (e) => {
  const knop = e.target.closest("[data-toonscherm]");
  if (!knop) return;
  const wpId = knop.dataset.toonscherm;
  if (!schermen.length) return toast("Geen schermen bekend — start eerst de client", "err");
  if (schermen.length === 1) return nuTonen(schermen[0], wpId);
  const keuze = prompt(
    "Op welk scherm?\n" + schermen.map((s, i) => `${i + 1}) ${s.device} ${s.monitor} #${s.index}`).join("\n"),
    "1"
  );
  const idx = parseInt(keuze, 10);
  if (!isNaN(idx) && idx >= 1 && idx <= schermen.length) await nuTonen(schermen[idx - 1], wpId);
});

laadSchermen();
setInterval(laadSchermen, 15000);
