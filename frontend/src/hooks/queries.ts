import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type UnitKind } from "@/lib/api";

export function useLocations() {
  return useQuery({ queryKey: ["locations"], queryFn: api.listLocations });
}

export function useAnalytics(from?: string, to?: string, locationId?: string) {
  return useQuery({
    queryKey: ["analytics", from, to, locationId],
    queryFn: () => api.getAnalytics(from, to, locationId),
  });
}

export function useLocationStatus(locationId: string) {
  return useQuery({
    queryKey: ["status", locationId],
    queryFn: () => api.locationStatus(locationId),
    refetchInterval: 60_000, // refresh the board every minute
  });
}

export function useUnits(locationId: string) {
  return useQuery({
    queryKey: ["units", locationId],
    queryFn: () => api.listUnits(locationId),
  });
}

export function useReadings(locationId: string, from?: string, to?: string) {
  return useQuery({
    queryKey: ["readings", locationId, from, to],
    queryFn: () => api.listReadings(locationId, from, to),
  });
}

export function useCreateLocation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; timezone?: string }) => api.createLocation(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["locations"] }),
  });
}

export function useCreateUnit(locationId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      name: string;
      kind: UnitKind;
      min_temp_f: number;
      max_temp_f: number;
      log_interval_minutes: number;
    }) => api.createUnit(locationId, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["units", locationId] });
      qc.invalidateQueries({ queryKey: ["status", locationId] });
    },
  });
}

export function useLogReading(locationId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { unit_id: string; temp_f: number; note?: string }) => api.createReading(body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["status", locationId] });
      qc.invalidateQueries({ queryKey: ["readings", locationId] });
    },
  });
}

export function useGateways(locationId: string) {
  return useQuery({
    queryKey: ["gateways", locationId],
    queryFn: () => api.listGateways(locationId),
  });
}

export function useCreateGateway(locationId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string }) => api.createGateway(locationId, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["gateways", locationId] }),
  });
}

export function useSetUnitSensor(locationId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ unitId, mac }: { unitId: string; mac: string }) => api.setUnitSensor(unitId, mac),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["units", locationId] });
      qc.invalidateQueries({ queryKey: ["status", locationId] });
    },
  });
}

export function useAlerts(locationId: string) {
  return useQuery({
    queryKey: ["alerts", locationId],
    queryFn: () => api.listAlerts(locationId),
    refetchInterval: 60_000,
  });
}

export function useCorrectiveActions(alertId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["corrective-actions", alertId],
    queryFn: () => api.listCorrectiveActions(alertId),
    enabled,
  });
}

export function useAddCorrectiveAction(alertId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { action: string; disposition: string; note: string }) =>
      api.addCorrectiveAction(alertId, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["corrective-actions", alertId] });
      qc.invalidateQueries({ queryKey: ["alerts"] });
    },
  });
}

export function useBilling() {
  return useQuery({ queryKey: ["billing"], queryFn: api.getBilling });
}

export function useIntegrity() {
  return useQuery({ queryKey: ["integrity"], queryFn: api.getIntegrity });
}

export function useUsers() {
  return useQuery({ queryKey: ["users"], queryFn: api.listUsers });
}

export function useInvites() {
  return useQuery({ queryKey: ["invites"], queryFn: api.listInvites });
}

export function useCreateInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { email: string; role: string }) => api.createInvite(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["invites"] }),
  });
}

export function useRevokeInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.revokeInvite(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["invites"] }),
  });
}
