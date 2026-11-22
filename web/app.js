import { t, setLang, lang, locale } from "./i18n.js";
import { icon, brand, hasIcon } from "./icons.js";
import { helpSections } from "./help.js";
import { openEditor, mountInlineEditor } from "./editor.js";
import { renderPreview } from "./preview.js";
import { avatarURI, loadAvatarLib, avatarLibReady, avatarEditor } from "./avatar.js";

/* ═════════ helpers ═════════ */
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const h = (html) => { const d = document.createElement("template"); d.innerHTML = html.trim(); return d.content.firstElementChild; };
const md = (s) => (window.DOMPurify && window.marked ? DOMPurify.sanitize(marked.parse(s || "", { breaks: true, gfm: true })) : esc(s));
const store = { get: (k, d) => { try { return JSON.parse(localStorage.getItem("hiveloop." + k)) ?? d; } catch { return d; } }, set: (k, v) => { try { localStorage.setItem("hiveloop." + k, JSON.stringify(v)); } catch {} } };
/* ═════════ personalisation (theme, accent, buttons; saved per user) ═════════ */
const ACCENTS = { blue: "#2563eb", indigo: "#4f46e5", violet: "#7c3aed", pink: "#db2777", red: "#dc2626", green: "#15803d", teal: "#0f766e", cyan: "#0e7490", olive: "#4d7c0f", slate: "#475569" };
function applyPrefs(p = {}) {
  const root = document.documentElement;
  for (const [k, a] of [["theme", "theme"], ["accent", "accent"], ["buttons", "btn"], ["vision", "vision"]]) { if (p[k]) root.dataset[a] = p[k]; else delete root.dataset[a]; }
  // personal zoom, like a monitor's scale setting (CSS zoom on the whole page)
  const z = p.zoom ? p.zoom / 100 : 1;
  root.style.zoom = z === 1 ? "" : String(z);
  root.style.setProperty("--zf", String(z));
  markNarrow();
  store.set("prefs", p);
  const metas = document.querySelectorAll('meta[name="theme-color"]');
  metas.forEach((m, i) => {
    if (p.theme) { m.removeAttribute("media"); m.content = p.theme === "dark" ? "#121211" : "#f3f2ee"; }
    else { m.media = i ? "(prefers-color-scheme: dark)" : "(prefers-color-scheme: light)"; m.content = i ? "#0c0c0b" : "#f3f2ee"; }
  });
  markBtns(document.body || root);
}
// buttons with both an icon and a label, so the "icons only" / "text only" choice can apply
function markBtn(b) {
  const txt = b.textContent.trim();
  b.classList.toggle("has-t", !!txt);
  b.classList.toggle("has-i", !!b.querySelector("svg"));
  if (txt && document.documentElement.dataset.btn === "icons" && !b.title) b.title = txt;
  if (txt && !b.getAttribute("aria-label")) b.setAttribute("aria-label", txt);
}
function markBtns(root) {
  if (!root?.querySelectorAll) return;
  if (root.classList?.contains("btn")) markBtn(root);
  root.querySelectorAll(".btn").forEach(markBtn);
  const iconsOnly = document.documentElement.dataset.btn === "icons";
  root.querySelectorAll(".nav a.item[aria-label]").forEach((a) => { if (iconsOnly) a.title = a.getAttribute("aria-label"); else a.removeAttribute("title"); });
}
new MutationObserver((ms) => ms.forEach((m) => m.addedNodes.forEach((n) => { if (n.nodeType === 1) { markBtns(n); fillAvatars(n); } }))).observe(document.documentElement, { childList: true, subtree: true });

// avatars: every ".avatar[data-uid]" shows the user's cartoon avatar, or their initials
let avatarsById = {};
function fillAvatars(root) {
  if (!root?.querySelectorAll || !avatarLibReady()) return;
  const sel = ".avatar[data-uid], .mark[data-uid]";
  const els = root.matches?.(sel) ? [root] : root.querySelectorAll(sel);
  els.forEach((el) => {
    const o = avatarsById[el.dataset.uid];
    if (o) {
      if (!el.classList.contains("has-av")) el.dataset.ini = el.innerHTML;
      const src = avatarURI(o);
      if (el.firstElementChild?.getAttribute("src") !== src) el.innerHTML = `<img alt="" src="${src}">`;
      el.classList.add("has-av");
    } else if (el.classList.contains("has-av")) { el.innerHTML = el.dataset.ini || ""; el.classList.remove("has-av"); }
  });
}
async function loadAvatars() {
  try { avatarsById = await api("GET", "/api/avatars"); } catch { return; }
  if (Object.keys(avatarsById).length) await loadAvatarLib();
  fillAvatars(document.body);
}
// with a strong zoom (or high readability) a phone is effectively very narrow: flag it so
// toolbars wrap and the bottom bar keeps only icons
function markNarrow() {
  const root = document.documentElement, z = parseFloat(root.style.zoom) || 1;
  const w = innerWidth / z / (root.dataset.vision === "high" ? 1.12 : 1);
  if (w < 380) root.dataset.narrow = ""; else delete root.dataset.narrow;
}
addEventListener("resize", markNarrow);
applyPrefs(store.get("prefs", {}));
setInterval(async () => {
  if (!S.me || !S.aiSched?.on || document.visibilityState !== "visible") return;
  const was = S.ai;
  try { await loadMe(); } catch { return; }
  if (S.ai !== was) { renderShell(); route(); }
}, 60000);

const initials = (u) => (u?.name || u?.username || "?").split(/\s+/).map((x) => x[0]).slice(0, 2).join("");
// avatar: the only way to show a person — their cartoon avatar if they made one, else initials.
// size: "" (follows the place it is in), "xs", "sm", "md", "xl".
const avatar = (id, who, size = "") => `<span class="avatar${size ? " " + size : ""}" data-uid="${id}">${esc(initials(who))}</span>`;
const fmtSize = (n) => (n < 1024 ? n + " B" : n < 1048576 ? (n / 1024).toFixed(0) + " KB" : (n / 1048576).toFixed(1) + " MB");
const pad = (n) => String(n).padStart(2, "0");
const localInput = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
const dateInput = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const sameDay = (a, b) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
const startOfDay = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate());
const addDays = (d, n) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
const fmt = (d, o) => new Intl.DateTimeFormat(locale(), o).format(d);
const fmtTime = (d) => fmt(d, { hour: "2-digit", minute: "2-digit" });
// group palette (same as the server's); a colour used by a group isn't offered to the others
const GROUP_COLORS = ["#334155", "#e11d48", "#d946ef", "#7c3aed", "#4f46e5", "#0284c7", "#0d9488", "#16a34a", "#ca8a04", "#78716c",
  "#dc2626", "#b45309", "#65a30d", "#059669", "#0891b2", "#2563eb", "#9333ea", "#c026d3", "#db2777", "#be123c",
  "#a16207", "#57534e", "#0f766e", "#1d4ed8", "#6d28d9", "#4d7c0f"];
const freeColors = (g) => {
  const used = new Set(S.groups.filter((x) => x.id !== g?.id).map((x) => (x.color || "").toLowerCase()));
  return GROUP_COLORS.filter((c) => !used.has(c));
};

class ApiError extends Error {}
async function api(method, path, body, raw = false) {
  const opt = { method, headers: { "X-Hiveloop": "1" }, credentials: "same-origin" };
  if (body instanceof FormData) opt.body = body;
  else if (body instanceof Blob) { opt.body = body; opt.headers["Content-Type"] = body.type || "application/octet-stream"; }
  else if (body !== undefined) { opt.body = JSON.stringify(body); opt.headers["Content-Type"] = "application/json"; }
  const r = await fetch(path, opt);
  if (raw) return r;
  if (r.status === 401 && path !== "/api/login") { S.me = null; renderLogin(); throw new ApiError("unauthorized"); }
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new ApiError(data.error || "error");
  return data;
}

function toast(msg, bad = false, onClick = null, ic = null) {
  const el = h(`<div class="toast ${bad ? "bad" : ""} ${onClick ? "click" : ""}">${icon(ic || (bad ? "x" : "check"))}<span>${esc(msg)}</span>${onClick ? icon("right") : ""}</div>`);
  if (onClick) el.onclick = () => { el.remove(); onClick(); };
  $("#toasts").append(el);
  setTimeout(() => { el.style.transition = "opacity .3s"; el.style.opacity = 0; setTimeout(() => el.remove(), 300); }, onClick ? 6000 : 2600);
}
// views register what they show so live updates can refresh them in place
let liveView = null; // { kinds: [...], fn }
let liveTimer = null;
function liveRefresh(kind) {
  if (!liveView?.kinds.includes(kind)) return;
  clearTimeout(liveTimer);
  const lv = liveView;
  liveTimer = setTimeout(() => { if (liveView === lv) lv.fn(); }, 350);
}
const fail = (e) => { if (!(e instanceof ApiError && e.message === "unauthorized")) toast(t(e.message) || t("error"), true); };

/* Full-page panels (event, forms, previews…). Each one is a history entry, so the
   back arrow, Android's Back button and the browser's Back all close it. */
const pages = [];               // open panels, top last
let histWait = Promise.resolve(); // pending history.back() from a programmatic close
let ignorePop = 0;
window.addEventListener("popstate", () => {
  if (ignorePop > 0) { ignorePop--; return; }
  pages.at?.(-1)?.dismiss(); // Back pressed: close the top panel without touching history again
});

function sheet({ title, body, foot, wide = false, onClose, dialog = false }) {
  const root = $("#sheet-root");
  const bd = h(`<div class="backdrop ${dialog ? "" : "page-bd"}"></div>`);
  const el = h(`<div class="sheet ${dialog ? "dialog" : "page"} ${wide ? "wide" : ""}" role="dialog" aria-modal="true">
    <div class="sh">${dialog ? "" : `<button class="ibtn ghost back" data-x aria-label="${t("back")}">${icon("left")}</button>`}<h3></h3>${dialog ? `<button class="ibtn ghost" data-x aria-label="${t("close")}">${icon("x")}</button>` : ""}</div>
    <div class="sb"><div class="sbi"></div></div></div>`);
  $("h3", el).textContent = title;
  const sb = $(".sbi", el);
  typeof body === "string" ? (sb.innerHTML = body) : sb.append(body);
  if (foot) { const f = h(`<div class="sf"><div class="sfi"></div></div>`); const fi = $(".sfi", f); typeof foot === "string" ? (fi.innerHTML = foot) : fi.append(foot); el.append(f); }

  let open = true, pushed = false;
  const entry = { el };
  // visual close, shared by all ways of closing
  const dismiss = () => {
    if (!open) return;
    open = false;
    pages.splice(pages.indexOf(entry), 1);
    el.classList.add("closing"); bd.style.transition = "opacity .2s"; bd.style.opacity = 0;
    document.removeEventListener("keydown", onKey);
    setTimeout(() => { el.remove(); bd.remove(); onClose?.(); }, 220);
  };
  // close from the UI (back arrow, Save, Cancel…): also drop our history entry
  const close = () => {
    if (!open) return;
    const mine = pushed && history.state?.hlPage === entry.id;
    dismiss();
    if (mine) {
      ignorePop++;
      histWait = new Promise((res) => {
        const done = () => { window.removeEventListener("popstate", done); res(); };
        window.addEventListener("popstate", done);
        setTimeout(done, 600);
        history.back();
      });
    }
  };
  entry.dismiss = dismiss;
  entry.id = Math.random().toString(36).slice(2);
  pages.push(entry);
  if (!dialog) histWait.then(() => { if (open) { history.pushState({ ...(history.state || {}), hlPage: entry.id }, ""); pushed = true; } });

  const onKey = (e) => e.key === "Escape" && pages.at(-1) === entry && close();
  document.addEventListener("keydown", onKey);
  if (dialog) bd.onclick = close;
  $("[data-x]", el).onclick = close;
  root.append(bd, el);
  el.querySelector(".sb").scrollTop = 0;
  if (!matchMedia("(pointer: coarse)").matches) setTimeout(() => $("input:not([type=checkbox]):not([type=radio]),textarea", sb)?.focus({ preventScroll: true }), 60);
  return { el, close };
}
// navigating to another section closes every open panel
function closeAllPages() { while (pages.length) pages.at(-1).dismiss(); }
function confirmSheet(msg) {
  return new Promise((res) => {
    let ok = false;
    const s = sheet({
      title: t("confirm_delete"), body: `<p class="muted">${esc(msg)}</p>`,
      foot: `<button class="btn" data-c>${t("cancel")}</button><button class="btn danger" data-ok>${icon("trash")}${t("delete")}</button>`,
      onClose: () => res(ok),
      dialog: true,
    });
    $("[data-c]", s.el).onclick = s.close;
    $("[data-ok]", s.el).onclick = () => { ok = true; s.close(); };
  });
}

/* ═════════ state ═════════ */
const S = { me: null, myGroups: [], groups: [], users: [] };
const isAdmin = () => ["admin", "superadmin"].includes(S.me?.role);
const isSuper = () => S.me?.role === "superadmin";
const groupById = (id) => S.groups.find((g) => g.id === id);

async function loadMe() {
  const r = await api("GET", "/api/me");
  S.me = r.user; S.myGroups = r.groups; S.pending = r.pending || 0; S.registration = r.registration;
  S.prefs = r.prefs || {};
  S.avatar = r.avatar || null;
  S.general = r.general || {};
  S.ai = r.ai !== false;            // usable now (switch on and inside the hours)
  S.aiSwitch = r.ai_switch !== false; // master switch
  S.aiSched = r.ai_schedule || { on: false, from: "08:00", to: "20:00", days: [] };
  S.feat = r.features || { doc_create: true, doc_upload: true };
  S.aiQuota = r.ai_quota || { used: 0, left: -1, limit: -1 }; // left -1 = no daily limit
  loadAvatars();
  applyPrefs(S.prefs);
  setLang(S.me.lang);
}
async function loadGroups() { S.groups = await api("GET", "/api/groups"); S.myGroups = S.groups.filter((g) => g.is_member).map((g) => g.id); }

/* ═════════ login ═════════ */
let regMode = null;
async function renderLogin(mode = "login") {
  document.title = "Hiveloop";
  if (regMode === null) { try { regMode = (await (await fetch("/api/config")).json()).registration; } catch { regMode = "closed"; } }
  const reg = mode === "register";
  const root = $("#root");
  root.innerHTML = `<div class="login">
    <div class="art"><div class="orb"></div>
      <div class="meta mono"><span>HIVELOOP / ${new Date().getFullYear()}</span><span>${fmt(new Date(), { day: "2-digit", month: "short" })}</span></div>
      <h1 class="word">hiveloop<span>.</span></h1>
      <div class="tag">${t("app_tagline")}</div>
    </div>
    <div class="form" style="position:relative">
      <div class="lang seg"><button data-l="en" class="${lang() === "en" ? "on" : ""}">EN</button><button data-l="it" class="${lang() === "it" ? "on" : ""}">IT</button></div>
      <form autocomplete="on" novalidate>
        <h2>${reg ? t("register") : t("login")}</h2>
        ${reg ? `<label class="field"><span>${t("full_name")}</span><input class="input" name="name" autocomplete="name" required maxlength="80"></label>` : ""}
        <label class="field"><span>${t("username")}</span><input class="input" name="username" autocomplete="username" required autocapitalize="none" spellcheck="false" ${reg ? `minlength="3" maxlength="32"` : ""}>${reg ? `<small class="hint">${t("username_hint")}</small>` : ""}</label>
        <label class="field"><span>${t("password")}</span><input class="input" name="password" type="password" autocomplete="${reg ? "new-password" : "current-password"}" required ${reg ? `minlength="8"` : ""}>${reg ? `<small class="hint">${t("password_hint_self")}</small>` : ""}</label>
        ${reg ? `<input name="website" tabindex="-1" autocomplete="off" class="hp" aria-hidden="true">` : ""}
        <div class="err"></div>
        <button class="btn primary" style="width:100%;justify-content:center;height:44px">${reg ? t("register") : t("login")}${icon("send")}</button>
        ${regMode !== "closed" ? `<p class="switch muted">${reg ? t("have_account") : t("no_account")} <a href="#" data-sw>${reg ? t("login") : t("register")}</a></p>` : ""}
        ${reg && regMode === "approval" ? `<p class="hint">${t("approval_note")}</p>` : ""}
      </form>
      <div class="dl-wrap"><div class="mono muted dl-t">${t("get_app")}</div>${downloadButtons()}</div>
    </div></div>`;
  $$("[data-l]", root).forEach((b) => (b.onclick = () => { setLang(b.dataset.l); store.set("lang", b.dataset.l); renderLogin(mode); }));
  $("[data-sw]", root)?.addEventListener("click", (e) => { e.preventDefault(); renderLogin(reg ? "login" : "register"); });
  bindDownloads(root);
  const un = $("input[name=username]", root);
  if (reg) un.oninput = () => { un.value = un.value.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase().replace(/\s+/g, ".").replace(/[^a-z0-9._-]/g, ""); };
  $("form", root).onsubmit = async (e) => {
    e.preventDefault();
    const form = e.target, errEl = $(".err", root);
    let bad = null;
    $$("input.input", form).forEach((i) => { const b = !i.checkValidity(); i.classList.toggle("bad", b); if (b && !bad) bad = i; });
    if (bad) { bad.focus(); errEl.textContent = bad.name === "username" ? t("invalid_username") : bad.name === "password" && reg ? t("password_too_short") : bad.name === "name" ? t("name_required") : t("fill_fields"); return; }
    const f = new FormData(form);
    try {
      if (reg) {
        const r = await api("POST", "/api/register", { name: f.get("name"), username: f.get("username"), password: f.get("password"), website: f.get("website") });
        if (r.status === "active") return boot();
        form.innerHTML = `<h2>${t("request_sent")}</h2><p class="muted">${t("request_sent_text").replace("{u}", `<b>@${esc(r.username || "")}</b>`)}</p><button type="button" class="btn" data-back>${icon("left")}${t("login")}</button>`;
        $("[data-back]", form).onclick = () => renderLogin("login");
        return;
      }
      await api("POST", "/api/login", { username: f.get("username"), password: f.get("password") });
      await boot();
    } catch (err) { errEl.textContent = t(err.message); }
  };
}

/* ═════════ shell & router ═════════ */
const NAV = [
  ["calendar", "calendar", "nav_calendar"],
  ["chat", "chat", "nav_chat"],
  ["groups", "groups", "nav_groups"],
  ["docs", "docs", "nav_docs"],
  ["ai", "ai", "nav_ai", () => S.ai],
  ["admin", "admin", "nav_admin", isAdmin],
  ["settings", "user", "nav_settings"],
  ["help", "help", "nav_help", null, "hide-m"],
];

const MAIN_M = ["calendar", "chat", "groups", "docs"]; // bottom bar on phones; the rest goes in "Altro"
function openMoreMenu() {
  const items = NAV.filter((n) => (!n[3] || n[3]()) && !MAIN_M.includes(n[0]));
  const s = sheet({
    title: t("more_menu"),
    body: `<div class="more-list">${items.map((n) => `<a href="#/${n[0]}" class="more-item">${icon(n[1])}<span>${t(n[2])}</span><b class="nb ${n[0] === "admin" && S.pending ? "on at" : ""}">${n[0] === "admin" && S.pending ? S.pending : ""}</b>${icon("right")}</a>`).join("")}</div>`,
  });
  $$(".more-item", s.el).forEach((a) => a.addEventListener("click", (e) => {
    e.preventDefault();
    const to = a.getAttribute("href");
    s.close();
    histWait.then(() => { location.hash = to; });
  }));
}
function renderShell() {
  const items = NAV.filter((n) => !n[3] || n[3]());
  $("#root").innerHTML = `<div class="shell">
    <nav class="nav">
      <a class="brand" href="#/calendar"><b>hiveloop</b><i></i></a>
      ${items.map((n, i) => `<a class="item ${n[4] || ""} ${MAIN_M.includes(n[0]) ? "" : "sec-m"}" href="#/${n[0]}" data-r="${n[0]}" aria-label="${t(n[2])}">${icon(n[1])}<span class="lb">${t(n[2])}</span><span class="n">0${i + 1}</span><b class="nb" data-nb="${n[0]}"></b></a>`).join("")}
      <a class="item more-m" href="#" data-more aria-label="${t("more_menu")}">${icon("menu")}<span class="lb">${t("more_menu")}</span><b class="nb" data-nb="more"></b></a>
      <div class="spacer"></div>
      <div class="me">${avatar(S.me.id, S.me)}<div class="who"><b>${esc(S.me.name || S.me.username)}</b><span>${t("role_" + S.me.role)}</span></div></div>
    </nav>
    <main id="main"></main></div>`;
  $("[data-more]").onclick = (e) => { e.preventDefault(); openMoreMenu(); };
}

