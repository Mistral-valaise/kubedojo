import { useCallback, useRef, useState } from "react";
import { Plus, SquareTerminal, X } from "lucide-react";
import { useConfig } from "../config";
import { t } from "../i18n";
import { Terminal, shortcutModifiers, type TerminalShortcut } from "./Terminal";
import { cx } from "./ui";

/** Used until the server's limit is known, and when it cannot be fetched. */
export const DEFAULT_MAX_TERMINALS = 4;

interface Tab { id: number; label: string }

const first: Tab = { id: 1, label: "Terminal 1" };

/**
 * Terminal area with tabs. Every tab is an independent shell in the same
 * sandbox, so one can run `watch` or follow logs while another one works.
 *
 * All tabs stay mounted: a background tab keeps its WebSocket, its shell and
 * its scrollback, and keeps receiving output. It is hidden with `visibility`,
 * not `display`, so that it keeps its size and is resized together with the
 * visible one. Closing a tab unmounts its Terminal, which closes the WebSocket
 * and ends the shell; unmounting this component (navigation, exam finished)
 * does that for all of them.
 */
export function TerminalTabs(props: { sessionId: string; max?: number }) {
  // A different sandbox starts over with a single terminal.
  return <Tabs key={props.sessionId} {...props} />;
}

function Tabs({ sessionId, max: maxProp }: { sessionId: string; max?: number }) {
  const [tabs, setTabs] = useState<Tab[]>([first]);
  const [active, setActive] = useState(first.id);
  const nextId = useRef(first.id + 1);
  const tabRefs = useRef(new Map<number, HTMLButtonElement>());

  // The limit of the server counts unless the caller sets one.
  const config = useConfig(maxProp === undefined);
  const limit = maxProp ?? (config && config.maxTerminals > 0 ? config.maxTerminals : DEFAULT_MAX_TERMINALS);

  const full = tabs.length >= limit;

  const add = useCallback(() => {
    if (tabs.length >= limit) return;
    const id = nextId.current++;
    setTabs([...tabs, { id, label: `Terminal ${id}` }]);
    setActive(id);
  }, [tabs, limit]);

  const close = useCallback((id: number) => {
    const i = tabs.findIndex((x) => x.id === id);
    if (i < 0 || tabs.length <= 1) return; // there is always one terminal
    const next = tabs.filter((x) => x.id !== id);
    setTabs(next);
    // Closing the visible tab shows its neighbour; closing another one changes nothing.
    if (active === id) setActive(next[Math.min(i, next.length - 1)].id);
  }, [tabs, active]);

  const step = useCallback((delta: number) => {
    const i = tabs.findIndex((x) => x.id === active);
    setActive(tabs[(i + delta + tabs.length) % tabs.length].id);
  }, [tabs, active]);

  const onShortcut = useCallback((s: TerminalShortcut) => {
    if (s === "new") add();
    else if (s === "close") close(active);
    else if (s === "next") step(1);
    else if (s === "previous") step(-1);
    else if (tabs[s.select - 1]) setActive(tabs[s.select - 1].id);
  }, [add, close, step, active, tabs]);

  // Arrow keys move between tabs when the tab bar has the focus (WAI-ARIA tabs).
  const onTabKey = (ev: React.KeyboardEvent, i: number) => {
    const to = ev.key === "ArrowRight" ? i + 1 : ev.key === "ArrowLeft" ? i - 1 : ev.key === "Home" ? 0 : ev.key === "End" ? tabs.length - 1 : null;
    if (to === null) return;
    ev.preventDefault();
    const tab = tabs[(to + tabs.length) % tabs.length];
    setActive(tab.id);
    tabRefs.current.get(tab.id)?.focus();
  };

  return (
    <div className="term-surface flex h-full w-full flex-col bg-term">
      <div className="flex h-11 shrink-0 items-center gap-1 border-b border-term-line px-1.5 text-label font-medium text-term-muted">
        <div role="tablist" aria-label={t("Terminals")} className="flex min-w-0 items-center gap-1 overflow-x-auto p-0.5">
          {tabs.map((tab, i) => {
            const selected = tab.id === active;
            return (
              // The tab and its close button are one pill; the selected one is filled.
              <div key={tab.id} className={cx("flex h-9 shrink-0 items-stretch rounded-lg transition-colors",
                selected ? "bg-term-line text-term-text" : "hover:bg-term-line/60 hover:text-term-text")}>
                <button
                  ref={(el) => { if (el) tabRefs.current.set(tab.id, el); else tabRefs.current.delete(tab.id); }}
                  role="tab" id={`terminal-tab-${tab.id}`} aria-selected={selected} aria-controls={`terminal-panel-${tab.id}`}
                  tabIndex={selected ? 0 : -1}
                  onClick={() => setActive(tab.id)} onKeyDown={(ev) => onTabKey(ev, i)}
                  className={cx("flex items-center gap-1.5 rounded-lg pl-3", tabs.length > 1 ? "pr-1.5" : "pr-3")}
                >
                  <SquareTerminal className="size-3.5" aria-hidden />
                  <span className="whitespace-nowrap">{tab.label}</span>
                </button>
                {tabs.length > 1 && (
                  <button
                    onClick={() => close(tab.id)}
                    aria-label={t("Close {name}", { name: tab.label })} title={t("Close {name}", { name: tab.label })}
                    className="mr-1 flex size-7 items-center justify-center self-center rounded-md text-term-muted hover:bg-term hover:text-term-text"
                  >
                    <X className="size-3.5" aria-hidden />
                  </button>
                )}
              </div>
            );
          })}
        </div>
        <button
          onClick={add} disabled={full}
          aria-label={t("New terminal")}
          title={full
            ? t("At most {n} terminals can be open. Close one to open another.", { n: limit })
            : t("New terminal ({keys}+T)", { keys: shortcutModifiers() })}
          className="flex size-9 shrink-0 items-center justify-center rounded-lg hover:bg-term-line hover:text-term-text disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent disabled:hover:text-term-muted"
        >
          <Plus className="size-4" aria-hidden />
        </button>
      </div>
      <div className="relative min-h-0 flex-1">
        {tabs.map((tab) => {
          const selected = tab.id === active;
          return (
            <div key={tab.id} role="tabpanel" id={`terminal-panel-${tab.id}`} aria-labelledby={`terminal-tab-${tab.id}`}
              aria-hidden={!selected}
              className={cx("absolute inset-0", !selected && "invisible")}>
              <Terminal sessionId={sessionId} active={selected} onShortcut={onShortcut} />
            </div>
          );
        })}
      </div>
    </div>
  );
}
