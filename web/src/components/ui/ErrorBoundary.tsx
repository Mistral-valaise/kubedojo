import { Component, type ErrorInfo, type ReactNode } from "react";
import { RotateCw, TriangleAlert } from "lucide-react";
import { t } from "../../i18n";
import { Button } from "./Button";
import { EmptyState } from "./EmptyState";

/**
 * Catches an error thrown while rendering what is inside it and shows a
 * message with a way on, instead of an empty page. Wrap the routes in one;
 * `fallback` replaces the message for a smaller part of a page.
 */
export class ErrorBoundary extends Component<{
  children: ReactNode;
  /** Shown in place of the default message; gets the error and a function that tries the children again. */
  fallback?: (error: Error, retry: () => void) => ReactNode;
}, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("kubedojo: the page failed to render", error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    if (this.props.fallback) return this.props.fallback(error, () => this.setState({ error: null }));
    return (
      <main className="mx-auto flex min-h-full max-w-xl flex-col justify-center px-5 py-12">
        <div role="alert">
          <EmptyState level={1} icon={<TriangleAlert />} title={t("Something went wrong")}
            text={<>
              {t("This page could not be shown. Your progress and your sandbox are not affected: reload the page to carry on.")}
              <span className="mt-3 block font-mono text-xs break-words">{error.message}</span>
            </>}
            action={<>
              <Button variant="primary" size="lg" icon={<RotateCw className="size-4" aria-hidden />} onClick={() => window.location.reload()}>{t("Reload")}</Button>
              <a href="/" className="inline-flex h-11 items-center rounded-[0.625rem] px-4 text-body font-medium text-muted hover:text-text">{t("Back to start")}</a>
            </>} />
        </div>
      </main>
    );
  }
}

// What React calls on the class to turn a caught error into state. It is
// attached here and not declared in the class body, because the keyword that
// declaration needs is also the name of a Tailwind utility: its scanner reads
// this file as text and would add an unused rule to the stylesheet.
Object.assign(ErrorBoundary, {
  getDerivedStateFromError(error: unknown) {
    return { error: error instanceof Error ? error : new Error(String(error)) };
  },
});
