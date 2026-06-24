import { useState } from "react";
import { AlertTriangle, CheckCircle2, ClipboardCheck, FileWarning } from "lucide-react";
import { useAlerts } from "@/hooks/queries";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CorrectiveActionDialog } from "@/components/CorrectiveActionDialog";
import { fmtTime } from "@/lib/utils";
import type { Alert } from "@/lib/api";

const KIND_LABEL: Record<Alert["kind"], string> = {
  out_of_range: "Out of range",
  overdue: "Check overdue",
};

export function AlertsPanel({ locationId }: { locationId: string }) {
  const { data: alerts } = useAlerts(locationId);
  if (!alerts || alerts.length === 0) return null;

  const open = alerts.filter((a) => a.status === "open");
  const recent = alerts.filter((a) => a.status === "resolved").slice(0, 5);

  return (
    <Card>
      <CardContent className="space-y-2 py-4">
        <div className="flex items-center gap-2 text-sm font-medium">
          <AlertTriangle className={open.length ? "h-4 w-4 text-destructive" : "h-4 w-4 text-muted-foreground"} />
          {open.length ? `${open.length} active alert${open.length > 1 ? "s" : ""}` : "No active alerts"}
        </div>
        <div className="divide-y">
          {open.map((a) => (
            <AlertRow key={a.id} alert={a} />
          ))}
          {recent.map((a) => (
            <AlertRow key={a.id} alert={a} resolved />
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function AlertRow({ alert, resolved }: { alert: Alert; resolved?: boolean }) {
  const [open, setOpen] = useState(false);
  const documented = alert.corrective_action_count > 0;
  return (
    <div className="flex items-center justify-between gap-2 py-2 text-sm">
      <div className="flex min-w-0 items-center gap-2">
        {resolved ? (
          <CheckCircle2 className="h-4 w-4 shrink-0 text-ok" />
        ) : (
          <AlertTriangle className="h-4 w-4 shrink-0 text-destructive" />
        )}
        <div className="min-w-0">
          <span className="font-medium">{alert.unit_name}</span>
          <span className="text-muted-foreground"> — {alert.detail}</span>
          <div className="text-xs text-muted-foreground">
            {!resolved && <span className="mr-2">{KIND_LABEL[alert.kind]}</span>}
            {resolved && alert.resolved_at ? `resolved ${fmtTime(alert.resolved_at)}` : fmtTime(alert.opened_at)}
          </div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {documented ? (
          <Badge variant="ok" className="gap-1">
            <ClipboardCheck className="h-3 w-3" />
            {alert.corrective_action_count}
          </Badge>
        ) : (
          <span className="hidden items-center gap-1 text-xs text-warn sm:flex">
            <FileWarning className="h-3 w-3" />
            Needs action
          </span>
        )}
        <Button size="sm" variant={documented ? "ghost" : "outline"} onClick={() => setOpen(true)}>
          {documented ? "View" : "Log action"}
        </Button>
      </div>
      <CorrectiveActionDialog alertId={alert.id} unitName={alert.unit_name} open={open} onOpenChange={setOpen} />
    </div>
  );
}