const navNum = (r) => pad(NAV.filter((n) => !n[3] || n[3]()).findIndex((n) => n[0] === r) + 1);
const VIEWS = { help: viewHelp, chat: viewChat, calendar: viewCalendar, groups: viewGroups, docs: viewDocs, ai: viewAI, admin: viewAdmin, settings: viewSettings };
let cleanup = null;
async function route() {
  if (!S.me) return;
  let [name, ...args] = location.hash.replace(/^#\/?/, "").split("/");
  if (!VIEWS[name] || (name === "admin" && !isAdmin()) || (name === "ai" && !S.ai)) name = "calendar";
  $$(".nav .item").forEach((a) => a.classList.toggle("on", a.dataset.r === name));
  $(".nav .more-m")?.classList.toggle("on-m", !MAIN_M.includes(name));
  closeAllPages();
  cleanup?.(); cleanup = null; liveView = null;
  const main = $("#main");
  const v = h(`<div class="view"></div>`);
  main.replaceChildren(v);
  window.scrollTo(0, 0);
  document.title = `${t("nav_" + name)} · Hiveloop`;
  try { cleanup = (await VIEWS[name](v, args)) || null; } catch (e) { fail(e); }
}

function pageHead(kicker, title, sub, actions = "") {
  if (!title && !sub) return actions ? `<header class="ph bar"><div class="actions">${actions}</div></header>` : "";
  title = title.replace("<em>.</em>", "");
  return `<header class="ph"><div><h1>${title}</h1>${sub ? `<p>${esc(sub)}</p>` : ""}</div><div class="actions">${actions}</div></header>`;
}

/* ═════════ calendar ═════════ */
const cal = { cur: startOfDay(new Date()), mode: store.get("calMode", "month"), scope: store.get("calScope", "all"), sel: startOfDay(new Date()), events: [] };

function canCreateEvents() { return isAdmin() || S.myGroups.length > 0; }

async function viewCalendar(v, args) {
  await loadGroups();
  if (args[0]) { const d = new Date(/^\d{4}-\d{2}-\d{2}$/.test(args[0]) ? args[0] + "T00:00" : args[0]); if (!isNaN(d)) { cal.cur = startOfDay(d); cal.sel = startOfDay(d); } }
  if (/^e\d+$/.test(args[1] || "")) cal.openId = +args[1].slice(1);
  if (!["all", "mine", "booked"].includes(cal.scope) && !groupById(+cal.scope)) cal.scope = "all";
  const render = async () => {
    const y = cal.cur.getFullYear(), m = cal.cur.getMonth();
    const first = new Date(y, m, 1);
    const gridStart = addDays(first, -((first.getDay() + 6) % 7));
    const last = new Date(y, m + 1, 0);
    const gridEnd = addDays(last, 7 - ((last.getDay() + 6) % 7));
    const monthName = fmt(first, { month: "long" });
    const mLong = monthName[0].toUpperCase() + monthName.slice(1), mShort = fmt(first, { month: "short" }).replace(".", "");
    v.innerHTML = `<div class="cal-bar tool-bar">
        <button class="ibtn xs" data-p aria-label="${t("prev")}" title="${t("prev")}">${icon("left")}</button>
        <h1 class="cal-title"><span class="mo"><span class="hide-m">${esc(mLong)}</span><span class="only-m">${esc(mShort[0].toUpperCase() + mShort.slice(1))}</span></span><span class="yr">${y}</span></h1>
        <button class="ibtn xs" data-n aria-label="${t("next")}" title="${t("next")}">${icon("right")}</button>
        <button class="btn sm" data-today>${t("today")}</button>
        <span class="grow"></span>
        <div class="seg"><button data-mode="month" class="${cal.mode === "month" ? "on" : ""}" title="${t("month")}">${icon("grid")}<span class="hide-m">${t("month")}</span></button><button data-mode="agenda" class="${cal.mode === "agenda" ? "on" : ""}" title="${t("agenda")}">${icon("list")}<span class="hide-m">${t("agenda")}</span></button></div>
        ${canCreateEvents() ? `<button class="btn sm primary" data-new title="${t("new_event")}">${icon("plus")}<span>${t("add")}</span></button>` : ""}
      </div>
      <div class="chips" style="margin-bottom:16px">
        <button class="chip ${cal.scope === "all" ? "on" : ""}" data-s="all">${t("all")}</button>
        <button class="chip ${cal.scope === "mine" ? "on" : ""}" data-s="mine">${icon("user")}${t("my_groups")}</button>
        <button class="chip ${cal.scope === "booked" ? "on" : ""}" data-s="booked">${icon("check")}${t("my_bookings")}</button>
        ${S.groups.map((g) => `<button class="chip ${cal.scope == g.id ? "on" : ""}" data-s="${g.id}" style="--c:${esc(g.color)}"><i class="sw"></i>${esc(g.name)}</button>`).join("")}
      </div>
      <div data-body></div>`;
    $("[data-p]", v).onclick = () => { cal.cur = new Date(y, m - 1, 1); render(); };
    $("[data-n]", v).onclick = () => { cal.cur = new Date(y, m + 1, 1); render(); };
    $("[data-today]", v).onclick = () => { cal.cur = startOfDay(new Date()); cal.sel = startOfDay(new Date()); render(); };
    $$("[data-mode]", v).forEach((b) => (b.onclick = () => { cal.mode = b.dataset.mode; store.set("calMode", cal.mode); render(); }));
    $$("[data-s]", v).forEach((b) => (b.onclick = () => { cal.scope = b.dataset.s; store.set("calScope", cal.scope); render(); }));
    $$("[data-new]", v).forEach((x) => x.addEventListener("click", () => eventForm(null, cal.sel, render)));

    const from = cal.mode === "month" ? gridStart : first;
    const to = cal.mode === "month" ? gridEnd : new Date(y, m + 1, 1);
    try {
      cal.events = (await api("GET", `/api/events?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}&scope=${cal.scope}`))
        .map((e) => ({ ...e, s: new Date(e.start), e: new Date(e.end) }));
    } catch (e) { fail(e); cal.events = []; }
    const body = $("[data-body]", v);
    if (cal.mode === "month") renderMonth(body, gridStart, gridEnd, m, render);
    else renderAgenda(body, first, new Date(y, m + 1, 1), render);
    if (cal.openId) { const e = cal.events.find((x) => x.id === cal.openId); cal.openId = null; if (e && !$(".sheet:not(.closing)")) showEvent(e, render); }
  };
  liveView = { kinds: ["event", "groups"], fn: () => { if (!$(".sheet")) render(); } };
  await render();
}

function eventsOn(day) {
  const ds = day.getTime(), de = addDays(day, 1).getTime();
  return cal.events.filter((e) => e.s.getTime() < de && (e.e.getTime() > ds || e.s.getTime() >= ds));
}
const evColor = (e) => e.group_color || S.general?.color || "var(--ink-2)";
const missText = (n) => (n === 1 ? t("vol_missing1") : t("vol_missing").replace("{n}", n));
const evMiss = (e) => (e.missing > 0 && e.e > new Date() ? e.missing : 0);

function renderMonth(body, gridStart, gridEnd, month, rerender) {
  const today = startOfDay(new Date());
  const dows = [...Array(7)].map((_, i) => fmt(addDays(new Date(2024, 0, 1), i), { weekday: "short" }));
  let html = `<div class="cal rv">${dows.map((d) => `<div class="dow">${esc(d)}</div>`).join("")}`;
  for (let d = gridStart; d < gridEnd; d = addDays(d, 1)) {
    const evs = eventsOn(d);
    const cls = [d.getMonth() !== month && "out", sameDay(d, today) && "today", sameDay(d, cal.sel) && "sel"].filter(Boolean).join(" ");
    html += `<div class="day ${cls}" data-d="${d.getTime()}"><div class="num">${d.getDate()}</div>
      ${evs.slice(0, 3).map((e) => { const ms = e.my_signup, mine = ms && !ms.all_day && sameDay(new Date(ms.start), d); return `<button class="ev ${ms ? "me" : ""}" data-e="${e.id}" style="--c:${esc(evColor(e))}">${ms ? icon("check") : ""}${mine ? `<span class="tm">${fmtTime(new Date(ms.start))}</span>` : e.all_day ? "" : `<span class="tm">${fmtTime(e.s)}</span>`}<span class="tt">${e.private ? icon("lock") : ""}${esc(e.title)}</span>${evMiss(e) ? `<span class="pc need">${evMiss(e)}</span>` : e.participants?.length ? `<span class="pc">${e.participants.length}</span>` : ""}</button>`; }).join("")}
      ${evs.length > 3 ? `<div class="more">+${evs.length - 3} ${t("more")}</div>` : ""}
      <div class="dots">${evs.slice(0, 4).map((e) => `<i style="--c:${esc(evColor(e))}"></i>`).join("")}</div></div>`;
  }
  html += `</div><div class="mtop agenda" data-dayagenda style="flex-direction:column;margin-top:14px"></div>`;
  body.innerHTML = html;
  const dayAgenda = () => {
    const el = $("[data-dayagenda]", body);
    const evs = eventsOn(cal.sel);
    el.innerHTML = dayBlock(cal.sel, evs, true);
    bindAgendaItems(el, rerender);
  };
  $$(".day", body).forEach((c) => (c.onclick = (e) => {
    if (e.target.closest(".ev")) return;
    const d = new Date(+c.dataset.d);
    const mobile = matchMedia("(max-width: 760px)").matches;
    if (!mobile && sameDay(d, cal.sel) && canCreateEvents()) return eventForm(null, d, rerender);
    cal.sel = d;
    $$(".day.sel", body).forEach((x) => x.classList.remove("sel"));
    c.classList.add("sel");
    dayAgenda();
  }));
  $$(".ev", body).forEach((b) => (b.onclick = () => showEvent(cal.events.find((e) => e.id == b.dataset.e), rerender)));
  dayAgenda();
}

function dayBlock(day, evs, showEmpty = false) {
  if (!evs.length && !showEmpty) return "";
  const today = sameDay(day, new Date());
  return `<div class="dayblock ${today ? "today" : ""}">
    <div class="dlabel"><b>${day.getDate()}</b><span>${esc(fmt(day, { weekday: "short", month: "short" }))}</span>${evs.length && canShareDay() ? `<button class="ibtn xs ghost dshare" data-dshare="${ymd(day)}" title="${t("share_day")}" aria-label="${t("share_day")}">${icon("share")}</button>` : ""}</div>
    <div class="items">${evs.length ? evs.map((e, i) => `<button class="item rv" style="--i:${i};--c:${esc(evColor(e))}" data-e="${e.id}"><span class="bar"></span>
      <span><div class="t">${e.private ? `<span class="lockt" title="${t("ev_private_short")}">${icon("lock")}</span>` : ""}${esc(e.title)}</div><div class="s">
        <span>${icon("clock")}${e.all_day ? t("all_day") : `${fmtTime(e.s)} – ${fmtTime(e.e)}`}</span>
        ${e.location ? `<span>${icon("pin")}${isURL(e.location) ? t("map") : esc(e.location)}</span>` : ""}
        <span class="gtag" style="--c:${esc(evColor(e))}"><i></i>${esc(e.group_name || t("general"))}</span>
        ${e.created_by && e.author ? `<span class="by">${avatar(e.created_by, { name: e.author }, "xs")}${t("by")} ${esc(e.author)}</span>` : ""}
        ${e.participants?.length ? `<span>${icon("groups")}${e.participants.length}</span>` : ""}
        ${evMiss(e) ? `<span class="need">${icon("alert")}${t("vol_title")}: ${missText(evMiss(e))}</span>` : ""}
      </div>${e.my_signup ? `<div class="mybook">${icon("check")}${t("you_are_booked")}: ${esc(slotText(e.my_signup))}</div>` : ""}</span>${e.via_ai ? `<span class="badge acc">AI</span>` : "<span></span>"}</button>`).join("")
      : `<div class="muted" style="padding:10px 0">${t("no_events")}</div>`}</div></div>`;
}
function bindAgendaItems(el, rerender) {
  $$("[data-dshare]", el).forEach((b) => (b.onclick = () => {
    const d = new Date(b.dataset.dshare + "T12:00");
    const g = cal.scope !== "all" ? groupById(+cal.scope) : null;
    publicLinkSheet({ path: `/api/day-link?day=${b.dataset.dshare}&scope=${cal.scope}`, title: fmt(d, { weekday: "long", day: "numeric", month: "long" }) + (g ? ` · ${g.name}` : ""), hint: t(g ? "public_day_hint_g" : "public_day_hint") });
  }));
  $$("[data-e]", el).forEach((b) => (b.onclick = () => showEvent(cal.events.find((e) => e.id == b.dataset.e), rerender)));
}

function renderAgenda(body, from, to, rerender) {
  let html = "";
  for (let d = from; d < to; d = addDays(d, 1)) html += dayBlock(d, eventsOn(d));
  body.innerHTML = `<div class="agenda">${html || `<div class="empty"><span class="serif">${t("no_events")}</span></div>`}</div>`;
  bindAgendaItems(body, rerender);
}

function showEvent(e, rerender) {
  if (!e) return;
  const when = e.all_day
    ? fmt(e.s, { weekday: "long", day: "numeric", month: "long" }) + (sameDay(e.s, e.e) ? "" : " → " + fmt(e.e, { day: "numeric", month: "long" }))
    : sameDay(e.s, e.e)
      ? `${fmt(e.s, { weekday: "long", day: "numeric", month: "long" })}, ${fmtTime(e.s)} – ${fmtTime(e.e)}`
      : `${fmt(e.s, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })} → ${fmt(e.e, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}`;
  const s = sheet({
    title: e.title,
    body: `<div class="ev-detail">
    ${e.created_by ? `<div class="ev-author">${avatar(e.created_by, { name: e.author }, "md")}<span class="who"><small>${t("created_by")}</small><b>${esc(e.author || "—")}</b></span>
      ${e.created_by !== S.me.id && e.author_username ? `<button class="btn sm" data-dm>${icon("chat")}${t("write_to")}</button>` : ""}</div>` : ""}
    <div class="meta">
      <div>${icon("clock")}<span>${esc(when)}</span></div>
      ${e.location ? `<div>${icon("pin")}<span>${isURL(e.location) ? "" : `${esc(e.location)} · `}<a href="${esc(mapsURL(e.location))}" target="_blank" rel="noopener">${t("open_map")}</a></span></div>` : ""}
      <div>${icon("groups")}<span class="gtag" style="--c:${esc(evColor(e))}"><i></i>${esc(e.group_name || t("general"))}</span></div>
      ${e.private ? `<div class="priv">${icon("lock")}<span>${t("ev_private_line")}</span></div>` : ""}
      ${e.via_ai ? `<div>${icon("ai")}<span><span class="badge acc">${t("via_ai")}</span></span></div>` : ""}
    </div>
    ${shiftsHTML(e)}
    <div class="parts"><div class="parts-h"><b>${t("participants")}</b><span class="mono muted">${e.participants?.length || 0}</span><span style="flex:1"></span>
      ${e.my_signup ? `<button class="btn sm" data-book>${icon("edit")}${t("change_booking")}</button><button class="btn sm ghost danger" data-unbook>${icon("x")}</button>` : `<button class="btn sm primary" data-book>${icon("check")}${t("book")}</button>`}</div>
      ${e.participants?.length ? `<div class="plist">${e.participants.map((p) => `<div class="prow ${p.user_id === S.me.id ? "me" : ""}">${avatar(p.user_id, p)}<span class="pn">${esc(p.name)}${p.note ? `<small>${esc(p.note)}</small>` : ""}</span><span class="mono pt">${esc(slotText(p, e))}</span>${e.can_edit && p.user_id !== S.me.id ? `<button class="ibtn sm ghost danger" data-rmp="${p.user_id}" title="${t("delete")}">${icon("x")}</button>` : ""}</div>`).join("")}</div>` : `<p class="muted" style="margin:6px 0 0">${t("nobody_yet")}</p>`}
    </div>
    ${e.description ? `<div class="md">${md(e.description)}</div>` : ""}</div>`,
    foot: e.can_edit ? `<button class="btn danger left" data-del>${icon("trash")}${t("delete")}</button>${e.private ? "" : `<button class="btn" data-pub>${icon("share")}${t("share")}</button>`}<button class="btn primary" data-edit>${icon("edit")}${t("edit")}</button>` : "",
  });
  $("[data-edit]", s.el)?.addEventListener("click", () => { s.close(); eventForm(e, null, rerender); });
  $("[data-pub]", s.el)?.addEventListener("click", () => publicLinkSheet({ path: `/api/events/${e.id}/link`, title: e.title, hint: t("public_event_hint") }));
  const reopen = async (updated) => { s.close(); await rerender(); showEvent({ ...updated, s: new Date(updated.start), e: new Date(updated.end) }, rerender); };
  $("[data-book]", s.el)?.addEventListener("click", () => signupForm(e, reopen));
  $("[data-dm]", s.el)?.addEventListener("click", () => openDM(e.created_by, t("ask_about").replace("{t}", e.title)));
  $$("[data-sjoin]", s.el).forEach((b) => (b.onclick = async () => { try { reopen(await api("PUT", `/api/events/${e.id}/shifts/${b.dataset.sjoin}/signup`)); toast(t("vol_joined"), false, null, "check"); } catch (err) { fail(err); } }));
  $$("[data-sleave]", s.el).forEach((b) => (b.onclick = async () => { try { reopen(await api("DELETE", `/api/events/${e.id}/shifts/${b.dataset.sleave}/signup`)); toast(t("vol_left")); } catch (err) { fail(err); } }));
  $$("[data-srm]", s.el).forEach((b) => (b.onclick = async () => { const [sid, uid] = b.dataset.srm.split(":"); try { reopen(await api("DELETE", `/api/events/${e.id}/shifts/${sid}/signups/${uid}`)); } catch (err) { fail(err); } }));
  $("[data-unbook]", s.el)?.addEventListener("click", async () => { try { reopen(await api("DELETE", `/api/events/${e.id}/signup`)); toast(t("booking_cancelled")); } catch (err) { fail(err); } });
  $$("[data-rmp]", s.el).forEach((b) => (b.onclick = async () => { try { reopen(await api("DELETE", `/api/events/${e.id}/signups/${b.dataset.rmp}`)); } catch (err) { fail(err); } }));
  $("[data-del]", s.el)?.addEventListener("click", async () => {
    if (!(await confirmSheet(e.title))) return;
    try { await api("DELETE", `/api/events/${e.id}`); toast(t("deleted")); s.close(); rerender(); } catch (err) { fail(err); }
  });
}

function shiftTime(sh, e) {
  const s = new Date(sh.start), en = new Date(sh.end);
  const day = sameDay(new Date(e.start), new Date(e.end)) ? "" : fmt(s, { weekday: "short", day: "numeric", month: "short" }) + " · ";
  return `${day}${fmtTime(s)} – ${fmtTime(en)}`;
}
function shiftsHTML(e) {
  if (!e.shifts?.length) return "";
  const miss = e.missing > 0 && new Date(e.end) > new Date() ? e.missing : 0;
  return `<div class="shifts"><div class="parts-h"><b>${t("vol_title")}</b>${miss ? `<span class="need">${icon("alert")}${missText(miss)}</span>` : `<span class="okc">${icon("check")}${t("vol_full")}</span>`}</div>
    ${e.shifts.map((sh) => {
      const have = sh.people.length, full = have >= sh.needed, past = new Date(sh.start) < new Date();
      return `<div class="shift ${full ? "full" : ""}"><div class="sh-top"><span class="sh-time">${esc(shiftTime(sh, e))}</span>${sh.label ? `<span class="sh-label">${esc(sh.label)}</span>` : ""}<span class="grow"></span>
        <span class="sh-count ${full ? "ok" : "bad"}">${have}/${sh.needed}</span>
        ${past ? "" : sh.mine ? `<button class="btn sm ghost" data-sleave="${sh.id}">${t("vol_leave")}</button>` : `<button class="btn sm ${full ? "" : "primary"}" data-sjoin="${sh.id}">${icon("check")}${t("vol_join")}</button>`}</div>
        <div class="sh-people">${sh.people.map((p) => `<span class="sp ${p.user_id === S.me.id ? "me" : ""}">${avatar(p.user_id, p, "xs")}${esc(p.name)}${e.can_edit && p.user_id !== S.me.id ? `<button class="ibtn xs ghost danger" data-srm="${sh.id}:${p.user_id}" title="${t("delete")}">${icon("x")}</button>` : ""}</span>`).join("") || `<span class="muted">${t("vol_none")}</span>`}</div></div>`;
    }).join("")}</div>`;
}

// openDM opens (or starts) the private conversation with someone, optionally with a first line ready.
async function openDM(uid, prefill = "") {
  try {
    const r = await api("POST", "/api/dm", { user_id: uid });
    chat.prefill = prefill;
    closeAllPages();
    if (location.hash === `#/chat/${r.room_id}`) route(); else location.hash = `#/chat/${r.room_id}`;
  } catch (e) { fail(e); }
}

const isURL = (s) => /^https:\/\/\S+$/.test((s || "").trim());
const mapsURL = (place) => isURL(place) ? place.trim() : "https://www.google.com/maps/search/?api=1&query=" + encodeURIComponent(place.trim());
const ymd = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
function canShareDay() {
  if (cal.scope === "all") return isAdmin();
  return /^\d+$/.test(cal.scope) && (isAdmin() || S.myGroups.includes(+cal.scope));
}

// Public, read-only link for people without an account (event or whole day).
async function publicLinkSheet({ path, title, hint }) {
  let link;
  try { link = await api("POST", path); } catch (e) { return fail(e); }
  const text = `${title}\n${link.url}`;
  const s = sheet({
    title: t("public_link"),
    body: `<p class="muted" style="margin-top:0">${esc(hint)}</p>
      <div class="publink"><input class="input" readonly value="${esc(link.url)}" data-url></div>
      <div class="pubacts">
        <button class="btn primary" data-cp>${icon("copy")}${t("copy_link")}</button>
        <a class="btn" href="https://wa.me/?text=${encodeURIComponent(text)}" target="_blank" rel="noopener">${icon("send")}WhatsApp</a>
        ${navigator.share ? `<button class="btn" data-sh>${icon("share")}${t("share")}</button>` : ""}
        <a class="btn ghost" href="${esc(link.url)}" target="_blank" rel="noopener">${icon("globe")}${t("open_preview")}</a>
      </div>`,
    foot: `<button class="btn ghost danger left" data-off>${icon("x")}${t("disable_link")}</button><button class="btn" data-c>${t("close")}</button>`,
  });
  const inp = $("[data-url]", s.el);
  inp.onclick = () => inp.select();
  $("[data-cp]", s.el).onclick = async () => {
    try { await navigator.clipboard.writeText(link.url); } catch { inp.select(); document.execCommand?.("copy"); }
    toast(t("copied"));
  };
  $("[data-sh]", s.el)?.addEventListener("click", () => navigator.share({ title, text: title, url: link.url }).catch(() => {}));
  $("[data-c]", s.el).onclick = s.close;
  $("[data-off]", s.el).onclick = async () => {
    if (!(await confirmSheet(t("disable_link_q")))) return;
    try { await api("DELETE", path); toast(t("link_disabled")); s.close(); } catch (e) { fail(e); }
  };
}

function slotText(p, ev) {
  if (p.all_day) return t("whole_event");
  const s = new Date(p.start), e = new Date(p.end);
  const multi = ev ? !sameDay(new Date(ev.start), new Date(ev.end)) : !sameDay(s, e);
  return multi || !sameDay(s, e)
    ? `${fmt(s, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })} – ${fmt(e, sameDay(s, e) ? { hour: "2-digit", minute: "2-digit" } : { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}`
    : `${fmtTime(s)} – ${fmtTime(e)}`;
}

function signupForm(e, done) {
  const ms = e.my_signup, es = new Date(e.start), ee = new Date(e.end);
  const slot = ms && !ms.all_day;
  const s0 = slot ? new Date(ms.start) : es, e0 = slot ? new Date(ms.end) : ee;
  const form = h(`<form>
    <p class="muted" style="margin-top:0">${t("booking_hint")}</p>
    <div class="seg" style="margin-bottom:14px"><button type="button" data-m="all" class="${slot ? "" : "on"}">${t("whole_event")}</button><button type="button" data-m="slot" class="${slot ? "on" : ""}">${icon("clock")}${t("time_slot")}</button></div>
    <div class="row slot ${slot ? "" : "hidden"}">
      <label class="field"><span>${t("from")}</span><input class="input" type="datetime-local" name="start" value="${localInput(s0)}" min="${localInput(es)}" max="${localInput(ee)}"></label>
      <label class="field"><span>${t("to")}</span><input class="input" type="datetime-local" name="end" value="${localInput(e0)}" min="${localInput(es)}" max="${localInput(ee)}"></label>
    </div>
    <label class="field"><span>${t("note")} · ${t("optional")}</span><input class="input" name="note" maxlength="300" value="${esc(ms?.note)}" placeholder="${t("note_hint")}"></label>
  </form>`);
  let mode = slot ? "slot" : "all";
  $$("[data-m]", form).forEach((b) => (b.onclick = () => { mode = b.dataset.m; $$("[data-m]", form).forEach((x) => x.classList.toggle("on", x === b)); $(".slot", form).classList.toggle("hidden", mode !== "slot"); }));
  const s = sheet({ title: `${t("book")} · ${e.title}`, body: form, foot: `<button class="btn" data-c>${t("cancel")}</button><button class="btn primary" data-ok>${icon("check")}${t("save")}</button>` });
  $("[data-c]", s.el).onclick = s.close;
  const submit = async () => {
    const f = new FormData(form);
    const body = { note: f.get("note") };
    if (mode === "all") body.all_day = true;
    else {
      const a = new Date(f.get("start")), b = new Date(f.get("end"));
      if (isNaN(a) || isNaN(b) || b <= a) return toast(t("bad_slot"), true);
      Object.assign(body, { all_day: false, start: a.toISOString(), end: b.toISOString() });
    }
    try { const r = await api("PUT", `/api/events/${e.id}/signup`, body); toast(t("booking_saved")); s.close(); done(r); } catch (err) { fail(err); }
  };
  $("[data-ok]", s.el).onclick = submit;
  form.onsubmit = (ev) => { ev.preventDefault(); submit(); };
}

function eventForm(ev, day, rerender) {
  const editable = isAdmin() ? S.groups : S.groups.filter((g) => g.is_member);
  const base = day ? new Date(day) : new Date();
  if (!ev) { const n = new Date(); base.setHours(sameDay(base, n) ? Math.min(n.getHours() + 1, 23) : 9, 0, 0, 0); }
  const s0 = ev ? new Date(ev.start) : base;
  const e0 = ev ? new Date(ev.end) : new Date(base.getTime() + 3600e3);
  const defGroup = ev ? ev.group_id : (+cal.scope && editable.some((g) => g.id == cal.scope) ? +cal.scope : isAdmin() ? null : editable[0]?.id);
  const allDay = ev?.all_day || false;
  const form = h(`<form>
    <label class="field"><span>${t("title")}</span><input class="input" name="title" required maxlength="200" value="${esc(ev?.title)}"></label>
    <label class="check"><input type="checkbox" name="all_day" ${allDay ? "checked" : ""}>${t("all_day")}</label>
    <div class="row">
      <label class="field"><span>${t("start")}</span><input class="input" name="start" type="${allDay ? "date" : "datetime-local"}" required value="${allDay ? dateInput(s0) : localInput(s0)}"></label>
      <label class="field"><span>${t("end")}</span><input class="input" name="end" type="${allDay ? "date" : "datetime-local"}" required value="${allDay ? dateInput(e0) : localInput(e0)}"></label>
    </div>
    <label class="field"><span>${t("group")}</span><select class="input" name="group">
      ${isAdmin() ? `<option value="">${t("general")}</option>` : ""}
      ${editable.map((g) => `<option value="${g.id}" ${g.id === defGroup ? "selected" : ""}>${esc(g.name)}</option>`).join("")}
    </select></label>
    <div class="privbox ${defGroup ? "" : "hidden"}" data-privrow><label class="check"><input type="checkbox" name="private" ${(ev ? ev.private : groupById(defGroup)?.private) ? "checked" : ""}>${icon("lock")}${t("ev_private")}</label>
      <small class="hint">${t("ev_private_hint")}</small></div>
    <label class="field"><span>${t("location")} · ${t("optional")}</span><input class="input" name="location" maxlength="200" value="${esc(ev?.location)}"></label>
    <div class="field vol-ed"><span>${t("vol_title")} · ${t("optional")}</span>
      <small class="hint" style="margin:0 0 8px">${t("vol_hint")}</small>
      <div class="shift-rows" data-shifts></div>
      <button type="button" class="btn sm" data-addshift>${icon("plus")}${t("vol_add")}</button></div>
    <div class="field"><span>${t("description")}</span>
      <div class="ev-ed hl-ed md hidden" data-evdesc></div>
      <textarea class="input" name="description" rows="9" placeholder="${t("description_hint")}">${esc(ev?.description)}</textarea>
      <small class="hint hidden" data-evhint>${t("ev_editor_hint")}</small></div>
  </form>`);
  // programme: the same visual editor as documents (stored as Markdown); plain text if it can't load
  let progEd = null;
  const ta = $("[name=description]", form), holder = $("[data-evdesc]", form);
  mountInlineEditor(holder, ta.value, { placeholder: t("ev_editor_ph") }).then((ed) => {
    if (!form.isConnected) return ed.destroy();
    progEd = ed;
    holder.classList.remove("hidden"); ta.classList.add("hidden"); $("[data-evhint]", form).classList.remove("hidden");
  }).catch(() => {});
  // private: only for a group's own events
  const gsel = $("[name=group]", form), privRow = $("[data-privrow]", form);
  // private groups: their events start private; sharing with everyone is a choice
  gsel.onchange = () => { privRow.classList.toggle("hidden", !gsel.value); $("[name=private]", form).checked = !!groupById(+gsel.value)?.private; };
  // volunteer shifts
  const shiftsEl = $("[data-shifts]", form);
  const shiftRow = (sh) => {
    const row = h(`<div class="shift-row" data-sid="${sh.id || ""}">
      <input class="input" type="datetime-local" name="ss" value="${localInput(new Date(sh.start))}" aria-label="${t("from")}">
      <span class="muted">–</span>
      <input class="input" type="datetime-local" name="se" value="${localInput(new Date(sh.end))}" aria-label="${t("to")}">
      <label class="sh-n"><input class="input" type="number" name="sn" min="1" max="50" value="${sh.needed || 2}" aria-label="${t("vol_needed")}"><span>${t("vol_people")}</span></label>
      <input class="input sh-l" name="sl" maxlength="80" placeholder="${t("vol_label_ph")}" value="${esc(sh.label || "")}">
      <button type="button" class="ibtn sm ghost danger" data-rmshift aria-label="${t("delete")}" title="${t("delete")}">${icon("x")}</button></div>`);
    $("[data-rmshift]", row).onclick = () => row.remove();
    shiftsEl.append(row);
  };
  (ev?.shifts || []).forEach(shiftRow);
  $("[data-addshift]", form).onclick = () => {
    const v = (n, def) => { const x = $(`[name=${n}]`, form).value; return x.length === 10 ? x + def : x; };
    const st = new Date(v("start", "T09:00")), en = new Date(v("end", "T13:00"));
    const last = $$(".shift-row", shiftsEl).pop();
    const from = last ? new Date($("[name=se]", last).value) : st;
    const to = last ? new Date(Math.min(from.getTime() + (en - st), en.getTime())) : en;
    shiftRow({ start: isNaN(from) ? new Date() : from, end: isNaN(to) || to <= from ? new Date(from.getTime() + 3 * 3600e3) : to, needed: 2 });
  };
  const ad = $("[name=all_day]", form);
  ad.onchange = () => {
    ["start", "end"].forEach((n) => {
      const i = $(`[name=${n}]`, form); const v = i.value;
      i.type = ad.checked ? "date" : "datetime-local";
      i.value = ad.checked ? v.slice(0, 10) : (v.length === 10 ? v + (n === "start" ? "T09:00" : "T10:00") : v);
    });
  };
  const s = sheet({ title: ev ? t("edit_event") : t("new_event"), body: form, onClose: () => progEd?.destroy(), foot: `<button class="btn" data-c>${t("cancel")}</button><button class="btn primary" data-ok>${icon("check")}${t("save")}</button>` });
  $("[data-c]", s.el).onclick = s.close;
  const submit = async () => {
    if (!form.reportValidity()) return;
    const f = new FormData(form);
    const all = ad.checked;
    const toISO = (v, end) => (all ? new Date(v + (end ? "T23:59:59" : "T00:00:00")) : new Date(v)).toISOString();
    const g = f.get("group");
    const description = progEd ? await progEd.getMd() : f.get("description");
    const body = { title: f.get("title"), start: toISO(f.get("start")), end: toISO(f.get("end"), true), all_day: all, location: f.get("location"), description };
    if (g) body.group_id = +g; else body.clear_group = true;
    body.private = !!g && $("[name=private]", form).checked;
    const shifts = $$(".shift-row", shiftsEl).map((r) => ({ id: +r.dataset.sid || undefined, start: new Date($("[name=ss]", r).value), end: new Date($("[name=se]", r).value), needed: +$("[name=sn]", r).value, label: $("[name=sl]", r).value.trim() }));
    if (shifts.some((x) => isNaN(x.start) || isNaN(x.end) || x.end <= x.start || !(x.needed >= 1 && x.needed <= 50))) return toast(t("vol_bad"), true);
    body.shifts = shifts.map((x) => ({ ...x, start: x.start.toISOString(), end: x.end.toISOString() }));
    try {
      const saved = ev ? await api("PATCH", `/api/events/${ev.id}`, body) : await api("POST", "/api/events", body);
      const sd = startOfDay(new Date(saved.start));
      cal.cur = sd; cal.sel = sd;
      const hidden = (cal.scope === "mine" && saved.group_id && !S.myGroups.includes(saved.group_id)) || (cal.scope !== "all" && cal.scope !== "mine" && saved.group_id && saved.group_id != cal.scope);
      if (hidden) { cal.scope = "all"; store.set("calScope", "all"); }
      toast(t("saved")); s.close(); rerender();
    } catch (e) { fail(e); }
  };
  $("[data-ok]", s.el).onclick = submit;
  form.onsubmit = (e) => { e.preventDefault(); submit(); };
}

/* ═════════ groups ═════════ */
async function viewGroups(v) {
  liveView = { kinds: ["groups"], fn: () => { if (!$(".sheet")) render(); } };
  const render = async () => {
    await loadGroups();
    v.innerHTML = pageHead("", "", "",
      isAdmin() ? `<button class="btn primary" data-new>${icon("plus")}<span>${t("new_group")}</span></button>` : "") +
      (S.groups.length ? `<div class="grid-cards">${S.groups.map((g, i) => `
        <div class="card gcard rv ${g.image_v ? "has-img" : ""}" style="--i:${i};--c:${esc(g.color)}" data-open="${g.id}" tabindex="0" role="button" aria-label="${esc(g.name)}">
          ${g.image_v ? `<div class="gcover"><img alt="" loading="lazy" src="${groupImg(g)}"></div>` : ""}
          <div style="display:flex;justify-content:space-between;align-items:center"><span class="mono muted">${g.members} ${g.members === 1 ? t("member") : t("members")}${g.private ? ` · <span class="lockt" title="${t("group_private_short")}">${icon("lock")}</span>` : ""}</span>
          ${isAdmin() || g.is_leader ? `<span style="display:flex;gap:2px"><button class="btn sm ghost act" data-mem="${g.id}" title="${t("manage_members")}">${icon("groups")}<span class="lbl">${t("manage_members")}</span></button><button class="btn sm ghost act" data-edit="${g.id}" title="${t("edit")}">${icon("edit")}<span class="lbl">${t("edit")}</span></button></span>` : ""}</div>
          <div class="leader-line">${icon("crown")}${g.leader_id ? `<span>${t("leader")}: <b>${esc(g.leader_id === S.me.id ? t("you_obj") : g.leader_name)}</b></span>` : `<span class="muted">${t("no_leader")}</span>`}</div>
          <h3><span class="sw"></span>${esc(g.name)}</h3><p>${esc(g.description)}</p>
          <div class="foot">
            <span style="display:flex;gap:2px"><a class="btn sm ghost act" href="#/calendar" data-cal="${g.id}" title="${t("nav_calendar")}">${icon("calendar")}<span class="lbl">${t("nav_calendar")}</span></a>${g.is_member || isAdmin() ? `<a class="btn sm ghost act" href="#/chat/${g.id}" title="${t("nav_chat")}">${icon("chat")}<span class="lbl">${t("nav_chat")}</span></a>` : ""}</span>
            ${g.is_member ? `<button class="btn sm" data-leave="${g.id}">${icon("check")}${t("joined")}</button>` : g.private ? `<span class="badge">${icon("lock")}${t("invite_only")}</span>` : `<button class="btn sm primary" data-join="${g.id}">${icon("plus")}${t("join")}</button>`}
          </div></div>`).join("")}</div>`
        : `<div class="empty"><span class="serif">${t("no_groups")}</span></div>`);
    $("[data-new]", v)?.addEventListener("click", () => groupForm(null, render));
    $$("[data-edit]", v).forEach((b) => (b.onclick = () => groupForm(groupById(+b.dataset.edit), render)));
    $$("[data-mem]", v).forEach((b) => (b.onclick = () => membersSheet(groupById(+b.dataset.mem), render)));
    $$("[data-join]", v).forEach((b) => (b.onclick = async () => { try { await api("POST", `/api/groups/${b.dataset.join}/join`); render(); } catch (e) { fail(e); } }));
    $$("[data-leave]", v).forEach((b) => {
      b.onmouseenter = () => (b.innerHTML = `${icon("x")}${t("leave")}`);
      b.onmouseleave = () => (b.innerHTML = `${icon("check")}${t("joined")}`);
      b.onclick = async () => { try { await api("POST", `/api/groups/${b.dataset.leave}/leave`); render(); } catch (e) { fail(e); } };
    });
    $$("[data-cal]", v).forEach((a) => (a.onclick = () => { cal.scope = a.dataset.cal; store.set("calScope", cal.scope); }));
    $$("[data-open]", v).forEach((c) => {
      const open = (e) => { if (e.target.closest("a,button,input,select")) return; groupPage(groupById(+c.dataset.open), render); };
      c.onclick = open;
      c.onkeydown = (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(e); } };
    });
  };
  await render();
}

