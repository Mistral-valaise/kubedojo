import { Link } from "react-router";
import { ExternalLink, Info, Settings } from "lucide-react";
import { useConfig } from "../config";
import { t } from "../i18n";
import { project, versionLabel } from "../project";
import { IconButton, Popover, cx } from "./ui";

/**
 * The About button of the workspace views: the version, the settings, the
 * source and the author, a click away in the header. The compact footer of
 * these views shows the credit too; the popover adds the way to Settings,
 * which their header has no room for otherwise. The configuration
 * is fetched only once the popover is opened. `className` is for hiding it
 * where a header has no room (`hidden sm:inline-flex`).
 */
export function About({ className }: { className?: string }) {
  return (
    <Popover label={t("About kubedojo")} className={className} panelClassName="w-64"
      trigger={(props) => (
        <IconButton {...props} aria-label={t("About kubedojo")}><Info className="size-4.5" aria-hidden /></IconButton>
      )}>
      <AboutItems />
    </Popover>
  );
}

function AboutItems() {
  const config = useConfig();
  const link = "flex min-h-9 items-center gap-1.5 rounded-lg px-2 hover:bg-panel2";
  return (
    <div className="flex flex-col">
      <p className="px-2 pt-1 pb-2 font-semibold">{versionLabel(config?.version)}</p>
      <Link to="/settings" className={cx(link, "text-muted hover:text-text")}>
        <Settings className="size-3.5" aria-hidden />{t("Settings")}
      </Link>
      <a href={project.repository} target="_blank" rel="noopener noreferrer" className={cx(link, "text-muted hover:text-text")}>
        {t("Open source on GitHub")}
      </a>
      <a href={project.author.url} target="_blank" rel="noopener noreferrer" className={cx(link, "font-medium")}>
        {t("Created by {name}", { name: project.author.name })}<ExternalLink className="size-3.5 text-muted" aria-hidden />
      </a>
    </div>
  );
}
