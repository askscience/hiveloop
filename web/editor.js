// Visual block editor for Markdown documents (Editor.js, themed to match Hiveloop).
import { t, lang } from "./i18n.js";
import { icon } from "./icons.js";
import { mdToBlocks, blocksToMd } from "./mdblocks.js";

const V = "/vendor/editorjs/";
const SCRIPTS = ["editorjs.umd.js", "header.umd.js", "editorjs-list.umd.js", "quote.umd.js", "delimiter.umd.js", "table.umd.js", "inline-code.umd.js"];
let loading = null;
function loadEditor() {
  if (window.EditorJS && window.EditorjsList) return Promise.resolve();
  if (loading) return loading;
  if (!document.querySelector('link[href="/editor.css"]')) {
    const l = document.createElement("link"); l.rel = "stylesheet"; l.href = "/editor.css"; document.head.append(l);
  }
  loading = SCRIPTS.reduce((p, f) => p.then(() => new Promise((res, rej) => {
    const s = document.createElement("script"); s.src = V + f; s.onload = res;
    s.onerror = () => { loading = null; rej(new Error("editor_load_failed")); };
    document.head.append(s);
  })), Promise.resolve());
  return loading;
}

const IT = {
  messages: {
    ui: {
      blockTunes: { toggler: { "Click to tune": "Opzioni", "or drag to move": "o trascina per spostare" } },
      inlineToolbar: { converter: { "Convert to": "Trasforma in" } },
      toolbar: { toolbox: { Add: "Aggiungi blocco" } },
      popover: { Filter: "Cerca", "Nothing found": "Nessun risultato", "Convert to": "Trasforma in" },
    },
    toolNames: {
      Text: "Testo", Heading: "Titolo", "Unordered List": "Elenco puntato", "Ordered List": "Elenco numerato", Checklist: "Lista di controllo",
      List: "Elenco", Quote: "Citazione", Delimiter: "Separatore", Table: "Tabella", Bold: "Grassetto", Italic: "Corsivo", Link: "Link", InlineCode: "Codice",
    },
    tools: {
      link: { "Add a link": "Incolla o scrivi un link" },
      header: { "Heading 1": "Titolo 1", "Heading 2": "Titolo 2", "Heading 3": "Titolo 3", "Heading 4": "Titolo 4" },
      list: { Unordered: "Puntato", Ordered: "Numerato", Checklist: "Lista di controllo", "Start with": "Inizia da", "Counter type": "Tipo di numerazione", Numeric: "Numeri", "Lower Roman": "Romani minuscoli", "Upper Roman": "Romani maiuscoli", "Lower Alpha": "Lettere minuscole", "Upper Alpha": "Lettere maiuscole" },
      table: { "With headings": "Con intestazione", "Without headings": "Senza intestazione", "Add row above": "Riga sopra", "Add row below": "Riga sotto", "Delete row": "Elimina riga", "Add column to left": "Colonna a sinistra", "Add column to right": "Colonna a destra", "Delete column": "Elimina colonna" },
      quote: { "Align Left": "Allinea a sinistra", "Align Center": "Centra" },
      stub: { "The block can not be displayed correctly.": "Questo blocco non può essere mostrato." },
      convertTo: { "Convert to": "Trasforma in" },
    },
    blockTunes: {
      delete: { Delete: "Elimina", "Click to delete": "Conferma eliminazione" },
      moveUp: { "Move up": "Sposta su" },
      moveDown: { "Move down": "Sposta giù" },
    },
  },
};

// true if the editor holds any real text (used to avoid creating empty documents)
function hasContent(data) {
  const txt = (h) => String(h || "").replace(/<[^>]*>|&nbsp;/g, "").trim();
  const items = (list) => (list || []).some((i) => txt(i.content ?? i.text) || items(i.items));
  return (data.blocks || []).some((b) => {
    const d = b.data || {};
    if (b.type === "delimiter") return true;
    if (b.type === "list") return items(d.items);
    if (b.type === "table") return (d.content || []).some((r) => r.some((c) => txt(c)));
    return txt(d.text) || txt(d.caption);
  });
}

/**
 * Opens the full-screen editor for a Markdown document.
 * @param {{id:number,name:string}} doc
 * @param {{api:Function,toast:Function,fail:Function,onSaved?:Function}} deps
 */
// the block types offered, shared by the document editor and the inline one (event programme)
const tools = () => ({
  header: { class: Header, inlineToolbar: true, shortcut: "CMD+SHIFT+H", config: { levels: [1, 2, 3, 4], defaultLevel: 2, placeholder: t("ed_heading") } },
  list: { class: EditorjsList, inlineToolbar: true, shortcut: "CMD+SHIFT+L", config: { defaultStyle: "unordered" } },
  quote: { class: Quote, inlineToolbar: true, config: { quotePlaceholder: t("ed_quote"), captionPlaceholder: t("ed_caption") } },
  table: { class: Table, inlineToolbar: true, config: { rows: 2, cols: 2, withHeadings: true } },
  delimiter: Delimiter,
  inlineCode: { class: InlineCode, shortcut: "CMD+SHIFT+C" },
});

