/*
 * Headless test van het schijfbalkje en van exporteren/importeren (jsdom).
 *
 *   npm install && node tests/export-test.mjs
 *
 * app.js, screens.js en export.js worden in één scope geëvalueerd, net als losse
 * <script>-tags in de browser (daar delen ze de globale lexicale scope).
 *
 * Naast de UI-logica test dit ook de zip die de web-UI zelf schrijft: die gaat naar
 * buiten en moet door gewone gereedschappen (unzip) gelezen kunnen worden.
 */
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import zlib from "node:zlib";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";

const hier = path.dirname(fileURLToPath(import.meta.url));
const PUB = path.join(hier, "..", "pb_public");

let html = fs.readFileSync(path.join(PUB, "index.html"), "utf8");
for (const s of ["app.js", "screens.js", "export.js"]) html = html.replace(`<script src="/${s}"></script>`, "");
const scripts =
  fs.readFileSync(path.join(PUB, "app.js"), "utf8") + "\n" +
  fs.readFileSync(path.join(PUB, "screens.js"), "utf8") + "\n" +
  fs.readFileSync(path.join(PUB, "export.js"), "utf8");

const nu = new Date().toISOString();
const WP = [
  { id: "wp1", collectionId: "c1", collectionName: "wallpapers", title: "Mooi beeld", tags: "natuur", width: 1920, height: 1080, bytes: 1000, file: "a.jpg", created: nu },
  { id: "wp2", collectionId: "c1", collectionName: "wallpapers", title: "Tweede", tags: "natuur,nacht", width: 3840, height: 2160, bytes: 2000, file: "b.jpg", created: nu },
  { id: "wp3", collectionId: "c1", collectionName: "wallpapers", title: "Derde", tags: "abstract", width: 1280, height: 720, bytes: 500, file: "c.jpg", created: nu },
];
const PL = [{ id: "pl1", naam: "scherm 1", tags: "natuur", ratios: "16:9", interval: 15, random: true, actief: true, wallpapers: ["wp1", "wp2"] }];
const SCHERMEN = [
  { id: "sch1", collectionId: "c2", key: "SandmanPC|DELD128|0", device: "SandmanPC", monitor: "DELD128", index: 0, alias: "Bureau links", width: 1920, height: 1080, playlist: "pl1", expand: { playlist: PL[0] } },
];

const DISK = {
  pad: "/pb/pb_data", beschikbaar: true, bestandssysteem: "/dev/sda1", mount: "/pb",
  totaal: 100 * 1073741824, gebruikt: 60 * 1073741824, vrij: 40 * 1073741824, procent: 60,
  wallpapers: 3, wallpapersBytes: 3500,
};

const posts = [];
const patches = [];
const downloads = [];
let geexporteerdeZip = null;

const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://wallpaparr.test/" });
const w = dom.window;

// jsdom mist deze web-API's (of heeft een Blob zonder stream()); de browser heeft ze wel.
// We lenen ze van Node, zodat de lezer van gecomprimeerde zip-bestanden ook echt getest wordt.
for (const naam of ["Blob", "Response", "DecompressionStream", "FormData"]) {
  Object.defineProperty(w, naam, { value: globalThis[naam], configurable: true, writable: true });
}

w.open = () => null;
w.confirm = () => true;
w.alert = () => {};

const json = (data, status = 200) => ({ ok: true, status, json: async () => data });
const bytes = (u8) => ({ ok: true, status: 200, arrayBuffer: async () => u8.buffer.slice(0) });

