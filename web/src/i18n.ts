// Minimal gettext-style i18n: the English text is the key. To add a language,
// add a dictionary below; untranslated strings fall back to English.
type Dict = Record<string, string>;

const dictionaries: Record<string, Dict> = {
  en: {},
};

let lang = "en";
try {
  const saved = localStorage.getItem("kubedojo.lang");
  if (saved && dictionaries[saved]) lang = saved;
} catch { /* storage unavailable */ }

export function t(text: string, vars?: Record<string, string | number>): string {
  let out = dictionaries[lang]?.[text] ?? text;
  if (vars) for (const [k, v] of Object.entries(vars)) out = out.replaceAll(`{${k}}`, String(v));
  return out;
}

/**
 * A count with its unit: `one` for exactly 1, `other` for every other number.
 * Both texts get the count as {n}.
 */
export function plural(n: number, one: string, other: string, vars?: Record<string, string | number>): string {
  return t(n === 1 ? one : other, { n, ...vars });
}

export const languages = Object.keys(dictionaries);