// Cover: the group photo fills the top of the page (sticky). The header block (name,
// description, details, buttons) is the first part of the content: it starts on the photo and,
// scrolling up, moves with the rest and turns into a normal part of the page while the photo fades.
function heroImage(sheetEl, src) {
  const sb = $(".sb", sheetEl), sbi = $(".sbi", sheetEl);
  let hero = $(".ghero", sb);
  if (!src) { hero?.remove(); sheetEl.classList.remove("has-hero"); sbi.style.marginTop = ""; return; }
  const layout = () => {
    const info = $(".ghero-info", sheetEl);
    const H = Math.round(Math.max(sb.clientHeight * 0.74, (info?.offsetHeight || 0) + 160));
    hero.style.height = H + "px";
    const ih = info ? info.offsetHeight : 0;
    sbi.style.setProperty("--ih", ih + "px");
    sbi.style.marginTop = -(ih + 16) + "px"; // the header sits on the bottom of the photo
  };
  if (!hero) {
    hero = h(`<div class="ghero"><img alt=""></div>`);
    sb.prepend(hero);
    sheetEl.classList.add("has-hero");
    const img = $("img", hero);
    img.onload = layout;
    addEventListener("resize", layout);
    sb.addEventListener("scroll", () => {
      const k = Math.min(1, sb.scrollTop / ((hero.offsetHeight || 1) * 0.55));
      sheetEl.style.setProperty("--k", k.toFixed(3));
      img.style.opacity = String(1 - k * 0.92);
      img.style.transform = `scale(${1 + k * 0.08})`;
      $(".ghero-info", sheetEl)?.classList.toggle("on-page", k > 0.5);
    }, { passive: true });
  }
  const img = $("img", hero);
  if (img.getAttribute("src") !== src) img.src = src;
  requestAnimationFrame(layout);
}

// Full-page overview of a group: what you see depends on membership and role.
async function groupPage(g, rerender) {
  if (!g) return;
  const body = h(`<div class="gpage"><div class="typing"><i></i><i></i><i></i></div></div>`);
  const s = sheet({ title: g.name, body, wide: true, onClose: rerender });
  const render = async () => {
    await loadGroups();
    g = groupById(g.id);
    if (!g) return s.close();
    const member = g.is_member, admin = isAdmin(), manage = admin || g.is_leader, inside = member || admin;
    const now = new Date(), until = addDays(now, 90);
    const [members, evs, docs] = await Promise.all([
      inside ? api("GET", `/api/groups/${g.id}/members`).catch(() => []) : Promise.resolve([]),
      api("GET", `/api/events?from=${encodeURIComponent(now.toISOString())}&to=${encodeURIComponent(until.toISOString())}&scope=${g.id}`).catch(() => []),
      inside ? api("GET", `/api/docs?group=${g.id}`).catch(() => null) : Promise.resolve(null),
    ]);
    const events = evs.filter((e) => e.group_id === g.id).slice(0, 5).map((e) => ({ ...e, s: new Date(e.start), e: new Date(e.end) }));
    const allDocs = docs ? [...docs.docs, ...docs.shared] : [];
    const cover = !!g.image_v;
    const head = `
      <div class="ghead" style="--c:${esc(g.color)}">
        <div class="gname">${esc(g.name)}</div>
        <div class="gmeta"><span><i class="sw"></i>${g.members} ${g.members === 1 ? t("member") : t("members")}</span>
          <span>${icon("crown")}${g.leader_id ? `${t("leader")}: <b>${esc(g.leader_id === S.me.id ? t("you_obj") : g.leader_name)}</b>` : `<span class="muted">${t("no_leader")}</span>`}</span>
          ${g.private ? `<span class="badge">${icon("lock")}${t("group_private_short")}</span>` : ""}
          ${member ? `<span class="badge acc">${icon("check")}${t("joined")}</span>` : ""}</div>
        <div class="gacts">
          ${member || g.private ? "" : `<button class="btn primary" data-join>${icon("plus")}${t("join")}</button>`}
          ${inside ? `<a class="btn" href="#/chat/${g.id}" data-go>${icon("chat")}${t("nav_chat")}</a>` : ""}
          <a class="btn" href="#/calendar" data-gcal>${icon("calendar")}${t("nav_calendar")}</a>
          ${member && !g.leader_id ? `<button class="btn" data-vote>${icon("crown")}${t("vote")}</button>` : ""}
          ${manage ? `<button class="btn ghost" data-gedit>${icon("edit")}${t("edit")}</button>` : ""}
        </div>
      </div>`;
    const desc = g.description ? (cover ? `<p class="ghero-desc">${esc(g.description)}</p>` : `<section class="gsec"><h4>${t("group_desc")}</h4><p class="gdesc">${esc(g.description)}</p></section>`) : "";
    body.innerHTML = `
      ${cover ? `<div class="ghero-info">${head.replace('<div class="gmeta">', `${desc}<div class="gmeta">`)}</div>` : head + desc}
      <section class="gsec"><h4>${t("upcoming_events")}</h4>
        ${events.length ? `<div class="glist">${events.map((e) => `<button class="gev" data-ev="${e.id}"><span class="gd"><b>${e.s.getDate()}</b><small>${esc(fmt(e.s, { month: "short" }).replace(".", ""))}</small></span>
          <span class="gt"><b>${esc(e.title)}</b><small>${e.all_day ? t("all_day") : `${fmtTime(e.s)} – ${fmtTime(e.e)}`}${e.location ? ` · ${esc(e.location)}` : ""}</small></span>
          ${e.my_signup ? `<span class="badge acc">${icon("check")}</span>` : ""}${icon("right")}</button>`).join("")}</div>`
          : `<p class="muted">${t("no_upcoming")}</p>`}
      </section>
      <section class="gsec"><h4>${t("members_title")}${inside ? ` <span class="muted">· ${members.length}</span>` : ""}${manage ? `<button class="btn sm ghost" data-mem>${icon("groups")}${t("manage_members")}</button>` : ""}</h4>
        ${inside ? `<div class="gmembers">${members.map((u) => `<div class="gm">${avatar(u.id, u)}<span>${esc(u.name || u.username)}${u.id === S.me.id ? ` <small class="muted">(${t("you")})</small>` : ""}${u.id === g.leader_id ? ` ${icon("crown")}` : ""}</span></div>`).join("") || `<p class="muted">—</p>`}</div>`
          : `<p class="muted">${t("join_to_see_members")}</p>`}
      </section>
      <section class="gsec"><h4>${t("room_docs")}${inside ? ` <span class="muted">· ${allDocs.length}</span>` : ""}</h4>
        ${inside ? (allDocs.length ? `<div class="list">${allDocs.map((d, i) => docRow({ ...d, owner: d.owner || S.me.name || S.me.username }, i, false)
          .replace('<div class="acts">', `<div class="acts">${manage && d.shares.includes(g.id) ? `<button class="btn sm ghost act danger" data-unshare="${d.id}">${icon("x")}<span class="lbl">${t("remove_from_group")}</span></button>` : ""}`)).join("")}</div>` : `<p class="muted">${t("no_shared")}</p>`)
          : `<p class="muted">${t("join_to_see_docs")}</p>`}
      </section>
      ${member ? `<div class="gleave"><button class="btn ghost danger" data-leave>${icon("x")}${t("leave_group")}</button></div>` : ""}`;
    heroImage(s.el, cover ? groupImg(g) : "");
    $("[data-join]", body)?.addEventListener("click", async () => { try { await api("POST", `/api/groups/${g.id}/join`); toast(t("joined")); render(); } catch (e) { fail(e); } });
    $("[data-leave]", body)?.addEventListener("click", async () => {
      if (!(await confirmSheet(`${t("leave_group")}: ${g.name}?`))) return;
      try { await api("POST", `/api/groups/${g.id}/leave`); render(); } catch (e) { fail(e); }
    });
    // leave the overview first (it is a history entry), then go
    const go = async (hash) => { s.close(); await histWait; location.hash = hash; };
    $("[data-gcal]", body).onclick = (e) => { e.preventDefault(); cal.scope = String(g.id); store.set("calScope", cal.scope); go("#/calendar"); };
    $("[data-go]", body)?.addEventListener("click", (e) => { e.preventDefault(); go(`#/chat/${g.id}`); });
    $("[data-vote]", body)?.addEventListener("click", () => voteSheet(g));
    $("[data-gedit]", body)?.addEventListener("click", () => groupForm(g, render));
    $("[data-mem]", body)?.addEventListener("click", () => membersSheet(g, render));
    $$(".gm", body).forEach((el) => {
      const uid = +$("[data-uid]", el)?.dataset.uid;
      if (!uid || uid === S.me.id) return;
      el.classList.add("tap"); el.title = t("write_to");
      el.onclick = () => openDM(uid);
    });
    $$("[data-ev]", body).forEach((b) => (b.onclick = () => {
      const e = events.find((x) => x.id == b.dataset.ev);
      cal.scope = String(g.id); store.set("calScope", cal.scope); cal.openId = e.id;
      go(`#/calendar/${ymd(e.s)}`);
    }));
    $$("[data-prev]", body).forEach((b) => (b.onclick = () => previewDoc(allDocs.find((d) => d.id == b.dataset.prev))));
    $$("[data-editdoc]", body).forEach((b) => (b.onclick = () => editDoc(allDocs.find((d) => d.id == b.dataset.editdoc), render)));
    $$("[data-unshare]", body).forEach((b) => (b.onclick = async () => {
      const d = allDocs.find((x) => x.id == b.dataset.unshare);
      if (!(await confirmSheet(`${d.name} — ${t("remove_from_group")}`))) return;
      try { await api("DELETE", `/api/docs/${d.id}/shares/${g.id}`); toast(t("saved")); render(); } catch (e) { fail(e); }
    }));
  };
  await render();
}

const groupImg = (g) => (g?.image_v ? `/api/groups/${g.id}/image?v=${g.image_v}` : "");
// "AI 7/10": assistant messages left today (only when a limit applies)
function aiCount(el, q) {
  if (!el) return;
  const show = S.ai && q && q.left >= 0;
  el.classList.toggle("hidden", !show);
  if (!show) return;
  el.classList.toggle("zero", q.left === 0);
  el.innerHTML = `${icon("ai")}<span>${q.left}/${q.limit}</span>`;
  el.title = `${t("ai_left_today")}: ${q.left} / ${q.limit} (${t("ai_by_" + q.by)})`;
}
const aiQuotaFor = (room = 0) => api("GET", `/api/ai/quota?room=${room}`).catch(() => null);

