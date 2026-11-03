import type { ReactNode } from "react";
import { BadgeCheck, KeyRound, ShieldCheck, Users } from "lucide-react";
import { BrandMark, TenantLogo } from "@/components/TenantLogo";
import { PreferencesControls } from "@/components/Preferences";
import { getAuthTemplate, type AuthPattern, type AuthTemplate } from "@/lib/authTemplates";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { PublicTenant } from "@/lib/api";

const features = [
  { icon: ShieldCheck, title: "OIDC & OAuth 2.0", text: "Standards-based single sign-on with PKCE and rotating refresh tokens." },
  { icon: Users, title: "Multi-tenant by design", text: "Isolated users, roles and applications for every organization." },
  { icon: KeyRound, title: "Secure by default", text: "Argon2id passwords, TOTP MFA and complete audit trails." },
];

function Pattern({ pattern, panel = false }: { pattern: AuthPattern; panel?: boolean }) {
  if (pattern === "mesh") return <div className="brand-mesh absolute inset-0 -z-10" />;
  if (pattern === "grid") return <div className="auth-grid-pattern absolute inset-0 -z-10 opacity-70" />;
  if (pattern === "waves") return <div className="auth-waves absolute inset-0 -z-10" />;
  if (pattern === "blobs") return <div className="auth-blobs absolute inset-0 -z-10" />;
  if (pattern === "terminal") {
    return (
      <div className="auth-terminal-lines absolute inset-0 -z-10">
        <pre className="absolute bottom-8 left-8 hidden font-mono text-[10px] leading-6 text-emerald-400/40 lg:block">
{`$ ucenter login --tenant ${"acme"}
> OIDC discovery ... ok
> PKCE challenge ... ok
> session established`}
        </pre>
      </div>
    );
  }
  if (pattern === "seal") {
    return (
      <div className="auth-seal absolute inset-0 -z-10">
        <div className="absolute end-10 top-10 hidden h-24 w-24 rounded-full border-2 border-amber-300/30 lg:flex lg:items-center lg:justify-center">
          <BadgeCheck className="h-10 w-10 text-amber-300/50" />
        </div>
      </div>
    );
  }
  if (pattern === "sketch") {
    return (
      <div className="auth-sketch-doodles absolute inset-0 -z-10">
        <svg className="absolute start-[6%] top-[10%] h-24 w-24 text-[#3f3a33]/35" viewBox="0 0 100 100" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round">
          <circle cx="50" cy="50" r="18" />
          <path d="M50 20v-8M50 88v-8M20 50h-8M88 50h-8M29 29l-6-6M77 77l-6-6M71 29l6-6M23 77l6-6" />
        </svg>
        <svg className="absolute end-[8%] top-[18%] h-20 w-32 text-[#3f3a33]/35" viewBox="0 0 140 90" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round">
          <path d="M12 62c26-30 58-32 78-12 9 9 5 22-4 23-10 1-12-10 0-17 14-8 33-3 42 8" />
          <path d="M128 64l-2-10M128 64l-9 3" />
        </svg>
        <svg className="absolute bottom-[12%] start-[12%] h-16 w-24 text-[#3f3a33]/25" viewBox="0 0 120 80" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round">
          <path d="M8 40c10-18 24-18 30-4 5 12-6 20-12 12-5-7 4-15 15-13 16 3 22 17 35 15 8-1 16-7 24-17" />
        </svg>
      </div>
    );
  }
  if (pattern === "retail") {
    return (
      <div className="auth-retail-shapes absolute inset-0 -z-10">
        <div className="absolute start-[8%] top-[18%] h-16 w-16 rotate-12 rounded-2xl bg-orange-400/20" />
        <div className="absolute end-[12%] bottom-[20%] h-24 w-24 -rotate-12 rounded-full bg-rose-400/15" />
      </div>
    );
  }
  return panel ? null : <div className="absolute inset-0 -z-10 bg-muted/20" />;
}

