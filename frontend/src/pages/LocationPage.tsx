import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, Plus, Thermometer, FileText, Clock } from "lucide-react";
import { toast } from "sonner";
import { useLocationStatus, useCreateUnit, useLogReading, useReadings } from "@/hooks/queries";
import { ApiError, fetchReportBlob, type Reading, type UnitKind, type UnitStatus } from "@/lib/api";
import { cn, fmtTemp, fmtTime, relativeTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { StatusBadge } from "@/components/StatusBadge";
import { TempTrendChart, type TrendPoint } from "@/components/TempTrendChart";
import { AlertsPanel } from "@/components/AlertsPanel";
import { GatewaysDialog } from "@/components/GatewaysDialog";
import { BindSensorDialog } from "@/components/BindSensorDialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

const KIND_LABEL: Record<UnitKind, string> = { fridge: "Fridge", freezer: "Freezer", hot_hold: "Hot holding" };
const KIND_DEFAULTS: Record<UnitKind, { min: number; max: number }> = {
  fridge: { min: 33, max: 40 },
  freezer: { min: -10, max: 10 },
  hot_hold: { min: 135, max: 165 },
};

type LocationTab = "board" | "readings" | "alerts";

const TABS: { key: LocationTab; label: string }[] = [
  { key: "board", label: "Board" },
  { key: "readings", label: "Readings" },
  { key: "alerts", label: "Alerts" },
];

export function LocationPage() {
  const { id = "" } = useParams();
  const { data, isLoading } = useLocationStatus(id);
  const [tab, setTab] = useState<LocationTab>("board");
  const units = data?.units ?? [];

  return (
    <div className="space-y-6">
      <Link to="/" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-4 w-4" />
        All locations
      </Link>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold">{data?.location.name ?? "Location"}</h1>
        <div className="flex gap-2">
          <GatewaysDialog locationId={id} />
          <AddUnitDialog locationId={id} />
          <ExportReportDialog locationId={id} />
        </div>
      </div>

      <div className="flex gap-6 border-b">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={cn(
              "-mb-px border-b-2 pb-2 text-sm font-medium transition-colors",
              tab === t.key
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "board" && <BoardTab locationId={id} units={units} isLoading={isLoading} />}
      {tab === "readings" && <ReadingsTab locationId={id} units={units} />}
      {tab === "alerts" && <AlertsPanel locationId={id} />}
    </div>
  );
}

// groupSeries turns the location's flat reading list into per-unit time series
// (oldest-first) for charting.
function groupSeries(readings?: Reading[]): Record<string, TrendPoint[]> {
  const out: Record<string, TrendPoint[]> = {};
  for (const r of readings ?? []) {
    (out[r.unit_id] ??= []).push({ t: new Date(r.recorded_at).getTime(), v: r.temp_f });
  }
  for (const k in out) out[k].sort((a, b) => a.t - b.t);
  return out;
}

function BoardTab({ locationId, units, isLoading }: { locationId: string; units: UnitStatus[]; isLoading: boolean }) {
  const { data: readings } = useReadings(locationId);
  const series = useMemo(() => groupSeries(readings), [readings]);

  if (isLoading) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <Card key={i}>
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between">
                <Skeleton className="h-4 w-28" />
                <Skeleton className="h-5 w-20 rounded-full" />
              </div>
              <Skeleton className="mt-2 h-3 w-36" />
            </CardHeader>
            <CardContent className="space-y-3">
              <Skeleton className="h-8 w-20" />
              <Skeleton className="h-3 w-32" />
              <Skeleton className="h-9 w-full" />
            </CardContent>
          </Card>
        ))}
      </div>
    );
  }

  if (units.length === 0) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-3 py-12 text-center">
          <Thermometer className="h-8 w-8 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">
            No units yet. Add a fridge, freezer, or hot-holding station to start logging.
          </p>
          <AddUnitDialog locationId={locationId} />
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {units.map((u) => (
        <UnitCard key={u.id} unit={u} locationId={locationId} series={series[u.id] ?? []} />
      ))}
    </div>
  );
}

