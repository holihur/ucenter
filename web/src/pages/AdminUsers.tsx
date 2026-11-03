import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronLeft,
  ChevronRight,
  KeyRound,
  Mail,
  MoreHorizontal,
  RefreshCw,
  Search,
  ShieldCheck,
  ShieldOff,
  Trash2,
  UserPlus,
  Users,
} from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyButton } from "@/components/CopyButton";
import { EmptyState } from "@/components/EmptyState";
import { PageHeader } from "@/components/PageHeader";
import { StatusBadge } from "@/components/StatusBadge";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAuth } from "@/lib/auth";
import { useAdminTenant } from "@/lib/useAdminTenant";
import { useI18n } from "@/lib/i18n";
import { api, type AdminUser, type Paginated, type Role } from "@/lib/api";
import { formatDate, initials, timeAgo } from "@/lib/utils";

interface SecretInfo {
  title: string;
  email: string;
  value: string;
  kind: "password" | "secret";
}

export function AdminUsers() {
  const { t } = useI18n();
  const { can } = useAuth();
  const tenant = useAdminTenant();
  const queryClient = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("all");
  const [role, setRole] = useState("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<AdminUser | null>(null);
  const [deleting, setDeleting] = useState<AdminUser | null>(null);
  const [resetting, setResetting] = useState<AdminUser | null>(null);
  const [revoking, setRevoking] = useState<AdminUser | null>(null);
  const [resettingMFA, setResettingMFA] = useState<AdminUser | null>(null);
  const [busy, setBusy] = useState(false);
  const [secret, setSecret] = useState<SecretInfo | null>(null);

  const rolesQuery = useQuery({
    queryKey: ["roles"],
    queryFn: () => api.get<{ items: Role[] }>("/admin/roles"),
  });
  const roles = rolesQuery.data?.items ?? [];

  const query = useQuery({
    queryKey: ["users", page, q, status, role],
    queryFn: () => {
      const sp = new URLSearchParams({ page: String(page), pageSize: "10" });
      if (q) sp.set("q", q);
      if (status !== "all") sp.set("status", status);
      if (role !== "all") sp.set("role", role);
      return api.get<Paginated<AdminUser>>(`/admin/users?${sp.toString()}`);
    },
  });

  useEffect(() => {
    const timer = setTimeout(() => {
      setQ(search.trim());
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  useEffect(() => {
    if (params.get("new") === "1") {
      setCreateOpen(true);
      params.delete("new");
      setParams(params, { replace: true });
    }
  }, [params, setParams]);

  const roleName = useMemo(() => {
    const map = new Map(roles.map((r) => [r.key, r.name]));
    return (key: string) => map.get(key) ?? key;
  }, [roles]);

  const totalPages = Math.max(1, Math.ceil((query.data?.total ?? 0) / 10));

  function refresh() {
    queryClient.invalidateQueries({ queryKey: ["users"] });
    queryClient.invalidateQueries({ queryKey: ["admin-overview"] });
  }

  async function onResetPassword() {
    if (!resetting) return;
    setBusy(true);
    try {
      const res = await api.post<{ tempPassword: string }>(`/admin/users/${resetting.id}/reset-password`);
      setSecret({ title: t("Temporary password"), email: resetting.email, value: res.tempPassword, kind: "password" });
      setResetting(null);
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to reset password"));
    } finally {
      setBusy(false);
    }
  }

  async function onDelete() {
    if (!deleting) return;
    setBusy(true);
    try {
      await api.del(`/admin/users/${deleting.id}`);
      toast.success(t("User removed"));
      setDeleting(null);
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to remove user"));
    } finally {
      setBusy(false);
    }
  }

  async function onResetMFA() {
    if (!resettingMFA) return;
    setBusy(true);
    try {
      await api.post(`/admin/users/${resettingMFA.id}/reset-mfa`);
      toast.success(t("MFA reset"));
      setResettingMFA(null);
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to reset MFA"));
    } finally {
      setBusy(false);
    }
  }

  async function sendInvite(user: AdminUser) {
    try {
      await api.post(`/admin/users/${user.id}/send-invite`);
      toast.success(t("Invitation email sent"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to send invitation"));
    }
  }

  async function onRevokeSessions() {
    if (!revoking) return;
    setBusy(true);
    try {
      await api.post(`/admin/users/${revoking.id}/revoke-sessions`);
      toast.success(t("Sessions revoked"));
      setRevoking(null);
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to revoke sessions"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Users")}
        description={`Manage members, roles and access for ${tenant?.name ?? "this tenant"}.`}
        actions={
          can("users:write") ? (
            <Button onClick={() => setCreateOpen(true)}>
              <UserPlus className="h-4 w-4" />
              {t("Invite user")}
            </Button>
          ) : null
        }
      />

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative flex-1">
          <Search className="absolute start-3 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            placeholder={t("Search by name, email or username…")}
            className="ps-9"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <Select
          value={status}
          onValueChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
        >
          <SelectTrigger className="w-full sm:w-40">
            <SelectValue placeholder={t("Status")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("All statuses")}</SelectItem>
            <SelectItem value="active">{t("Active")}</SelectItem>
            <SelectItem value="disabled">{t("Disabled")}</SelectItem>
            <SelectItem value="invited">{t("Invited")}</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={role}
          onValueChange={(v) => {
            setRole(v);
            setPage(1);
          }}
        >
          <SelectTrigger className="w-full sm:w-40">
            <SelectValue placeholder={t("Role")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("All roles")}</SelectItem>
            {roles.map((r) => (
              <SelectItem key={r.key} value={r.key}>
                {r.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="rounded-xl border bg-card shadow-soft">
        {query.isLoading ? (
          <div className="space-y-3 p-4">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-14 w-full" />
            ))}
          </div>
        ) : query.data?.items.length ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("User")}</TableHead>
                <TableHead>{t("Roles")}</TableHead>
                <TableHead>{t("Status")}</TableHead>
                <TableHead>{t("MFA")}</TableHead>
                <TableHead>{t("Last sign-in")}</TableHead>
                <TableHead>{t("Created")}</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {query.data.items.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>
                    <div className="flex items-center gap-3">
                      <Avatar className="h-9 w-9">
                        {user.picture && <AvatarImage src={user.picture} alt={user.name} />}
                        <AvatarFallback>{initials(user.name, user.email)}</AvatarFallback>
                      </Avatar>
                      <div className="min-w-0">
                        <p className="truncate font-medium">{user.name || user.email}</p>
                        <p className="truncate text-xs text-muted-foreground">{user.email}</p>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {user.roles.length ? (
                        user.roles.map((r) => (
                          <Badge key={r} variant={r === "owner" ? "default" : "secondary"}>
                            {roleName(r)}
                          </Badge>
                        ))
                      ) : (
                        <span className="text-xs text-muted-foreground">—</span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={user.status} />
                  </TableCell>
                  <TableCell>
                    {user.mfaEnabled ? (
                      <Badge variant="success">
                        <ShieldCheck className="mr-1 h-3 w-3" /> {t("Enabled")}
                      </Badge>
                    ) : (
                      <span className="text-xs text-muted-foreground">{t("Not enabled")}</span>
                    )}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                    {user.lastLoginAt ? timeAgo(user.lastLoginAt) : "Never"}
                  </TableCell>
                  <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                    {formatDate(user.createdAt)}
                  </TableCell>
                  <TableCell>
                    {can("users:write") && (
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon">
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onSelect={() => setEditing(user)}>{t("Edit user")}</DropdownMenuItem>
                          <DropdownMenuItem onSelect={() => setResetting(user)}>
                            <KeyRound className="h-4 w-4" />
                            {t("Reset password")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onSelect={() => setResettingMFA(user)}>
                            <ShieldOff className="h-4 w-4" />
                            {t("Reset MFA")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onSelect={() => void sendInvite(user)}>
                            <Mail className="h-4 w-4" />
                            {t("Send invite email")}
                          </DropdownMenuItem>
                          {can("sessions:write") && (
                            <DropdownMenuItem onSelect={() => setRevoking(user)}>
                              <RefreshCw className="h-4 w-4" />
                              {t("Revoke sessions")}
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            className="text-destructive focus:text-destructive"
                            onSelect={() => setDeleting(user)}
                          >
                            <Trash2 className="h-4 w-4" />
                            {t("Remove from tenant")}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <div className="p-6">
            <EmptyState
              icon={Users}
              title={t("No users found")}
              description={q ? t("Try a different search term or filter.") : t("Invite your first teammate to get started.")}
              action={
                can("users:write") ? (
                  <Button onClick={() => setCreateOpen(true)}>
                    <UserPlus className="h-4 w-4" />
                    {t("Invite user")}
                  </Button>
                ) : null
              }
            />
          </div>
        )}
      </div>

      {query.data && query.data.total > 0 && (
        <div className="flex items-center justify-between text-sm text-muted-foreground">
          <span>
            {(page - 1) * 10 + 1}–{Math.min(page * 10, query.data.total)} of {query.data.total}
          </span>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
              <ChevronLeft className="h-4 w-4" />
              {t("Previous")}
            </Button>
            <span className="text-xs">
              Page {page} of {totalPages}
            </span>
            <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
              {t("Next")}
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}

      <UserDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        roles={roles}
        onSaved={(result) => {
          refresh();
          if (result.tempPassword) {
            setSecret({
              title: "User invited",
              email: result.email,
              value: result.tempPassword,
              kind: "password",
            });
          } else {
            toast.success(t("User added to the organization"));
          }
        }}
      />

      <UserDialog
        open={Boolean(editing)}
        onOpenChange={(open) => !open && setEditing(null)}
        user={editing}
        roles={roles}
        onSaved={() => {
          refresh();
          setEditing(null);
          toast.success(t("User updated"));
        }}
      />

      <Dialog open={Boolean(secret)} onOpenChange={(open) => !open && setSecret(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{secret?.title}</DialogTitle>
            <DialogDescription>
              Share this {secret?.kind === "password" ? "temporary password" : "secret"} securely with{" "}
              <span className="font-medium text-foreground">{secret?.email}</span>. It will not be shown again.
            </DialogDescription>
          </DialogHeader>
          {secret && (
            <div className="space-y-3">
              <div className="flex items-center gap-2 rounded-lg border bg-muted/40 p-3">
                <code className="flex-1 break-all font-mono text-sm">{secret.value}</code>
                <CopyButton value={secret.value} />
              </div>
              <p className="text-xs text-muted-foreground">
                {t("The user should sign in and change this password immediately.")}
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
        title={t("Remove user from tenant?")}
        description={`${deleting?.email ?? "This user"} will lose access to this organization and all active sessions will be revoked.`}
        confirmLabel={t("Remove user")}
        destructive
        loading={busy}
        onConfirm={onDelete}
      />

      <ConfirmDialog
        open={Boolean(resetting)}
        onOpenChange={(open) => !open && setResetting(null)}
        title={t("Reset password?")}
        description={`A new temporary password will be generated for ${resetting?.email ?? "this user"} and all their sessions will be revoked.`}
        confirmLabel={t("Reset password")}
        loading={busy}
        onConfirm={onResetPassword}
      />

      <ConfirmDialog
        open={Boolean(resettingMFA)}
        onOpenChange={(open) => !open && setResettingMFA(null)}
        title={t("Reset MFA?")}
        description={t("This will disable TOTP and remove all passkeys for {email}.", { email: resettingMFA?.email ?? "" })}
        confirmLabel={t("Reset MFA")}
        destructive
        loading={busy}
        onConfirm={onResetMFA}
      />

      <ConfirmDialog
        open={Boolean(revoking)}
        onOpenChange={(open) => !open && setRevoking(null)}
        title={t("Revoke all sessions?")}
        description={`${revoking?.email ?? "This user"} will be signed out of every device and application.`}
        confirmLabel={t("Revoke sessions")}
        loading={busy}
        onConfirm={onRevokeSessions}
      />
    </div>
  );
}

function UserDialog({
  open,
  onOpenChange,
  user,
  roles,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user?: AdminUser | null;
  roles: Role[];
  onSaved: (result: { email: string; tempPassword?: string }) => void;
}) {
  const { t } = useI18n();
  const isEdit = Boolean(user);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [status, setStatus] = useState("active");
  const [selectedRoles, setSelectedRoles] = useState<string[]>(["member"]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setEmail(user?.email ?? "");
    setName(user?.name ?? "");
    setUsername(user?.username ?? "");
    setPassword("");
    setStatus(user?.status ?? "active");
    setSelectedRoles(user?.roles?.length ? [...user.roles] : ["member"]);
  }, [open, user]);

  function toggleRole(key: string) {
    setSelectedRoles((prev) => (prev.includes(key) ? prev.filter((r) => r !== key) : [...prev, key]));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      if (isEdit && user) {
        await api.patch(`/admin/users/${user.id}`, {
          name,
          username,
          status,
          roles: selectedRoles,
        });
        onSaved({ email: user.email });
      } else {
        const res = await api.post<{ user: AdminUser; tempPassword: string }>("/admin/users", {
          email,
          name,
          username,
          password: password || undefined,
          roles: selectedRoles,
        });
        onSaved({ email: res.user.email, tempPassword: res.tempPassword || undefined });
      }
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to save user"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? t("Edit user") : t("Invite a user")}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? t("Update profile details, status and tenant roles.")
              : t("Create an identity and add the user to this organization.")}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="user-email">{t("Email")}</Label>
              <Input
                id="user-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                disabled={isEdit}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="user-name">{t("Full name")}</Label>
              <Input id="user-name" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="user-username">{t("Username")}</Label>
              <Input
                id="user-username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder={t("Optional")}
              />
            </div>
            {isEdit ? (
              <div className="space-y-2">
                <Label>{t("Status")}</Label>
                <Select value={status} onValueChange={setStatus}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="active">{t("Active")}</SelectItem>
                    <SelectItem value="disabled">{t("Disabled")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            ) : (
              <div className="space-y-2">
                <Label htmlFor="user-password">{t("Password (optional)")}</Label>
                <Input
                  id="user-password"
                  type="text"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={t("Leave blank to auto-generate")}
                />
              </div>
            )}
          </div>

          <div className="space-y-2">
            <Label>{t("Roles")}</Label>
            <div className="grid gap-2 rounded-lg border p-3 sm:grid-cols-2">
              {roles.map((role) => (
                <label
                  key={role.key}
                  className="flex cursor-pointer items-start gap-2 rounded-md p-1.5 hover:bg-muted/60"
                >
                  <Checkbox
                    checked={selectedRoles.includes(role.key)}
                    onCheckedChange={() => toggleRole(role.key)}
                    className="mt-0.5"
                  />
                  <span>
                    <span className="block text-sm font-medium">{role.name}</span>
                    <span className="block text-xs text-muted-foreground">{role.description}</span>
                  </span>
                </label>
              ))}
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? t("Saving…") : isEdit ? t("Save changes") : t("Invite user")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
