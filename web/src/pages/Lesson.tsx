import { useParams } from "react-router";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { t } from "../i18n";
import { standing, words } from "../track";
import { Markdown } from "../components/Markdown";
import { ButtonLink, Card, Loading, Page, Skeleton } from "../components/ui";
import { LoadError, QuestionRow } from "./Track";

export function LessonPage() {
  const { track: id = "", domain = "" } = useParams();
  const track = useLoad(() => api.track(id), [id]);
  const lesson = useLoad(() => api.lesson(id, domain), [id, domain]);
  // A failed load leaves the rows without their state; the lesson itself is not affected.
  const progress = useLoad(() => api.progress(id), [id]);

  if (track.error || lesson.error) {
    return <Page docs={null}><LoadError message={track.error ?? lesson.error ?? ""} onRetry={() => { void track.reload(); void lesson.reload(); }} /></Page>;
  }
  if (!track.data || !lesson.data) return <Page docs={null}><LessonSkeleton /></Page>;
  const tr = track.data, w = words(tr);
  const qs = (tr.questions ?? []).filter((q) => q.domain === domain);
  const idx = tr.domains.findIndex((d) => d.id === domain);
  const prev = tr.domains[idx - 1], next = tr.domains[idx + 1];

  return (
    <Page docs={w.docs} crumbs={[{ label: tr.short, to: `/t/${tr.id}` }, { label: lesson.data.domain.title }]}>
      <div className="grid grid-cols-[minmax(0,1fr)] gap-8 lg:grid-cols-[minmax(0,1fr)_340px]">
        <article className="min-w-0">
          <Markdown>{lesson.data.markdown}</Markdown>
          {/* Domain titles are long: on a narrow screen the two links share the row and cut their titles off. */}
          <nav aria-label={t("Lessons")} className="mt-10 flex justify-between gap-3 border-t pt-5">
            {prev ? (
              <div className="flex min-w-0">
                <ButtonLink to={`/t/${tr.id}/lesson/${prev.id}`} className="max-w-full" title={prev.title} aria-label={t("Previous lesson: {title}", { title: prev.title })}
                  icon={<ArrowLeft className="size-4 shrink-0" aria-hidden />}><span className="truncate">{prev.title}</span></ButtonLink>
              </div>
            ) : <span />}
            {next && (
              <div className="flex min-w-0">
                <ButtonLink to={`/t/${tr.id}/lesson/${next.id}`} className="max-w-full" title={next.title} aria-label={t("Next lesson: {title}", { title: next.title })}>
                  <span className="truncate">{next.title}</span><ArrowRight className="size-4 shrink-0" aria-hidden />
                </ButtonLink>
              </div>
            )}
          </nav>
        </article>
        <aside className="lg:sticky lg:top-20 lg:self-start">
          <Card className="overflow-hidden">
            <div className="border-b px-5 py-3.5">
              <h2 className="text-sm font-semibold">{t("Practise this domain")}</h2>
              <div className="text-label text-muted">{w.practiseDomain(qs.length)}</div>
              {progress.error && <div role="alert" className="pt-1 text-label text-bad">{t("Your progress could not be loaded.")}</div>}
            </div>
            {qs.length === 0 && <div className="px-5 py-4 text-sm text-muted">{w.noQuestions}</div>}
            <ul className="divide-y">
              {qs.map((q) => <QuestionRow key={q.id} track={tr.id} q={q} best={standing(progress.data, q)} compact />)}
            </ul>
          </Card>
        </aside>
      </div>
    </Page>
  );
}

function LessonSkeleton() {
  return (
    <Loading label={t("Loading the lesson")} className="grid grid-cols-[minmax(0,1fr)] gap-8 lg:grid-cols-[minmax(0,1fr)_340px]">
      <div className="space-y-4">
        <Skeleton className="h-9 w-3/4" />
        {[0, 1, 2, 3, 4, 5].map((i) => <Skeleton key={i} className={i % 3 === 2 ? "h-4 w-2/3" : "h-4"} />)}
        <Skeleton className="h-28 rounded-xl" />
        {[0, 1, 2].map((i) => <Skeleton key={i} className="h-4" />)}
      </div>
      <Skeleton className="h-64 rounded-2xl" />
    </Loading>
  );
}
