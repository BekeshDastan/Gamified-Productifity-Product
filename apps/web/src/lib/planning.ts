"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
import { calendarEvent } from "@/lib/calendar";
export const planWindow = z.object({
  start: z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/),
  end: z.string().regex(/^(([01]\d|2[0-3]):[0-5]\d|24:00)$/),
});
export const planInput = z.object({
  available_minutes: z.number().int().min(0).max(1440),
  windows: z.array(planWindow).max(8),
  quest_ids: z.array(z.uuid()).max(100),
  top_ids: z.array(z.uuid()).max(3, "Можно выбрать до трёх главных задач"),
  rest: z.boolean(),
});
export const planItem = z.object({
  id: z.uuid(),
  title: z.string(),
  version: z.number().int(),
  estimated_minutes: z.number().int(),
  weight: z.number().int(),
  status: z.string(),
  archived: z.boolean(),
  completed_at: z.string().nullable(),
});
export const planProgress = z.object({
  committed: z.number().int(),
  completed: z.number().int(),
  total_weight: z.number().int(),
  completed_weight: z.number().int(),
  score: z.number().int().nullable(),
});
export const planAvailability = z.object({
  work_minutes: z.number(),
  busy_minutes: z.number(),
  free_minutes: z.number(),
  scheduled_minutes: z.number(),
  remaining_capacity: z.number(),
  planned_minutes: z.number(),
  remaining_minutes: z.number(),
  deficit_minutes: z.number(),
  free_slots: z.array(z.object({ start: z.string(), end: z.string() })),
  conflicts: z.array(
    z.object({ first: z.string(), second: z.string(), start: z.string(), end: z.string() }),
  ),
  more_conflicts: z.boolean(),
  unfit_ids: z.array(z.string()),
  unknown_ids: z.array(z.string()),
  blocks: z.array(calendarEvent),
  materializing: z.boolean(),
});
export const dayPlan = planInput.extend({
  id: z.string(),
  date: z.string(),
  timezone: z.string(),
  version: z.number().int(),
  confirmed_at: z.string().nullable(),
  snapshot: z
    .object({ input: planInput, items: z.array(planItem), blocks: z.array(calendarEvent) })
    .nullable(),
  items: z.array(planItem),
  changes: z.array(
    z.object({
      kind: z.string(),
      reason: z.string(),
      at: z.string(),
      before: z.record(z.string(), z.unknown()),
      after: z.record(z.string(), z.unknown()),
    }),
  ),
  progress: planProgress,
  availability: planAvailability.nullable(),
});
export type DayPlan = z.infer<typeof dayPlan>;
export type PlanInput = z.infer<typeof planInput>;
export async function getPlan(date: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/plans/day/{date}", {
    params: { path: { date } },
    signal,
  });
  return dayPlan.parse(r.data?.data);
}
export async function getPlanWeek(from: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/plans/week", {
    params: { query: { from } },
    signal,
  });
  return z.array(dayPlan).parse(r.data?.data);
}
function params(plan: DayPlan, key: string) {
  return {
    path: { date: plan.date },
    header: {
      ...authParams().header,
      "If-Match": '"' + plan.version + '"',
      "Idempotency-Key": key,
    },
  };
}
export async function savePlan(plan: DayPlan, input: PlanInput, reason: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = plan.confirmed_at
      ? await api.POST("/api/v1/plans/day/{date}/addendum", {
          params: params(plan, key),
          body: { input, reason },
        })
      : await api.PUT("/api/v1/plans/day/{date}", { params: params(plan, key), body: input });
    return dayPlan.parse(r.data?.data);
  });
}
export async function confirmPlan(plan: DayPlan, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/plans/day/{date}/confirm", {
      params: params(plan, key),
      body: {},
    });
    return dayPlan.parse(r.data?.data);
  });
}
export const blockInput = z.object({
  quest_id: z.uuid("Выберите задачу"),
  quest_version: z.number().int(),
  reason: z.string().trim().min(1, "Укажите причину").max(1000),
  start: z.string().min(1),
  end: z.string().min(1),
});
export async function addPlanBlock(plan: DayPlan, input: z.infer<typeof blockInput>, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/plans/day/{date}/blocks", {
      params: params(plan, key),
      body: input,
    });
    return calendarEvent.parse(r.data?.data);
  });
}
export async function movePlanQuest(
  plan: DayPlan,
  input: { quest_id: string; quest_version: number; reason: string },
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/plans/day/{date}/move", {
      params: params(plan, key),
      body: input,
    });
    return dayPlan.parse(r.data?.data);
  });
}
