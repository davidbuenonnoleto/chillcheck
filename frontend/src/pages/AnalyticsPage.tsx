import { useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, Activity, AlertTriangle, ClipboardCheck } from "lucide-react";
import { useAnalytics } from "@/hooks/queries";
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

function fromDays(days: number): string {
  return new Date(Date.now() - days * 86400000).toISOString().slice(0, 10);
}

function pctColor(p: number): string {
  if (p >= 90) return "text-ok";
  if (p >= 70) return "text-warn";
  return "text-destructive";
}

export function AnalyticsPage() {
  const [days, setDays] = useState("7");
  const { data, isLoading } = useAnalytics(fromDays(Number(days)));

  return (
    <div className="space-y-6">
      <Link to="/" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-4 w-4" />
        Back
      </Link>

      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-2xl font-semibold">Analytics</h1>
          <p className="text-sm text-muted-foreground">Compliance across your locations.</p>
        </div>
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
      </div>

      {isLoading && <p className="text-sm text-muted-foreground">Loading...</p>}

      {data && (
        <>
          <div className="grid gap-3 sm:grid-cols-3">
            <SummaryCard
              icon={<Activity className="h-5 w-5 text-primary" />}
              label="Logging compliance"
              value={`${data.compliance_pct}%`}
              sub={`${data.readings} of ~${data.expected_logs} expected logs`}
              valueClass={pctColor(data.compliance_pct)}
            />
            <SummaryCard
              icon={<ClipboardCheck className="h-5 w-5 text-primary" />}
              label="Deviations documented"
              value={`${data.documentation_pct}%`}
              sub={`${data.documented_deviations} of ${data.deviations} deviations`}
              valueClass={pctColor(data.documentation_pct)}
            />
            <SummaryCard
              icon={<AlertTriangle className="h-5 w-5 text-primary" />}
              label="Out-of-range readings"
              value={`${data.out_of_range}`}
              sub={`across ${data.locations.length} location${data.locations.length === 1 ? "" : "s"}`}
            />
          </div>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">By location</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Location</TableHead>
                    <TableHead className="text-right">Units</TableHead>
                    <TableHead className="text-right">Logged / expected</TableHead>
                    <TableHead className="text-right">Compliance</TableHead>
                    <TableHead className="text-right">Out of range</TableHead>
                    <TableHead className="text-right">Deviations</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.locations.map((l) => (
                    <TableRow key={l.location_id}>
                      <TableCell className="font-medium">{l.location_name}</TableCell>
                      <TableCell className="text-right">{l.units}</TableCell>
                      <TableCell className="text-right">
                        {l.readings} / ~{l.expected_logs}
                      </TableCell>
                      <TableCell className={`text-right font-medium ${pctColor(l.compliance_pct)}`}>
                        {l.compliance_pct}%
                      </TableCell>
                      <TableCell className="text-right">{l.out_of_range}</TableCell>
                      <TableCell className="text-right">
                        {l.documented_deviations}/{l.deviations} documented
                      </TableCell>
                    </TableRow>
                  ))}
                  {data.locations.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={6} className="py-6 text-center text-sm text-muted-foreground">
                        No locations yet.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <p className="text-xs text-muted-foreground">
            Expected logs are estimated from each unit's logging interval; compliance is capped at 100%.
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
