// Markdown <-> Editor.js blocks. Users edit visual blocks; files stay plain Markdown.

/* ---------- Markdown -> blocks ---------- */
const clean = (html) =>
  DOMPurify.sanitize(html, { ALLOWED_TAGS: ["b", "strong", "i", "em", "a", "code", "br", "s", "del", "u"], ALLOWED_ATTR: ["href"] })
    .replace(/<(\/?)strong>/g, "<$1b>").replace(/<(\/?)em>/g, "<$1i>").replace(/<(\/?)del>/g, "<$1s>").trim();
const inline = (md) => clean(marked.parseInline(md || "", { gfm: true }));

function listItems(items) {
  return items.map((it) => {
    const nested = (it.tokens || []).filter((t) => t.type === "list");
    const textTok = (it.tokens || []).filter((t) => t.type === "text" || t.type === "paragraph");
    const text = textTok.map((t) => t.text).join("\n") || (it.text || "").split("\n")[0];
    return {
      content: inline(text),
      meta: it.task ? { checked: !!it.checked } : {},
      items: nested.length ? listItems(nested.flatMap((n) => n.items)) : [],
    };
  });
}

export function mdToBlocks(md) {
  const blocks = [];
  for (const tk of marked.lexer(md || "", { gfm: true })) {
    switch (tk.type) {
      case "heading":
        blocks.push({ type: "header", data: { text: inline(tk.text), level: Math.min(Math.max(tk.depth, 1), 4) } });
        break;
      case "paragraph":
        blocks.push({ type: "paragraph", data: { text: inline(tk.text).replace(/\n/g, "<br>") } });
        break;
      case "list": {
        const style = tk.items.some((i) => i.task) ? "checklist" : tk.ordered ? "ordered" : "unordered";
        blocks.push({ type: "list", data: { style, meta: style === "ordered" && tk.start > 1 ? { start: tk.start } : {}, items: listItems(tk.items) } });
        break;
      }
      case "blockquote": {
        const lines = tk.text.split("\n");
        let caption = "";
        if (lines.length > 1 && /^[—–-]\s/.test(lines[lines.length - 1])) caption = inline(lines.pop().replace(/^[—–-]\s*/, ""));
        blocks.push({ type: "quote", data: { text: inline(lines.join("\n")).replace(/\n/g, "<br>"), caption, alignment: "left" } });
        break;
      }
      case "hr":
        blocks.push({ type: "delimiter", data: {} });
        break;
      case "table":
        blocks.push({ type: "table", data: { withHeadings: true, content: [tk.header.map((c) => inline(c.text)), ...tk.rows.map((r) => r.map((c) => inline(c.text)))] } });
        break;
      case "code":
        blocks.push({ type: "paragraph", data: { text: tk.text.split("\n").map((l) => `<code>${l.replace(/&/g, "&amp;").replace(/</g, "&lt;")}</code>`).join("<br>") } });
        break;
      case "space":
        break;
      default:
        if (tk.raw?.trim()) blocks.push({ type: "paragraph", data: { text: inline(tk.raw.trim()) } });
    }
  }
  return blocks.length ? blocks : [{ type: "paragraph", data: { text: "" } }];
}

/* ---------- blocks -> Markdown ---------- */
const escMd = (s) => s.replace(/\\/g, "\\\\").replace(/([*_`[\]])/g, "\\$1");

export function htmlToMd(html, { table = false } = {}) {
  const box = document.createElement("div");
  box.innerHTML = html || "";
  const walk = (n) => {
    if (n.nodeType === 3) return escMd(n.nodeValue.replace(/ /g, " "));
    if (n.nodeType !== 1) return "";
    const inner = [...n.childNodes].map(walk).join("");
    const wrap = (m) => {
      const t = inner.match(/^(\s*)([\s\S]*?)(\s*)$/);
      return t[2] ? `${t[1]}${m}${t[2]}${m}${t[3]}` : inner;
    };
    switch (n.tagName) {
      case "B": case "STRONG": return wrap("**");
      case "I": case "EM": return wrap("*");
      case "S": case "DEL": case "STRIKE": return wrap("~~");
      case "CODE": return inner ? "`" + n.textContent + "`" : "";
      case "A": return inner ? `[${inner}](${(n.getAttribute("href") || "").replace(/\)/g, "%29")})` : "";
      case "BR": return table ? "<br>" : "\n";
      default: return inner;
    }
  };
  let md = [...box.childNodes].map(walk).join("");
  if (table) md = md.replace(/\|/g, "\\|").replace(/\n/g, " ");
  return md.replace(/[ \t]+$/gm, "").trim();
}

function listToMd(items, style, depth = 0, start = 1) {
  const pad = "   ".repeat(depth);
  return items.map((it, i) => {
    const content = it.content ?? it.text ?? "";
    const bullet = style === "ordered" ? `${start + i}.` : style === "checklist" ? `- [${it.meta?.checked ? "x" : " "}]` : "-";
    const line = `${pad}${bullet} ${htmlToMd(content).replace(/\n/g, " ")}`;
    const sub = it.items?.length ? "\n" + listToMd(it.items, style, depth + 1) : "";
    return line + sub;
  }).join("\n");
}

export function blocksToMd(data) {
  const out = [];
  for (const b of data.blocks || []) {
    const d = b.data || {};
    switch (b.type) {
      case "header": out.push(`${"#".repeat(d.level || 2)} ${htmlToMd(d.text).replace(/\s*\n\s*/g, " ")}`); break;
      case "paragraph": { const t = htmlToMd(d.text); if (t) out.push(t); break; }
      case "list": if (d.items?.length) out.push(listToMd(d.items, d.style || "unordered", 0, d.meta?.start || 1)); break;
      case "quote": {
        const t = htmlToMd(d.text).split("\n").map((l) => `> ${l}`).join("\n");
        out.push(d.caption ? `${t}\n> — ${htmlToMd(d.caption)}` : t);
        break;
      }
      case "delimiter": out.push("---"); break;
      case "table": {
        const rows = (d.content || []).map((r) => r.map((c) => htmlToMd(c, { table: true }) || " "));
        if (!rows.length) break;
        const cols = Math.max(...rows.map((r) => r.length));
        const norm = rows.map((r) => [...r, ...Array(cols - r.length).fill(" ")]);
        const head = d.withHeadings ? norm.shift() : Array(cols).fill(" ");
        out.push([`| ${head.join(" | ")} |`, `|${" --- |".repeat(cols)}`, ...norm.map((r) => `| ${r.join(" | ")} |`)].join("\n"));
        break;
      }
    }
  }
  return out.join("\n\n") + "\n";
}