// Inline visual editor for a Markdown field (e.g. an event's programme): same blocks as the
// document editor, value read back as Markdown.
export async function mountInlineEditor(holder, md, { placeholder = "" } = {}) {
  await loadEditor();
  const editor = new EditorJS({
    holder,
    data: { blocks: mdToBlocks(md || "") },
    minHeight: 60,
    placeholder,
    i18n: lang() === "it" ? IT : undefined,
    inlineToolbar: ["bold", "italic", "link", "inlineCode"],
    tools: tools(),
  });
  await editor.isReady;
  return {
    getMd: async () => blocksToMd(await editor.save()).trim(),
    destroy: () => { try { editor.destroy(); } catch {} },
  };
}

export async function openEditor(doc, { api, toast, fail, onSaved }) {
  const root = document.createElement("div");
  root.className = "hl-ed-shell";
  root.innerHTML = `
    <header class="ed-bar">
      <button class="ibtn ghost" data-close aria-label="${t("close")}">${icon("left")}</button>
      <div class="ed-title"><input class="ed-name" maxlength="150" aria-label="${t("name")}" spellcheck="false"><span class="mono muted ed-status" data-status>${t("loading")}</span></div>
      <button class="btn ghost hide-m" data-help>${icon("help")}<span>${t("ed_help")}</span></button>
      <button class="btn primary" data-save disabled>${icon("check")}<span>${t("save")}</span></button>
    </header>
    <div class="ed-scroll"><div class="ed-page"><div class="ed-tip hidden" data-tip>${t("ed_tip")}</div><div class="hl-ed md" data-holder></div></div></div>`;
  document.body.append(root);
  document.body.classList.add("ed-open");
  const $ = (s) => root.querySelector(s);
  const status = (txt) => ($("[data-status]").textContent = txt);
  const nameEl = $(".ed-name");
  nameEl.value = (doc.name || "").replace(/\.md$/i, "");
  nameEl.placeholder = t("ed_title_ph");

  let editor, version = 0, dirty = false, saving = false, timer = null, closed = false;
  const setDirty = (d) => { dirty = d; $("[data-save]").disabled = !d; status(d ? t("ed_unsaved") : t("ed_saved")); };

  const close = async () => {
    if (dirty && !doc.id && editor) {
      const d = await editor.save().catch(() => ({ blocks: [] }));
      if (!hasContent(d) && !nameEl.value.trim()) dirty = false;
    }
    if (dirty && !(await save())) return;
    closed = true;
    clearTimeout(timer);
    document.removeEventListener("keydown", onKey);
    window.removeEventListener("beforeunload", onUnload);
    try { editor?.destroy(); } catch {}
    root.classList.add("closing");
    document.body.classList.remove("ed-open");
    setTimeout(() => root.remove(), 220);
  };
  const onKey = (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") { e.preventDefault(); save(); }
    if (e.key === "Escape" && !root.querySelector(".ce-popover--opened, .ce-inline-toolbar--showed")) close();
  };
  const onUnload = (e) => { if (dirty) { e.preventDefault(); e.returnValue = ""; } };
  document.addEventListener("keydown", onKey);
  window.addEventListener("beforeunload", onUnload);
  $("[data-close]").onclick = close;
  $("[data-help]").onclick = () => $("[data-tip]").classList.toggle("hidden");

  async function save() {
    if (saving || !editor) return false;
    saving = true; status(t("ed_saving"));
    try {
      const data = await editor.save();
      const content = blocksToMd(data);
      let r;
      if (!doc.id) {
        // new document: created on the first save
        const d = await api("POST", "/api/docs/new", { name: nameEl.value.trim(), content });
        doc.id = d.id; doc.name = d.name; version = d.version;
        r = { version: d.version, name: d.name };
      } else {
        r = await api("PUT", `/api/docs/${doc.id}/content`, { content, version, name: nameEl.value.trim() });
      }
      version = r.version; doc.name = r.name;
      setDirty(false);
      onSaved?.(r);
      return true;
    } catch (e) {
      if (e.message === "edited_elsewhere") { status(t("edited_elsewhere")); toast(t("edited_elsewhere"), true); }
      else { status(t("ed_unsaved")); fail(e); }
      return false;
    } finally { saving = false; }
  }
  $("[data-save]").onclick = save;
  const changed = () => { if (closed) return; setDirty(true); clearTimeout(timer); timer = setTimeout(save, 2500); };
  nameEl.oninput = changed;

  try {
    const [, src] = await Promise.all([loadEditor(), doc.id ? api("GET", `/api/docs/${doc.id}/text`) : Promise.resolve({ text: "", can_edit: true, version: 0 })]);
    if (closed) return;
    if (!src.can_edit) { toast(t("forbidden"), true); return close(); }
    version = src.version;
    editor = new EditorJS({
      holder: $("[data-holder]"),
      data: { blocks: doc.id ? mdToBlocks(src.text) : [] },
      autofocus: !doc.id && !matchMedia("(pointer: coarse)").matches,
      minHeight: 120,
      placeholder: t("ed_placeholder"),
      i18n: lang() === "it" ? IT : undefined,
      inlineToolbar: ["bold", "italic", "link", "inlineCode"],
      tools: tools(),
      onChange: changed,
      onReady: () => {
        status(doc.id ? t("ed_saved") : t("ed_new"));
        if (!doc.id) nameEl.focus();
        try { if (!localStorage.getItem("hiveloop.edTip")) { $("[data-tip]").classList.remove("hidden"); localStorage.setItem("hiveloop.edTip", "1"); } } catch {}
      },
    });
  } catch (e) {
    fail(e);
    dirty = false;
    close();
  }
}
