import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, CreditCard, ExternalLink } from "lucide-react";
import { toast } from "sonner";
import { useBilling } from "@/hooks/queries";
import { useAuth } from "@/auth/AuthContext";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

const STATUS_LABEL: Record<string, string> = {
  trialing: "Free trial",
  active: "Active",
  past_due: "Payment failed",
  canceled: "Canceled",
};

function fmtDate(iso: string | null): string {
  return iso ? new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" }) : "—";
}

export function BillingPage() {
  const { data, isLoading, refetch } = useBilling();
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const [busy, setBusy] = useState(false);

  // Handle the redirect back from Stripe Checkout.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const r = params.get("checkout");
    if (r === "success") {
      toast.success("Thanks — your subscription is being activated.");
      refetch();
    } else if (r === "cancel") {
      toast("Checkout canceled.");
    }
    if (r) window.history.replaceState({}, "", "/billing");
  }, [refetch]);

  async function go(kind: "checkout" | "portal") {
    setBusy(true);
    try {
      const res = kind === "checkout" ? await api.billingCheckout() : await api.billingPortal();
      window.location.href = res.url;
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not open Stripe");
      setBusy(false);
    }
  }

  const active = data?.status === "active" || data?.status === "past_due";

  return (
    <div className="mx-auto max-w-xl space-y-6">
      <Link to="/" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-4 w-4" />
        Back
      </Link>

      <div>
        <h1 className="text-2xl font-semibold">Billing</h1>
        <p className="text-sm text-muted-foreground">Manage your ChillCheck subscription.</p>
      </div>

      {isLoading && <p className="text-sm text-muted-foreground">Loading...</p>}

      {data && !data.billing_enabled && (
        <Card>
          <CardContent className="py-6 text-sm text-muted-foreground">
            Billing isn't enabled on this server, so all features are available without a subscription.
          </CardContent>
        </Card>
      )}

      {data && data.billing_enabled && (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle className="flex items-center gap-2">
                <CreditCard className="h-5 w-5 text-primary" />
                Subscription
              </CardTitle>
              <Badge variant={data.status === "active" ? "ok" : data.entitled ? "default" : "destructive"}>
                {STATUS_LABEL[data.status] ?? data.status}
              </Badge>
            </div>
            <CardDescription>
              {data.status === "trialing"
                ? `Trial ends ${fmtDate(data.trial_end)}.`
                : active
                  ? `Renews ${fmtDate(data.current_period_end)}.`
                  : "No active subscription."}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {data.plan && <p className="text-sm text-muted-foreground">Plan: {data.plan}</p>}
            <p className="text-xs text-muted-foreground">Billed per location.</p>
            {isAdmin ? (
              <div className="flex gap-2">
                {active ? (
                  <Button onClick={() => go("portal")} disabled={busy}>
                    <ExternalLink className="h-4 w-4" />
                    Manage billing
                  </Button>
                ) : (
                  <Button onClick={() => go("checkout")} disabled={busy}>
                    <CreditCard className="h-4 w-4" />
                    {busy ? "Opening..." : "Subscribe"}
                  </Button>
                )}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                Only an admin on your account can manage billing.
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
