import type { ReactNode } from "react";
import { BadgeCheck, KeyRound, ShieldCheck, Users } from "lucide-react";
import { BrandMark, TenantLogo } from "@/components/TenantLogo";
import { PreferencesControls } from "@/components/Preferences";
import { useI18n } from "@/lib/i18n";
import type { PublicTenant } from "@/lib/api";

const features = [
  { icon: ShieldCheck, title: "OIDC & OAuth 2.0", text: "Standards-based single sign-on with PKCE and rotating refresh tokens." },
  { icon: Users, title: "Multi-tenant by design", text: "Isolated users, roles and applications for every organization." },
  { icon: KeyRound, title: "Secure by default", text: "Argon2id passwords, TOTP MFA and complete audit trails." },
];

export function AuthShell({
  tenant,
  title,
  subtitle,
  children,
  footer,
  wide = false,
}: {
  tenant?: PublicTenant | null;
  title: string;
  subtitle?: string;
  children: ReactNode;
  footer?: ReactNode;
  wide?: boolean;
}) {
  const { t } = useI18n();
  return (
    <div className="relative min-h-screen overflow-hidden bg-background">
      <PreferencesControls compact className="absolute end-4 top-4 z-20" />
      <div className="absolute inset-0 -z-10 grid-pattern opacity-40" />
      <div className="mx-auto grid min-h-screen max-w-6xl grid-cols-1 lg:grid-cols-[1.05fr_1fr]">
        <div className="relative hidden flex-col justify-between overflow-hidden p-10 lg:flex">
          <div className="brand-mesh absolute inset-0 -z-10" />
          <div className="flex items-center gap-3">
            <BrandMark className="h-10 w-10" />
            <div>
              <p className="text-sm font-semibold tracking-tight">UCenter</p>
              <p className="text-xs text-muted-foreground">{t("Identity platform")}</p>
            </div>
          </div>

          <div className="max-w-md space-y-8 py-10">
            <div className="space-y-4">
              {tenant ? (
                <div className="flex items-center gap-3">
                  <TenantLogo tenant={tenant} className="h-12 w-12" />
                  <div>
                    <p className="text-sm text-muted-foreground">{t("Signing in to")}</p>
                    <p className="text-lg font-semibold">{tenant.name}</p>
                  </div>
                </div>
              ) : (
                <p className="text-sm font-medium text-primary">{t("Multi-tenant by design")}</p>
              )}
              <h1 className="text-4xl font-semibold tracking-tight">
                {t("One identity.")}
                <br />
                {t("Every application.")}
              </h1>
              <p className="text-muted-foreground">
                {t("A modern SSO user center with OIDC discovery, tenant isolation and enterprise-grade access controls.")}
              </p>
            </div>

            <div className="space-y-4">
              {features.map((f) => (
                <div key={t(f.title)} className="flex gap-3">
                  <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-background/70 shadow-sm ring-1 ring-border">
                    <f.icon className="h-4 w-4 text-primary" />
                  </div>
                  <div>
                    <p className="text-sm font-medium">{t(f.title)}</p>
                    <p className="text-xs leading-relaxed text-muted-foreground">{t(f.text)}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <BadgeCheck className="h-4 w-4 text-primary" />
            <span>{t("OpenID Connect authorization code flow · RS256 · PKCE")}</span>
          </div>
        </div>

        <div className="flex items-center justify-center px-5 py-10 sm:px-10">
          <div className={wide ? "w-full max-w-xl animate-fade-in" : "w-full max-w-md animate-fade-in"}>
            <div className="mb-8 flex items-center gap-3 lg:hidden">
              <BrandMark className="h-9 w-9" />
              <div>
                <p className="text-sm font-semibold">UCenter</p>
                {tenant && <p className="text-xs text-muted-foreground">{tenant.name}</p>}
              </div>
            </div>
            <div className="mb-7 space-y-2">
              <h2 className="text-2xl font-semibold tracking-tight">{title}</h2>
              {subtitle && <p className="text-sm text-muted-foreground">{subtitle}</p>}
            </div>
            {children}
            {footer && <div className="mt-6">{footer}</div>}
          </div>
        </div>
      </div>
    </div>
  );
}
