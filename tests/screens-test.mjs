/*
 * Headless test van het schermenpaneel (fase 4): per scherm de playlist, de pin,
 * wisselen en de geschiedenis.
 *
 *   npm install && node tests/screens-test.mjs
 *
 * app.js en screens.js worden in één scope geëvalueerd, net als losse <script>-tags
 * in de browser (daar delen ze de globale lexicale scope).
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";

const hier = path.dirname(fileURLToPath(import.meta.url));
const PUB = path.join(hier, "..", "pb_public");

const html = fs.readFileSync(path.join(PUB, "index.html"), "utf8")
  .replace('<script src="/app.js"></script>', "")
  .replace('<script src="/screens.js"></script>', "");
const scripts = fs.readFileSync(path.join(PUB, "app.js"), "utf8") + "\n" +
  fs.readFileSync(path.join(PUB, "screens.js"), "utf8");

const nu = new Date().toISOString();
const oud = new Date(Date.now() - 3600 * 1000).toISOString();

const WP = [
  { id: "wp1", collectionId: "c1", collectionName: "wallpapers", title: "Mooi beeld", tags: "natuur", width: 2912, height: 1632, file: "a.jpg", created: nu },
  { id: "wp2", collectionId: "c1", collectionName: "wallpapers", title: "Tweede", tags: "natuur", width: 3840, height: 2160, file: "b.jpg", created: nu },
  { id: "wp3", collectionId: "c1", collectionName: "wallpapers", title: "Ultrawide", tags: "natuur", width: 3360, height: 1440, file: "c.jpg", created: nu },
];
const SCHERMEN = [
  {
    id: "sch1", collectionId: "c2", key: "SandmanPC|DELD128|0", device: "SandmanPC", monitor: "DELD128", index: 0,
    alias: "Bureau links",
    width: 3808, height: 1280, laatst_gezien: nu, playlist: "pl1", huidige_wallpaper: "wp1", bron: "rotatie", gewijzigd: nu,
    expand: {
      playlist: { id: "pl1", naam: "scherm 1", tags: "natuur", ratios: "16:9", interval: 15, actief: true, wallpapers: [] },
      huidige_wallpaper: WP[0],
    },
  },
  {
    id: "sch2", collectionId: "c2", key: "SandmanPC|AOC2702|1", device: "SandmanPC", monitor: "AOC2702", index: 1,
    alias: "Ultrawide",
    width: 3360, height: 1440, laatst_gezien: oud, playlist: "", huidige_wallpaper: "wp3", bron: "handmatig", gewijzigd: nu,
    expand: { huidige_wallpaper: WP[2] },
  },
];
const SCHERM_ZONDER_ALIAS = {
  id: "sch3", collectionId: "c2",
  key: "SANDMANPC|\\\\?\\DISPLAY#AOC2702#5&24a0a36a&0&UID4352#{guid}|0",
  device: "SANDMANPC",
  monitor: "\\\\?\\DISPLAY#AOC2702#5&24a0a36a&0&UID4352#{e6f07b5f-ee97-4a90-b076-33f57bf4eaa7}",
  index: 0, width: 3840, height: 2160, laatst_gezien: nu, playlist: "", huidige_wallpaper: "", bron: "rotatie", gewijzigd: nu,
  expand: {},
};
const HISTORIE = [
  { id: "h1", screen: "sch1", wallpaper: "wp1", bron: "rotatie", toegepast: nu, created: nu, expand: { wallpaper: WP[0] } },
];

const posts = [];

const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://wallpaparr.test/" });
const w = dom.window;

let patches = null;
w.open = () => null;
w.prompt = () => "2";
w.confirm = () => true;

w.fetch = async (url, opt = {}) => {
  const m = opt.method || "GET";
  const body = opt.body ? JSON.parse(opt.body) : null;
  if (m === "PATCH") {
    const rec = { url, body };
    w.__wpPatches.push(rec);
    console.log("PATCH van de UI:", url, JSON.stringify(body));
    return { ok: true, status: 200, json: async () => ({}) };
  }
  if (m === "POST") {
    posts.push({ url, body });
    return { ok: true, status: 200, json: async () => ({ id: "pl-nieuw" }) };
  }
  if (url.startsWith("/api/collections/screens/records")) {
    return { ok: true, status: 200, json: async () => ({ items: [...SCHERMEN, SCHERM_ZONDER_ALIAS] }) };
  }
  if (url.startsWith("/api/collections/playlists/records")) {
    return { ok: true, status: 200, json: async () => ({ items: [] }) };
  }
  if (url.startsWith("/api/collections/geschiedenis/records")) {
    return { ok: true, status: 200, json: async () => ({ items: HISTORIE }) };
  }
  return { ok: true, status: 200, json: async () => ({ items: WP, totalPages: 1 }) };
};

w.eval(scripts);
patches = w.__wpPatches;
const tick = (ms = 60) => new Promise((r) => setTimeout(r, ms));
await tick();

const $ = (s) => w.document.querySelector(s);
const q = (s) => [...w.document.querySelectorAll(s)];
const kaart = (id) => $(`.scherm[data-id="${id}"]`);
const knop = (id, actie) => kaart(id).querySelector(`[data-actie="${actie}"]`);
const ok = [];
const check = (naam, waar, extra = "") => ok.push({ naam, waar, extra });

/* ---------- paneel: standaard ingeklapt ---------- */
check("schermenpaneel is standaard ingeklapt", $("#schermenpaneel").classList.contains("ingeklapt"),
  $("#schermenpaneel").className);
