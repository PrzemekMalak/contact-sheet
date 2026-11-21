"use strict";

// ---------- helpers ----------

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);

const ICONS = {
  x: '<path d="M6 6l12 12M18 6L6 18"/>',
  check: '<path d="M5 12.5l4.5 4.5L19 7.5"/>',
  star: '<path d="M12 3.5l2.6 5.3 5.9.9-4.3 4.1 1 5.8L12 16.9l-5.2 2.7 1-5.8-4.3-4.1 5.9-.9z"/>',
  alert: '<circle cx="12" cy="12" r="9"/><path d="M12 8v5M12 16.5h.01"/>',
  spinner: '<path d="M12 3a9 9 0 1 0 9 9"/>',
  trash: '<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13"/>',
  image: '<rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="M21 16l-5-5-8 8"/>',
  sortDesc: '<path d="M8 4v16M4 16l4 4 4-4M14 6h6M14 11h4M14 16h2"/>',
  sortAsc: '<path d="M8 20V4M4 8l4-4 4 4M14 6h2M14 11h4M14 16h6"/>',
  noAlbum: '<path d="M4 7h16M4 12h10M4 17h6"/>',
};
const icon = (name, cls = "icon") => `<svg class="${cls}" viewBox="0 0 24 24" aria-hidden="true">${ICONS[name]}</svg>`;

async function api(method, url, body) {
  const opts = { method, headers: {} };
  if (body instanceof FormData) opts.body = body;
  else if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(url, opts);
  const text = await res.text();
  let data;
  try { data = JSON.parse(text); } catch { data = text; }
  if (!res.ok) throw new Error((data && data.error) || text.trim() || res.statusText);
  return data;
}

const plural = (n, word) => `${n} ${word}${n === 1 ? "" : "s"}`;

// ---------- preferences (per browser) ----------

function loadPrefs() {
  const p = { sort: { key: "taken", dir: "desc" }, thumb: 180 };
  try {
    const s = JSON.parse(localStorage.getItem("photoSort"));
    if (s && s.key && s.dir) p.sort = s;
    const t = parseInt(localStorage.getItem("photoThumb"), 10);
    if (t >= 100 && t <= 300) p.thumb = t;
  } catch {}
  return p;
}
function savePrefs() {
  try {
    localStorage.setItem("photoSort", JSON.stringify(state.sort));
    localStorage.setItem("photoThumb", String(state.thumb));
  } catch {}
}

// ---------- state ----------

const prefs = loadPrefs();
const state = {
  photos: [], albums: [], tags: [], extraAlbums: new Set(),
  albumTitles: {},        // slug -> title from the album page, when it has one
  view: "all",            // all | unassigned | featured | album
  album: "", tag: "", query: "",
  sort: prefs.sort, thumb: prefs.thumb,
  selected: new Set(), anchor: null,
  order: [],              // ids currently shown in the grid, in display order
  dragIds: null,
};
const byId = (id) => state.photos.find((p) => p.id === id);
const albumsOf = (p) => p.albums || [];
const albumLabel = (slug) => state.albumTitles[slug] || slug;
const tagsOf = (p) => p.tags || [];

// ---------- data ----------

async function load() {
  const d = await api("GET", "/api/photos");
  state.photos = d.photos || [];
  state.tags = d.tags || [];
  state.albumTitles = d.albumTitles || {};
  state.albums = [...new Set([...(d.albums || []), ...state.extraAlbums])].sort((a, b) => a.localeCompare(b));
  const ids = new Set(state.photos.map((p) => p.id));
  for (const id of [...state.selected]) if (!ids.has(id)) state.selected.delete(id);
  if (state.view === "album" && !state.albums.includes(state.album)) state.view = "all";
  if (state.tag && !state.tags.includes(state.tag)) state.tag = "";
  renderAll();
}

async function reload() {
  await load();
  refreshStatus();
}

// Run a mutation; on success reload, on failure show the error.
async function mutate(fn) {
  try { await fn(); return true; }
  catch (e) { toast(esc(e.message), { type: "err" }); return false; }
  finally { await reload().catch(() => {}); }
}

// ---------- filtering & sorting ----------

function matchesQuery(p, q) {
  const e = p.exif || {};
  return [p.id, p.caption, e.camera, e.lens, ...tagsOf(p), ...albumsOf(p)]
    .some((v) => v && String(v).toLowerCase().includes(q));
}

function visible() {
  const q = state.query.trim().toLowerCase();
  return state.photos.filter((p) => {
    if (state.view === "album" && !albumsOf(p).includes(state.album)) return false;
    if (state.view === "unassigned" && albumsOf(p).length) return false;
    if (state.view === "featured" && !p.featured) return false;
    if (state.tag && !tagsOf(p).includes(state.tag)) return false;
    if (q && !matchesQuery(p, q)) return false;
    return true;
  });
}

// Photos with no capture date always sort last (both directions); ties break by id.
function sortList(list) {
  const { key, dir } = state.sort;
  const sign = dir === "asc" ? 1 : -1;
  return list.slice().sort((a, b) => {
    let c;
    if (key === "taken") {
      const av = a.taken || "", bv = b.taken || "";
      if (av !== bv) {
        if (!av) return 1;
        if (!bv) return -1;
        c = av < bv ? -1 : 1;
      } else c = 0;
    } else {
      const av = (a[key] ?? "").toString().toLowerCase();
      const bv = (b[key] ?? "").toString().toLowerCase();
      c = av === bv ? 0 : av < bv ? -1 : 1;
    }
    if (c !== 0) return c * sign;
    return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
  });
}

