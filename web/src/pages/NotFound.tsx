import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { BrandMark } from "@/components/TenantLogo";
import { useI18n } from "@/lib/i18n";

export function NotFound() {
  const { t } = useI18n();
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-5 px-6 text-center">
      <BrandMark className="h-12 w-12" />
      <div>
        <p className="text-6xl font-semibold tracking-tight">404</p>
        <p className="mt-2 text-muted-foreground">{t("This page does not exist or has moved.")}</p>
      </div>
      <Button asChild>
        <Link to="/">{t("Back to UCenter")}</Link>
      </Button>
    </div>
  );
}
