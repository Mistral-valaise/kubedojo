import { Link, useParams } from "react-router";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { api } from "../api";
import { useLoad } from "../hooks";
import { t } from "../i18n";
import { Markdown } from "../components/Markdown";
import { Button, Card, ErrorNote, Page, Spinner } from "../components/ui";
import { QuestionRow } from "./Track";

export function LessonPage() {
  const { track: id = "", domain = "" } = useParams();
  const track = useLoad(() => api.track(id), [id]);
  const lesson = useLoad(() => api.lesson(id, domain), [id, domain]);
  const progress = useLoad(() => api.progress(id), [id]);

  if (track.error || lesson.error) return <Page><ErrorNote>{track.error ?? lesson.error}</ErrorNote></Page>;
  if (!track.data || !lesson.data) return <Page><Spinner /></Page>;
  const tr = track.data;
  const qs = (tr.questions ?? []).filter((q) => q.domain === domain);
  const idx = tr.domains.findIndex((d) => d.id === domain);
  const prev = tr.domains[idx - 1], next = tr.domains[idx + 1];

  return (
    <Page crumbs={[{ label: tr.short, to: `/t/${tr.id}` }, { label: lesson.data.domain.title }]}>
      <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_340px]">
        <article className="min-w-0">
          <Markdown>{lesson.data.markdown}</Markdown>
          <div className="mt-10 flex justify-between gap-3 border-t pt-5">
            {prev ? <Link to={`/t/${tr.id}/lesson/${prev.id}`}><Button icon={<ArrowLeft className="size-4" />}>{prev.title}</Button></Link> : <span />}
            {next && <Link to={`/t/${tr.id}/lesson/${next.id}`}><Button>{next.title}<ArrowRight className="size-4" /></Button></Link>}
          </div>
        </article>
        <aside className="lg:sticky lg:top-20 lg:self-start">
          <Card className="overflow-hidden">
            <div className="border-b px-5 py-3.5">
              <div className="text-sm font-semibold">{t("Practise this domain")}</div>
              <div className="text-xs text-muted">{t("{n} questions on a live cluster", { n: qs.length })}</div>
            </div>
            <ul className="divide-y">
              {qs.map((q) => <QuestionRow key={q.id} track={tr.id} q={q} best={progress.data?.questions[q.id]?.best} compact />)}
            </ul>
          </Card>
        </aside>
      </div>
    </Page>
  );
}