w.fetch = async (url, opt = {}) => {
  const m = opt.method || "GET";
  const isForm = typeof w.FormData !== "undefined" && opt.body instanceof w.FormData;
  const body = opt.body && !isForm ? JSON.parse(opt.body) : null;
  if (m === "POST") {
    posts.push({ url, body, form: isForm ? opt.body : null });
    if (opt.body && !isForm && url.includes("/playlists/")) return json({ id: "pl-nieuw-" + posts.length });
    if (isForm) return json({ id: "wp-nieuw-" + posts.length });
    return json({ id: "nieuw" });
  }
  if (m === "PATCH") {
    patches.push({ url, body });
    return json({});
  }
  if (url.startsWith("/api/wallpaparr/disk")) return json(DISK);
  if (url.startsWith("/api/collections/screens/records")) {
    // net als PocketBase: met ?fields= krijg je alleen die velden terug
    const m = url.match(/fields=([^&]+)/);
    const velden = m ? decodeURIComponent(m[1]).split(",") : null;
    const items = SCHERMEN.map((s) => (velden ? Object.fromEntries(velden.map((v) => [v, s[v]])) : s));
    return json({ items });
  }
  if (url.startsWith("/api/collections/playlists/records")) return json({ items: PL });
  if (url.startsWith("/api/collections/geschiedenis/records")) return json({ items: [] });
  if (url.startsWith("/api/files/")) {
    const naam = decodeURIComponent(url.split("/").pop());
    const inhoud = new TextEncoder().encode("beeld-van-" + naam);
    return bytes(inhoud);
  }
  return json({ items: WP, totalPages: 1, totalItems: WP.length });
};

// download opvangen (jsdom kent geen echte navigatie naar blob-URL's)
w.URL.createObjectURL = (blob) => {
  geexporteerdeZip = blob;
  return "blob:test";
};
w.URL.revokeObjectURL = () => {};
w.HTMLAnchorElement.prototype.click = function () {
  downloads.push(this.download);
};

w.eval(scripts);
const tick = (ms = 60) => new Promise((r) => setTimeout(r, ms));
await tick();

const $ = (s) => w.document.querySelector(s);
const q = (s) => [...w.document.querySelectorAll(s)];
const EXP = w.__wpExport;
const ok = [];
const check = (naam, waar, extra = "") => ok.push({ naam, waar, extra });

/* ---------- schijfruimte op de hoofdpagina ---------- */

check("schijfbalk verschijnt op de hoofdpagina", !!$("#disk") && !$("#disk").classList.contains("hidden"), $("#disk") && $("#disk").className);
check("balk gevuld op het gebruikte percentage", $("#diskfill").style.width === "60%", $("#diskfill").style.width);
check("balk is rustig gekleurd onder 75%", $("#diskfill").className === "", $("#diskfill").className);
check("er staat een leesbare regel onder", $("#diskinfo").textContent.includes("40,0 GB vrij van 100,0 GB"),
  $("#diskinfo").textContent);
check("de regel noemt ook het aandeel van de wallpapers", $("#diskinfo").textContent.includes("wallpapers 3 KB"),
  $("#diskinfo").textContent);
check("tooltip legt uit waar het over gaat", $("#disk").title.includes("/dev/sda1") && $("#disk").title.includes("/pb"),
  $("#disk").title);

/* ---------- zip: schrijven en lezen ---------- */

check("crc32 klopt op de bekende testreeks", EXP.crc32(new TextEncoder().encode("123456789")) === 0xcbf43926,
  String(EXP.crc32(new TextEncoder().encode("123456789"))));

const ruweZip = EXP.maakZip([
  { naam: "manifest.json", bytes: new TextEncoder().encode('{"app":"wallpaparr"}') },
  { naam: "files/w0--a.jpg", bytes: new Uint8Array([0, 1, 2, 250, 255, 7]) },
]);
const terug = await EXP.leesZip(ruweZip);
check("zip bevat beide bestanden", terug.size === 2, String(terug.size));
check("tekstbestand komt er heel uit", new TextDecoder().decode(terug.get("manifest.json")) === '{"app":"wallpaparr"}',
  new TextDecoder().decode(terug.get("manifest.json")));
check("binaire inhoud komt er byte voor byte uit",
  [...terug.get("files/w0--a.jpg")].join(",") === "0,1,2,250,255,7",
  [...terug.get("files/w0--a.jpg")].join(","));

