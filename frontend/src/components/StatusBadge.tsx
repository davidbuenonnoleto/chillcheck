import { Badge } from "@/components/ui/badge";
import type { UnitStatusValue } from "@/lib/api";

const MAP: Record<UnitStatusValue, { label: string; variant: "ok" | "warn" | "destructive" | "outline" }> = {
  ok: { label: "In range", variant: "ok" },
  out_of_range: { label: "Out of range", variant: "destructive" },
  overdue: { label: "Check overdue", variant: "warn" },
  no_data: { label: "No readings yet", variant: "outline" },
};

export function StatusBadge({ status }: { status: UnitStatusValue }) {
  const { label, variant } = MAP[status];
  return <Badge variant={variant}>{label}</Badge>;
}
