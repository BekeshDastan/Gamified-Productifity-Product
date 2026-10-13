"use client";
import { z } from "zod";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
export const bossSchema = z.object({
  id: z.uuid(),
  goal_id: z.uuid(),
  title: z.string(),
  state: z.enum(["active", "paused", "closed"]),
  status: z.string(),
  version: z.number().int().positive(),
  goal_version: z.number().int().positive(),
  hp: z.number().min(0).max(100),
  progress: z.number().min(0).max(100),
  stages: z.array(
    z.object({
      id: z.uuid(),
      title: z.string(),
      required: z.boolean(),
      progress: z.number().min(0).max(100),
      source_url: z.string().regex(/^\/app\//),
    }),
  ),
  won_at: z.string().nullable(),
});
export type Boss = z.infer<typeof bossSchema>;
export const monthlyBoss = z.object({
  month: z.string(),
  timezone: z.string(),
  version: z.number().int().positive(),
  boss_id: z.uuid().nullable(),
  boss: bossSchema.nullable(),
});
export type MonthlyBoss = z.infer<typeof monthlyBoss>;
export const monthlyHistory = z.object({
  items: z.array(
    z.object({
      id: z.number().int(),
      month: z.string(),
      timezone: z.string(),
      boss_id: z.uuid().nullable(),
      title: z.string(),
      reason: z.string(),
      created_at: z.string(),
    }),
  ),
  next_cursor: z.string().nullable(),
});
export async function getBosses(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/bosses", { signal });
  return z.array(bossSchema).parse(r.data?.data);
}
export function useBosses() {
  const auth = useAuth();
  return useQuery({
    queryKey: ["account", auth.session?.account.id, "bosses"],
    enabled: !!auth.session?.account.id,
    queryFn: ({ signal }) => getBosses(signal),
    staleTime: 0,
    refetchInterval: 15000,
  });
}
export async function getMonthlyBoss(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/bosses/monthly", { signal });
  return monthlyBoss.parse(r.data?.data);
}
export function useMonthlyBoss() {
  const auth = useAuth();
  return useQuery({
    queryKey: ["account", auth.session?.account.id, "monthly-boss"],
    enabled: !!auth.session?.account.id,
    queryFn: ({ signal }) => getMonthlyBoss(signal),
    staleTime: 0,
    refetchInterval: 15000,
  });
}
export async function createBoss(goal_id: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.POST("/api/v1/bosses", {
        params: { header: { ...authParams().header, "Idempotency-Key": key } },
        body: { goal_id },
      });
    return bossSchema.parse(r.data?.data);
  });
}
export async function actOnBoss(
  b: Boss,
  action: "pause" | "resume" | "close",
  reason: string,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PATCH("/api/v1/bosses/{id}", {
        params: {
          path: { id: b.id },
          header: {
            ...authParams().header,
            "Idempotency-Key": key,
            "If-Match": '"' + b.version + '"',
          },
        },
        body: { action, reason },
      });
    return bossSchema.parse(r.data?.data);
  });
}
export async function selectMonthlyBoss(
  version: number,
  month: string,
  boss_id: string | null,
  reason: string,
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient(),
      r = await api.PUT("/api/v1/bosses/monthly", {
        params: {
          header: {
            ...authParams().header,
            "Idempotency-Key": key,
            "If-Match": '"' + version + '"',
          },
        },
        body: { boss_id, reason, month },
      });
    return monthlyBoss.parse(r.data?.data);
  });
}
export async function getMonthlyHistory(before?: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/bosses/monthly/history", {
    params: { query: { before } },
    signal,
  });
  return monthlyHistory.parse(r.data?.data);
}
