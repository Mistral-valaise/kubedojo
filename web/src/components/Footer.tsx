import { ExternalLink } from "lucide-react";
import { useConfig } from "../config";
import { t } from "../i18n";
import { project, versionLabel } from "../project";
import { cx } from "./ui/cx";

/**
 * The footer of every view: the version, the source and the author.
 *
 * A Page has the full footer. The workspace views (practice, an exam and its
 * waiting screens) have the `compact` one, a single line of small text as
 * the last row of the window, so that the terminal gives up as little height
 * as it can. On a phone it leaves out the source, so that the line fits.
 */
export function Footer({ compact = false }: { compact?: boolean }) {
  const config = useConfig();
  const source = (
    <a href={project.repository} target="_blank" rel="noopener noreferrer"
      className={cx(compact ? "max-sm:hidden inline-flex h-full items-center" : "py-2.5", "hover:text-text")}>
      {t("Open source on GitHub")}
    </a>
  );
  const author = (
    <a href={project.author.url} target="_blank" rel="noopener noreferrer"
      className={cx("ml-auto inline-flex items-center gap-1.5 font-medium text-text hover:text-accent", compact ? "h-full" : "py-2.5")}>
      {t("Created by {name}", { name: project.author.name })}<ExternalLink className={compact ? "size-3" : "size-3.5"} aria-hidden />
    </a>
  );
  if (compact) {
    return (
      <footer className="flex h-8 shrink-0 items-center gap-x-5 border-t bg-panel px-4 text-xs text-muted">
        <span className="whitespace-nowrap">{versionLabel(config?.version)}</span>
        {source}
        {author}
      </footer>
    );
  }
  return (
    <footer className="border-t">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 px-5 py-3 text-sm text-muted sm:px-6">
        <span>{versionLabel(config?.version)}</span>
        {source}
        {author}
      </div>
    </footer>
  );
}
