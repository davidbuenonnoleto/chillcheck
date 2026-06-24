import { useState } from "react";
import { toast } from "sonner";
import { useCorrectiveActions, useAddCorrectiveAction } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { fmtTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

export const ACTION_LABEL: Record<string, string> = {
  adjusted_equipment: "Adjusted / repaired equipment",
  relocated_product: "Relocated product",
  discarded_product: "Discarded product",
  other: "Other",
};

export const DISPOSITION_LABEL: Record<string, string> = {
  not_affected: "Product not affected",
  relocated: "Product relocated",
  discarded: "Product discarded",
};

export function CorrectiveActionDialog({
  alertId,
  unitName,
  open,
  onOpenChange,
}: {
  alertId: string;
  unitName: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const { data: actions } = useCorrectiveActions(alertId, open);
  const add = useAddCorrectiveAction(alertId);
  const [action, setAction] = useState("adjusted_equipment");
  const [disposition, setDisposition] = useState("not_affected");
  const [note, setNote] = useState("");

  async function submit() {
    try {
      await add.mutateAsync({ action, disposition, note });
      setNote("");
      toast.success("Corrective action logged");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not log action");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Corrective action</DialogTitle>
          <DialogDescription>{unitName} — document what was done about this deviation.</DialogDescription>
        </DialogHeader>

        {actions && actions.length > 0 && (
          <div className="space-y-2">
            {actions.map((ca) => (
              <div key={ca.id} className="rounded-md border p-2 text-sm">
                <div className="font-medium">
                  {ACTION_LABEL[ca.action] ?? ca.action} · {DISPOSITION_LABEL[ca.disposition] ?? ca.disposition}
                </div>
                {ca.note && <div className="text-muted-foreground">{ca.note}</div>}
                <div className="text-xs text-muted-foreground">
                  {ca.recorded_by_name} · {fmtTime(ca.created_at)}
                </div>
              </div>
            ))}
          </div>
        )}

        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>Action taken</Label>
            <Select value={action} onValueChange={setAction}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(ACTION_LABEL).map(([v, l]) => (
                  <SelectItem key={v} value={v}>
                    {l}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label>Product disposition</Label>
            <Select value={disposition} onValueChange={setDisposition}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(DISPOSITION_LABEL).map(([v, l]) => (
                  <SelectItem key={v} value={v}>
                    {l}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="ca-note">Note (optional)</Label>
            <textarea
              id="ca-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              rows={2}
              placeholder="e.g. moved product to walk-in, called for repair"
              className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          <Button onClick={submit} disabled={add.isPending} className="w-full">
            {add.isPending ? "Saving..." : "Log action"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
