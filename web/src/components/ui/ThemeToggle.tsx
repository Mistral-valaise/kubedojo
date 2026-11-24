import { Moon, Sun } from "lucide-react";
import { useTheme } from "../../hooks";
import { t } from "../../i18n";
import { IconButton } from "./IconButton";

/** Switches between the dark and the light theme. A toggle button: pressed while the dark theme is on. */
export function ThemeToggle({ size }: { size?: "md" | "lg" }) {
  const [theme, toggle] = useTheme();
  const dark = theme === "dark";
  return (
    <IconButton size={size} onClick={toggle} aria-label={t("Dark theme")} aria-pressed={dark}
      title={dark ? t("Switch to the light theme") : t("Switch to the dark theme")}>
      {dark ? <Sun className="size-4.5" aria-hidden /> : <Moon className="size-4.5" aria-hidden />}
    </IconButton>
  );
}