function aspect(p) {
  const w = p.width || p.pw, h = p.height || p.ph;
  const ar = w && h ? w / h : 1.5;
  return Math.min(Math.max(ar, 0.5), 2.6);
}

function monthLabel(taken) {
  if (!taken) return "No date";
  const [y, m] = taken.split("-").map(Number);
  if (!y || !m) return "No date";
  return new Date(y, m - 1, 1).toLocaleString("en-GB", { month: "long", year: "numeric" });
}

function fmtTaken(taken) {
  if (!taken) return "No capture date";
  const d = new Date(taken);
  if (isNaN(d)) return taken;
  return d.toLocaleString("en-GB", { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });
}

// ---------- rendering ----------

function renderAll() {
  renderSidebar();
  renderToolbar();
  renderGrid();
  renderInspector();
}

function coverFor(slug) {
  // Newest photo in the album stands in for its cover in the sidebar.
  let best = null;
  for (const p of state.photos) {
    if (!albumsOf(p).includes(slug)) continue;
    if (!best || (p.taken || "") > (best.taken || "")) best = p;
  }
  return best ? "/assets/" + best.thumb : "";
}

function renderSidebar() {
  const all = state.photos.length;
  $("n-all").textContent = all;
  $("n-unassigned").textContent = state.photos.filter((p) => !albumsOf(p).length).length;
  $("n-featured").textContent = state.photos.filter((p) => p.featured).length;
  document.querySelectorAll("[data-view]").forEach((a) =>
    a.classList.toggle("active", state.view === a.dataset.view));

  const counts = {};
  for (const p of state.photos) for (const a of albumsOf(p)) counts[a] = (counts[a] || 0) + 1;
  $("album-list").innerHTML = state.albums.map((slug) => {
    const cover = coverFor(slug);
    const active = state.view === "album" && state.album === slug ? " active" : "";
    return `<a href="#" class="nav-item album-item${active}" data-album="${esc(slug)}" title="${esc(slug)}">
      ${cover ? `<img class="cover" src="${esc(cover)}" alt="" loading="lazy">` : '<span class="cover"></span>'}
      <span class="grow">${esc(albumLabel(slug))}</span><span class="adds" hidden></span>
      <span class="count">${counts[slug] || 0}</span></a>`;
  }).join("") || '<div class="muted small" style="padding:4px 10px">No albums yet</div>';

  const tcounts = {};
  for (const p of state.photos) for (const t of tagsOf(p)) tcounts[t] = (tcounts[t] || 0) + 1;
  $("tag-list").innerHTML = state.tags.map((t) =>
    `<a href="#" class="tag-pill${state.tag === t ? " active" : ""}" data-tag="${esc(t)}">${esc(t)} <span class="count">${tcounts[t] || 0}</span></a>`
  ).join("") || '<span class="muted small">No tags yet</span>';
}

function viewTitle() {
  const base = { all: "All photos", unassigned: "Unassigned", featured: "Featured", album: albumLabel(state.album) }[state.view];
  return state.tag ? `${base} · #${state.tag}` : base;
}

function renderToolbar() {
  $("view-title").textContent = viewTitle();
  $("album-actions").hidden = state.view !== "album";
  document.querySelectorAll("[data-sort]").forEach((b) =>
    b.classList.toggle("active", b.dataset.sort === state.sort.key));
  const desc = state.sort.dir === "desc";
  const dirBtn = $("sort-dir");
  dirBtn.innerHTML = icon(desc ? "sortDesc" : "sortAsc");
  const label = state.sort.key === "taken" ? (desc ? "Newest first" : "Oldest first") : (desc ? "Z to A" : "A to Z");
  dirBtn.title = label;
  dirBtn.setAttribute("aria-label", label);
  $("thumb-size").value = state.thumb;
}

function renderCount() {
  const shown = state.order.length, total = state.photos.length, sel = state.selected.size;
  let txt = shown === total ? plural(total, "photo") : `${shown} of ${plural(total, "photo")}`;
  if (sel) txt += ` · ${sel} selected`;
  $("count").textContent = txt;
}

function tileHTML(p) {
  const ar = aspect(p);
  const marks = [];
  if (p.featured) marks.push(`<span class="mark star" title="Featured">${icon("star")}</span>`);
  if (!albumsOf(p).length) marks.push(`<span class="mark" title="Not in any album">${icon("noAlbum")}</span>`);
  return `<button class="tile" draggable="true" data-id="${esc(p.id)}" data-ar="${ar}"
      style="flex:${ar} 1 ${Math.round(ar * state.thumb)}px;aspect-ratio:${ar}" aria-label="${esc(p.caption || p.id)}">
    <img src="/assets/${esc(p.thumb)}" alt="" loading="lazy" draggable="false">
    <span class="shade"></span>
    <span class="check"><svg viewBox="0 0 24 24" aria-hidden="true">${ICONS.check}</svg></span>
    ${marks.length ? `<span class="marks">${marks.join("")}</span>` : ""}
  </button>`;
}

function renderGrid() {
  const list = sortList(visible());
  state.order = list.map((p) => p.id);
  const grid = $("grid");
  if (!list.length) {
    let msg = "No photos here yet.";
    if (state.query) msg = `Nothing matches “${esc(state.query)}”.`;
    else if (state.view === "album") msg = "This album is empty. Drag photos onto it in the sidebar, or drop files here.";
    else if (!state.photos.length) msg = "Drop images anywhere to add your first photos.";
    grid.innerHTML = `<div class="empty-state">${icon("image", "icon xl")}<div>${msg}</div></div>`;
  } else if (state.sort.key === "taken") {
    const groups = [];
    for (const p of list) {
      const label = monthLabel(p.taken);
      if (!groups.length || groups[groups.length - 1].label !== label) groups.push({ label, items: [] });
      groups[groups.length - 1].items.push(p);
    }
    grid.innerHTML = groups.map((g) =>
      `<h2 class="section-label group-label">${esc(g.label)}</h2><div class="rows">${g.items.map(tileHTML).join("")}</div>`
    ).join("");
  } else {
    grid.innerHTML = `<div class="rows" style="padding-top:2px">${list.map(tileHTML).join("")}</div>`;
  }
  renderSelection();
}

