const BASE = import.meta.env.VITE_API_URL ?? "http://localhost:8080";
const TOKEN_KEY = "chillcheck_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}
export function setToken(t: string) {
  localStorage.setItem(TOKEN_KEY, t);
}
export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

// ---- types (mirror the Go JSON) ----

export interface User {
  id: string;
  org_id: string;
  name: string;
  email: string;
  role: string;
}
export interface Organization {
  id: string;
  name: string;
}
export interface Location {
  id: string;
  org_id: string;
  name: string;
  timezone: string;
  unit_count: number;
  created_at: string;
}
export type UnitKind = "fridge" | "freezer" | "hot_hold";
export interface Unit {
  id: string;
  location_id: string;
  name: string;
  kind: UnitKind;
  min_temp_f: number;
  max_temp_f: number;
  log_interval_minutes: number;
  sensor_mac: string | null;
}
export interface Gateway {
  id: string;
  org_id: string;
  location_id: string;
  name: string;
  key_prefix: string;
  last_seen_at: string | null;
  created_at: string;
}
export type AlertKind = "out_of_range" | "overdue";
export interface Alert {
  id: string;
  org_id: string;
  unit_id: string;
  unit_name: string;
  kind: AlertKind;
  status: "open" | "resolved";
  detail: string;
  opened_at: string;
  notified_at: string | null;
  resolved_at: string | null;
  corrective_action_count: number;
}
export interface CorrectiveAction {
  id: string;
  org_id: string;
  alert_id: string;
  action: string;
  disposition: string;
  note: string;
  recorded_by: string | null;
  recorded_by_name: string;
  created_at: string;
}
export interface BillingStatus {
  billing_enabled: boolean;
  status: string; // trialing | active | past_due | canceled | ...
  plan: string | null;
  trial_end: string | null;
  current_period_end: string | null;
  entitled: boolean;
}
export interface Invite {
  id: string;
  org_id: string;
  email: string;
  role: string;
  expires_at: string;
  accepted_at: string | null;
  created_at: string;
}
export interface InviteInfo {
  org_name: string;
  email: string;
  role: string;
}
export interface ChainStatus {
  ok: boolean;
  count: number;
  first_seq: number;
  last_seq: number;
  broken_at_seq: number | null;
}
export interface Reading {
  id: string;
  unit_id: string;
  unit_name: string;
  temp_f: number;
  source: string;
  note: string;
  recorded_by: string | null;
  recorded_at: string;
}
export type UnitStatusValue = "ok" | "out_of_range" | "overdue" | "no_data";
export interface UnitStatus extends Unit {
  latest_temp_f: number | null;
  latest_at: string | null;
  latest_by: string | null;
  status: UnitStatusValue;
}
export interface LocationStatus {
  location: Location;
  units: UnitStatus[];
}
export interface TrendBucket {
  date: string; // YYYY-MM-DD (UTC)
  in_range: number;
  total: number;
  pct: number;
}
export interface UnitStat {
  unit_id: string;
  unit_name: string;
  location_name: string;
  total_readings: number;
  in_range_pct: number;
  deviations: number;
  avg_temp_f: number | null;
  min_temp_f: number | null;
  max_temp_f: number | null;
  last_reading_at: string | null;
}
export interface Analytics {
  from: string;
  to: string;
  total_readings: number;
  in_range_pct: number;
  deviations: number;
  overdue_events: number;
  undocumented_deviations: number;
  trend: TrendBucket[];
  units: UnitStat[];
}

// ---- core request helper ----

