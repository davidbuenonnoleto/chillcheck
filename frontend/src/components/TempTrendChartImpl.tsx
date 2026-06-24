import {
  Line,
  LineChart,
  ReferenceArea,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { fmtTemp, fmtTime } from "@/lib/utils";
import type { TrendPoint } from "./TempTrendChart";

// Renders a unit's recent temperatures with its safe range shaded. `compact`
// draws an axis-less sparkline for board cards; otherwise a full chart with axes
// + tooltip. The line uses the chart-1 token so it themes with light/dark.
// Lazy-loaded via TempTrendChart so recharts stays out of the main bundle.
export default function TempTrendChartImpl({
  data,
  min,
  max,
  compact = false,
}: {
  data: TrendPoint[];
  min: number;
  max: number;
  compact?: boolean;
}) {
  if (data.length < 2) {
    if (compact) return null;
    return <p className="py-8 text-center text-sm text-muted-foreground">Not enough readings to chart yet.</p>;
  }

  const temps = data.map((d) => d.v);
  const lo = Math.min(min, ...temps);
  const hi = Math.max(max, ...temps);
  const pad = Math.max(2, (hi - lo) * 0.12);
  const domain: [number, number] = [Math.floor(lo - pad), Math.ceil(hi + pad)];

  return (
    <ResponsiveContainer width="100%" height={compact ? 56 : 220}>
      <LineChart data={data} margin={compact ? { top: 4, right: 6, bottom: 0, left: 6 } : { top: 8, right: 12, bottom: 4, left: -8 }}>
        <ReferenceArea y1={min} y2={max} fill="hsl(var(--ok))" fillOpacity={0.12} stroke="none" />
        <XAxis
          dataKey="t"
          type="number"
          scale="time"
          domain={["dataMin", "dataMax"]}
          hide={compact}
          tickFormatter={(t) => fmtTime(new Date(t as number).toISOString())}
          tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
          stroke="hsl(var(--border))"
          minTickGap={48}
        />
        <YAxis
          domain={domain}
          hide={compact}
          width={40}
          tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
          stroke="hsl(var(--border))"
          tickFormatter={(v) => `${v}°`}
        />
        {!compact && (
          <Tooltip
            contentStyle={{
              background: "hsl(var(--card))",
              border: "1px solid hsl(var(--border))",
              borderRadius: 8,
              fontSize: 12,
              color: "hsl(var(--foreground))",
            }}
            labelFormatter={(t) => fmtTime(new Date(t as number).toISOString())}
            formatter={(v) => [fmtTemp(v as number), "Temp"]}
          />
        )}
        <Line
          type="monotone"
          dataKey="v"
          stroke="hsl(var(--chart-1))"
          strokeWidth={2}
          dot={false}
          isAnimationActive={false}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}
