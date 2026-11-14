/*
 * Headless test van de vaste beeldverhouding-filters (jsdom).
 *
 * Controleert het groeperen (kleine verschillen in dezelfde groep), de vaste
 * knoppenset, de tellingen per groep en het filtergedrag.
 *
 *   npm install && node tests/ratio-test.mjs
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

/* Fixture met echte maten. De AI-wallpapers van de gebruiker zijn 2912x1632 —
   dat moet in de 16:9-groep vallen en niet een eigen knop krijgen. */
const maten = [
  [2912, 1632],   // R1 — 1,784  -> 16:9 (iets breder dan exact, zelfde groep)
  [3440, 1440],   // R2 — 2,389  -> 21:9 (ultrawide)
  [2400, 1080],   // R3 — 2,222  -> 21:9
  [1000, 1000],   // R4 — 1,000  -> 1:1
  [2000, 1000],   // R5 — 2,000  -> 1:2 (2:1 is dezelfde verhouding, gedraaid)
  [1080, 1920],   // R6 — 0,5625 -> 16:9 (portret 9:16)
  [1600, 1200],   // R7 — 1,333  -> overig (4:3 komt bij geen groep in de buurt)
];
const RECORDS = maten.map(([w, h], i) => ({
  id: String(i + 1).repeat(15),
  collectionId: "c1",
  collectionName: "wallpapers",
  title: `R${i + 1}`,
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
const aantal = (key) => {
  const b = ratio(key);
  return b ? b.querySelector(".n").textContent : "(knop ontbreekt)";
};
const titels = () => q(".card .title").map((t) => t.textContent).sort().join(",");
const ok = [];
const check = (naam, waar, extra = "") => ok.push({ naam, waar, extra });

/* ---------- vaste knoppen + tellingen ---------- */
const vast = ["1:1", "1:2", "2:3", "16:9", "21:9", "27:9"];
check("de 6 vaste verhoudingknoppen staan er", vast.every((k) => !!ratio(k)), String(q("#ratios .ratio").length) + " knoppen totaal");
check("zoekbalk smaller (max-width 300px in css)", /\.bar \.input\[type="search"\][^}]*max-width:\s*300px/.test(css));
check("label 'verhouding' aanwezig", $("#ratios .ratio-label").textContent === "verhouding");
check("2912x1632 valt in 16:9 (geen eigen knop)", aantal("16:9") === "2", "16:9=" + aantal("16:9") + " (R1 + R6-portret)");
check("3440x1440 + 2400x1080 vallen in 21:9", aantal("21:9") === "2", "21:9=" + aantal("21:9"));
check("1000x1000 valt in 1:1", aantal("1:1") === "1", "1:1=" + aantal("1:1"));
check("lege groepen tonen 0", aantal("2:3") === "0" && aantal("27:9") === "0",
  `2:3=${aantal("2:3")} 27:9=${aantal("27:9")}`);
check("2:1 panorama valt in de 1:2-groep (zelfde verhouding, gedraaid)", aantal("1:2") === "1", "1:2=" + aantal("1:2"));
check("4:3 valt buiten de standaardgroepen -> 'overig'", !!ratio("overig") && aantal("overig") === "1", "overig=" + aantal("overig"));
check("tooltip noemt ook de gedraaide verhouding", /2:1/.test(ratio("1:2").title), ratio("1:2").title);
check("geen filter actief => alle 7 zichtbaar", q(".card").length === 7, String(q(".card").length));

/* ---------- filteren ---------- */
ratio("16:9").click();
check("16:9-filter toont R1 + R6", q(".card").length === 2 && titels() === "R1,R6", titels());
check("knop gemarkeerd als actief", ratio("16:9").classList.contains("on"));

ratio("21:9").click();
check("16:9 + 21:9 samen => 4", q(".card").length === 4, titels());

ratio("overig").click();
check("ook overig erbij => 5", q(".card").length === 5, titels());

ratio("16:9").click();
check("16:9 uit => 3 over (21:9 + overig actief)", q(".card").length === 3 && titels() === "R2,R3,R7", titels());

/* alleen een groep zonder treffers */
ratio("21:9").click();
ratio("overig").click();
check("21:9 + overig uit => 7", q(".card").length === 7, String(q(".card").length));
ratio("27:9").click();
check("filter zonder treffers toont niets", q(".card").length === 0, String(q(".card").length));
check("lege-staat-melding zichtbaar", !$("#empty").classList.contains("hidden"));

/* ---------- combineren met zoeken en tagfilters ---------- */
ratio("27:9").click();
check("alle filters uit => 7", q(".card").length === 7, String(q(".card").length));

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
