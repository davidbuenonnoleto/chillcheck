import { lazy, Suspense } from "react";

export interface TrendPoint {
  t: number; // epoch ms (measurement time)
  v: number; // temperature °F
}

export interface TempTrendChartProps {
  data: TrendPoint[];
  min: number;
  max: number;
  compact?: boolean;
}

// recharts is ~250 KB gzipped and only used on the location page, so the actual
// chart lives in TempTrendChartImpl and is loaded lazily. The Suspense fallback
// reserves the chart's height to avoid layout shift while it streams in.
const Impl = lazy(() => import("./TempTrendChartImpl"));

export function TempTrendChart(props: TempTrendChartProps) {
  return (
    <Suspense fallback={<div style={{ height: props.compact ? 56 : 220 }} />}>
      <Impl {...props} />
    </Suspense>
  );
}