const usageText = (g) => {
  const q = (g.quota_mb || g.def_quota_mb) * 1048576;
  const lim = g.ai_daily >= 0 ? g.ai_daily : g.def_ai_daily || -1;
  return `${fmtSize(g.used)} / ${fmtSize(q)}${S.aiSwitch ? ` · AI ${g.ai_today}/${lim < 0 ? "∞" : lim}` : ""}`;
};
const generalImg = () => (S.general?.image_v ? `/api/groups/0/image?v=${S.general.image_v}` : "");
const generalColor = () => S.general?.color || "var(--ink)";
// group cover in the browser: the whole photo, longest side at most 1080 px, WebP when the
// browser can encode it (else JPEG; the server optimises it). No EXIF/GPS left.
async function shrinkImage(file, maxSide = 1080) {
  let src = await createImageBitmap(file, { imageOrientation: "from-image" }).catch(() => null);
  if (!src) {
    src = await new Promise((res, rej) => { const i = new Image(); i.onload = () => res(i); i.onerror = rej; i.src = URL.createObjectURL(file); });
  }
  const w0 = src.width, h0 = src.height, k = Math.min(1, maxSide / Math.max(w0, h0));
  const c = document.createElement("canvas");
  c.width = Math.max(1, Math.round(w0 * k)); c.height = Math.max(1, Math.round(h0 * k));
  const ctx = c.getContext("2d");
  ctx.fillStyle = "#fff"; ctx.fillRect(0, 0, c.width, c.height);
  ctx.drawImage(src, 0, 0, c.width, c.height);
  const enc = (type, q) => new Promise((r) => c.toBlob(r, type, q));
  let blob = await enc("image/webp", 0.72);
  if (!blob || blob.type !== "image/webp") blob = await enc("image/jpeg", 0.8); // Safari: no WebP encoder
  return blob;
}

function groupForm(g, rerender) {
  const free = freeColors(g);
  if (g?.color && !free.includes(g.color.toLowerCase())) free.unshift(g.color.toLowerCase()); // its own colour, even if not in the palette
  const color = (g?.color || free[0] || "").toLowerCase();
  const form = h(`<form>
    <label class="field"><span>${t("name")}</span><input class="input" name="name" required maxlength="80" value="${esc(g?.name)}"></label>
    <label class="field"><span>${t("group_desc")} · ${t("optional")}</span><textarea class="input" name="description" rows="3" style="min-height:80px">${esc(g?.description)}</textarea></label>
    ${isAdmin() ? `<div class="field"><span>${t("color")}</span><div class="colors">${free.length ? free.map((c, i) => `<input type="radio" name="color" id="c${i}" value="${c}" ${c === color ? "checked" : ""}><label for="c${i}" style="--c:${c}"></label>`).join("") : `<span class="muted">${t("colors_all_used")}</span>`}</div></div>` : ""}
    ${isAdmin() ? `<div class="privbox"><label class="check"><input type="checkbox" name="private" ${g?.private ? "checked" : ""}>${icon("lock")}${t("group_private")}</label>
      <small class="hint">${t("group_private_hint")}</small></div>` : ""}
    ${g ? `<div class="field"><span>${t("group_image")} · ${t("optional")}</span>
      <div class="gimg-edit" data-gimg></div>
      <small class="hint">${t("group_image_hint")}</small></div>` : ""}
  </form>`);
  const s = sheet({ title: g ? t("edit_group") : t("new_group"), body: form,
    foot: `${g && isAdmin() ? `<button class="btn danger left" data-del>${icon("trash")}${t("delete")}</button>` : ""}<button class="btn" data-c>${t("cancel")}</button><button class="btn primary" data-ok>${icon("check")}${g ? t("save") : t("create")}</button>` });
  $("[data-c]", s.el).onclick = s.close;
  const submit = async () => {
    if (!form.reportValidity()) return;
    const f = Object.fromEntries(new FormData(form));
    if (isAdmin()) f.private = $("[name=private]", form).checked;
    try { g ? await api("PATCH", `/api/groups/${g.id}`, f) : await api("POST", "/api/groups", f); toast(t("saved")); s.close(); rerender(); } catch (e) { fail(e); }
  };
  $("[data-ok]", s.el).onclick = submit;
  form.onsubmit = (e) => { e.preventDefault(); submit(); };
  // cover image: uploaded at once (no need to press Save)
  const imgBox = $("[data-gimg]", form);
  const drawImg = (busy = false) => {
    if (!imgBox) return;
    imgBox.innerHTML = `<div class="gimg-prev">${busy ? `<div class="typing"><i></i><i></i><i></i></div>` : g.image_v ? `<img alt="" src="${groupImg(g)}">` : icon("image")}</div>
      <div class="gimg-btns"><label class="btn">${icon("upload")}${t(g.image_v ? "change_image" : "choose_image")}<input type="file" accept="image/*" hidden data-gfile></label>
      ${g.image_v ? `<button type="button" class="btn ghost danger" data-gnoimg>${icon("x")}${t("remove_image")}</button>` : ""}</div>`;
    $("[data-gfile]", imgBox).onchange = async (e) => {
      const f = e.target.files[0]; if (!f) return;
      drawImg(true);
      try {
        const r = await api("PUT", `/api/groups/${g.id}/image`, await shrinkImage(f));
        g = { ...g, image_v: r.image_v }; toast(t("saved")); rerender();
      } catch (err) { fail(err); }
      drawImg();
    };
    $("[data-gnoimg]", imgBox)?.addEventListener("click", async () => {
      if (!(await confirmSheet(t("remove_image") + "?"))) return;
      try { await api("DELETE", `/api/groups/${g.id}/image`); g = { ...g, image_v: 0 }; toast(t("saved")); rerender(); drawImg(); } catch (err) { fail(err); }
    });
  };
  drawImg();
  $("[data-del]", s.el)?.addEventListener("click", async () => {
    if (!(await confirmSheet(g.name))) return;
    try { await api("DELETE", `/api/groups/${g.id}`); toast(t("deleted")); s.close(); rerender(); } catch (e) { fail(e); }
  });
}

async function membersSheet(g, rerender) {
  const body = h(`<div></div>`);
  const s = sheet({ title: g.name, body, onClose: rerender });
  const render = async () => {
    await loadGroups();
    g = groupById(g.id) || g;
    const manage = isAdmin() || g.is_leader;
    const [members, users] = await Promise.all([api("GET", `/api/groups/${g.id}/members`), manage ? api("GET", "/api/users") : Promise.resolve([])]);
    const ids = new Set(members.map((m) => m.id));
    const others = users.filter((u) => !ids.has(u.id) && u.status !== "pending");
    body.innerHTML = `
      <div class="leader-box">${icon("crown")}<div class="grow">${g.leader_id ? `<b>${t("leader")}: ${esc(g.leader_id === S.me.id ? t("you_obj") : g.leader_name)}</b>` : `<b>${t("no_leader")}</b><small class="muted">${t("vote_hint")}</small>`}</div>
        ${g.is_leader ? `<button class="btn sm" data-resign>${t("step_down")}</button>` : isAdmin() && g.leader_id ? `<button class="btn sm danger" data-unlead>${t("remove_leader")}</button>` : ""}</div>
      ${manage && others.length ? `<div class="field"><span>${t("add_member")}</span><div style="display:flex;gap:8px"><select class="input" data-u>${others.map((u) => `<option value="${u.id}">${esc(u.name || u.username)} (@${esc(u.username)})</option>`).join("")}</select><button class="btn primary" data-add>${icon("plus")}${t("add")}</button></div></div>` : ""}
      <div class="list">${members.map((u) => `<div class="li">${avatar(u.id, u)}<div class="grow"><div class="t">${esc(u.name || u.username)}${u.id === g.leader_id ? ` <span class="badge acc lead-badge">${icon("crown")}${t("leader")}</span>` : ""}</div><div class="s mono">@${esc(u.username)}</div></div>
        <div class="acts">${isAdmin() && u.id !== g.leader_id ? `<button class="btn sm ghost act" data-lead="${u.id}">${icon("crown")}<span class="lbl">${t("make_leader")}</span></button>` : ""}${manage && u.id !== S.me.id ? `<button class="btn sm ghost act danger" data-rm="${u.id}">${icon("x")}<span class="lbl">${t("remove")}</span></button>` : ""}</div></div>`).join("") || `<div class="empty">—</div>`}</div>`;
    $("[data-add]", body)?.addEventListener("click", async () => { try { await api("POST", `/api/groups/${g.id}/members`, { user_id: +$("[data-u]", body).value }); render(); } catch (e) { fail(e); } });
    $$("[data-rm]", body).forEach((b) => (b.onclick = async () => {
      const u = members.find((x) => x.id == b.dataset.rm);
      if (!(await confirmSheet(`${u.name || u.username} — ${g.name}`))) return;
      try { await api("DELETE", `/api/groups/${g.id}/members/${b.dataset.rm}`); render(); } catch (e) { fail(e); }
    }));
    $$("[data-lead]", body).forEach((b) => (b.onclick = async () => { try { await api("PUT", `/api/groups/${g.id}/leader`, { user_id: +b.dataset.lead }); toast(t("saved")); render(); } catch (e) { fail(e); } }));
    $("[data-unlead]", body)?.addEventListener("click", async () => { try { await api("PUT", `/api/groups/${g.id}/leader`, { user_id: 0 }); render(); } catch (e) { fail(e); } });
    $("[data-resign]", body)?.addEventListener("click", async () => { try { await api("DELETE", `/api/groups/${g.id}/leader`); toast(t("saved")); render(); } catch (e) { fail(e); } });
  };
  render().catch(fail);
  return s;
}

// election of the capogruppo (from the group chat)
async function voteSheet(g) {
  if (!g) return;
  const body = h(`<div><div class="typing"><i></i><i></i><i></i></div></div>`);
  const s = sheet({ title: `${t("vote_leader")} · ${g.name}`, body });
  const render = async () => {
    const st = await api("GET", `/api/groups/${g.id}/votes`);
    if (st.leader_id) {
      body.innerHTML = `<div class="empty">${icon("crown")}<div><b>${esc(st.leader_name)}</b> ${t("is_leader_now")}</div></div>`;
      return;
    }
    body.innerHTML = `<p class="muted" style="margin-top:0">${t("vote_explain").replace("{n}", st.needed).replace("{m}", st.members)}</p>
      <div class="list">${st.candidates.map((c) => `<button class="li vote-row ${st.my_vote === c.id ? "on" : ""}" data-c="${c.id}">
        ${avatar(c.id, c)}<div class="grow"><div class="t">${esc(c.name)}${c.id === S.me.id ? ` <span class="muted">(${t("you")})</span>` : ""}</div>
        <div class="vote-bar"><i style="width:${Math.min(100, (c.votes / st.needed) * 100)}%"></i></div></div>
        <span class="mono vote-n">${c.votes}/${st.needed}</span>${st.my_vote === c.id ? `<span class="badge acc">${icon("check")}${t("your_vote")}</span>` : ""}</button>`).join("")}</div>`;
    $$("[data-c]", body).forEach((b) => (b.onclick = async () => {
      try {
        const r = await api("POST", `/api/groups/${g.id}/vote`, { candidate_id: +b.dataset.c });
        await loadGroups(); chat.leaderUI?.();
        if (r.leader_id) { toast(`${r.leader_name} ${t("is_leader_now")}`, false, null, "crown"); s.close(); } else render();
      } catch (e) { fail(e); }
    }));
  };
  render().catch(fail);
  window._voteRefresh = () => { if (document.body.contains(s.el)) render().catch(() => {}); };
}

/* ═════════ documents ═════════ */
const fileIcon = (d) => (d.mime.startsWith("image/") ? "image" : /markdown|text|pdf|json|csv/.test(d.mime) || /\.(md|txt|csv|pdf)$/i.test(d.name) ? "filetext" : "file");

const docTab = { v: store.get("docTab", "mine") };
const shareLabel = (shares) => {
  if (!shares?.length) return `<span class="badge">${icon("lock")}${t("private")}</span>`;
  return shares.map((g) => g === 0 ? `<span class="badge acc">${t("everyone")}</span>` : (() => { const gr = groupById(g); return gr ? `<span class="badge gb" style="--c:${esc(gr.color)}"><i></i>${esc(gr.name)}</span>` : ""; })()).join("");
};

async function viewDocs(v, args) {
  await loadGroups().catch(() => {});
  liveView = { kinds: ["docs", "groups"], fn: () => { if (!$(".sheet") && !$(".hl-ed-shell")) render(); } };
  const render = async () => {
    const r = await api("GET", "/api/docs");
    const pct = Math.min(100, (r.used / r.quota) * 100);
    const tab = docTab.v;
    const list = tab === "mine" ? r.docs : r.shared;
    v.innerHTML = pageHead("", "", "",
      `${S.feat.doc_create ? `<button class="btn" data-newdoc>${icon("edit")}<span>${t("new_doc")}</span></button>` : ""}${S.feat.doc_upload ? `<button class="btn primary" data-up>${icon("upload")}<span>${t("upload")}</span></button>` : ""}`) +
      `<input type="file" multiple hidden data-file>
      <div class="cal-bar"><div class="seg"><button data-tab="mine" class="${tab === "mine" ? "on" : ""}">${icon("lock")}${t("mine")} <span class="mono">${r.docs.length}</span></button><button data-tab="shared" class="${tab === "shared" ? "on" : ""}">${icon("share")}${t("shared_with_me")} <span class="mono">${r.shared.length}</span></button></div></div>
      ${tab === "mine" ? `${S.feat.doc_upload ? `<div class="drop" data-drop>${icon("upload")}<span>${t("drop_here")} · max ${fmtSize(r.max_file)}</span></div>` : ""}
      <div class="quota mono muted"><span>${fmtSize(r.used)} ${t("used_of")} ${fmtSize(r.quota)}</span><div class="bar"><i style="width:${pct}%"></i></div></div>` : ""}
      <div class="list">${list.map((d, i) => docRow(d, i, tab === "mine")).join("") || `<div class="empty"><span class="serif">${tab === "mine" ? t("no_docs") : t("no_shared")}</span></div>`}</div>`;
    $$("[data-tab]", v).forEach((b) => (b.onclick = () => { docTab.v = b.dataset.tab; store.set("docTab", docTab.v); render(); }));
    const input = $("[data-file]", v);
    const upload = async (files) => {
      if (!files.length) return;
      const fd = new FormData();
      [...files].forEach((f) => fd.append("file", f, f.name));
      try { await api("POST", "/api/docs", fd); toast(t("saved")); docTab.v = "mine"; render(); } catch (e) { fail(e); }
    };
    $("[data-up]", v)?.addEventListener("click", () => input.click());
    input.onchange = () => upload(input.files);
    const drop = $("[data-drop]", v);
    if (drop) {
      drop.onclick = () => input.click();
      drop.ondragover = (e) => { e.preventDefault(); drop.classList.add("over"); };
      drop.ondragleave = () => drop.classList.remove("over");
      drop.ondrop = (e) => { e.preventDefault(); drop.classList.remove("over"); upload(e.dataTransfer.files); };
    }
    const all = [...r.docs, ...r.shared];
    $$("[data-prev]", v).forEach((b) => (b.onclick = () => previewDoc(all.find((d) => d.id == b.dataset.prev), render)));
    $$("[data-share]", v).forEach((b) => (b.onclick = () => shareSheet(r.docs.find((d) => d.id == b.dataset.share), render)));
    $$("[data-editdoc]", v).forEach((b) => (b.onclick = () => editDoc(all.find((d) => d.id == b.dataset.editdoc), render)));
    $("[data-newdoc]", v).onclick = () => {
      // same editor as "Modifica": the file is created on the first save
      docTab.v = "mine"; store.set("docTab", "mine");
      editDoc({ id: null, name: "", isNew: true }, render);
    };
    $$("[data-del]", v).forEach((b) => (b.onclick = async () => {
      const d = r.docs.find((x) => x.id == b.dataset.del);
      if (!(await confirmSheet(d.name))) return;
      try { await api("DELETE", `/api/docs/${d.id}`); toast(t("deleted")); render(); } catch (e) { fail(e); }
    }));
    if (args[0]) { const d = all.find((x) => x.id == args[0]); args = []; if (d) previewDoc(d); }
  };
  await render();
}

function docRow(d, i, own) {
  return `<div class="li rv" style="--i:${Math.min(i, 12)}">
    <div class="ficon">${icon(fileIcon(d))}<span class="ext">${esc((d.name.split(".").pop() || "").slice(0, 4))}</span></div>
    <div class="grow" style="cursor:pointer" data-prev="${d.id}"><div class="t">${esc(d.name)}</div>
      <div class="s mono">${fmtSize(d.size)} · ${fmt(new Date(d.created_at * 1000), { day: "numeric", month: "short" })}</div>
      <div class="dtags"><span class="dtag">${icon(d.via_ai ? "ai" : "upload")}${esc(t(d.via_ai ? "doc_by_ai" : "doc_by"))} ${esc(own || d.owner_id === S.me.id ? t("you_obj") : d.owner || "—")}</span><span class="shares">${own || isAdmin() || d.owner_id === S.me.id ? shareLabel(d.shares) : shareLabel(d.shares.filter((g) => g === 0 || S.myGroups.includes(g)))}</span></div></div>
    <div class="acts">${d.can_edit ? `<button class="btn sm ghost act" data-editdoc="${d.id}" title="${t("edit_doc")}">${icon("edit")}<span class="lbl">${t("edit_doc")}</span></button>` : ""}${own ? `<button class="btn sm ghost act" data-share="${d.id}" title="${t("share")}">${icon("share")}<span class="lbl">${t("share")}</span></button>` : ""}<a class="btn sm ghost act" href="/api/docs/${d.id}" download title="${t("download")}">${icon("download")}<span class="lbl">${t("download")}</span></a>${own ? `<button class="btn sm ghost act danger" data-del="${d.id}" title="${t("delete")}">${icon("trash")}<span class="lbl">${t("delete")}</span></button>` : ""}</div>
  </div>`;
}

function shareSheet(d, done) {
  const targets = [{ id: 0, name: t("everyone"), color: "var(--ink)" }, ...(isAdmin() ? S.groups : S.groups.filter((g) => g.is_member))];
  const form = h(`<form><p class="muted" style="margin-top:0">${t("share_hint")}</p><div class="list">${targets.map((g) => `
    <label class="li" style="cursor:pointer"><span class="gtag" style="--c:${esc(g.color)}"><i></i></span><div class="grow"><div class="t">${esc(g.name)}</div>${g.id ? `<div class="s mono">${g.members} ${t("members")}</div>` : ""}</div>
    <input type="checkbox" value="${g.id}" ${d.shares.includes(g.id) ? "checked" : ""} style="accent-color:var(--accent);width:18px;height:18px"></label>`).join("")}</div></form>`);
  const s = sheet({ title: d.name, body: form, foot: `<button class="btn" data-c>${t("cancel")}</button><button class="btn primary" data-ok>${icon("share")}${t("save")}</button>` });
  $("[data-c]", s.el).onclick = s.close;
  $("[data-ok]", s.el).onclick = async () => {
    try { await api("PUT", `/api/docs/${d.id}/shares`, { groups: $$("input:checked", form).map((c) => +c.value) }); toast(t("saved")); s.close(); done(); } catch (e) { fail(e); }
  };
}

const isMd = (d) => /\.(md|markdown)$/i.test(d.name) || (d.mime || "").includes("markdown");
function editDoc(d, after) { openEditor(d, { api, toast, fail, onSaved: () => after?.() }); }

async function previewDoc(d, after) {
  if (!d) return;
  const body = h(`<div><div class="typing"><i></i><i></i><i></i></div></div>`);
  const sh = sheet({ title: d.name, body, wide: true, foot: `<a class="btn" href="/api/docs/${d.id}" download>${icon("download")}${t("download")}</a>${isMd(d) ? `<button class="btn ghost hidden" data-hist>${icon("history")}${t("old_versions")}</button><button class="btn primary hidden" data-editdoc>${icon("edit")}${t("edit_doc")}</button>` : ""}` });
  $("[data-editdoc]", sh.el)?.addEventListener("click", () => { sh.close(); editDoc(d, after); });
  const r = await renderPreview(body, d, { api, md });
  if (!r.canEdit) return;
  $("[data-editdoc]", sh.el)?.classList.remove("hidden");
  // History: only for people who can edit, and only once there is an older version.
  try {
    const h = await api("GET", `/api/docs/${d.id}/versions`);
    if (!h.versions.length) return;
    const b = $("[data-hist]", sh.el);
    b.classList.remove("hidden");
    b.onclick = () => versionsSheet(d, h.versions, () => { sh.close(); previewDoc(d, after); after?.(); });
  } catch {}
}

const whenSaved = (ts) => { const d = new Date(ts * 1000); return `${dayLabel(d)}, ${fmtTime(d)}`; };

function versionsSheet(d, versions, restored) {
  const s = sheet({
    title: t("old_versions"),
    body: `<p class="muted" style="margin-top:0">${t("versions_hint")}</p>
      <div class="vlist">${versions.map((v) => `<button class="vrow" data-v="${v.version}">${icon("history")}<span><b>${esc(whenSaved(v.saved_at))}</b><small>${esc(v.author || "—")}${v.via_ai ? ` · ${t("by_assistant")}` : ""} · ${t("version")} ${v.version}</small></span>${icon("right")}</button>`).join("")}</div>`,
  });
  $$("[data-v]", s.el).forEach((b) => (b.onclick = async () => {
    let v;
    try { v = await api("GET", `/api/docs/${d.id}/versions/${b.dataset.v}`); } catch (e) { return fail(e); }
    const vs = sheet({
      title: d.name, wide: true,
      body: `<div class="oldver">${icon("history")}<span><b>${t("old_version")}</b> · ${esc(whenSaved(v.saved_at))}<br>${t("not_current")}</span></div><div class="md">${md(v.text)}</div>`,
      foot: `<button class="btn" data-c>${t("close")}</button><button class="btn primary" data-restore>${icon("history")}${t("restore")}</button>`,
    });
    $("[data-c]", vs.el).onclick = vs.close;
    $("[data-restore]", vs.el).onclick = async () => {
      if (!(await confirmSheet(t("restore_q")))) return;
      try {
        await api("POST", `/api/docs/${d.id}/versions/${v.version}/restore`);
        toast(t("restored"));
        vs.close(); s.close(); restored();
      } catch (e) { fail(e); }
    };
  }));
}

/* ═════════ live updates & organisational chat ═════════ */
const phone = () => matchMedia("(max-width: 760px)").matches;
const chat = { room: store.get("room", 0), rooms: [], live: null, es: null, unread: 0 };
const roomName = (r) => (r.id === 0 ? t("general_chat") : r.name);

