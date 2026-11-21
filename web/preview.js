// Document previews that work everywhere (also Android, where browsers can't show a PDF
// inside a page): PDF via PDF.js, images, video/audio, Markdown, CSV table, JSON, text.
import { t } from "./i18n.js";
import { icon } from "./icons.js";

const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const ext = (d) => (d.name.split(".").pop() || "").toLowerCase();
export const kindOf = (d) => {
  const m = d.mime || "", e = ext(d);
  if (["doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods", "odp", "zip", "rar", "7z", "exe", "apk"].includes(e) || /officedocument|msword|ms-excel|ms-powerpoint|opendocument|zip/.test(m)) return "none";
  if (m === "application/pdf" || e === "pdf") return "pdf";
  if (m.startsWith("image/") && m !== "image/svg+xml") return "image";
  if (m.startsWith("video/") || ["mp4", "webm", "mov", "m4v"].includes(e)) return "video";
  if (m.startsWith("audio/") || ["mp3", "m4a", "wav", "ogg", "oga", "aac", "flac"].includes(e)) return "audio";
  if (m.includes("markdown") || e === "md" || e === "markdown") return "md";
  if (m.includes("csv") || e === "csv") return "csv";
  if (m.includes("json") || e === "json") return "json";
  return "text";
};

/* ---------- PDF.js ---------- */
let pdfjs = null;
async function loadPdfjs() {
  if (!pdfjs) {
    pdfjs = await import("/vendor/pdfjs/pdf.min.mjs");
    pdfjs.GlobalWorkerOptions.workerSrc = "/vendor/pdfjs/pdf.worker.min.mjs";
  }
  return pdfjs;
}

async function renderPdf(box, url) {
  const lib = await loadPdfjs();
  const doc = await lib.getDocument({ url, isEvalSupported: false, withCredentials: true }).promise;
  const n = doc.numPages;
  box.innerHTML = `<div class="pdf-info">${icon("filetext")}<span>${n} ${n === 1 ? t("page") : t("pages")}</span></div><div class="pdf-pages"></div>`;
  const pages = box.querySelector(".pdf-pages");
  const first = await doc.getPage(1);
  const vp1 = first.getViewport({ scale: 1 });
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  const draw = async (wrap, num) => {
    if (wrap.dataset.done) return;
    wrap.dataset.done = "1";
    const page = num === 1 ? first : await doc.getPage(num);
    const vp = page.getViewport({ scale: 1 });
    const scale = (pages.clientWidth || 600) / vp.width;
    const view = page.getViewport({ scale: scale * dpr });
    const c = document.createElement("canvas");
    c.width = Math.floor(view.width); c.height = Math.floor(view.height);
    c.style.aspectRatio = `${vp.width} / ${vp.height}`;
    wrap.replaceChildren(c);
    await page.render({ canvasContext: c.getContext("2d"), viewport: view }).promise;
  };
  const io = new IntersectionObserver((entries) => {
    for (const en of entries) if (en.isIntersecting) { io.unobserve(en.target); draw(en.target, +en.target.dataset.n).catch(() => {}); }
  }, { root: box.closest(".sb") || null, rootMargin: "600px 0px" });
  for (let i = 1; i <= Math.min(n, 300); i++) {
    const w = document.createElement("div");
    w.className = "pdf-page"; w.dataset.n = i;
    w.style.aspectRatio = `${vp1.width} / ${vp1.height}`;
    w.innerHTML = `<span class="pdf-num">${i}</span>`;
    pages.append(w);
    io.observe(w);
  }
  if (n > 300) pages.insertAdjacentHTML("beforeend", `<p class="muted">${t("pdf_more")}</p>`);
}

/* ---------- CSV ---------- */
function parseCSV(text) {
  const firstLine = text.split("\n", 1)[0];
  const sep = (firstLine.match(/;/g) || []).length > (firstLine.match(/,/g) || []).length ? ";" : firstLine.includes("\t") ? "\t" : ",";
  const rows = []; let row = [], cell = "", q = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (q) {
      if (ch === '"' && text[i + 1] === '"') { cell += '"'; i++; }
      else if (ch === '"') q = false;
      else cell += ch;
    } else if (ch === '"') q = true;
    else if (ch === sep) { row.push(cell); cell = ""; }
    else if (ch === "\n" || ch === "\r") { if (ch === "\r" && text[i + 1] === "\n") i++; row.push(cell); rows.push(row); row = []; cell = ""; }
    else cell += ch;
    if (rows.length > 2000) break;
  }
  if (cell || row.length) { row.push(cell); rows.push(row); }
  return rows.filter((r) => r.some((c) => c.trim()));
}

/**
 * Fills `box` with a preview of document `d`.
 * deps: { api, md } from the app. Returns { canEdit } when known.
 */
export async function renderPreview(box, d, { api, md }) {
  const url = `/api/docs/${d.id}?inline=1`;
  const kind = kindOf(d);
  const unsupported = () => {
    box.innerHTML = `<div class="empty prev-none">${icon("file")}<div>${t("unsupported_format")}</div>
      <a class="btn primary" href="/api/docs/${d.id}" download>${icon("download")}${t("download")}</a></div>`;
  };
  if (kind === "none") { unsupported(); return {}; }
  try {
    if (kind === "pdf") { await renderPdf(box, url); return {}; }
    if (kind === "image") { box.innerHTML = `<img class="preview-img" src="${url}" alt="${esc(d.name)}">`; return {}; }
    if (kind === "video") { box.innerHTML = `<video class="preview-media" src="${url}" controls playsinline preload="metadata"></video>`; return {}; }
    if (kind === "audio") { box.innerHTML = `<div class="preview-audio">${icon("file")}<audio src="${url}" controls preload="metadata"></audio></div>`; return {}; }
    const r = await api("GET", `/api/docs/${d.id}/text`);
    if (kind === "md") box.innerHTML = `<div class="md doc-read">${md(r.text)}</div>`;
    else if (kind === "csv") {
      const rows = parseCSV(r.text);
      box.innerHTML = rows.length ? `<div class="csv-wrap"><table class="csv"><thead><tr>${rows[0].map((c) => `<th>${esc(c)}</th>`).join("")}</tr></thead>
        <tbody>${rows.slice(1).map((rw) => `<tr>${rw.map((c) => `<td>${esc(c)}</td>`).join("")}</tr>`).join("")}</tbody></table></div>` : `<pre class="preview-text"></pre>`;
    } else if (kind === "json") {
      let txt = r.text; try { txt = JSON.stringify(JSON.parse(r.text), null, 2); } catch {}
      box.innerHTML = `<pre class="preview-text">${esc(txt)}</pre>`;
    } else box.innerHTML = `<pre class="preview-text">${esc(r.text)}</pre>`;
    return { canEdit: r.can_edit };
  } catch (e) {
    unsupported();
    return {};
  }
}