// Cheap update after a selection change: classes only, no image reloads.
function renderSelection() {
  $("grid").classList.toggle("selecting", state.selected.size > 0);
  document.querySelectorAll(".tile").forEach((t) => {
    const on = state.selected.has(t.dataset.id);
    t.classList.toggle("sel", on);
    t.setAttribute("aria-pressed", on ? "true" : "false");
  });
  renderCount();
}

function resizeTiles() {
  document.querySelectorAll(".tile").forEach((t) => {
    const ar = parseFloat(t.dataset.ar);
    t.style.flexBasis = Math.round(ar * state.thumb) + "px";
  });
}

// ---------- inspector ----------

function chipsHTML(items, kind) {
  // items: [{name, n, total}]; partial when n < total.
  return items.map(({ name, n, total }) => {
    const partial = n < total;
    const label = kind === "album" ? albumLabel(name) : name;
    return `<span class="chip${partial ? " partial" : ""}"${label !== name ? ` title="${esc(name)}"` : ""}>${esc(label)}${partial ? ` <span class="frac">${n}/${total}</span>` : ""}
      <button data-remove-${kind}="${esc(name)}" aria-label="Remove ${esc(name)}" title="Remove">${icon("x")}</button></span>`;
  }).join("");
}

function tally(sel, pick) {
  const counts = {};
  for (const p of sel) for (const v of pick(p)) counts[v] = (counts[v] || 0) + 1;
  return Object.keys(counts).sort((a, b) => a.localeCompare(b)).map((name) => ({ name, n: counts[name], total: sel.length }));
}

function renderInspector() {
  const box = $("inspector");
  const focusKey = document.activeElement && box.contains(document.activeElement) ? document.activeElement.dataset.key : null;
  const sel = state.photos.filter((p) => state.selected.has(p.id));

  if (!sel.length) {
    box.innerHTML = `<div class="placeholder">${icon("image")}
      <div>Select photos to edit them</div>
      <div class="small">Shift-click selects a range · ⌘A selects all<br>Drag photos onto an album to add them</div></div>`;
    return;
  }

  const single = sel.length === 1 ? sel[0] : null;
  const many = sel.length;
  const albumDatalist = `<datalist id="dl-albums">${state.albums.map((a) => `<option value="${esc(a)}"${albumLabel(a) !== a ? ` label="${esc(albumLabel(a))}"` : ""}>`).join("")}</datalist>`;
  const tagDatalist = `<datalist id="dl-tags">${state.tags.map((t) => `<option value="${esc(t)}">`).join("")}</datalist>`;
  const featuredAll = sel.every((p) => p.featured), featuredSome = sel.some((p) => p.featured);

  let head;
  if (single) {
    const e = single.exif || {};
    const cell = (v) => `<div title="${esc(v || "")}">${esc(v || "—")}</div>`;
    const hasExif = e.camera || e.lens || e.focal || e.aperture || e.shutter || e.iso;
    head = `<img class="preview" src="/assets/${esc(single.preview || single.thumb)}" alt="" style="aspect-ratio:${aspect(single)}">
      <div class="title-block"><strong>${esc(single.id)}</strong><div class="muted small">${esc(fmtTaken(single.taken))}</div></div>
      ${hasExif ? `<div class="exif">
        <div class="gear"><span>${esc(e.camera || "Unknown camera")}</span><span class="muted">${esc(e.lens || "Unknown lens")}</span></div>
        ${cell(e.focal)}${cell(e.aperture)}${cell(e.shutter)}${cell(e.iso ? "ISO " + e.iso : "")}
      </div>` : ""}
      <label class="field"><span class="label">Caption</span>
        <textarea id="caption" class="input" rows="2" placeholder="Add a caption…" data-key="caption">${esc(single.caption || "")}</textarea>
        <span id="caption-hint" class="hint">Saves automatically</span>
      </label>`;
  } else {
    const stack = sel.slice(0, 3).map((p, i) =>
      `<img src="/assets/${esc(p.thumb)}" alt="" style="left:${i * 10}px;top:${i * 8}px">`).join("");
    head = `<div class="row" style="gap:12px"><div class="stack">${stack}</div>
      <div><strong style="font-size:14px">${many} photos selected</strong><br>
      <button class="btn-text link" data-action="clear">Clear selection</button></div></div>`;
  }

  const albums = tally(sel, albumsOf), tags = tally(sel, tagsOf);
  const coverRow = single && albumsOf(single).length ? `<div class="setting"><span>Use as album cover</span>
      <select id="cover-select" aria-label="Use as cover of album"><option value="">Choose album…</option>
      ${albumsOf(single).map((a) => `<option value="${esc(a)}">${esc(albumLabel(a))}</option>`).join("")}</select></div>` : "";

  box.innerHTML = `${head}
    <div class="field"><span class="label">Albums</span>
      <div class="chip-input" data-focus="album-input">${chipsHTML(albums, "album")}
        <input id="album-input" list="dl-albums" data-key="album-input" placeholder="${single ? "Add to album…" : "Add all to album…"}" aria-label="Add to album"></div>
      ${!single && albums.some((a) => a.n < a.total) ? '<span class="hint">Dashed = only some of the selection is in it</span>' : ""}
    </div>
    <div class="field"><span class="label">Tags</span>
      <div class="chip-input" data-focus="tag-input">${chipsHTML(tags, "tag")}
        <input id="tag-input" list="dl-tags" data-key="tag-input" placeholder="${single ? "Add tag…" : "Tag all…"}" aria-label="Add tag"></div>
    </div>
    <div class="settings">
      <label class="setting"><span>Featured on home page</span>
        <span class="switch"><input type="checkbox" id="featured" role="switch" data-key="featured" ${featuredAll ? "checked" : ""}><span></span></span></label>
      ${coverRow}
    </div>
    <button class="btn-text danger" data-action="delete">${icon("trash")} Delete ${single ? "photo" : plural(many, "photo")}</button>
    ${albumDatalist}${tagDatalist}`;

  const feat = $("featured");
  feat.indeterminate = featuredSome && !featuredAll;

  if (focusKey) {
    const el = box.querySelector(`[data-key="${focusKey}"]`);
    if (el) el.focus();
  }
}

