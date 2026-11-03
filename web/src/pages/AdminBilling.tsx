import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUpRight, Check, CreditCard, Gauge, Receipt, ShieldCheck, Users } from "lucide-react";
import { toast } from "sonner";
import { EmptyState } from "@/components/EmptyState";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAuth } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";
import { api } from "@/lib/api";
import { cn, formatDate } from "@/lib/utils";

interface Plan {
  key: string;
  name: string;
  description: string;
  priceCents: number;
  currency: string;
  limits: { users: number; clients: number; monthlyApi: number; monthlyEmails: number };
  features: string[];
}

interface Subscription {
  plan: string;
  status: string;
  provider: string;
  currentPeriodStart: string;
  currentPeriodEnd: string;
  cancelAtPeriodEnd: boolean;
}

interface Invoice {
  id: string;
  number: string;
  plan: string;
  amountCents: number;
  currency: string;
  status: string;
  createdAt: string;
}

interface BillingData {
  subscription: Subscription;
  plan: Plan;
  plans: Plan[];
  limits: Plan["limits"];
  usage: Record<string, number>;
  counts: { users: number; clients: number };
  invoices: Invoice[];
  periodStart: string;
}

export function AdminBilling() {
  const { t } = useI18n();
  const { can } = useAuth();
  const queryClient = useQueryClient();
  const [busy, setBusy] = useState("");
  const query = useQuery({ queryKey: ["billing"], queryFn: () => api.get<BillingData>("/admin/billing") });
  const data = query.data;
  const editable = can("tenant:write");

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("checkout") === "success") toast.success(t("Plan updated"));
  }, []);

  async function act(action: "checkout" | "cancel" | "resume", plan?: string) {
    setBusy(plan ?? action);
    try {
      if (action === "checkout") {
        await api.post("/admin/billing/checkout", { plan });
        toast.success(t("Plan updated"));
      } else {
        await api.post(`/admin/billing/${action}`);
        toast.success(action === "cancel" ? t("Cancellation scheduled") : t("Subscription resumed"));
      }
      await queryClient.invalidateQueries({ queryKey: ["billing"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("Billing update failed"));
    } finally {
      setBusy("");
    }
  }

  if (query.isLoading || !data) return <div className="space-y-4"><Skeleton className="h-40 w-full" /><Skeleton className="h-64 w-full" /></div>;

  const usage = data.usage;
  const meters = [
    { label: t("Users"), value: data.counts.users, limit: data.limits.users, icon: Users },
    { label: t("Applications"), value: data.counts.clients, limit: data.limits.clients, icon: ShieldCheck },
    { label: t("API requests this month"), value: usage.api_requests ?? 0, limit: data.limits.monthlyApi, icon: Gauge },
    { label: t("Emails this month"), value: usage.emails ?? 0, limit: data.limits.monthlyEmails, icon: CreditCard },
  ];

  return (
    <div className="space-y-6">
      <PageHeader title={t("Billing & usage")} description={t("Plan limits, metered usage and subscription status for this tenant.")} />

      <div className="grid gap-4 lg:grid-cols-[1.2fr_1fr]">
        <Card>
          <CardHeader className="flex-row items-start justify-between space-y-0">
            <div>
              <CardDescription>{t("Current plan")}</CardDescription>
              <CardTitle className="mt-1 flex items-center gap-2 text-2xl">{data.plan.name}<Badge variant={data.subscription.status === "active" ? "success" : "warning"}>{data.subscription.status}</Badge></CardTitle>
              <p className="mt-1 text-sm text-muted-foreground">{data.plan.description}</p>
            </div>
            <Badge variant="outline" className="font-mono text-[10px]">{data.subscription.provider || "mock"}</Badge>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap gap-x-8 gap-y-2 text-sm">
              <div><span className="text-muted-foreground">{t("Price")}: </span>{data.plan.priceCents === 0 ? t("Free") : `$${(data.plan.priceCents / 100).toFixed(0)}/${t("month")}`}</div>
              <div><span className="text-muted-foreground">{t("Renews")}: </span>{formatDate(data.subscription.currentPeriodEnd)}</div>
              {data.subscription.cancelAtPeriodEnd && <Badge variant="warning">{t("Cancels at period end")}</Badge>}
            </div>
            {editable && (
              <div className="flex flex-wrap gap-2">
                {data.subscription.cancelAtPeriodEnd ? (
                  <Button variant="outline" size="sm" disabled={busy === "resume"} onClick={() => void act("resume")}>{t("Resume subscription")}</Button>
                ) : (
                  <Button variant="outline" size="sm" disabled={busy === "cancel"} onClick={() => void act("cancel")}>{t("Cancel at period end")}</Button>
                )}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="text-base">{t("This month")}</CardTitle><CardDescription>{t("Metered from {date}", { date: formatDate(data.periodStart) })}</CardDescription></CardHeader>
          <CardContent className="space-y-4">
            {meters.map((meter) => {
              const pct = meter.limit > 0 ? Math.min(100, Math.round((meter.value / meter.limit) * 100)) : 0;
              return (
                <div key={meter.label} className="space-y-1.5">
                  <div className="flex items-center justify-between text-xs"><span className="inline-flex items-center gap-1.5"><meter.icon className="h-3.5 w-3.5 text-muted-foreground" />{meter.label}</span><span className="text-muted-foreground">{meter.value.toLocaleString()} / {meter.limit.toLocaleString()}</span></div>
                  <div className="h-2 w-full overflow-hidden rounded-full bg-muted"><div className={cn("h-full rounded-full bg-primary transition-all", pct >= 90 && "bg-destructive")} style={{ width: `${pct}%` }} /></div>
                </div>
              );
            })}
          </CardContent>
        </Card>
      </div>

      <div>
        <h2 className="mb-3 text-sm font-semibold">{t("Plans")}</h2>
        <div className="grid gap-4 md:grid-cols-3">
          {data.plans.map((plan) => {
            const current = plan.key === data.subscription.plan;
            return (
              <Card key={plan.key} className={cn("relative", current && "ring-2 ring-primary")}>
                {current && <Badge className="absolute end-3 top-3">{t("Current")}</Badge>}
                <CardHeader><CardTitle className="text-lg">{plan.name}</CardTitle><CardDescription>{plan.description}</CardDescription></CardHeader>
                <CardContent className="space-y-4">
                  <div className="text-3xl font-semibold tracking-tight">{plan.priceCents === 0 ? t("Free") : `$${(plan.priceCents / 100).toFixed(0)}`}<span className="text-sm font-normal text-muted-foreground">{plan.priceCents === 0 ? "" : `/${t("month")}`}</span></div>
                  <ul className="space-y-1.5 text-xs text-muted-foreground">
                    <li className="flex items-center gap-1.5"><Check className="h-3.5 w-3.5 text-primary" />{t("{count} users", { count: plan.limits.users.toLocaleString() })}</li>
                    <li className="flex items-center gap-1.5"><Check className="h-3.5 w-3.5 text-primary" />{t("{count} applications", { count: plan.limits.clients.toLocaleString() })}</li>
                    <li className="flex items-center gap-1.5"><Check className="h-3.5 w-3.5 text-primary" />{t("{count} API requests / month", { count: plan.limits.monthlyApi.toLocaleString() })}</li>
                    {plan.features.map((f) => <li key={f} className="flex items-center gap-1.5"><Check className="h-3.5 w-3.5 text-primary" />{f}</li>)}
                  </ul>
                  <Button className="w-full" variant={current ? "outline" : "default"} disabled={!editable || current || busy === plan.key} onClick={() => void act("checkout", plan.key)}>
                    {current ? t("Current plan") : <>{t("Switch to {plan}", { plan: plan.name })}<ArrowUpRight className="h-4 w-4" /></>}
                  </Button>
                </CardContent>
              </Card>
            );
          })}
        </div>
      </div>

      <Card>
        <CardHeader><CardTitle className="flex items-center gap-2 text-base"><Receipt className="h-4 w-4" />{t("Invoices")}</CardTitle></CardHeader>
        <CardContent className="p-0">
          {data.invoices.length ? (
            <Table>
              <TableHeader><TableRow><TableHead>{t("Invoice")}</TableHead><TableHead>{t("Plan")}</TableHead><TableHead>{t("Amount")}</TableHead><TableHead>{t("Status")}</TableHead><TableHead>{t("Date")}</TableHead></TableRow></TableHeader>
              <TableBody>{data.invoices.map((invoice) => (
                <TableRow key={invoice.id}><TableCell className="font-mono text-xs">{invoice.number}</TableCell><TableCell className="capitalize">{invoice.plan}</TableCell><TableCell>${(invoice.amountCents / 100).toFixed(2)} {invoice.currency.toUpperCase()}</TableCell><TableCell><Badge variant="success">{invoice.status}</Badge></TableCell><TableCell className="text-xs text-muted-foreground">{formatDate(invoice.createdAt)}</TableCell></TableRow>
              ))}</TableBody>
            </Table>
          ) : <div className="p-6"><EmptyState icon={Receipt} title={t("No invoices yet")} description={t("Invoices appear after a paid plan change.")} /></div>}
        </CardContent>
      </Card>
    </div>
  );
}
