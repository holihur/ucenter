import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, Download, Globe, KeyRound, Loader2, Palette, Save, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyButton } from "@/components/CopyButton";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { AUTH_TEMPLATES, type AuthTemplate } from "@/lib/authTemplates";
import { api, type Tenant, type TenantSettings } from "@/lib/api";
import { applyBrandColor, cn } from "@/lib/utils";

export function AdminSettings() {
  const { can, me } = useAuth();
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["settings"],
    queryFn: () => api.get<{ tenant: Tenant }>("/admin/settings"),
  });

  const [name, setName] = useState("");
  const [domains, setDomains] = useState("");
  const [branding, setBranding] = useState({
    logoUrl: "",
    primaryColor: "#4f46e5",
    backgroundColor: "#f8fafc",
    supportEmail: "",
    loginTemplate: "aurora",
    loginTagline: "",
    brandName: "",
    hidePlatformBranding: false,
  });
  const [settings, setSettings] = useState<TenantSettings>({
    allowRegistration: false,
    requireMfa: false,
    passwordMinLength: 10,
    sessionTtlHours: 12,
    allowedEmailDomains: [],
    allowPasswordLogin: true,
    quotaUsers: 0,
    quotaClients: 0,
  });
  const [emailDomains, setEmailDomains] = useState("");
  const [saving, setSaving] = useState(false);
  const [rotateOpen, setRotateOpen] = useState(false);
  const [rotating, setRotating] = useState(false);

  useEffect(() => {
    const tenant = query.data?.tenant;
    if (!tenant) return;
    setName(tenant.name);
    setDomains((tenant.domains ?? []).join("\n"));
    setBranding({
      logoUrl: tenant.branding?.logoUrl ?? "",
      primaryColor: tenant.branding?.primaryColor || "#4f46e5",
      backgroundColor: tenant.branding?.backgroundColor || "#f8fafc",
      supportEmail: tenant.branding?.supportEmail ?? "",
      loginTemplate: tenant.branding?.loginTemplate || "aurora",
      loginTagline: tenant.branding?.loginTagline ?? "",
      brandName: tenant.branding?.brandName ?? "",
      hidePlatformBranding: tenant.branding?.hidePlatformBranding ?? false,
    });
    setSettings({
      ...tenant.settings,
      allowedEmailDomains: tenant.settings?.allowedEmailDomains ?? [],
    });
    setEmailDomains((tenant.settings?.allowedEmailDomains ?? []).join("\n"));
  }, [query.data]);

  const tenant = query.data?.tenant;
  const editable = can("tenant:write");

  async function rotateSigningKey() {
    setRotating(true);
    try {
      const res = await api.post<{ kid: string }>("/admin/signing-keys/rotate");
      toast.success(t("Signing key rotated"), { description: `${t("New key")}: ${res.kid}` });
      setRotateOpen(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to rotate signing key"));
    } finally {
      setRotating(false);
    }
  }

  async function save() {
    setSaving(true);
    try {
      await api.patch("/admin/settings", {
        name,
        domains: domains
          .split("\n")
          .map((d) => d.trim())
          .filter(Boolean),
        branding,
        settings: {
          ...settings,
          allowedEmailDomains: emailDomains
            .split("\n")
            .map((d) => d.trim())
            .filter(Boolean),
        },
      });
      applyBrandColor(branding.primaryColor);
      toast.success(t("Settings saved"));
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["settings"] }),
        queryClient.invalidateQueries({ queryKey: ["admin-context"] }),
        queryClient.invalidateQueries({ queryKey: ["me"] }),
      ]);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to save settings"));
    } finally {
      setSaving(false);
    }
  }

  if (query.isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-9 w-56" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  const issuer = `${window.location.origin}/t/${tenant?.slug ?? me?.tenant.slug ?? ""}`;

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Tenant settings")}
        description={t("Branding, registration and security policy for this organization.")}
        actions={
          editable ? (
            <>
              <Button variant="outline" asChild>
                <a href="/api/admin/export">
                  <Download className="h-4 w-4" />
                  {t("Export tenant data")}
                </a>
              </Button>
              <Button onClick={save} disabled={saving}>
                {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
                {t("Save changes")}
              </Button>
            </>
          ) : null
        }
      />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Globe className="h-4 w-4 text-primary" />
            OIDC issuer
          </CardTitle>
          <CardDescription>{t("Use this issuer with any OpenID Connect client library.")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center gap-2 rounded-lg border bg-muted/30 p-2.5">
            <code className="flex-1 break-all font-mono text-xs">{issuer}</code>
            <CopyButton value={issuer} label="" />
          </div>
          <div className="flex flex-wrap gap-3 text-xs">
            <a className="text-primary hover:underline" href={`${issuer}/.well-known/openid-configuration`} target="_blank" rel="noreferrer">
              {t("Discovery document")}
            </a>
            <a className="text-primary hover:underline" href={`${issuer}/oidc/jwks`} target="_blank" rel="noreferrer">
              {t("JWKS")}
            </a>
            <span className="text-muted-foreground">{t("Tenant slug:")} <code className="font-mono">{tenant?.slug}</code></span>
          </div>
        </CardContent>
      </Card>

      <Tabs defaultValue="general" className="space-y-4">
        <TabsList>
          <TabsTrigger value="general">
            <Building2 className="mr-1.5 h-3.5 w-3.5" />
            {t("General")}
          </TabsTrigger>
          <TabsTrigger value="branding">
            <Palette className="mr-1.5 h-3.5 w-3.5" />
            {t("Branding")}
          </TabsTrigger>
          <TabsTrigger value="security">
            <ShieldCheck className="mr-1.5 h-3.5 w-3.5" />
            {t("Security")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="general">
          <Card>
            <CardHeader>
              <CardTitle>{t("Organization profile")}</CardTitle>
              <CardDescription>{t("Shown to users on the sign-in and consent screens.")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="tenant-name">{t("Display name")}</Label>
                  <Input id="tenant-name" value={name} onChange={(e) => setName(e.target.value)} disabled={!editable} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="tenant-slug">{t("Slug")}</Label>
                  <Input id="tenant-slug" value={tenant?.slug ?? ""} disabled />
                  <p className="text-[11px] text-muted-foreground">{t("The slug is permanent and used in the issuer URL.")}</p>
                </div>
              </div>
              <Separator />
              <div className="space-y-2">
                <Label htmlFor="tenant-domains">{t("Custom domains")}</Label>
                <Textarea
                  id="tenant-domains"
                  rows={3}
                  value={domains}
                  onChange={(e) => setDomains(e.target.value)}
                  disabled={!editable}
                  placeholder={"login.acme.com\nsso.acme.com"}
                  className="font-mono text-xs"
                />
                <p className="text-[11px] text-muted-foreground">
                  {t("One domain per line. Requests from these hostnames resolve to this tenant; each domain can belong to only one tenant.")}
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="branding">
          <Card>
            <CardHeader>
              <CardTitle>{t("Brand appearance")}</CardTitle>
              <CardDescription>{t("Applied to the login experience and admin console accents.")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="brand-name">{t("Brand name")}</Label>
                  <Input
                    id="brand-name"
                    value={branding.brandName}
                    onChange={(e) => setBranding({ ...branding, brandName: e.target.value })}
                    disabled={!editable}
                    placeholder="Acme Identity"
                  />
                  <p className="text-[11px] text-muted-foreground">{t("Replaces the UCenter name on login pages when platform branding is hidden.")}</p>
                </div>
                <label className="flex items-center justify-between gap-4 rounded-lg border p-3 text-sm">
                  <span>
                    <span className="font-medium">{t("Hide platform branding")}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">{t("Show only this organization's logo and brand name on sign-in pages.")}</span>
                  </span>
                  <Switch checked={branding.hidePlatformBranding} onCheckedChange={(v) => setBranding({ ...branding, hidePlatformBranding: v })} disabled={!editable} />
                </label>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="logo-url">{t("Logo URL")}</Label>
                  <Input
                    id="logo-url"
                    value={branding.logoUrl}
                    onChange={(e) => setBranding({ ...branding, logoUrl: e.target.value })}
                    disabled={!editable}
                    placeholder="https://…/logo.svg"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="support-email">{t("Support email")}</Label>
                  <Input
                    id="support-email"
                    type="email"
                    value={branding.supportEmail}
                    onChange={(e) => setBranding({ ...branding, supportEmail: e.target.value })}
                    disabled={!editable}
                    placeholder="support@example.com"
                  />
                </div>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="primary-color">{t("Primary color")}</Label>
                  <div className="flex gap-2">
                    <input
                      id="primary-color"
                      type="color"
                      value={branding.primaryColor || "#4f46e5"}
                      onChange={(e) => setBranding({ ...branding, primaryColor: e.target.value })}
                      disabled={!editable}
                      className="h-9 w-12 cursor-pointer rounded-md border bg-background p-1"
                    />
                    <Input
                      value={branding.primaryColor}
                      onChange={(e) => setBranding({ ...branding, primaryColor: e.target.value })}
                      disabled={!editable}
                      className="font-mono text-xs"
                    />
                  </div>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="background-color">{t("Background color")}</Label>
                  <div className="flex gap-2">
                    <input
                      id="background-color"
                      type="color"
                      value={branding.backgroundColor || "#f8fafc"}
                      onChange={(e) => setBranding({ ...branding, backgroundColor: e.target.value })}
                      disabled={!editable}
                      className="h-9 w-12 cursor-pointer rounded-md border bg-background p-1"
                    />
                    <Input
                      value={branding.backgroundColor}
                      onChange={(e) => setBranding({ ...branding, backgroundColor: e.target.value })}
                      disabled={!editable}
                      className="font-mono text-xs"
                    />
                  </div>
                </div>
              </div>
              <div className="space-y-3 rounded-lg border p-4">
                <div>
                  <Label>{t("Login page template")}</Label>
                  <p className="text-xs text-muted-foreground">
                    {t("Choose a built-in design language for login, registration and consent pages.")}
                  </p>
                </div>
                <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                  {AUTH_TEMPLATES.map((template) => (
                    <button
                      key={template.id}
                      type="button"
                      disabled={!editable}
                      onClick={() => setBranding({ ...branding, loginTemplate: template.id })}
                      className={cn(
                        "rounded-xl border p-2.5 text-left transition hover:-translate-y-0.5 hover:shadow-md disabled:pointer-events-none disabled:opacity-60",
                        branding.loginTemplate === template.id && "ring-2 ring-primary ring-offset-2 ring-offset-background",
                      )}
                    >
                      <TemplatePreview template={template} />
                      <div className="mt-2 flex items-baseline justify-between gap-2">
                        <span className="text-sm font-medium">{template.name}</span>
                        <span className="truncate text-[10px] text-muted-foreground">{t(template.industry)}</span>
                      </div>
                      <p className="mt-0.5 line-clamp-1 text-[10px] text-muted-foreground">{t(template.description)}</p>
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="login-tagline">{t("Login page tagline")}</Label>
                <Input
                  id="login-tagline"
                  value={branding.loginTagline}
                  onChange={(e) => setBranding({ ...branding, loginTagline: e.target.value })}
                  disabled={!editable}
                  placeholder={t("One identity for the whole organization.")}
                />
                <p className="text-[11px] text-muted-foreground">
                  {t("Shown on the login page next to the organization name.")}
                </p>
              </div>

              <div className="rounded-lg border p-4">
                <p className="mb-2 text-xs font-medium text-muted-foreground">{t("Preview")}</p>
                <div
                  className="rounded-lg p-4"
                  style={{ backgroundColor: branding.backgroundColor || "#f8fafc" }}
                >
                  <div className="flex items-center gap-3">
                    <div
                      className="flex h-10 w-10 items-center justify-center rounded-lg text-sm font-semibold text-white"
                      style={{ backgroundColor: branding.primaryColor || "#4f46e5" }}
                    >
                      {(name || "UC").slice(0, 2).toUpperCase()}
                    </div>
                    <div>
                      <p className="text-sm font-semibold text-slate-900">{name || "Organization"}</p>
                      <p className="text-xs text-slate-500">{t("Sign in to continue")}</p>
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="security">
          <Card>
            <CardHeader>
              <CardTitle>{t("Authentication policy")}</CardTitle>
              <CardDescription>{t("Controls how members sign in and how long sessions last.")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="space-y-4">
                <ToggleRow
                  title={t("Allow password sign-in")}
                  description={t("When disabled, users can only be authenticated through external identity flows.")}
                  checked={settings.allowPasswordLogin}
                  onCheckedChange={(v) => setSettings({ ...settings, allowPasswordLogin: v })}
                  disabled={!editable}
                />
                <ToggleRow
                  title={t("Allow self-registration")}
                  description={t("Let people create an account from the login page without an invitation.")}
                  checked={settings.allowRegistration}
                  onCheckedChange={(v) => setSettings({ ...settings, allowRegistration: v })}
                  disabled={!editable}
                />
                <ToggleRow
                  title={t("Require two-factor authentication")}
                  description={t("Prompt members without MFA to enroll after signing in.")}
                  checked={settings.requireMfa}
                  onCheckedChange={(v) => setSettings({ ...settings, requireMfa: v })}
                  disabled={!editable}
                />
              </div>

              <Separator />

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="password-length">{t("Minimum password length")}</Label>
                  <Input
                    id="password-length"
                    type="number"
                    min={8}
                    max={128}
                    value={settings.passwordMinLength}
                    onChange={(e) => setSettings({ ...settings, passwordMinLength: Number(e.target.value) })}
                    disabled={!editable}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="session-ttl">{t("Session lifetime (hours)")}</Label>
                  <Input
                    id="session-ttl"
                    type="number"
                    min={1}
                    max={720}
                    value={settings.sessionTtlHours}
                    onChange={(e) => setSettings({ ...settings, sessionTtlHours: Number(e.target.value) })}
                    disabled={!editable}
                  />
                </div>
              </div>

              <div className="flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <p className="flex items-center gap-2 text-sm font-medium">
                    <KeyRound className="h-4 w-4 text-primary" />
                    {t("Signing keys")}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t("Rotate the tenant signing key. The previous key stays in JWKS for overlap.")}
                  </p>
                </div>
                <Button variant="outline" onClick={() => setRotateOpen(true)} disabled={!editable}>
                  {t("Rotate signing key")}
                </Button>
              </div>

              <div className="space-y-2">
                <Label htmlFor="email-domains">{t("Allowed email domains")}</Label>
                <Textarea
                  id="email-domains"
                  rows={3}
                  value={emailDomains}
                  onChange={(e) => setEmailDomains(e.target.value)}
                  disabled={!editable}
                  placeholder={"acme.com\nsubsidiary.com"}
                  className="font-mono text-xs"
                />
                <p className="text-[11px] text-muted-foreground">
                  {t("Leave empty to allow any email domain for registration.")}
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <ConfirmDialog
        open={rotateOpen}
        onOpenChange={setRotateOpen}
        title={t("Rotate signing key?")}
        description={t("New tokens will be signed with a new RSA key. Existing tokens remain valid while the old key is published in JWKS.")}
        confirmLabel={t("Rotate key")}
        loading={rotating}
        onConfirm={rotateSigningKey}
      />
    </div>
  );
}

const previewPatternClass: Record<AuthTemplate["pattern"], string> = {
  mesh: "brand-mesh",
  grid: "auth-grid-pattern",
  waves: "auth-waves",
  blobs: "auth-blobs",
  terminal: "auth-terminal-lines",
  seal: "auth-seal",
  retail: "auth-retail-shapes",
  sketch: "auth-sketch-doodles",
  none: "",
};

function TemplatePreview({ template }: { template: AuthTemplate }) {
  const centered = template.layout !== "split";
  return (
    <div className={cn("relative h-24 overflow-hidden rounded-lg border", template.shellClass, template.panelClass)}>
      <div className={cn("absolute inset-0 opacity-80", previewPatternClass[template.pattern])} />
      <div className={cn("absolute left-2.5 top-2.5 h-5 w-5 rounded bg-white/85 shadow-sm", centered && "left-1/2 -translate-x-1/2")} />
      <div className={cn("absolute bottom-2.5 left-2.5 space-y-1", centered && "left-1/2 w-14 -translate-x-1/2")}>
        <div className="h-1.5 w-12 rounded bg-white/75" />
        <div className="h-1 w-8 rounded bg-white/50" />
      </div>
      <div
        className={cn(
          "absolute right-2.5 top-2.5 w-[82px] rounded-md border bg-white/90 p-2 shadow-sm dark:border-white/10 dark:bg-slate-900/90",
          centered && "left-1/2 right-auto top-9 -translate-x-1/2",
        )}
      >
        <div className="h-1.5 w-10 rounded bg-slate-300 dark:bg-slate-600" />
        <div className="mt-1.5 h-2.5 rounded border bg-white dark:border-slate-700 dark:bg-slate-950" />
        <div className="mt-1 h-2.5 rounded border bg-white dark:border-slate-700 dark:bg-slate-950" />
        <div className="mt-1.5 h-2.5 rounded" style={{ backgroundColor: template.accent }} />
      </div>
    </div>
  );
}

function ToggleRow({
  title,
  description,
  checked,
  onCheckedChange,
  disabled,
}: {
  title: string;
  description: string;
  checked: boolean;
  onCheckedChange: (value: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <div className="flex items-start justify-between gap-4 rounded-lg border p-4">
      <div>
        <p className="text-sm font-medium">{title}</p>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
    </div>
  );
}
