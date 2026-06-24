import { Link } from "react-router-dom";
import { useBilling } from "@/hooks/queries";

function daysLeft(iso: string | null): number | null {
  if (!iso) return null;
  return Math.max(0, Math.ceil((new Date(iso).getTime() - Date.now()) / 86400000));
}

export function BillingBanner() {
  const { data } = useBilling();
  if (!data || !data.billing_enabled || data.status === "active") return null;

  let tone = "bg-accent text-accent-foreground";
  let message = "";
  let cta = "Billing";

  if (!data.entitled) {
    tone = "bg-destructive text-destructive-foreground";
    message = "Your subscription is inactive. Subscribe to keep adding locations and units.";
    cta = "Subscribe";
  } else if (data.status === "past_due") {
    tone = "bg-warn text-warn-foreground";
    message = "A payment failed. Update your billing details to avoid interruption.";
    cta = "Update billing";
  } else if (data.status === "trialing") {
    const d = daysLeft(data.trial_end);
    message = d === null ? "You're on a free trial." : `Free trial — ${d} day${d === 1 ? "" : "s"} left.`;
    cta = "Subscribe";
  } else {
    return null;
  }

  return (
    <div className={`${tone} rounded-md px-4 py-2 text-sm`}>
      <div className="flex items-center justify-between gap-3">
        <span>{message}</span>
        <Link to="/billing" className="shrink-0 font-medium underline underline-offset-4">
          {cta}
        </Link>
      </div>
    </div>
  );
}
