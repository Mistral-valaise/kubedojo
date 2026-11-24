import { Moon, Sun } from "lucide-react";
import { useTheme } from "../../hooks";
import { t } from "../../i18n";
import { Button } from "./Button";

export function ThemeToggle() {
  const [theme, toggle] = useTheme();
  return (
    <Button variant="ghost" size="sm" onClick={toggle} aria-label={t("Toggle theme")} title={t("Toggle theme")}>
      {theme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </Button>
  );
}
