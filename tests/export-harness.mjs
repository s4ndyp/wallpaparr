/*
 * Proef van exporteren/importeren tegen een ECHTE PocketBase in plaats van een mock.
 *
 *   cd ~/dev/pb-test && ./pocketbase serve --http=127.0.0.1:8099 --https= --dir=./pb_data
 *   node tests/export-harness.mjs
 *
 * Zet drie wallpapers, een playlist en een scherm neer, exporteert alles als zip, gooit het
 * weer weg en zet het terug — en controleert daarna op de server of tags, playlist en
 * schermkoppeling kloppen. Ruimt zijn eigen records (titel "E2E …") weer op.
 * Draait NIET mee in npm test / CI (vraagt een draaiende server).
 */
import fs from "node:fs";
import { JSDOM } from "jsdom";

const B = "http://127.0.0.1:8099";
const PUB = "/home/sandman/dev/wallpaparr/pb_public";
const png = Uint8Array.from(Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64"));

const api = async (pad, opt = {}) => {
  const r = await fetch(B + pad, opt);
  const tekst = await r.text();
  let data = null;
  try { data = JSON.parse(tekst); } catch { data = tekst; }
  if (!r.ok) throw new Error(pad + " → " + r.status + " " + tekst.slice(0, 200));
  return data;
};

const alles = async (col) => (await api(`/api/collections/${col}/records?perPage=500`)).items;

// ---- opruimen van een eerdere ronde ----
for (const w of await alles("wallpapers")) if (w.title.startsWith("E2E ")) await api(`/api/collections/wallpapers/records/${w.id}`, { method: "DELETE" });
for (const p of await alles("playlists")) if (p.naam.startsWith("E2E")) await api(`/api/collections/playlists/records/${p.id}`, { method: "DELETE" });
for (const s of await alles("screens")) if (s.key.startsWith("E2EPC")) await api(`/api/collections/screens/records/${s.id}`, { method: "DELETE" });

// ---- drie wallpapers + een playlist + een scherm neerzetten ----
const namen = [["E2E een", "natuur,zon", "e2e-een.png"], ["E2E twee", "natuur,nacht", "e2e-twee.png"], ["E2E drie", "abstract", "e2e-drie.png"]];
const ids = [];
for (const [titel, tags, bestand] of namen) {
  const fd = new FormData();
  fd.set("title", titel);
  fd.set("tags", tags);
  fd.set("width", "1");
  fd.set("height", "1");
  fd.set("bytes", String(png.length));
  fd.set("file", new Blob([png], { type: "image/png" }), bestand);
  ids.push((await api("/api/collections/wallpapers/records", { method: "POST", body: fd })).id);
}
const pl = await api("/api/collections/playlists/records", {
  method: "POST", headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ naam: "E2E playlist", tags: "natuur", ratios: "1:1", interval: 7, random: true, actief: true, wallpapers: ids }),
});
const scherm = await api("/api/collections/screens/records", {
  method: "POST", headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ key: "E2EPC|MON|0", device: "E2EPC", monitor: "MON", index: 0, playlist: pl.id }),
});
console.log("neergezet:", ids.length, "wallpapers, playlist", pl.id, "scherm", scherm.id);

// ---- de web-UI in jsdom, met de ECHTE server ----
let html = fs.readFileSync(PUB + "/index.html", "utf8");
for (const s of ["app.js", "screens.js", "export.js"]) html = html.replace(`<script src="/${s}"></script>`, "");
const scripts = ["app.js", "screens.js", "export.js"].map((f) => fs.readFileSync(PUB + "/" + f, "utf8")).join("\n");
const dom = new JSDOM(html, { runScripts: "outside-only", url: B + "/" });
const w = dom.window;
for (const n of ["Blob", "Response", "DecompressionStream", "FormData", "File", "Headers", "Request"]) {
  Object.defineProperty(w, n, { value: globalThis[n], configurable: true, writable: true });
}
const echt = globalThis.fetch;
w.fetch = (url, opt) => echt(typeof url === "string" && url.startsWith("/") ? B + url : url, opt);
let laatsteBlob = null;
w.URL.createObjectURL = (b) => { laatsteBlob = b; return "blob:test"; };
w.URL.revokeObjectURL = () => {};
w.HTMLAnchorElement.prototype.click = function () {};
w.eval(scripts);
await new Promise((r) => setTimeout(r, 800));