function startLive() {
  chat.es?.close();
  const es = new EventSource("/api/stream");
  chat.es = es;
  es.onopen = () => { chat.live?.resync(); checkVersion(); };
  es.onmessage = (e) => {
    let ev; try { ev = JSON.parse(e.data); } catch { return; }
    if (ev.type === "message") {
      const m = ev.msg, mine = m.user_id === S.me.id;
      if (chat.live && chat.live.room === m.room_id && document.visibilityState === "visible") { chat.live.add(m); return; }
      if (mine) return;
      if (m.meta?.kind === "volunteers" && !(m.meta.skip || []).includes(S.me.id)) {
        toast(t("vol_alert").replace("{t}", m.meta.title), false, () => openEventAt(m.meta.event_id, new Date(m.meta.start)), "megaphone");
      } else if (m.mentions.includes(S.me.id)) {
        const dm = m.room_id < 0;
        const where = m.room_id === 0 ? t("general_chat") : ev.room_name;
        const go = () => { location.hash = `#/chat/${m.room_id}`; };
        toast(dm ? `${m.author} ${t("dm_wrote")}` : `@${m.username} ${t("mentioned_you")} ${where}`, false, go, dm ? "chat" : "at");
        if (!pushOn && "Notification" in window && Notification.permission === "granted" && document.visibilityState !== "visible") {
          const n = new Notification(dm ? m.author : `${m.author} → ${where}`, { body: m.content.slice(0, 140), tag: "chat" + m.room_id });
          n.onclick = () => { window.focus(); go(); n.close(); };
        }
      }
      if (chat.live && chat.live.room === m.room_id) chat.live.add(m);
      refreshBadges();
    } else if (ev.type === "delete") chat.live?.remove(ev.id);
    else if (ev.type === "event") {
      liveRefresh("event");
      if (ev.by_id !== S.me.id && (ev.action === "create" || ev.action === "update")) {
        const d = new Date(ev.start);
        const msg = `${ev.by} ${t(ev.action === "create" ? "live_event_new" : "live_event_upd")} «${ev.title}» · ${fmt(d, { day: "numeric", month: "short" })}`;
        toast(msg, false, () => { cal.openId = ev.id; cal.cur = startOfDay(d); cal.sel = startOfDay(d); if (location.hash.startsWith("#/calendar")) route(); else location.hash = `#/calendar/${ev.start.slice(0, 10)}`; }, "calendar");
      }
    } else if (ev.type === "groups") { loadGroups().then(() => { liveRefresh("groups"); refreshBadges(); chat.leaderUI?.(); window._voteRefresh?.(); }).catch(() => {}); }
    else if (ev.type === "docs" || ev.type === "users") liveRefresh(ev.type);
    else if (ev.type === "avatars") loadAvatars();
    else if (ev.type === "settings") loadMe().then(() => { renderShell(); route(); }).catch(() => {});
    else if (ev.type === "pending") {
      const more = (ev.count || 0) > (S.pending || 0);
      S.pending = ev.count || 0; pendingBadge(); liveRefresh("pending");
      if (more) toast(t("live_pending"), false, () => { location.hash = "#/admin"; }, "user");
    }
    else if (ev.type === "ai_typing" && chat.live?.room === ev.room_id) chat.live.typing(ev.on);
  };
}

let badgeTimer;
function refreshBadges() {
  clearTimeout(badgeTimer);
  badgeTimer = setTimeout(async () => {
    try { chat.rooms = await api("GET", "/api/rooms"); } catch { return; }
    const unread = chat.rooms.reduce((n, r) => n + r.unread, 0), ment = chat.rooms.reduce((n, r) => n + r.mentions, 0);
    const b = $('[data-nb="chat"]');
    if (b) { b.textContent = ment ? "@" + ment : unread > 99 ? "99+" : unread || ""; b.classList.toggle("on", unread > 0 || ment > 0); b.classList.toggle("at", ment > 0); }
    chat.live?.rooms();
  }, 250);
}

