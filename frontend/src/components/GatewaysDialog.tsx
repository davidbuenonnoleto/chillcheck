import { useState } from "react";
import { Radio, Plus, Copy, Check } from "lucide-react";
import { toast } from "sonner";
import { useGateways, useCreateGateway } from "@/hooks/queries";
import { ApiError } from "@/lib/api";
import { relativeTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

export function GatewaysDialog({ locationId }: { locationId: string }) {
  const [open, setOpen] = useState(false);
  const { data: gateways } = useGateways(locationId);
  const create = useCreateGateway(locationId);
  const [name, setName] = useState("");
  const [newKey, setNewKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const res = await create.mutateAsync({ name });
      setNewKey(res.key);
      setName("");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not create gateway");
    }
  }

  function copyKey() {
    if (!newKey) return;
    navigator.clipboard.writeText(newKey).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  function onOpenChange(v: boolean) {
    setOpen(v);
    if (!v) setNewKey(null);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger asChild>
        <Button variant="outline">
          <Radio className="h-4 w-4" />
          Gateways
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Sensor gateways</DialogTitle>
          <DialogDescription>
            A gateway is the on-site device that forwards Bluetooth sensor readings for this location.
          </DialogDescription>
        </DialogHeader>

        {newKey ? (
          <div className="space-y-3 rounded-md border border-warn/40 bg-warn/5 p-3">
            <p className="text-sm font-medium">Copy this gateway key now — it won't be shown again.</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 truncate rounded bg-muted px-2 py-1.5 text-xs">{newKey}</code>
              <Button size="icon" variant="outline" onClick={copyKey}>
                {copied ? <Check className="h-4 w-4 text-ok" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Put it in the gateway's <code>config.yaml</code> (or <code>CHILLCHECK_GATEWAY_KEY</code>).
            </p>
          </div>
        ) : (
          <form onSubmit={submit} className="flex items-end gap-2">
            <div className="flex-1 space-y-1.5">
              <Label htmlFor="gw-name">New gateway name</Label>
              <Input id="gw-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Kitchen Pi" required />
            </div>
            <Button type="submit" disabled={create.isPending}>
              <Plus className="h-4 w-4" />
              Add
            </Button>
          </form>
        )}

        <div className="divide-y">
          {gateways?.length === 0 && !newKey && (
            <p className="py-2 text-sm text-muted-foreground">No gateways yet.</p>
          )}
          {gateways?.map((g) => (
            <div key={g.id} className="flex items-center justify-between py-2 text-sm">
              <div>
                <div className="font-medium">{g.name}</div>
                <code className="text-xs text-muted-foreground">{g.key_prefix}…</code>
              </div>
              <span className="text-xs text-muted-foreground">
                {g.last_seen_at ? `seen ${relativeTime(g.last_seen_at)}` : "never connected"}
              </span>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