const EXP = w.__wpExport;
console.log("items in de UI:", w.__wpState.items.length);

// ---- exporteren (alles, met afbeeldingen) ----
const uitExp = await EXP.exporteer("alles", true, () => {});
const zip = new Uint8Array(await laatsteBlob.arrayBuffer());
fs.writeFileSync("/tmp/e2e-export.zip", zip);
const bundel = await EXP.leesBundel(zip, "e2e.zip");
console.log("geëxporteerd:", uitExp, "zip-bytes:", zip.length,
  "manifest:", bundel.manifest.wallpapers.length, "wallpapers,", bundel.manifest.playlists.length, "playlists,",
  bundel.manifest.schermen.length, "schermen");
const inZip = [...bundel.bestanden.keys()];
const platen = inZip.filter((k) => k.startsWith("files/"));
console.log("afbeeldingen in de zip:", platen.join(", "));
const mijn = platen.filter((k) => /e2e_/.test(k) || /e2e-/.test(k));
if (mijn.length !== 3) { console.log("FOUT: niet alle afbeeldingen in de zip"); process.exit(1); }

// ---- alles weggooien en terugzetten ----
const oud = { wallpapers: (await alles("wallpapers")).length, playlists: (await alles("playlists")).length };
for (const id of ids) await api(`/api/collections/wallpapers/records/${id}`, { method: "DELETE" });
await api(`/api/collections/playlists/records/${pl.id}`, { method: "DELETE" });
console.log("verwijderd; wallpapers nu:", (await alles("wallpapers")).length, "playlists nu:", (await alles("playlists")).length);

const uitImp = await EXP.importeer(bundel, { dubbel: "overslaan" }, () => {});
console.log("import:", JSON.stringify(uitImp));

// ---- controleren op de server ----
const naWp = (await alles("wallpapers")).filter((x) => x.title.startsWith("E2E "));
const naPl = (await alles("playlists")).filter((x) => x.naam === "E2E playlist");
const naScherm = (await alles("screens")).find((x) => x.key === "E2EPC|MON|0");
const tags = Object.fromEntries(naWp.map((x) => [x.title, x.tags]));
console.log("terug op de server:", naWp.length, "wallpapers,", naPl.length, "playlists");
console.log("tags:", JSON.stringify(tags));
console.log("playlist verwijst naar", (naPl[0] || {}).wallpapers, "interval", (naPl[0] || {}).interval);
console.log("scherm hangt aan playlist:", naScherm && naScherm.playlist, "(nieuwe playlist id", naPl[0] && naPl[0].id + ")");

let fout = 0;
const eis = (naam, waar) => { console.log(`${waar ? "OK  " : "FOUT"}  ${naam}`); if (!waar) fout++; };
eis("drie wallpapers terug", naWp.length === 3);
eis("tags kloppen", tags["E2E een"] === "natuur,zon" && tags["E2E twee"] === "natuur,nacht" && tags["E2E drie"] === "abstract");
eis("playlist terug met dezelfde instellingen", naPl.length === 1 && naPl[0].interval === 7 && naPl[0].ratios === "1:1" && naPl[0].wallpapers.length === 3);
eis("scherm hangt weer aan een playlist met dezelfde naam", !!naScherm && naScherm.playlist === naPl[0].id);
eis("afbeeldingen zijn echt geüpload", naWp.every((x) => x.file && /^e2e/i.test(x.file)), JSON.stringify(naWp.map((x) => x.file)));

// ---- schijfruimte-endpoint ----
const disk = await api("/api/wallpaparr/disk");
console.log("disk:", JSON.stringify(disk));
eis("schijfendpoint levert vrije ruimte", disk.beschikbaar === true && disk.vrij > 0 && disk.procent > 0);
eis("schijfendpoint telt de wallpapers", disk.wallpapers >= 3);

// ---- opruimen ----
for (const x of naWp) await api(`/api/collections/wallpapers/records/${x.id}`, { method: "DELETE" });
for (const x of naPl) await api(`/api/collections/playlists/records/${x.id}`, { method: "DELETE" });
if (naScherm) await api(`/api/collections/screens/records/${naScherm.id}`, { method: "DELETE" });
console.log("opgeruimd. eindstand wallpapers:", (await alles("wallpapers")).length, "(was", oud.wallpapers + ")");
console.log(fout ? `\n${fout} FOUTEN` : "\nalles goed");
process.exit(fout ? 1 : 0);
