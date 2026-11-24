import type { ReactNode } from "react";
import { BookOpen, ChevronLeft, ChevronRight } from "lucide-react";
import type { Link as DocsLink } from "../api";
import { t } from "../i18n";
import { About } from "./About";
import { Brand, IconButton, SkipLink, ThemeToggle } from "./ui";

export interface WorkspaceHeaderProps {
  /** Left, after the brand: the way back out of the workspace. */
  back?: ReactNode;
  /** Left, after `back`: what the workspace shows. */
  title?: ReactNode;
  /** Left, after `title`: anything else that belongs to the title. */
  children?: ReactNode;
  /**
   * Between the left and the right side, such as the timer of an exam. It
   * carries `ml-auto` itself, and the controls follow it as items of the row.
   */
  center?: ReactNode;
  /** Right: moving between items, see HeaderPager. */
  nav?: ReactNode;
  /** Right: the documentation to link. Nothing is rendered for null. */
  docs?: DocsLink | null;
  /** Right, before the About button and the theme toggle. */
  actions?: ReactNode;
  /**
   * Where the About button shows: everywhere, or from the `sm` breakpoint on
   * only, for a header that is full on a phone. Default "always".
   */
  about?: "always" | "sm";
  /** Right, after the theme toggle: the one action that ends the view. */
  end?: ReactNode;
}

const given = (node: ReactNode) => node !== undefined && node !== null && node !== false;

/**
 * The header of a full-height workspace view: practice of a task or a choice
 * question, the waiting screens of an exam and the exam itself.
 *
 * Left to right: brand, `back`, `title`, children, `center`, `nav`, `docs`,
 * `actions`, About, theme toggle, `end`. Every slot is optional. The About
 * button stands in for the footer these views do not have. With a `back`, a
 * `title` or children the brand is followed by a slash and hidden on a phone,
 * where the header has room for the way back and the controls only.
 *
 * The right-hand controls are one group pushed to the right. With a `center`
 * that element pushes right instead and the controls are items of the header
 * row after it; the documentation link then shows an icon that stays on a phone.
 */
export function WorkspaceHeader({ back, title, children, center, nav, docs, actions, about = "always", end }: WorkspaceHeaderProps) {
  const named = given(back) || given(title) || given(children);
  const row = given(center);
  const controls = (
    <>
      {nav}
      {docs && (row ? (
        <a href={docs.url} target="_blank" rel="noopener noreferrer" title={docs.title} aria-label={t("Docs")} className="inline-flex h-9 min-w-9 shrink-0 items-center justify-center gap-1.5 rounded-lg px-1 text-sm text-muted hover:text-text">
          <BookOpen className="size-4" aria-hidden /><span className="hidden sm:inline">{t("Docs")}</span>
        </a>
      ) : (
        <a href={docs.url} target="_blank" rel="noopener noreferrer" title={docs.title} className="hidden px-2.5 py-2 text-sm text-muted hover:text-text sm:block">{t("Docs")}</a>
      ))}
      {actions}
      <About className={about === "sm" ? "max-sm:hidden" : undefined} />
      <ThemeToggle />
      {end}
    </>
  );
  return (
    <>
      <SkipLink />
      <header className="flex h-13 shrink-0 items-center gap-3 border-b bg-panel px-4">
        {named ? <span className="hidden items-center gap-3 sm:flex"><Brand /><span aria-hidden className="text-line">/</span></span> : <Brand />}
        {back}
        {title}
        {children}
        {center}
        {row ? controls : <div className="ml-auto flex shrink-0 items-center gap-1">{controls}</div>}
      </header>
    </>
  );
}

/** Previous and next with the position between them, for the `nav` slot. A missing handler disables its button. */
export function HeaderPager({ position, previousLabel, nextLabel, onPrevious, onNext }: {
  /** Such as "2 / 7"; empty when the position is not known. */
  position: string;
  previousLabel: string; nextLabel: string;
  onPrevious?: () => void; onNext?: () => void;
}) {
  return (
    <>
      <IconButton variant="outline" disabled={!onPrevious} onClick={onPrevious} aria-label={previousLabel}><ChevronLeft className="size-4" aria-hidden /></IconButton>
      <span className="px-1.5 text-label whitespace-nowrap tabular-nums text-muted">{position}</span>
      <IconButton variant="outline" disabled={!onNext} onClick={onNext} aria-label={nextLabel}><ChevronRight className="size-4" aria-hidden /></IconButton>
    </>
  );
}
