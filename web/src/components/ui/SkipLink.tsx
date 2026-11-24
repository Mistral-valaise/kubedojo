import { t } from "../../i18n";

/**
 * The id of the main landmark of every view; the skip link jumps to it.
 *
 * A Page makes its `<main>` focusable by script (`tabIndex={-1}`), so the jump
 * also moves the focus. The workspace views do not: there a click on the text
 * of a question must leave the focus on the page, where the number keys and
 * Enter answer it. The browser still continues the Tab order from the target.
 */
export const MAIN_ID = "main";

/** The first stop of the Tab key on every view: jumps over the header to the content. Visible only while focused. */
export function SkipLink() {
  return (
    <a href={`#${MAIN_ID}`}
      className="sr-only rounded-lg bg-accent px-4 py-2.5 text-sm font-semibold text-on-accent shadow-card focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50">
      {t("Skip to main content")}
    </a>
  );
}
