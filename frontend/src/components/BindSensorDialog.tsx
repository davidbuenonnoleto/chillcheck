import { useState } from "react";
import { Cpu } from "lucide-react";
import { toast } from "sonner";
import { useSetUnitSensor } from "@/hooks/queries";
import { ApiError, type Unit } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

export function BindSensorDialog({ unit, locationId }: { unit: Unit; locationId: string }) {
  const [open, setOpen] = useState(false);
  const [mac, setMac] = useState(unit.sensor_mac ?? "");
  const bind = useSetUnitSensor(locationId);

  async function save(value: string) {
    try {
      await bind.mutateAsync({ unitId: unit.id, mac: value });
      toast.success(value ? "Sensor linked" : "Sensor unlinked");
      setOpen(false);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not update sensor");
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (v) setMac(unit.sensor_mac ?? ""); }}>
      <DialogTrigger asChild>
        <button className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
          <Cpu className="h-3 w-3" />
          {unit.sensor_mac ? unit.sensor_mac : "Link sensor"}
        </button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Link a sensor</DialogTitle>
          <DialogDescription>
            Bind a Bluetooth sensor's MAC address to {unit.name}. The gateway will route this
            sensor's readings here.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            save(mac.trim());
          }}
          className="space-y-4"
        >
          <div className="space-y-1.5">
            <Label htmlFor="mac">Sensor MAC address</Label>
            <Input
              id="mac"
              value={mac}
              onChange={(e) => setMac(e.target.value)}
              placeholder="A4:C1:38:00:00:01"
              autoFocus
            />
          </div>
          <DialogFooter className="sm:justify-between">
            {unit.sensor_mac ? (
              <Button type="button" variant="ghost" onClick={() => save("")} disabled={bind.isPending}>
                Unlink
              </Button>
            ) : (
              <span />
            )}
            <Button type="submit" disabled={bind.isPending}>
              {bind.isPending ? "Saving..." : "Link sensor"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