// ---------- mutations ----------

async function addToAlbum(ids, slug) {
  const todo = ids.filter((id) => { const p = byId(id); return p && !albumsOf(p).includes(slug); });
  if (!todo.length) {
    toast(`${ids.length > 1 ? "All already in" : "Already in"} <strong>${esc(slug)}</strong>`, { type: "ok" });
    return;
  }
  const ok = await mutate(async () => {
    for (const id of todo) await api("POST", `/api/photos/${encodeURIComponent(id)}/albums`, { albums: [slug] });
  });
  if (ok) {
    toast(`Added ${plural(todo.length, "photo")} to <strong>${esc(slug)}</strong>`, {
      type: "ok",
      action: { label: "Undo", fn: () => removeFromAlbum(todo, slug, true) },
    });
  }
}

async function removeFromAlbum(ids, slug, quiet) {
  const todo = ids.filter((id) => { const p = byId(id); return p && albumsOf(p).includes(slug); });
  const ok = await mutate(async () => {
    for (const id of todo) await api("DELETE", `/api/photos/${encodeURIComponent(id)}/albums/${encodeURIComponent(slug)}`);
  });
  if (ok && !quiet) {
    toast(`Removed ${plural(todo.length, "photo")} from <strong>${esc(slug)}</strong>`, {
      type: "ok",
      action: { label: "Undo", fn: () => addToAlbum(todo, slug) },
    });
  }
}

async function setTags(sel, fn) {
  await mutate(async () => {
    for (const p of sel) {
      const next = fn(tagsOf(p));
      if (next.join("\n") !== tagsOf(p).join("\n")) await api("PATCH", `/api/photos/${encodeURIComponent(p.id)}`, { tags: next });
    }
  });
}

const selectedPhotos = () => state.photos.filter((p) => state.selected.has(p.id));

let captionTimer = null;
async function saveCaption() {
  clearTimeout(captionTimer);
  const ta = $("caption");
  const p = ta && byId([...state.selected][0]);
  if (!ta || !p || state.selected.size !== 1 || ta.value === (p.caption || "")) return;
  const hint = $("caption-hint");
  hint.textContent = "Saving…"; hint.className = "hint";
  try {
    const rec = await api("PATCH", `/api/photos/${encodeURIComponent(p.id)}`, { caption: ta.value });
    // Update in place so the inspector (and the caret) is not re-rendered while typing.
    Object.assign(p, rec && rec.id ? rec : { caption: ta.value });
    if ($("caption-hint") === hint) { hint.textContent = "Saved"; hint.className = "hint ok"; }
    refreshStatus();
  } catch (e) {
    if ($("caption-hint") === hint) { hint.textContent = "Not saved: " + e.message; hint.className = "hint"; }
  }
}

async function deleteSelected() {
  const sel = selectedPhotos();
  const ok = await confirmDialog(
    `Delete ${sel.length === 1 ? "this photo" : plural(sel.length, "photo")}?`,
    "They are removed from the catalog now. Originals in S3 are deleted a week after you publish.");
  if (!ok) return;
  const done = await mutate(async () => {
    for (const p of sel) await api("DELETE", `/api/photos/${encodeURIComponent(p.id)}`);
    state.selected.clear();
  });
  if (done) toast(`Deleted ${plural(sel.length, "photo")}`, { type: "ok" });
}

const isJpeg = (name) => /\.jpe?g$/i.test(name);
// photoId is the id photo add gives a file (the Go side's slugifyName):
// two files with the same id are the same photo to the catalog.
const photoId = (name) => name.replace(/\.[^.]*$/, "").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "photo";
const BATCH = 10;