function analyticsQuery(from?: string, to?: string, locationId?: string): string {
  const q = new URLSearchParams();
  if (from) q.set("from", from);
  if (to) q.set("to", to);
  if (locationId) q.set("location_id", locationId);
  return q.toString();
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { ...(opts.headers as Record<string, string>) };
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  if (opts.body) headers["Content-Type"] = "application/json";

  const res = await fetch(`${BASE}${path}`, { ...opts, headers });
  if (res.status === 401) {
    clearToken();
    throw new ApiError(401, "Your session expired. Please sign in again.");
  }
  if (!res.ok) {
    let msg = `Request failed (${res.status})`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* keep default */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

// ---- endpoints ----

export const api = {
  register: (body: { org_name: string; name: string; email: string; password: string }) =>
    request<{ token: string; user: User }>("/api/auth/register", { method: "POST", body: JSON.stringify(body) }),

  login: (body: { email: string; password: string }) =>
    request<{ token: string; user: User }>("/api/auth/login", { method: "POST", body: JSON.stringify(body) }),

  me: () => request<{ user: User; organization: Organization }>("/api/me"),

  listLocations: () => request<Location[]>("/api/locations"),

  createLocation: (body: { name: string; timezone?: string }) =>
    request<Location>("/api/locations", { method: "POST", body: JSON.stringify(body) }),

  getLocation: (id: string) => request<Location>(`/api/locations/${id}`),

  listUnits: (locationId: string) => request<Unit[]>(`/api/locations/${locationId}/units`),

  createUnit: (
    locationId: string,
    body: { name: string; kind: UnitKind; min_temp_f: number; max_temp_f: number; log_interval_minutes: number }
  ) => request<Unit>(`/api/locations/${locationId}/units`, { method: "POST", body: JSON.stringify(body) }),

  locationStatus: (id: string) => request<LocationStatus>(`/api/locations/${id}/status`),

  listGateways: (locationId: string) => request<Gateway[]>(`/api/locations/${locationId}/gateways`),

  createGateway: (locationId: string, body: { name: string }) =>
    request<{ gateway: Gateway; key: string }>(`/api/locations/${locationId}/gateways`, {
      method: "POST",
      body: JSON.stringify(body),
    }),

  setUnitSensor: (unitId: string, mac: string) =>
    request<Unit>(`/api/units/${unitId}/sensor`, { method: "PUT", body: JSON.stringify({ mac }) }),

  listAlerts: (locationId: string) => request<Alert[]>(`/api/locations/${locationId}/alerts`),

  listCorrectiveActions: (alertId: string) =>
    request<CorrectiveAction[]>(`/api/alerts/${alertId}/corrective-actions`),

  addCorrectiveAction: (alertId: string, body: { action: string; disposition: string; note: string }) =>
    request<CorrectiveAction>(`/api/alerts/${alertId}/corrective-actions`, {
      method: "POST",
      body: JSON.stringify(body),
    }),

  getBilling: () => request<BillingStatus>("/api/billing"),

  billingCheckout: () => request<{ url: string }>("/api/billing/checkout", { method: "POST" }),

  billingPortal: () => request<{ url: string }>("/api/billing/portal", { method: "POST" }),

  listUsers: () => request<User[]>("/api/users"),

  listInvites: () => request<Invite[]>("/api/invites"),

  createInvite: (body: { email: string; role: string }) =>
    request<{ invite: Invite; accept_url: string }>("/api/invites", { method: "POST", body: JSON.stringify(body) }),

  revokeInvite: (id: string) => request<void>(`/api/invites/${id}`, { method: "DELETE" }),

  getInvite: (token: string) => request<InviteInfo>(`/api/invites/lookup?token=${encodeURIComponent(token)}`),

  acceptInvite: (body: { token: string; name: string; password: string }) =>
    request<{ token: string; user: User }>("/api/invites/accept", { method: "POST", body: JSON.stringify(body) }),

  forgotPassword: (email: string) =>
    request<{ ok: boolean }>("/api/auth/forgot", { method: "POST", body: JSON.stringify({ email }) }),

  resetPassword: (token: string, password: string) =>
    request<{ ok: boolean }>("/api/auth/reset", { method: "POST", body: JSON.stringify({ token, password }) }),

  getIntegrity: () => request<ChainStatus>("/api/integrity"),

  getAnalytics: (from?: string, to?: string, locationId?: string) => {
    const q = analyticsQuery(from, to, locationId);
    return request<Analytics>(`/api/analytics${q ? `?${q}` : ""}`);
  },

  analyticsCsvPath: (from?: string, to?: string, locationId?: string) => {
    const q = analyticsQuery(from, to, locationId);
    return `${BASE}/api/analytics/export.csv${q ? `?${q}` : ""}`;
  },

  createReading: (body: { unit_id: string; temp_f: number; note?: string }) =>
    request<Reading>("/api/readings", { method: "POST", body: JSON.stringify(body) }),

  listReadings: (locationId: string, from?: string, to?: string) => {
    const q = new URLSearchParams({ location_id: locationId });
    if (from) q.set("from", from);
    if (to) q.set("to", to);
    return request<Reading[]>(`/api/readings?${q.toString()}`);
  },

  // Returns a full URL; the report opens in a new tab with the token in a header
  // is not possible for a plain link, so we fetch it as a blob (see LocationPage).
  reportPath: (locationId: string, from: string, to: string) => {
    const q = new URLSearchParams({ location_id: locationId, from, to });
    return `${BASE}/api/reports/compliance.pdf?${q.toString()}`;
  },
};

export async function fetchReportBlob(locationId: string, from: string, to: string): Promise<Blob> {
  const token = getToken();
  const res = await fetch(api.reportPath(locationId, from, to), {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) throw new ApiError(res.status, "Could not generate the report.");
  return res.blob();
}

// The JWT lives in a header, not a cookie, so we can't use a plain download link —
// fetch the CSV as a blob with the auth header (see AnalyticsPage). The filename
// comes from the server's Content-Disposition so it matches a direct download.
export async function fetchAnalyticsCsv(
  from?: string,
  to?: string,
  locationId?: string,
): Promise<{ blob: Blob; filename: string }> {
  const token = getToken();
  const res = await fetch(api.analyticsCsvPath(from, to, locationId), {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) throw new ApiError(res.status, "Could not export the CSV.");
  const match = /filename="?([^"]+)"?/.exec(res.headers.get("Content-Disposition") ?? "");
  return { blob: await res.blob(), filename: match?.[1] ?? `analytics-${from ?? "export"}.csv` };
}
