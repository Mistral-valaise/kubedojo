import { type ReactNode, isValidElement, memo, useEffect, useState, useSyncExternalStore } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Check, Copy, Play } from "lucide-react";
import { hasTerminal, onTerminalChange, runInTerminal } from "../terminalBus";
import { t } from "../i18n";

function textOf(node: ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (isValidElement<{ children?: ReactNode }>(node)) return textOf(node.props.children);
  return "";
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Clipboard API can be blocked (insecure origin, embedded pane): fall back.
    try {
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      const ok = document.execCommand("copy");
      ta.remove();
      return ok;
    } catch {
      return false;
    }
  }
}

function useCopied(): [boolean, (text: string) => void] {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const id = setTimeout(() => setCopied(false), 1300);
    return () => clearTimeout(id);
  }, [copied]);
  return [copied, (text) => void copyText(text).then((ok) => ok && setCopied(true))];
}

/**
 * Inline code: one click copies it — names, paths and commands in task texts.
 *
 * It is a convenience for the pointer and no control: a task text has dozens
 * of snippets, and a tab stop on each would bury the real controls for a
 * keyboard user, who selects and copies the text as any other. Only the
 * buttons of a code block are in the tab order.
 */
function InlineCode({ children }: { children?: ReactNode }) {
  const [copied, copy] = useCopied();
  const text = textOf(children);
  return (
    <code title={t("Click to copy")} onClick={() => copy(text)} className={"copyable" + (copied ? " copied" : "")}>
      {children}
      {copied && <span className="copied-tip" role="status">{t("Copied")}</span>}
    </code>
  );
}

const SHELL = /language-(bash|sh|shell|console)/;

function Pre({ children }: { children?: ReactNode }) {
  const [copied, copy] = useCopied();
  const [ran, setRan] = useState(false);
  const terminal = useSyncExternalStore(onTerminalChange, hasTerminal);
  const child = Array.isArray(children) ? children[0] : children;
  const className = isValidElement<{ className?: string }>(child) ? child.props.className ?? "" : "";
  const text = textOf(children).replace(/\n$/, "");
  const runnable = terminal && SHELL.test(className);
  const run = () => {
    if (runInTerminal(text)) { setRan(true); setTimeout(() => setRan(false), 1300); }
  };
  return (
    <pre>
      {/* Shown on hover and on focus, and always on a touch screen: see .pre-actions in markdown.css. */}
      <span className="pre-actions">
        {runnable && (
          <button type="button" onClick={run} title={t("Run in terminal")} aria-label={t("Run in terminal")}
            className="inline-flex h-9 items-center gap-1.5 rounded-lg border bg-panel px-2.5 font-sans text-xs font-semibold text-accent hover:border-accent">
            {ran ? <Check className="size-3.5 text-ok" aria-hidden /> : <Play className="size-3.5" aria-hidden />}{t("Run")}
          </button>
        )}
        <button type="button" onClick={() => copy(text)} title={t("Copy")} aria-label={t("Copy")}
          className="inline-flex size-9 items-center justify-center rounded-lg border bg-panel text-muted hover:text-text">
          {copied ? <Check className="size-4 text-ok" aria-hidden /> : <Copy className="size-4" aria-hidden />}
        </button>
      </span>
      <code className={className}>{text}</code>
    </pre>
  );
}

/**
 * `plain` renders inline code without the click-to-copy control, for markdown
 * that sits inside another control (an answer option), where a nested button
 * would be a second tab stop and would take the click.
 */
export const Markdown = memo(function Markdown({ children, className, plain }: { children: string; className?: string; plain?: boolean }) {
  return (
    <div className={"md " + (className ?? "")}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          pre: Pre,
          code: plain ? "code" : InlineCode,
          a: ({ href, children }) => <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>,
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
});