function ReadingsTab({ locationId, units }: { locationId: string; units: UnitStatus[] }) {
  const { data: readings, isLoading } = useReadings(locationId);
  const series = useMemo(() => groupSeries(readings), [readings]);
  const ranges = useMemo(
    () => Object.fromEntries(units.map((u) => [u.id, { min: u.min_temp_f, max: u.max_temp_f }])),
    [units]
  );

  return (
    <div className="space-y-6">
      {units.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-2">
          {units.map((u) => (
            <Card key={u.id}>
              <CardHeader className="pb-2">
                <CardTitle className="text-base">{u.name}</CardTitle>
                <p className="text-xs text-muted-foreground">
                  Safe {u.min_temp_f}&ndash;{u.max_temp_f}&deg;F &middot; last 7 days
                </p>
              </CardHeader>
              <CardContent>
                <TempTrendChart data={series[u.id] ?? []} min={u.min_temp_f} max={u.max_temp_f} />
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Recent readings</CardTitle>
          <p className="text-xs text-muted-foreground">Last 7 days, newest first.</p>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-8 w-full" />
              ))}
            </div>
          ) : (
            <ReadingsTable readings={readings ?? []} ranges={ranges} />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function ReadingsTable({
  readings,
  ranges,
}: {
  readings: Reading[];
  ranges: Record<string, { min: number; max: number }>;
}) {
  if (readings.length === 0) {
    return <p className="py-6 text-center text-sm text-muted-foreground">No readings in the last 7 days.</p>;
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Unit</TableHead>
          <TableHead>Temp</TableHead>
          <TableHead>Source</TableHead>
          <TableHead>Recorded</TableHead>
          <TableHead>By</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {readings.slice(0, 100).map((r) => {
          const rg = ranges[r.unit_id];
          const bad = rg && (r.temp_f < rg.min || r.temp_f > rg.max);
          return (
            <TableRow key={r.id}>
              <TableCell className="font-medium">{r.unit_name}</TableCell>
              <TableCell className={cn("tabular-nums", bad && "font-semibold text-destructive")}>
                {fmtTemp(r.temp_f)}
              </TableCell>
              <TableCell className="text-muted-foreground">{r.source}</TableCell>
              <TableCell className="text-muted-foreground">{fmtTime(r.recorded_at)}</TableCell>
              <TableCell className="text-muted-foreground">
                {r.recorded_by ?? (r.source === "sensor" ? "Sensor" : "—")}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

function UnitCard({ unit, locationId, series }: { unit: UnitStatus; locationId: string; series: TrendPoint[] }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base">{unit.name}</CardTitle>
          <StatusBadge status={unit.status} />
        </div>
        <p className="text-xs text-muted-foreground">
          {KIND_LABEL[unit.kind]} &middot; safe {unit.min_temp_f}&ndash;{unit.max_temp_f}&deg;F
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-baseline gap-2">
          <span className="text-3xl font-semibold tabular-nums">
            {unit.latest_temp_f !== null ? fmtTemp(unit.latest_temp_f) : "\u2014"}
          </span>
        </div>
        <div className="flex items-center gap-1 text-xs text-muted-foreground">
          <Clock className="h-3 w-3" />
          {unit.latest_at ? `${relativeTime(unit.latest_at)}${unit.latest_by ? ` by ${unit.latest_by}` : ""}` : "No readings yet"}
        </div>
        {series.length >= 2 && <TempTrendChart data={series} min={unit.min_temp_f} max={unit.max_temp_f} compact />}
        <BindSensorDialog unit={unit} locationId={locationId} />
        <LogReadingDialog unit={unit} locationId={locationId} />
      </CardContent>
    </Card>
  );
}

function LogReadingDialog({ unit, locationId }: { unit: UnitStatus; locationId: string }) {
  const [open, setOpen] = useState(false);
  const [temp, setTemp] = useState("");
  const [note, setNote] = useState("");
  const log = useLogReading(locationId);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const tempF = parseFloat(temp);
    if (Number.isNaN(tempF)) {
      toast.error("Enter a temperature");
      return;
    }
    try {
      await log.mutateAsync({ unit_id: unit.id, temp_f: tempF, note: note || undefined });
      const outOfRange = tempF < unit.min_temp_f || tempF > unit.max_temp_f;
      if (outOfRange) {
        toast.warning(`Logged ${fmtTemp(tempF)} \u2014 outside the safe range`);
      } else {
        toast.success(`Logged ${fmtTemp(tempF)}`);
      }
      setTemp("");
      setNote("");
      setOpen(false);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not save reading");
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="w-full">
          <Thermometer className="h-4 w-4" />
          Log temp
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Log temperature</DialogTitle>
          <DialogDescription>
            {unit.name} &middot; safe range {unit.min_temp_f}&ndash;{unit.max_temp_f}&deg;F
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="temp">Temperature (&deg;F)</Label>
            <Input
              id="temp"
              type="number"
              step="0.1"
              inputMode="decimal"
              value={temp}
              onChange={(e) => setTemp(e.target.value)}
              autoFocus
              required
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="note">Note (optional)</Label>
            <Input id="note" value={note} onChange={(e) => setNote(e.target.value)} placeholder="e.g. door left open" />
          </div>
          <DialogFooter>
            <Button type="submit" disabled={log.isPending}>
              {log.isPending ? "Saving..." : "Save reading"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function AddUnitDialog({ locationId }: { locationId: string }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [kind, setKind] = useState<UnitKind>("fridge");
  const [min, setMin] = useState(String(KIND_DEFAULTS.fridge.min));
  const [max, setMax] = useState(String(KIND_DEFAULTS.fridge.max));
  const [interval, setInterval] = useState("240");
  const create = useCreateUnit(locationId);

  function onKindChange(k: UnitKind) {
    setKind(k);
    setMin(String(KIND_DEFAULTS[k].min));
    setMax(String(KIND_DEFAULTS[k].max));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    try {
      await create.mutateAsync({
        name,
        kind,
        min_temp_f: parseFloat(min),
        max_temp_f: parseFloat(max),
        log_interval_minutes: parseInt(interval, 10) || 240,
      });
      toast.success("Unit added");
      setName("");
      setOpen(false);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not add unit");
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline">
          <Plus className="h-4 w-4" />
          Add unit
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add a unit</DialogTitle>
          <DialogDescription>A fridge, freezer, or hot-holding station to monitor.</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="unit-name">Name</Label>
            <Input id="unit-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Walk-in cooler" required />
          </div>
          <div className="space-y-1.5">
            <Label>Type</Label>
            <Select value={kind} onValueChange={(v) => onKindChange(v as UnitKind)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="fridge">Fridge</SelectItem>
                <SelectItem value="freezer">Freezer</SelectItem>
                <SelectItem value="hot_hold">Hot holding</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="min">Safe min (&deg;F)</Label>
              <Input id="min" type="number" step="0.1" value={min} onChange={(e) => setMin(e.target.value)} required />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="max">Safe max (&deg;F)</Label>
              <Input id="max" type="number" step="0.1" value={max} onChange={(e) => setMax(e.target.value)} required />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="interval">Check every (minutes)</Label>
            <Input id="interval" type="number" value={interval} onChange={(e) => setInterval(e.target.value)} />
          </div>
          <DialogFooter>
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? "Adding..." : "Add unit"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function ExportReportDialog({ locationId }: { locationId: string }) {
  const today = new Date().toISOString().slice(0, 10);
  const weekAgo = new Date(Date.now() - 7 * 86400_000).toISOString().slice(0, 10);
  const [open, setOpen] = useState(false);
  const [from, setFrom] = useState(weekAgo);
  const [to, setTo] = useState(today);
  const [busy, setBusy] = useState(false);

  async function download() {
    setBusy(true);
    try {
      const blob = await fetchReportBlob(locationId, from, to);
      const url = URL.createObjectURL(blob);
      window.open(url, "_blank");
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
      setOpen(false);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Could not generate the report");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <FileText className="h-4 w-4" />
          Export report
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Compliance report</DialogTitle>
          <DialogDescription>An inspection-ready PDF of every reading in the range.</DialogDescription>
        </DialogHeader>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <Label htmlFor="from">From</Label>
            <Input id="from" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="to">To</Label>
            <Input id="to" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button onClick={download} disabled={busy}>
            {busy ? "Generating..." : "Open PDF"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
