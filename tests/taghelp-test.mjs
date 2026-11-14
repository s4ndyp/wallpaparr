/*
 * Headless test van de taghulp (voorstellen bij het typen) en het tagvenster.
 *
 *   npm install && node tests/taghelp-test.mjs
 *
 * Wat hier vastligt:
 *  - suggesties komen uit de bestaande tags en matchen op de letters die je typt
 *  - een gekozen suggestie vult het HUIDIGE stuk na de laatste komma in en zet er
 *    een komma achter, zodat je meteen door kunt typen
 *  - tags die al in het veld staan worden niet voorgesteld
 *  - pijltjes + Enter kiezen, Escape sluit
 *  - het tagvenster van een wallpaper stuurt de nieuwe tags als PATCH weg
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

const RECORDS = [
  { id: "1".repeat(15), collectionId: "c1", collectionName: "wallpapers", title: "A", tags: "natuur,nacht", width: 2912, height: 1632, file: "a.jpg", created: "2026-11-14 01:00:00.000Z" },
  { id: "2".repeat(15), collectionId: "c1", collectionName: "wallpapers", title: "B", tags: "natuurgebied,abstract", width: 3360, height: 1440, file: "b.jpg", created: "2026-11-14 02:00:00.000Z" },
  { id: "3".repeat(15), collectionId: "c1", collectionName: "wallpapers", title: "C", tags: "Ai", width: 3808, height: 1280, file: "c.jpg", created: "2026-11-14 03:00:00.000Z" },
];

const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://wallpaparr.test/" });
const w = dom.window;
w.open = () => null;
const patches = [];
w.fetch = async (url, opt = {}) => {
  if (opt && opt.method === "PATCH") {
    patches.push({ url, body: JSON.parse(opt.body) });
    return { ok: true, status: 200, json: async () => ({}) };
  }
  return { ok: true, status: 200, json: async () => ({ items: RECORDS, totalPages: 1 }) };
};

w.eval(appjs);
const tick = () => new Promise((r) => setTimeout(r, 30));
await tick();

const $ = (s) => w.document.querySelector(s);
const q = (s) => [...w.document.querySelectorAll(s)];
const box = () => $("#taghulp");
const items = () => [...box().children].map((d) => d.textContent);
const typ = (input, waarde) => {
  input.value = waarde;
  input.dispatchEvent(new w.Event("input", { bubbles: true }));
};
const toets = (input, key) => input.dispatchEvent(new w.KeyboardEvent("keydown", { key, bubbles: true }));
const ok = [];
const check = (naam, waar, extra = "") => ok.push({ naam, waar, extra });

/* ---------- suggesties in het bulkveld ---------- */
const bulk = $("#bulktag");
check("bulkveld is gekoppeld aan de taghulp", bulk.dataset.taghulp === "1");

typ(bulk, "na");
check("suggesties bij 'na'", items().join(",") === "nacht,natuur,natuurgebied", items().join(",") || "(leeg)");
check("hulplijst is zichtbaar", !box().classList.contains("hidden"));

/* ---------- suggestie kiezen vult het huidige stuk in ---------- */
q("#taghulp div").find((d) => d.textContent === "natuur").onmousedown(new w.MouseEvent("mousedown", { bubbles: true }));
check("gekozen tag + komma in het veld", bulk.value === "natuur, ", JSON.stringify(bulk.value));
check("die tag staat niet meer in de suggesties", !items().includes("natuur"), items().join(","));

/* ---------- meerdere tags: alleen het laatste stuk wordt aangevuld ---------- */
typ(bulk, "test, na");
check("suggesties kijken naar het stuk na de komma", items().includes("natuur") && items().includes("nacht"), items().join(","));
q("#taghulp div").find((d) => d.textContent === "natuurgebied").onmousedown(new w.MouseEvent("mousedown", { bubbles: true }));
check("bestaande tekst blijft staan, nieuwe tag erachter", bulk.value === "test, natuurgebied, ", JSON.stringify(bulk.value));

/* ---------- al gebruikte tags niet voorstellen ---------- */
typ(bulk, "natuur, ");
check("al aanwezige tag wordt niet voorgesteld", !items().includes("natuur") && items().includes("nacht"), items().join(","));

/* ---------- pijltjes + Enter ---------- */
typ(bulk, "na");
toets(bulk, "ArrowDown");
toets(bulk, "ArrowDown");
toets(bulk, "Enter");
check("pijltjes + Enter kiest de tweede suggestie", bulk.value === "natuur, ", JSON.stringify(bulk.value));

/* ---------- Escape sluit de lijst ---------- */
typ(bulk, "na");
toets(bulk, "Escape");
check("Escape verbergt de lijst", box().classList.contains("hidden"));

/* ---------- tagvenster van een wallpaper ---------- */
const kaart = q(".card").find((c) => c.querySelector(".title").textContent === "A");
kaart.querySelector(".tagsrow .mini.add").click();
const veld = $("#etTags");
check("tagvenster opent met de huidige tags", !!veld && veld.value === "natuur, nacht", veld ? veld.value : "(geen veld)");
check("tagveld in het venster heeft de hulp", veld.dataset.taghulp === "1");

typ(veld, "natuur, nacht, abs");
check("suggesties werken in het venster", items().includes("abstract"), items().join(","));
q("#taghulp div").find((d) => d.textContent === "abstract").onmousedown(new w.MouseEvent("mousedown", { bubbles: true }));
$("#etSave").click();
await tick();
check("opslaan stuurt een PATCH", patches.length === 1, JSON.stringify(patches[0] || {}));
check("PATCH heeft de nieuwe tags", patches[0] && patches[0].body.tags === "natuur,nacht,abstract",
  patches[0] ? patches[0].body.tags : "-");
check("venster is gesloten", !$("#etTags"));

/* ---------- Escape sluit een venster ---------- */
kaart.querySelector(".tagsrow .mini.add").click();
check("venster weer open", !!$("#etTags"));
w.document.dispatchEvent(new w.KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
check("Escape sluit het venster", !$("#etTags"));

let fout = 0;
for (const r of ok) {
  if (!r.waar) fout++;
  console.log(`${r.waar ? "OK  " : "FOUT"}  ${r.naam}${r.extra ? "   [" + r.extra + "]" : ""}`);
}
console.log(`\n${ok.length - fout}/${ok.length} checks geslaagd`);
process.exit(fout ? 1 : 0);