const linkify = (s) => s.replace(/(https?:\/\/[^\s<]+[^\s<.,;:!?)\]'"])/g, '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>');
function fmtMsg(text, members) {
  const known = new Set(members.map((m) => m.username.toLowerCase()).concat(["tutti", "all", "ai"]));
  const ids = new Map(members.map((m) => [m.username.toLowerCase(), m.id]));
  return linkify(esc(text)).replace(/(^|[^\w@.\/])@([a-zA-Z0-9._-]{2,32})/g, (all, pre, name) => {
    const n = name.replace(/\.+$/, "");
    if (!known.has(n.toLowerCase())) return all;
    const me = n.toLowerCase() === S.me.username.toLowerCase() || ["tutti", "all"].includes(n.toLowerCase());
    const uid = ids.get(n.toLowerCase());
    return `${pre}<span class="mention ${me ? "me" : ""}">${uid && avatarsById[uid] ? `<span class="avatar xs" data-uid="${uid}"></span>` : ""}@${esc(n)}</span>${esc(name.slice(n.length))}`;
  });
}
const dayLabel = (d) => sameDay(d, new Date()) ? t("today") : sameDay(d, addDays(new Date(), -1)) ? t("yesterday") : fmt(d, { weekday: "long", day: "numeric", month: "long" });

const quoteHTML = (r) => r ? `<button type="button" class="rquote ${r.is_ai ? "q-bot" : ""}" data-jump="${r.id}"><b>${esc(r.author || "—")}</b><span>${r.deleted ? esc(t("msg_deleted")) : esc(r.text)}</span></button>` : "";

// volSummary: "sab 20 dic · 10:00–13:00 (1/2) · mancano 3"
function volSummary(x) {
  let lastDay = "";
  const parts = (x.shifts || []).map((sh) => {
    const s0 = new Date(sh.start), e0 = new Date(sh.end), d = fmt(s0, { weekday: "short", day: "numeric", month: "short" });
    const p = `${d !== lastDay ? d + " · " : ""}${fmtTime(s0)}–${fmtTime(e0)} (${sh.have}/${sh.needed})${sh.label ? " " + sh.label : ""}`;
    lastDay = d;
    return p;
  });
  return `${parts.join(", ")} — ${missText(x.missing)}`;
}
function openEventAt(id, day) {
  cal.openId = id; cal.cur = startOfDay(day); cal.sel = startOfDay(day);
  const h0 = `#/calendar/${ymd(day)}`;
  if (location.hash.startsWith("#/calendar")) { history.replaceState(null, "", h0); route(); } else location.hash = h0;
}
// dmPicker: choose someone to write to
async function dmPicker() {
  const people = (await api("GET", "/api/rooms/0/members").catch(() => [])).filter((u) => u.id !== S.me.id);
  const body = h(`<div><input class="input" data-q placeholder="${t("dm_search")}" autocomplete="off" style="margin-bottom:10px"><div class="list" data-list></div></div>`);
  const s = sheet({ title: t("dm_pick"), body });
  const draw = (q = "") => {
    const list = people.filter((u) => !q || (u.name || "").toLowerCase().includes(q) || u.username.toLowerCase().includes(q));
    $("[data-list]", body).innerHTML = list.map((u) => `<button class="li dmpick" data-u="${u.id}">${avatar(u.id, u)}<div class="grow"><div class="t">${esc(u.name || u.username)}</div><div class="s mono">@${esc(u.username)}</div></div>${icon("chat")}</button>`).join("") || `<div class="empty">—</div>`;
    fillAvatars(body);
    $$("[data-u]", body).forEach((b) => (b.onclick = () => { s.close(); openDM(+b.dataset.u); }));
  };
  $("[data-q]", body).oninput = (e) => draw(e.target.value.trim().toLowerCase());
  draw();
  if (!phone()) $("[data-q]", body).focus();
}

async function viewChat(v, args) {
  if (args[0] !== undefined && args[0] !== "") chat.room = +args[0];
  v.classList.add("ai-view");
  v.classList.add("chat-view");
  const bell = () => ("Notification" in window ? `<button type="button" class="ibtn sm ghost bell ${Notification.permission === "granted" ? "on" : ""}" data-notify title="${t("enable_notify")}" aria-label="${t("enable_notify")}">${icon("bell")}</button>` : "");
  v.innerHTML = `<div class="ai chatv">
      <aside class="side"><div class="side-t"><span>${t("your_chats")}</span>${bell()}</div><div class="convs" data-rooms></div></aside>
      <section class="main">
        <div class="room-h"><a class="ibtn sm ghost back-m" href="#/chat" aria-label="${t("your_chats")}">${icon("left")}</a><span class="gtag" data-rdot><i></i></span><b data-rname></b><span class="ai-count hidden" data-rquota></span><span class="mono muted" data-rcount></span><span class="room-leader" data-rleader></span><span style="flex:1"></span><button class="btn sm vote-btn hidden" data-vote>${icon("crown")}<span>${t("vote")}</span></button>
          <button class="btn sm ghost act" data-rdocs title="${t("room_docs")}">${icon("docs")}<span class="lbl">${t("room_docs")}</span></button>${bell()}</div>
        <div class="msgs cmsgs" data-msgs></div>
        <div class="composer"><div class="mention-pop hidden" data-pop></div><div class="ai-typing hidden" data-aityping><span class="aiav">${icon("ai")}</span>${t("ai_typing")}<span class="typing"><i></i><i></i><i></i></span></div>
          <div class="replybar hidden" data-replybar>${icon("reply")}<div class="rq"><b data-rqa></b><span data-rqt></span></div><button type="button" class="ibtn sm ghost" data-rqx aria-label="${t("cancel")}">${icon("x")}</button></div>
          <form data-form><button type="button" class="ibtn ghost att" data-attach title="${t("attach")}">${icon("paperclip")}</button><textarea rows="1" placeholder="${t("write_message")}" data-input></textarea><button class="ibtn" aria-label="${t("send")}">${icon("send")}</button></form></div>
      </section></div>`;
  const wrap = $(".ai", v), roomsEl = $("[data-rooms]", v), msgsEl = $("[data-msgs]", v), input = $("[data-input]", v), pop = $("[data-pop]", v);
  const unfit = fitPanel(wrap);
  $$("[data-notify]", v).forEach((b) => (b.onclick = async () => {
    if (Notification.permission === "denied") return toast(t("notify_denied"), true);
    if (await pushEnable()) { toast(t("notify_on"), false, null, "bell"); $$("[data-notify]", v).forEach((x) => x.classList.add("on")); }
  }));

  let members = [], lastId = 0, firstId = 0, loadingOld = false, readTimer;
  const markRead = () => { clearTimeout(readTimer); readTimer = setTimeout(() => api("POST", `/api/rooms/${chat.room}/read`).then(refreshBadges).catch(() => {}), 400); };

  const renderRooms = () => {
    const firstDM = chat.rooms.find((r) => r.dm)?.id;
    roomsEl.innerHTML = chat.rooms.map((r) => r.dm ? `${r.id === firstDM ? `<div class="side-sub">${t("dm_title")}</div>` : ""}<div class="conv room dm ${r.id === chat.room && !wrap.classList.contains("show-side") ? "on" : ""}" data-r="${r.id}">
      <span class="rdot person">${avatar(r.dm.id, r.dm)}</span>
      <span class="t"><b>${esc(r.name)}</b><small>${r.last_text ? `${r.last_by === (S.me.name || S.me.username) ? t("you_cap") + ": " : ""}${esc(r.last_text)}` : "—"}</small></span>
      ${r.mentions || (r.unread && r.id !== chat.room) ? `<span class="nb on">${r.unread || r.mentions}</span>` : ""}</div>` : `<div class="conv room ${r.id === chat.room && !wrap.classList.contains("show-side") ? "on" : ""}" data-r="${r.id}">
      <span class="rdot ${(r.id === 0 ? generalImg() : groupImg(groupById(r.id))) ? "pic" : ""}" style="--c:${esc(r.id === 0 ? generalColor() : r.color)}">${r.id === 0 ? (generalImg() ? `<img alt="" loading="lazy" src="${generalImg()}">` : icon("globe")) : groupById(r.id)?.image_v ? `<img alt="" loading="lazy" src="${groupImg(groupById(r.id))}">` : esc(r.name[0] || "#")}</span>
      <span class="t"><b>${r.id || S.general?.color ? `<i class="rsw" style="--c:${esc(r.id ? r.color : S.general.color)}"></i>` : ""}${esc(roomName(r))}</b><small>${r.last_text ? `${r.last_by ? esc(r.last_by.split(" ")[0]) + ": " : ""}${esc(r.last_text)}` : "—"}</small></span>
      ${r.mentions ? `<span class="nb on at">@${r.mentions}</span>` : r.unread && r.id !== chat.room ? `<span class="nb on">${r.unread}</span>` : ""}</div>`).join("")
      + `<button type="button" class="conv room newdm" data-newdm><span class="rdot">${icon("edit")}</span><span class="t"><b>${t("dm_new")}</b></span></button>`;
    fillAvatars(roomsEl);
    $("[data-newdm]", roomsEl).onclick = dmPicker;
    $$("[data-r]", roomsEl).forEach((el) => (el.onclick = () => {
      if (phone()) location.hash = `#/chat/${el.dataset.r}`;
      else openRoom(+el.dataset.r);
    }));
  };

  const msgEl = (m, prev) => {
    if (m.meta?.kind === "volunteers") {
      const x = m.meta, d0 = new Date(x.start);
      const sys = h(`<div class="cm-sys vol" data-id="${m.id}"><button type="button">${icon("megaphone")}<span><b>${esc(t("vol_alert").replace("{t}", x.title))}</b><small>${esc(volSummary(x))}</small></span>${icon("right")}</button></div>`);
      $("button", sys).onclick = () => openEventAt(x.event_id, d0);
      sys._m = m;
      return sys;
    }
    if (m.meta?.system) {
      let txt = m.content;
      if (m.meta.key && t(m.meta.key) !== m.meta.key) txt = t(m.meta.key).replace(/\{(\w+)\}/g, (_, k) => m.meta.args?.[k] ?? "");
      const sys = h(`<div class="cm-sys" data-id="${m.id}">${icon("crown")}<span>${esc(txt)}</span></div>`);
      sys._m = m;
      return sys;
    }
    const d = new Date(m.created_at * 1000), mine = m.user_id === S.me.id;
    const grouped = prev && !prev.is_ai && !m.is_ai && prev.user_id === m.user_id && m.created_at - prev.created_at < 300 && sameDay(new Date(prev.created_at * 1000), d);
    const frag = document.createDocumentFragment();
    if (!prev || !sameDay(new Date(prev.created_at * 1000), d)) frag.append(h(`<div class="dsep mono">${esc(dayLabel(d))}</div>`));
    if (m.is_ai) {
      const meta = m.meta || {};
      const el = h(`<div class="cm bot" data-id="${m.id}"><div class="avatar sm aiav" data-uid="ai">${icon("ai")}</div>
        <div class="cb"><div class="who"><b>Hiveloop AI</b><span class="mono">@ai</span></div>
        <div class="bubble">${quoteHTML(m.reply)}${(meta.tools || []).length ? `<div class="tools">${meta.tools.map((x) => `<span class="tool ${x.ok ? "ok" : "bad"}">${icon(x.ok ? "check" : "x")}<span>${esc(t("tool_" + x.name))}${x.summary && !/^\d+$/.test(x.summary) ? ": " + esc(x.summary) : ""}</span></span>`).join("")}</div>` : ""}
        <div class="md">${md(/^⚠️ [a-z_]+$/.test(m.content) ? "⚠️ " + t(m.content.slice(3)) : m.content)}</div><span class="tm mono">${fmtTime(d)}</span></div></div>
        <button class="ibtn sm ghost rep" data-rep title="${t("reply")}" aria-label="${t("reply")}">${icon("reply")}</button>
        ${isAdmin() ? `<button class="ibtn sm ghost danger del" data-del="${m.id}">${icon("trash")}</button>` : ""}<span class="swipe-ic">${icon("reply")}</span></div>`);
      $(".bubble", el).addEventListener("click", (e) => { if (!e.target.closest("a,button")) el.classList.toggle("act"); });
      $("[data-del]", el)?.addEventListener("click", async () => { if (!(await confirmSheet(m.content.slice(0, 80)))) return; try { await api("DELETE", `/api/room-messages/${m.id}`); } catch (e) { fail(e); } });
      el._m = m;
      frag.append(el);
      return frag;
    }
    const el = h(`<div class="cm ${mine ? "mine" : ""} ${grouped ? "grp" : ""} ${m.mentions.includes(S.me.id) ? "hit" : ""}" data-id="${m.id}">
      ${grouped ? `<div class="avatar sm"></div>` : `${avatar(m.user_id, { name: m.author })}`}
      <div class="cb">${grouped || mine ? "" : `<div class="who"><b>${esc(m.author)}</b><span class="mono">@${esc(m.username)}</span></div>`}
        <div class="bubble">${quoteHTML(m.reply)}${m.content ? `<div class="txt">${fmtMsg(m.content, members)}</div>` : ""}
          ${m.doc ? `<button class="mdoc" data-doc="${m.doc.id}">${icon(fileIcon({ mime: m.doc.mime, name: m.doc.name }))}<span>${esc(m.doc.name)}</span></button>` : ""}
          <span class="tm mono">${fmtTime(d)}</span></div></div>
      <button class="ibtn sm ghost rep" data-rep title="${t("reply")}" aria-label="${t("reply")}">${icon("reply")}</button>
      ${mine || isAdmin() ? `<button class="ibtn sm ghost danger del" data-del="${m.id}" title="${t("delete_msg")}">${icon("trash")}</button>` : ""}<span class="swipe-ic">${icon("reply")}</span></div>`);
    $("[data-doc]", el)?.addEventListener("click", () => previewDoc({ id: m.doc.id, name: m.doc.name, mime: m.doc.mime }));
    $("[data-del]", el)?.addEventListener("click", async () => { if (!(await confirmSheet(m.content.slice(0, 80) || m.doc?.name || ""))) return; try { await api("DELETE", `/api/room-messages/${m.id}`); } catch (e) { fail(e); } });
    $(".bubble", el).addEventListener("click", (e) => { if (!e.target.closest("a,button")) el.classList.toggle("act"); });
    el._m = m;
    frag.append(el);
    return frag;
  };
  const lastMsg = () => $$(".cm", msgsEl).pop()?._m;
  const nearBottom = () => msgsEl.scrollHeight - msgsEl.scrollTop - msgsEl.clientHeight < 160;
  const add = (m) => {
    if (m.room_id !== chat.room || $(`.cm[data-id="${m.id}"]`, msgsEl)) return;
    $(".empty", msgsEl)?.remove();
    const stick = nearBottom() || m.user_id === S.me.id;
    msgsEl.append(msgEl(m, lastMsg()));
    lastId = Math.max(lastId, m.id);
    if (m.is_ai) aiQuotaFor(chat.room).then((q) => aiCount($("[data-rquota]", v), q)); // the assistant answered: one less
    if (stick) msgsEl.scrollTop = msgsEl.scrollHeight;
    markRead();
  };
  const remove = (id) => {
    $$(`.rquote[data-jump="${id}"]`, msgsEl).forEach((q) => q.remove()); // deleted: no trace in replies either
    if (replyTo?.id === +id) clearReply();
    const el = $(`.cm[data-id="${id}"]`, msgsEl); if (el) { el.style.transition = "opacity .2s"; el.style.opacity = 0; setTimeout(() => el.remove(), 200); }
  };

  async function openRoom(id) {
    if (!chat.rooms.some((r) => r.id === id)) id = 0;
    chat.room = id; store.set("room", id);
    if (location.hash !== `#/chat/${id}`) history.replaceState(null, "", `#/chat/${id}`);
    chat.live = { room: id, add, remove, resync, rooms: renderRooms, typing: (on) => $("[data-aityping]", v).classList.toggle("hidden", !on) };
    $("[data-aityping]", v).classList.add("hidden");
    const r = chat.rooms.find((x) => x.id === id);
    const dm = !!r.dm;
    wrap.classList.toggle("is-dm", dm);
    $("[data-rname]", v).textContent = roomName(r);
    $("[data-rdot]", v).style.setProperty("--c", id === 0 ? generalColor() : r.color || "transparent");
    $("[data-rdot]", v).classList.toggle("hidden", dm);
    $("[data-rdocs]", v).classList.toggle("hidden", dm);
    $("[data-attach]", v).classList.toggle("hidden", dm);
    leaderUI();
    if (dm) aiCount($("[data-rquota]", v), null);
    else aiQuotaFor(id).then((q) => { if (chat.room === id) aiCount($("[data-rquota]", v), q); });
    renderRooms();
    msgsEl.innerHTML = `<div class="typing" style="margin:auto"><i></i><i></i><i></i></div>`;
    const [ms, list] = await Promise.all([api("GET", `/api/rooms/${id}/members`), api("GET", `/api/rooms/${id}/messages`)]);
    if (chat.room !== id) return;
    members = ms;
    $("[data-rcount]", v).textContent = dm ? `· ${t("dm_short")}` : `· ${members.length} ${members.length === 1 ? t("member") : t("members")}`;
    msgsEl.innerHTML = list.length ? "" : `<div class="empty"><span class="serif">${esc(roomName(r))}</span>${t("no_messages")}</div>`;
    let prev = null;
    list.forEach((m) => { msgsEl.append(msgEl(m, prev)); prev = m; });
    firstId = list[0]?.id || 0; lastId = list.length ? list[list.length - 1].id : 0;
    if (dm && !list.length) msgsEl.innerHTML = `<div class="empty"><span class="serif">${esc(roomName(r))}</span>${t("dm_hint")}</div>`;
    msgsEl.scrollTop = msgsEl.scrollHeight;
    markRead();
    if (chat.prefill) { input.value = chat.prefill; chat.prefill = ""; input.style.height = "auto"; input.style.height = Math.min(input.scrollHeight, 160) + "px"; input.focus(); input.setSelectionRange(input.value.length, input.value.length); }
    else if (!phone()) input.focus({ preventScroll: true });
  }
  async function resync() {
    if (!lastId) return openRoom(chat.room);
    const list = await api("GET", `/api/rooms/${chat.room}/messages?after=${lastId}`).catch(() => []);
    list.forEach(add);
  }
  msgsEl.onscroll = async () => {
    if (msgsEl.scrollTop > 60 || loadingOld || !firstId) return;
    loadingOld = true;
    const room = chat.room;
    const older = await api("GET", `/api/rooms/${room}/messages?before=${firstId}`).catch(() => []);
    if (room === chat.room && older.length) {
      const h0 = msgsEl.scrollHeight;
      const first = $(".cm", msgsEl), firstSep = msgsEl.firstElementChild?.classList.contains("dsep") ? msgsEl.firstElementChild : null;
      const frag = document.createDocumentFragment(); let prev = null;
      older.forEach((m) => { frag.append(msgEl(m, prev)); prev = m; });
      if (firstSep && first?._m && sameDay(new Date(prev.created_at * 1000), new Date(first._m.created_at * 1000))) firstSep.remove();
      msgsEl.prepend(frag);
      msgsEl.scrollTop += msgsEl.scrollHeight - h0;
      firstId = older[0].id;
    } else if (!older.length) firstId = 0;
    loadingOld = false;
  };

  const send = async (docId) => {
    const text = input.value.trim();
    if (!text && !docId) return;
    input.value = ""; input.style.height = ""; hidePop();
    const body = { content: text };
    if (docId) body.doc_id = docId;
    const rep = replyTo;
    if (rep) body.reply_to = rep.id;
    clearReply();
    try { add(await api("POST", `/api/rooms/${chat.room}/messages`, body)); }
    catch (e) { input.value = text; if (rep) startReply(rep); fail(e); }
  };

  // ---- replies: swipe a message to the left (phone) or press the reply arrow ----
  let replyTo = null;
  const bar = $("[data-replybar]", v);
  function startReply(m) {
    replyTo = m;
    $("[data-rqa]", bar).textContent = m.is_ai ? "Hiveloop AI" : m.author;
    $("[data-rqt]", bar).textContent = (m.content || m.doc?.name || "").replace(/[*_`#>]+/g, "").replace(/\s+/g, " ").trim().slice(0, 160);
    bar.classList.toggle("to-bot", !!m.is_ai);
    bar.classList.remove("hidden");
    input.placeholder = m.is_ai ? t("reply_ai_ph") : t("write_message");
    input.focus({ preventScroll: true });
  }
  function clearReply() { replyTo = null; bar.classList.add("hidden"); input.placeholder = t("write_message"); }
  $("[data-rqx]", bar).onclick = clearReply;
  msgsEl.addEventListener("click", (e) => {
    const rb = e.target.closest("[data-rep]");
    if (rb) { const el = rb.closest(".cm"); el.classList.remove("act"); return startReply(el._m); }
    const q = e.target.closest("[data-jump]");
    if (q) {
      const t0 = $(`.cm[data-id="${q.dataset.jump}"]`, msgsEl);
      if (t0) { t0.scrollIntoView({ block: "center", behavior: "smooth" }); t0.classList.remove("flash"); void t0.offsetWidth; t0.classList.add("flash"); }
    }
  });
  let sw = null;
  msgsEl.addEventListener("touchstart", (e) => {
    const el = e.target.closest(".cm");
    sw = el && el._m && e.touches.length === 1 ? { el, x: e.touches[0].clientX, y: e.touches[0].clientY, dx: 0, lock: null } : null;
  }, { passive: true });
  msgsEl.addEventListener("touchmove", (e) => {
    if (!sw) return;
    const dx = e.touches[0].clientX - sw.x, dy = e.touches[0].clientY - sw.y;
    if (!sw.lock && (Math.abs(dx) > 10 || Math.abs(dy) > 10)) {
      sw.lock = dx < 0 && Math.abs(dx) > Math.abs(dy) * 1.4 ? "x" : "y";
      if (sw.lock === "x") { sw.el.style.animation = "none"; sw.el.style.transition = "none"; sw.el.classList.add("swiping"); }
    }
    if (sw.lock !== "x") return;
    sw.dx = Math.max(-84, Math.min(0, dx));
    sw.el.style.transform = `translateX(${sw.dx}px)`;
    const ok = sw.dx < -56;
    if (ok && !sw.el.classList.contains("swipe-ok")) navigator.vibrate?.(8);
    sw.el.classList.toggle("swipe-ok", ok);
  }, { passive: true });
  const endSwipe = () => {
    if (!sw) return;
    const { el, dx, lock } = sw; sw = null;
    if (lock !== "x") return;
    el.style.transition = "transform .22s var(--ease)"; el.style.transform = "";
    el.classList.remove("swipe-ok", "swiping");
    setTimeout(() => { el.style.transition = ""; }, 240);
    if (dx < -56) startReply(el._m);
  };
  msgsEl.addEventListener("touchend", endSwipe);
  msgsEl.addEventListener("touchcancel", endSwipe);
  $("[data-form]", v).onsubmit = (e) => { e.preventDefault(); send(); };

  // @mention autocomplete
  let sel = 0, matches = [];
  const hidePop = () => { pop.classList.add("hidden"); matches = []; };
  const query = () => { const m = input.value.slice(0, input.selectionStart).match(/(^|\s)@([\w.-]*)$/); return m ? m[2].toLowerCase() : null; };
  const showPop = () => {
    const q = query();
    if (q === null) return hidePop();
    const dmRoom = chat.room < 0;
    const all = [...(S.ai && !dmRoom ? [{ username: "ai", name: "Hiveloop AI", id: -1 }] : []), ...(dmRoom ? [] : [{ username: "tutti", name: t("everyone"), id: 0 }]), ...members.filter((m) => m.id !== S.me.id)];
    matches = all.filter((m) => m.username.toLowerCase().startsWith(q) || (m.name || "").toLowerCase().split(/\s+/).some((p) => p.startsWith(q))).slice(0, 6);
    if (!matches.length) return hidePop();
    sel = Math.min(sel, matches.length - 1);
    pop.innerHTML = matches.map((m, i) => `<button type="button" class="${i === sel ? "on" : ""}" data-i="${i}">${m.id > 0 ? `${avatar(m.id, m)}` : m.id < 0 ? `<span class="avatar sm aiav" data-uid="ai">${icon("ai")}</span>` : `<span class="avatar sm">${icon("at")}</span>`}<b>${esc(m.name || m.username)}</b><span class="mono muted">@${esc(m.username)}</span></button>`).join("");
    pop.classList.remove("hidden");
    $$("[data-i]", pop).forEach((b) => (b.onmousedown = (e) => { e.preventDefault(); pick(+b.dataset.i); }));
  };
  const pick = (i) => {
    const m = matches[i]; if (!m) return;
    const pos = input.selectionStart, before = input.value.slice(0, pos).replace(/@([\w.-]*)$/, "@" + m.username + " ");
    input.value = before + input.value.slice(pos);
    input.setSelectionRange(before.length, before.length);
    hidePop(); input.focus();
  };
  input.oninput = () => { input.style.height = "auto"; input.style.height = Math.min(input.scrollHeight, 160) + "px"; sel = 0; showPop(); };
  input.onkeydown = (e) => {
    if (matches.length && !pop.classList.contains("hidden")) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); sel = (sel + (e.key === "ArrowDown" ? 1 : matches.length - 1)) % matches.length; showPop(); return; }
      if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pick(sel); return; }
      if (e.key === "Escape") { hidePop(); return; }
    }
    if (e.key === "Escape" && replyTo) { clearReply(); return; }
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing && !matchMedia("(pointer: coarse)").matches) { e.preventDefault(); send(); }
  };
  input.onblur = () => setTimeout(hidePop, 120);

  // attach one of my documents (shares it with this chat)
  $("[data-attach]", v).onclick = async () => {
    const r = await api("GET", "/api/docs").catch(() => null); if (!r) return;
    const body = h(`<div><p class="muted" style="margin-top:0">${t("attach_hint")}</p><div class="list">${r.docs.map((d, i) => `<button class="li" data-i="${i}" style="width:100%;background:none;border:0;border-bottom:1px solid var(--line);text-align:left"><div class="ficon">${icon(fileIcon(d))}</div><div class="grow"><div class="t">${esc(d.name)}</div><div class="s mono">${fmtSize(d.size)}</div></div>${icon("send")}</button>`).join("") || `<div class="empty">${t("no_docs")}</div>`}</div></div>`);
    const s = sheet({ title: t("attach"), body });
    $$("[data-i]", body).forEach((b) => (b.onclick = () => { s.close(); send(r.docs[+b.dataset.i].id); }));
  };
  // documents shared in this room
  // capogruppo: shown in the header; "Vota" while the group has none
  function leaderUI() {
    const g = chat.room ? groupById(chat.room) : null;
    const el = $("[data-rleader]", v), btn = $("[data-vote]", v);
    el.innerHTML = g && g.leader_id ? `${icon("crown")}<span>${esc(g.leader_id === S.me.id ? t("you_obj") : g.leader_name)}</span>` : "";
    btn.classList.toggle("hidden", !(g && !g.leader_id && g.is_member));
  }
  chat.leaderUI = leaderUI;
  $("[data-vote]", v).onclick = () => voteSheet(groupById(chat.room));
  $("[data-rdocs]", v).onclick = async () => {
    const body = h(`<div><div class="typing"><i></i><i></i><i></i></div></div>`);
    sheet({ title: `${t("room_docs")} · ${roomName(chat.rooms.find((x) => x.id === chat.room))}`, body, wide: true });
    const r = await api("GET", `/api/docs?group=${chat.room}`).catch(() => null);
    if (!r) return;
    const all = [...r.docs, ...r.shared];
    const g = groupById(chat.room), manage = isAdmin() || g?.is_leader;
    const draw = (list) => {
      body.innerHTML = `<div class="list">${list.map((d, i) => docRow({ ...d, owner: d.owner || S.me.name || S.me.username }, i, false)
        .replace('<div class="acts">', `<div class="acts">${manage && chat.room && d.shares.includes(chat.room) ? `<button class="btn sm ghost act danger" data-unshare="${d.id}">${icon("x")}<span class="lbl">${t("remove_from_group")}</span></button>` : ""}`)).join("") || `<div class="empty">${t("no_shared")}</div>`}</div>`;
      $$("[data-prev]", body).forEach((b) => (b.onclick = () => previewDoc(all.find((d) => d.id == b.dataset.prev))));
      $$("[data-editdoc]", body).forEach((b) => (b.onclick = () => editDoc(all.find((d) => d.id == b.dataset.editdoc))));
      $$("[data-unshare]", body).forEach((b) => (b.onclick = async () => {
        const d = all.find((x) => x.id == b.dataset.unshare);
        if (!(await confirmSheet(`${d.name} — ${t("remove_from_group")}`))) return;
        try { await api("DELETE", `/api/docs/${d.id}/shares/${chat.room}`); all.splice(all.indexOf(d), 1); draw(all); toast(t("saved")); } catch (e) { fail(e); }
      }));
    };
    draw(all);
  };

  chat.rooms = await api("GET", "/api/rooms");
  const noRoom = args[0] === undefined || args[0] === "";
  if (noRoom && phone()) {
    wrap.classList.add("show-side");
    chat.live = { room: null, add() {}, remove() {}, resync() {}, typing() {}, rooms: renderRooms };
    renderRooms();
    return () => { chat.live = null; unfit(); };
  }
  if (noRoom) {
    const cur = chat.rooms.find((r) => r.id === chat.room);
    const hot = chat.rooms.find((r) => r.mentions) || chat.rooms.find((r) => r.unread);
    if (hot && !(cur && (cur.mentions || cur.unread))) chat.room = hot.id;
  }
  await openRoom(chat.room);
  return () => { chat.live = null; unfit(); };
}

/* Chat/assistant panel height: fills the space that is really visible, above the
   on-screen keyboard too. While typing on a phone the bottom bar and the title hide. */
function fitPanel(wrap) {
  const vv = window.visualViewport;
  const fit = () => {
    const mobile = matchMedia("(max-width: 760px)").matches;
    const typing = mobile && !!document.activeElement?.matches?.("textarea, input:not([type=checkbox]):not([type=radio])");
    const kb = mobile && (typing || (vv && window.innerHeight - vv.height > 120));
    document.body.classList.toggle("kb-open", kb);
    if (kb) window.scrollTo(0, 0);
    requestAnimationFrame(() => {
      const visible = vv ? vv.height : window.innerHeight;
      const top = wrap.getBoundingClientRect().top + (vv ? -vv.offsetTop : 0);
      const nav = mobile && !kb ? ($(".nav")?.offsetHeight || 0) + 10 : 6;
      const zf = parseFloat(document.documentElement.style.zoom) || 1;
      wrap.style.setProperty("--ai-h", Math.max(180, Math.floor((visible - top) / zf - nav - (mobile ? 0 : 14))) + "px");
      const msgs = $(".msgs", wrap);
      if (kb && msgs) msgs.scrollTop = msgs.scrollHeight;
    });
  };
  const later = () => { fit(); setTimeout(fit, 120); setTimeout(fit, 400); };
  fit();
  addEventListener("resize", fit);
  vv?.addEventListener("resize", fit);
  vv?.addEventListener("scroll", fit);
  document.addEventListener("focusin", later);
  document.addEventListener("focusout", later);
  return () => {
    removeEventListener("resize", fit);
    vv?.removeEventListener("resize", fit);
    vv?.removeEventListener("scroll", fit);
    document.removeEventListener("focusin", later);
    document.removeEventListener("focusout", later);
    document.body.classList.remove("kb-open");
  };
}

/* ═════════ AI assistant ═════════ */
const ai = { conv: store.get("aiConv", null), busy: false };

async function viewAI(v) {
  v.innerHTML = pageHead("", "", "",
    `<button class="btn mtop" data-side>${icon("chat")}</button><button class="btn primary" data-new>${icon("plus")}<span>${t("new_chat")}</span></button>`).replace('<header class="ph bar">', '<header class="ph bar"><span class="ai-count big hidden" data-aquota></span>') +
    `<div class="ai">
      <aside class="side"><div class="mono muted" style="padding:4px 10px">${t("chats")}</div><div class="convs" data-convs></div></aside>
      <section class="main"><div class="msgs" data-msgs></div>
        <div class="composer"><form data-form><textarea rows="1" placeholder="${t("ask_placeholder")}" data-input></textarea><button class="ibtn" aria-label="${t("send")}">${icon("send")}</button></form></div>
      </section></div>`;
  v.classList.add("ai-view");
  const wrap = $(".ai", v);
  const unfit = fitPanel(wrap);
  const msgsEl = $("[data-msgs]", v), input = $("[data-input]", v), convsEl = $("[data-convs]", v);
  let convs = [];
  const quota = () => aiQuotaFor(0).then((q) => { if (q) { S.aiQuota = q; aiCount($("[data-aquota]", v), q); } });
  aiCount($("[data-aquota]", v), S.aiQuota); quota();

  const renderConvs = () => {
    convsEl.innerHTML = convs.map((c) => `<div class="conv ${c.id === ai.conv ? "on" : ""}" data-c="${c.id}">${icon("chat")}<span class="t">${esc(c.title || t("new_chat"))}</span><button class="ibtn sm ghost danger" data-dc="${c.id}">${icon("trash")}</button></div>`).join("");
    $$("[data-c]", convsEl).forEach((el) => (el.onclick = (e) => { if (e.target.closest("[data-dc]")) return; openConv(+el.dataset.c); wrap.classList.remove("show-side"); }));
    $$("[data-dc]", convsEl).forEach((b) => (b.onclick = async () => {
      try { await api("DELETE", `/api/chats/${b.dataset.dc}`); convs = convs.filter((c) => c.id != b.dataset.dc); if (ai.conv == b.dataset.dc) { ai.conv = null; store.set("aiConv", null); hero(); } renderConvs(); } catch (e) { fail(e); }
    }));
  };
  const hero = () => {
    msgsEl.innerHTML = `<div class="ai-hero rv"><span class="serif">${lang() === "it" ? "Ciao" : "Hi"} ${esc((S.me.name || S.me.username).split(" ")[0])}, ${lang() === "it" ? "come posso aiutarti?" : "how can I help?"}</span>
      <div class="sugg">${[1, 2, 3].map((i) => `<button data-sg>${icon("ai")}<span>${t("ai_suggest_" + i)}</span></button>`).join("")}</div></div>`;
    $$("[data-sg]", msgsEl).forEach((b) => (b.onclick = () => { input.value = b.textContent; send(); }));
  };
  const toolChip = (r) => {
    const ok = r.ok;
    const el = h(`<button class="tool ${ok ? "ok" : "bad"}">${icon(ok ? "check" : "x")}<span>${esc(t("tool_" + r.name))}${r.summary && !/^\d+$/.test(r.summary) ? ": " + esc(r.summary) : ""}</span></button>`);
    if (ok && r.ref?.event_id) el.onclick = () => { cal.scope = "all"; location.hash = `#/calendar/${r.ref.start.slice(0, 10)}`; };
    else if (ok && r.ref?.doc_id) el.onclick = () => { location.hash = `#/docs/${r.ref.doc_id}`; };
    else if (ok && /^https?:\/\//.test(r.ref?.url || "")) el.onclick = () => window.open(r.ref.url, "_blank", "noopener,noreferrer");
    else el.disabled = true;
    return el;
  };
  const addMsg = (m) => {
    if (m.role === "user") { const el = h(`<div class="msg user"><div class="bubble"></div></div>`); $(".bubble", el).textContent = m.content; msgsEl.append(el); return el; }
    const el = h(`<div class="msg assistant"><div class="mark" data-uid="ai">${icon("ai")}</div><div class="body"><div class="tools"></div><div class="md"></div></div></div>`);
    const meta = m.meta || {};
    (meta.tools || []).forEach((r) => $(".tools", el).append(toolChip(r)));
    $(".md", el).innerHTML = md(m.content);
    if (meta.error) $(".md", el).insertAdjacentHTML("beforeend", `<p class="badge bad">${esc(t(meta.error) || meta.error)}</p>`);
    msgsEl.append(el);
    return el;
  };
  const scroll = () => (msgsEl.scrollTop = msgsEl.scrollHeight);
  const openConv = async (id) => {
    ai.conv = id; store.set("aiConv", id); renderConvs();
    try {
      const msgs = await api("GET", `/api/chats/${id}`);
      msgsEl.innerHTML = "";
      if (!msgs.length) return hero();
      msgs.forEach(addMsg); scroll();
    } catch { ai.conv = null; store.set("aiConv", null); hero(); }
  };
  const newConv = () => { ai.conv = null; store.set("aiConv", null); renderConvs(); hero(); if (!phone()) input.focus(); wrap.classList.remove("show-side"); };

  async function send() {
    const text = input.value.trim();
    if (!text || ai.busy) return;
    ai.busy = true; input.value = ""; input.style.height = "";
    try {
      if (!ai.conv) { const c = await api("POST", "/api/chats"); ai.conv = c.id; store.set("aiConv", c.id); convs.unshift({ ...c, title: text.slice(0, 60) }); renderConvs(); }
      if ($(".ai-hero", msgsEl)) msgsEl.innerHTML = "";
      addMsg({ role: "user", content: text });
      const el = addMsg({ role: "assistant", content: "" });
      const mdEl = $(".md", el), toolsEl = $(".tools", el);
      mdEl.innerHTML = `<div class="typing"><i></i><i></i><i></i></div>`;
      scroll();
      const res = await api("POST", `/api/chats/${ai.conv}/messages`, { content: text }, true);
      if (!res.ok) { const d = await res.json().catch(() => ({})); throw new ApiError(d.error || "error"); }
      const reader = res.body.getReader(), dec = new TextDecoder();
      let buf = "", acc = "", pending = false, running = null;
      const paint = () => { pending = false; mdEl.innerHTML = md(acc) || `<div class="typing"><i></i><i></i><i></i></div>`; const near = msgsEl.scrollHeight - msgsEl.scrollTop - msgsEl.clientHeight < 140; if (near) scroll(); };
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        let i;
        while ((i = buf.indexOf("\n\n")) >= 0) {
          const line = buf.slice(0, i); buf = buf.slice(i + 2);
          if (!line.startsWith("data: ")) continue;
          const ev = JSON.parse(line.slice(6));
          if (ev.type === "delta") { acc += ev.text; if (!pending) { pending = true; requestAnimationFrame(paint); } }
          else if (ev.type === "tool_start") { running = h(`<span class="tool run">${icon("clock")}<span>${esc(t("tool_" + ev.name))}…</span></span>`); toolsEl.append(running); scroll(); }
          else if (ev.type === "tool") { running?.remove(); running = null; toolsEl.append(toolChip(ev.result)); }
          else if (ev.type === "error") { acc += `\n\n<span class="badge bad">${esc(ev.error)}</span>`; }
        }
      }
      paint();
      convs = await api("GET", "/api/chats"); renderConvs();
    } catch (e) { fail(e); }
    ai.busy = false;
    quota();
  }

  $("[data-form]", v).onsubmit = (e) => { e.preventDefault(); send(); };
  input.onkeydown = (e) => { if (e.key === "Enter" && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); } };
  input.oninput = () => { input.style.height = "auto"; input.style.height = Math.min(input.scrollHeight, 180) + "px"; };
  $("[data-new]", v).onclick = newConv;
  $("[data-side]", v).onclick = () => wrap.classList.toggle("show-side");

  convs = await api("GET", "/api/chats");
  renderConvs();
  if (ai.conv && convs.some((c) => c.id === ai.conv)) await openConv(ai.conv); else { ai.conv = null; hero(); }
  return () => unfit();
}

/* ═════════ admin ═════════ */
async function viewAdmin(v) {
  v.innerHTML = pageHead("", "", "", `<button class="btn primary" data-new>${icon("plus")}<span>${t("new_user")}</span></button>`) +
    `<div data-users></div>`;
  const usersEl = $("[data-users]", v);

  const renderUsers = async () => {
    const all = await api("GET", "/api/users");
    S.users = all.filter((u) => u.status !== "pending");
    const pending = all.filter((u) => u.status === "pending");
    S.pending = pending.length; pendingBadge();
    await loadGroups();
    const can = (u) => u.role !== "superadmin" && (isSuper() || u.role === "user");
    const modes = ["closed", "approval", "open"];
    usersEl.innerHTML = `
    ${isSuper() ? `<div class="regbox card"><div><b>${t("registration")}</b><small class="muted">${t("reg_" + (S.registration || "approval") + "_d")}</small></div>
      <div class="seg">${modes.map((m) => `<button data-reg="${m}" class="${(S.registration || "approval") === m ? "on" : ""}">${t("reg_" + m)}</button>`).join("")}</div></div>` : ""}
    <div class="card looks" data-looks>
      <div><b>${t("looks_title")}</b><small class="muted">${t("looks_hint")}</small></div>
      <div class="looks-row"><span class="rdot ${generalImg() ? "pic" : ""}" style="--c:${esc(generalColor())}">${generalImg() ? `<img alt="" src="${generalImg()}">` : icon("globe")}</span>
        <div class="grow"><div class="lbl">${t("general_chat")}</div>
          <div class="colors sm">${["", ...GROUP_COLORS].map((c, i) => `<input type="radio" name="gcol" id="gc${i}" value="${c}" ${(S.general?.color || "") === c ? "checked" : ""}><label for="gc${i}" style="--c:${c || "var(--ink)"}" title="${c || t("accent_default")}"></label>`).join("")}</div>
          <div class="gimg-btns"><label class="btn sm">${icon("upload")}${t(generalImg() ? "change_image" : "choose_image")}<input type="file" accept="image/*" hidden data-genimg></label>
          ${generalImg() ? `<button type="button" class="btn sm ghost danger" data-gennoimg>${icon("x")}${t("remove_image")}</button>` : ""}</div></div></div>
      ${S.aiSwitch ? `<div class="looks-row"><span class="avatar xlai aiav" data-uid="ai">${icon("ai")}</span>
        <div class="grow"><div class="lbl">${t("ai_avatar")}</div>
          <div class="gimg-btns"><button type="button" class="btn sm" data-aiav>${icon("edit")}${t(avatarsById.ai ? "avatar_edit" : "avatar_create")}</button>
          ${avatarsById.ai ? `<button type="button" class="btn sm ghost danger" data-aiavdel>${icon("x")}${t("avatar_remove")}</button>` : ""}</div></div></div>` : ""}
    </div>
    ${isSuper() ? `<div class="regbox card aibox"><div><b>${t("ai_switch")}</b><small class="muted">${t(!S.aiSwitch ? "ai_off_hint" : S.ai ? "ai_on_hint" : "ai_hours_out")}</small></div>
      <div class="seg" data-aisw><button type="button" data-v="1" class="${S.aiSwitch ? "on" : ""}">${icon("ai")}${t("ai_on")}</button><button type="button" data-v="0" class="${S.aiSwitch ? "" : "on"}">${t("ai_off")}</button></div>
      ${S.aiSwitch ? `<div class="ai-sched" data-sched>
        <label class="check"><input type="checkbox" data-son ${S.aiSched.on ? "checked" : ""}>${t("ai_hours")}</label>
        <div class="sched-body ${S.aiSched.on ? "" : "hidden"}">
          <div class="sched-times"><label>${t("from")} <input class="input" type="time" data-sfrom value="${esc(S.aiSched.from || "08:00")}"></label><label>${t("to")} <input class="input" type="time" data-sto value="${esc(S.aiSched.to || "20:00")}"></label></div>
          <div class="chips sched-days">${[1, 2, 3, 4, 5, 6, 7].map((d) => `<label class="chip ${!(S.aiSched.days || []).length || S.aiSched.days.includes(d) ? "on" : ""}"><input type="checkbox" hidden data-sday="${d}" ${!(S.aiSched.days || []).length || S.aiSched.days.includes(d) ? "checked" : ""}>${esc(fmt(new Date(2024, 0, d), { weekday: "short" }))}</label>`).join("")}</div>
          <small class="hint">${t("ai_hours_hint")}</small>
        </div>
        <div><button type="button" class="btn sm primary" data-ssave>${icon("check")}${t("save")}</button></div>
      </div>` : ""}</div>` : ""}
    ${isSuper() && S.aiSwitch ? `<div class="card aiconf" data-aiconf>
      <div><b>${t("aic_title")}</b><small class="muted">${t("aic_hint")}</small></div>
      <div class="seg" data-aicp><button type="button" data-v="ollama">Ollama</button><button type="button" data-v="openai">${t("aic_openai")}</button></div>
      <label class="field"><span>${t("aic_url")}</span><input class="input" data-aicurl spellcheck="false" autocomplete="off"></label>
      <small class="hint" data-aichint></small>
      <label class="field"><span>${t("aic_model")}</span><input class="input" data-aicmodel spellcheck="false" autocomplete="off"></label>
      <div class="aic-models" data-aicmodels></div>
      <label class="field"><span>${t("aic_key")}</span><input class="input" type="password" data-aickey autocomplete="new-password" spellcheck="false"></label>
      <div class="aic-acts"><button type="button" class="btn sm" data-aictest>${icon("check")}${t("try")}</button><button type="button" class="btn sm primary" data-aicsave>${icon("check")}${t("save")}</button><button type="button" class="btn sm ghost danger hidden" data-aicclear>${icon("x")}${t("aic_clear_key")}</button></div>
      <div class="aic-result" data-aicres></div>
    </div>` : ""}
    ${isSuper() ? `<div class="card limits" data-limits>
      <div><b>${t("limits_title")}</b><small class="muted">${t("limits_hint")}</small></div>
      <div class="lim-row"><span>${t("feat_create")}</span><div class="seg" data-feat="doc_create"><button type="button" data-v="1">${t("ai_on")}</button><button type="button" data-v="0">${t("ai_off")}</button></div></div>
      <div class="lim-row"><span>${t("feat_upload")}</span><div class="seg" data-feat="doc_upload"><button type="button" data-v="1">${t("ai_on")}</button><button type="button" data-v="0">${t("ai_off")}</button></div></div>
      <div class="lim-row"><span>${t("space_default")}</span><div class="lim-in"><input class="input" type="number" min="1" max="100000" inputmode="numeric" data-lim="quota_mb"><span class="muted">MB</span></div></div>
      ${S.aiSwitch ? `<div class="lim-row"><span>${t("ai_total")} <small class="muted" data-aitoday></small></span><div class="lim-in"><input class="input" type="number" min="0" max="1000000" inputmode="numeric" data-lim="ai_total" placeholder="${t("unlimited")}"><span class="muted">${t("per_day")}</span></div></div>` : ""}
      ${S.aiSwitch ? `<div class="lim-row"><span>${t("ai_default")}</span><div class="lim-in"><input class="input" type="number" min="0" max="10000" inputmode="numeric" data-lim="ai_daily" placeholder="${t("unlimited")}"><span class="muted">${t("per_day")}</span></div></div>` : ""}
      <div><button type="button" class="btn sm primary" data-limsave>${icon("check")}${t("save")}</button></div>
      ${S.aiSwitch ? `<div class="glim" data-glim>
        <div class="lim-row"><span><b>${t("glim_title")}</b></span><div class="seg" data-gmode><button type="button" data-v="">${t("glim_none")}</button><button type="button" data-v="auto">${t("glim_auto")}</button><button type="button" data-v="manual">${t("glim_manual")}</button></div></div>
        <div data-glist></div>
      </div>` : ""}
    </div>` : ""}
    ${isSuper() && S.aiSwitch ? `<div class="regbox card searchbox"><div><b>${t("web_search")}</b><small class="muted" data-srch>${t("loading")}</small></div>
      <div class="srch-act"><input class="input" type="password" autocomplete="off" spellcheck="false" placeholder="${t("brave_key_ph")}" data-bkey><button class="btn sm primary" data-bsave>${t("save")}</button><button class="btn sm" data-btest>${t("try")}</button><button class="btn sm ghost danger hidden" data-bdel>${t("remove")}</button></div></div>` : ""}
    ${pending.length ? `<div class="section-t" style="margin-top:6px"><h2>${t("pending_requests")} <span class="mono muted">${pending.length}</span></h2></div>
      <div class="list">${pending.map((u) => `<div class="li"><div class="avatar">${esc(initials(u))}</div>
        <div class="grow"><div class="t">${esc(u.name || u.username)}</div><div class="s mono">@${esc(u.username)} · ${fmt(new Date(u.created_at * 1000), { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}</div></div>
        <button class="btn sm primary" data-ok-u="${u.id}">${icon("check")}${t("approve")}</button><button class="ibtn sm ghost danger" data-no-u="${u.id}" title="${t("reject")}">${icon("x")}</button></div>`).join("")}</div>
      <div class="section-t"><h2>${t("users")}</h2></div>` : ""}
    <div class="list utable">${S.users.map((u, i) => `<div class="li rv" style="--i:${Math.min(i, 12)}">
      ${avatar(u.id, u)}
      <div class="grow"><div class="t">${esc(u.name || u.username)} ${u.id === S.me.id ? `<span class="muted">(${t("you")})</span>` : ""}</div><div class="s mono">@${esc(u.username)}${u.usage ? ` · ${usageText(u.usage)}` : ""}</div></div>
      <span class="badge ${u.role !== "user" ? "acc" : ""}">${t("role_" + u.role)}</span>
      <div class="acts">${can(u) && u.id !== S.me.id ? `<button class="btn sm ghost act" data-ed="${u.id}" title="${t("edit")}">${icon("edit")}<span class="lbl">${t("edit")}</span></button><button class="btn sm ghost act danger" data-del="${u.id}" title="${t("delete")}">${icon("trash")}<span class="lbl">${t("delete")}</span></button>` : ""}</div>
    </div>`).join("")}</div>`;
    $$("[data-ed]", usersEl).forEach((b) => (b.onclick = () => userForm(S.users.find((u) => u.id == b.dataset.ed), renderUsers)));
    // general chat colour/photo and the assistant's avatar (admins)
    {
      const box = $("[data-looks]", usersEl);
      const refresh = async () => { await loadMe(); await loadAvatars(); renderUsers(); };
      $$("[name=gcol]", box).forEach((r) => (r.onchange = async () => { try { S.general = await api("PUT", "/api/settings/general", { color: r.value }); toast(t("saved")); } catch (e) { fail(e); } }));
      $("[data-genimg]", box).onchange = async (e) => {
        const f = e.target.files[0]; if (!f) return;
        try { S.general = await api("PUT", "/api/settings/general/image", await shrinkImage(f)); toast(t("saved")); refresh(); } catch (err) { fail(err); }
      };
      $("[data-gennoimg]", box)?.addEventListener("click", async () => { if (!(await confirmSheet(t("remove_image") + "?"))) return; try { S.general = await api("DELETE", "/api/settings/general/image"); refresh(); } catch (e) { fail(e); } });
      const saveAI = async (o) => { try { await api("PUT", "/api/settings/ai-avatar", { avatar: o || {} }); toast(t("saved")); refresh(); return true; } catch (e) { fail(e); return false; } };
      $$("[data-aisw] [data-v]", usersEl).forEach((b) => (b.onclick = async () => {
        const on = b.dataset.v === "1";
        if (on === S.aiSwitch) return;
        if (!on && !(await confirmSheet(t("ai_off_q")))) return;
        try { await api("PUT", "/api/settings/ai", { enabled: on }); await loadMe(); toast(t("saved")); renderShell(); route(); } catch (e) { fail(e); }
      }));
      // AI provider (super admin): Ollama or any OpenAI-compatible API
      const ac = $("[data-aiconf]", usersEl);
      if (ac) {
        let prov = "ollama", saved = null, mtimer = null;
        const ph = { ollama: "http://127.0.0.1:11434", openai: "https://api.openai.com/v1" };
        const keyPh = (k) => (k.key_set ? `${t("aic_key_saved")} (${k.key_hint})${k.key_from_env ? " · env" : ""}` : t(prov === "ollama" ? "aic_key_opt" : "aic_key_ph"));
        const showConf = (c) => {
          saved = c; prov = c.provider;
          $("[data-aichint]", ac).textContent = t(prov === "ollama" ? "aic_ollama_hint" : "aic_openai_hint");
          $$("[data-aicp] [data-v]", ac).forEach((b) => b.classList.toggle("on", b.dataset.v === prov));
          $("[data-aicurl]", ac).value = c.url || ""; $("[data-aicurl]", ac).placeholder = ph[prov];
          $("[data-aicmodel]", ac).value = c.model || "";
          $("[data-aickey]", ac).value = "";
          $("[data-aickey]", ac).placeholder = keyPh(c);
          $("[data-aicclear]", ac).classList.toggle("hidden", !c.key_set || c.key_from_env);
          loadModels();
        };
        // models of the address in the form, as buttons to pick from
        const loadModels = async () => {
          const box = $("[data-aicmodels]", ac);
          box.innerHTML = `<span class="muted">${t("loading")}</span>`;
          let r;
          try { r = await api("POST", "/api/settings/ai-config/models", form()); } catch { box.innerHTML = ""; return; }
          $("[data-aickey]", ac).placeholder = keyPh(r);
          const cur = $("[data-aicmodel]", ac).value;
          box.innerHTML = r.models?.length
            ? r.models.map((m) => `<button type="button" class="chip ${m === cur ? "on" : ""}" data-m="${esc(m)}">${esc(m)}</button>`).join("")
            : `<span class="muted">${t(r.key_set || prov === "ollama" ? "aic_no_models" : "aic_need_key")}</span>`;
          $$("[data-m]", box).forEach((b) => (b.onclick = () => { $("[data-aicmodel]", ac).value = b.dataset.m; $$("[data-m]", box).forEach((x) => x.classList.toggle("on", x === b)); }));
        };
        const later = () => { clearTimeout(mtimer); mtimer = setTimeout(loadModels, 700); };
        $("[data-aicurl]", ac).addEventListener("input", later);
        $("[data-aickey]", ac).addEventListener("input", later);
        const form = () => {
          const b = { provider: prov, url: $("[data-aicurl]", ac).value.trim() || ph[prov], model: $("[data-aicmodel]", ac).value.trim() || undefined };
          const k = $("[data-aickey]", ac).value.trim(); if (k) b.key = k;
          return b;
        };
        api("GET", "/api/settings/ai-config").then(showConf).catch(() => {});
        // switching service: its own address (the saved one if it is the saved service), no model, models reloaded
        $$("[data-aicp] [data-v]", ac).forEach((b) => (b.onclick = () => {
          if (b.dataset.v === prov) return;
          prov = b.dataset.v;
          $$("[data-aicp] [data-v]", ac).forEach((x) => x.classList.toggle("on", x === b));
          const same = saved && saved.provider === prov;
          const u = $("[data-aicurl]", ac); u.placeholder = ph[prov]; u.value = same ? saved.url : ph[prov];
          $("[data-aicmodel]", ac).value = same ? saved.model : "";
          $("[data-aickey]", ac).value = "";
          $("[data-aichint]", ac).textContent = t(prov === "ollama" ? "aic_ollama_hint" : "aic_openai_hint");
          $("[data-aicres]", ac).innerHTML = "";
          loadModels();
        }));
        const res = $("[data-aicres]", ac);
        $("[data-aictest]", ac).onclick = async (e) => {
          const btn = e.currentTarget; btn.disabled = true;
          res.className = "aic-result"; res.innerHTML = `<div class="typing"><i></i><i></i><i></i></div>`;
          try {
            const r = await api("POST", "/api/settings/ai-config/test", form());
            res.classList.add(r.error ? "bad" : "ok");
            res.innerHTML = r.error
              ? `${icon("x")}<span>${esc(t("aic_fail"))}: ${esc(r.error)}</span>`
              : `${icon("check")}<span>${esc(t("aic_ok"))} (${(r.ms / 1000).toFixed(1)} s): “${esc(r.reply)}”${r.models?.length ? ` · ${r.models.length} ${t("aic_models_found")}` : ""}</span>`;
          } catch (err) { res.innerHTML = ""; fail(err); }
          btn.disabled = false;
        };
        $("[data-aicsave]", ac).onclick = async () => { try { showConf(await api("PUT", "/api/settings/ai-config", form())); toast(t("saved")); } catch (e) { fail(e); } };
        $("[data-aicclear]", ac).onclick = async () => {
          if (!(await confirmSheet(t("aic_clear_key") + "?"))) return;
          try { showConf(await api("PUT", "/api/settings/ai-config", { clear_key: true })); toast(t("saved")); } catch (e) { fail(e); }
        };
      }
      const sch = $("[data-sched]", usersEl);
      if (sch) {
        $("[data-son]", sch).onchange = (e) => $(".sched-body", sch).classList.toggle("hidden", !e.target.checked);
        $$("[data-sday]", sch).forEach((c) => (c.onchange = () => c.parentElement.classList.toggle("on", c.checked)));
        $("[data-ssave]", sch).onclick = async () => {
          const days = $$("[data-sday]", sch).filter((c) => c.checked).map((c) => +c.dataset.sday);
          const body = { on: $("[data-son]", sch).checked, from: $("[data-sfrom]", sch).value, to: $("[data-sto]", sch).value, days: days.length === 7 ? [] : days };
          if (body.on && !days.length) return toast(t("ai_hours_nodays"), true);
          try { await api("PUT", "/api/settings/ai/schedule", body); await loadMe(); toast(t("saved")); renderShell(); route(); } catch (e) { fail(e); }
        };
      }
      $("[data-aiav]", box)?.addEventListener("click", () => avatarEditor(avatarsById.ai, { sheet, icon, onSave: saveAI }));
      $("[data-aiavdel]", box)?.addEventListener("click", async () => { if (await confirmSheet(t("avatar_remove_q"))) saveAI(null); });
    }
    if (isSuper()) {
      const box = $("[data-limits]", usersEl);
      const show = (l) => {
        $$("[data-feat]", box).forEach((g) => $$("[data-v]", g).forEach((b) => b.classList.toggle("on", (b.dataset.v === "1") === !!l[g.dataset.feat])));
        $("[data-lim=quota_mb]", box).value = l.quota_mb;
        const ai = $("[data-lim=ai_daily]", box); if (ai) ai.value = l.ai_daily || "";
        const tot = $("[data-lim=ai_total]", box); if (tot) tot.value = l.ai_total || "";
        const td = $("[data-aitoday]", box); if (td) td.textContent = `· ${t("today")}: ${l.ai_total_today}`;
      };
      api("GET", "/api/settings/limits").then(show).catch(() => {});
      $$("[data-feat] [data-v]", box).forEach((b) => (b.onclick = async () => {
        try { show(await api("PUT", "/api/settings/limits", { [b.closest("[data-feat]").dataset.feat]: b.dataset.v === "1" })); toast(t("saved")); } catch (e) { fail(e); }
      }));
      // group limits: automatic share of the global limit, or manual with suggestions
      const gl = $("[data-glim]", box);
      const drawGroups = (d) => {
        if (!gl) return;
        $$("[data-gmode] [data-v]", gl).forEach((b) => b.classList.toggle("on", b.dataset.v === d.mode));
        const list = $("[data-glist]", gl);
        const used = (g) => `<span class="muted">${t("today")}: ${g.used}</span>`;
        const members = (g) => `${g.members} ${g.members === 1 ? t("member") : t("members")}`;
        if (d.mode === "") { list.innerHTML = `<p class="muted glim-note">${t("glim_none_hint")}</p>`; return; }
        if (d.mode === "auto") {
          list.innerHTML = d.total > 0
            ? `<p class="muted glim-note">${t("glim_auto_hint").replace("{n}", d.total)}</p>${d.groups.map((g) => `<div class="glim-row"><span><i class="rsw" style="--c:${esc(g.color)}"></i>${esc(g.name)} <small class="muted">${members(g)}</small></span><span><b>${g.auto}</b> ${t("per_day")} · ${used(g)}</span></div>`).join("")}`
            : `<p class="warn">${icon("alert")}${t("glim_auto_needs_total")}</p>`;
          return;
        }
        // manual
        list.innerHTML = `<p class="muted glim-note">${t("glim_manual_hint")}</p>${d.groups.map((g) => `<div class="glim-row"><span><i class="rsw" style="--c:${esc(g.color)}"></i>${esc(g.name)} <small class="muted">${members(g)}</small></span>
            <span class="lim-in"><input class="input" type="number" min="0" max="1000000" inputmode="numeric" data-gid="${g.id}" value="${g.manual >= 0 ? g.manual : ""}" placeholder="${t("unlimited")}">${used(g)}</span></div>`).join("")}
          <div data-gwarn></div>
          <div><button type="button" class="btn sm primary" data-gsave>${icon("check")}${t("save")}</button></div>`;
        const check = () => {
          const vals = $$("[data-gid]", list).map((i) => ({ id: i.dataset.gid, v: i.value === "" ? -1 : Math.max(0, +i.value) }));
          const warns = [];
          const sum = vals.reduce((s, x) => s + (x.v < 0 ? 0 : x.v), 0), unlimited = vals.some((x) => x.v < 0);
          if (d.total > 0 && (sum > d.total || unlimited)) {
            warns.push(t(unlimited ? "glim_warn_unlimited" : "glim_warn_sum").replace("{sum}", sum).replace("{n}", d.total));
          }
          if (d.total > 0 && d.per_user > d.total) warns.push(t("glim_warn_user").replace("{u}", d.per_user).replace("{n}", d.total));
          const sugg = d.groups.map((g) => ({ id: String(g.id), v: g.auto }));
          const w = $("[data-gwarn]", list);
          w.innerHTML = warns.length ? `<div class="warn">${icon("alert")}<div>${warns.map((x) => `<p>${esc(x)}</p>`).join("")}
            ${d.total > 0 ? `<p>${t("glim_suggest")}: ${d.groups.map((g) => `${esc(g.name)} <b>${g.auto}</b>`).join(" · ")}</p><button type="button" class="btn sm" data-gsugg>${t("glim_use_suggested")}</button>` : ""}</div></div>` : "";
          $("[data-gsugg]", w)?.addEventListener("click", () => { sugg.forEach((x) => { const i = $(`[data-gid="${x.id}"]`, list); if (i) i.value = x.v; }); check(); });
        };
        $$("[data-gid]", list).forEach((i) => (i.oninput = check));
        check();
        $("[data-gsave]", list).onclick = async () => {
          const limits = Object.fromEntries($$("[data-gid]", list).map((i) => [i.dataset.gid, i.value === "" ? -1 : Math.max(0, +i.value)]));
          try { drawGroups(await api("PUT", "/api/settings/ai-groups", { limits })); toast(t("saved")); } catch (e) { fail(e); }
        };
      };
      if (gl) {
        api("GET", "/api/settings/ai-groups").then(drawGroups).catch(() => {});
        $$("[data-gmode] [data-v]", gl).forEach((b) => (b.onclick = async () => {
          try { drawGroups(await api("PUT", "/api/settings/ai-groups", { mode: b.dataset.v })); } catch (e) { fail(e); }
        }));
      }
      $("[data-limsave]", box).onclick = async () => {
        const body = { quota_mb: +$("[data-lim=quota_mb]", box).value };
        const ai = $("[data-lim=ai_daily]", box); if (ai) body.ai_daily = +ai.value || 0;
        const tot = $("[data-lim=ai_total]", box); if (tot) body.ai_total = +tot.value || 0;
        try { show(await api("PUT", "/api/settings/limits", body)); toast(t("saved")); renderUsers(); } catch (e) { fail(e); }
        if (gl) api("GET", "/api/settings/ai-groups").then(drawGroups).catch(() => {});
      };
    }
    if (isSuper() && S.aiSwitch) {
      const info = $("[data-srch]", usersEl), del = $("[data-bdel]", usersEl);
      const show = (s) => { info.textContent = s.brave ? `${t("brave_on")} (${s.hint})` : t("brave_off"); del.classList.toggle("hidden", !s.brave || s.from_env); };
      api("GET", "/api/settings/search").then(show).catch(() => {});
      $("[data-bsave]", usersEl).onclick = async () => {
        const k = $("[data-bkey]", usersEl).value.trim(); if (!k) return;
        try { show(await api("PUT", "/api/settings/search", { brave_key: k })); $("[data-bkey]", usersEl).value = ""; toast(t("saved")); } catch (e) { fail(e); }
      };
      del.onclick = async () => { if (!(await confirmSheet(t("brave_remove_q")))) return; try { show(await api("PUT", "/api/settings/search", { brave_key: "" })); } catch (e) { fail(e); } };
      $("[data-btest]", usersEl).onclick = async (e) => {
        const b = e.currentTarget; b.disabled = true;
        try { const r = await api("POST", "/api/settings/search/test"); toast(`${t("search_ok")} ${r.engine} · ${r.results.length} ${t("results")}`); } catch (err) { fail(err); }
        b.disabled = false;
      };
    }
    $$("[data-reg]", usersEl).forEach((b) => (b.onclick = async () => { try { const r = await api("PUT", "/api/settings/registration", { mode: b.dataset.reg }); S.registration = r.registration; regMode = r.registration; toast(t("saved")); renderUsers(); } catch (e) { fail(e); } }));
    $$("[data-ok-u]", usersEl).forEach((b) => (b.onclick = async () => { try { await api("POST", `/api/users/${b.dataset.okU}/approve`); toast(t("approved")); renderUsers(); } catch (e) { fail(e); } }));
    $$("[data-no-u]", usersEl).forEach((b) => (b.onclick = async () => {
      const u = pending.find((x) => x.id == b.dataset.noU);
      if (!(await confirmSheet(`@${u.username}`))) return;
      try { await api("DELETE", `/api/users/${u.id}`); toast(t("deleted")); renderUsers(); } catch (e) { fail(e); }
    }));
    $$("[data-del]", usersEl).forEach((b) => (b.onclick = async () => {
      const u = S.users.find((x) => x.id == b.dataset.del);
      if (!(await confirmSheet(`@${u.username}`))) return;
      try { await api("DELETE", `/api/users/${u.id}`); toast(t("deleted")); renderUsers(); } catch (e) { fail(e); }
    }));
  };
  $("[data-new]", v).onclick = () => userForm(null, renderUsers);

  liveView = { kinds: ["users", "pending", "groups"], fn: () => { if (!$(".sheet")) renderUsers(); } };
  await renderUsers();
}

function userForm(u, rerender) {
  const myEditable = S.groups;
  const form = h(`<form>
    ${u ? "" : `<label class="field"><span>${t("username")}</span><input class="input" name="username" required minlength="3" maxlength="32" autocapitalize="none" autocomplete="off" spellcheck="false"><small class="hint">${t("username_hint")}</small></label>`}
    <label class="field"><span>${t("name")}</span><input class="input" name="name" value="${esc(u?.name)}"></label>
    <label class="field"><span>${u ? t("reset_password") + " · " + t("optional") : t("password")}</span><input class="input" name="password" type="password" minlength="8" ${u ? "" : "required"} autocomplete="new-password"><small class="hint">${t("password_hint")}</small></label>
    ${isSuper() ? `<label class="field"><span>${t("role")}</span><select class="input" name="role"><option value="user">${t("role_user")}</option><option value="admin" ${u?.role === "admin" ? "selected" : ""}>${t("role_admin")}</option></select></label>` : ""}
    ${isSuper() && u?.usage ? `<div class="row">
      <label class="field"><span>${t("space_mb")}</span><input class="input" name="quota_mb" type="number" min="1" max="100000" inputmode="numeric" value="${u.usage.quota_mb || ""}" placeholder="${t("default_val")} (${u.usage.def_quota_mb})"></label>
      ${S.aiSwitch ? `<label class="field"><span>${t("ai_per_day")}</span><input class="input" name="ai_daily" type="number" min="0" max="10000" inputmode="numeric" value="${u.usage.ai_daily >= 0 ? u.usage.ai_daily : ""}" placeholder="${t("default_val")} (${u.usage.def_ai_daily || t("unlimited")})"></label>` : ""}
    </div><small class="hint">${t("limits_user_hint")}</small>` : ""}
    ${!u && myEditable.length ? `<div class="field"><span>${t("nav_groups")}</span><div class="chips" style="flex-wrap:wrap">${myEditable.map((g) => `<label class="chip" style="--c:${esc(g.color)};cursor:pointer"><input type="checkbox" name="groups" value="${g.id}" hidden><i class="sw"></i>${esc(g.name)}</label>`).join("")}</div></div>` : ""}
  </form>`);
  const un = $("input[name=username]", form);
  if (un) un.oninput = () => {
    const pos = un.selectionStart, before = un.value;
    un.value = before.normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase().replace(/\s+/g, ".").replace(/[^a-z0-9._-]/g, "");
    const np = Math.max(0, pos - (before.length - un.value.length));
    if (un.value !== before) un.setSelectionRange(np, np);
  };
  // requirement errors shown inline (red field + clear message) instead of browser bubbles
  form.noValidate = true;
  const check = () => {
    let ok = true;
    $$("input", form).forEach((i) => { const bad = !i.checkValidity(); i.classList.toggle("bad", bad); if (bad) ok = false; });
    if (!ok) { $("input.bad", form)?.focus(); toast(un && !un.checkValidity() ? t("invalid_username") : t("password_too_short"), true); }
    return ok;
  };
  $$("input[name=groups]", form).forEach((c) => (c.onchange = () => c.parentElement.classList.toggle("on", c.checked)));
  const s = sheet({ title: u ? `@${u.username}` : t("new_user"), body: form, foot: `<button class="btn" data-c>${t("cancel")}</button><button class="btn primary" data-ok>${icon("check")}${u ? t("save") : t("create")}</button>` });
  $("[data-c]", s.el).onclick = s.close;
  const submit = async () => {
    if (!check()) return;
    const f = new FormData(form);
    const body = { name: f.get("name") };
    if (f.get("password")) body.password = f.get("password");
    if (isSuper()) body.role = f.get("role");
    if (isSuper() && u?.usage) {
      body.quota_mb = +f.get("quota_mb") || 0;
      if (f.has("ai_daily")) body.ai_daily = f.get("ai_daily") === "" ? -1 : Math.max(0, +f.get("ai_daily"));
    }
    try {
      if (u) await api("PATCH", `/api/users/${u.id}`, body);
      else await api("POST", "/api/users", { ...body, username: f.get("username"), groups: f.getAll("groups").map(Number) });
      toast(t("saved")); s.close(); rerender();
    } catch (e) { fail(e); }
  };
  $("[data-ok]", s.el).onclick = submit;
  form.onsubmit = (e) => { e.preventDefault(); submit(); };
}

/* ═════════ settings ═════════ */
async function viewSettings(v) {
  v.innerHTML = `<div class="grid-cards" style="grid-template-columns:repeat(auto-fit,minmax(min(100%,320px),1fr))">
      <form class="card rv" style="padding:20px" data-prof>
        <div class="field"><span>${t("username")}</span><div class="mono">@${esc(S.me.username)} · ${t("role_" + S.me.role)}</div></div>
        <label class="field"><span>${t("name")}</span><input class="input" name="name" value="${esc(S.me.name)}"></label>
        <div class="field"><span>${t("language")}</span><div class="seg" style="width:max-content"><button type="button" data-l="en" class="${lang() === "en" ? "on" : ""}">${icon("globe")}English</button><button type="button" data-l="it" class="${lang() === "it" ? "on" : ""}">Italiano</button></div></div>
        <button class="btn primary">${icon("check")}${t("save")}</button>
      </form>
      <div class="card rv avcard" style="padding:20px;--i:1">
        <div class="field" style="margin:0"><span>${t("avatar")}</span></div>
        <div class="avrow">${avatar(S.me.id, S.me, "xl")}
          <div class="avbtns"><button type="button" class="btn primary" data-avedit>${icon("edit")}${t(S.avatar ? "avatar_edit" : "avatar_create")}</button>
          ${S.avatar ? `<button type="button" class="btn ghost danger" data-avdel>${icon("x")}${t("avatar_remove")}</button>` : ""}</div></div>
        <p class="muted" style="margin:0;font-size:13.5px">${t("avatar_hint")}</p>
      </div>
      <div class="card rv appear" style="padding:20px;--i:1" data-appear>
        <div class="field" style="margin:0"><span>${t("appearance")}</span></div>
        <div><div class="lbl">${t("theme")}</div><div class="seg" data-pref="theme">
          <button type="button" data-v="">${t("theme_system")}</button><button type="button" data-v="light">${t("theme_light")}</button><button type="button" data-v="dark">${t("theme_dark")}</button></div></div>
        <div><div class="lbl">${t("accent_color")}</div><div class="swatches" data-pref="accent">
          <button type="button" class="swatch def" data-v="" title="${t("accent_default")}" aria-label="${t("accent_default")}">${icon("check")}</button>
          ${Object.entries(ACCENTS).map(([k, c]) => `<button type="button" class="swatch" data-v="${k}" style="--c:${c}" title="${t("acc_" + k)}" aria-label="${t("acc_" + k)}">${icon("check")}</button>`).join("")}</div></div>
        <div><div class="lbl">${t("zoom")}</div><div class="seg zoomseg" data-pref="zoom">
          ${[80, 90, 0, 110, 125, 150, 175].map((z) => `<button type="button" data-v="${z || ""}">${z || 100}%</button>`).join("")}</div></div>
        <div><div class="lbl">${t("vision")}</div><div class="seg" data-pref="vision">
          <button type="button" data-v="">${t("vision_normal")}</button><button type="button" data-v="high">${icon("eye")}${t("vision_high")}</button></div></div>
        <div><div class="lbl">${t("buttons_style")}</div><div class="seg" data-pref="buttons">
          <button type="button" data-v="">${t("btn_both")}</button><button type="button" data-v="icons">${t("btn_icons")}</button><button type="button" data-v="text">${t("btn_text")}</button></div></div>
        <div class="preview-btns"><button type="button" class="btn">${icon("edit")}${t("edit")}</button><button type="button" class="btn primary">${icon("check")}${t("save")}</button></div>
      </div>
      <div class="card rv notifcard" style="padding:20px;--i:1" data-notif>
        <div class="field" style="margin:0"><span>${t("notif_title")}</span></div>
        <p class="muted" style="margin:0;font-size:13.5px">${t("notif_hint")}</p>
        <div data-nstate class="muted">…</div>
      </div>
      <form class="card rv" style="padding:20px;--i:1" data-pw>
        <div class="field"><span>${t("change_password")}</span></div>
        <label class="field"><span>${t("old_password")}</span><input class="input" type="password" name="old" required autocomplete="current-password"></label>
        <label class="field"><span>${t("new_password")}</span><input class="input" type="password" name="new" required minlength="8" autocomplete="new-password"></label>
        <button class="btn primary">${icon("key")}${t("save")}</button>
      </form>
      ${isStandalone() ? "" : `<button class="card rv guide-card" style="--i:2;text-align:left;width:100%" data-install-card>${icon("download")}<span><b>${t("install_app")}</b><small>${t("install_sub")}</small></span>${icon("right")}</button>`}
      <a class="card rv guide-card" style="--i:3" href="#/help">${icon("help")}<span><b>${t("nav_help")}</b><small>${t("help_card")}</small></span>${icon("right")}</a></div>
      <div class="logout-row"><button class="btn danger" data-out>${icon("logout")}${t("logout")}</button></div>`;
  $("[data-install-card]", v)?.addEventListener("click", installApp);
  // notifications on this device
  const nEl = $("[data-nstate]", v);
  const drawNotif = async () => {
    if (isAndroidApp()) { nEl.innerHTML = `<p style="margin:0">${icon("android")} ${t("notif_android")}</p>`; return; }
    const st = await pushState();
    if (st === "unsupported") { nEl.innerHTML = `<p style="margin:0">${t(isIOS() && !isStandalone() ? "notif_ios" : "notif_unsupported")}</p>`; return; }
    if (st === "denied") { nEl.innerHTML = `<p style="margin:0">${t("notif_denied")}</p>`; return; }
    nEl.innerHTML = `<div class="nrow"><span class="nst ${st === "on" ? "on" : ""}">${icon(st === "on" ? "bell" : "x")}${t(st === "on" ? "notif_on_dev" : "notif_off_dev")}</span><span class="grow"></span>
      ${st === "on" ? `<button type="button" class="btn sm" data-ntest>${icon("send")}${t("notif_test")}</button><button type="button" class="btn sm ghost" data-noff>${t("notif_disable")}</button>` : `<button type="button" class="btn sm primary" data-non>${icon("bell")}${t("notif_enable")}</button>`}</div>`;
    $("[data-non]", nEl)?.addEventListener("click", async () => { if (await pushEnable()) toast(t("notify_on"), false, null, "bell"); drawNotif(); });
    $("[data-noff]", nEl)?.addEventListener("click", async () => { await pushDisable().catch(() => {}); drawNotif(); });
    $("[data-ntest]", nEl)?.addEventListener("click", async () => { try { await api("POST", "/api/push/test"); toast(t("notif_test_sent"), false, null, "bell"); } catch (e) { fail(e); } });
  };
  drawNotif();
  // avatar
  const saveAvatar = async (o) => {
    try {
      await api("PATCH", "/api/me", { avatar: o || {} });
      S.avatar = o; if (o) avatarsById[S.me.id] = o; else delete avatarsById[S.me.id];
      fillAvatars(document.body); toast(t("saved")); viewSettings(v); return true;
    } catch (e) { fail(e); return false; }
  };
  $("[data-avedit]", v).onclick = () => avatarEditor(S.avatar, { sheet, icon, onSave: saveAvatar });
  $("[data-avdel]", v)?.addEventListener("click", async () => { if (await confirmSheet(t("avatar_remove_q"))) saveAvatar(null); });
  // appearance: applied at once, saved for this user (all devices)
  const showPrefs = () => $$("[data-pref]", v).forEach((g) => $$("[data-v]", g).forEach((b) => b.classList.toggle("on", String(S.prefs[g.dataset.pref] || "") === b.dataset.v)));
  showPrefs();
  $$("[data-pref] [data-v]", v).forEach((b) => (b.onclick = async () => {
    const key = b.closest("[data-pref]").dataset.pref, before = { ...S.prefs };
    S.prefs = { ...S.prefs, [key]: key === "zoom" ? +b.dataset.v || 0 : b.dataset.v };
    applyPrefs(S.prefs); showPrefs();
    try { await api("PATCH", "/api/me", { prefs: S.prefs }); } catch (e) { S.prefs = before; applyPrefs(before); showPrefs(); fail(e); }
  }));
  $("[data-out]", v).onclick = async () => { await pushDisable().catch(() => {}); await api("POST", "/api/logout").catch(() => {}); S.me = null; chat.es?.close(); location.hash = ""; renderLogin(); };
  $$("[data-l]", v).forEach((b) => (b.onclick = async () => {
    try { await api("PATCH", "/api/me", { lang: b.dataset.l }); S.me.lang = b.dataset.l; setLang(b.dataset.l); store.set("lang", b.dataset.l); renderShell(); route(); refreshBadges(); } catch (e) { fail(e); }
  }));
  $("[data-prof]", v).onsubmit = async (e) => {
    e.preventDefault();
    try { const name = new FormData(e.target).get("name"); await api("PATCH", "/api/me", { name }); S.me.name = name; toast(t("saved")); renderShell(); route(); refreshBadges(); } catch (err) { fail(err); }
  };
  $("[data-pw]", v).onsubmit = async (e) => {
    e.preventDefault();
    const f = new FormData(e.target);
    try { await api("PATCH", "/api/me", { old_password: f.get("old"), password: f.get("new") }); toast(t("saved")); e.target.reset(); } catch (err) { fail(err); }
  };
}

/* ═════════ help ═════════ */
async function viewHelp(v) {
  // with the assistant off, its section and every paragraph/list item about it disappear
  const aiRe = /assistent|assistant|@ai\b|\bAI\b|:ai:|ricerca web|web search/i;
  const noAI = (body) => body.split("\n\n").map((p) => p.split("\n").filter((l) => !(/^\s*([-*]\s|\|)/.test(l) && aiRe.test(l))).join("\n"))
    .filter((p) => p.trim() && !aiRe.test(p.split("\n")[0])).join("\n\n");
  const secs = helpSections(S.me.role, lang()).filter((s) => S.ai || s.id !== "ai").map((s) => (S.ai ? s : { ...s, body: noAI(s.body) }));
  v.innerHTML = pageHead("", "", "", `<span class="badge acc">${t("help_for")} ${t("role_" + S.me.role)}</span>`) +
    `<div class="help"><nav class="toc">${secs.map((s, i) => `<a href="#" data-to="${s.id}" class="rv" style="--i:${i}"><span class="mono">${pad(i + 1)}</span>${icon(s.icon)}<span>${esc(s.title)}</span></a>`).join("")}</nav>
    <div class="hsecs">${secs.map((s, i) => `<section class="card hsec rv" id="h-${s.id}" style="--i:${i}"><div class="hh"><span class="mono muted">${pad(i + 1)}</span><h2>${esc(s.title)}</h2>${icon(s.icon)}</div><div class="md">${md(s.body).replace(/:([a-z0-9]+):/g, (m, n) => (hasIcon(n) ? icon(n, "inl") : m))}</div></section>`).join("")}</div></div>`;
  $$("[data-to]", v).forEach((a) => (a.onclick = (e) => { e.preventDefault(); $("#h-" + a.dataset.to).scrollIntoView({ behavior: "smooth", block: "start" }); }));
}

/* ═════════ install as app (PWA) ═════════ */
const pwa = { prompt: null };
const isStandalone = () => matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
const isIOS = () => /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
const isMobile = () => matchMedia("(max-width: 760px), (pointer: coarse)").matches;
if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {});
  navigator.serviceWorker.addEventListener("message", (e) => { if (e.data?.type === "open" && typeof e.data.url === "string" && e.data.url.startsWith(location.origin)) location.href = e.data.url; });
}
window.addEventListener("beforeinstallprompt", (e) => { e.preventDefault(); pwa.prompt = e; showInstallBanner(); $$("[data-install]").forEach((b) => b.classList.remove("hidden")); });
window.addEventListener("appinstalled", () => { pwa.prompt = null; $(".install-banner")?.remove(); toast(t("installed")); });

