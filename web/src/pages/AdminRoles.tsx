import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Lock, MoreHorizontal, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { EmptyState } from "@/components/EmptyState";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { api, type Permission, type Role } from "@/lib/api";
import { cn } from "@/lib/utils";

export function AdminRoles() {
  const { t } = useI18n();
  const { can } = useAuth();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Role | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<Role | null>(null);
  const [busy, setBusy] = useState(false);

  const rolesQuery = useQuery({
    queryKey: ["roles"],
    queryFn: () => api.get<{ items: Role[] }>("/admin/roles"),
  });
  const permissionsQuery = useQuery({
    queryKey: ["permissions"],
    queryFn: () => api.get<{ items: Permission[] }>("/admin/permissions"),
  });

  const permissions = permissionsQuery.data?.items ?? [];
  const permissionMap = useMemo(() => new Map(permissions.map((p) => [p.key, p])), [permissions]);
  const roles = rolesQuery.data?.items ?? [];

  function refresh() {
    queryClient.invalidateQueries({ queryKey: ["roles"] });
    queryClient.invalidateQueries({ queryKey: ["users"] });
  }

  async function onDelete() {
    if (!deleting) return;
    setBusy(true);
    try {
      await api.del(`/admin/roles/${deleting.id}`);
      toast.success(t("Role deleted"));
      setDeleting(null);
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to delete role");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Roles & permissions")}
        description={t("Define reusable access policies and assign them to tenant members.")}
        actions={
          can("roles:write") ? (
            <Button onClick={() => setCreating(true)}>
              <Plus className="h-4 w-4" />
              {t("New role")}
            </Button>
          ) : null
        }
      />

      {rolesQuery.isLoading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-52 w-full" />
          ))}
        </div>
      ) : roles.length ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {roles.map((role) => (
            <Card key={role.id} className="flex flex-col">
              <CardHeader className="flex-row items-start justify-between space-y-0 pb-3">
                <div className="flex items-center gap-2.5">
                  <div
                    className={cn(
                      "flex h-9 w-9 items-center justify-center rounded-lg",
                      role.key === "owner" ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground",
                    )}
                  >
                    {role.system ? <ShieldCheck className="h-4 w-4" /> : <Lock className="h-4 w-4" />}
                  </div>
                  <div>
                    <CardTitle className="text-base">{role.name}</CardTitle>
                    <p className="font-mono text-[11px] text-muted-foreground">{role.key}</p>
                  </div>
                </div>
                {can("roles:write") && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" className="h-8 w-8">
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setEditing(role)}>{t("Edit role")}</DropdownMenuItem>
                      {!role.system && (
                        <>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            className="text-destructive focus:text-destructive"
                            onSelect={() => setDeleting(role)}
                          >
                            <Trash2 className="h-4 w-4" />
                            {t("Delete role")}
                          </DropdownMenuItem>
                        </>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </CardHeader>
              <CardContent className="flex flex-1 flex-col gap-4">
                <CardDescription className="line-clamp-2 min-h-10">
                  {role.description || t("No description provided.")}
                </CardDescription>
                <div className="flex flex-wrap gap-1.5">
                  {role.system && <Badge variant="secondary">{t("System")}</Badge>}
                  <Badge variant="outline">{role.permissions.length} permissions</Badge>
                  {role.key === "owner" && <Badge variant="default">{t("Full access")}</Badge>}
                </div>
                <div className="mt-auto flex flex-wrap gap-1.5">
                  {role.permissions.slice(0, 4).map((p) => (
                    <span
                      key={p}
                      className="rounded-md bg-muted px-2 py-0.5 font-mono text-[10px] text-muted-foreground"
                    >
                      {p}
                    </span>
                  ))}
                  {role.permissions.length > 4 && (
                    <span className="rounded-md bg-muted px-2 py-0.5 text-[10px] text-muted-foreground">
                      +{role.permissions.length - 4} more
                    </span>
                  )}
                  {role.permissions.length === 0 && (
                    <span className="text-xs text-muted-foreground">{t("No administrative permissions")}</span>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <EmptyState
          icon={ShieldCheck}
          title={t("No roles yet")}
          description={t("Create a role to group permissions and assign it to users.")}
        />
      )}

      <RoleDialog
        open={creating || Boolean(editing)}
        onOpenChange={(open) => {
          if (!open) {
            setCreating(false);
            setEditing(null);
          }
        }}
        role={editing}
        permissions={permissions}
        permissionMap={permissionMap}
        onSaved={() => {
          refresh();
          setCreating(false);
          setEditing(null);
          toast.success(editing ? t("Role updated") : t("Role created"));
        }}
      />

      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t("Delete role?")}
        description={`The role "${deleting?.name}" will be permanently deleted. Roles assigned to users cannot be deleted.`}
        confirmLabel={t("Delete role")}
        destructive
        loading={busy}
        onConfirm={onDelete}
      />
    </div>
  );
}

function RoleDialog({
  open,
  onOpenChange,
  role,
  permissions,
  permissionMap,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  role: Role | null;
  permissions: Permission[];
  permissionMap: Map<string, Permission>;
  onSaved: () => void;
}) {
  const { t } = useI18n();
  const isEdit = Boolean(role);
  const [key, setKey] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setKey(role?.key ?? "");
    setName(role?.name ?? "");
    setDescription(role?.description ?? "");
    setSelected(role?.permissions ?? []);
  }, [open, role]);

  const groups = useMemo(() => {
    const map = new Map<string, Permission[]>();
    for (const p of permissions) {
      if (!map.has(p.group)) map.set(p.group, []);
      map.get(p.group)!.push(p);
    }
    return Array.from(map.entries());
  }, [permissions]);

  function toggle(permission: string) {
    setSelected((prev) =>
      prev.includes(permission) ? prev.filter((p) => p !== permission) : [...prev, permission],
    );
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      if (isEdit && role) {
        await api.patch(`/admin/roles/${role.id}`, { name, description, permissions: selected });
      } else {
        await api.post("/admin/roles", { key, name, description, permissions: selected });
      }
      onSaved();
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to save role");
    } finally {
      setSaving(false);
    }
  }

  const locked = Boolean(role?.system);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? t("Edit {name}", { name: role?.name ?? "" }) : t("Create role")}</DialogTitle>
          <DialogDescription>
            {locked
              ? t("System role permissions are fixed. You can still update the display name and description.")
              : t("Select the permissions members with this role should receive.")}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="role-key">{t("Key")}</Label>
              <Input
                id="role-key"
                value={key}
                onChange={(e) => setKey(e.target.value.toLowerCase())}
                disabled={isEdit}
                placeholder="support-agent"
                pattern="[a-z][a-z0-9_\-]{1,31}"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="role-name">{t("Display name")}</Label>
              <Input id="role-name" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="role-description">{t("Description")}</Label>
            <Textarea
              id="role-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t("What is this role responsible for?")}
              rows={2}
            />
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label>{t("Permissions")}</Label>
              <span className="text-xs text-muted-foreground">{selected.length} selected</span>
            </div>
            <div className="max-h-[320px] space-y-4 overflow-y-auto rounded-lg border p-4">
              {groups.map(([group, items]) => (
                <div key={t(group)}>
                  <p className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                    {t(group)}
                  </p>
                  <div className="grid gap-1.5 sm:grid-cols-2">
                    {items.map((permission) => (
                      <label
                        key={permission.key}
                        className={cn(
                          "flex cursor-pointer items-start gap-2 rounded-md p-1.5 hover:bg-muted/60",
                          locked && "cursor-not-allowed opacity-70",
                        )}
                      >
                        <Checkbox
                          checked={selected.includes(permission.key)}
                          onCheckedChange={() => !locked && toggle(permission.key)}
                          disabled={locked}
                          className="mt-0.5"
                        />
                        <span>
                          <span className="block text-sm font-medium">{t(permission.label)}</span>
                          <span className="block text-[11px] text-muted-foreground">
                            {t(permissionMap.get(permission.key)?.description ?? "")}
                          </span>
                        </span>
                      </label>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? t("Saving…") : isEdit ? t("Save changes") : t("Create role")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
