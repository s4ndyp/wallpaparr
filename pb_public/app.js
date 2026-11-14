/* Wallpaparr web-UI — praat rechtstreeks met de publieke PocketBase-API. */
const COL = "/api/collections/wallpapers/records";
const $ = (s) => document.querySelector(s);

/* Vaste filters op beeldverhouding. Kleine verschillen (bv. 2912x1632 naast
   echt 16:9) horen in dezelfde groep: vergelijk logaritmisch met een tolerantie. */
const RATIOS = [
  { key: "1:1", v: 1 },
  { key: "1:2", v: 0.5 },
  { key: "2:3", v: 2 / 3 },
  { key: "16:9", v: 16 / 9 },
  { key: "21:9", v: 21 / 9 },
  { key: "27:9", v: 27 / 9 },
];
const RATIO_TOL = 0.10; // 10% afwijking valt nog in dezelfde groep

const state = {
  items: [],
  selected: new Set(),
  activeTags: new Set(),
  ratio: null, // verhoudingfilter: altijd maar één tegelijk
  q: "",
  sort: "-created",
  selectMode: false,
};

/* ---------- helpers ---------- */

function toast(msg, kind = "") {
  const el = document.createElement("div");
  el.className = "toast " + kind;
  el.textContent = msg;
  $("#toasts").appendChild(el);
  setTimeout(() => el.remove(), 4200);
}

function fileURL(rec, filename, thumb) {
  const u = `/api/files/${rec.collectionId}/${rec.id}/${encodeURIComponent(filename)}`;
  return thumb ? `${u}?thumb=${thumb}` : u;
}

function fileSize(n) {
  if (!n) return "";
  const mb = n / 1048576;
  return mb >= 1 ? mb.toFixed(1) + " MB" : Math.round(n / 1024) + " KB";
}

function tagList(s) {
  return (s || "")
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
}

/* In welke verhoudinggroep valt deze wallpaper? 'overig' als niets in de buurt komt.
   NIET symmetrisch: een gekantelde resolutie (9:16, 9:21) hoort bewust niet bij de
   liggende groep — die krijgt een eigen knop zodra zulke wallpapers bestaan. */
function bucketOf(r) {
  if (!r || !r.width || !r.height) return null;
  const x = r.width / r.height;
  let best = null;
  let bestD = Infinity;
  for (const t of RATIOS) {
    const d = Math.abs(Math.log(x / t.v));
    if (d < bestD) {
      bestD = d;
      best = t.key;
    }
  }
  return bestD <= RATIO_TOL ? best : "overig";
}

function prettify(name) {
  return name
    .replace(/\.[a-z0-9]+$/i, "")
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/^\w/, (c) => c.toUpperCase());
}

/* Afmetingen van een afbeelding uitlezen vóór het uploaden. */
function dimensions(file) {
  return new Promise((resolve) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      URL.revokeObjectURL(url);
      resolve({ width: img.naturalWidth, height: img.naturalHeight });
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      resolve({ width: 0, height: 0 });
    };
    img.src = url;
  });
}

/* ---------- data ---------- */

async function fetchAll() {
  const out = [];
  let page = 1;
  for (;;) {
    const r = await fetch(`${COL}?page=${page}&perPage=200&sort=-created`);
    if (!r.ok) throw new Error(`Server antwoordde ${r.status}`);
    const data = await r.json();
    out.push(...(data.items || []));
    if (page >= (data.totalPages || 1)) break;
    page++;
  }
  return out;
}

async function load() {
  try {
    state.items = await fetchAll();
    // selectie opschonen: verwijderde records mogen niet in de selectie blijven hangen
    const ids = new Set(state.items.map((r) => r.id));
    for (const id of [...state.selected]) if (!ids.has(id)) state.selected.delete(id);
    render();
  } catch (e) {
    toast(e.message, "err");
  }
}

/* ---------- render ---------- */