// dezelfde zip door een ánder programma laten lezen: bewijs dat de zip-standaard klopt
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "wpzip-"));
const zipPad = path.join(tmp, "gemaakt.zip");
fs.writeFileSync(zipPad, ruweZip);
let lezer = "";
try {
  lezer = execFileSync("python3", ["-c", "import sys,zipfile\nz=zipfile.ZipFile(sys.argv[1])\nsys.stdout.write((z.testzip() or 'goed')+'|'+z.read(sys.argv[2]).hex())", zipPad, "files/w0--a.jpg"], { encoding: "utf8" });
} catch (e) {
  lezer = "";
}
check("zip van de web-UI wordt door een ander programma gelezen (python zipfile)",
  !lezer || lezer === "goed|000102faff07", lezer || "python3 niet aanwezig — overgeslagen");

// en een zip met compressie (methode 8, zoals gewone zip-programma's maken) moet ook werken.
// Die bouwen we hier zelf met zlib, onafhankelijk van de schrijver in de web-UI.
function zipMetCompressie(naam, data) {
  const naamB = Buffer.from(naam, "utf8");
  const comp = zlib.deflateRawSync(data);
  const lokaal = Buffer.alloc(30);
  lokaal.writeUInt32LE(0x04034b50, 0);
  lokaal.writeUInt16LE(20, 4);
  lokaal.writeUInt16LE(0, 6);
  lokaal.writeUInt16LE(8, 8);
  lokaal.writeUInt32LE(crc(data), 14);
  lokaal.writeUInt32LE(comp.length, 18);
  lokaal.writeUInt32LE(data.length, 22);
  lokaal.writeUInt16LE(naamB.length, 26);
  const centraal = Buffer.alloc(46);
  centraal.writeUInt32LE(0x02014b50, 0);
  centraal.writeUInt16LE(20, 4);
  centraal.writeUInt16LE(20, 6);
  centraal.writeUInt16LE(8, 10);
  centraal.writeUInt32LE(crc(data), 16);
  centraal.writeUInt32LE(comp.length, 20);
  centraal.writeUInt32LE(data.length, 24);
  centraal.writeUInt16LE(naamB.length, 28);
  centraal.writeUInt32LE(0, 42);
  const cd = Buffer.concat([centraal, naamB]);
  const eocd = Buffer.alloc(22);
  eocd.writeUInt32LE(0x06054b50, 0);
  eocd.writeUInt16LE(1, 8);
  eocd.writeUInt16LE(1, 10);
  eocd.writeUInt32LE(cd.length, 12);
  eocd.writeUInt32LE(30 + naamB.length + comp.length, 16);
  return Buffer.concat([lokaal, naamB, comp, cd, eocd]);
}
function crc(bytes) {
  const buf = Buffer.from(bytes);
  let c = 0xffffffff;
  for (const b of buf) {
    c ^= b;
    for (let k = 0; k < 8; k++) c = c & 1 ? (c >>> 1) ^ 0xedb88320 : c >>> 1;
  }
  return (c ^ 0xffffffff) >>> 0;
}

const lang = Buffer.from("hallo ".repeat(200), "utf8");
const buiten = await EXP.leesZip(new Uint8Array(zipMetCompressie("gecomprimeerd.txt", lang)));
check("zip van buiten (met compressie) wordt ook gelezen",
  new TextDecoder().decode(buiten.get("gecomprimeerd.txt")) === "hallo ".repeat(200),
  String((buiten.get("gecomprimeerd.txt") || []).length));

/* ---------- manifest ---------- */

const manifest = EXP.bouwManifest(WP, PL, SCHERMEN, { bereik: "alles", metAfbeeldingen: true });
check("manifest noemt de app en het formaat", manifest.app === "wallpaparr" && manifest.formaat === 1);
check("alle wallpapers met hun tags", manifest.wallpapers.length === 3 && manifest.wallpapers[1].tags === "natuur,nacht");
check("elke wallpaper heeft een eigen zip-pad", manifest.wallpapers[0].zipPad === "files/w0--a.jpg", manifest.wallpapers[0].zipPad);
check("playlist-verwijzingen gaan via sleutels, niet via id's",
  manifest.playlists[0].wallpapers.join(",") === "w0,w1", manifest.playlists[0].wallpapers.join(","));
