"use client";
import { z } from "zod";
import { createApiClient } from "@/lib/api/client";
import { authClient, authParams, withAuthLock } from "@/lib/auth/session";
import { requireAccount } from "@/lib/profile";
export const goalStates = {
  draft: "Черновик",
  active: "Активна",
  paused: "Приостановлена",
  achieved: "Достигнута",
  not_achieved: "Завершена без достижения",
  cancelled: "Отменена",
} as const;
export const progressTypes = {
  milestones: "Этапы",
  quests: "Связанные задачи",
  metric: "Числовой результат",
} as const;
export const goalInput = z.object({
  title: z.string().trim().min(1, "Укажите название").max(200),
  motive: z.string().max(10000),
  criteria: z.string().trim().min(1, "Укажите критерий успеха").max(2000),
  area_id: z.uuid().nullable(),
  start_date: z.iso.date().nullable(),
  target_date: z.iso.date().nullable(),
  progress_type: z.enum(["milestones", "quests", "metric"]),
  start: z.number().min(0).max(1e9),
  target: z.number().positive().max(1e9),
  unit: z.string().max(40),
});
export const goalSchema = goalInput.extend({
  id: z.uuid(),
  state: z.enum(["draft", "active", "paused", "achieved", "not_achieved", "cancelled"]),
  progress: z.number().min(0).max(100),
  current: z.number(),
  version: z.number().int().positive(),
  overdue: z.boolean(),
  archived_at: z.iso.datetime({ offset: true }).nullable(),
  created_at: z.iso.datetime({ offset: true }),
  updated_at: z.iso.datetime({ offset: true }),
});
export const goalPage = z.object({
  items: z.array(goalSchema),
  total: z.number().int(),
  page: z.number().int(),
  page_size: z.number().int(),
});
export type Goal = z.infer<typeof goalSchema>;
export type GoalInput = z.infer<typeof goalInput>;
export const emptyGoal: GoalInput = {
  title: "",
  motive: "",
  criteria: "",
  area_id: null,
  start_date: null,
  target_date: null,
  progress_type: "milestones",
  start: 0,
  target: 1,
  unit: "",
};
export type GoalFilters = { state: string; search: string; area_id: string; archived: boolean };
export const emptyGoalFilters: GoalFilters = {
  state: "",
  search: "",
  area_id: "",
  archived: false,
};
export async function listGoals(page: number, filters: GoalFilters, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/goals", {
    signal,
    params: {
      query: {
        ...filters,
        archived: String(filters.archived),
        page: String(page),
        page_size: "25",
      },
    },
  });
  return goalPage.parse(r.data?.data);
}
export async function getGoal(id: string, signal?: AbortSignal) {
  await requireAccount();
  const r = await createApiClient().GET("/api/v1/goals/{id}", { signal, params: { path: { id } } });
  return goalSchema.parse(r.data?.data);
}
export async function saveGoal(
  input: GoalInput,
  reason: string,
  state: "draft" | "active",
  current?: Goal,
) {
  await requireAccount();
  return withAuthLock(async () => {
    const api = await authClient();
    const r = current
      ? await api.PATCH("/api/v1/goals/{id}", {
          params: {
            path: { id: current.id },
            header: { ...authParams().header, "If-Match": '"' + current.version + '"' },
          },
          body: { ...input, reason },
        })
      : await api.POST("/api/v1/goals", { params: authParams(), body: { ...input, state } });
    return goalSchema.parse(r.data?.data);
  });
}
