import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AtSign,
  Bot,
  Building2,
  Fingerprint,
  Github,
  Globe,
  KeyRound,
  LogIn,
  MessageCircle,
  Wallet,
} from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { api, type IdentityProviderItem } from "@/lib/api";
import { cn } from "@/lib/utils";

const icons: Record<string, typeof Globe> = {
  wechat_open: MessageCircle,
  wechat_mp: MessageCircle,
  wecom: Building2,
  alipay: Wallet,
  dingtalk: MessageCircle,
  feishu: MessageCircle,
  qq: MessageCircle,
  weibo: AtSign,
  douyin: MessageCircle,
  google: Globe,
  microsoft: Globe,
  github: Github,
  apple: Globe,
  custom_oauth2: KeyRound,
  custom_oidc: Fingerprint,
  mock: Bot,
};

export function AdminIdentityProviders() {
  const { can } = useAuth();
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<IdentityProviderItem | null>(null);
  const [savingType, setSavingType] = useState("");
  const [category, setCategory] = useState("all");

  const query = useQuery({
    queryKey: ["identity-providers"],
    queryFn: () => api.get<{ items: IdentityProviderItem[] }>("/admin/identity-providers"),
  });
  const items = query.data?.items ?? [];
  const categories = useMemo(() => Array.from(new Set(items.map((i) => i.category))), [items]);
  const filtered = category === "all" ? items : items.filter((i) => i.category === category);

  async function toggle(item: IdentityProviderItem, enabled: boolean) {
    setSavingType(item.type);
    try {
      await api.put(`/admin/identity-providers/${item.type}`, {
        name: item.name,
        enabled,
        config: item.config,
        autoProvision: item.autoProvision,
        defaultRoles: item.defaultRoles,
        trustEmail: item.trustEmail,
      });
      toast.success(t("Identity provider updated"));
      queryClient.invalidateQueries({ queryKey: ["identity-providers"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to update identity provider"));
    } finally {
      setSavingType("");
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Identity providers")}
        description={t("Let members sign in with WeChat, Alipay, WeCom, social or a custom OIDC provider.")}
      />

      <div className="flex flex-wrap items-center gap-2">
        <Button variant={category === "all" ? "default" : "outline"} size="sm" onClick={() => setCategory("all")}>
          {t("All")}
        </Button>
        {categories.map((c) => (
          <Button key={c} variant={category === c ? "default" : "outline"} size="sm" onClick={() => setCategory(c)}>
            {c}
          </Button>
        ))}
        <Badge variant="muted" className="ms-auto">
          {t("Only configured and enabled providers are shown on the login page.")}
        </Badge>
      </div>

      {query.isLoading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-52 w-full" />)}
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filtered.map((item) => {
            const Icon = icons[item.type] ?? KeyRound;
            const configured = item.configured || item.fields.every((f) => !f.required || item.config[f.key]);
            return (
              <Card key={item.type} className={cn("flex flex-col", item.enabled && "border-primary/40")}>
                <CardHeader className="flex-row items-start justify-between space-y-0 pb-3">
                  <div className="flex items-start gap-3">
                    <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                      <Icon className="h-5 w-5" />
                    </div>
                    <div>
                      <CardTitle className="text-base">{item.name}</CardTitle>
                      <CardDescription className="mt-1 line-clamp-2 text-xs">{item.description}</CardDescription>
                    </div>
                  </div>
                  <Switch
                    checked={item.enabled}
                    onCheckedChange={(v) => void toggle(item, v)}
                    disabled={savingType === item.type || !can("tenant:write") || (!configured && !item.enabled)}
                  />
                </CardHeader>
                <CardContent className="flex flex-1 flex-col gap-3">
                  <div className="flex flex-wrap gap-1.5">
                    <Badge variant="outline">{item.category}</Badge>
                    <Badge variant="outline">{item.mode}</Badge>
                    <Badge variant={item.enabled ? "success" : configured ? "warning" : "muted"}>
                      {item.enabled ? t("Enabled") : configured ? t("Configured") : t("Not configured")}
                    </Badge>
                    {item.autoProvision && <Badge variant="secondary">{t("Auto create users")}</Badge>}
                  </div>
                  <div className="mt-auto flex items-center justify-between border-t pt-3">
                    <span className="text-[11px] text-muted-foreground">
                      {Object.values(item.config).filter(Boolean).length} {t("fields configured")}
                    </span>
                    <Button variant="outline" size="sm" onClick={() => setEditing(item)} disabled={!can("tenant:write")}>
                      <LogIn className="h-3.5 w-3.5" />
                      {t("Configure")}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <IdentityProviderDialog
        item={editing}
        onOpenChange={(open) => !open && setEditing(null)}
        onSaved={() => {
          queryClient.invalidateQueries({ queryKey: ["identity-providers"] });
          setEditing(null);
          toast.success(t("Identity provider saved"));
        }}
      />
    </div>
  );
}

function IdentityProviderDialog({
  item,
  onOpenChange,
  onSaved,
}: {
  item: IdentityProviderItem | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const { t } = useI18n();
  const [config, setConfig] = useState<Record<string, string>>({});
  const [enabled, setEnabled] = useState(false);
  const [autoProvision, setAutoProvision] = useState(false);
  const [trustEmail, setTrustEmail] = useState(false);
  const [roles, setRoles] = useState("member");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!item) return;
    setConfig({ ...item.config });
    setEnabled(item.enabled);
    setAutoProvision(item.autoProvision);
    setTrustEmail(item.trustEmail);
    setRoles((item.defaultRoles.length ? item.defaultRoles : ["member"]).join(", "));
  }, [item]);

  if (!item) return null;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      await api.put(`/admin/identity-providers/${item!.type}`, {
        name: item!.name,
        enabled,
        config,
        autoProvision,
        defaultRoles: roles.split(",").map((r) => r.trim()).filter(Boolean),
        trustEmail,
      });
      onSaved();
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to save identity provider"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={Boolean(item)} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{item.name}</DialogTitle>
          <DialogDescription>{item.description}</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            {item.fields.map((field) => (
              <div key={field.key} className="space-y-2">
                <Label htmlFor={`idp-${field.key}`}>
                  {field.label}
                  {field.required && <span className="text-destructive"> *</span>}
                </Label>
                <Input
                  id={`idp-${field.key}`}
                  type={field.secret ? "password" : "text"}
                  value={config[field.key] ?? ""}
                  onChange={(e) => setConfig({ ...config, [field.key]: e.target.value })}
                  placeholder={field.placeholder}
                  autoComplete="off"
                />
                {field.help && <p className="text-[11px] text-muted-foreground">{field.help}</p>}
              </div>
            ))}
          </div>

          <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-2">
            <label className="flex items-center justify-between gap-3 text-sm">
              <span>
                <span className="block font-medium">{t("Enabled")}</span>
                <span className="block text-xs text-muted-foreground">{t("Show on the login page")}</span>
              </span>
              <Switch checked={enabled} onCheckedChange={setEnabled} />
            </label>
            <label className="flex items-center justify-between gap-3 text-sm">
              <span>
                <span className="block font-medium">{t("Auto create users")}</span>
                <span className="block text-xs text-muted-foreground">{t("Create a member on first sign-in")}</span>
              </span>
              <Switch checked={autoProvision} onCheckedChange={setAutoProvision} />
            </label>
            <label className="flex items-center justify-between gap-3 text-sm">
              <span>
                <span className="block font-medium">{t("Trust provider email")}</span>
                <span className="block text-xs text-muted-foreground">{t("Allow linking by verified email")}</span>
              </span>
              <Switch checked={trustEmail} onCheckedChange={setTrustEmail} />
            </label>
            <div className="space-y-2">
              <Label htmlFor="idp-roles">{t("Default roles")}</Label>
              <Input id="idp-roles" value={roles} onChange={(e) => setRoles(e.target.value)} placeholder="member" />
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? t("Working…") : t("Save changes")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
