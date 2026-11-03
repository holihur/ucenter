import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, ExternalLink, Loader2, Plus, Power, RotateCcw, Trash2, XCircle } from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyButton } from "@/components/CopyButton";
import { EmptyState } from "@/components/EmptyState";
import { PageHeader } from "@/components/PageHeader";
import { StatusBadge } from "@/components/StatusBadge";
import { TenantLogo } from "@/components/TenantLogo";
import { Button } from "@/components/ui/button";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { api, setAdminTenantId, type Branding } from "@/lib/api";
import { formatDate } from "@/lib/utils";

interface PlatformTenant {
  id: string;
  slug: string;
  name: string;
  status: string;
  domains: string[] | null;
  branding: Branding;
  users: number;
  clients: number;
  createdAt: string;
  deletedAt?: string | null;
  purgeAfter?: string | null;
}

export function AdminTenants() {
  const { t } = useI18n();
  const { can } = useAuth();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [tempPassword, setTempPassword] = useState<{ email: string; password: string; tenant: string } | null>(
    null,
  );
  const [action, setAction] = useState<{ tenant: PlatformTenant; kind: "delete" | "restore" | "purge" } | null>(null);
  const [actionBusy, setActionBusy] = useState(false);

  const query = useQuery({
    queryKey: ["admin-tenants"],
    queryFn: () => api.get<{ items: PlatformTenant[] }>("/admin/tenants"),
  });

  const tenants = query.data?.items ?? [];

  function manage(tenant: PlatformTenant) {
    setAdminTenantId(tenant.id);
    queryClient.invalidateQueries();
    navigate("/admin");
  }

  async function performAction() {
    if (!action) return;
    setActionBusy(true);
    try {
      if (action.kind === "delete") {
        await api.del(`/admin/tenants/${action.tenant.id}`);
        toast.success(t("Tenant disabled"));
      } else if (action.kind === "restore") {
        await api.post(`/admin/tenants/${action.tenant.id}/restore`, {});
        toast.success(t("Tenant restored"));
      } else {
        await api.post(`/admin/tenants/${action.tenant.id}/purge`, {});
        toast.success(t("Tenant purged"));
      }
      setAction(null);
      queryClient.invalidateQueries({ queryKey: ["admin-tenants"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to update tenant"));
    } finally {
      setActionBusy(false);
    }
  }

  async function toggleStatus(tenant: PlatformTenant) {
    const next = tenant.status === "active" ? "disabled" : "active";
    try {
      await api.patch(`/admin/tenants/${tenant.id}`, { status: next });
      toast.success(`${tenant.name}: ${next === "active" ? t("Enabled") : t("Disabled")}`);
      queryClient.invalidateQueries({ queryKey: ["admin-tenants"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to update tenant"));
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Tenants")}
        description={t("Organizations hosted on this UCenter instance. Each tenant has isolated users, roles and OIDC clients.")}
        actions={
          can("platform:manage") ? (
            <Button onClick={() => setCreating(true)}>
              <Plus className="h-4 w-4" />
              {t("New tenant")}
            </Button>
          ) : null
        }
      />

      <div className="rounded-xl border bg-card shadow-soft">
        {query.isLoading ? (
          <div className="space-y-3 p-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-16 w-full" />
            ))}
          </div>
        ) : tenants.length ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("Organization")}</TableHead>
                <TableHead>{t("Status")}</TableHead>
                <TableHead>{t("Users")}</TableHead>
                <TableHead>{t("Applications")}</TableHead>
                <TableHead>{t("Created")}</TableHead>
                <TableHead>{t("Enabled")}</TableHead>
                <TableHead className="w-28" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {tenants.map((tenant) => (
                <TableRow key={tenant.id}>
                  <TableCell>
                    <div className="flex items-center gap-3">
                      <TenantLogo tenant={tenant} className="h-9 w-9" />
                      <div>
                        <p className="font-medium">{tenant.name}</p>
                        <p className="font-mono text-xs text-muted-foreground">{tenant.slug}</p>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={tenant.deletedAt ? "deleted" : tenant.status} />
                    {tenant.purgeAfter && (
                      <p className="mt-1 text-[10px] text-muted-foreground">
                        {t("Purge after {date}", { date: formatDate(tenant.purgeAfter) })}
                      </p>
                    )}
                  </TableCell>
                  <TableCell className="text-sm">{tenant.users}</TableCell>
                  <TableCell className="text-sm">{tenant.clients}</TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {formatDate(tenant.createdAt)}
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={tenant.status === "active" && !tenant.deletedAt}
                      onCheckedChange={() => void toggleStatus(tenant)}
                      disabled={!can("platform:manage") || Boolean(tenant.deletedAt)}
                    />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <Button variant="outline" size="sm" onClick={() => manage(tenant)}>
                        <ExternalLink className="h-3.5 w-3.5" />
                        {t("Manage")}
                      </Button>
                      {tenant.deletedAt ? (
                        <>
                          <Button variant="ghost" size="sm" onClick={() => setAction({ tenant, kind: "restore" })}>
                            <RotateCcw className="h-3.5 w-3.5" />
                            {t("Restore")}
                          </Button>
                          <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setAction({ tenant, kind: "purge" })}>
                            <Trash2 className="h-3.5 w-3.5" />
                            {t("Purge")}
                          </Button>
                        </>
                      ) : (
                        <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setAction({ tenant, kind: "delete" })}>
                          <XCircle className="h-3.5 w-3.5" />
                          {t("Delete")}
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <div className="p-6">
            <EmptyState
              icon={Building2}
              title={t("No tenants yet")}
              description={t("Create the first organization to start onboarding users.")}
              action={
                <Button onClick={() => setCreating(true)}>
                  <Plus className="h-4 w-4" />
                  {t("New tenant")}
                </Button>
              }
            />
          </div>
        )}
      </div>

      <CreateTenantDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={(info) => {
          queryClient.invalidateQueries({ queryKey: ["admin-tenants"] });
          setCreating(false);
          if (info.password) {
            setTempPassword(info);
          } else {
            toast.success(t("Tenant created"));
          }
        }}
      />

      <ConfirmDialog
        open={Boolean(action)}
        onOpenChange={(open) => !open && setAction(null)}
        title={action?.kind === "delete" ? t("Disable tenant?") : action?.kind === "restore" ? t("Restore tenant?") : t("Permanently purge tenant?")}
        description={
          action?.kind === "delete"
            ? t("{name} will be disabled and scheduled for purge in 30 days.", { name: action.tenant.name })
            : action?.kind === "restore"
              ? t("{name} will be active again.", { name: action?.tenant.name ?? "" })
              : t("All users, clients, roles, sessions and audit events for {name} will be permanently deleted.", { name: action?.tenant.name ?? "" })
        }
        confirmLabel={action?.kind === "delete" ? t("Disable tenant") : action?.kind === "restore" ? t("Restore tenant") : t("Purge tenant")}
        destructive={action?.kind !== "restore"}
        loading={actionBusy}
        onConfirm={performAction}
      />

      <Dialog open={Boolean(tempPassword)} onOpenChange={(open) => !open && setTempPassword(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("Tenant owner created")}</DialogTitle>
            <DialogDescription>
              Share these credentials with the owner of {tempPassword?.tenant}. The password is shown only once.
            </DialogDescription>
          </DialogHeader>
          {tempPassword && (
            <div className="space-y-3">
              <div className="rounded-lg border bg-muted/40 p-3 text-sm">
                <p>
                  <span className="text-muted-foreground">{t("Email:")}</span> {tempPassword.email}
                </p>
                <div className="mt-2 flex items-center gap-2">
                  <code className="flex-1 break-all font-mono text-xs">{tempPassword.password}</code>
                  <CopyButton value={tempPassword.password} label="" />
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                {t("The owner should sign in and change their password immediately.")}
              </p>
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setTempPassword(null)}>{t("Done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function CreateTenantDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (info: { tenant: string; email: string; password: string }) => void;
}) {
  const { t } = useI18n();
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");
  const [ownerName, setOwnerName] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setSlug("");
    setName("");
    setOwnerEmail("");
    setOwnerName("");
  }, [open]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      const res = await api.post<{ tenant: PlatformTenant; tempPassword: string }>("/admin/tenants", {
        slug,
        name,
        ownerEmail: ownerEmail || undefined,
        ownerName: ownerName || undefined,
      });
      onCreated({ tenant: res.tenant.name, email: ownerEmail, password: res.tempPassword });
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to create tenant"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("Create tenant")}</DialogTitle>
          <DialogDescription>
            {t("A new isolated organization with owner/admin/member roles and its own signing key.")}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="tenant-name-new">{t("Organization name")}</Label>
              <Input id="tenant-name-new" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="tenant-slug-new">{t("Slug")}</Label>
              <Input
                id="tenant-slug-new"
                value={slug}
                onChange={(e) => setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ""))}
                placeholder="acme"
                required
              />
              <p className="text-[11px] text-muted-foreground">{t("Used in the OIDC issuer URL and cannot change.")}</p>
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="owner-email">{t("Owner email (optional)")}</Label>
              <Input
                id="owner-email"
                type="email"
                value={ownerEmail}
                onChange={(e) => setOwnerEmail(e.target.value)}
                placeholder="owner@acme.com"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="owner-name">{t("Owner name")}</Label>
              <Input
                id="owner-name"
                value={ownerName}
                onChange={(e) => setOwnerName(e.target.value)}
                placeholder="Ada Lovelace"
              />
            </div>
          </div>
          {ownerEmail && (
            <p className="rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground">
              {t("A temporary password will be generated for the owner and shown once after creation.")}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Power className="h-4 w-4" />}
              Create tenant
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
