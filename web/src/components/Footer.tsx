import { ExternalLink } from "lucide-react";
import { useConfig } from "../config";
import { t } from "../i18n";
import { project, versionLabel } from "../project";

/**
 * The footer of every Page: the version, the source and the author. The
 * workspace views have no footer, so that the terminal keeps its height; they
 * show the same three items in the About popover of their header.
 */
export function Footer() {
  const config = useConfig();
  return (
    <footer className="border-t">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 px-5 py-3 text-sm text-muted sm:px-6">
        <span>{versionLabel(config?.version)}</span>
        <a href={project.repository} target="_blank" rel="noopener noreferrer" className="py-2.5 hover:text-text">
          {t("Open source on GitHub")}
        </a>
        <a href={project.author.url} target="_blank" rel="noopener noreferrer"
          className="ml-auto inline-flex items-center gap-1.5 py-2.5 font-medium text-text hover:text-accent">
          {t("Created by {name}", { name: project.author.name })}<ExternalLink className="size-3.5" aria-hidden />
        </a>
      </div>
    </footer>
  );
}
