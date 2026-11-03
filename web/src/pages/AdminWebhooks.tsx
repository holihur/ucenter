import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, MoreHorizontal, Plus, RefreshCw, Send, Trash2, Webhook } from "lucide-react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyButton } from "@/components/CopyButton";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { api } from "@/lib/api";
import { formatDate, timeAgo } from "@/lib/utils";

interface WebhookEndpoint {
  id: string;
  name: string;
  url: string;
  events: string[];
  enabled: boolean;
  secretSet: boolean;
  createdAt: string;
  updatedAt: string;
}

interface WebhookDelivery {
  id: string;
  event: string;
  status: string;
  attempts: number;
  nextAttemptAt: string;
  responseCode?: number;
  error?: string;
  createdAt: string;
  deliveredAt?: string;
  payload: Record<string, unknown>;
}

export function AdminWebhooks() {
  const { can } = useAuth();
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<WebhookEndpoint | null>(null);
  const [secret, setSecret] = useState<{ name: string; value: string } | null>(null);
  const [deleting, setDeleting] = useState<WebhookEndpoint | null>(null);
  const [rotating, setRotating] = useState<WebhookEndpoint | null>(null);
  const [selected, setSelected] = useState<WebhookDelivery | null>(null);
  const [status, setStatus] = useState("all");
  const [busy, setBusy] = useState(false);

  const query = useQuery({
    queryKey: ["webhooks"],
    queryFn: () => api.get<{ items: WebhookEndpoint[]; events: string[]; allowPrivate: boolean }>("/admin/webhooks"),
  });
  const deliveries = useQuery({
    queryKey: ["webhook-deliveries", status],
    queryFn: () => api.get<{ items: WebhookDelivery[]; total: number }>(`/admin/webhook-deliveries?pageSize=50${status !== "all" ? `&status=${status}` : ""}`),
    refetchInterval: 5000,
  });

  const endpoints = query.data?.items ?? [];
  const events = query.data?.events ?? [];
  const deliveriesItems = deliveries.data?.items ?? [];

  function refresh() {
    queryClient.invalidateQueries({ queryKey: ["webhooks"] });
    queryClient.invalidateQueries({ queryKey: ["webhook-deliveries"] });
  }

  async function test(endpoint: WebhookEndpoint) {
    try {
      await api.post(`/admin/webhooks/${endpoint.id}/test`);
      toast.success(t("Test delivery queued"));
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to queue test"));
    }
  }

  async function toggle(endpoint: WebhookEndpoint, enabled: boolean) {
    try {
      await api.patch(`/admin/webhooks/${endpoint.id}`, { enabled });
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to update webhook"));
    }
  }

  async function rotate() {
    if (!rotating) return;
    setBusy(true);
    try {
      const res = await api.post<{ secret: string }>(`/admin/webhooks/${rotating.id}/rotate-secret`);
      setSecret({ name: rotating.name, value: res.secret });
      setRotating(null);
      refresh();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to rotate secret"));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!deleting) return;
    setBusy(true);
    try {
      await api.del(`/admin/webhooks/${deleting.id}`);
      setDeleting(null);
      refresh();
      toast.success(t("Webhook deleted"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to delete webhook"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("Webhooks")}
        description={t("Send signed tenant events to your own HTTPS receiver with retries and delivery logs.")}
        actions={can("tenant:write") ? <Button onClick={() => setCreating(true)}><Plus className="h-4 w-4" />{t("New webhook")}</Button> : null}
      />

      {query.isLoading ? (
        <Skeleton className="h-52 w-full" />
      ) : endpoints.length ? (
        <div className="grid gap-4 lg:grid-cols-2">
          {endpoints.map((endpoint) => (
            <Card key={endpoint.id}>
              <CardHeader className="flex-row items-start justify-between space-y-0">
                <div className="flex items-start gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary"><Webhook className="h-5 w-5" /></div>
                  <div className="min-w-0">
                    <CardTitle className="text-base">{endpoint.name}</CardTitle>
                    <CardDescription className="mt-1 truncate font-mono text-xs">{endpoint.url}</CardDescription>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Switch checked={endpoint.enabled} onCheckedChange={(v) => void toggle(endpoint, v)} disabled={!can("tenant:write")} />
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="h-8 w-8"><MoreHorizontal className="h-4 w-4" /></Button></DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => setEditing(endpoint)}>{t("Edit webhook")}</DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => void test(endpoint)}><Send className="h-4 w-4" />{t("Send test event")}</DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => setRotating(endpoint)}><RefreshCw className="h-4 w-4" />{t("Rotate secret")}</DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem className="text-destructive focus:text-destructive" onSelect={() => setDeleting(endpoint)}><Trash2 className="h-4 w-4" />{t("Delete webhook")}</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex flex-wrap gap-1.5">
                  {endpoint.events.length ? endpoint.events.map((event) => <Badge key={event} variant="outline" className="font-mono text-[10px]">{event}</Badge>) : <Badge variant="muted">{t("All events")}</Badge>}
                </div>
                <div className="flex items-center justify-between border-t pt-2 text-[11px] text-muted-foreground">
                  <span>{t("Created {date}", { date: formatDate(endpoint.createdAt) })}</span>
                  <span className="inline-flex items-center gap-1"><Copy className="h-3 w-3" />{t("HMAC SHA-256 signed")}</span>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <EmptyState icon={Webhook} title={t("No webhooks yet")} description={t("Create a webhook to receive signed tenant events.")} />
      )}

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle className="text-base">{t("Delivery log")}</CardTitle>
            <CardDescription>{t("Failed deliveries retry with exponential backoff; dead letters stay visible here.")}</CardDescription>
          </div>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="w-40"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("All statuses")}</SelectItem>
              <SelectItem value="pending">pending</SelectItem>
              <SelectItem value="retrying">retrying</SelectItem>
              <SelectItem value="delivered">delivered</SelectItem>
              <SelectItem value="dead">dead</SelectItem>
            </SelectContent>
          </Select>
        </CardHeader>
        <CardContent className="p-0">
          {deliveries.isLoading ? (
            <div className="space-y-2 p-4">{Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}</div>
          ) : deliveriesItems.length ? (
            <Table>
              <TableHeader><TableRow><TableHead>{t("Event")}</TableHead><TableHead>{t("Status")}</TableHead><TableHead>{t("Attempts")}</TableHead><TableHead>{t("Response")}</TableHead><TableHead>{t("Created")}</TableHead><TableHead className="w-16" /></TableRow></TableHeader>
              <TableBody>
                {deliveriesItems.map((delivery) => (
                  <TableRow key={delivery.id}>
                    <TableCell className="font-mono text-xs">{delivery.event}</TableCell>
                    <TableCell><Badge variant={delivery.status === "delivered" ? "success" : delivery.status === "dead" ? "destructive" : delivery.status === "retrying" ? "warning" : "muted"}>{delivery.status}</Badge></TableCell>
                    <TableCell className="text-sm">{delivery.attempts}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">{delivery.responseCode ?? "—"}</TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">{timeAgo(delivery.createdAt)}</TableCell>
                    <TableCell><Button variant="ghost" size="sm" onClick={() => setSelected(delivery)}>{t("Details")}</Button></TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <div className="p-6"><EmptyState icon={Webhook} title={t("No deliveries yet")} description={t("Trigger an event or send a test webhook.")} /></div>
          )}
        </CardContent>
      </Card>

      <WebhookDialog open={creating || Boolean(editing)} onOpenChange={(open) => { if (!open) { setCreating(false); setEditing(null); } }} endpoint={editing} events={events} onSaved={(value) => { refresh(); setCreating(false); setEditing(null); if (value) setSecret(value); }} />

      <Dialog open={Boolean(secret)} onOpenChange={(open) => !open && setSecret(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("Webhook signing secret")}</DialogTitle><DialogDescription>{t("Store this secret now. It is shown only once and is used to verify X-UCenter-Signature.")}</DialogDescription></DialogHeader>
          {secret && <div className="flex items-center gap-2 rounded-lg border bg-muted/40 p-3"><code className="flex-1 break-all font-mono text-xs">{secret.value}</code><CopyButton value={secret.value} label="" /></div>}
          <DialogFooter><Button onClick={() => setSecret(null)}>{t("Done")}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{selected?.event}</DialogTitle><DialogDescription>{selected?.status} · {selected?.attempts} attempts</DialogDescription></DialogHeader>
          {selected && <div className="space-y-3 text-sm"><p><span className="text-muted-foreground">{t("Error")}:</span> {selected.error || "—"}</p><pre className="max-h-72 overflow-auto rounded-lg border bg-muted/40 p-3 text-xs">{JSON.stringify(selected.payload, null, 2)}</pre></div>}
        </DialogContent>
      </Dialog>

      <ConfirmDialog open={Boolean(deleting)} onOpenChange={(open) => !open && setDeleting(null)} title={t("Delete webhook?")} description={t("{name} will stop receiving events immediately.", { name: deleting?.name ?? "" })} confirmLabel={t("Delete webhook")} destructive loading={busy} onConfirm={remove} />
      <ConfirmDialog open={Boolean(rotating)} onOpenChange={(open) => !open && setRotating(null)} title={t("Rotate signing secret?")} description={t("The old secret stops working immediately. Update your receiver before rotating.")} confirmLabel={t("Rotate secret")} loading={busy} onConfirm={rotate} />
    </div>
  );
}

function WebhookDialog({
  open,
  onOpenChange,
  endpoint,
  events,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  endpoint: WebhookEndpoint | null;
  events: string[];
  onSaved: (secret?: { name: string; value: string }) => void;
}) {
  const { t } = useI18n();
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [selectedEvents, setSelectedEvents] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setName(endpoint?.name ?? "");
    setUrl(endpoint?.url ?? "");
    setEnabled(endpoint?.enabled ?? true);
    setSelectedEvents(endpoint?.events ?? []);
  }, [open, endpoint]);

  function toggleEvent(event: string) {
    setSelectedEvents((prev) => (prev.includes(event) ? prev.filter((e) => e !== event) : [...prev, event]));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      if (endpoint) {
        await api.patch(`/admin/webhooks/${endpoint.id}`, { name, url, events: selectedEvents, enabled });
        onSaved();
      } else {
        const res = await api.post<{ webhook: WebhookEndpoint; secret: string }>("/admin/webhooks", { name, url, events: selectedEvents, enabled });
        onSaved({ name: res.webhook.name, value: res.secret });
      }
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Unable to save webhook"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader><DialogTitle>{endpoint ? t("Edit webhook") : t("New webhook")}</DialogTitle><DialogDescription>{t("HTTPS endpoint, HMAC SHA-256 signature, exponential retry.")}</DialogDescription></DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="space-y-2"><Label htmlFor="webhook-name">{t("Name")}</Label><Input id="webhook-name" value={name} onChange={(e) => setName(e.target.value)} required /></div>
          <div className="space-y-2"><Label htmlFor="webhook-url">{t("Endpoint URL")}</Label><Input id="webhook-url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.com/hooks/ucenter" required /><p className="text-[11px] text-muted-foreground">{t("Private and loopback addresses are blocked unless configured for development.")}</p></div>
          <div className="space-y-2">
            <div className="flex items-center justify-between"><Label>{t("Events")}</Label><span className="text-xs text-muted-foreground">{selectedEvents.length || t("All events")}</span></div>
            <div className="grid max-h-64 gap-1 overflow-y-auto rounded-lg border p-3 sm:grid-cols-2">
              {events.map((event) => <label key={event} className="flex cursor-pointer items-center gap-2 rounded p-1 text-xs hover:bg-muted"><Checkbox checked={selectedEvents.includes(event)} onCheckedChange={() => toggleEvent(event)} /><span className="font-mono">{event}</span></label>)}
            </div>
          </div>
          <label className="flex items-center justify-between rounded-lg border p-3 text-sm"><span>{t("Enabled")}</span><Switch checked={enabled} onCheckedChange={setEnabled} /></label>
          <DialogFooter><Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>{t("Cancel")}</Button><Button type="submit" disabled={saving}>{saving ? t("Working…") : t("Save webhook")}</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