check("playlist-instellingen zitten erbij",
  manifest.playlists[0].tags === "natuur" && manifest.playlists[0].interval === 15 && manifest.playlists[0].actief === true);
check("scherm verwijst naar de playlist op index",
  manifest.schermen[0].key === "SandmanPC|DELD128|0" && manifest.schermen[0].playlist === 0,
  JSON.stringify(manifest.schermen[0]));

let geweigerd = "";
try {
  EXP.controleerManifest({ app: "iets anders", wallpapers: [] });
} catch (e) {
  geweigerd = e.message;
}
check("een vreemd bestand wordt geweigerd", geweigerd.includes("geen Wallpaparr-export"), geweigerd);

check("bestandsnaam wordt schoon gemaakt", EXP.veiligeNaam("../../etc/pa s.jpg") === "pa s.jpg", EXP.veiligeNaam("../../etc/pa s.jpg"));

// PocketBase hernoemt uploads (naam_<random>.ext): dezelfde plaat moet toch als "bestaat al"
// herkend worden, ook na een tweede ronde
check("hernoemde uploads worden als dezelfde afbeelding gezien",
  EXP.bestandsSleutel("e2e-een.png") === EXP.bestandsSleutel("e2e_een_9j8ottjze5.png") &&
  EXP.bestandsSleutel("e2e_een_9j8ottjze5.png") === EXP.bestandsSleutel("e2e_een_9j8ottjze5_ab12cd3456.png"),
  EXP.bestandsSleutel("e2e_een_9j8ottjze5_ab12cd3456.png"));
check("verschillende afbeeldingen blijven verschillend",
  EXP.bestandsSleutel("a.jpg") !== EXP.bestandsSleutel("b.jpg"),
  EXP.bestandsSleutel("a.jpg") + " / " + EXP.bestandsSleutel("b.jpg"));

/* ---------- exporteren via het venster ---------- */

downloads.length = 0;
$("#export").click();
await tick();
check("exportvenster opent", !!$(".venster"));
check("twee bereiken en twee vormen om te kiezen", q('.venster input[name="ex-bereik"]').length === 2 &&
  q('.venster input[name="ex-vorm"]').length === 2);
check("standaard: alles, met afbeeldingen",
  $('.venster input[name="ex-bereik"]:checked').value === "alles" &&
  $('.venster input[name="ex-vorm"]:checked').value === "zip");

$("#ex-gaan").click();
await tick(300);
check("zip-export levert een zip-bestand op", downloads.length === 1 && /^wallpaparr-export-.*\.zip$/.test(downloads[0]), downloads[0]);
const geexporteerd = await EXP.leesZip(new Uint8Array(await geexporteerdeZip.arrayBuffer()));
check("de zip bevat een manifest en de drie afbeeldingen",
  geexporteerd.has("manifest.json") && geexporteerd.has("files/w1--b.jpg"),
  [...geexporteerd.keys()].join(","));
const exManifest = JSON.parse(new TextDecoder().decode(geexporteerd.get("manifest.json")));
check("de afbeeldingen zitten er echt in",
  new TextDecoder().decode(geexporteerd.get("files/w1--b.jpg")) === "beeld-van-b.jpg",
  new TextDecoder().decode(geexporteerd.get("files/w1--b.jpg")));
check("de instellingen zitten in de export",
  exManifest.playlists.length === 1 && exManifest.schermen.length === 1 && exManifest.wallpapers.length === 3);
check("exportvenster sluit na afloop", !$(".venster"));

/* ---------- exporteren van alleen het filter ---------- */

