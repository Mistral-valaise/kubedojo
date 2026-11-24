import type { ReactNode } from "react";

export function ErrorNote({ children }: { children: ReactNode }) {
  return <div className="rounded-xl border border-bad/40 bg-bad-soft px-4 py-3 text-sm text-bad">{children}</div>;
}
