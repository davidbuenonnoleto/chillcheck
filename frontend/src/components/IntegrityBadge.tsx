import { ShieldCheck, ShieldAlert } from "lucide-react";
import { useIntegrity } from "@/hooks/queries";

// IntegrityBadge surfaces the tamper-evident reading chain: a quiet "verified"
// reassurance, or a loud warning if the hash chain doesn't validate.
export function IntegrityBadge() {
  const { data } = useIntegrity();
  if (!data || data.count === 0) return null;

  if (data.ok) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-ok" title="Reading records are hash-chained and verify intact">
        <ShieldCheck className="h-3.5 w-3.5" />
        Records verified · {data.count} readings
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs font-medium text-destructive" title="The reading hash chain failed to verify">
      <ShieldAlert className="h-3.5 w-3.5" />
      Integrity check failed{data.broken_at_seq ? ` (entry #${data.broken_at_seq})` : ""}
    </span>
  );
}