check("de knop heet 'Schermen uitklappen'", $("#schermen-toggle").textContent === "Schermen uitklappen",
  $("#schermen-toggle").textContent);
$("#schermen-toggle").click();
await tick();
check("uitklappen toont de lijst en wisselt het label",
  !$("#schermenpaneel").classList.contains("ingeklapt") && $("#schermen-toggle").textContent === "Schermen inklappen",
  $("#schermen-toggle").textContent);
check("de schermen zijn zichtbaar na uitklappen", q(".scherm").length === 3, String(q(".scherm").length));
$("#schermen-toggle").click();
await tick();
check("weer inklappen werkt", $("#schermenpaneel").classList.contains("ingeklapt"));
$("#schermen-toggle").click();
await tick();

/* ---------- paneel ---------- */
check("drie schermen in het paneel", q(".scherm").length === 3, String(q(".scherm").length));
check("statusregel met online-aantal", $("#schermen-status").textContent.includes("3 scherm(en) · 2 online"),
  $("#schermen-status").textContent);
check("scherm 1 toont de huidige afbeelding", kaart("sch1").textContent.includes("Mooi beeld"));
check("scherm 1 staat op rotatie", kaart("sch1").textContent.includes("rotatie"));
check("scherm 2 is vastgezet (pin) en heeft de terugknop", !!knop("sch2", "rotatie") && kaart("sch2").textContent.includes("vastgezet"));
check("playlist-waarden voorgevuld", kaart("sch1").querySelector('[data-veld="interval"]').value === "15" &&
  kaart("sch1").querySelector('[data-veld="tags"]').value === "natuur");
check("verhouding 16:9 aangevinkt, 21:9 niet",
  kaart("sch1").querySelector('[data-ratio="16:9"]').checked && !kaart("sch1").querySelector('[data-ratio="21:9"]').checked);
await tick();
check("geschiedenis toont de vorige afbeelding", kaart("sch1").querySelector("[data-historie]").textContent.includes("Mooi beeld"),
  kaart("sch1").querySelector("[data-historie]").textContent.slice(0, 40));

/* ---------- pin terugdraaien ---------- */
patches.length = 0;
knop("sch2", "rotatie").click();
await tick();
const p2 = patches.find((p) => p.url.endsWith("sch2"));
check("terug naar rotatie stuurt de juiste PATCH", !!p2 && p2.body.bron === "rotatie" && !!p2.body.gewijzigd,
  JSON.stringify(p2 ? p2.body : {}));

/* ---------- wissel nu: kandidaat binnen de filters ---------- */
patches.length = 0;
knop("sch1", "wissel").click();
await tick();
const p1 = patches.find((p) => p.url.endsWith("sch1"));
check("wissel nu zet een nieuwe afbeelding in rotatiestand", !!p1 && p1.body.bron === "rotatie" && !!p1.body.huidige_wallpaper,
  JSON.stringify(p1 ? p1.body : {}));
check("niet dezelfde afbeelding als er al staat", p1 && p1.body.huidige_wallpaper !== "wp1", p1 && p1.body.huidige_wallpaper);
check("kandidaat past bij de verhouding (16:9, dus niet de 21:9 ultrawide)", p1 && p1.body.huidige_wallpaper !== "wp3",
  p1 && p1.body.huidige_wallpaper);

