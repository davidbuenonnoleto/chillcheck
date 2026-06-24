import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { ArrowLeft, Activity, AlertTriangle, ClipboardList, Download, FileWarning, Thermometer } from "lucide-react";
import { useAnalytics, useLocations } from "@/hooks/queries";
import { fetchAnalyticsCsv } from "@/lib/api";
import { fmtTemp } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { AnalyticsTrendChart } from "@/components/AnalyticsTrendChart";

const ALL = "all";

function fromDays(days: number): string {
  return new Date(Date.now() - days * 86400000).toISOString().slice(0, 10);
}

function pctColor(p: number): string {
  if (p >= 90) return "text-ok";
  if (p >= 70) return "text-warn";
  return "text-destructive";
}

function temp(v: number | null): string {
  return v == null ? "—" : fmtTemp(v);
}

function fmtDate(iso: string | null): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
}

export function AnalyticsPage() {
  const [days, setDays] = useState("30");
  const [location, setLocation] = useState(ALL);
  const [exporting, setExporting] = useState(false);

  const from = fromDays(Number(days));
  const locationId = location === ALL ? undefined : location;
  const { data, isLoading } = useAnalytics(from, undefined, locationId);
  const { data: locations } = useLocations();

  async function exportCsv() {
    setExporting(true);
    try {
      const blob = await fetchAnalyticsCsv(from, undefined, locationId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `analytics-${from}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not export the CSV.");
    } finally {
      setExporting(false);
    }
  }

  return (
    <div className="space-y-6">
      <Link to="/" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-4 w-4" />
        Back
      </Link>

      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Analytics</h1>
          <p className="text-sm text-muted-foreground">Compliance across your locations.</p>
        </div>
        <div className="flex items-center gap-2">
          <Select value={location} onValueChange={setLocation}>
            <SelectTrigger className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>All locations</SelectItem>
              {locations?.map((l) => (
                <SelectItem key={l.id} value={l.id}>
                  {l.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={days} onValueChange={setDays}>
            <SelectTrigger className="w-36">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="7">Last 7 days</SelectItem>
              <SelectItem value="30">Last 30 days</SelectItem>
              <SelectItem value="90">Last 90 days</SelectItem>
            </SelectContent>
          </Select>
          <Button variant="outline" size="sm" onClick={exportCsv} disabled={exporting || !data}>
            <Download className="h-4 w-4" />
            <span className="hidden sm:inline">{exporting ? "Exporting..." : "Export CSV"}</span>
          </Button>
        </div>
      </div>

      {isLoading && <p className="text-sm text-muted-foreground">Loading...</p>}

      {data && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <SummaryCard
              icon={<Activity className="h-5 w-5 text-primary" />}
              label="Readings in range"
              value={`${data.in_range_pct}%`}
              sub={`${data.total_readings} readings logged`}
              valueClass={pctColor(data.in_range_pct)}
            />
            <SummaryCard
              icon={<AlertTriangle className="h-5 w-5 text-primary" />}
              label="Deviations"
              value={`${data.deviations}`}
              sub={`${data.overdue_events} overdue event${data.overdue_events === 1 ? "" : "s"}`}
            />
            <SummaryCard
              icon={<FileWarning className="h-5 w-5 text-primary" />}
              label="Undocumented"
              value={`${data.undocumented_deviations}`}
              sub="out-of-range, no action logged"
              valueClass={data.undocumented_deviations > 0 ? "text-destructive" : "text-ok"}
            />
            <SummaryCard
              icon={<Thermometer className="h-5 w-5 text-primary" />}
              label="Units monitored"
              value={`${data.units.length}`}
              sub="in this view"
            />
          </div>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">In-range trend</CardTitle>
            </CardHeader>
            <CardContent>
              <AnalyticsTrendChart data={data.trend} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base flex items-center gap-2">
                <ClipboardList className="h-4 w-4" />
                By unit
              </CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Unit</TableHead>
                    <TableHead>Location</TableHead>
                    <TableHead className="text-right">Readings</TableHead>
                    <TableHead className="text-right">In range</TableHead>
                    <TableHead className="text-right">Deviations</TableHead>
                    <TableHead className="text-right">Avg / Min / Max</TableHead>
                    <TableHead className="text-right">Last reading</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.units.map((u) => (
                    <TableRow key={u.unit_id}>
                      <TableCell className="font-medium">{u.unit_name}</TableCell>
                      <TableCell className="text-muted-foreground">{u.location_name}</TableCell>
                      <TableCell className="text-right">{u.total_readings}</TableCell>
                      <TableCell className={`text-right font-medium ${u.total_readings ? pctColor(u.in_range_pct) : ""}`}>
                        {u.total_readings ? `${u.in_range_pct}%` : "—"}
                      </TableCell>
                      <TableCell className="text-right">{u.deviations}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {temp(u.avg_temp_f)} / {temp(u.min_temp_f)} / {temp(u.max_temp_f)}
                      </TableCell>
                      <TableCell className="text-right text-muted-foreground">{fmtDate(u.last_reading_at)}</TableCell>
                    </TableRow>
                  ))}
                  {data.units.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={7} className="py-6 text-center text-sm text-muted-foreground">
                        No units yet.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <p className="text-xs text-muted-foreground">
            Compliance is the share of readings within each unit's safe range. Daily trend buckets are in UTC.
          </p>
        </>
      )}
    </div>
  );
}

function SummaryCard({
  icon,
  label,
  value,
  sub,
  valueClass,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  sub: string;
  valueClass?: string;
}) {
  return (
    <Card>
      <CardContent className="space-y-1 py-4">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          {icon}
          {label}
        </div>
        <div className={`text-2xl font-semibold ${valueClass ?? ""}`}>{value}</div>
        <div className="text-xs text-muted-foreground">{sub}</div>
      </CardContent>
    </Card>
  );
}
