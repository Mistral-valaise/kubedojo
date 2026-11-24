import { Loader2 } from "lucide-react";
import { cx } from "./cx";

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cx("spin size-5 text-muted", className)} />;
}