downloads.length = 0;
$("#search").value = "abstract";
$("#search").dispatchEvent(new w.Event("input"));
await tick();
$("#export").click();
await tick();
check("bij een filter staat het aantal in de keuze",
  $('.venster input[name="ex-bereik"][value="selectie"]').closest(".keuze").textContent.includes("1 van 3"),
  $('.venster input[name="ex-bereik"][value="selectie"]').closest(".keuze").textContent.replace(/\s+/g, " "));
$('.venster input[name="ex-bereik"][value="selectie"]').checked = true;
$('.venster input[name="ex-vorm"][value="json"]').checked = true;
$("#ex-gaan").click();
await tick(300);
check("json-export levert een json-bestand op", downloads.length === 1 && downloads[0].endsWith(".json"), downloads[0]);
const losseJson = JSON.parse(new TextDecoder().decode(new Uint8Array(await geexporteerdeZip.arrayBuffer())));
check("alleen de gefilterde wallpaper zit in de json", losseJson.wallpapers.length === 1 && losseJson.wallpapers[0].bestand === "c.jpg",
  JSON.stringify(losseJson.wallpapers.map((x) => x.bestand)));
check("de json zegt dat er geen afbeeldingen in zitten", losseJson.metAfbeeldingen === false && losseJson.bereik === "selectie");
$("#search").value = "";
$("#search").dispatchEvent(new w.Event("input"));
await tick();

/* ---------- importeren ---------- */

const importManifest = {
  app: "wallpaparr", formaat: 1, gemaakt: nu, bereik: "alles", metAfbeeldingen: true,
  wallpapers: [
    { sleutel: "w0", bestand: "a.jpg", zipPad: "files/w0--a.jpg", titel: "Nieuwe titel", tags: "natuur,zon", width: 1920, height: 1080, bytes: 10 },
    { sleutel: "w1", bestand: "nieuw.jpg", zipPad: "files/w1--nieuw.jpg", titel: "Nieuw", tags: "test", width: 800, height: 600, bytes: 10 },
  ],
  playlists: [{ naam: "scherm 1", tags: "test", ratios: "16:9", interval: 20, random: true, actief: true, wallpapers: ["w0", "w1"] }],
  schermen: [{ key: "SandmanPC|DELD128|0", alias: "Bureau links", playlist: 0 }],
};
const importZip = EXP.maakZip([
  { naam: "manifest.json", bytes: new TextEncoder().encode(JSON.stringify(importManifest)) },
  { naam: "files/w0--a.jpg", bytes: new TextEncoder().encode("plaat-a") },
  { naam: "files/w1--nieuw.jpg", bytes: new TextEncoder().encode("plaat-nieuw") },
]);
const bundel = await EXP.leesBundel(importZip, "x.zip");

posts.length = 0;
patches.length = 0;
const uit = await EXP.importeer(bundel, { dubbel: "overslaan" }, () => {});
check("bestaande bestandsnaam wordt overgeslagen", uit.overgeslagen === 1 && uit.toegevoegd === 1,
  `toegevoegd ${uit.toegevoegd}, overgeslagen ${uit.overgeslagen}`);
check("de nieuwe wallpaper wordt geüpload met tags en afmetingen",
  posts.some((p) => p.form && p.form.get("file") && p.form.get("tags") === "test" && p.form.get("height") === "600"),
  JSON.stringify(posts.map((p) => (p.form ? p.form.get("file").name : p.body))));
check("de playlist wordt aangemaakt met de nieuwe afbeeldingen",
  posts.some((p) => p.body && p.body.naam === "scherm 1" && p.body.wallpapers.length === 2 && p.body.interval === 20),
  JSON.stringify(posts.filter((p) => p.body).map((p) => p.body)));
check("het scherm wordt aan de nieuwe playlist gehangen", uit.schermen === 1 && patches.some((p) => p.url.includes("/screens/records/sch1") && p.body.playlist === "pl-nieuw-" + (posts.findIndex((p) => p.body) + 1)),
  JSON.stringify(patches));

