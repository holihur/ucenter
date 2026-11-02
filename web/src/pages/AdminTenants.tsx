import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, ExternalLink, Loader2, Plus, Power } from "lucide-react";
import { toast } from "sonner";
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
                    <StatusBadge status={tenant.status} />
                  </TableCell>
                  <TableCell className="text-sm">{tenant.users}</TableCell>
                  <TableCell className="text-sm">{tenant.clients}</TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                    {formatDate(tenant.createdAt)}
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={tenant.status === "active"}
                      onCheckedChange={() => void toggleStatus(tenant)}
                      disabled={!can("platform:manage")}
                    />
                  </TableCell>
                  <TableCell>
                    <Button variant="outline" size="sm" onClick={() => manage(tenant)}>
                      <ExternalLink className="h-3.5 w-3.5" />
                      {t("Manage")}
                    </Button>
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