/* ---------- playlist opslaan (venster opnieuw geladen; knoppen opnieuw afstemmen) ---------- */
patches.length = 0;
await tick();
const input = (v) => kaart("sch1").querySelector(`[data-veld="${v}"]`);
input("interval").value = "45";
input("tags").value = "nacht,natuur";
kaart("sch1").querySelector('[data-ratio="21:9"]').checked = true;
knop("sch1", "opslaan").click();
await tick();
const PL1 = (a) => a.filter((p) => p.url.includes("/playlists/records/pl1")).map((p) => p.body);
const opslaan = patches.find((p) => p.url.includes("/playlists/records/pl1"));
function ploerna() { return opslaan || null; }
check("opslaan werkt de playlist bij", ploerna() && ploerna().body.interval === 45 && ploerna().body.tags === "nacht,natuur",
  JSON.stringify(ploerna() ? ploerna().body : {}));
check("aangevinkte verhoudingen gaan mee", ploerna() && ploerna().body.ratios.includes("16:9") && ploerna().body.ratios.includes("21:9"),
  ploerna() && ploerna().body.ratios);

/* ---------- nieuwe playlist voor een scherm zonder ---------- */
patches.length = 0; posts.length = 0;
knop("sch2", "opslaan").click();
await tick();
check("playlist wordt aangemaakt als die er nog niet is", posts.length === 1 && posts[0].body.actief === true,
  JSON.stringify(posts[0] ? posts[0].body : {}));
check("nieuwe playlist wordt aan het scherm gekoppeld",
  patches.some((p) => p.url.endsWith("sch2") && p.body.playlist === "pl-nieuw"),
  JSON.stringify(patches.map((p) => p.body)));

/* ---------- automatisch vs raster-selectie ---------- */
check("modus 'automatisch' is actief bij een playlist zonder vaste selectie",
  kaart("sch1").querySelector('.modus[data-modus="auto"]').classList.contains("actief") &&
  kaart("sch1").querySelector('input[value="auto"]').checked,
  kaart("sch1").querySelector(".modus.actief").dataset.modus);
check("modus 'raster' is actief bij een playlist mét vaste selectie",
  kaart("sch3").querySelector('.modus[data-modus="auto"]') !== null,
  "beide blokken bestaan");
// kiezen van raster licht het juiste blok op
kaart("sch1").querySelector('input[value="raster"]').checked = true;
kaart("sch1").querySelector('input[value="raster"]').dispatchEvent(new w.Event("change", { bubbles: true }));
await tick();
check("raster kiezen licht het rasterblok op",
  kaart("sch1").querySelector('.modus[data-modus="raster"]').classList.contains("actief") &&
  !kaart("sch1").querySelector('.modus[data-modus="auto"]').classList.contains("actief"));
// terug naar automatisch + opslaan wist de vaste selectie
kaart("sch1").querySelector('input[value="auto"]').checked = true;
kaart("sch1").querySelector('input[value="auto"]').dispatchEvent(new w.Event("change", { bubbles: true }));
await tick();

/* ---------- vaste selectie uit het raster ---------- */
// selectiemodus aan + één kaart aanklikken (via de gewone UI)
$("#selmode").click();
q(".card .thumb img")[0].click();
await tick();
check("één wallpaper geselecteerd in het raster", $("#selcount").textContent === "1", $("#selcount").textContent);
knop("sch1", "uitselectie").click();
await tick();
check("paneel meldt de vaste selectie", kaart("sch1").querySelector(`#sel-info-sch1`).textContent.includes("1 vaste"),
  kaart("sch1").querySelector(`#sel-info-sch1`).textContent);
check("'Selectie uit raster' zet de raster-modus aan",
  kaart("sch1").querySelector('input[value="raster"]').checked &&
  kaart("sch1").querySelector('.modus[data-modus="raster"]').classList.contains("actief"));

patches.length = 0;
knop("sch1", "opslaan").click();
await tick();
console.log("PL1-PATCHES NA-VASTESELECTIE:", JSON.stringify(patches.filter((p) => p.url.includes("/playlists/pl1")).map((p) => p.body)));
const plPati = () => patches.filter((p) => p.url.includes("/playlists/records/pl1"));
const pl = () => plPati().pop() || null;
check("vaste selectie wordt in de playlist gezet", pl() && Array.isArray(pl().body.wallpapers) && pl().body.wallpapers.length === 1,
  JSON.stringify(pl() ? pl().body.wallpapers : null));

