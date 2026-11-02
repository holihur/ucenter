import { Fingerprint } from "lucide-react";
import { cn, initials } from "@/lib/utils";
import type { PublicTenant } from "@/lib/api";

export function BrandMark({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "flex items-center justify-center rounded-xl bg-gradient-to-br from-primary to-violet-600 text-white shadow-lg shadow-primary/25",
        className,
      )}
    >
      <Fingerprint className="h-1/2 w-1/2" strokeWidth={2.2} />
    </div>
  );
}

export function TenantLogo({
  tenant,
  className,
  rounded = "rounded-xl",
}: {
  tenant?: Pick<PublicTenant, "name" | "branding"> | null;
  className?: string;
  rounded?: string;
}) {
  const logo = tenant?.branding?.logoUrl;
  if (logo) {
    return (
      <img
        src={logo}
        alt={tenant?.name || "Tenant logo"}
        className={cn("h-10 w-10 object-contain", rounded, className)}
      />
    );
  }
  return (
    <div
      className={cn(
        "flex items-center justify-center bg-gradient-to-br from-primary to-violet-600 text-sm font-semibold text-white shadow-sm",
        rounded,
        className,
      )}
    >
      {initials(tenant?.name || "UC")}
    </div>
  );
}
