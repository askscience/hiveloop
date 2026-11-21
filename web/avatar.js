// Cartoon avatars (Avataaars by Pablo Stanley, via the bundled DiceBear library in
// /vendor/avataaars.mjs). Drawn in the browser; the server only keeps the user's choices.
import { t } from "./i18n.js";

let lib = null, loading = null;
export const avatarLibReady = () => !!lib;
export function loadAvatarLib() {
  return (loading ||= import("/vendor/avataaars.mjs").then((m) => (lib = m)).catch(() => { loading = null; }));
}

// What the editor offers (a curated, realistic subset of the library's options).
export const PARTS = [
  { key: "top", none: true, values: ["shortFlat", "shortRound", "shortWaved", "shortCurly", "theCaesar", "theCaesarAndSidePart", "sides", "shavedSides", "frizzle", "shaggy", "shaggyMullet", "dreads01", "dreads02",
    "bob", "bun", "curly", "curvy", "dreads", "frida", "fro", "froBand", "longButNotTooLong", "miaWallace", "straight01", "straight02", "straightAndStrand", "bigHair", "hat", "winterHat1", "winterHat02", "winterHat03", "winterHat04", "hijab", "turban"] },
  { key: "hairColor", color: true, real: 10, values: ["2c1b18", "4a312c", "724133", "a55728", "b58143", "d6b370", "c93305", "ecdcbf", "e8e1e1", "f59797", "ff2d55", "ff6b00", "ffcc00", "b6ff00", "00e676", "00bfa5", "00b0ff", "2962ff", "7c4dff", "d500f9", "ff4081", "ffffff", "9e9e9e", "000000"] },
  { key: "eyes", values: ["default", "happy", "wink", "squint", "side", "surprised", "closed", "eyeRoll", "hearts", "winkWacky", "cry", "xDizzy"] },
  { key: "eyebrows", values: ["defaultNatural", "default", "flatNatural", "raisedExcitedNatural", "raisedExcited", "upDownNatural", "upDown", "sadConcernedNatural", "sadConcerned", "frownNatural", "angryNatural", "angry", "unibrowNatural"] },
  { key: "mouth", values: ["smile", "default", "twinkle", "serious", "tongue", "eating", "disbelief", "concerned", "sad", "grimace", "screamOpen"] },
  { key: "facialHair", none: true, values: ["beardLight", "beardMedium", "beardMajestic", "moustacheFancy", "moustacheMagnum"] },
  { key: "facialHairColor", color: true, real: 9, values: ["2c1b18", "4a312c", "724133", "a55728", "b58143", "d6b370", "c93305", "ecdcbf", "e8e1e1", "ff2d55", "ff6b00", "ffcc00", "b6ff00", "00e676", "00bfa5", "00b0ff", "2962ff", "7c4dff", "d500f9", "ff4081", "ffffff", "9e9e9e", "000000"], when: (o) => o.facialHair && o.facialHair !== "none" },
  { key: "accessories", none: true, values: ["prescription01", "prescription02", "round", "kurt", "wayfarers", "sunglasses", "eyepatch"] },
  { key: "accessoriesColor", color: true, real: 10, values: ["262e33", "3c4f5c", "929598", "e6e6e6", "65c9ff", "5199e4", "ff5c5c", "ff488e", "a7ffc4", "ffffb1", "ff2d55", "ff6b00", "ffcc00", "b6ff00", "00e676", "00bfa5", "00b0ff", "2962ff", "7c4dff", "d500f9", "ff4081", "ffffff", "9e9e9e", "000000"], when: (o) => o.accessories && o.accessories !== "none" },
  { key: "clothing", values: ["shirtCrewNeck", "shirtScoopNeck", "shirtVNeck", "collarAndSweater", "hoodie", "blazerAndShirt", "blazerAndSweater", "overall", "graphicShirt"] },
  { key: "clothingGraphic", values: ["bear", "deer", "bat", "pizza", "diamond", "hola", "cumbia", "resist", "skull", "skullOutline"], when: (o) => o.clothing === "graphicShirt" },
  { key: "clothesColor", color: true, real: 14, values: ["262e33", "3c4f5c", "25557c", "5199e4", "65c9ff", "b1e2ff", "a7ffc4", "ffffb1", "ffafb9", "ff488e", "ff5c5c", "929598", "e6e6e6", "ffffff", "ff2d55", "ff6b00", "ffcc00", "b6ff00", "00e676", "00bfa5", "00b0ff", "2962ff", "7c4dff", "d500f9", "ff4081", "9e9e9e", "000000"] },
  { key: "hatColor", color: true, real: 10, values: ["262e33", "3c4f5c", "25557c", "5199e4", "65c9ff", "a7ffc4", "ffafb9", "ff5c5c", "929598", "e6e6e6", "ff2d55", "ff6b00", "ffcc00", "b6ff00", "00e676", "00bfa5", "00b0ff", "2962ff", "7c4dff", "d500f9", "ff4081", "ffffff", "9e9e9e", "000000"], when: (o) => /hat|hijab|turban/i.test(o.top || "") },
  { key: "skinColor", color: true, real: 5, values: ["ffdbb4", "edb98a", "d08b5b", "ae5d29", "614335", "fd9841", "f8d25c", "8bc34a", "4dd0e1", "64b5f6", "b39ddb", "f48fb1", "e0e0e0", "9e9e9e", "5d4037"] },
  { key: "backgroundColor", color: true, real: 7, values: ["b6e3f4", "c0aede", "d1d4f9", "ffd5dc", "c1f4c5", "fff3b0", "e6e6e6", "ff2d55", "ff6b00", "ffcc00", "b6ff00", "00e676", "00bfa5", "00b0ff", "2962ff", "7c4dff", "d500f9", "ff4081", "ffffff", "9e9e9e", "000000", "ffe0b2", "dcedc8", "263238"] },
];

