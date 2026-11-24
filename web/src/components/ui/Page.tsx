import type { ReactNode } from "react";
import { Link } from "react-router";
import type { Link as DocsLink } from "../../api";
import { t } from "../../i18n";
import { Brand } from "./Logo";
import { ThemeToggle } from "./ThemeToggle";

/**
 * Standard page chrome for non-workspace pages. A page about one track passes
 * that track's documentation (null when it has none to link); pages about no
 * track link the Kubernetes documentation.
 */
export function Page({ children, crumbs, docs = { title: t("Kubernetes docs"), url: "https://kubernetes.io/docs/" } }: {
  children: ReactNode; crumbs?: { label: string; to?: string }[]; docs?: DocsLink | null;
}) {
  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-20 border-b bg-bg/85 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-5">
          <Brand />
          {crumbs?.map((c, i) => (
            <span key={i} className="flex min-w-0 items-center gap-3 text-sm text-muted">
              <span className="text-line">/</span>
              {c.to ? <Link to={c.to} className="truncate hover:text-text">{c.label}</Link> : <span className="truncate text-text">{c.label}</span>}
            </span>
          ))}
          <div className="ml-auto flex items-center gap-1">
            {docs && (
              <a href={docs.url} target="_blank" rel="noreferrer" className="hidden px-2 text-sm text-muted hover:text-text sm:block">
                {docs.title}
              </a>
            )}
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-5 py-8">{children}</main>
    </div>
  );
}