function visibleItems() {
  const q = state.q.toLowerCase();
  let list = state.items.filter((r) => {
    if (state.ratio && bucketOf(r) !== state.ratio) return false;
    const tags = tagList(r.tags);
    for (const t of state.activeTags) {
      if (!tags.some((x) => x.toLowerCase() === t.toLowerCase())) return false;
    }
    if (!q) return true;
    return (
      (r.title || "").toLowerCase().includes(q) ||
      tags.some((t) => t.toLowerCase().includes(q))
    );
  });
  const s = state.sort;
  list.sort((a, b) => {
    if (s === "title") return (a.title || "").localeCompare(b.title || "");
    if (s === "-width") return (b.width || 0) - (a.width || 0);
    const d = (a.created || "").localeCompare(b.created || "");
    return s === "created" ? d : -d;
  });
  return list;
}

function renderTags() {
  const counts = new Map();
  for (const r of state.items) {
    for (const t of tagList(r.tags)) {
      const key = [...counts.keys()].find((k) => k.toLowerCase() === t.toLowerCase()) || t;
      counts.set(key, (counts.get(key) || 0) + 1);
    }
  }
  const sorted = [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  const nav = $("#tags");
  nav.innerHTML = "";
  if (!sorted.length) return;
  const all = document.createElement("span");
  all.className = "chip" + (state.activeTags.size ? "" : " on");
  all.textContent = "alles";
  all.onclick = () => { state.activeTags.clear(); render(); };
  nav.appendChild(all);

  for (const [tag, n] of sorted) {
    const c = document.createElement("span");
    const on = [...state.activeTags].some((t) => t.toLowerCase() === tag.toLowerCase());
    c.className = "chip" + (on ? " on" : "");
    c.innerHTML = `${escapeHtml(tag)}<span class="n">${n}</span>`;
    c.title = on ? "Filter uit" : "Filter op deze tag";
    c.onclick = () => {
      const existing = [...state.activeTags].find((t) => t.toLowerCase() === tag.toLowerCase());
      if (existing) state.activeTags.delete(existing);
      else state.activeTags.add(tag);
      render();
    };
    nav.appendChild(c);
  }
}

/* Verhoudingknoppen achter de zoekbalk: alleen groepen die echt voorkomen,
   één tegelijk te kiezen (geen multi-select) en een wis-knop als er gefilterd wordt. */
function renderRatios() {
  const counts = new Map();
  for (const r of state.items) {
    const b = bucketOf(r);
    if (b) counts.set(b, (counts.get(b) || 0) + 1);
  }
  const nav = $("#ratios");
  nav.innerHTML = "";
  const keys = RATIOS.filter((t) => counts.get(t.key)).map((t) => t.key);
  const heeftOverig = !!counts.get("overig");
  if (!keys.length && !heeftOverig) return; // niets te filteren: geen lege balk

  const label = document.createElement("span");
  label.className = "ratio-label";
  label.textContent = "verhouding";
  nav.appendChild(label);

  for (const key of keys) nav.appendChild(ratioBtn(key, counts.get(key), key));
  if (heeftOverig) nav.appendChild(ratioBtn("overig", counts.get("overig"), "overige verhoudingen"));
  if (state.ratio) nav.appendChild(clearRatioBtn());
}

function ratioBtn(key, n, titel) {
  const on = state.ratio === key;
  const b = document.createElement("button");
  b.type = "button";
  b.className = "ratio" + (on ? " on" : "");
  b.title = on ? `${titel} wissen (je ziet dan alles)` : `Alleen ${titel}`;
  b.innerHTML = `${escapeHtml(key)}<span class="n">${n}</span>`;
  b.onclick = () => {
    state.ratio = on ? null : key; // één tegelijk; nogmaals klikken = uit
    render();
  };
  return b;
}

function clearRatioBtn() {
  const b = document.createElement("button");
  b.type = "button";
  b.className = "ratio clear";
  b.textContent = "× filter wissen";
  b.title = "Verhoudingfilter wissen — je ziet dan alles";
  b.onclick = () => { state.ratio = null; render(); };
  return b;
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

function toggleSelect(id) {
  if (state.selected.has(id)) state.selected.delete(id);
  else state.selected.add(id);
  render();
}

function render() {
  const list = visibleItems();
  $("#count").textContent = state.items.length;
  const allTags = new Set();
  state.items.forEach((r) => tagList(r.tags).forEach((t) => allTags.add(t.toLowerCase())));
  $("#tagcount").textContent = allTags.size;

  // selectieknop
  const sb = $("#selmode");
  sb.classList.toggle("primary", state.selectMode);
  sb.textContent = state.selectMode ? "Selectie uit" : "Selecteren";
  sb.title = state.selectMode
    ? "Selectievakjes uitzetten (klikken op de afbeelding opent weer volledig)"
    : "Selectievakjes aanzetten";

  renderTags();
  renderRatios();

  const grid = $("#grid");
  grid.innerHTML = "";
  $("#empty").classList.toggle("hidden", list.length > 0);

  for (const r of list) {
    const card = document.createElement("div");
    card.className = "card" + (state.selected.has(r.id) ? " sel" : "");

    const thumb = document.createElement("div");
    thumb.className = "thumb" + (state.selectMode ? " pickable" : "");
    const img = document.createElement("img");
    img.loading = "lazy";
    img.src = fileURL(r, r.file, "400x0");
    img.alt = r.title || r.file;
    img.title = state.selectMode ? "Klik om te selecteren" : "Klik om volledig te openen";
    img.onclick = () => {
      if (state.selectMode) toggleSelect(r.id);
      else window.open(fileURL(r, r.file), "_blank");
    };
    thumb.appendChild(img);

    // selectievakje bestaat alleen in selectiemodus
    if (state.selectMode) {
      const chk = document.createElement("input");
      chk.type = "checkbox";
      chk.className = "pick";
      chk.checked = state.selected.has(r.id);
      chk.title = "Selecteren";
      chk.onclick = (e) => { e.stopPropagation(); toggleSelect(r.id); };
      thumb.appendChild(chk);
    }

    if (r.width && r.height) {
      const res = document.createElement("span");
      res.className = "res";
      res.textContent = `${r.width}×${r.height}`;
      thumb.appendChild(res);
    }
    card.appendChild(thumb);

    const meta = document.createElement("div");
    meta.className = "meta";

    const title = document.createElement("div");
    title.className = "title";
    title.textContent = r.title || r.file;
    title.title = r.title || r.file; // weergave is 1 regel; volledige titel bij hover
    meta.appendChild(title);

    const tagsrow = document.createElement("div");
    tagsrow.className = "tagsrow";
    for (const t of tagList(r.tags)) {
      const m = document.createElement("span");
      m.className = "mini";
      const label = document.createElement("span");
      label.textContent = t;
      label.title = "Filteren op deze tag";
      label.onclick = () => { state.activeTags.add(t); render(); };
      const x = document.createElement("span");
      x.className = "x";
      x.textContent = "×";
      x.title = `Tag "${t}" van deze wallpaper verwijderen`;
      x.onclick = (e) => { e.stopPropagation(); removeTag(r, t); };
      m.append(label, x);
      tagsrow.appendChild(m);
    }
    const add = document.createElement("span");
    add.className = "mini add";
    add.textContent = "+ tag";
    add.onclick = () => editTags(r);
    tagsrow.appendChild(add);
    meta.appendChild(tagsrow);

    const foot = document.createElement("div");
    foot.className = "rowfoot";
    const info = document.createElement("span");
    info.className = "grow";
    info.textContent = [fileSize(r.bytes), (r.created || "").slice(0, 10)].filter(Boolean).join(" · ");
    foot.appendChild(info);

    foot.appendChild(iconBtn("URL", "Kopieer directe link", async () => {
      try {
        await navigator.clipboard.writeText(new URL(fileURL(r, r.file), location.origin).href);
        toast("Link gekopieerd", "ok");
      } catch {
        toast("Kopiëren mislukt", "err");
      }
    }));
    foot.appendChild(iconBtn("↓", "Downloaden", () => {
      const a = document.createElement("a");
      a.href = fileURL(r, r.file);
      a.download = r.file;
      a.click();
    }));
    foot.appendChild(iconBtn("✎", "Titel/tags aanpassen", () => editTags(r, true)));
    foot.appendChild(iconBtn("✕", "Verwijderen", () => removeRecord(r), "del"));
    meta.appendChild(foot);

    card.appendChild(meta);
    grid.appendChild(card);
  }

  // bulkbalk hoort bij de selectiemodus
  $("#bulk").classList.toggle("hidden", !state.selectMode);
  $("#selcount").textContent = state.selected.size;
  const none = state.selected.size === 0;
  for (const id of ["applytag", "deltag", "delsel", "clearsel"]) $("#" + id).disabled = none;
}

function iconBtn(label, title, fn, extra = "") {
  const b = document.createElement("button");
  b.className = "icon " + extra;
  b.textContent = label;
  b.title = title;
  b.onclick = fn;
  return b;
}

/* ---------- acties ---------- */

async function editTags(rec, alsoTitle = false) {
  const current = tagList(rec.tags).join(", ");
  const tags = prompt("Tags (kommagescheiden):", current);
  if (tags === null) return;
  const body = { tags: tagList(tags).join(",") };
  if (alsoTitle) {
    const t = prompt("Titel:", rec.title || "");
    if (t !== null && t.trim()) body.title = t.trim();
  }
  await patch(rec.id, body);
}

async function patch(id, body) {
  const r = await fetch(`${COL}/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok) return toast("Opslaan mislukt (" + r.status + ")", "err");
  toast("Opgeslagen", "ok");
  load();
}

/* Eén tag van één wallpaper halen. */
async function removeTag(rec, tag) {
  const rest = tagList(rec.tags).filter((t) => t.toLowerCase() !== tag.toLowerCase());
  const r = await fetch(`${COL}/${rec.id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tags: rest.join(",") }),
  });
  if (!r.ok) return toast("Tag verwijderen mislukt (" + r.status + ")", "err");
  toast(`Tag "${tag}" verwijderd`, "ok");
  load();
}