async function installApp() {
  if (pwa.prompt) {
    pwa.prompt.prompt();
    const r = await pwa.prompt.userChoice.catch(() => null);
    if (r?.outcome === "accepted") { pwa.prompt = null; $(".install-banner")?.remove(); }
    return;
  }
  const ios = isIOS();
  const steps = ios
    ? [["iosshare", t("ios_step1")], ["plus2", t("ios_step2")], ["check", t("ios_step3")]]
    : [["menu", t("other_step1")], ["plus2", t("other_step2")], ["check", t("ios_step3")]];
  sheet({
    title: t("install_app"),
    body: `<div style="margin-bottom:16px">${downloadButtons()}</div><p class="muted">${ios ? t("ios_intro") : t("other_intro")}</p>
      <ol class="steps">${steps.map(([ic, s], i) => `<li><span class="sn mono">${i + 1}</span>${icon(ic)}<span>${s}</span></li>`).join("")}</ol>
      ${ios && !/Safari/.test(navigator.userAgent) ? `<p class="note">${t("ios_safari")}</p>` : ""}`,
  });
  bindDownloads($("#sheet-root"));
}

/* app downloads: Android APK (GitHub Actions build) + iOS Web Clip profile */
let dlInfo = null;
function downloadButtons() {
  return `<div class="dl-btns">
    <a class="dl-btn" data-dl="android" href="/download/hiveloop.apk" download>${brand("android")}<span><small>${t("dl_android_s")}</small><b>Android</b></span></a>
    <button type="button" class="dl-btn" data-dl="ios">${brand("apple")}<span><small>${t("dl_ios_s")}</small><b>iPhone · iPad</b></span></button>
  </div>`;
}
async function bindDownloads(root) {
  const and = $("[data-dl=android]", root), ios = $("[data-dl=ios]", root);
  if (!and) return;
  ios.onclick = iosSheet;
  if (!dlInfo) { try { dlInfo = await (await fetch("/api/downloads")).json(); } catch { dlInfo = {}; } }
  if (!dlInfo.android) {
    and.classList.add("off"); and.removeAttribute("href");
    $("small", and).textContent = t("dl_soon");
    and.onclick = (e) => { e.preventDefault(); toast(t("dl_soon_long")); };
  } else {
    and.onclick = () => setTimeout(() => sheet({ title: "Android", body: `<ol class="steps">${[t("apk_step1"), t("apk_step2"), t("apk_step3")].map((s, i) => `<li><span class="sn mono">${i + 1}</span><span>${s}</span></li>`).join("")}</ol>` }), 400);
  }
}
function iosSheet() {
  const s = sheet({
    title: "iPhone · iPad",
    body: `<p class="muted" style="margin-top:0">${t("ios_profile_intro")}</p>
      <a class="btn primary" href="/download/hiveloop.mobileconfig" style="width:100%;justify-content:center;height:44px;margin-bottom:14px">${brand("apple")}${t("ios_profile_btn")}</a>
      <ol class="steps">${[t("ios_p1"), t("ios_p2"), t("ios_p3")].map((x, i) => `<li><span class="sn mono">${i + 1}</span><span>${x}</span></li>`).join("")}</ol>
      <p class="note" style="margin-top:14px">${t("ios_profile_note")}</p>
      <details class="alt"><summary>${t("ios_alt")}</summary><ol class="steps" style="margin-top:10px">${[t("ios_step1"), t("ios_step2"), t("ios_step3")].map((x, i) => `<li><span class="sn mono">${i + 1}</span><span>${x}</span></li>`).join("")}</ol></details>`,
  });
  return s;
}

