import type { ReactNode } from "react";
import { BookOpen, ChevronLeft, ChevronRight } from "lucide-react";
import type { Link as DocsLink } from "../api";
import { t } from "../i18n";
import { Brand, Button, ThemeToggle } from "./ui";

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
  /** Right, before the theme toggle. */
  actions?: ReactNode;
  /** Right, after the theme toggle: the one action that ends the view. */
  end?: ReactNode;
}

const given = (node: ReactNode) => node !== undefined && node !== null && node !== false;

/**
 * The header of a full-height workspace view: practice of a task or a choice
 * question, the waiting screens of an exam and the exam itself.
 *
 * Left to right: brand, `back`, `title`, children, `center`, `nav`, `docs`,
 * `actions`, theme toggle, `end`. Every slot is optional. With a `back`, a
 * `title` or children the brand is followed by a slash and hidden on a phone,
 * where the header has room for the way back and the controls only.
 *
 * The right-hand controls are one group pushed to the right. With a `center`
 * that element pushes right instead and the controls are items of the header
 * row after it; the documentation link then shows an icon that stays on a phone.
 */
export function WorkspaceHeader({ back, title, children, center, nav, docs, actions, end }: WorkspaceHeaderProps) {
  const named = given(back) || given(title) || given(children);
  const row = given(center);
  const alone = !given(nav) && !docs && !given(actions) && !given(end);
  const controls = (
    <>
      {nav}
      {docs && (row ? (
        <a href={docs.url} target="_blank" rel="noreferrer" title={docs.title} aria-label={t("Docs")} className="inline-flex shrink-0 items-center gap-1.5 px-1 text-sm text-muted hover:text-text">
          <BookOpen className="size-3.5" aria-hidden /><span className="hidden sm:inline">{t("Docs")}</span>
        </a>
      ) : (
        <a href={docs.url} target="_blank" rel="noreferrer" title={docs.title} className="hidden px-2 text-sm text-muted hover:text-text sm:block">{t("Docs")}</a>
      ))}
      {actions}
      <ThemeToggle />
      {end}
    </>
  );
  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b bg-bg px-4">
      {named ? <span className="hidden items-center gap-3 sm:flex"><Brand /><span className="text-line">/</span></span> : <Brand />}
      {back}
      {title}
      {children}
      {center}
      {row ? controls : <div className={alone ? "ml-auto" : "ml-auto flex shrink-0 items-center gap-1"}>{controls}</div>}
    </header>
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
      <Button size="sm" variant="ghost" disabled={!onPrevious} onClick={onPrevious} aria-label={previousLabel}><ChevronLeft className="size-4" /></Button>
      <span className="px-1 text-xs whitespace-nowrap tabular-nums text-muted">{position}</span>
      <Button size="sm" variant="ghost" disabled={!onNext} onClick={onNext} aria-label={nextLabel}><ChevronRight className="size-4" /></Button>
    </>
  );
}