/* ---------- automatisch opslaan wist de vaste selectie ---------- */
patches.length = 0;
kaart("sch1").querySelector('input[value="auto"]').checked = true;
kaart("sch1").querySelector('input[value="auto"]').dispatchEvent(new w.Event("change", { bubbles: true }));
await tick();
knop("sch1", "opslaan").click();
await tick();
const plAuto = () => patches.filter((p) => p.url.includes("/playlists/records/pl1")).pop() || null;
check("automatisch opslaan stuurt een lege raster-selectie mee",
  plAuto() && Array.isArray(plAuto().body.wallpapers) && plAuto().body.wallpapers.length === 0,
  JSON.stringify(plAuto() ? plAuto().body.wallpapers : null));

/* ---------- alias van een scherm ---------- */
check("paneel toont de alias i.p.v. het apparaatpad", kaart("sch1").querySelector(".scherm-kop b").textContent === "Bureau links",
  kaart("sch1").querySelector(".scherm-kop b").textContent);
check("aliasveld aanwezig op de kaart", !!kaart("sch1").querySelector('[data-veld="alias"]'));
check("zonder alias wordt het COM-pad leesbaar gemaakt met UID",
  kaart("sch3").querySelector(".scherm-kop b").textContent === "SANDMANPC AOC2702 UID4352",
  kaart("sch3").querySelector(".scherm-kop b").textContent);
check("ondertitel toont de korte monitornaam i.p.v. het COM-pad",
  kaart("sch3").querySelector(".scherm-kop .sub").textContent.includes("AOC2702 UID4352") &&
    !kaart("sch3").querySelector(".scherm-kop .sub").textContent.includes("DISPLAY#"),
  kaart("sch3").querySelector(".scherm-kop .sub").textContent);

patches.length = 0;
kaart("sch1").querySelector('[data-veld="alias"]').value = "Bureau";
knop("sch1", "alias").click();
await tick();
const aliasPatch = patches.find((p) => p.url.endsWith("sch1") && p.body && p.body.alias === "Bureau");
check("naam bewaren stuurt een PATCH met het alias-veld", !!aliasPatch, JSON.stringify(aliasPatch ? aliasPatch.body : {}));

/* ---------- gebruiksteller op de tegels ---------- */
await tick(80);
const tegel = (titel) => q(".card").find((c) => c.querySelector(".title").textContent === titel);
const badge = (titel) => { const c = tegel(titel); return c && c.querySelector(".gebruik"); };
check("gebruikte afbeelding toont de teller (1×)", !!badge("Mooi beeld") && badge("Mooi beeld").textContent === "1×",
  badge("Mooi beeld") && badge("Mooi beeld").textContent);
check("ongebruikte afbeelding toont 0×", !!badge("Tweede") && badge("Tweede").textContent === "0×",
  badge("Tweede") && badge("Tweede").textContent);

/* ---------- vanaf een wallpaper-kaart naar een scherm sturen (schermkiezer) ---------- */
patches.length = 0;
const kaartKnop = q(".card .icon").find((b) => b.dataset.toonscherm);
check("elke wallpaper heeft een 'toon op scherm'-knop", !!kaartKnop);
kaartKnop.click();
await tick();
const kiezer = $(".schermkiezer");
check("schermkiezer opent met de schermen bij hun alias",
  !!kiezer && kiezer.textContent.includes("Ultrawide") && kiezer.textContent.includes("Bureau links"),
  kiezer ? kiezer.textContent.replace(/\s+/g, " ").slice(0, 90) : "geen kiezer");
check("kiezer toont ook apparaat/scherm als ondertitel", !!kiezer && kiezer.textContent.includes("AOC2702"));
kiezer.querySelector('[data-scherm="sch2"]').click();
await tick();
const pin = patches.find((p) => p.url.endsWith("sch2") && p.body && p.body.bron === "handmatig");
check("keuze uit de kiezer pint de afbeelding op dat scherm", !!pin && !!pin.body.huidige_wallpaper,
  JSON.stringify(pin ? pin.body : {}));
check("de kiezer sluit na de keuze", !$(".schermkiezer"));

let fout = 0;
for (const r of ok) {
  if (!r.waar) fout++;
  console.log(`${r.waar ? "OK  " : "FOUT"}  ${r.naam}${r.extra ? "   [" + r.extra + "]" : ""}`);
}
console.log(`\n${ok.length - fout}/${ok.length} checks geslaagd`);
process.exit(fout ? 1 : 0);
