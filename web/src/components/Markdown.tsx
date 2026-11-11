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

/** Inline code: one click copies it — names, paths and commands in task texts. */
function InlineCode({ children }: { children?: ReactNode }) {
  const [copied, copy] = useCopied();
  const text = textOf(children);
  return (
    <code
      role="button" tabIndex={0} title={t("Click to copy")}
      onClick={() => copy(text)}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); copy(text); } }}
      className={"copyable" + (copied ? " copied" : "")}
    >
      {children}
      {copied && <span className="copied-tip" aria-live="polite">{t("Copied")}</span>}
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
    <pre className="group">
      <span className="absolute right-2 top-2 flex gap-1 opacity-0 transition group-hover:opacity-100 focus-within:opacity-100">
        {runnable && (
          <button onClick={run} title={t("Run in terminal")} aria-label={t("Run in terminal")}
            className="inline-flex items-center gap-1 rounded-md border bg-panel px-1.5 py-1 font-sans text-[11px] font-medium text-muted hover:border-accent hover:text-accent">
            {ran ? <Check className="size-3.5 text-ok" /> : <Play className="size-3.5" />}{t("Run")}
          </button>
        )}
        <button onClick={() => copy(text)} title={t("Copy")} aria-label={t("Copy")}
          className="rounded-md border bg-panel p-1.5 text-muted hover:text-text">
          {copied ? <Check className="size-3.5 text-ok" /> : <Copy className="size-3.5" />}
        </button>
      </span>
      <code className={className}>{text}</code>
    </pre>
  );
}

export const Markdown = memo(function Markdown({ children, className }: { children: string; className?: string }) {
  return (
    <div className={"md " + (className ?? "")}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          pre: Pre,
          code: InlineCode,
          a: ({ href, children }) => <a href={href} target="_blank" rel="noreferrer">{children}</a>,
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
});