const pick = (a) => a[Math.floor(Math.random() * a.length)];
export function randomAvatar() {
  const o = {};
  for (const p of PARTS) o[p.key] = pick(p.real ? p.values.slice(0, p.real) : p.values);
  o.facialHair = Math.random() < 0.25 ? pick(PARTS.find((p) => p.key === "facialHair").values) : "none";
  o.accessories = Math.random() < 0.25 ? pick(["prescription01", "prescription02", "round", "kurt", "wayfarers"]) : "none";
  if (/hat|hijab|turban/i.test(o.top) && Math.random() < 0.7) o.top = pick(PARTS[0].values.slice(0, 27));
  if (o.clothing === "graphicShirt" && Math.random() < 0.5) o.clothing = "hoodie";
  o.mouth = pick(["smile", "default", "twinkle", "serious"]);
  o.eyes = pick(["default", "happy", "wink", "squint", "side"]);
  o.eyebrows = pick(["defaultNatural", "default", "flatNatural", "raisedExcitedNatural", "upDownNatural"]);
  return o;
}

function libOptions(o) {
  const x = { seed: "hiveloop", style: ["circle"], nose: ["default"], radius: 0 };
  for (const p of PARTS) if (o[p.key] && o[p.key] !== "none") x[p.key] = [o[p.key]];
  x.topProbability = o.top === "none" ? 0 : 100;
  x.facialHairProbability = o.facialHair && o.facialHair !== "none" ? 100 : 0;
  x.accessoriesProbability = o.accessories && o.accessories !== "none" ? 100 : 0;
  if (!x.backgroundColor) x.backgroundColor = ["b6e3f4"];
  return x;
}

const cache = new Map();
export function avatarURI(o) {
  if (!lib || !o) return "";
  const k = JSON.stringify(o);
  if (!cache.has(k)) {
    if (cache.size > 400) cache.clear();
    cache.set(k, lib.createAvatar(lib.avataaars, libOptions(o)).toDataUri());
  }
  return cache.get(k);
}

/* ---------- editor (full page) ---------- */
export async function avatarEditor(current, { sheet, icon, onSave }) {
  await loadAvatarLib();
  let o = { ...(current && Object.keys(current).length ? current : randomAvatar()) };
  const body = document.createElement("div");
  body.className = "av-ed";
  const s = sheet({
    title: t("avatar"), body, wide: true,
    foot: `<button class="btn" data-rand>${icon("grid")}${t("avatar_random")}</button><button class="btn primary" data-save>${icon("check")}${t("save")}</button>`,
  });
  const render = () => {
    const keepY = s.el.querySelector(".sb")?.scrollTop;
    body.innerHTML = `<div class="av-big"><img alt="" src="${avatarURI(o)}"></div>` + PARTS.filter((p) => !p.when || p.when(o)).map((p) => {
      const vals = p.none ? ["none", ...p.values] : p.values;
      const custom = p.color && o[p.key] && !vals.includes(o[p.key]) ? o[p.key] : "";
      return `<div class="av-row"><div class="av-l">${t("av_" + p.key)}</div><div class="av-opts ${p.color ? "colors" : ""}">${vals.map((v) => p.color
        ? `<button type="button" class="av-c ${o[p.key] === v ? "on" : ""}" data-k="${p.key}" data-v="${v}" style="--c:#${v}" aria-label="${v}"></button>`
        : `<button type="button" class="av-o ${(o[p.key] || "none") === v ? "on" : ""}" data-k="${p.key}" data-v="${v}" title="${v === "none" ? t("av_none") : v}">${v === "none" ? `<span class="av-none">${t("av_none")}</span>` : `<img alt="" loading="lazy" src="${avatarURI({ ...o, [p.key]: v })}">`}</button>`).join("")}
        ${p.color ? `<label class="av-c av-pick ${custom ? "on" : ""}" style="--c:${custom ? "#" + custom : "transparent"}" title="${t("av_custom")}" aria-label="${t("av_custom")}"><span>+</span><input type="color" data-pick="${p.key}" value="#${custom || o[p.key] || "888888"}"></label>` : ""}</div></div>`;
    }).join("");
    body.querySelectorAll("[data-k]").forEach((b) => (b.onclick = () => { o = { ...o, [b.dataset.k]: b.dataset.v }; render(); }));
    body.querySelectorAll("[data-pick]").forEach((inp) => (inp.onchange = () => { o = { ...o, [inp.dataset.pick]: inp.value.replace("#", "").toLowerCase() }; render(); }));
    if (keepY != null) s.el.querySelector(".sb").scrollTop = keepY;
  };
  render();
  s.el.querySelector("[data-rand]").onclick = () => { o = randomAvatar(); render(); };
  s.el.querySelector("[data-save]").onclick = async () => { if (await onSave(o)) s.close(); };
}
