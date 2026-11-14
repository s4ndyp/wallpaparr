/*
 * Headless test van de Wallpaparr web-UI (jsdom).
 *
 * Draait zonder server: de PocketBase-API wordt nagebootst. Controleert de
 * interactielogica die je met een syntaxcheck niet vangt — selectiemodus,
 * klikken op de afbeelding, fullscreen-gedrag en tagverwijdering.
 *
 *   npm install jsdom --no-save && node tests/ui-test.mjs
 *
 * Draait ook in CI (.github/workflows/build.yml, job 'ui-test').
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";

const hier = path.dirname(fileURLToPath(import.meta.url));
const PUB = path.join(hier, "..", "pb_public");

const html = fs.readFileSync(path.join(PUB, "index.html"), "utf8")
  .replace('<script src="/app.js"></script>', "");
const appjs = fs.readFileSync(path.join(PUB, "app.js"), "utf8");
const css = fs.readFileSync(path.join(PUB, "app.css"), "utf8");

const RECORDS = [
  { id: "aaaaaaaaaaaaaaa", collectionId: "c1", collectionName: "wallpapers", title: "Test A", tags: "natuur,nacht", width: 1920, height: 1080, bytes: 1000, file: "a.png", created: "2026-11-14 10:00:00.000Z" },
  { id: "bbbbbbbbbbbbbbb", collectionId: "c1", collectionName: "wallpapers", title: "Test B", tags: "abstract,nacht", width: 2560, height: 1440, bytes: 2000, file: "b.png", created: "2026-11-14 09:00:00.000Z" },
  { id: "ccccccccccccccc", collectionId: "c1", collectionName: "wallpapers", title: "Test C", tags: "abstract", width: 1280, height: 720, bytes: 500, file: "c.png", created: "2026-11-14 08:00:00.000Z" },
];

const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://wallpaparr.test/" });
const w = dom.window;

const patches = [];
const deletes = [];
let opened = 0;
w.open = () => { opened++; return null; };

w.fetch = async (url, opt = {}) => {
  if (opt.method === "PATCH") {
    patches.push({ url, body: JSON.parse(opt.body) });
    return { ok: true, status: 200, json: async () => ({}) };
  }
  if (opt.method === "DELETE") {
    deletes.push(url);
    return { ok: true, status: 204, json: async () => ({}) };
  }
  if (opt.method === "POST") return { ok: true, status: 200, json: async () => ({}) };
  return { ok: true, status: 200, json: async () => ({ items: RECORDS, totalPages: 1, totalItems: RECORDS.length }) };
};

w.eval(appjs);
const tick = () => new Promise((r) => setTimeout(r, 30));
await tick();

const $ = (s) => w.document.querySelector(s);
const q = (s) => [...w.document.querySelectorAll(s)];
const card = (t) => q(".card").find((c) => c.querySelector(".title").textContent === t);
const ok = [];
const check = (naam, waar, extra = "") => ok.push({ naam, waar, extra });

/* ---------- basis ---------- */
check("kaarten gerenderd (3)", q(".card").length === 3, String(q(".card").length));
check("geen selectievakjes zonder selectiemodus", q(".thumb .pick").length === 0, String(q(".thumb .pick").length));
check("bulkbalk verborgen", $("#bulk").classList.contains("hidden"));
check("knoplabel = Selecteren", $("#selmode").textContent === "Selecteren", $("#selmode").textContent);
check("tagchip heeft gouden kleur", css.includes("--tag: #f2b45c"));
check("tagchip heeft verwijderkruisje", !!card("Test A").querySelector(".tagsrow .mini .x"));
check("tagnaam klikbaar (filter)", typeof card("Test A").querySelector(".tagsrow .mini > span").onclick === "function");