async function removeRecord(rec) {
  if (!confirm(`"${rec.title || rec.file}" verwijderen?`)) return;
  const r = await fetch(`${COL}/${rec.id}`, { method: "DELETE" });
  if (!r.ok) return toast("Verwijderen mislukt (" + r.status + ")", "err");
  state.selected.delete(rec.id);
  toast("Verwijderd", "ok");
  load();
}

async function uploadOne(file, title, tags, onStatus) {
  const { width, height } = await dimensions(file);
  const fd = new FormData();
  fd.set("title", title || prettify(file.name));
  fd.set("tags", tagList(tags).join(","));
  fd.set("width", String(width));
  fd.set("height", String(height));
  fd.set("bytes", String(file.size));
  fd.set("file", file);
  onStatus("uploaden…");
  const r = await fetch(COL, { method: "POST", body: fd });
  if (!r.ok) {
    let msg = "HTTP " + r.status;
    try {
      const j = await r.json();
      msg = j.message || JSON.stringify(j.data || j);
    } catch {}
    onStatus("mislukt: " + msg);
    return false;
  }
  onStatus("klaar");
  return true;
}

/* Uploadvenster: per bestand een titel, één tagveld voor alle bestanden. */
function openUpload(files) {
  const wrap = document.createElement("div");
  wrap.className = "drop";
  wrap.style.cssText = "flex-direction:column;gap:14px;padding:24px;overflow:auto;background:rgba(11,13,18,.94)";
  wrap.innerHTML = `
    <div style="width:min(680px,100%)">
      <h2 style="margin-bottom:12px;font-size:18px">Uploaden (${files.length})</h2>
      <div style="display:flex;gap:10px;margin-bottom:14px">
        <input id="upTags" class="input" style="flex:1" placeholder="tags voor alle bestanden, kommagescheiden">
      </div>
      <div id="upList" style="display:flex;flex-direction:column;gap:10px;max-height:44vh;overflow:auto"></div>
      <div style="display:flex;gap:10px;margin-top:16px;justify-content:flex-end">
        <button id="upCancel" class="btn">Annuleren</button>
        <button id="upGo" class="btn primary">Uploaden</button>
      </div>
    </div>`;

  const list = wrap.querySelector("#upList");
  const rows = [];
  for (const f of files) {
    const row = document.createElement("div");
    row.style.cssText = "display:flex;gap:10px;align-items:center;background:var(--bg-2);border:1px solid var(--line);border-radius:10px;padding:8px";
    const img = document.createElement("img");
    img.src = URL.createObjectURL(f);
    img.style.cssText = "width:64px;height:44px;object-fit:cover;border-radius:6px";
    const col = document.createElement("div");
    col.style.cssText = "flex:1;display:flex;flex-direction:column;gap:4px";
    const ti = document.createElement("input");
    ti.className = "input";
    ti.value = prettify(f.name);
    const st = document.createElement("span");
    st.style.cssText = "font-size:12px;color:var(--dim)";
    st.textContent = `${f.name} · ${fileSize(f.size)}`;
    col.append(ti, st);
    row.append(img, col);
    list.appendChild(row);
    rows.push({ file: f, input: ti, status: st });
  }

  wrap.querySelector("#upCancel").onclick = () => wrap.remove();
  wrap.querySelector("#upGo").onclick = async () => {
    const tags = wrap.querySelector("#upTags").value;
    const btn = wrap.querySelector("#upGo");
    btn.disabled = true;
    let ok = 0;
    for (const r of rows) {
      const done = await uploadOne(r.file, r.input.value.trim(), tags, (s) => (r.status.textContent = s));
      if (done) ok++;
    }
    wrap.remove();
    toast(`${ok}/${rows.length} geüpload`, ok ? "ok" : "err");
    load();
  };
  document.body.appendChild(wrap);
}

