import type { ReactNode } from "react";
import { Link } from "react-router";
import { Settings } from "lucide-react";
import type { Link as DocsLink } from "../../api";
import { t } from "../../i18n";
import { Footer } from "../Footer";
import { Brand } from "./Logo";
import { MAIN_ID, SkipLink } from "./SkipLink";
import { ThemeToggle } from "./ThemeToggle";

/**
 * Standard page chrome for non-workspace pages: header, the content as the
 * main landmark, and the footer, which sits at the bottom of the window on a
 * short page. A page about one track passes that track's documentation (null
 * when it has none to link); pages about no track link the Kubernetes
 * documentation.
 */
export function Page({ children, crumbs, docs = { title: t("Kubernetes docs"), url: "https://kubernetes.io/docs/" } }: {
  children: ReactNode; crumbs?: { label: string; to?: string }[]; docs?: DocsLink | null;
}) {
  return (
    <div className="flex min-h-full flex-col">
      <SkipLink />
      <header className="sticky top-0 z-20 border-b bg-bg/85 backdrop-blur">
        <div className="mx-auto flex h-15 max-w-6xl items-center gap-3 px-5 sm:px-6">
          <Brand />
          {crumbs && crumbs.length > 0 && (
            <nav aria-label={t("Breadcrumb")} className="flex min-w-0 items-center gap-3 text-sm text-muted">
              {crumbs.map((c, i) => {
                // The last crumb gives way on a narrow screen; the ones before it are short and the way back.
                const last = i === crumbs.length - 1;
                const fit = last ? "truncate" : "whitespace-nowrap";
                return (
                  <span key={i} className={last ? "flex min-w-0 items-center gap-3" : "flex shrink-0 items-center gap-3"}>
                    <span aria-hidden className="text-line">/</span>
                    {c.to ? <Link to={c.to} className={`${fit} hover:text-text`}>{c.label}</Link> : <span aria-current="page" className={`${fit} text-text`}>{c.label}</span>}
                  </span>
                );
              })}
            </nav>
          )}
          <div className="ml-auto flex shrink-0 items-center gap-1">
            {docs && (
              <a href={docs.url} target="_blank" rel="noopener noreferrer" className="hidden px-3 py-2.5 text-sm text-muted hover:text-text sm:block">
                {docs.title}
              </a>
            )}
            {/* On a phone the icon alone: its name is the label. */}
            <Link to="/settings" aria-label={t("Settings")} title={t("Settings")}
              className="inline-flex h-10 min-w-10 items-center justify-center gap-1.5 rounded-lg px-2.5 text-sm text-muted transition hover:bg-panel2 hover:text-text">
              <Settings className="size-4.5" aria-hidden /><span className="hidden sm:inline">{t("Settings")}</span>
            </Link>
            <ThemeToggle size="lg" />
          </div>
        </div>
      </header>
      <main id={MAIN_ID} tabIndex={-1} className="mx-auto w-full max-w-6xl flex-1 px-5 py-8 sm:px-6 sm:py-10">{children}</main>
      <Footer />
    </div>
  );
}