// addFiles uploads files in batches of BATCH. skipped is how many files a
// folder import left out (not JPEG), for the summary.
async function addFiles(files, skipped = 0) {
  files = files.filter((f) => f.type.startsWith("image/") || isJpeg(f.name));
  // Files with the same name (e.g. IMG_0001.JPG in two folders) would become
  // one photo; keep the first and say which ones were left out.
  const seen = new Set(), dupes = [];
  files = files.filter((f) => {
    const id = photoId(f.name);
    if (seen.has(id)) { dupes.push(f.name); return false; }
    seen.add(id); return true;
  });
  let skippedNote = skipped ? ` Skipped ${plural(skipped, "file")} that ${skipped === 1 ? "isn't" : "aren't"} JPEG.` : "";
  if (dupes.length) {
    const names = dupes.slice(0, 3).map(esc).join(", ") + (dupes.length > 3 ? ", …" : "");
    skippedNote += ` Skipped ${plural(dupes.length, "file")} with the same name as another one (${names}); rename ${dupes.length === 1 ? "it" : "them"} and add again.`;
  }
  if (!files.length) {
    if (skippedNote) toast(`Nothing to add.${skippedNote}`, { type: "err" });
    return;
  }
  const target = state.view === "album" ? state.album : "";
  const where = target ? ` to <strong>${esc(albumLabel(target))}</strong>` : "";
  const busy = toast(`Adding ${plural(files.length, "photo")}${where}…`, { type: "busy", timeout: 0 });
  const added = [];
  try {
    for (let i = 0; i < files.length; i += BATCH) {
      if (files.length > BATCH) busy.set(`Adding ${Math.min(i + BATCH, files.length)} of ${files.length}${where}…`);
      const fd = new FormData();
      for (const f of files.slice(i, i + BATCH)) fd.append("files", f, f.name);
      if (target) fd.append("albums", target);
      const recs = await api("POST", "/api/add", fd);
      if (Array.isArray(recs)) added.push(...recs.map((r) => r.id));
    }
    busy.close();
    toast(`Added ${plural(added.length, "photo")}.${skippedNote}`, { type: dupes.length ? "err" : "ok" });
  } catch (e) {
    busy.close();
    const n = added.length;
    const done = n ? ` ${plural(n, "photo")} ${n === 1 ? "was" : "were"} added before it stopped.` : "";
    toast(`Add failed: ${esc(e.message.replace(/\.$/, ""))}.${done}`, { type: "err" });
  }
  state.selected = new Set(added);
  await reload();
}

// jpegsFromFolder keeps the JPEGs of a folder listing and counts the rest.
// A file is hidden when it, or any folder above it, starts with a dot
// (.thumbnails/, .DS_Store, AppleDouble ._ files): those are left out silently.
const isHidden = (f) => (f.webkitRelativePath || f.relPath || f.name).split("/").some((seg) => seg.startsWith("."));

function jpegsFromFolder(files) {
  const visible = files.filter((f) => !isHidden(f));
  const keep = visible.filter((f) => isJpeg(f.name));
  return { keep, skipped: visible.length - keep.length };
}

// filesFromDrop reads a drop: loose files as they are, folders walked
// recursively (JPEGs only, like photo add -r).
async function filesFromDrop(dt) {
  const entries = [...(dt.items || [])].map((it) => it.webkitGetAsEntry && it.webkitGetAsEntry()).filter(Boolean);
  if (!entries.some((e) => e.isDirectory)) return { files: [...dt.files], skipped: 0 };
  const loose = [], fromDirs = [];
  const fileOf = (entry) => new Promise((res, rej) => entry.file(res, rej));
  const readAll = (dir) => new Promise((res, rej) => {
    const reader = dir.createReader(), out = [];
    const next = () => reader.readEntries((batch) => {
      if (!batch.length) return res(out);
      out.push(...batch); next();
    }, rej);
    next();
  });
  async function walk(entry) {
    if (entry.name.startsWith(".")) return; // hidden file or folder
    if (entry.isFile) fromDirs.push(await fileOf(entry));
    else for (const child of await readAll(entry)) await walk(child);
  }
  for (const e of entries) {
    if (e.isDirectory) await walk(e);
    else loose.push(await fileOf(e));
  }
  const { keep, skipped } = jpegsFromFolder(fromDirs);
  return { files: [...loose, ...keep], skipped };
}

// ---------- albums ----------

async function renameAlbum() {
  const slug = state.album;
  const dlg = $("rename");
  $("rename-slug").textContent = slug;
  $("rename-input").value = albumLabel(slug);
  dlg.returnValue = "";
  dlg.showModal();
  $("rename-input").select();
  const ok = await new Promise((res) => dlg.addEventListener("close", () => res(dlg.returnValue === "ok"), { once: true }));
  const title = $("rename-input").value.trim();
  if (!ok || !title || title === albumLabel(slug)) return;
  const done = await mutate(() => api("PATCH", `/api/albums/${encodeURIComponent(slug)}`, { title }));
  if (done) toast(`Renamed to <strong>${esc(title)}</strong>`, { type: "ok" });
}

async function deleteAlbum() {
  const slug = state.album, label = albumLabel(slug);
  const inAlbum = state.photos.filter((p) => albumsOf(p).includes(slug));
  const orphans = inAlbum.filter((p) => albumsOf(p).length === 1).length;
  const one = orphans === 1;
  const body = `Its page is deleted and its ${plural(inAlbum.length, "photo")} ${inAlbum.length === 1 ? "is" : "are"} taken out of it. ` +
    (orphans
      ? `${plural(orphans, "photo")} will be in no album and move to Unassigned, unless you delete ${one ? "it" : "them"} too.`
      : "Every photo stays in its other albums.");
  const check = !orphans ? "" : one
    ? "Also delete the photo that is in no other album (its original in S3 is deleted a week after you publish)"
    : `Also delete the ${orphans} photos that are in no other album (their originals in S3 are deleted a week after you publish)`;
  const ok = await confirmDialog(`Delete album “${label}”?`, body, { ok: "Delete album", check });
  if (!ok) return;
  const purge = orphans > 0 && $("confirm-check").checked;
  let res;
  const done = await mutate(async () => {
    res = await api("DELETE", `/api/albums/${encodeURIComponent(slug)}${purge ? "?purge=true" : ""}`);
    state.extraAlbums.delete(slug);
    state.view = "all"; state.album = "";
  });
  if (done) toast(`Deleted album <strong>${esc(label)}</strong>` +
    (res && res.purged ? ` and ${plural(res.purged, "photo")}` : ""), { type: "ok" });
}

