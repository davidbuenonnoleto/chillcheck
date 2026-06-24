import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { TrendBucket } from "@/lib/api";

// Renders the daily "% of readings in range" trend across the selected window.
// Days with no readings show 0% (the store fills gaps). Uses the chart-1 token so
// it themes with light/dark; a reference line marks the 90% target. Lazy-loaded
// via AnalyticsTrendChart so recharts stays out of the main bundle.
export default function AnalyticsTrendChartImpl({ data }: { data: TrendBucket[] }) {
  if (data.length < 2) {
    return <p className="py-8 text-center text-sm text-muted-foreground">Not enough data to chart yet.</p>;
  }

  const fmtDate = (d: string) => {
    const [, m, day] = d.split("-");
    return `${Number(m)}/${Number(day)}`;
  };

  return (
    <ResponsiveContainer width="100%" height={240}>
      <AreaChart data={data} margin={{ top: 8, right: 12, bottom: 4, left: 0 }}>
        <defs>
          <linearGradient id="inRangeFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="hsl(var(--chart-1))" stopOpacity={0.3} />
            <stop offset="100%" stopColor="hsl(var(--chart-1))" stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
        <ReferenceLine y={90} stroke="hsl(var(--ok))" strokeDasharray="4 4" />
        <XAxis
          dataKey="date"
          tickFormatter={fmtDate}
          tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
          stroke="hsl(var(--border))"
          minTickGap={32}
        />
        <YAxis
          domain={[0, 100]}
          width={48}
          tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
          stroke="hsl(var(--border))"
          tickFormatter={(v) => `${v}%`}
        />
        <Tooltip
          contentStyle={{
            background: "hsl(var(--card))",
            border: "1px solid hsl(var(--border))",
            borderRadius: 8,
            fontSize: 12,
            color: "hsl(var(--foreground))",
          }}
          labelFormatter={(d) => fmtDate(d as string)}
          formatter={(v, _n, item) => {
            const b = item.payload as TrendBucket;
            return [`${v}% (${b.in_range}/${b.total})`, "In range"];
          }}
        />
        <Area
          type="monotone"
          dataKey="pct"
          stroke="hsl(var(--chart-1))"
          strokeWidth={2}
          fill="url(#inRangeFill)"
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}