function showInstallBanner() {
  if (!S.me || isStandalone() || !isMobile() || $(".install-banner")) return;
  const until = +store.get("installDismissed", 0);
  if (Date.now() < until) return;
  if (!pwa.prompt && !isIOS()) return;
  const el = h(`<div class="install-banner"><img src="/icon-192.png" alt=""><div><b>${t("install_banner")}</b><small>${t("install_sub")}</small></div>
    <button class="btn sm primary" data-i>${t("install_btn")}</button><button class="ibtn sm ghost" data-x aria-label="${t("close")}">${icon("x")}</button></div>`);
  $("[data-i]", el).onclick = () => installApp();
  $("[data-x]", el).onclick = () => { store.set("installDismissed", Date.now() + 14 * 864e5); el.remove(); };
  document.body.append(el);
}

/* ═════════ auto-update: reload when a new interface version is deployed ═════════ */
const upd = { v: null, wait: null, timer: null };
const busy = () => !!($(".hl-ed-shell") || $(".sheet") || [...document.querySelectorAll("textarea, input")].some((i) => i.value && i.offsetParent && document.activeElement === i));
async function checkVersion() {
  let v;
  try { v = (await (await fetch("/api/version", { cache: "no-store" })).json()).version; } catch { return; }
  if (!v) return;
  if (!upd.v) { upd.v = v; return; }
  if (v === upd.v) return;
  // new version: reload silently as soon as the person isn't writing or editing
  if (!busy()) { location.reload(); return; }
  if (!upd.wait) upd.wait = setInterval(() => { if (!busy()) location.reload(); }, 5000);
}
function startUpdateChecks() {
  checkVersion();
  clearInterval(upd.timer);
  upd.timer = setInterval(checkVersion, 5 * 60 * 1000);
  document.addEventListener("visibilitychange", () => { if (document.visibilityState === "visible") checkVersion(); });
  window.addEventListener("focus", checkVersion);
}

/* ═════════ boot ═════════ */
/* ═════════ notifications (Web Push) ═════════ */
let pushOn = false;
const isAndroidApp = () => /HiveloopAndroid/.test(navigator.userAgent);
const pushSupported = () => "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
const b64u = (s) => Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - (s.length % 4)) % 4)), (c) => c.charCodeAt(0));
async function pushSub() {
  const reg = await navigator.serviceWorker.ready;
  return { reg, sub: await reg.pushManager.getSubscription() };
}
async function pushState() {
  if (!pushSupported()) return "unsupported";
  if (Notification.permission === "denied") return "denied";
  const { sub } = await pushSub();
  return sub && Notification.permission === "granted" ? "on" : "off";
}
async function pushEnable() {
  if (isAndroidApp()) { toast(t("notif_android")); return false; }
  if (!pushSupported()) { toast(t(isIOS() && !isStandalone() ? "notif_ios" : "notif_unsupported"), true); return false; }
  const perm = await Notification.requestPermission();
  if (perm !== "granted") { if (perm === "denied") toast(t("notif_denied"), true); return false; }
  try {
    let { reg, sub } = await pushSub();
    if (!sub) {
      const { key } = await api("GET", "/api/push/key");
      sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: b64u(key) });
    }
    await api("POST", "/api/push/subscribe", sub.toJSON());
    pushOn = true;
    return true;
  } catch (e) { fail(e); return false; }
}
async function pushDisable() {
  pushOn = false;
  if (!pushSupported()) return;
  const { sub } = await pushSub();
  if (!sub) return;
  await api("POST", "/api/push/unsubscribe", { endpoint: sub.endpoint }).catch(() => {});
  await sub.unsubscribe();
}
// pushSync: tell the server again which device this is (the address can change, or the account)
async function pushSync() {
  try {
    if (!pushSupported() || Notification.permission !== "granted") return;
    const { sub } = await pushSub();
    if (sub) { await api("POST", "/api/push/subscribe", sub.toJSON()); pushOn = true; }
  } catch {}
}

async function boot() {
  if (!upd.timer) startUpdateChecks();
  setLang(store.get("lang", (navigator.language || "").toLowerCase().startsWith("it") ? "it" : "en"));
  try { await loadMe(); } catch { return renderLogin(); }
  store.set("lang", S.me.lang);
  await loadGroups().catch(() => {});
  renderShell();
  route();
  startLive();
  refreshBadges();
  pendingBadge();
  pushSync();
  setTimeout(showInstallBanner, 1500);
}
function pendingBadge() {
  const b = $('[data-nb="admin"]');
  if (b) { b.textContent = S.pending || ""; b.classList.toggle("on", S.pending > 0); b.classList.add("at"); }
  const m = $('[data-nb="more"]');
  if (m) { m.textContent = S.pending || ""; m.classList.toggle("on", S.pending > 0); m.classList.add("at"); }
}
window.addEventListener("hashchange", route);
boot();