// ---------- toasts & dialogs ----------

function toast(html, { type = "ok", action, timeout = 5000 } = {}) {
  const el = document.createElement("div");
  el.className = "toast " + type;
  const ic = type === "err" ? "alert" : type === "busy" ? "spinner" : "check";
  el.innerHTML = `${icon(ic)}<span>${html}</span>`;
  const close = () => el.remove();
  const set = (h) => { el.querySelector("span").innerHTML = h; };
  if (action) {
    const b = document.createElement("button");
    b.textContent = action.label;
    b.onclick = () => { close(); action.fn(); };
    el.append(b);
  }
  $("toasts").append(el);
  while ($("toasts").children.length > 3) $("toasts").firstElementChild.remove();
  const ms = type === "err" && timeout ? 9000 : timeout;
  if (ms) setTimeout(close, ms);
  return { close, set };
}

// confirmDialog resolves true for OK. With opts.check, it shows a checkbox
// (read it from $("confirm-check").checked after OK). opts.ok labels the button.
function confirmDialog(title, body, opts = {}) {
  const dlg = $("confirm");
  $("confirm-title").textContent = title;
  $("confirm-body").textContent = body;
  $("confirm-extra").hidden = !opts.check;
  $("confirm-check").checked = false;
  $("confirm-check-label").textContent = opts.check || "";
  $("confirm-ok").textContent = opts.ok || "Delete";
  dlg.returnValue = "";
  dlg.showModal();
  $("confirm-ok").focus();
  return new Promise((resolve) => {
    dlg.addEventListener("close", () => resolve(dlg.returnValue === "ok"), { once: true });
  });
}

// ---------- selection ----------

function clickTile(id, e) {
  if (e.shiftKey && state.anchor && state.order.includes(state.anchor)) {
    const a = state.order.indexOf(state.anchor), b = state.order.indexOf(id);
    const [lo, hi] = a < b ? [a, b] : [b, a];
    for (const x of state.order.slice(lo, hi + 1)) state.selected.add(x);
  } else if (state.selected.has(id)) {
    state.selected.delete(id);
  } else {
    state.selected.add(id);
  }
  state.anchor = id;
  renderSelection();
  renderInspector();
}

function clearSelection() {
  state.selected.clear();
  state.anchor = null;
  renderSelection();
  renderInspector();
}

// ---------- event wiring ----------

function setView(view, album = "") {
  state.view = view;
  state.album = album;
  renderSidebar(); renderToolbar(); renderGrid();
}

document.querySelector(".sidebar").addEventListener("click", (e) => {
  const v = e.target.closest("[data-view]");
  const a = e.target.closest("[data-album]");
  const t = e.target.closest("[data-tag]");
  if (v) { e.preventDefault(); setView(v.dataset.view); }
  else if (a) { e.preventDefault(); setView("album", a.dataset.album); }
  else if (t) {
    e.preventDefault();
    state.tag = state.tag === t.dataset.tag ? "" : t.dataset.tag;
    renderSidebar(); renderToolbar(); renderGrid();
  }
});

$("grid").addEventListener("click", (e) => {
  const t = e.target.closest(".tile");
  if (t) clickTile(t.dataset.id, e);
});

document.querySelectorAll("[data-sort]").forEach((b) => b.onclick = () => {
  if (state.sort.key === b.dataset.sort) return;
  state.sort.key = b.dataset.sort;
  state.sort.dir = b.dataset.sort === "taken" ? "desc" : "asc";
  savePrefs(); renderToolbar(); renderGrid();
});
$("sort-dir").onclick = () => {
  state.sort.dir = state.sort.dir === "desc" ? "asc" : "desc";
  savePrefs(); renderToolbar(); renderGrid();
};
$("thumb-size").oninput = (e) => { state.thumb = +e.target.value; resizeTiles(); };
$("thumb-size").onchange = savePrefs;

let searchTimer = null;
$("search").oninput = (e) => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => { state.query = e.target.value; renderGrid(); }, 120);
};

// Inspector (delegated; the panel is re-rendered on every change)
const insp = $("inspector");
insp.addEventListener("click", (e) => {
  const ra = e.target.closest("[data-remove-album]");
  const rt = e.target.closest("[data-remove-tag]");
  const act = e.target.closest("[data-action]");
  const focusBox = e.target.closest(".chip-input");
  if (ra) removeFromAlbum([...state.selected], ra.dataset.removeAlbum);
  else if (rt) { const t = rt.dataset.removeTag; setTags(selectedPhotos(), (ts) => ts.filter((x) => x !== t)); }
  else if (act && act.dataset.action === "clear") clearSelection();
  else if (act && act.dataset.action === "delete") deleteSelected();
  else if (focusBox && e.target === focusBox) $(focusBox.dataset.focus).focus();
});
insp.addEventListener("keydown", (e) => {
  if (e.target.id === "album-input" && e.key === "Enter") {
    e.preventDefault();
    const slug = e.target.value.trim();
    if (slug) addToAlbum([...state.selected], slug);
  } else if (e.target.id === "tag-input" && (e.key === "Enter" || e.key === ",")) {
    e.preventDefault();
    const t = e.target.value.trim().replace(/,$/, "");
    if (t) setTags(selectedPhotos(), (ts) => ts.includes(t) ? ts : [...ts, t]);
  } else if (e.target.id === "caption" && e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
    e.preventDefault();
    saveCaption();
  }
});
insp.addEventListener("input", (e) => {
  if (e.target.id === "caption") {
    clearTimeout(captionTimer);
    const hint = $("caption-hint");
    hint.textContent = "Editing…"; hint.className = "hint";
    captionTimer = setTimeout(saveCaption, 900);
  }
});
insp.addEventListener("focusout", (e) => { if (e.target.id === "caption") saveCaption(); });
insp.addEventListener("change", (e) => {
  if (e.target.id === "featured") {
    const on = e.target.checked;
    const sel = selectedPhotos();
    mutate(async () => {
      for (const p of sel) if (!!p.featured !== on) await api("PATCH", `/api/photos/${encodeURIComponent(p.id)}`, { featured: on });
    });
  } else if (e.target.id === "cover-select" && e.target.value) {
    const slug = e.target.value;
    const p = selectedPhotos()[0];
    mutate(() => api("PATCH", `/api/albums/${encodeURIComponent(slug)}`, { cover: p.thumb }))
      .then((ok) => ok && toast(`Cover set for <strong>${esc(slug)}</strong>`, { type: "ok" }));
  }
});

