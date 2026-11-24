import { t } from "../../i18n";
import { Badge } from "./Badge";

export function Difficulty({ level }: { level: string }) {
  const tone = level === "easy" ? "ok" : level === "hard" ? "bad" : "warn";
  return <Badge tone={tone}>{t(level)}</Badge>;
}
