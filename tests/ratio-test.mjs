/*
 * Headless test van de vaste beeldverhouding-filters (jsdom).
 *
 *   npm install && node tests/ratio-test.mjs
 *
 * Gedrag dat hier vastligt:
 *  - alleen groepen die echt voorkomen krijgen een knop (geen lege knoppen)
 *  - één verhouding tegelijk; nogmaals klikken zet het filter uit
 *  - zodra er gefilterd wordt verschijnt een "filter wissen"-knop
 *  - gekantelde resoluties (9:16, 2:1, 4:3) horen NIET bij de liggende groep
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

const maten = [
  [2912, 1632],   // R1 — 1,784 -> 16:9 (iets breder dan exact: zelfde groep)
  [3440, 1440],   // R2 — 2,389 -> 21:9
  [2400, 1080],   // R3 — 2,222 -> 21:9
  [1000, 1000],   // R4 — 1,000 -> 1:1
  [2000, 1000],   // R5 — 2,000 -> overig (2:1 is geen 1:2, geen 16:9)
  [1080, 1920],   // R6 — 0,5625 -> overig (gekanteld: hoort NIET bij 16:9)
  [1600, 1200],   // R7 — 1,333 -> overig (4:3)
];
const RECORDS = maten.map(([w, h], i) => ({
  id: String(i + 1).repeat(15),
  collectionId: "c1",
  collectionName: "wallpapers",
  title: `R${i + 1} een behoorlijk lange titel om de weergave op 1 regel te testen`,
  tags: i === 0 ? "natuur" : "",
  width: w,
  height: h,
  bytes: 1000,
  file: `r${i + 1}.jpg`,
  created: `2026-11-14 0${i}:00:00.000Z`,
}));

const dom = new JSDOM(html, { runScripts: "outside-only", url: "http://wallpaparr.test/" });
const w = dom.window;
w.open = () => null;
w.fetch = async () => ({ ok: true, status: 200, json: async () => ({ items: RECORDS, totalPages: 1 }) });

w.eval(appjs);
const tick = () => new Promise((r) => setTimeout(r, 30));
await tick();

const $ = (s) => w.document.querySelector(s);
const q = (s) => [...w.document.querySelectorAll(s)];
const ratio = (key) => q("#ratios .ratio").find((b) => b.textContent.startsWith(key));
const aantal = (key) => { const b = ratio(key); return b ? b.querySelector(".n").textContent : "(knop ontbreekt)"; };
const wisKnop = () => q("#ratios .ratio.clear")[0];
const titels = () => q(".card .title").map((t) => t.textContent.split(" ")[0]).sort().join(",");
const ok = [];
const check = (naam, waar, extra = "") => ok.push({ naam, waar, extra });

/* ---------- alleen aanwezige groepen ---------- */
check("knoppen alleen voor groepen die voorkomen (1:1, 16:9, 21:9 + overig)",
  ["1:1", "16:9", "21:9", "overig"].every((k) => !!ratio(k)), String(q("#ratios .ratio").length) + " knoppen");
check("lege groepen hebben geen knop", !ratio("1:2") && !ratio("2:3") && !ratio("27:9"));
check("2912x1632 valt in 16:9", aantal("16:9") === "1", "16:9=" + aantal("16:9"));
check("3440x1440 + 2400x1080 vallen in 21:9", aantal("21:9") === "2", "21:9=" + aantal("21:9"));
check("1000x1000 valt in 1:1", aantal("1:1") === "1", "1:1=" + aantal("1:1"));
check("gekantelde 9:16 hoort NIET bij 16:9", aantal("16:9") === "1", "16:9=" + aantal("16:9"));
check("2:1 en 4:3 vallen onder overig", aantal("overig") === "3", "overig=" + aantal("overig"));
check("zonder filter geen wis-knop", !wisKnop());
check("zonder filter alles zichtbaar (7)", q(".card").length === 7, String(q(".card").length));
check("titel is 1 regel met ellips (css)",
  /\.title\s*\{[^}]*white-space:\s*nowrap/.test(css) && /\.title\s*\{[^}]*text-overflow:\s*ellipsis/.test(css));
check("volledige titel staat in het title-attribuut (hover)", (() => {
  const t = q(".card .title")[0];
  return !!t.getAttribute("title") && t.getAttribute("title") === t.textContent;
})(), q(".card .title")[0].getAttribute("title").slice(0, 30) + "…");

/* ---------- één tegelijk + wissen ---------- */
ratio("16:9").click();
check("16:9-filter toont alleen R1", q(".card").length === 1 && titels() === "R1", titels());
check("knop gemarkeerd als actief", ratio("16:9").classList.contains("on"));
check("wis-knop verschijnt bij een actief filter", !!wisKnop());

ratio("21:9").click();
check("21:9 vervangt 16:9 (single-select)", q(".card").length === 2 && titels() === "R2,R3", titels());
check("16:9 is niet meer actief", !ratio("16:9").classList.contains("on"));

ratio("21:9").click();
check("nogmaals klikken zet het filter uit", q(".card").length === 7 && !wisKnop(), String(q(".card").length));

ratio("overig").click();
check("overig toont de 3 uitschieters", q(".card").length === 3 && titels() === "R5,R6,R7", titels());
wisKnop().click();
check("'filter wissen' toont alles en haalt de knop weg", q(".card").length === 7 && !wisKnop() && !ratio("overig").classList.contains("on"),
  String(q(".card").length));

/* ---------- combineren met zoeken en tagfilters ---------- */
ratio("16:9").click();
$("#search").value = "R1";
$("#search").dispatchEvent(new w.Event("input"));
check("zoekbalk werkt samen met verhoudingfilter", q(".card").length === 1 && titels() === "R1", titels());
$("#search").value = "";
$("#search").dispatchEvent(new w.Event("input"));

q("#tags .chip").find((c) => c.textContent.startsWith("natuur")).click();
check("tagfilter + verhoudingfilter combineren (alleen R1 heeft beide)", q(".card").length === 1 && titels() === "R1", titels());

let fout = 0;
for (const r of ok) {
  if (!r.waar) fout++;
  console.log(`${r.waar ? "OK  " : "FOUT"}  ${r.naam}${r.extra ? "   [" + r.extra + "]" : ""}`);
}
console.log(`\n${ok.length - fout}/${ok.length} checks geslaagd`);
process.exit(fout ? 1 : 0);