// New album
$("new-album-btn").onclick = () => {
  const f = $("new-album");
  f.hidden = !f.hidden;
  if (!f.hidden) $("na-slug").focus();
};
$("na-cancel").onclick = () => { $("new-album").hidden = true; };
$("new-album").onsubmit = async (e) => {
  e.preventDefault();
  const slug = $("na-slug").value.trim();
  const title = $("na-title").value.trim();
  try {
    await api("POST", "/api/albums", title ? { slug, title } : { slug });
    state.extraAlbums.add(slug);
    $("na-slug").value = ""; $("na-title").value = "";
    $("new-album").hidden = true;
    toast(`Created album <strong>${esc(slug)}</strong>. Drag photos onto it to fill it.`, { type: "ok" });
    await reload();
  } catch (err) { toast("Could not create album: " + esc(err.message), { type: "err" }); }
};

// Add photos: button, file picker, and whole-window drop
$("add-btn").onclick = () => $("file-input").click();
$("file-input").onchange = (e) => { addFiles([...e.target.files]); e.target.value = ""; };
$("add-folder-btn").onclick = () => $("folder-input").click();
$("folder-input").onchange = (e) => {
  const { keep, skipped } = jpegsFromFolder([...e.target.files]);
  e.target.value = "";
  addFiles(keep, skipped);
};
$("rename-album").onclick = renameAlbum;
$("rename-cancel").onclick = () => $("rename").close("cancel");
$("delete-album").onclick = deleteAlbum;

const PHOTO_MIME = "application/x-photo-ids";
const isInternal = (e) => [...(e.dataTransfer?.types || [])].includes(PHOTO_MIME);
const isFiles = (e) => !isInternal(e) && [...(e.dataTransfer?.types || [])].includes("Files");
let fileDragDepth = 0;
window.addEventListener("dragenter", (e) => {
  if (!isFiles(e)) return;
  fileDragDepth++;
  $("drop-overlay").hidden = false;
});
window.addEventListener("dragleave", (e) => {
  if (!isFiles(e)) return;
  fileDragDepth = Math.max(0, fileDragDepth - 1);
  if (!fileDragDepth) $("drop-overlay").hidden = true;
});
window.addEventListener("dragover", (e) => { if (isFiles(e)) e.preventDefault(); });
window.addEventListener("drop", (e) => {
  if (!isFiles(e)) return;
  e.preventDefault();
  fileDragDepth = 0;
  $("drop-overlay").hidden = true;
  filesFromDrop(e.dataTransfer)
    .then(({ files, skipped }) => addFiles(files, skipped))
    .catch((err) => toast("Could not read the dropped files: " + esc(err.message), { type: "err" }));
});

// Drag photos onto albums
function dragGhost(n, src) {
  const g = document.createElement("div");
  g.style.cssText = "position:fixed;top:-200px;left:-200px;width:72px;height:72px;";
  g.innerHTML = `<img src="${src}" style="width:64px;height:64px;object-fit:cover;border-radius:8px;border:2px solid #fff;box-shadow:0 6px 18px rgba(0,0,0,.5)">` +
    (n > 1 ? `<span style="position:absolute;top:-6px;right:0;background:#3866e8;color:#fff;font:600 12px/20px system-ui;min-width:20px;padding:0 6px;border-radius:999px;text-align:center">${n}</span>` : "");
  document.body.append(g);
  setTimeout(() => g.remove(), 0);
  return g;
}

$("grid").addEventListener("dragstart", (e) => {
  const t = e.target.closest(".tile");
  if (!t) return;
  const id = t.dataset.id;
  const ids = state.selected.has(id) ? state.order.filter((x) => state.selected.has(x)) : [id];
  state.dragIds = ids;
  e.dataTransfer.effectAllowed = "copy";
  e.dataTransfer.setData(PHOTO_MIME, JSON.stringify(ids));
  e.dataTransfer.setData("text/plain", ids.join("\n"));
  e.dataTransfer.setDragImage(dragGhost(ids.length, t.querySelector("img").src), 36, 36);
  document.body.classList.add("dragging-photos");
  const hint = $("drag-hint");
  hint.textContent = `Drop on an album to add ${plural(ids.length, "photo")}`;
  hint.hidden = false;
});
$("grid").addEventListener("dragend", () => {
  state.dragIds = null;
  document.body.classList.remove("dragging-photos");
  $("drag-hint").hidden = true;
  document.querySelectorAll(".drop-over").forEach((x) => x.classList.remove("drop-over"));
});