/* ---------- selectiemodus aan ---------- */
$("#selmode").click();
check("selectievakjes verschijnen (3)", q(".thumb .pick").length === 3, String(q(".thumb .pick").length));
check("knoplabel = Selectie uit", $("#selmode").textContent === "Selectie uit", $("#selmode").textContent);
check("bulkbalk zichtbaar", !$("#bulk").classList.contains("hidden"));
check("bulkknoppen uitgeschakeld bij 0 selectie", ["applytag", "deltag", "delsel", "clearsel"].every((i) => $("#" + i).disabled));
check("afbeelding is pickable", card("Test A").querySelector(".thumb").classList.contains("pickable"));

/* ---------- klik op de afbeelding = selecteren ---------- */
card("Test A").querySelector(".thumb img").click();
check("klik op afbeelding selecteert", $("#selcount").textContent === "1", $("#selcount").textContent);
check("kaart krijgt .sel", card("Test A").classList.contains("sel"));
check("checkbox aangevinkt", card("Test A").querySelector(".thumb .pick").checked);
check("GEEN fullscreen geopend in selectiemodus", opened === 0, "window.open=" + opened);
check("uitvinken deselecteert", (() => {
  card("Test A").querySelector(".thumb .pick").click();
  return $("#selcount").textContent === "0" && opened === 0;
})());

/* ---------- tag verwijderen via het kruisje ---------- */
patches.length = 0;
card("Test A").querySelector(".tagsrow .mini .x").click();
await tick();
check("tag verwijderen stuurt PATCH", patches.length === 1, JSON.stringify(patches[0] || {}));
check("PATCH houdt overige tags over", patches[0] && patches[0].body.tags === "nacht", JSON.stringify(patches[0] && patches[0].body));

/* ---------- bulk: tag verwijderen ---------- */
$("#selmode").click(); // uit (wist selectie)
$("#selmode").click(); // weer aan
check("selectie gewist bij uitzetten", $("#selcount").textContent === "0");
card("Test A").querySelector(".thumb img").click();
card("Test B").querySelector(".thumb img").click();
check("twee geselecteerd", $("#selcount").textContent === "2", $("#selcount").textContent);
check("bulkknoppen nu actief", ["applytag", "deltag", "delsel", "clearsel"].every((i) => !$("#" + i).disabled));
patches.length = 0;
$("#bulktag").value = "nacht";
$("#deltag").click();
await tick();
check("bulk 'Tag verwijderen' patcht alleen records mét die tag", patches.length === 2, "aantal PATCH=" + patches.length);
check("record zonder die tag wordt overgeslagen",
  patches.filter((p) => p.url.endsWith("aaaaaaaaaaaaaaa")).length === 1 &&
  patches.filter((p) => p.url.endsWith("bbbbbbbbbbbbbbb")).length === 1 &&
  patches.filter((p) => p.url.endsWith("ccccccccccccccc")).length === 0,
  patches.map((p) => p.url.split("/").pop()).join(","));

/* ---------- bulk: tag toepassen en wallpapers verwijderen ---------- */
patches.length = 0;
$("#bulktag").value = "nieuw";
$("#applytag").click();
await tick();
check("bulk 'Tag toepassen' zet de tag bij beide", patches.length === 2 && patches.every((p) => p.body.tags.includes("nieuw")), "aantal PATCH=" + patches.length);

/* ---------- Escape zet de modus uit ---------- */
$("#selmode").click();
$("#selmode").click();
w.document.dispatchEvent(new w.KeyboardEvent("keydown", { key: "Escape" }));
check("Escape stopt selectiemodus", $("#selmode").textContent === "Selecteren" && q(".thumb .pick").length === 0, $("#selmode").textContent);

/* ---------- zonder selectiemodus opent de afbeelding weer volledig ---------- */
card("Test A").querySelector(".thumb img").click();
check("klik op afbeelding opent weer volledig", opened === 1, "window.open=" + opened);

let fout = 0;
for (const r of ok) {
  if (!r.waar) fout++;
  console.log(`${r.waar ? "OK  " : "FOUT"}  ${r.naam}${r.extra ? "   [" + r.extra + "]" : ""}`);
}
console.log(`\n${ok.length - fout}/${ok.length} checks geslaagd`);
process.exit(fout ? 1 : 0);
