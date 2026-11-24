import { useLocation } from "react-router";
import { ArrowLeft, Compass } from "lucide-react";
import { t } from "../i18n";
import { ButtonLink, EmptyState, Page } from "../components/ui";

/** The page for an address that leads nowhere: it says so and offers the way back. */
export function NotFound() {
  const { pathname } = useLocation();
  return (
    <Page>
      <EmptyState level={1} className="mx-auto mt-6 max-w-xl sm:mt-12" icon={<Compass />} title={t("Page not found")}
        text={<>
          {t("There is nothing at this address. The link may be old, or the address has a typo.")}
          <span className="mt-3 block font-mono text-xs break-all">{pathname}</span>
        </>}
        action={<ButtonLink to="/" variant="primary" size="lg" icon={<ArrowLeft className="size-4" aria-hidden />}>{t("Back to the start page")}</ButtonLink>} />
    </Page>
  );
}
