"use client";
import { z } from "zod";
export const rewardSchema = z.object({
  entitlement_id: z.string(),
  ledger_id: z.string(),
  source_kind: z.string(),
  source_key: z.string(),
  economy_version: z.string(),
  kind: z.string(),
  requested_xp: z.number().int(),
  awarded_xp: z.number().int(),
  awarded_coins: z.number().int(),
  xp_delta: z.number().int(),
  coins_delta: z.number().int(),
  total_xp: z.number().int(),
  available_coins: z.number().int(),
  coin_correction: z.number().int(),
  level_before: z.number().int(),
  level_after: z.number().int(),
  reason: z.string(),
});
export type Reward = z.infer<typeof rewardSchema>;
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { requireAccount } from "@/lib/profile";
const integer = z.number().int();
export const progressionBalance = z.object({
  total_xp: integer.nonnegative(),
  coins: integer,
  available_coins: integer.nonnegative(),
  coin_correction: integer.nonnegative(),
  version: integer.positive(),
  economy_version: z.string(),
  as_of: z.iso.datetime({ offset: true }),
  level: z.object({
    number: integer.positive(),
    threshold: z.number().nonnegative().refine(Number.isInteger),
    next_threshold: z.number().positive().refine(Number.isInteger),
    inside: integer.nonnegative(),
    to_next: integer.positive(),
  }),
  quota: z.object({
    task_used: integer.nonnegative(),
    habit_used: integer.nonnegative(),
    challenge_used: integer.nonnegative(),
    total_used: integer.nonnegative(),
    task_cap: integer.positive(),
    habit_cap: integer.positive(),
    challenge_cap: integer.positive(),
    total_cap: integer.positive(),
    window_start: z.string(),
    window_end: z.string(),
  }),
  rules: z.object({
    task_xp: z.array(integer.positive()).length(5),
    habit_xp: integer.positive(),
    challenge_xp: integer.positive(),
    coins_divisor: integer.positive(),
  }),
});
export const progressionEntry = z.object({
  id: z.uuid(),
  sequence: integer.positive(),
  source_kind: z.enum(["task", "habit", "challenge", "spend", "refund", "bonus"]),
  source_key: z.string(),
  source_title: z.string(),
  source_url: z
    .string()
    .regex(/^\/app\//)
    .nullable(),
  kind: z.enum(["grant", "reversal", "restoration", "spend", "refund", "bonus"]),
  economy_version: z.string(),
  area_id: z.uuid().nullable(),
  area_name: z.string().nullable(),
  difficulty: integer,
  requested_xp: integer.nonnegative(),
  awarded_xp: integer.nonnegative(),
  xp_delta: integer,
  coins_delta: integer,
  reason: z.string(),
  occurred_at: z.iso.datetime({ offset: true }),
});
export const progressionHistory = z.object({
  items: z.array(progressionEntry),
  next_cursor: z.uuid().nullable(),
});
export async function getProgression(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/progression", { signal });
  return progressionBalance.parse(r.data?.data);
}
export async function getProgressionHistory(before?: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/progression/history", {
    params: { query: { before } },
    signal,
  });
  return progressionHistory.parse(r.data?.data);
}
export function useProgression() {
  const auth = useAuth();
  return useQuery({
    queryKey: ["account", auth.session?.account.id, "progression"],
    enabled: !!auth.session?.account.id,
    queryFn: ({ signal }) => getProgression(signal),
    staleTime: 0,
    refetchInterval: 10000,
  });
}
