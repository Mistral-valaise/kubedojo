import type { ReactNode } from "react";
import { t } from "../i18n";
import { Workspace } from "./Workspace";

/**
 * Two panes. From the `md` breakpoint they sit side by side with a draggable
 * divider whose position is remembered. Below it there is no room for both:
 * each pane takes the full width and two tabs switch between them.
 *
 * Both panes stay mounted at every width; see Workspace, which this is the
 * two-pane form of.
 */
export function Split({ left, right, storageKey = "kubedojo.split", labels }: {
  left: ReactNode; right: ReactNode; storageKey?: string;
  /** Names of the two tabs on a narrow screen. */
  labels?: { left: string; right: string };
}) {
  const names = labels ?? { left: t("Task"), right: t("Terminal") };
  return (
    <Workspace storageKey={storageKey} panes={[
      { id: "left", label: names.left, content: left },
      { id: "right", label: names.right, content: right },
    ]} />
  );
}
