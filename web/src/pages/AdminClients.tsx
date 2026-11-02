import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AppWindow,
  Copy,
  Globe,
  KeyRound,
  MoreHorizontal,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyButton } from "@/components/CopyButton";
import { EmptyState } from "@/components/EmptyState";
import { PageHeader } from "@/components/PageHeader";
import { StatusBadge } from "@/components/StatusBadge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useAuth } from "@/lib/auth";
import { useAdminTenant } from "@/lib/useAdminTenant";
import { useI18n } from "@/lib/i18n";
import { api, type OidcClient } from "@/lib/api";
import { copyText } from "@/lib/utils";

interface ClientSecretInfo {
  name: string;
  clientId: string;
  secret: string;
}

export function AdminClients() {
  const { t } = useI18n();
  const { can } = useAuth();
  const tenant = useAdminTenant();
  const queryClient = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<OidcClient | null>(null);
  const [deleting, setDeleting] = useState<OidcClient | null>(null);
  const [rotating, setRotating] = useState<OidcClient | null>(null);
  const [secret, setSecret] = useState<ClientSecretInfo | null>(null);
  const [busy, setBusy] = useState(false);

  const query = useQuery({
    queryKey: ["clients"],
    queryFn: () => api.get<{ items: OidcClient[] }>("/admin/clients"),
  });

  const clients = query.data?.items ?? [];

  useEffect(() => {
    if (params.get("new") === "1") {
      setCreating(true);
      params.delete("new");
      setParams(params, { replace: true });
    }
  }, [params, setParams]);

  function refresh() {
    queryClient.invalidateQueries({ queryKey: ["clients"] });
    queryClient.invalidateQueries({ queryKey: ["admin-overview"] });
  }

  async function onDelete() {
    if (!deleting) return;
    setBusy(true);
    try {
      await api.del(`/admin/clients/${deleting.id}`);
      toast.success(t("Application deleted"));
      setDeleting(null);
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to delete application");
    } finally {
      setBusy(false);
    }
  }

  async function onRotate() {
    if (!rotating) return;
    setBusy(true);
    try {
      const res = await api.post<{ clientSecret: string }>(`/admin/clients/${rotating.id}/rotate-secret`);
      setSecret({ name: rotating.name, clientId: rotating.clientId, secret: res.clientSecret });
      setRotating(null);
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to rotate secret");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Applications")}
        description={t("OIDC clients that use this tenant as their identity provider.")}
        actions={
          can("clients:write") ? (
            <Button onClick={() => setCreating(true)}>
              <Plus className="h-4 w-4" />
              {t("New application")}
            </Button>
          ) : null
        }
      />

      <div className="rounded-xl border bg-muted/30 px-4 py-3 text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{t("Issuer:")}</span>{" "}
        <code className="break-all font-mono">{window.location.origin}/t/{tenant?.slug ?? "tenant"}</code>
        <span className="mx-2">·</span>
        <a
          className="text-primary hover:underline"
          href={`/t/${tenant?.slug ?? ""}/.well-known/openid-configuration`}
          target="_blank"
          rel="noreferrer"
        >
          {t("Open discovery document")}
        </a>
      </div>

      {query.isLoading ? (
        <div className="grid gap-4 md:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-64 w-full" />
          ))}
        </div>
      ) : clients.length ? (
        <div className="grid gap-4 lg:grid-cols-2">
          {clients.map((client) => (
            <Card key={client.id} className="flex flex-col">
              <CardHeader className="flex-row items-start justify-between space-y-0 pb-3">
                <div className="flex items-start gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <AppWindow className="h-5 w-5" />
                  </div>
                  <div className="min-w-0">
                    <CardTitle className="text-base">{client.name}</CardTitle>
                    <div className="mt-1 flex flex-wrap items-center gap-1.5">
                      <Badge variant={client.type === "confidential" ? "default" : "secondary"}>
                        {client.type}
                      </Badge>
                      <StatusBadge status={client.enabled ? "active" : "disabled"} />
                      {client.trusted && <Badge variant="success">{t("Trusted")}</Badge>}
                      {client.requirePkce && <Badge variant="outline">{t("PKCE")}</Badge>}
                    </div>
                  </div>
                </div>
                {can("clients:write") && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" className="h-8 w-8">
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setEditing(client)}>{t("Edit application")}</DropdownMenuItem>
                      {client.type === "confidential" && (
                        <DropdownMenuItem onSelect={() => setRotating(client)}>
                          <RefreshCw className="h-4 w-4" />
                          {t("Rotate secret")}
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        className="text-destructive focus:text-destructive"
                        onSelect={() => setDeleting(client)}
                      >
                        <Trash2 className="h-4 w-4" />
                        {t("Delete application")}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </CardHeader>
              <CardContent className="flex flex-1 flex-col gap-4">
                {client.description && (
                  <p className="text-sm text-muted-foreground">{client.description}</p>
                )}
                <div className="space-y-1.5">
                  <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">{t("Client ID")}</p>
                  <div className="flex items-center gap-2 rounded-md border bg-muted/30 px-2.5 py-1.5">
                    <code className="flex-1 truncate font-mono text-xs">{client.clientId}</code>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-6 w-6"
                      onClick={() => {
                        void copyText(client.clientId);
                        toast.success(t("Client ID copied"));
                      }}
                    >
                      <Copy className="h-3 w-3" />
                    </Button>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                    {t("Redirect URIs")}
                  </p>
                  <div className="space-y-1">
                    {(client.redirectUris ?? []).length ? (
                      (client.redirectUris ?? []).map((uri) => (
                        <div key={uri} className="flex items-center gap-1.5 text-xs text-muted-foreground">
                          <Globe className="h-3 w-3 shrink-0" />
                          <span className="truncate font-mono">{uri}</span>
                        </div>
                      ))
                    ) : (
                      <p className="text-xs text-muted-foreground">{t("No redirect URIs (client credentials only)")}</p>
                    )}
                  </div>
                </div>

                <div className="flex flex-wrap gap-1">
                  {(client.allowedScopes ?? []).map((scope) => (
                    <span
                      key={scope}
                      className="rounded-md bg-muted px-2 py-0.5 font-mono text-[10px] text-muted-foreground"
                    >
                      {scope}
                    </span>
                  ))}
                </div>
                <div className="mt-auto flex flex-wrap items-center gap-3 border-t pt-3 text-[11px] text-muted-foreground">
                  <span className="inline-flex items-center gap-1">
                    <ShieldCheck className="h-3 w-3" />
                    {(client.grantTypes ?? []).join(", ")}
                  </span>
                  {client.secretSet && (
                    <span className="inline-flex items-center gap-1">
                      <KeyRound className="h-3 w-3" />
                      {t("Secret configured")}
                    </span>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <EmptyState
          icon={AppWindow}
          title={t("No applications yet")}
          description={t("Register your first OIDC client to enable single sign-on.")}
          action={
            can("clients:write") ? (
              <Button onClick={() => setCreating(true)}>
                <Plus className="h-4 w-4" />
                {t("New application")}
              </Button>
            ) : null
          }
        />
      )}

      <ClientDialog
        open={creating || Boolean(editing)}
        onOpenChange={(open) => {
          if (!open) {
            setCreating(false);
            setEditing(null);
          }
        }}
        client={editing}
        onSaved={(client, clientSecret) => {
          refresh();
          setCreating(false);
          setEditing(null);
          if (clientSecret && client) {
            setSecret({ name: client.name, clientId: client.clientId, secret: clientSecret });
          } else {
            toast.success(editing ? t("Application updated") : t("Application created"));
          }
        }}
      />

      <Dialog open={Boolean(secret)} onOpenChange={(open) => !open && setSecret(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Client secret")}</DialogTitle>
            <DialogDescription>
              {t("Copy the secret for {name} now. It will not be shown again.", { name: secret?.name ?? "" })}
            </DialogDescription>
          </DialogHeader>
          {secret && (
            <div className="space-y-3">
              <div>
                <Label className="text-xs text-muted-foreground">{t("Client ID")}</Label>
                <div className="mt-1 flex items-center gap-2 rounded-lg border bg-muted/40 p-2.5">
                  <code className="flex-1 break-all font-mono text-xs">{secret.clientId}</code>
                  <CopyButton value={secret.clientId} label="" />
                </div>
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">{t("Client secret")}</Label>
                <div className="mt-1 flex items-center gap-2 rounded-lg border bg-muted/40 p-2.5">
                  <code className="flex-1 break-all font-mono text-xs">{secret.secret}</code>
                  <CopyButton value={secret.secret} label="" />
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                Store this value in your application&apos;s secret manager. Rotating the secret invalidates the old one
                immediately.
              </p>
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setSecret(null)}>{t("Done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t("Delete application?")}
        description={`"${deleting?.name}" will stop working immediately and all refresh tokens for this client will be revoked.`}
        confirmLabel={t("Delete application")}
        destructive
        loading={busy}
        onConfirm={onDelete}
      />

      <ConfirmDialog
        open={Boolean(rotating)}
        onOpenChange={(open) => !open && setRotating(null)}
        title={t("Rotate client secret?")}
        description={`The current secret for "${rotating?.name}" will stop working immediately. Update your application with the new secret.`}
        confirmLabel={t("Rotate secret")}
        loading={busy}
        onConfirm={onRotate}
      />
    </div>
  );
}

const scopeOptions = ["openid", "profile", "email", "offline_access", "roles"];
const grantOptions = [
  { value: "authorization_code", label: "Authorization code" },
  { value: "refresh_token", label: "Refresh token" },
  { value: "client_credentials", label: "Client credentials" },
];

function ClientDialog({
  open,
  onOpenChange,
  client,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  client: OidcClient | null;
  onSaved: (client?: OidcClient, secret?: string) => void;
}) {
  const { t } = useI18n();
  const isEdit = Boolean(client);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [type, setType] = useState<"confidential" | "public">("confidential");
  const [redirectUris, setRedirectUris] = useState("");
  const [postLogoutUris, setPostLogoutUris] = useState("");
  const [scopes, setScopes] = useState<string[]>(scopeOptions);
  const [customScope, setCustomScope] = useState("");
  const [grants, setGrants] = useState<string[]>(["authorization_code", "refresh_token"]);
  const [requirePkce, setRequirePkce] = useState(true);
  const [trusted, setTrusted] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [logoUrl, setLogoUrl] = useState("");
  const [accessTtl, setAccessTtl] = useState(3600);
  const [idTtl, setIdTtl] = useState(3600);
  const [refreshTtl, setRefreshTtl] = useState(2592000);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setName(client?.name ?? "");
    setDescription(client?.description ?? "");
    setType(client?.type ?? "confidential");
    setRedirectUris((client?.redirectUris ?? []).join("\n"));
    setPostLogoutUris((client?.postLogoutRedirectUris ?? []).join("\n"));
    setScopes(client?.allowedScopes ?? scopeOptions);
    setCustomScope("");
    setGrants(client?.grantTypes ?? ["authorization_code", "refresh_token"]);
    setRequirePkce(client?.requirePkce ?? true);
    setTrusted(client?.trusted ?? false);
    setEnabled(client?.enabled ?? true);
    setLogoUrl(client?.logoUrl ?? "");
    setAccessTtl(client?.accessTokenTtl ?? 3600);
    setIdTtl(client?.idTokenTtl ?? 3600);
    setRefreshTtl(client?.refreshTokenTtl ?? 2592000);
  }, [open, client]);

  const effectiveScopes = useMemo(() => {
    const extra = customScope
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    return Array.from(new Set([...scopes, ...extra]));
  }, [scopes, customScope]);

  function toggleScope(scope: string) {
    setScopes((prev) => (prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]));
  }

  function toggleGrant(grant: string) {
    setGrants((prev) => (prev.includes(grant) ? prev.filter((g) => g !== grant) : [...prev, grant]));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    const payload = {
      name,
      description,
      type,
      redirectUris: redirectUris
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean),
      postLogoutRedirectUris: postLogoutUris
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean),
      allowedScopes: effectiveScopes,
      grantTypes: grants,
      requirePkce: type === "public" ? true : requirePkce,
      trusted,
      enabled,
      logoUrl,
      accessTokenTtl: accessTtl,
      idTokenTtl: idTtl,
      refreshTokenTtl: refreshTtl,
    };
    try {
      if (isEdit && client) {
        const res = await api.patch<{ client: OidcClient; clientSecret: string }>(
          `/admin/clients/${client.id}`,
          payload,
        );
        onSaved(res.client, res.clientSecret || undefined);
      } else {
        const res = await api.post<{ client: OidcClient; clientSecret: string }>("/admin/clients", payload);
        onSaved(res.client, res.clientSecret || undefined);
      }
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to save application");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? `Edit ${client?.name}` : "Register application"}</DialogTitle>
          <DialogDescription>
            {t("Configure an OpenID Connect client. Redirect URIs must match exactly.")}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="client-name">{t("Name")}</Label>
              <Input id="client-name" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
            <div className="space-y-2">
              <Label>{t("Client type")}</Label>
              <Select
                value={type}
                onValueChange={(v) => {
                  setType(v as "confidential" | "public");
                  if (v === "public") setRequirePkce(true);
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="confidential">{t("Confidential (server-side)")}</SelectItem>
                  <SelectItem value="public">{t("Public (SPA / native)")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="client-description">{t("Description")}</Label>
            <Textarea
              id="client-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
              placeholder={t("What does this application do?")}
            />
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="redirect-uris">{t("Redirect URIs")}</Label>
              <Textarea
                id="redirect-uris"
                value={redirectUris}
                onChange={(e) => setRedirectUris(e.target.value)}
                rows={3}
                className="font-mono text-xs"
                placeholder={"https://app.example.com/callback\nhttp://localhost:3000/callback"}
              />
              <p className="text-[11px] text-muted-foreground">{t("One per line. HTTP is only allowed for localhost.")}</p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="logout-uris">{t("Post-logout redirect URIs")}</Label>
              <Textarea
                id="logout-uris"
                value={postLogoutUris}
                onChange={(e) => setPostLogoutUris(e.target.value)}
                rows={3}
                className="font-mono text-xs"
                placeholder={"https://app.example.com\nhttp://localhost:3000"}
              />
              <p className="text-[11px] text-muted-foreground">{t("Used by the OIDC end-session endpoint.")}</p>
            </div>
          </div>

          <div className="space-y-2">
            <Label>{t("Scopes")}</Label>
            <div className="flex flex-wrap gap-3 rounded-lg border p-3">
              {scopeOptions.map((scope) => (
                <label key={scope} className="flex cursor-pointer items-center gap-2 text-sm">
                  <Checkbox checked={scopes.includes(scope)} onCheckedChange={() => toggleScope(scope)} />
                  <span className="font-mono text-xs">{scope}</span>
                </label>
              ))}
            </div>
            <Input
              value={customScope}
              onChange={(e) => setCustomScope(e.target.value)}
              placeholder={t("Additional scopes, comma separated (optional)")}
              className="text-xs"
            />
            {effectiveScopes.length > 0 && (
              <p className="text-[11px] text-muted-foreground">Granted: {effectiveScopes.join(", ")}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label>{t("Grant types")}</Label>
            <div className="flex flex-wrap gap-4 rounded-lg border p-3">
              {grantOptions.map((grant) => (
                <label key={grant.value} className="flex cursor-pointer items-center gap-2 text-sm">
                  <Checkbox checked={grants.includes(grant.value)} onCheckedChange={() => toggleGrant(grant.value)} />
                  {t(grant.label)}
                </label>
              ))}
            </div>
          </div>

          <div className="grid gap-3 rounded-lg border p-4 sm:grid-cols-2">
            <label className="flex items-center justify-between gap-3 text-sm">
              <span>
                <span className="block font-medium">{t("Require PKCE")}</span>
                <span className="block text-xs text-muted-foreground">{t("Recommended for all clients")}</span>
              </span>
              <Switch
                checked={type === "public" ? true : requirePkce}
                onCheckedChange={setRequirePkce}
                disabled={type === "public"}
              />
            </label>
            <label className="flex items-center justify-between gap-3 text-sm">
              <span>
                <span className="block font-medium">{t("Trusted application")}</span>
                <span className="block text-xs text-muted-foreground">{t("Skip the consent screen")}</span>
              </span>
              <Switch checked={trusted} onCheckedChange={setTrusted} />
            </label>
            <label className="flex items-center justify-between gap-3 text-sm">
              <span>
                <span className="block font-medium">{t("Enabled")}</span>
                <span className="block text-xs text-muted-foreground">{t("Allow new sign-ins")}</span>
              </span>
              <Switch checked={enabled} onCheckedChange={setEnabled} />
            </label>
            <div className="space-y-1.5">
              <Label htmlFor="logo-url" className="text-xs">
                {t("Logo URL")}
              </Label>
              <Input
                id="logo-url"
                value={logoUrl}
                onChange={(e) => setLogoUrl(e.target.value)}
                placeholder="https://…"
                className="h-8 text-xs"
              />
            </div>
          </div>

          <details className="rounded-lg border p-4">
            <summary className="cursor-pointer text-sm font-medium">{t("Token lifetimes")}</summary>
            <div className="mt-3 grid gap-3 sm:grid-cols-3">
              {[
                { label: "Access token (s)", value: accessTtl, set: setAccessTtl },
                { label: "ID token (s)", value: idTtl, set: setIdTtl },
                { label: "Refresh token (s)", value: refreshTtl, set: setRefreshTtl },
              ].map((field) => (
                <div key={t(field.label)} className="space-y-1.5">
                  <Label className="text-xs">{t(field.label)}</Label>
                  <Input
                    type="number"
                    value={field.value}
                    onChange={(e) => field.set(Number(e.target.value))}
                    min={60}
                  />
                </div>
              ))}
            </div>
          </details>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? t("Saving…") : isEdit ? t("Save changes") : t("Create application")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
