import { lazy, Suspense } from "react";
import type { TrendBucket } from "@/lib/api";

export interface AnalyticsTrendChartProps {
  data: TrendBucket[];
}

// recharts is heavy and only used on a couple of pages, so the actual chart lives
// in AnalyticsTrendChartImpl and is loaded lazily (same split as TempTrendChart).
// The Suspense fallback reserves height to avoid layout shift while it streams in.
const Impl = lazy(() => import("./AnalyticsTrendChartImpl"));

export function AnalyticsTrendChart(props: AnalyticsTrendChartProps) {
  return (
    <Suspense fallback={<div style={{ height: 240 }} />}>
      <Impl {...props} />
    </Suspense>
  );
}
