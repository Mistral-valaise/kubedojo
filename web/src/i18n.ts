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

export const languages = Object.keys(dictionaries);