function FeatureList({ template }: { template: AuthTemplate }) {
  const { t } = useI18n();
  return (
    <div className="space-y-4">
      {features.map((f) => (
        <div key={f.title} className="flex gap-3">
          <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-background/70 shadow-sm ring-1 ring-border">
            <f.icon className="h-4 w-4 text-primary" />
          </div>
          <div>
            <p className={cn("text-sm font-medium", template.panelTextClass)}>{t(f.title)}</p>
            <p className={cn("text-xs leading-relaxed", template.panelMutedClass)}>{t(f.text)}</p>
          </div>
        </div>
      ))}
    </div>
  );
}

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
  const template = getAuthTemplate(tenant?.branding?.loginTemplate);
  const split = template.layout === "split";
  const description = tenant?.branding?.loginTagline || t("A modern SSO user center with OIDC discovery, tenant isolation and enterprise-grade access controls.");
  const hidePlatformBranding = Boolean(tenant?.branding?.hidePlatformBranding);
  const brandName = tenant?.branding?.brandName?.trim() || "UCenter";

  const form = (
    <div className={cn(split ? "w-full max-w-md animate-fade-in" : "w-full animate-fade-in", split && template.formClass)}>
      <div className="mb-7 space-y-2">
        <h2 className={cn("text-2xl font-semibold tracking-tight", template.headlineClass)}>{title}</h2>
        {subtitle && <p className="text-sm text-muted-foreground">{subtitle}</p>}
      </div>
      {children}
      {footer && <div className="mt-6">{footer}</div>}
    </div>
  );

  const brandHeader = (
    <div className={cn("flex items-center gap-3", template.panelTextClass)}>
      {hidePlatformBranding && tenant ? (
        <TenantLogo tenant={tenant} className="h-10 w-10" />
      ) : (
        <BrandMark className="h-10 w-10" />
      )}
      <div>
        <p className="text-sm font-semibold tracking-tight">{brandName}</p>
        <p className={cn("text-xs", template.panelMutedClass)}>{hidePlatformBranding && tenant ? tenant.name : t("Identity platform")}</p>
      </div>
    </div>
  );

  if (!split) {
    return (
      <div className={cn("auth-shell relative min-h-screen overflow-hidden", template.shellClass)} data-template={template.id}>
        <Pattern pattern={template.pattern} />
        <div className="absolute end-4 top-4 z-20 rounded-full border bg-background/70 p-1 backdrop-blur">
          <PreferencesControls compact />
        </div>
        <div className="relative z-10 flex min-h-screen flex-col items-center justify-center px-5 py-16 sm:px-8">
          <div className="mb-8 flex flex-col items-center gap-4 text-center">
            {hidePlatformBranding && tenant ? (
              <TenantLogo tenant={tenant} className="h-12 w-12" />
            ) : (
              <BrandMark className="h-11 w-11" />
            )}
            {hidePlatformBranding && tenant && <p className={cn("text-sm font-semibold", template.panelTextClass)}>{brandName}</p>}
            {tenant && (
              <div className={cn("flex items-center gap-2", template.panelTextClass)}>
                <TenantLogo tenant={tenant} className="h-6 w-6" rounded="rounded-md" />
                <span className="text-sm font-medium">{tenant.name}</span>
              </div>
            )}
            <div>
              <h1 className={cn("text-3xl font-semibold tracking-tight sm:text-4xl", template.headlineClass, template.panelTextClass)}>
                {t("One identity.")} <span className="opacity-60">{t("Every application.")}</span>
              </h1>
              {tenant?.branding?.loginTagline && (
                <p className={cn("mx-auto mt-2 max-w-md text-sm", template.panelMutedClass)}>{tenant.branding.loginTagline}</p>
              )}
            </div>
          </div>
          <div className={cn("auth-form-card w-full", wide ? "max-w-xl" : "max-w-md", template.formClass || "rounded-2xl border bg-card p-8 shadow-soft")}>
            <div className="mb-7 space-y-2">
              <h2 className={cn("text-2xl font-semibold tracking-tight", template.headlineClass)}>{title}</h2>
              {subtitle && <p className="text-sm text-muted-foreground">{subtitle}</p>}
            </div>
            {children}
            {footer && <div className="mt-6">{footer}</div>}
          </div>
          <div className="mt-8 flex flex-wrap justify-center gap-x-5 gap-y-2 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1"><ShieldCheck className="h-3.5 w-3.5" />OIDC</span>
            <span className="inline-flex items-center gap-1"><KeyRound className="h-3.5 w-3.5" />PKCE</span>
            <span className="inline-flex items-center gap-1"><BadgeCheck className="h-3.5 w-3.5" />RS256</span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={cn("auth-shell relative min-h-screen overflow-hidden bg-background", template.shellClass)} data-template={template.id}>
      <div className="absolute end-4 top-4 z-20 rounded-full border bg-background/70 p-1 backdrop-blur">
        <PreferencesControls compact />
      </div>
      <div className="mx-auto grid min-h-screen max-w-6xl grid-cols-1 lg:grid-cols-[1.05fr_1fr]">
        <div className={cn("relative hidden flex-col justify-between overflow-hidden p-10 lg:flex", template.panelClass)}>
          <Pattern pattern={template.pattern} panel />
          {brandHeader}
          <div className="max-w-md space-y-8 py-10">
            <div className="space-y-4">
              {tenant ? (
                <div className="flex items-center gap-3">
                  <TenantLogo tenant={tenant} className="h-12 w-12" />
                  <div>
                    <p className={cn("text-sm", template.panelMutedClass)}>{t("Signing in to")}</p>
                    <p className={cn("text-lg font-semibold", template.panelTextClass)}>{tenant.name}</p>
                  </div>
                </div>
              ) : (
                <p className={cn("text-sm font-medium", template.panelTextClass)}>{t("Multi-tenant by design")}</p>
              )}
              <h1 className={cn("text-4xl font-semibold tracking-tight", template.headlineClass, template.panelTextClass)}>
                {t("One identity.")}
                <br />
                {t("Every application.")}
              </h1>
              <p className={cn("text-sm leading-relaxed", template.panelMutedClass)}>{description}</p>
            </div>
            <FeatureList template={template} />
          </div>
          <div className={cn("flex items-center gap-2 text-xs", template.panelMutedClass)}>
            <BadgeCheck className="h-4 w-4" />
            <span>{t("OpenID Connect authorization code flow · RS256 · PKCE")}</span>
          </div>
        </div>

        <div className="flex items-center justify-center px-5 py-10 sm:px-10">
          <div className={cn(wide ? "w-full max-w-xl" : "w-full max-w-md", "animate-fade-in")}>{form}</div>
        </div>
      </div>
    </div>
  );
}