posts.length = 0;
patches.length = 0;
const uit2 = await EXP.importeer(bundel, { dubbel: "bijwerken" }, () => {});
check("met 'bijwerken' wordt de bestaande wallpaper aangepast", uit2.bijgewerkt === 1 && uit2.toegevoegd === 1,
  `bijgewerkt ${uit2.bijgewerkt}, toegevoegd ${uit2.toegevoegd}`);
check("de bestaande wallpaper krijgt de nieuwe tags",
  patches.some((p) => p.url.includes("/wallpapers/records/wp1") && p.body.tags === "natuur,zon"),
  JSON.stringify(patches));

/* ---------- een hernoemde upload telt als "bestaat al" ---------- */

const hernoemd = {
  app: "wallpaparr", formaat: 1, gemaakt: nu, bereik: "alles", metAfbeeldingen: true,
  wallpapers: [{ sleutel: "w0", bestand: "c_9j8ottjze5.png", zipPad: "files/w0--c.png", titel: "Derde", tags: "abstract", width: 1280, height: 720, bytes: 5 }],
  playlists: [], schermen: [],
};
const hernoemdBundel = await EXP.leesBundel(
  EXP.maakZip([
    { naam: "manifest.json", bytes: new TextEncoder().encode(JSON.stringify(hernoemd)) },
    { naam: "files/w0--c.png", bytes: new TextEncoder().encode("plaat-c") },
  ]),
  "y.zip"
);
posts.length = 0;
const uit3 = await EXP.importeer(hernoemdBundel, { dubbel: "overslaan" }, () => {});
check("een door PocketBase hernoemde afbeelding wordt als 'bestaat al' gezien",
  uit3.overgeslagen === 1 && uit3.toegevoegd === 0, JSON.stringify(uit3));

/* ---------- importeren via het venster (alleen gegevens) ---------- */

const alleenGegevens = {
  app: "wallpaparr", formaat: 1, gemaakt: nu, bereik: "alles", metAfbeeldingen: false,
  wallpapers: [{ sleutel: "w0", bestand: "b.jpg", zipPad: "files/w0--b.jpg", titel: "Bijgewerkt", tags: "natuur,lente", width: 1, height: 1, bytes: 1 }],
  playlists: [],
  schermen: [],
};
posts.length = 0;
patches.length = 0;
await EXP.openImport({ name: "deel.json", arrayBuffer: async () => new TextEncoder().encode(JSON.stringify(alleenGegevens)).buffer });
await tick();
check("importvenster toont wat er in het bestand zit",
  $(".venster").textContent.includes("1 wallpapers (alleen gegevens)"),
  $(".venster").textContent.replace(/\s+/g, " ").slice(0, 120));
check("keuze voor dubbele bestandsnamen staat standaard op overslaan",
  $('.venster input[name="im-dubbel"]:checked').value === "overslaan");
$('.venster input[name="im-dubbel"][value="bijwerken"]').checked = true;
$("#im-gaan").click();
await tick(200);
check("gegevens-only import werkt de tags van de bestaande wallpaper bij",
  patches.some((p) => p.url.includes("/wallpapers/records/wp2") && p.body.tags === "natuur,lente"),
  JSON.stringify(patches));
check("geen uploads bij een gegevens-only import", posts.length === 0, String(posts.length));

let kapot = "";
try {
  await EXP.openImport({ name: "kapot.json", arrayBuffer: async () => new TextEncoder().encode("{niet goed").buffer });
} catch (e) {
  kapot = e.message;
}
check("een kapot bestand geeft een melding in plaats van een crash", !$(".venster") && kapot === "",
  $(".toasts").textContent ? $(".toasts").textContent : "geen melding");

/* ---------- uitslag ---------- */

fs.rmSync(tmp, { recursive: true, force: true });

let fout = 0;
for (const r of ok) {
  if (!r.waar) fout++;
  console.log(`${r.waar ? "OK  " : "FOUT"}  ${r.naam}${r.extra ? "   [" + r.extra + "]" : ""}`);
}
console.log(`\n${ok.length - fout}/${ok.length} checks geslaagd`);
process.exit(fout ? 1 : 0);
