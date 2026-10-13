"use client";
import { z } from "zod";
import { rewardSchema } from "./progression";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth/provider";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
export const habitSchedule = z.object({
  frequency: z.enum(["daily", "weekly", "monthly"]),
  weekdays: z.array(z.number().int().min(1).max(7)),
  target: z.number().positive().max(1000000),
});
export const habitInput = z.object({
  starter_id: z.uuid().nullable().optional(),
  title: z.string().trim().min(1, "Укажите название").max(200),
  kind: z.enum(["binary", "count", "duration"]),
  unit: z.string().trim().min(1, "Укажите единицу").max(40),
  minimum: z.number().positive("Минимальный шаг должен быть положительным"),
  area_id: z.uuid().nullable(),
  starts_on: z.iso.date(),
  ends_on: z.iso.date().nullable(),
  schedule: habitSchedule,
});
export const habitPeriod = z.object({
  id: z.uuid(),
  key: z.string(),
  start: z.string(),
  end: z.string(),
  timezone: z.string(),
  schedule_version: z.number().int(),
  target: z.number(),
  amount: z.number(),
  progress: z.number().int(),
  state: z.enum(["open", "success", "missed", "rest"]),
  excluded: z.boolean(),
  emergency_reason: z.string(),
  emergency_at: z.string().nullable(),
  rest_reason: z.string(),
});
export const habitSchema = habitInput.extend({
  reward: rewardSchema.optional(),
  as_of: z.string(),
  archives: z.array(
    z.object({
      id: z.number().int(),
      archived_at: z.string(),
      restored_at: z.string().nullable(),
      archive_reason: z.string(),
      restore_reason: z.string().nullable(),
    }),
  ),
  id: z.uuid(),
  version: z.number().int(),
  timezone: z.string(),
  archived_at: z.string().nullable(),
  schedules: z.array(
    z.object({
      version: z.number().int(),
      effective_from: z.string(),
      schedule: habitSchedule,
      reason: z.string(),
    }),
  ),
  periods: z.array(habitPeriod),
  has_more: z.boolean(),
  stats: z.object({
    current: z.number().int(),
    best: z.number().int(),
    successful: z.number().int(),
    missed: z.number().int(),
    rate: z.number().int(),
    recovering: z.boolean(),
  }),
  pauses: z.array(
    z.object({
      id: z.uuid(),
      starts_on: z.string(),
      ends_on: z.string().nullable(),
      reason: z.string(),
      version: z.number().int(),
      created_at: z.string(),
    }),
  ),
});
export type Habit = z.infer<typeof habitSchema>;
export type HabitInput = z.infer<typeof habitInput>;
export type HabitPeriod = z.infer<typeof habitPeriod>;
export const habitLog = z.object({
  id: z.uuid(),
  period_id: z.uuid(),
  amount: z.number(),
  original_amount: z.number(),
  occurred_at: z.string(),
  accepted_at: z.string(),
  backfilled: z.boolean(),
  corrections: z.array(
    z.object({
      id: z.number().int(),
      before: z.number(),
      after: z.number(),
      reason: z.string(),
      created_at: z.string(),
    }),
  ),
});
export async function listHabits(archived = false, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/habits", {
    params: { query: { archived } },
    signal,
  });
  return z.array(habitSchema).parse(r.data?.data);
}
export function useHabits(archived = false) {
  const auth = useAuth();
  return useQuery({
    queryKey: ["account", auth.session?.account.id, "habits", archived],
    enabled: !!auth.session?.account.id,
    queryFn: ({ signal }) => listHabits(archived, signal),
    refetchInterval: (q) => (q.state.data?.some((h) => h.has_more) ? 1000 : 30000),
  });
}
export async function createHabit(input: HabitInput, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/habits", {
      params: { header: { ...authParams().header, "Idempotency-Key": key } },
      body: input,
    });
    return habitSchema.parse(r.data?.data);
  });
}
function params(h: Habit, key: string) {
  return {
    path: { id: h.id },
    header: { ...authParams().header, "If-Match": '"' + h.version + '"', "Idempotency-Key": key },
  };
}
export async function logHabit(
  h: Habit,
  input: { period_id: string; amount: number; occurred_at: string },
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/habits/{id}/logs", { params: params(h, key), body: input });
    return habitSchema.parse(r.data?.data);
  });
}
export async function pauseHabit(
  h: Habit,
  input: { starts_on: string; ends_on: string | null; reason: string },
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/habits/{id}/pauses", { params: params(h, key), body: input });
    return habitSchema.parse(r.data?.data);
  });
}
export async function emergencyHabit(
  h: Habit,
  input: { period_id: string; reason: string },
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/habits/{id}/emergency", {
      params: params(h, key),
      body: input,
    });
    return habitSchema.parse(r.data?.data);
  });
}
export async function resumeHabit(
  h: Habit,
  pause: string,
  input: { resumes_on: string; reason: string },
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/habits/{id}/pauses/{pause}/resume", {
      params: { ...params(h, key), path: { id: h.id, pause } },
      body: input,
    });
    return habitSchema.parse(r.data?.data);
  });
}
export async function getHabitLogs(h: Habit, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/habits/{id}/logs", {
    params: { path: { id: h.id } },
    signal,
  });
  return habitLogPage.parse(r.data?.data).items;
}
export async function correctHabitLog(
  h: Habit,
  log: string,
  input: { amount: number; reason: string },
  key: string,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/habits/{id}/logs/{log}/correct", {
      params: { ...params(h, key), path: { id: h.id, log } },
      body: input,
    });
    return habitSchema.parse(r.data?.data);
  });
}
export const habitLogPage = z.object({
  items: z.array(habitLog),
  next_cursor: z.string().nullable(),
});
export const habitPeriodPage = z.object({
  items: z.array(habitPeriod),
  next_cursor: z.string().nullable(),
});
export const habitScheduleChange = z.object({
  effective_from: z.iso.date(),
  schedule: habitSchedule,
  reason: z.string().trim().min(1, "Укажите причину").max(1000),
});
export const habitSchedulePreview = z.object({
  habit_id: z.uuid(),
  version: z.number().int(),
  effective_from: z.string(),
  current: habitSchedule,
  proposed: habitSchedule,
  first_start: z.string(),
  first_end: z.string(),
  reason: z.string(),
});
export type HabitScheduleChange = z.infer<typeof habitScheduleChange>;
export type HabitSchedulePreview = z.infer<typeof habitSchedulePreview>;
export async function getHabitLogPage(h: Habit, before?: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/habits/{id}/logs", {
    params: { path: { id: h.id }, query: { before } },
    signal,
  });
  return habitLogPage.parse(r.data?.data);
}
export async function getHabitPeriodPage(h: Habit, before?: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/habits/{id}/periods", {
    params: { path: { id: h.id }, query: { before } },
    signal,
  });
  return habitPeriodPage.parse(r.data?.data);
}
export async function previewHabitSchedule(h: Habit, input: HabitScheduleChange) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/habits/{id}/schedules/preview", {
      params: params(h, crypto.randomUUID()),
      body: input,
    });
    return habitSchedulePreview.parse(r.data?.data);
  });
}
export async function changeHabitSchedule(h: Habit, input: HabitScheduleChange, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = await api.POST("/api/v1/habits/{id}/schedules", {
      params: params(h, key),
      body: input,
    });
    return habitSchema.parse(r.data?.data);
  });
}
export async function archiveHabit(h: Habit, restore: boolean, reason: string, key: string) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = restore
      ? await api.POST("/api/v1/habits/{id}/restore", { params: params(h, key), body: { reason } })
      : await api.POST("/api/v1/habits/{id}/archive", { params: params(h, key), body: { reason } });
    return habitSchema.parse(r.data?.data);
  });
}

export const habitStarter = z.object({ id: z.uuid(), title: z.string() });
export async function getHabitStarter(signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/habits/starter", { signal });
  return habitStarter.nullable().parse(r.data?.data);
}

export async function getHabit(id: string) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/habits/{id}", { params: { path: { id } } });
  return habitSchema.parse(r.data?.data);
}