/* ---------- events ---------- */

$("#pick").onclick = () => $("#file").click();
$("#file").onchange = (e) => {
  if (e.target.files.length) openUpload([...e.target.files]);
  e.target.value = "";
};
$("#refresh").onclick = () => load();
$("#search").oninput = (e) => { state.q = e.target.value; render(); };
$("#sort").onchange = (e) => { state.sort = e.target.value; render(); };
$("#clearsel").onclick = () => { state.selected.clear(); render(); };

$("#selmode").onclick = () => {
  state.selectMode = !state.selectMode;
  if (!state.selectMode) state.selected.clear(); // uitzetten = selectie wissen
  render();
};

$("#applytag").onclick = async () => {
  const t = $("#bulktag").value.trim();
  if (!t) return toast("Vul een tag in", "err");
  let n = 0;
  for (const id of state.selected) {
    const rec = state.items.find((r) => r.id === id);
    if (!rec) continue;
    const tags = tagList(rec.tags);
    if (tags.some((x) => x.toLowerCase() === t.toLowerCase())) continue;
    tags.push(t);
    await fetch(`${COL}/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ tags: tags.join(",") }),
    });
    n++;
  }
  $("#bulktag").value = "";
  toast(n ? `Tag "${t}" bij ${n} wallpaper(s) gezet` : `Tag "${t}" stond er al`, n ? "ok" : "err");
  load();
};

$("#deltag").onclick = async () => {
  const t = $("#bulktag").value.trim();
  if (!t) return toast("Vul een tag in", "err");
  let n = 0;
  for (const id of state.selected) {
    const rec = state.items.find((r) => r.id === id);
    if (!rec) continue;
    const tags = tagList(rec.tags);
    const rest = tags.filter((x) => x.toLowerCase() !== t.toLowerCase());
    if (rest.length === tags.length) continue;
    await fetch(`${COL}/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ tags: rest.join(",") }),
    });
    n++;
  }
  $("#bulktag").value = "";
  toast(n ? `Tag "${t}" bij ${n} wallpaper(s) verwijderd` : `Tag "${t}" kwam niet voor`, n ? "ok" : "err");
  load();
};

$("#delsel").onclick = async () => {
  if (!confirm(`${state.selected.size} wallpapers verwijderen?`)) return;
  for (const id of state.selected) await fetch(`${COL}/${id}`, { method: "DELETE" });
  state.selected.clear();
  toast("Verwijderd", "ok");
  load();
};

/* slepen vanaf het bureaublad */
let dragDepth = 0;
window.addEventListener("dragenter", (e) => {
  e.preventDefault();
  if (++dragDepth === 1) $("#drop").classList.remove("hidden");
});
window.addEventListener("dragover", (e) => e.preventDefault());
window.addEventListener("dragleave", () => {
  if (--dragDepth <= 0) { dragDepth = 0; $("#drop").classList.add("hidden"); }
});
window.addEventListener("drop", (e) => {
  e.preventDefault();
  dragDepth = 0;
  $("#drop").classList.add("hidden");
  const files = [...e.dataTransfer.files].filter((f) => f.type.startsWith("image/"));
  if (files.length) openUpload(files);
  else toast("Geen afbeeldingen herkend", "err");
});

/* Escape zet de selectiemodus uit */
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && state.selectMode) {
    state.selectMode = false;
    state.selected.clear();
    render();
  }
});

load();