const albumList = $("album-list");
albumList.addEventListener("dragover", (e) => {
  const item = e.target.closest(".album-item");
  if (!item || !isInternal(e)) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = "copy";
  if (item.classList.contains("drop-over")) return;
  document.querySelectorAll(".drop-over").forEach((x) => x.classList.remove("drop-over"));
  item.classList.add("drop-over");
  const slug = item.dataset.album;
  const adds = (state.dragIds || []).filter((id) => { const p = byId(id); return p && !albumsOf(p).includes(slug); }).length;
  const badge = item.querySelector(".adds");
  badge.textContent = adds ? `+${adds}` : "✓";
  badge.hidden = false;
});
albumList.addEventListener("dragleave", (e) => {
  const item = e.target.closest(".album-item");
  if (item && !item.contains(e.relatedTarget)) {
    item.classList.remove("drop-over");
    item.querySelector(".adds").hidden = true;
  }
});
albumList.addEventListener("drop", (e) => {
  const item = e.target.closest(".album-item");
  if (!item || !isInternal(e)) return;
  e.preventDefault();
  let ids = [];
  try { ids = JSON.parse(e.dataTransfer.getData(PHOTO_MIME)); } catch {}
  item.classList.remove("drop-over");
  if (ids.length) addToAlbum(ids, item.dataset.album);
});

// Publish popover
const pop = $("publish-pop");
function setPop(open) {
  pop.hidden = !open;
  $("publish-btn").setAttribute("aria-expanded", String(open));
  if (open) { refreshStatus(); $("commit-msg").select(); }
}
$("publish-btn").onclick = (e) => { e.stopPropagation(); setPop(pop.hidden); };
pop.addEventListener("click", (e) => { if (e.target.closest("[data-close]")) setPop(false); });
document.addEventListener("click", (e) => {
  if (!pop.hidden && !pop.contains(e.target) && !e.target.closest(".toast")) setPop(false);
});
$("commit-msg").onkeydown = (e) => { if (e.key === "Enter") { e.preventDefault(); $("commit-btn").click(); } };

const GIT_CODES = { "??": "U", M: "M", A: "A", D: "D", R: "M", C: "A" };
async function refreshStatus() {
  let s;
  try { s = await api("GET", "/api/status"); }
  catch (e) {
    $("git-error").textContent = "Status unavailable: " + e.message;
    $("git-error").hidden = false;
    return;
  }
  if (s.site) {
    $("site-name").textContent = s.site;
    document.title = `${s.site} Admin`;
  }
  const lines = (s.git || "").split("\n").filter(Boolean);
  const pending = s.pendingDeletes || 0;
  const due = s.dueDeletes || 0;
  const n = lines.length + pending + due;
  const badge = $("change-badge");
  badge.textContent = n;
  badge.hidden = !n;
  $("change-list").innerHTML = lines.map((l) => {
    const xy = l.slice(0, 2);
    const code = GIT_CODES[xy] || GIT_CODES[xy.trim()[0]] || "M";
    return `<li><span class="st ${code}">${code}</span><span class="path" title="${esc(l.slice(3))}">${esc(l.slice(3))}</span></li>`;
  }).join("") || '<li class="empty">No local changes.</li>';
  const pd = $("pending-deletes");
  const sched = (s.scheduledDeletes || 0) - due;
  const notes = [];
  if (pending) notes.push(`${plural(pending, "original")} will be deleted from S3 by a Publish at least a week after this one, so the live site keeps working while it rebuilds.`);
  if (due) notes.push(`${plural(due, "original")} published over a week ago will be deleted from S3 now.`);
  if (sched) {
    const next = s.nextDeleteDue ? new Date(s.nextDeleteDue).toLocaleDateString(undefined, { day: "numeric", month: "short" }) : "";
    notes.push(`${plural(sched, "earlier delete")} ${sched === 1 ? "is" : "are"} scheduled${next ? `; the first is due ${next}` : ""}.`);
  }
  pd.textContent = notes.join(" ");
  pd.hidden = !notes.length;
  const ge = $("git-error");
  ge.textContent = s.gitError ? "git: " + s.gitError : "";
  ge.hidden = !s.gitError;
  $("commit-btn").disabled = !n;
}

$("commit-btn").onclick = async () => {
  const message = $("commit-msg").value.trim();
  if (!message) { toast("Enter a commit message.", { type: "err" }); $("commit-msg").focus(); return; }
  const btn = $("commit-btn");
  btn.disabled = true;
  const busy = toast("Publishing…", { type: "busy", timeout: 0 });
  try {
    const res = await api("POST", "/api/publish", { message });
    busy.close();
    const gone = res && res.deletedFromS3 ? ` Deleted ${plural(res.deletedFromS3, "original")} from S3 that ${res.deletedFromS3 === 1 ? "was" : "were"} published over a week ago.` : "";
    toast(`Published. The site will rebuild shortly.${gone}`, { type: "ok" });
    setPop(false);
  } catch (e) {
    busy.close();
    toast("Publish failed: " + esc(e.message), { type: "err" });
  }
  btn.disabled = false;
  await reload();
};

// Keyboard shortcuts
document.addEventListener("keydown", (e) => {
  if ($("confirm").open) return; // the dialog handles its own keys
  const typing = e.target.closest("input, textarea, select, [contenteditable]");
  if (e.key === "Escape") {
    if (!pop.hidden) { setPop(false); return; }
    if (typing) { e.target.blur(); return; }
    if (state.selected.size) clearSelection();
    return;
  }
  if (typing) return;
  if (e.key === "/") { e.preventDefault(); $("search").focus(); }
  else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "a") {
    e.preventDefault();
    for (const id of state.order) state.selected.add(id);
    renderSelection(); renderInspector();
  }
});

load().catch((e) => {
  $("grid").innerHTML = `<div class="empty-state">${icon("alert", "icon xl")}<div>Failed to load: ${esc(e.message)}</div></div>`;
});
refreshStatus();
